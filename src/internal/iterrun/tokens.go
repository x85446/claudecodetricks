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
//
// A Codex session reaches the same hook log under its own session id, and
// its usage lives in a rollout file instead:
// $CODEX_HOME/sessions/YYYY/MM/DD/rollout-<time>-<session-id>.jsonl. One
// token_usage_record per provider response carries that response's exact
// counters; older rollouts carry only token_count events, whose
// last_token_usage is the same figure. Both are normalized into the same
// sample shape, so a Codex lane and a Claude lane report equivalent fields.

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

	// Observed counts the identities the hook log saw for this lane — a
	// coordinator's sessions, a team's agent/session pairs — and Found the
	// ones whose transcript was read. Found < Observed is partial capture.
	Observed int `json:"observed"`
	Found    int `json:"found"`
	// Sessions are the sessions whose transcript was read for this lane;
	// Agents are the subagent ids folded into it.
	Sessions []string `json:"sessions,omitempty"`
	Agents   []string `json:"agents,omitempty"`
	// AgentsFound are the agents at least one of whose transcripts was read.
	AgentsFound []string `json:"agents_found,omitempty"`
	// Engines and Providers name what wrote the transcripts: claude-code /
	// anthropic for a Claude Code transcript, codex / the rollout's
	// model_provider for a Codex session.
	Engines   []string `json:"engines,omitempty"`
	Providers []string `json:"providers,omitempty"`
	// ReasoningKnown counts requests that reported a reasoning (thinking)
	// figure. Usage.Thinking is a complete total only when it equals
	// Usage.Messages; otherwise the figure is partial.
	ReasoningKnown int `json:"reasoning_known"`
}

// AllowanceObservation is one subscription-allowance reading a provider
// reported alongside its usage. It describes the account's whole window,
// shared with every other session using that account, so it is never
// converted into this plan's usage.
type AllowanceObservation struct {
	At            time.Time  `json:"at"`
	Provider      string     `json:"provider"`
	Limit         string     `json:"limit"`
	Window        string     `json:"window"` // primary | secondary
	WindowMinutes int        `json:"window_minutes"`
	UsedPercent   float64    `json:"used_percent"`
	ResetsAt      *time.Time `json:"resets_at,omitempty"`
	PlanType      string     `json:"plan_type,omitempty"`
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
	// CostSessions counts the sessions whose cost-state record was read.
	CostSessions int `json:"cost_sessions"`

	// From and To are the attribution window: the plan's own execution
	// span. ToOpen is set when the plan has not finished and To is the
	// time of the read.
	From   *time.Time `json:"from,omitempty"`
	To     time.Time  `json:"to"`
	ToOpen bool       `json:"to_open"`

	// Duplicates counts requests seen in more than one transcript — a
	// subagent's request echoed into its parent's file, or history copied
	// into a resumed session — that were counted once.
	Duplicates int `json:"duplicates"`
	// Allowance holds the first and last in-window reading of each
	// subscription window a provider reported.
	Allowance []AllowanceObservation `json:"allowance,omitempty"`
	// Models splits the counted requests by model.
	Models []ModelTokens `json:"models,omitempty"`
}

// ModelTokens is the plan's counted usage for one model.
type ModelTokens struct {
	Model          string     `json:"model"`
	Provider       string     `json:"provider"`
	Engine         string     `json:"engine"`
	Usage          TokenUsage `json:"usage"`
	ReasoningKnown int        `json:"reasoning_known"`
}

// transcriptSample is one retained record from a transcript: an assistant
// request's usage, or a prompt that starts a tick. Compact on purpose —
// a 178 MB transcript reduces to a few thousand of these, which is what
// makes windowing a plan out of a months-long session cheap.
type transcriptSample struct {
	At      time.Time
	ID      string // provider message or response id; "" when the record has none
	Model   string
	Usage   TokenUsage
	Ctx     int64    // context this request paid for; diffed to get growth
	Tools   []string // tool_use blocks this message emitted
	Tick    string   // non-empty: this sample is a prompt, not a request
	Skipped bool     // a duplicate transcript write, already counted
	// ReasoningKnown is set when the record reported a reasoning figure,
	// so a missing figure is never read as zero reasoning.
	ReasoningKnown bool
}

