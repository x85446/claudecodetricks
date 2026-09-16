package iterrun

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// tokenTestHome points HOME at a temp dir and gives back a helper that
// writes a transcript for one session (and optionally one subagent of it)
// in the exact layout Claude Code uses, so these tests exercise the real
// path resolution rather than a stubbed one.
func tokenTestHome(t *testing.T) func(slug, session, agent string, lines ...string) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	ResetTranscriptCache()
	return func(slug, session, agent string, lines ...string) string {
		t.Helper()
		var path string
		if agent == "" {
			path = filepath.Join(projectsRoot(), slug, session+".jsonl")
		} else {
			path = filepath.Join(projectsRoot(), slug, session, "subagents", "agent-"+agent+".jsonl")
		}
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(strings.Join(lines, "\n")+"\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		return path
	}
}

// assistantLine builds one assistant transcript record. id is what the
// dedupe keys on, so passing the same id twice simulates a streaming
// rewrite.
func assistantLine(ts, id, model string, in, cacheRead, cacheWrite, out, thinking int64, tools ...string) string {
	var blocks []string
	for i, name := range tools {
		blocks = append(blocks, fmt.Sprintf(`{"type":"tool_use","id":"toolu_%d","name":%q,"input":{}}`, i, name))
	}
	return fmt.Sprintf(`{"type":"assistant","timestamp":%q,"requestId":"req_x","message":{"id":%q,"model":%q,"role":"assistant","content":[%s],"usage":{"input_tokens":%d,"cache_read_input_tokens":%d,"cache_creation_input_tokens":%d,"output_tokens":%d,"output_tokens_details":{"thinking_tokens":%d}}}}`,
		ts, id, model, strings.Join(blocks, ","), in, cacheRead, cacheWrite, out, thinking)
}

func promptLine(ts, text string) string {
	return fmt.Sprintf(`{"type":"user","timestamp":%q,"message":{"role":"user","content":[{"type":"text","text":%q}]}}`, ts, text)
}

func planFor(name, proj, executing, finished string) PlanSummary {
	return PlanSummary{Name: name, ProjectDir: proj, Executing: executing, Finished: finished}
}

func coordEvent(plan, proj, session string) Event {
	return Event{Plan: plan, CWD: proj, SessionID: session, Hook: "pre", ToolName: "Bash"}
}

func teamEvent(plan, session, agentID string) Event {
	return Event{Plan: plan, SessionID: session, AgentID: agentID, Hook: "pre", ToolName: "Bash"}
}

// TESTMASTER: id=tokens-dedupe-keeps-final-write tier=fast parallel=yes
//
// The single most damaging thing this collector could get wrong. Claude
// Code rewrites an assistant record repeatedly while a turn streams, and
// the early writes carry a PLACEHOLDER output_tokens (5, 3, 2) — only the
// last write is true. Keeping the first occurrence of each id undercounted
// one real teammate transcript's output by 9x (16,239 vs 150,021) and
// truncated its tool_use blocks the same way.
func TestTranscriptDedupeKeepsTheFinalStreamingWrite(t *testing.T) {
	write := tokenTestHome(t)
	proj := t.TempDir()
	write("slug", "sess1", "",
		promptLine("2026-09-15T10:00:00Z", "/iterate"),
		// Same message id three times: two partial writes, then the truth.
		assistantLine("2026-09-15T10:00:01Z", "msg_a", "claude-opus-5", 10, 1000, 0, 5, 0),
		assistantLine("2026-09-15T10:00:02Z", "msg_a", "claude-opus-5", 10, 1000, 0, 5, 0),
		assistantLine("2026-09-15T10:00:03Z", "msg_a", "claude-opus-5", 10, 1000, 0, 900, 400, "Bash"),
	)

	pt := PlanTokenUsage(
		planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z"),
		[]Event{coordEvent("badger", proj, "sess1")}, nil)

	if pt.Total.Messages != 1 {
		t.Fatalf("three writes of one message should count once, got %d requests", pt.Total.Messages)
	}
	if pt.Total.Output != 900 {
		t.Errorf("output = %d, want 900 (the final write, not the 5-token placeholder)", pt.Total.Output)
	}
	if pt.Total.Thinking != 400 {
		t.Errorf("thinking = %d, want 400", pt.Total.Thinking)
	}
	if pt.Total.CacheRead != 1000 {
		t.Errorf("cache read = %d, want 1000 counted once", pt.Total.CacheRead)
	}
	// The final write is also the only one carrying the tool_use block.
	if len(pt.Tools) != 1 || pt.Tools[0].Tool != "Bash" {
		t.Errorf("tools = %+v, want the Bash call from the final write", pt.Tools)
	}
}

