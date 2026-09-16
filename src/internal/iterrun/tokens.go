package iterrun

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// Token accounting reads the one place the numbers actually exist: the
// session transcripts Claude Code writes under ~/.claude/projects/. The
// hook event log (events.jsonl) records WHAT ran and for how long, but a
// PreToolUse/PostToolUse payload carries no usage figures at all, so the
// timeline and the token panel are built from two different sources joined
// on the ids both happen to carry — session_id and agent_id.
//
// The join is exact, not heuristic. A coordinator's transcript is
// <project-slug>/<session-id>.jsonl; an in-process teammate's is
// <project-slug>/<session-id>/subagents/agent-<agent-id>.jsonl, and that
// <agent-id> is byte-for-byte the agent_id the hook already recorded for
// every tool call that teammate made. So "which lane spent this" needs no
// inference: the filename says it.

// Price ratios inside one model's own pricing. Anthropic publishes a cache
// read at 10% of a fresh input token, a cache write at 125%, and an output
// token at 5x. Weighting by these turns four incomparable counters into one
// number that can be shared out as a percentage — which is the only form
// this file reports, deliberately. Absolute dollars are NOT derived from
// these ratios: they are close enough to rank lanes honestly and not close
// enough to bill against, and the real figure is already computed for us
// (see SessionCostUSD).
// Tick labels for wake-ups that are not slash commands.
const (
	tickHuman = "(you)"
	tickNudge = "(nudge)" // the coordinator asking a team for something
	tickTask  = "(task)"  // a background task reporting back
)

const (
	weightInput      = 1.00
	weightCacheWrite = 1.25
	weightCacheRead  = 0.10
	weightOutput     = 5.00
)

// TokenUsage is one lane's, bucket's or tick's raw counters. Kept as four
// separate fields because they are four different prices — collapsing them
// into one "tokens" number is what makes a cache-heavy coordinator look
// twenty times more expensive than it is.
type TokenUsage struct {
	Input      int64 `json:"input"`
	CacheRead  int64 `json:"cache_read"`
	CacheWrite int64 `json:"cache_write"`
	Output     int64 `json:"output"`
	Thinking   int64 `json:"thinking"` // subset of Output, not an addition to it
	Messages   int   `json:"messages"`
}

func (u *TokenUsage) add(o TokenUsage) {
	u.Input += o.Input
	u.CacheRead += o.CacheRead
	u.CacheWrite += o.CacheWrite
	u.Output += o.Output
	u.Thinking += o.Thinking
	u.Messages += o.Messages
}

// Context is what one request had to be sent: everything that was not
// generated. This is the number that grows without bound as a session runs,
// and the reason a long coordinator costs more than the work it does.
func (u TokenUsage) Context() int64 { return u.Input + u.CacheRead + u.CacheWrite }

// Weighted is Context+Output converted to comparable units by the price
// ratios above. Only ever reported as a share of a total.
func (u TokenUsage) Weighted() float64 {
	return weightInput*float64(u.Input) +
		weightCacheWrite*float64(u.CacheWrite) +
		weightCacheRead*float64(u.CacheRead) +
		weightOutput*float64(u.Output)
}

// LaneTokens is one row of the "who spent it" table: the coordinator, or
// one team. Keyed the same way the timeline keys its rows, so the two
// tables on the page line up row for row.
type LaneTokens struct {
	Key    string     `json:"key"` // "" for the coordinator, else the team name
	Label  string     `json:"label"`
	Usage  TokenUsage `json:"usage"`
	Share  float64    `json:"share"` // percent of the plan's weighted total
	Models []string   `json:"models"`
	// PeakContext is the largest single request this lane ever sent. It
	// is the number that predicts a compaction or a context-limit stall,
	// which totals cannot: a lane can spend enormously in many small
	// requests and never come near the wall.
	PeakContext int64 `json:"peak_context"`
	Missing     bool  `json:"missing"` // lane had hook activity but no transcript was found
}