// transcriptDoc is the subset of a transcript line this file reads.
type transcriptDoc struct {
	Type      string          `json:"type"`
	Timestamp string          `json:"timestamp"`
	RequestID string          `json:"requestId"`
	Message   json.RawMessage `json:"message"`
	// cost-state fields
	TotalCostUSD float64 `json:"totalCostUSD"`
	// Payload is a Codex rollout line's body.
	Payload json.RawMessage `json:"payload"`
}

// codexPayload is the subset of a Codex rollout payload this file reads.
type codexPayload struct {
	Type          string      `json:"type"`
	Model         string      `json:"model"`
	ModelProvider string      `json:"model_provider"`
	Name          string      `json:"name"`
	Message       string      `json:"message"`
	ResponseID    string      `json:"response_id"`
	Usage         *codexUsage `json:"usage"`
	Info          *struct {
		Total *codexUsage `json:"total_token_usage"`
		Last  *codexUsage `json:"last_token_usage"`
	} `json:"info"`
	RateLimits *struct {
		LimitID   string       `json:"limit_id"`
		PlanType  string       `json:"plan_type"`
		Primary   *codexWindow `json:"primary"`
		Secondary *codexWindow `json:"secondary"`
	} `json:"rate_limits"`
}

// codexUsage is one Codex usage object. Input includes cached input, and
// output includes reasoning: total_tokens is input + output.
type codexUsage struct {
	InputTokens           int64  `json:"input_tokens"`
	CachedInputTokens     int64  `json:"cached_input_tokens"`
	CacheWriteInputTokens int64  `json:"cache_write_input_tokens"`
	OutputTokens          int64  `json:"output_tokens"`
	ReasoningOutputTokens *int64 `json:"reasoning_output_tokens"`
	TotalTokens           int64  `json:"total_tokens"`
}

type codexWindow struct {
	UsedPercent   float64 `json:"used_percent"`
	WindowMinutes int     `json:"window_minutes"`
	ResetsAt      int64   `json:"resets_at"`
}