// TESTMASTER: id=tokens-skip-synthetic tier=fast parallel=yes
//
// Claude Code writes synthetic assistant records (interruptions, API-error
// placeholders) with a usage object of all zeros. They were never billed,
// and counting them made 368 coalesced cron firings look like wake-ups
// that had run and done nothing — dragging every per-tick median to zero.
func TestSyntheticZeroUsageRecordsAreNotCountedAsRequests(t *testing.T) {
	write := tokenTestHome(t)
	proj := t.TempDir()
	write("slug", "sess1", "",
		assistantLine("2026-09-15T10:00:01Z", "msg_syn", "<synthetic>", 0, 0, 0, 0, 0),
		assistantLine("2026-09-15T10:00:02Z", "msg_real", "claude-opus-5", 5, 500, 0, 100, 0),
	)

	pt := PlanTokenUsage(
		planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z"),
		[]Event{coordEvent("badger", proj, "sess1")}, nil)

	if pt.Total.Messages != 1 {
		t.Errorf("requests = %d, want 1 — the synthetic record is not a request", pt.Total.Messages)
	}
}

// TESTMASTER: id=tokens-prompt-tick-shapes tier=fast parallel=yes
//
// Every shape here came off a real transcript. The bare "/iterate" is the
// one that matters most: that is what a cron or /loop firing looks like,
// it has no <command-name> wrapper, and 507 of them were galago's single
// largest cost.
func TestPromptTickClassifiesEveryRealWakeupShape(t *testing.T) {
	cases := []struct {
		name, text, want string
	}{
		{"bare cron firing", "/iterate", "/iterate"},
		{"cron firing with args", "/iterate galago", "/iterate"},
		{"typed slash command", "<command-message>iterate</command-message>\n<command-name>/iterate</command-name>\n<command-args>galago</command-args>", "/iterate"},
		{"skill body after a command", "Base directory for this skill: /Users/x/.claude/skills/iterate\n\nlots of text", ""},
		{"harness reminder", "<system-reminder>\nother agents active\n</system-reminder>", ""},
		{"local command caveat", "<local-command-caveat>Caveat: ...</local-command-caveat>", ""},
		{"compaction banner", "This session is being continued from a previous conversation...", ""},
		{"coordinator nudge to a team", "<teammate-message teammate_id=\"team-lead\" summary=\"ping\">\ndo the thing", tickNudge},
		{"background task report", "<task-notification>\n<task-id>abc</task-id>", tickTask},
		{"a human typing", "why didn't it finish?", tickHuman},
		{"empty", "   \n ", ""},
		{"long text that merely starts with a slash", "/" + strings.Repeat("x", 200), tickHuman},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := promptTick(c.text); got != c.want {
				t.Errorf("promptTick(%.40q) = %q, want %q", c.text, got, c.want)
			}
		})
	}
}