// ToolTokens is one row of the "what it was spent on" table.
//
// Two genuinely different costs are tracked per tool, because they answer
// two different questions:
//
//   - Emitting is output tokens the model spent writing the call itself —
//     the arguments of an Edit, the script in a Bash. Cheap to attribute
//     and exactly correct: it is the output of the message that made the
//     call, divided among the calls that message made.
//   - ContextGrowth is how much bigger the next request became because
//     this call happened — the tool's result, measured by what it cost to
//     send it back. This is the real price of reading: a Read costs almost
//     no output and then rides along in the context of every subsequent
//     turn. Measured as the growth in context between consecutive requests
//     in the same lane, attributed to the calls the earlier request made.
//     Split evenly when one message made several calls, which is why a
//     single figure here is an attribution and not a receipt.
type ToolTokens struct {
	Tool          string  `json:"tool"`
	Calls         int     `json:"calls"`
	Emitting      int64   `json:"emitting"`
	ContextGrowth int64   `json:"context_growth"`
	Share         float64 `json:"share"` // percent of all attributed growth+emitting
}

// TickTokens is one wake-up: a slash command or a human prompt, and
// everything the lane spent before the next one arrived. A `/iterate` tick
// is what a cron or /loop firing looks like from inside the transcript, so
// this is what answers "what does one loop iteration cost".
type TickTokens struct {
	At    time.Time  `json:"at"`
	Label string     `json:"label"` // "/iterate", "(nudge)", "(you)", ...
	Lane  string     `json:"lane"`
	Usage TokenUsage `json:"usage"`
	Tools int        `json:"tools"` // tool calls made; 0 means the wake-up did nothing
}

// TickGroup aggregates every wake-up of one kind. This is the form the
// question "what does a cron tick cost me" actually has an answer in: one
// tick is noise, 131 of them is the bill.
//
// Two kinds of wake-up produce no work, and conflating them would turn a
// real finding into a scary-looking artifact:
//
//   - Coalesced: the prompt arrived while the previous turn was still
//     running, so it never got its own request. It cost nothing. On a
//     one-minute /loop against multi-minute turns most firings are these,
//     which is why a median taken over all of them reads as zero.
//   - Idle: the wake-up DID run — it re-sent the entire context, paid for
//     it, and made no tool call. This is the expensive one, and it is
//     invisible in any per-message or per-tool view.
type TickGroup struct {
	Label     string     `json:"label"`
	Count     int        `json:"count"`
	Ran       int        `json:"ran"`
	Idle      int        `json:"idle"`
	Coalesced int        `json:"coalesced"`
	Usage     TokenUsage `json:"usage"`
	Median    int64      `json:"median"` // median context of the wake-ups that ran
	Share     float64    `json:"share"`
}

// PlanTokens is everything the token panel renders for one plan.
type PlanTokens struct {
	Plan     string       `json:"plan"`
	Lanes    []LaneTokens `json:"lanes"`
	Tools    []ToolTokens `json:"tools"`
	Ticks    []TickTokens `json:"ticks"`
	Groups   []TickGroup  `json:"groups"`
	Total    TokenUsage   `json:"total"`
	Sessions []string     `json:"sessions"`

	// CostUSD is Claude Code's OWN figure for the sessions this plan ran
	// in, read from the transcript's cost-state records — never a number
	// this package computes. It covers the whole session, which for a
	// long-lived coordinator is more than this one plan, so it is reported
	// as session cost and never divided across lanes. CostPartial says so.
	CostUSD     float64 `json:"cost_usd"`
	CostPartial bool    `json:"cost_partial"`
	HaveCost    bool    `json:"have_cost"`
}

// transcriptSample is one retained record from a transcript: an assistant
// request's usage, or a prompt that starts a tick. Compact on purpose —
// a 178 MB transcript reduces to a few thousand of these, which is what
// makes windowing a plan out of a months-long session cheap.
type transcriptSample struct {
	At      time.Time
	Model   string
	Usage   TokenUsage
	Ctx     int64    // context this request paid for; diffed to get growth
	Tools   []string // tool_use blocks this message emitted
	Tick    string   // non-empty: this sample is a prompt, not a request
	Skipped bool     // a duplicate transcript write, already counted
}