// usage normalizes a Codex usage object to the Claude-shaped counters:
// Input is fresh (uncached) input, and the cache figures are carved out of
// Codex's input rather than added to it.
func (u codexUsage) usage() (TokenUsage, bool) {
	fresh := u.InputTokens - u.CachedInputTokens - u.CacheWriteInputTokens
	if fresh < 0 {
		fresh = 0
	}
	t := TokenUsage{Input: fresh, CacheRead: u.CachedInputTokens, CacheWrite: u.CacheWriteInputTokens, Output: u.OutputTokens, Messages: 1}
	if u.ReasoningOutputTokens != nil {
		t.Thinking = *u.ReasoningOutputTokens
		return t, true
	}
	return t, false
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
		OutputDetails       *struct {
			ThinkingTokens *int64 `json:"thinking_tokens"`
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

// transcriptState is one transcript file's parsed state: every retained
// sample, the keep-last index, the last cost-state total, the allowance
// readings, and the Codex context a later line needs (the current model,
// tool calls waiting for the usage record of the response that made them).
type transcriptState struct {
	offset    int64
	samples   []transcriptSample
	idx       map[string]int // message id -> position in samples, for keep-last
	cost      float64
	haveCost  bool
	allowance []AllowanceObservation

	engine, provider string
	codexModel       string
	codexTools       []string
	codexRecords     bool  // the rollout carries token_usage_record lines
	codexLastTotal   int64 // token_count total already counted
}

// scanTranscriptFrom parses new lines of a transcript starting at the
// state's offset, appending samples and advancing the offset just past the
// last COMPLETE line. Stopping at the last newline is what makes this safe
// to run against a file Claude Code or Codex is still appending to: a torn
// final line is left for the next pass.
func scanTranscriptFrom(path string, st *transcriptState) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()

	if st.offset > 0 {
		if _, err := f.Seek(st.offset, 0); err != nil {
			return err
		}
	}

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
		st.offset += int64(len(line))

		interesting := bytes.Contains(raw, []byte(`"usage"`)) ||
			bytes.Contains(raw, []byte(`"cost-state"`)) ||
			(bytes.Contains(raw, []byte(`"type":"user"`)) && !bytes.Contains(raw, []byte(`"tool_result"`))) ||
			bytes.Contains(raw, []byte(`"payload"`))
		if !interesting {
			continue
		}

		var doc transcriptDoc
		if json.Unmarshal(raw, &doc) != nil {
			continue
		}

		switch doc.Type {
		case "cost-state":
			st.cost, st.haveCost = doc.TotalCostUSD, true
			continue
		case "user":
			at, ok := parseTranscriptTime(doc.Timestamp)
			if !ok {
				continue
			}
			if label := promptTick(contentText(doc.Message)); label != "" {
				st.samples = append(st.samples, transcriptSample{At: at, Tick: label})
			}
			continue
		case "assistant":
		case "session_meta", "turn_context", "response_item", "event_msg", "token_usage_record":
			scanCodexLine(st, doc)
			continue
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
		if st.engine == "" {
			st.engine, st.provider = "claude-code", "anthropic"
		}

		s := transcriptSample{
			At:    at,
			Model: msg.Model,
			Usage: TokenUsage{
				Input:      msg.Usage.InputTokens,
				CacheRead:  msg.Usage.CacheReadTokens,
				CacheWrite: msg.Usage.CacheCreationTokens,
				Output:     msg.Usage.OutputTokens,
				Messages:   1,
			},
		}
		if d := msg.Usage.OutputDetails; d != nil && d.ThinkingTokens != nil {
			s.Usage.Thinking, s.ReasoningKnown = *d.ThinkingTokens, true
		}
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
		s.ID = key
		st.add(s)
	}
	return nil
}

// add appends a request sample, or replaces the earlier write of the same
// id in place.
func (st *transcriptState) add(s transcriptSample) {
	s.Ctx = s.Usage.Context()
	if s.ID != "" {
		if at, ok := st.idx[s.ID]; ok {
			st.samples[at] = s
			return
		}
		st.idx[s.ID] = len(st.samples)
	}
	st.samples = append(st.samples, s)
}

// scanCodexLine folds one Codex rollout line into the state.
func scanCodexLine(st *transcriptState, doc transcriptDoc) {
	var p codexPayload
	if len(doc.Payload) == 0 || json.Unmarshal(doc.Payload, &p) != nil {
		return
	}
	st.engine = "codex"
	if st.provider == "" {
		st.provider = "openai"
	}
	at, ok := parseTranscriptTime(doc.Timestamp)
	switch doc.Type {
	case "session_meta":
		if p.ModelProvider != "" {
			st.provider = p.ModelProvider
		}
	case "turn_context":
		if p.Model != "" {
			st.codexModel = p.Model
		}
	case "response_item":
		switch p.Type {
		case "function_call", "custom_tool_call":
			if p.Name != "" {
				st.codexTools = append(st.codexTools, p.Name)
			}
		case "local_shell_call":
			st.codexTools = append(st.codexTools, "shell")
		}
	case "token_usage_record":
		if !ok || p.Usage == nil {
			return
		}
		st.codexRecords = true
		st.codexUsage(at, p.ResponseID, *p.Usage)
	case "event_msg":
		switch p.Type {
		case "user_message":
			if ok {
				if label := promptTick(p.Message); label != "" {
					st.samples = append(st.samples, transcriptSample{At: at, Tick: label})
				}
			}
		case "token_count":
			if !ok {
				return
			}
			if rl := p.RateLimits; rl != nil {
				for _, w := range []struct {
					name string
					win  *codexWindow
				}{{"primary", rl.Primary}, {"secondary", rl.Secondary}} {
					if w.win != nil {
						st.observeAllowance(AllowanceObservation{At: at, Provider: st.provider, Limit: rl.LimitID, Window: w.name,
							WindowMinutes: w.win.WindowMinutes, UsedPercent: w.win.UsedPercent, ResetsAt: unixTime(w.win.ResetsAt), PlanType: rl.PlanType})
					}
				}
			}
			// Rollouts with per-response records count those; token_count
			// repeats them and is counted only in older rollouts, once per
			// growth of the session total.
			if st.codexRecords || p.Info == nil || p.Info.Last == nil || p.Info.Total == nil {
				return
			}
			if p.Info.Total.TotalTokens <= st.codexLastTotal {
				return
			}
			st.codexLastTotal = p.Info.Total.TotalTokens
			st.codexUsage(at, fmt.Sprintf("token_count:%d", p.Info.Total.TotalTokens), *p.Info.Last)
		}
	}
}

// codexUsage records one Codex response's usage and charges the tool calls
// that response made.
func (st *transcriptState) codexUsage(at time.Time, id string, u codexUsage) {
	usage, known := u.usage()
	if usage.Input == 0 && usage.CacheRead == 0 && usage.CacheWrite == 0 && usage.Output == 0 {
		return
	}
	s := transcriptSample{At: at, ID: id, Model: st.codexModel, Usage: usage, ReasoningKnown: known, Tools: st.codexTools}
	st.codexTools = nil
	st.add(s)
}

// observeAllowance keeps an allowance reading when it differs from the
// previous reading of the same window, so a long rollout keeps its changes
// and not one copy per response.
func (st *transcriptState) observeAllowance(a AllowanceObservation) {
	for i := len(st.allowance) - 1; i >= 0; i-- {
		prev := st.allowance[i]
		if prev.Limit == a.Limit && prev.Window == a.Window {
			if prev.UsedPercent == a.UsedPercent && prev.WindowMinutes == a.WindowMinutes && sameTime(prev.ResetsAt, a.ResetsAt) {
				return
			}
			break
		}
	}
	st.allowance = append(st.allowance, a)
}

func timeKey(t *time.Time) string {
	if t == nil {
		return ""
	}
	return t.UTC().Format(time.RFC3339)
}

func unixTime(sec int64) *time.Time {
	if sec <= 0 {
		return nil
	}
	t := time.Unix(sec, 0).UTC()
	return &t
}

func sameTime(a, b *time.Time) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Equal(*b)
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
var transcriptCache = struct {
	sync.Mutex
	m map[string]*transcriptState
}{m: map[string]*transcriptState{}}

// transcriptRead is a snapshot of one transcript's parsed state.
type transcriptRead struct {
	samples          []transcriptSample
	cost             float64
	haveCost         bool
	allowance        []AllowanceObservation
	engine, provider string
}

// readTranscript returns every sample in a transcript, parsing only what
// has been appended since the last call.
func readTranscript(path string) transcriptRead {
	info, err := os.Stat(path)
	if err != nil {
		return transcriptRead{}
	}

	transcriptCache.Lock()
	defer transcriptCache.Unlock()

	st := transcriptCache.m[path]
	if st == nil || info.Size() < st.offset {
		st = &transcriptState{idx: map[string]int{}}
		transcriptCache.m[path] = st
	}
	if info.Size() > st.offset {
		_ = scanTranscriptFrom(path, st)
	}
	return transcriptRead{samples: st.samples, cost: st.cost, haveCost: st.haveCost,
		allowance: st.allowance, engine: st.engine, provider: st.provider}
}

// ResetTranscriptCache drops every cached transcript. Tests write a file,
// read it, then write it again under the same path, which the append-only
// cache would otherwise treat as a continuation.
func ResetTranscriptCache() {
	transcriptCache.Lock()
	defer transcriptCache.Unlock()
	transcriptCache.m = map[string]*transcriptState{}
}

// projectsRoot is where Claude Code keeps its transcripts.
func projectsRoot() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".claude", "projects")
}