// TESTMASTER: id=tokens-lane-split tier=fast parallel=yes
//
// The whole point of the panel: the coordinator and each team are separate
// lanes, resolved by joining the hook event log's agent_id to the
// subagents/agent-<id>.jsonl filename.
func TestPlanTokenUsageSplitsTheCoordinatorFromEachTeam(t *testing.T) {
	write := tokenTestHome(t)
	proj := t.TempDir()
	write("slug", "sess1", "",
		assistantLine("2026-09-15T10:00:00Z", "c1", "claude-opus-5", 0, 1000, 0, 100, 0),
	)
	write("slug", "sess1", "abadger-app-0123456789abcdef",
		assistantLine("2026-09-15T10:01:00Z", "a1", "claude-opus-5", 0, 400, 0, 40, 0),
	)
	write("slug", "sess1", "abadger-tests-fedcba9876543210",
		assistantLine("2026-09-15T10:02:00Z", "t1", "claude-opus-5", 0, 200, 0, 20, 0),
	)

	pt := PlanTokenUsage(
		planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z"),
		[]Event{
			coordEvent("badger", proj, "sess1"),
			teamEvent("badger", "sess1", "abadger-app-0123456789abcdef"),
			teamEvent("badger", "sess1", "abadger-tests-fedcba9876543210"),
		}, nil)

	if len(pt.Lanes) != 3 {
		t.Fatalf("lanes = %d, want 3 (coordinator + app + tests): %+v", len(pt.Lanes), pt.Lanes)
	}
	if pt.Lanes[0].Key != "" || pt.Lanes[0].Label != "coordinator" {
		t.Errorf("first lane = %+v, want the coordinator", pt.Lanes[0])
	}
	byKey := map[string]LaneTokens{}
	for _, l := range pt.Lanes {
		byKey[l.Key] = l
	}
	if got := byKey["app"].Usage.Output; got != 40 {
		t.Errorf("app output = %d, want 40 — its own transcript, not the coordinator's", got)
	}
	if got := byKey["tests"].Usage.Output; got != 20 {
		t.Errorf("tests output = %d, want 20", got)
	}
	if got := byKey[""].Usage.Output; got != 100 {
		t.Errorf("coordinator output = %d, want 100 — teams must not be folded in", got)
	}
	// Shares are of the weighted total and must account for everything.
	var sum float64
	for _, l := range pt.Lanes {
		sum += l.Share
	}
	if sum < 99.9 || sum > 100.1 {
		t.Errorf("lane shares sum to %.2f%%, want 100%%", sum)
	}
}

// TESTMASTER: id=tokens-redispatch-folds tier=fast parallel=yes
//
// A team that got re-dispatched (badger-app-2) is the same team. Folding
// it keeps the token table row-for-row with the Gantt, which already folds
// redispatches via parseAgentID.
func TestRedispatchedTeamsShareOneLane(t *testing.T) {
	write := tokenTestHome(t)
	proj := t.TempDir()
	write("slug", "sess1", "abadger-app-0123456789abcdef",
		assistantLine("2026-09-15T10:01:00Z", "a1", "claude-opus-5", 0, 400, 0, 40, 0),
	)
	write("slug", "sess1", "abadger-app-2-fedcba9876543210",
		assistantLine("2026-09-15T10:05:00Z", "a2", "claude-opus-5", 0, 400, 0, 60, 0),
	)

	pt := PlanTokenUsage(
		planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z"),
		[]Event{
			teamEvent("badger", "sess1", "abadger-app-0123456789abcdef"),
			teamEvent("badger", "sess1", "abadger-app-2-fedcba9876543210"),
		}, nil)

	var app *LaneTokens
	for i := range pt.Lanes {
		if pt.Lanes[i].Key == "app" {
			app = &pt.Lanes[i]
		}
	}
	if app == nil {
		t.Fatalf("no app lane: %+v", pt.Lanes)
	}
	if app.Usage.Output != 100 {
		t.Errorf("app output = %d, want 100 (both dispatches in one lane)", app.Usage.Output)
	}
}

// TESTMASTER: id=tokens-window tier=fast parallel=yes
//
// A coordinator session routinely outlives many plans — one live session
// here has run plans months apart. Without windowing to the plan's own
// execution span, every plan reports the whole session's spend as its own.
func TestPlanTokenUsageExcludesSpendOutsideTheRunWindow(t *testing.T) {
	write := tokenTestHome(t)
	proj := t.TempDir()
	write("slug", "sess1", "",
		assistantLine("2026-09-10T08:00:00Z", "before", "claude-opus-5", 0, 9999, 0, 9999, 0),
		assistantLine("2026-09-15T10:00:00Z", "during", "claude-opus-5", 0, 100, 0, 50, 0),
		assistantLine("2026-09-20T08:00:00Z", "after", "claude-opus-5", 0, 8888, 0, 8888, 0),
	)

	pt := PlanTokenUsage(
		planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z"),
		[]Event{coordEvent("badger", proj, "sess1")}, nil)

	if pt.Total.Messages != 1 || pt.Total.Output != 50 {
		t.Errorf("windowed total = %d requests / %d output, want 1 / 50 — spend from other plans must not count",
			pt.Total.Messages, pt.Total.Output)
	}
}