// transcriptDoc is the subset of a transcript line this file reads.
type transcriptDoc struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	RequestID string          `json:"requestId"`
	Message   json.RawMessage `json:"message"`
	// cost-state fields
	TotalCostUSD float64 `json:"totalCostUSD"`
}

type transcriptMessage struct {
	ID      string `json:"id"`
	Model   string `json:"model"`
	Content []struct {
		Type string          `json:"type"`
		Name string          `json:"name"`
		Text string          `json:"text"`
		Raw  json.RawMessage `json:"-"`
	} `json:"content"`
	Usage *struct {
		InputTokens         int64 `json:"input_tokens"`
		CacheReadTokens     int64 `json:"cache_read_input_tokens"`
		CacheCreationTokens int64 `json:"cache_creation_input_tokens"`
		OutputTokens        int64 `json:"output_tokens"`
		OutputDetails       struct {
			ThinkingTokens int64 `json:"thinking_tokens"`
		} `json:"output_tokens_details"`
	} `json:"usage"`
}

// contentText flattens a user message's content to plain text. A prompt
// arrives either as a bare string or as a list of text blocks.
func contentText(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	var m struct {
		Content json.RawMessage `json:"content"`
	}
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	if json.Unmarshal(m.Content, &s) == nil {
		return s
	}
	var blocks []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	}
	if json.Unmarshal(m.Content, &blocks) != nil {
		return ""
	}
	var b strings.Builder
	for _, bl := range blocks {
		if bl.Type == "text" || bl.Type == "" {
			b.WriteString(bl.Text)
			b.WriteByte('\n')
		}
	}
	return b.String()
}

// promptTick classifies a prompt into the wake-up it represents, or "" when
// it is harness bookkeeping that must not start a tick at all.
//
// Every shape here was read off real transcripts, and the distinctions
// matter more than they look:
//
//   - A cron or /loop firing arrives as a BARE "/iterate" — no
//     <command-name> wrapper, nothing else on the line. Those are the
//     131-per-run wake-ups that make up most of an unattended plan's
//     spend, and treating them as ordinary human prompts is what hid them.
//   - A slash command the user typed arrives wrapped in <command-name>,
//     immediately followed by a SECOND user message carrying the skill
//     body ("Base directory for this skill: ..."). Counting that second
//     message would split one wake-up into two and halve both.
//   - A teammate's own transcript has no human prompts at all; its user
//     messages are the coordinator's dispatch and its later nudges, which
//     is a real and separately interesting cost.
func promptTick(text string) string {
	t := strings.TrimSpace(text)
	if t == "" {
		return ""
	}

	const open, close = "<command-name>", "</command-name>"
	if i := strings.Index(t, open); i >= 0 {
		rest := t[i+len(open):]
		if j := strings.Index(rest, close); j >= 0 {
			if name := strings.TrimSpace(rest[:j]); name != "" {
				return name
			}
		}
	}

	// The skill body that follows an expanded slash command, and the
	// reminder blocks the harness injects — neither is a wake-up.
	if strings.HasPrefix(t, "Base directory for this skill:") ||
		strings.HasPrefix(t, "<system-reminder>") ||
		strings.HasPrefix(t, "<local-command-caveat>") ||
		strings.HasPrefix(t, "This session is being continued from a previous") {
		return ""
	}

	if strings.HasPrefix(t, "<teammate-message") {
		return tickNudge
	}
	if strings.HasPrefix(t, "<task-notification") {
		return tickTask
	}

	// A bare slash command on its own line: the cron/loop case.
	if strings.HasPrefix(t, "/") {
		line := t
		if i := strings.IndexByte(line, '\n'); i >= 0 {
			line = line[:i]
		}
		if len(line) <= 64 {
			if sp := strings.IndexByte(line, ' '); sp > 0 {
				return line[:sp]
			}
			return line
		}
	}
	return tickHuman
}

