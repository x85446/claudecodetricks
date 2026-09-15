package iterrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestPendingSpawnLabelFIFOOrdering(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := PushPendingSpawnLabel("sess1", "owl-database"); err != nil {
		t.Fatal(err)
	}
	if err := PushPendingSpawnLabel("sess1", "owl-gui"); err != nil {
		t.Fatal(err)
	}

	label, ok, err := PopPendingSpawnLabel("sess1")
	if err != nil || !ok || label != "owl-database" {
		t.Fatalf("first pop = %q, %v, %v; want owl-database, true, nil", label, ok, err)
	}

	label, ok, err = PopPendingSpawnLabel("sess1")
	if err != nil || !ok || label != "owl-gui" {
		t.Fatalf("second pop = %q, %v, %v; want owl-gui, true, nil", label, ok, err)
	}

	_, ok, err = PopPendingSpawnLabel("sess1")
	if err != nil || ok {
		t.Fatalf("third pop on drained queue = ok=%v, err=%v; want ok=false, err=nil", ok, err)
	}
}

func TestPendingSpawnLabelEmptyQueueUnknownSession(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	label, ok, err := PopPendingSpawnLabel("never-pushed")
	if err != nil || ok || label != "" {
		t.Fatalf("pop on unknown session = %q, %v, %v; want \"\", false, nil", label, ok, err)
	}
}

func TestHandleHookSubagentStartClaimsQueuedLabel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	if err := PushPendingSpawnLabel("sess1", "owl-database"); err != nil {
		t.Fatal(err)
	}

	payload := `{"session_id":"sess1","cwd":"/tmp/proj","agent_id":"agent-abc","agent_type":"default","hook_event_name":"SubagentStart"}`
	HandleHook("subagent-start", strings.NewReader(payload))

	labels, err := ReadLabels()
	if err != nil {
		t.Fatal(err)
	}
	if got := labels["agent-abc"]; got != "owl-database" {
		t.Fatalf("labels[agent-abc] = %q; want owl-database (labels: %v)", got, labels)
	}

	// The queue should be drained, not just peeked.
	if _, ok, _ := PopPendingSpawnLabel("sess1"); ok {
		t.Fatal("expected queue to be empty after SubagentStart claimed the label")
	}
}

func TestHandleHookSubagentStartNoQueuedLabelIsNoop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	payload := `{"session_id":"sess-no-queue","cwd":"/tmp/proj","agent_id":"agent-xyz","agent_type":"default","hook_event_name":"SubagentStart"}`
	HandleHook("subagent-start", strings.NewReader(payload))

	labels, err := ReadLabels()
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := labels["agent-xyz"]; ok {
		t.Fatalf("expected no label assigned when nothing was queued, got %q", labels["agent-xyz"])
	}
}

// Confirmed against a real Codex CLI 0.147.0 run: a spawn_agent PostToolUse
// reports tool_name "collaborationspawn_agent" and a tool_response with no
// agent id at all — just {"task_name": "..."}. This is why the label has to
// be queued here and claimed later at SubagentStart, unlike Claude Code's
// Agent tool (see TestHandleHookPostAgentToolStillResolvesLabelDirectly).
func TestHandleHookPostRecognizesCodexSpawnToolAndQueuesLabel(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	payload := `{"session_id":"sess1","cwd":"/tmp/proj","tool_name":"collaborationspawn_agent","tool_use_id":"call_1","tool_input":{"task_name":"owl-database","fork_turns":"all","message":"gAAAAA-encrypted-blob"},"tool_response":"{\"task_name\":\"/root/owl-database\"}"}`
	HandleHook("post", strings.NewReader(payload))

	label, ok, err := PopPendingSpawnLabel("sess1")
	if err != nil {
		t.Fatal(err)
	}
	if !ok || label != "owl-database" {
		t.Fatalf("PopPendingSpawnLabel = %q, %v; want owl-database, true", label, ok)
	}
}

func TestHandleHookPostIgnoresNonSpawnToolForQueue(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	payload := `{"session_id":"sess1","cwd":"/tmp/proj","tool_name":"Bash","tool_use_id":"call_1","tool_input":{"command":"echo hi"}}`
	HandleHook("post", strings.NewReader(payload))

	if _, ok, _ := PopPendingSpawnLabel("sess1"); ok {
		t.Fatal("expected no queued label from an ordinary Bash PostToolUse event")
	}
}

// Unchanged Claude Code path: the Agent tool's own PostToolUse response
// carries the new agent's id directly, so the label resolves in one step,
// with no queue involved at all. This must keep working exactly as before.
func TestHandleHookPostAgentToolStillResolvesLabelDirectly(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	payload := `{"session_id":"sess1","cwd":"/tmp/proj","tool_name":"Agent","tool_use_id":"call_1","tool_input":{"description":"owl-database","subagent_type":"backend-expert"},"tool_response":{"agentId":"claude-agent-1"}}`
	HandleHook("post", strings.NewReader(payload))

	labels, err := ReadLabels()
	if err != nil {
		t.Fatal(err)
	}
	if got := labels["claude-agent-1"]; got != "owl-database" {
		t.Fatalf("labels[claude-agent-1] = %q; want owl-database", got)
	}

	// The Claude path never touches the Codex spawn queue.
	if _, ok, _ := PopPendingSpawnLabel("sess1"); ok {
		t.Fatal("Agent-tool PostToolUse should not push anything onto the Codex spawn-label queue")
	}
}