// TESTMASTER: id=tokens-coordinator-cwd-scope tier=fast parallel=yes
//
// Plan codenames are drawn from one shared pool and DO collide across
// projects. A coordinator always runs from its plan's own project, so cwd
// is what keeps a same-named plan elsewhere out of these numbers.
func TestCoordinatorLaneIgnoresASameNamedPlanInAnotherProject(t *testing.T) {
	write := tokenTestHome(t)
	proj := t.TempDir()
	other := t.TempDir()
	write("slug", "mine", "",
		assistantLine("2026-09-15T10:00:00Z", "m1", "claude-opus-5", 0, 100, 0, 50, 0),
	)
	write("slug", "theirs", "",
		assistantLine("2026-09-15T10:00:00Z", "t1", "claude-opus-5", 0, 7777, 0, 7777, 0),
	)

	pt := PlanTokenUsage(
		planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z"),
		[]Event{
			coordEvent("badger", proj, "mine"),
			coordEvent("badger", other, "theirs"),
		}, nil)

	if pt.Total.Output != 50 {
		t.Errorf("output = %d, want 50 — the other project's badger must not be counted", pt.Total.Output)
	}
	if len(pt.Sessions) != 1 || pt.Sessions[0] != "mine" {
		t.Errorf("sessions = %v, want [mine]", pt.Sessions)
	}
}

// TESTMASTER: id=tokens-tool-attribution tier=fast parallel=yes
//
// "How many tokens did reading cost?" is answered by how much bigger the
// NEXT request got because the read happened — not by the size of the call
// the model wrote. A Read costs almost no output and then rides along in
// every later turn's context, which is the cost that actually matters.
func TestToolAttributionChargesGrowthToTheCallThatCausedIt(t *testing.T) {
	write := tokenTestHome(t)
	proj := t.TempDir()
	write("slug", "sess1", "",
		// A Read, cheap to write; the next request is 50k bigger.
		assistantLine("2026-09-15T10:00:00Z", "m1", "claude-opus-5", 0, 10_000, 0, 20, 0, "Read"),
		assistantLine("2026-09-15T10:00:10Z", "m2", "claude-opus-5", 0, 60_000, 0, 30, 0, "Edit"),
		// The Edit's result barely grows it.
		assistantLine("2026-09-15T10:00:20Z", "m3", "claude-opus-5", 0, 60_500, 0, 10, 0),
	)

	pt := PlanTokenUsage(
		planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z"),
		[]Event{coordEvent("badger", proj, "sess1")}, nil)

	byTool := map[string]ToolTokens{}
	for _, tl := range pt.Tools {
		byTool[tl.Tool] = tl
	}
	if got := byTool["Read"].ContextGrowth; got != 50_000 {
		t.Errorf("Read context growth = %d, want 50000", got)
	}
	if got := byTool["Edit"].ContextGrowth; got != 500 {
		t.Errorf("Edit context growth = %d, want 500", got)
	}
	if got := byTool["Read"].Emitting; got != 20 {
		t.Errorf("Read emitting = %d, want 20 — writing the call is cheap, its result is not", got)
	}
	if pt.Tools[0].Tool != "Read" {
		t.Errorf("tools sorted %+v, want the expensive one first", pt.Tools)
	}
}

// TESTMASTER: id=tokens-growth-split tier=fast parallel=yes
//
// One request can make several calls, and their results come back
// together — the growth is genuinely not separable, so it is split evenly
// and the page says so. What must never happen is charging the full growth
// to each call, which would multiply the total by the batch size.
func TestParallelCallsSplitTheirSharedContextGrowth(t *testing.T) {
	write := tokenTestHome(t)
	proj := t.TempDir()
	write("slug", "sess1", "",
		assistantLine("2026-09-15T10:00:00Z", "m1", "claude-opus-5", 0, 1000, 0, 40, 0, "Bash", "Read"),
		assistantLine("2026-09-15T10:00:10Z", "m2", "claude-opus-5", 0, 5000, 0, 10, 0),
	)

	pt := PlanTokenUsage(
		planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z"),
		[]Event{coordEvent("badger", proj, "sess1")}, nil)

	var total int64
	for _, tl := range pt.Tools {
		if tl.ContextGrowth != 2000 {
			t.Errorf("%s growth = %d, want 2000 (4000 split two ways)", tl.Tool, tl.ContextGrowth)
		}
		total += tl.ContextGrowth
	}
	if total != 4000 {
		t.Errorf("attributed growth = %d, want exactly the 4000 that happened", total)
	}
}