// scanTranscriptFrom parses new lines of a transcript starting at offset,
// returning the samples found, the last cost-state total seen, and the
// offset just past the last COMPLETE line. Stopping at the last newline is
// what makes this safe to run against a file Claude Code is still
// appending to: a torn final line is left for the next pass.
func scanTranscriptFrom(path string, offset int64, idx map[string]int, base []transcriptSample) (out []transcriptSample, cost float64, haveCost bool, next int64, err error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, false, offset, err
	}
	defer f.Close()

	if offset > 0 {
		if _, err := f.Seek(offset, 0); err != nil {
			return nil, 0, false, offset, err
		}
	}
	next = offset

	// Read newline-terminated lines only. bufio.Scanner would hand back a
	// final unterminated line as though it were complete, which is
	// exactly what a transcript being appended to right now looks like:
	// the offset would advance past a half-written record and the rest of
	// it would never be read. So a line without its '\n' is left where it
	// is for the next pass, and the offset stops short of it.
	br := bufio.NewReaderSize(f, 256*1024)
	for {
		line, _ := br.ReadString('\n')
		if !strings.HasSuffix(line, "\n") {
			// Torn tail (or clean EOF with nothing pending): stop here.
			break
		}
		raw := []byte(strings.TrimRight(line, "\n"))
		next += int64(len(line))

		interesting := bytes.Contains(raw, []byte(`"usage"`)) ||
			bytes.Contains(raw, []byte(`"cost-state"`)) ||
			(bytes.Contains(raw, []byte(`"type":"user"`)) && !bytes.Contains(raw, []byte(`"tool_result"`)))
		if !interesting {
			continue
		}

		var doc transcriptDoc
		if json.Unmarshal(raw, &doc) != nil {
			continue
		}

		switch doc.Type {
		case "cost-state":
			cost, haveCost = doc.TotalCostUSD, true
			continue
		case "user":
			at, ok := parseTranscriptTime(doc.Timestamp)
			if !ok {
				continue
			}
			if label := promptTick(contentText(doc.Message)); label != "" {
				out = append(out, transcriptSample{At: at, Tick: label})
			}
			continue
		case "assistant":
		default:
			continue
		}

		var msg transcriptMessage
		if json.Unmarshal(doc.Message, &msg) != nil || msg.Usage == nil {
			continue
		}
		at, ok := parseTranscriptTime(doc.Timestamp)
		if !ok {
			continue
		}

		// Claude Code also writes synthetic assistant records — an
		// interruption, an API error placeholder — which carry a usage
		// object of all zeros. They are not requests and were never
		// billed, so counting them inflates a lane's message count and
		// drags every per-tick median to zero. Confirmed live: 373 of
		// them in one coordinator transcript, which made 368 cron
		// wake-ups look like they had run and done nothing when they had
		// simply not run.
		if msg.Usage.InputTokens == 0 && msg.Usage.CacheReadTokens == 0 &&
			msg.Usage.CacheCreationTokens == 0 && msg.Usage.OutputTokens == 0 {
			continue
		}

		s := transcriptSample{
			At:    at,
			Model: msg.Model,
			Usage: TokenUsage{
				Input:      msg.Usage.InputTokens,
				CacheRead:  msg.Usage.CacheReadTokens,
				CacheWrite: msg.Usage.CacheCreationTokens,
				Output:     msg.Usage.OutputTokens,
				Thinking:   msg.Usage.OutputDetails.ThinkingTokens,
				Messages:   1,
			},
		}
		s.Ctx = s.Usage.Context()
		for _, c := range msg.Content {
			if c.Type == "tool_use" && c.Name != "" {
				s.Tools = append(s.Tools, c.Name)
			}
		}

		// Claude Code rewrites an assistant record repeatedly as the turn
		// streams, and the earlier writes are PARTIAL: output_tokens sits
		// at a placeholder (5, 3, 2) until the final write carries the
		// real total. So a repeat is not a duplicate to drop — it is a
		// correction to apply, and only the last write of an id is true.
		//
		// Confirmed live on one teammate transcript: 32 distinct messages
		// written 100+ times between them. Keeping the first occurrence
		// of each gave 16,239 output tokens; keeping the last gave
		// 150,021 — a 9x undercount, and the tool_use blocks were
		// truncated the same way. The sample stays at the position of its
		// FIRST write, which is the turn it actually belongs to.
		key := msg.ID
		if key == "" {
			key = doc.RequestID
		}
		if key != "" {
			if at, ok := idx[key]; ok {
				if at < len(base) {
					base[at] = s
				} else if j := at - len(base); j < len(out) {
					out[j] = s
				}
				continue
			}
			idx[key] = len(base) + len(out)
		}
		out = append(out, s)
	}
	return out, cost, haveCost, next, nil
}