// codexHome is where Codex keeps its sessions: $CODEX_HOME, else ~/.codex.
func codexHome() string {
	if dir := os.Getenv("CODEX_HOME"); dir != "" {
		return dir
	}
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".codex")
}

// globSafe reports whether an id from the hook log can be placed in a glob
// pattern as a literal.
func globSafe(id string) bool {
	return id != "" && !strings.ContainsAny(id, `/\*?[]`) && id != "." && id != ".."
}

// coordinatorTranscript finds a session's own transcript. The directory is
// a slug of the cwd the session started in, which is not reliably
// derivable from anything the event log holds (a session that moved, a
// slug rule that changed), so the session id is matched by glob instead.
// A session with no Claude Code transcript is looked up as a Codex
// session.
func coordinatorTranscript(sessionID string) string {
	if !globSafe(sessionID) {
		return ""
	}
	hits, _ := filepath.Glob(filepath.Join(projectsRoot(), "*", sessionID+".jsonl"))
	if len(hits) == 0 {
		return codexTranscript(sessionID)
	}
	sort.Strings(hits)
	return hits[0]
}

// codexTranscript finds a Codex session's rollout file, live or archived.
func codexTranscript(sessionID string) string {
	if !globSafe(sessionID) {
		return ""
	}
	root := codexHome()
	hits, _ := filepath.Glob(filepath.Join(root, "sessions", "*", "*", "*", "rollout-*-"+sessionID+".jsonl"))
	if len(hits) == 0 {
		hits, _ = filepath.Glob(filepath.Join(root, "archived_sessions", "rollout-*-"+sessionID+".jsonl"))
	}
	if len(hits) == 0 {
		return ""
	}
	sort.Strings(hits)
	return hits[0]
}