// TESTMASTER: id=tokens-tick-groups tier=fast parallel=yes
//
// A one-minute /loop against multi-minute turns fires far more often than
// it runs. Reporting those as wake-ups that "ran and did nothing" is the
// difference between a real finding and a scary artifact.
func TestTickGroupsSeparateCoalescedFiringsFromIdleOnes(t *testing.T) {
	write := tokenTestHome(t)
	proj := t.TempDir()
	write("slug", "sess1", "",
		// Fires and works.
		promptLine("2026-09-15T10:00:00Z", "/iterate"),
		assistantLine("2026-09-15T10:00:05Z", "m1", "claude-opus-5", 0, 1000, 0, 50, 0, "Bash"),
		// Fires while that turn is still going: no request of its own.
		promptLine("2026-09-15T10:01:00Z", "/iterate"),
		// Fires, runs, makes no tool call: the expensive kind.
		promptLine("2026-09-15T10:02:00Z", "/iterate"),
		assistantLine("2026-09-15T10:02:05Z", "m2", "claude-opus-5", 0, 2000, 0, 10, 0),
	)

	pt := PlanTokenUsage(
		planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z"),
		[]Event{coordEvent("badger", proj, "sess1")}, nil)

	if len(pt.Groups) != 1 {
		t.Fatalf("groups = %+v, want one /iterate group", pt.Groups)
	}
	g := pt.Groups[0]
	if g.Label != "/iterate" {
		t.Errorf("label = %q, want /iterate", g.Label)
	}
	if g.Count != 3 || g.Ran != 2 || g.Coalesced != 1 || g.Idle != 1 {
		t.Errorf("fired/ran/coalesced/idle = %d/%d/%d/%d, want 3/2/1/1", g.Count, g.Ran, g.Coalesced, g.Idle)
	}
	if g.Median != 2000 {
		t.Errorf("median context = %d, want 2000 — median over the firings that RAN", g.Median)
	}
}