func parseTranscriptTime(s string) (time.Time, bool) {
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return time.Time{}, false
	}
	return t.UTC(), true
}

// transcriptCache keeps parsed samples per file and re-reads only the bytes
// appended since last time. A live plan's coordinator transcript is tens to
// hundreds of megabytes and the dashboard re-renders on every page load, so
// a full reparse per request is the difference between a panel and a
// timeout. Keyed by path; invalidated when a file shrinks (truncated or
// replaced), which is the only way an append-only log can go backwards.
type transcriptCacheEntry struct {
	offset  int64
	samples []transcriptSample
	idx     map[string]int // message id -> position in samples, for keep-last
	cost    float64
	haveCst bool
}

var transcriptCache = struct {
	sync.Mutex
	m map[string]*transcriptCacheEntry
}{m: map[string]*transcriptCacheEntry{}}

// readTranscript returns every sample in a transcript, parsing only what
// has been appended since the last call.
func readTranscript(path string) ([]transcriptSample, float64, bool) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, 0, false
	}

	transcriptCache.Lock()
	defer transcriptCache.Unlock()

	e := transcriptCache.m[path]
	if e == nil || st.Size() < e.offset {
		e = &transcriptCacheEntry{idx: map[string]int{}}
		transcriptCache.m[path] = e
	}
	if st.Size() > e.offset {
		add, cost, haveCost, next, _ := scanTranscriptFrom(path, e.offset, e.idx, e.samples)
		e.samples = append(e.samples, add...)
		e.offset = next
		if haveCost {
			e.cost, e.haveCst = cost, true
		}
	}
	return e.samples, e.cost, e.haveCst
}

// ResetTranscriptCache drops every cached transcript. Tests write a file,
// read it, then write it again under the same path, which the append-only
// cache would otherwise treat as a continuation.
func ResetTranscriptCache() {
	transcriptCache.Lock()
	defer transcriptCache.Unlock()
	transcriptCache.m = map[string]*transcriptCacheEntry{}
}

// projectsRoot is where Claude Code keeps its transcripts.
func projectsRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".claude", "projects")
}

// coordinatorTranscript finds a session's own transcript. The directory is
// a slug of the cwd the session started in, which is not reliably
// derivable from anything the event log holds (a session that moved, a
// slug rule that changed), so the session id is matched by glob instead.
func coordinatorTranscript(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	hits, _ := filepath.Glob(filepath.Join(projectsRoot(), "*", sessionID+".jsonl"))
	if len(hits) == 0 {
		return ""
	}
	sort.Strings(hits)
	return hits[0]
}

// subagentTranscript finds one teammate's transcript inside its parent
// session's directory.
func subagentTranscript(sessionID, agentID string) string {
	if sessionID == "" || agentID == "" {
		return ""
	}
	hits, _ := filepath.Glob(filepath.Join(projectsRoot(), "*", sessionID, "subagents", "agent-"+agentID+".jsonl"))
	if len(hits) == 0 {
		return ""
	}
	sort.Strings(hits)
	return hits[0]
}

// laneSource is one lane and the transcripts that hold its spend.
type laneSource struct {
	key   string
	label string
	paths []string
}