func TestHandleHookSubagentStopIsNoop(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	payload := `{"session_id":"sess1","cwd":"/tmp/proj","agent_id":"agent-abc","agent_type":"default","hook_event_name":"SubagentStop","last_assistant_message":"done"}`
	HandleHook("subagent-stop", strings.NewReader(payload))

	events, err := ReadEvents()
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 0 {
		t.Fatalf("expected subagent-stop to record no Event, got %d", len(events))
	}
}

func TestResolvePlanTeamNeverFilesASubagentUnderTheCoordinator(t *testing.T) {
	// A team shares the plan's working tree — that is the design, teams never
	// switch branches — so cwd carries the plan pointer for team events too.
	// Resolving cwd first returned team "" (the coordinator's key) for every
	// one of them, which is the bug this pins shut.
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".claude", "iterate"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude", "iterate", "current"), []byte("galago\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plan, team := resolvePlanTeam(dir, "agalago-reporting-f68431d4419554c9")
	if plan != "galago" || team != "reporting" {
		t.Errorf("team event resolved to (%q, %q), want (galago, reporting)", plan, team)
	}

	// The coordinator is the one that legitimately has no agent id.
	plan, team = resolvePlanTeam(dir, "")
	if plan != "galago" || team != "" {
		t.Errorf("coordinator event resolved to (%q, %q), want (galago, \"\")", plan, team)
	}

	// An unrecognized subagent gets its own lane, never the coordinator's.
	plan, team = resolvePlanTeam(dir, "some-unlabeled-agent")
	if team == "" {
		t.Error("an unresolvable subagent was filed under the coordinator")
	}
	if plan != "galago" {
		t.Errorf("plan = %q, want galago from cwd even when the team is unknown", plan)
	}
}

func TestParseAgentIDFoldsRedispatchesIntoOneTeam(t *testing.T) {
	cases := []struct {
		id, plan, team string
	}{
		{"agalago-reporting-f68431d4419554c9", "galago", "reporting"},
		{"agalago-trees-2-ad83a94607dcf3c4", "galago", "trees"},
		{"agalago-ledger-2-582347734cd7356f", "galago", "ledger"},
		{"akagu-app-b8bb3f96baf442e5", "kagu", "app"},
		{"agalago-skills-harness-1234567890abcdef", "galago", "skills-harness"},
		// Not subagent ids: a session uuid, a short hash, an empty team.
		{"019ffc85-3f60-7e40-bb89-aac9f9c5904f", "", ""},
		{"a32fb967a117a0a81", "", ""},
		{"", "", ""},
	}
	for _, c := range cases {
		plan, team := parseAgentID(c.id)
		if plan != c.plan || team != c.team {
			t.Errorf("parseAgentID(%q) = (%q, %q), want (%q, %q)", c.id, plan, team, c.plan, c.team)
		}
	}
}

func TestBuildRowsFromHookEventsGivesEachTeamItsOwnLane(t *testing.T) {
	base := time.Date(2026, 9, 15, 17, 0, 0, 0, time.UTC)
	ev := func(hook, tid, agent, team string, off time.Duration) Event {
		return Event{
			Hook: hook, ToolUseID: tid, AgentID: agent, Team: team,
			Plan: "galago", CWD: "/p", TS: base.Add(off), ToolName: "Bash",
		}
	}
	events := []Event{
		// coordinator
		ev("pre", "c1", "", "", 0), ev("post", "c1", "", "", 2*time.Second),
		// reporting, working in the plan's own tree — the case that used to
		// land in the coordinator's row
		ev("pre", "r1", "agalago-reporting-f68431d4419554c9", "reporting", time.Minute),
		ev("post", "r1", "agalago-reporting-f68431d4419554c9", "reporting", time.Minute+30*time.Second),
		ev("pre", "r2", "agalago-reporting-f68431d4419554c9", "reporting", 20*time.Minute),
		ev("post", "r2", "agalago-reporting-f68431d4419554c9", "reporting", 20*time.Minute+5*time.Second),
	}
	rows := BuildRowsFromHookEvents(events, nil, "galago", "/p", time.Time{})
	byKey := map[string]Row{}
	for _, r := range rows {
		byKey[r.key] = r
	}
	coord, ok := byKey[""]
	if !ok {
		t.Fatal("no coordinator row")
	}
	if len(coord.spans) != 1 {
		t.Errorf("coordinator has %d spans, want 1 — team calls are leaking into it", len(coord.spans))
	}
	rep, ok := byKey["reporting"]
	if !ok {
		t.Fatal("no reporting row — its calls went somewhere else")
	}
	if len(rep.spans) != 2 {
		t.Errorf("reporting has %d spans, want 2", len(rep.spans))
	}
	if len(rep.gaps) != 1 {
		t.Errorf("reporting has %d gaps, want 1 (its own 18m idle stretch)", len(rep.gaps))
	}
}