// TESTMASTER: id=tokens-incremental-cache tier=fast parallel=yes
//
// A live coordinator transcript is hundreds of megabytes and the dashboard
// re-renders on every page load, so only appended bytes are re-read. The
// risk that buys is double-counting across passes, or missing the append.
func TestTranscriptCacheReadsOnlyAppendedBytesAndStaysCorrect(t *testing.T) {
	write := tokenTestHome(t)
	proj := t.TempDir()
	plan := planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z")
	events := []Event{coordEvent("badger", proj, "sess1")}

	path := write("slug", "sess1", "",
		assistantLine("2026-09-15T10:00:00Z", "m1", "claude-opus-5", 0, 1000, 0, 100, 0),
	)
	if got := PlanTokenUsage(plan, events, nil).Total.Output; got != 100 {
		t.Fatalf("first pass output = %d, want 100", got)
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.WriteString(assistantLine("2026-09-15T10:05:00Z", "m2", "claude-opus-5", 0, 2000, 0, 200, 0) + "\n"); err != nil {
		t.Fatal(err)
	}
	f.Close()

	pt := PlanTokenUsage(plan, events, nil)
	if pt.Total.Output != 300 || pt.Total.Messages != 2 {
		t.Errorf("after append: %d output / %d requests, want 300 / 2 (neither lost nor double-counted)",
			pt.Total.Output, pt.Total.Messages)
	}
}

// TESTMASTER: id=tokens-streaming-tail tier=fast parallel=yes
//
// The transcript is being appended to while it is read, so the last line
// can be torn mid-write. It must be left for the next pass, not parsed as
// garbage or skipped forever.
func TestATornFinalLineIsPickedUpOnceItIsComplete(t *testing.T) {
	write := tokenTestHome(t)
	proj := t.TempDir()
	plan := planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z")
	events := []Event{coordEvent("badger", proj, "sess1")}

	full := assistantLine("2026-09-15T10:05:00Z", "m2", "claude-opus-5", 0, 2000, 0, 200, 0)
	path := filepath.Join(projectsRoot(), "slug", "sess1.jsonl")
	_ = write("slug", "sess1", "", assistantLine("2026-09-15T10:00:00Z", "m1", "claude-opus-5", 0, 1000, 0, 100, 0))

	// Append half a line, with no newline: a write in flight.
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString(full[:len(full)/2])
	f.Close()

	if got := PlanTokenUsage(plan, events, nil).Total.Output; got != 100 {
		t.Fatalf("with a torn tail output = %d, want 100 — the partial line must be ignored", got)
	}

	// Now the rest of it lands.
	f, _ = os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString(full[len(full)/2:] + "\n")
	f.Close()

	if got := PlanTokenUsage(plan, events, nil).Total.Output; got != 300 {
		t.Errorf("once complete output = %d, want 300 — the line must not be lost", got)
	}
}

// TESTMASTER: id=tokens-lane-missing-transcript tier=fast parallel=yes
//
// A team that made tool calls but whose transcript cannot be found must
// say so. Rendering it as a zero row would read as "this team was free".
func TestALaneWithNoTranscriptIsReportedNotZeroed(t *testing.T) {
	write := tokenTestHome(t)
	proj := t.TempDir()
	write("slug", "sess1", "",
		assistantLine("2026-09-15T10:00:00Z", "c1", "claude-opus-5", 0, 100, 0, 50, 0),
	)

	pt := PlanTokenUsage(
		planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z"),
		[]Event{
			coordEvent("badger", proj, "sess1"),
			teamEvent("badger", "sess1", "abadger-ghost-0123456789abcdef"),
		}, nil)

	var ghost *LaneTokens
	for i := range pt.Lanes {
		if pt.Lanes[i].Key == "ghost" {
			ghost = &pt.Lanes[i]
		}
	}
	if ghost == nil {
		t.Fatalf("the ghost team must still get a lane: %+v", pt.Lanes)
	}
	if !ghost.Missing {
		t.Error("ghost lane should be flagged Missing, not shown as zero spend")
	}
	out := RenderTimelineHTMLWithTokens(
		[]Row{{key: "", label: "coordinator", spans: []span{{start: time.Now().Add(-time.Hour), end: time.Now()}}}},
		planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z"), "", &pt)
	if !strings.Contains(out, "no transcript found") {
		t.Error("page should say the ghost lane's spend is uncounted")
	}
}

// TESTMASTER: id=tokens-panel-render tier=fast parallel=yes
func TestTokenPanelRendersLanesToolsAndWakeups(t *testing.T) {
	write := tokenTestHome(t)
	proj := t.TempDir()
	write("slug", "sess1", "",
		promptLine("2026-09-15T10:00:00Z", "/iterate"),
		assistantLine("2026-09-15T10:00:05Z", "m1", "claude-opus-5", 0, 1_000_000, 0, 5000, 2000, "Read"),
		assistantLine("2026-09-15T10:00:15Z", "m2", "claude-opus-5", 0, 1_500_000, 0, 500, 100),
	)
	write("slug", "sess1", "abadger-app-0123456789abcdef",
		assistantLine("2026-09-15T10:01:00Z", "a1", "claude-opus-5", 0, 400_000, 0, 400, 0),
	)

	plan := planFor("badger", proj, "2026-09-15T09:00:00Z", "2026-09-15T11:00:00Z")
	pt := PlanTokenUsage(plan, []Event{
		coordEvent("badger", proj, "sess1"),
		teamEvent("badger", "sess1", "abadger-app-0123456789abcdef"),
	}, nil)

	rows := []Row{{key: "", label: "coordinator", spans: []span{{start: time.Now().Add(-time.Hour), end: time.Now()}}}}
	out := RenderTimelineHTMLWithTokens(rows, plan, "", &pt)

	for _, want := range []string{
		"Token spend", "context sent", "Who spent it", "coordinator",
		"What it went on, by tool", "Read", "What each wake-up cost", "/iterate",
		"tk-bar", "peak ctx",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("page missing %q", want)
		}
	}
	// The panel's CSS must actually be in the page's stylesheet.
	if !strings.Contains(out, "table.tk{") {
		t.Error("token panel CSS not present in the page head")
	}
	// Rendered before the footer, inside the wrapper.
	if strings.Index(out, "Token spend") > strings.Index(out, "<footer>") {
		t.Error("token panel must render above the footer")
	}
}

// TESTMASTER: id=tokens-panel-omitted tier=fast parallel=yes
//
// The panel is omitted entirely when nothing was measured. An empty table
// implying zero spend would be a lie about a plan that simply predates
// this data or whose teams ran on another machine.
func TestTokenPanelIsOmittedWhenThereIsNothingMeasured(t *testing.T) {
	tokenTestHome(t)
	plan := PlanSummary{Name: "badger"}
	rows := []Row{{key: "", label: "coordinator", spans: []span{{start: time.Now().Add(-time.Hour), end: time.Now()}}}}

	if out := RenderTimelineHTMLWithTokens(rows, plan, "", nil); strings.Contains(out, "Token spend") {
		t.Error("nil tokens must render no panel")
	}
	empty := PlanTokens{Plan: "badger"}
	if out := RenderTimelineHTMLWithTokens(rows, plan, "", &empty); strings.Contains(out, "Token spend") {
		t.Error("an empty PlanTokens must render no panel, not a zero table")
	}
}

// TESTMASTER: id=tokens-human-format tier=fast parallel=yes
//
// Exact below 10k because a 900-token difference matters when reading one
// wake-up; k/M above that, which is the granularity these numbers are
// trustworthy at anyway.
func TestHumanTokensStaysExactWherePrecisionMatters(t *testing.T) {
	cases := []struct {
		in   int64
		want string
	}{
		{0, "0"}, {-5, "0"}, {617, "617"}, {9999, "9999"},
		{10_000, "10k"}, {871_439, "871k"},
		{1_218_760, "1.2M"}, {406_532_685, "407M"},
	}
	for _, c := range cases {
		if got := humanTokens(c.in); got != c.want {
			t.Errorf("humanTokens(%d) = %q, want %q", c.in, got, c.want)
		}
	}
}

// TESTMASTER: id=tokens-cli-print tier=fast parallel=yes
func TestPrintPlanTokensSaysSoWhenThereIsNoData(t *testing.T) {
	var b strings.Builder
	PrintPlanTokens(&b, PlanTokens{Plan: "badger"})
	if !strings.Contains(b.String(), "no token data") {
		t.Errorf("want an explicit no-data line, got %q", b.String())
	}

	b.Reset()
	PrintPlanTokens(&b, PlanTokens{
		Plan:  "badger",
		Total: TokenUsage{CacheRead: 1000, Output: 100, Messages: 2},
		Lanes: []LaneTokens{{Key: "", Label: "coordinator", Share: 100, Usage: TokenUsage{CacheRead: 1000, Output: 100, Messages: 2}}},
		Tools: []ToolTokens{{Tool: "Bash", Calls: 3, ContextGrowth: 900, Share: 100}},
		Groups: []TickGroup{{Label: "/iterate", Count: 5, Ran: 2, Idle: 1, Coalesced: 3,
			Usage: TokenUsage{CacheRead: 1000}, Median: 500, Share: 100}},
	})
	for _, want := range []string{"coordinator", "Bash", "/iterate", "LANE", "TOOL", "WAKE-UP"} {
		if !strings.Contains(b.String(), want) {
			t.Errorf("CLI output missing %q:\n%s", want, b.String())
		}
	}
}

// TESTMASTER: id=tokens-lane-label-trim tier=fast parallel=yes
//
// Most lanes are short team names, but a plain Agent-tool dispatch is
// labelled with its own description, which is a sentence. Seen live on
// newcorder's xenops: "Map profile editor for Destination control" broke
// the CLI table's column alignment outright.
func TestLongLaneLabelsAreTrimmedToFitAColumn(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"coordinator", "coordinator"},
		{"notes-core", "notes-core"},
		{"Map profile editor for Destination control", "Map profile editor for…"},
		{"Map History tab and tab dispatch", "Map History tab and tab…"},
		// No usable word boundary early enough: a hard cut.
		{strings.Repeat("x", 40), strings.Repeat("x", 25) + "…"},
	}
	for _, c := range cases {
		got := laneLabel(c.in)
		if got != c.want {
			t.Errorf("laneLabel(%q) = %q, want %q", c.in, got, c.want)
		}
		if len([]rune(got)) > 26 {
			t.Errorf("laneLabel(%q) = %q, %d runes — wider than the column", c.in, got, len([]rune(got)))
		}
	}
}