// planLaneSources resolves a plan's lanes from the hook event log: the
// coordinator, plus one per team that ever made a tool call. Teams are
// keyed exactly as BuildRowsFromHookEvents keys them, so the token table
// and the Gantt agree on what a lane is called.
func planLaneSources(events []Event, labels map[string]string, plan, proj string) ([]laneSource, []string) {
	coordSessions := map[string]bool{}
	agentSessions := map[string]map[string]bool{} // agentID -> sessions
	for _, e := range events {
		if e.Plan != plan || e.SessionID == "" {
			continue
		}
		if e.AgentID == "" {
			// Plan codenames are drawn from a shared pool and DO collide
			// across projects; the coordinator always runs from the
			// plan's own project, so cwd is what keeps a same-named plan
			// elsewhere out of this one's numbers.
			if proj == "" || e.CWD == proj {
				coordSessions[e.SessionID] = true
			}
			continue
		}
		if agentSessions[e.AgentID] == nil {
			agentSessions[e.AgentID] = map[string]bool{}
		}
		agentSessions[e.AgentID][e.SessionID] = true
	}

	var sessions []string
	for s := range coordSessions {
		sessions = append(sessions, s)
	}
	sort.Strings(sessions)

	coord := laneSource{key: "", label: "coordinator"}
	for _, s := range sessions {
		if p := coordinatorTranscript(s); p != "" {
			coord.paths = append(coord.paths, p)
		}
	}

	// Fold every agent id onto its team name, so a re-dispatched team
	// (badger-app-2) lands in the same lane as its first attempt.
	byTeam := map[string][]string{}
	for agentID, sess := range agentSessions {
		team := labels[agentID]
		if team == "" {
			if _, t := parseAgentID(agentID); t != "" {
				team = t
			} else {
				team = agentID
			}
		}
		team = strings.TrimPrefix(team, plan+"-")
		team = redispatchSuffix.ReplaceAllString(team, "")
		for s := range sess {
			if p := subagentTranscript(s, agentID); p != "" {
				byTeam[team] = append(byTeam[team], p)
			} else {
				byTeam[team] = append(byTeam[team], "")
			}
		}
	}

	lanes := []laneSource{coord}
	var teams []string
	for t := range byTeam {
		teams = append(teams, t)
	}
	sort.Strings(teams)
	for _, t := range teams {
		var paths []string
		for _, p := range byTeam[t] {
			if p != "" {
				paths = append(paths, p)
			}
		}
		sort.Strings(paths)
		lanes = append(lanes, laneSource{key: t, label: t, paths: uniqueStrings(paths)})
	}
	return lanes, sessions
}

func uniqueStrings(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	out := in[:0:0]
	var last string
	for i, s := range in {
		if i > 0 && s == last {
			continue
		}
		out = append(out, s)
		last = s
	}
	return out
}