// subagentTranscript finds one teammate's transcript inside its parent
// session's directory, or a Codex subagent's own rollout.
func subagentTranscript(sessionID, agentID string) string {
	if !globSafe(sessionID) || !globSafe(agentID) {
		return ""
	}
	hits, _ := filepath.Glob(filepath.Join(projectsRoot(), "*", sessionID, "subagents", "agent-"+agentID+".jsonl"))
	if len(hits) == 0 {
		return codexTranscript(agentID)
	}
	sort.Strings(hits)
	return hits[0]
}

// laneSource is one lane and the transcripts that hold its spend.
type laneSource struct {
	key      string
	label    string
	paths    []string
	observed int      // identities the hook log saw
	found    int      // identities whose transcript exists
	sessions []string // sessions whose transcript was found
	agents   []string
	// agentsFound are agents with at least one transcript.
	agentsFound []string
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

	coord := laneSource{key: "", label: "coordinator", observed: len(sessions)}
	for _, s := range sessions {
		if p := coordinatorTranscript(s); p != "" {
			coord.paths = append(coord.paths, p)
			coord.sessions = append(coord.sessions, s)
			coord.found++
		}
	}

	// Fold every agent id onto its team name, so a re-dispatched team
	// (badger-app-2) lands in the same lane as its first attempt.
	byTeam := map[string]*laneSource{}
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
		ls := byTeam[team]
		if ls == nil {
			ls = &laneSource{key: team, label: team}
			byTeam[team] = ls
		}
		ls.agents = append(ls.agents, agentID)
		for s := range sess {
			ls.observed++
			if p := subagentTranscript(s, agentID); p != "" {
				ls.paths = append(ls.paths, p)
				ls.sessions = append(ls.sessions, s)
				ls.found++
				if !containsString(ls.agentsFound, agentID) {
					ls.agentsFound = append(ls.agentsFound, agentID)
				}
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
		ls := byTeam[t]
		sort.Strings(ls.paths)
		ls.paths = uniqueStrings(ls.paths)
		sort.Strings(ls.sessions)
		ls.sessions = uniqueStrings(ls.sessions)
		sort.Strings(ls.agents)
		sort.Strings(ls.agentsFound)
		lanes = append(lanes, *ls)
	}
	return lanes, sessions
}

// planWindowEnd is when a plan execution ended: its Finished: value, or
// for an archive that never recorded one, the archive filename's time. A
// session that kept working after the archive did so for something else.
func planWindowEnd(plan PlanSummary) (time.Time, bool) {
	if t, ok := plan.FinishedAt(); ok {
		return t, true
	}
	if !plan.Archived {
		return time.Time{}, false
	}
	stamp, _, ok := strings.Cut(plan.ArchiveFile, "Z-")
	if !ok {
		return time.Time{}, false
	}
	for _, layout := range []string{"20060102T150405", "2006-01-02T150405", "2006-01-02T15-04-05"} {
		if t, err := time.Parse(layout, stamp); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
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

func sortedKeys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// PlanTokenUsage builds the token panel for one plan: who spent it, what it
// went on, and what each wake-up cost. Windowed to the plan's own execution
// span, because a coordinator session routinely outlives many plans and an
// unwindowed total would report months of unrelated work as this plan's.
func PlanTokenUsage(plan PlanSummary, events []Event, labels map[string]string) PlanTokens {
	pt := PlanTokens{Plan: plan.Name}

	from, hasFrom := plan.EffectiveStart()
	to, hasTo := planWindowEnd(plan)
	if !hasTo {
		to = time.Now().UTC()
	}
	if hasFrom {
		pt.From = &from
	}
	pt.To, pt.ToOpen = to, !hasTo
	inWindow := func(t time.Time) bool {
		if hasFrom && t.Before(from) {
			return false
		}
		return !t.After(to)
	}

	lanes, sessions := planLaneSources(events, labels, plan.Name, plan.ProjectDir)
	pt.Sessions = sessions

	// A request can appear in more than one transcript: a subagent's
	// requests echoed into its parent's file, or a resumed session that
	// copies earlier history. Each provider request id is counted once,
	// and a request a team's own transcript holds belongs to that team,
	// so the team lanes claim their ids before the coordinator is read.
	claimed := map[string]string{} // request id -> lane key
	for _, ls := range lanes {
		if ls.key == "" {
			continue
		}
		for _, path := range ls.paths {
			for _, s := range readTranscript(path).samples {
				if s.ID != "" && s.Tick == "" {
					if _, ok := claimed[s.ID]; !ok {
						claimed[s.ID] = ls.key
					}
				}
			}
		}
	}
	counted := map[string]bool{}
	byModel := map[string]*ModelTokens{}
	allowance := map[string][]AllowanceObservation{}
	var allowanceOrder []string

	toolGrowth := map[string]int64{}
	toolEmit := map[string]int64{}
	toolCalls := map[string]int{}

	for _, ls := range lanes {
		lane := LaneTokens{Key: ls.key, Label: ls.label, Observed: ls.observed, Found: ls.found, Sessions: ls.sessions, Agents: ls.agents, AgentsFound: ls.agentsFound}
		if len(ls.paths) == 0 {
			lane.Missing = true
			pt.Lanes = append(pt.Lanes, lane)
			continue
		}

		models := map[string]bool{}
		engines := map[string]bool{}
		providers := map[string]bool{}
		var tick *TickTokens

		for _, path := range ls.paths {
			tr := readTranscript(path)
			if tr.haveCost {
				pt.CostUSD += tr.cost
				pt.HaveCost = true
				pt.CostSessions++
			}
			if tr.engine != "" {
				engines[tr.engine] = true
			}
			if tr.provider != "" {
				providers[tr.provider] = true
			}
			for _, a := range tr.allowance {
				if !inWindow(a.At) {
					continue
				}
				// A reading after a reset belongs to a new window.
				k := a.Provider + "\x00" + a.Limit + "\x00" + a.Window + "\x00" + timeKey(a.ResetsAt)
				if _, ok := allowance[k]; !ok {
					allowanceOrder = append(allowanceOrder, k)
				}
				allowance[k] = append(allowance[k], a)
			}

			// Growth attribution walks the lane in order and needs the
			// PREVIOUS request even when that request fell outside the
			// window — otherwise the first in-window call is credited
			// with the whole context built up before it.
			var prevCtx int64 = -1
			var prevTools []string

			for _, s := range tr.samples {
				if s.Tick != "" {
					if inWindow(s.At) {
						if tick != nil {
							pt.Ticks = append(pt.Ticks, *tick)
						}
						tick = &TickTokens{At: s.At, Label: s.Tick, Lane: ls.label}
					}
					continue
				}
				if s.ID != "" {
					if owner, ok := claimed[s.ID]; ok && owner != ls.key {
						if inWindow(s.At) {
							pt.Duplicates++
						}
						continue
					}
					if counted[s.ID] {
						if inWindow(s.At) {
							pt.Duplicates++
						}
						continue
					}
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
				if s.ID != "" {
					counted[s.ID] = true
				}
				lane.Usage.add(s.Usage)
				if s.ReasoningKnown {
					lane.ReasoningKnown++
				}
				if s.Ctx > lane.PeakContext {
					lane.PeakContext = s.Ctx
				}
				if s.Model != "" {
					models[s.Model] = true
				}
				mk := tr.provider + "\x00" + s.Model
				mt := byModel[mk]
				if mt == nil {
					mt = &ModelTokens{Model: s.Model, Provider: tr.provider, Engine: tr.engine}
					byModel[mk] = mt
				}
				mt.Usage.add(s.Usage)
				if s.ReasoningKnown {
					mt.ReasoningKnown++
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

		lane.Models = sortedKeys(models)
		lane.Engines = sortedKeys(engines)
		lane.Providers = sortedKeys(providers)
		pt.Total.add(lane.Usage)
		pt.Lanes = append(pt.Lanes, lane)
	}
	for _, mt := range byModel {
		pt.Models = append(pt.Models, *mt)
	}
	sort.SliceStable(pt.Models, func(i, j int) bool {
		if pt.Models[i].Usage.Weighted() != pt.Models[j].Usage.Weighted() {
			return pt.Models[i].Usage.Weighted() > pt.Models[j].Usage.Weighted()
		}
		return pt.Models[i].Model < pt.Models[j].Model
	})
	for _, k := range allowanceOrder {
		obs := allowance[k]
		sort.SliceStable(obs, func(i, j int) bool { return obs[i].At.Before(obs[j].At) })
		pt.Allowance = append(pt.Allowance, obs[0])
		if len(obs) > 1 {
			pt.Allowance = append(pt.Allowance, obs[len(obs)-1])
		}
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

	fmt.Fprintf(w, "\n%-26s %7s %10s %10s %10s %10s %6s\n", "LANE", "SHARE", "CONTEXT", "GENERATED", "THINKING", "PEAK CTX", "REQS")
	for _, l := range pt.Lanes {
		if l.Missing {
			fmt.Fprintf(w, "%-26s %7s %s\n", laneLabel(l.Label), "-", "no transcript found")
			continue
		}
		fmt.Fprintf(w, "%-26s %6.1f%% %10s %10s %10s %10s %6d\n", laneLabel(l.Label), l.Share,
			humanTokens(l.Usage.Context()), humanTokens(l.Usage.Output),
			humanTokens(l.Usage.Thinking), humanTokens(l.PeakContext), l.Usage.Messages)
	}

	if len(pt.Tools) > 0 {
		fmt.Fprintf(w, "\n%-26s %7s %6s %12s %10s\n", "TOOL", "SHARE", "CALLS", "RESULT>CTX", "WRITING")
		for i, t := range pt.Tools {
			if i >= 12 {
				break
			}
			fmt.Fprintf(w, "%-26s %6.1f%% %6d %12s %10s\n", laneLabel(t.Tool), t.Share, t.Calls,
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
