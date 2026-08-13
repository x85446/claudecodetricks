package iterrun

import (
	"strings"
	"testing"
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