// PlanTokenUsage builds the token panel for one plan: who spent it, what it
// went on, and what each wake-up cost. Windowed to the plan's own execution
// span, because a coordinator session routinely outlives many plans and an
// unwindowed total would report months of unrelated work as this plan's.
func PlanTokenUsage(plan PlanSummary, events []Event, labels map[string]string) PlanTokens {
	pt := PlanTokens{Plan: plan.Name}

	from, hasFrom := plan.EffectiveStart()
	to, hasTo := plan.FinishedAt()
	if !hasTo {
		to = time.Now().UTC()
	}
	inWindow := func(t time.Time) bool {
		if hasFrom && t.Before(from) {
			return false
		}
		return !t.After(to)
	}

	lanes, sessions := planLaneSources(events, labels, plan.Name, plan.ProjectDir)
	pt.Sessions = sessions

	toolGrowth := map[string]int64{}
	toolEmit := map[string]int64{}
	toolCalls := map[string]int{}

	for _, ls := range lanes {
		lane := LaneTokens{Key: ls.key, Label: ls.label}
		if len(ls.paths) == 0 {
			lane.Missing = true
			pt.Lanes = append(pt.Lanes, lane)
			continue
		}

		models := map[string]bool{}
		var tick *TickTokens

		for _, path := range ls.paths {
			samples, cost, haveCost := readTranscript(path)
			if haveCost {
				pt.CostUSD += cost
				pt.HaveCost = true
			}

			// Growth attribution walks the lane in order and needs the
			// PREVIOUS request even when that request fell outside the
			// window — otherwise the first in-window call is credited
			// with the whole context built up before it.
			var prevCtx int64 = -1
			var prevTools []string

			for _, s := range samples {
				if s.Tick != "" {
					if inWindow(s.At) {
						if tick != nil {
							pt.Ticks = append(pt.Ticks, *tick)
						}
						tick = &TickTokens{At: s.At, Label: s.Tick, Lane: ls.label}
					}
					continue
				}
				if prevCtx >= 0 && len(prevTools) > 0 && inWindow(s.At) {
					if growth := s.Ctx - prevCtx; growth > 0 {
						share := growth / int64(len(prevTools))
						for _, t := range prevTools {
							toolGrowth[t] += share
						}
					}
				}
				prevCtx, prevTools = s.Ctx, s.Tools

				if !inWindow(s.At) {
					continue
				}
				lane.Usage.add(s.Usage)
				if s.Ctx > lane.PeakContext {
					lane.PeakContext = s.Ctx
				}
				if s.Model != "" {
					models[s.Model] = true
				}
				if tick != nil {
					tick.Usage.add(s.Usage)
					tick.Tools += len(s.Tools)
				}
				if n := len(s.Tools); n > 0 {
					per := s.Usage.Output / int64(n)
					for _, t := range s.Tools {
						toolEmit[t] += per
						toolCalls[t]++
					}
				}
			}
		}
		if tick != nil {
			pt.Ticks = append(pt.Ticks, *tick)
		}

		for m := range models {
			lane.Models = append(lane.Models, m)
		}
		sort.Strings(lane.Models)
		pt.Total.add(lane.Usage)
		pt.Lanes = append(pt.Lanes, lane)
	}

	// A session's cost covers the whole session, and a coordinator session
	// usually spans more than this one plan — say so rather than implying
	// the figure is the plan's.
	if pt.HaveCost && hasFrom {
		pt.CostPartial = true
	}

	total := pt.Total.Weighted()
	for i := range pt.Lanes {
		if total > 0 {
			pt.Lanes[i].Share = 100 * pt.Lanes[i].Usage.Weighted() / total
		}
	}
	sort.SliceStable(pt.Lanes, func(i, j int) bool {
		// Coordinator first — it is the lane the others are compared
		// against — then biggest spender down.
		if (pt.Lanes[i].Key == "") != (pt.Lanes[j].Key == "") {
			return pt.Lanes[i].Key == ""
		}
		return pt.Lanes[i].Usage.Weighted() > pt.Lanes[j].Usage.Weighted()
	})

	var attributed float64
	for t := range toolGrowth {
		attributed += weightCacheRead * float64(toolGrowth[t])
	}
	for t := range toolEmit {
		attributed += weightOutput * float64(toolEmit[t])
	}
	for t, calls := range toolCalls {
		tt := ToolTokens{Tool: t, Calls: calls, Emitting: toolEmit[t], ContextGrowth: toolGrowth[t]}
		if attributed > 0 {
			tt.Share = 100 * (weightCacheRead*float64(tt.ContextGrowth) + weightOutput*float64(tt.Emitting)) / attributed
		}
		pt.Tools = append(pt.Tools, tt)
	}
	sort.SliceStable(pt.Tools, func(i, j int) bool {
		if pt.Tools[i].Share != pt.Tools[j].Share {
			return pt.Tools[i].Share > pt.Tools[j].Share
		}
		return pt.Tools[i].Tool < pt.Tools[j].Tool
	})

	sort.SliceStable(pt.Ticks, func(i, j int) bool { return pt.Ticks[i].At.Before(pt.Ticks[j].At) })
	pt.Groups = groupTicks(pt.Ticks, total)
	return pt
}

// groupTicks folds the raw wake-ups into one row per kind.
func groupTicks(ticks []TickTokens, planWeighted float64) []TickGroup {
	byLabel := map[string]*TickGroup{}
	ctxs := map[string][]int64{}
	var order []string
	for _, t := range ticks {
		g := byLabel[t.Label]
		if g == nil {
			g = &TickGroup{Label: t.Label}
			byLabel[t.Label] = g
			order = append(order, t.Label)
		}
		g.Count++
		switch {
		case t.Usage.Messages == 0:
			g.Coalesced++
		case t.Tools == 0:
			g.Ran++
			g.Idle++
		default:
			g.Ran++
		}
		g.Usage.add(t.Usage)
		if t.Usage.Messages > 0 {
			ctxs[t.Label] = append(ctxs[t.Label], t.Usage.Context())
		}
	}
	out := make([]TickGroup, 0, len(order))
	for _, label := range order {
		g := byLabel[label]
		c := ctxs[label]
		sort.Slice(c, func(i, j int) bool { return c[i] < c[j] })
		if len(c) > 0 {
			g.Median = c[len(c)/2]
		}
		if planWeighted > 0 {
			g.Share = 100 * g.Usage.Weighted() / planWeighted
		}
		out = append(out, *g)
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Share > out[j].Share })
	return out
}

// PrintPlanTokens writes the same three tables the dashboard renders, in
// the terse aligned form the rest of this binary's CLI uses. Exists
// because triage happens at a terminal: "what did last night cost and
// where did it go" should not require a browser.
func PrintPlanTokens(w io.Writer, pt PlanTokens) {
	if pt.Total.Messages == 0 {
		fmt.Fprintf(w, "%s\tno token data\t(no session transcript found for this plan)\n", pt.Plan)
		return
	}
	fmt.Fprintf(w, "%s\tcontext %s\tgenerated %s\tthinking %s\trequests %d",
		pt.Plan, humanTokens(pt.Total.Context()), humanTokens(pt.Total.Output),
		humanTokens(pt.Total.Thinking), pt.Total.Messages)
	if pt.HaveCost {
		fmt.Fprintf(w, "\tsession cost $%.2f", pt.CostUSD)
	}
	fmt.Fprintln(w)

	fmt.Fprintf(w, "\n%-18s %7s %10s %10s %10s %10s %6s\n", "LANE", "SHARE", "CONTEXT", "GENERATED", "THINKING", "PEAK CTX", "REQS")
	for _, l := range pt.Lanes {
		if l.Missing {
			fmt.Fprintf(w, "%-18s %7s %s\n", l.Label, "-", "no transcript found")
			continue
		}
		fmt.Fprintf(w, "%-18s %6.1f%% %10s %10s %10s %10s %6d\n", l.Label, l.Share,
			humanTokens(l.Usage.Context()), humanTokens(l.Usage.Output),
			humanTokens(l.Usage.Thinking), humanTokens(l.PeakContext), l.Usage.Messages)
	}

	if len(pt.Tools) > 0 {
		fmt.Fprintf(w, "\n%-26s %7s %6s %12s %10s\n", "TOOL", "SHARE", "CALLS", "RESULT>CTX", "WRITING")
		for i, t := range pt.Tools {
			if i >= 12 {
				break
			}
			fmt.Fprintf(w, "%-26s %6.1f%% %6d %12s %10s\n", t.Tool, t.Share, t.Calls,
				humanTokens(t.ContextGrowth), humanTokens(t.Emitting))
		}
	}

	if len(pt.Groups) > 0 {
		fmt.Fprintf(w, "\n%-12s %7s %6s %5s %5s %10s %11s\n", "WAKE-UP", "SHARE", "FIRED", "RAN", "IDLE", "COALESCED", "MEDIAN CTX")
		for _, g := range pt.Groups {
			fmt.Fprintf(w, "%-12s %6.1f%% %6d %5d %5d %10d %11s\n", g.Label, g.Share,
				g.Count, g.Ran, g.Idle, g.Coalesced, humanTokens(g.Median))
		}
	}
}
