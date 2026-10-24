package iterrun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeRecordPlan(t *testing.T, project, name, front string) string {
	t.Helper()
	dir := filepath.Join(project, ".claude", "iterate", "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+".md")
	body := "name: " + name + "\n" + front + "\n## Goal\nFixture only\n## Steps\n- [ ] 1. Write receipt\n## Validation\n- [ ] 1. Read receipt\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func recordHook(t *testing.T, cwd, session, agent string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"cwd": cwd, "session_id": session, "agent_id": agent, "tool_name": "Bash", "tool_use_id": "t-" + session + agent, "tool_input": map[string]string{"command": "true"}})
	HandleHook("pre", strings.NewReader(string(payload)))
}

// TESTMASTER: id=record-hook-observation tier=fast parallel=no
func TestRecordHookObservesOnlyExecutingPlanInItsProject(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ITERATE_RUN_RECORD_DIR", t.TempDir())
	project, other := t.TempDir(), t.TempDir()
	path := writeRecordPlan(t, project, "alpha", "phase: planned\nstatus: queued\nharness: codex\nexecutor-version: 5.22.0")
	if err := os.WriteFile(filepath.Join(project, ".claude", "iterate", "current"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recordHook(t, project, "s-draft", "")
	if runs, _ := ListRecordRuns("", ""); len(runs) != 0 {
		t.Fatalf("drafting activity opened a run: %+v", runs)
	}

	writeRecordPlan(t, project, "alpha", "phase: executing\nExecuting: 2026-10-20T10:00:00Z\nharness: codex\nexecutor-version: 5.22.0")
	recordHook(t, project, "s1", "")
	recordHook(t, project, "s1", "aalpha-app-0123456789abcdef")
	recordHook(t, project, "s1", "")
	text, _ := os.ReadFile(path)
	if err := os.WriteFile(path, []byte(strings.Replace(string(text), "phase: executing", "phase: executing\nstatus: paused", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	recordHook(t, project, "s1", "")
	recordHook(t, project, "s1", "")

	// A same-named plan in another project, executing, stays separate.
	writeRecordPlan(t, other, "alpha", "phase: executing\nExecuting: 2026-10-20T10:00:00Z")
	if err := os.WriteFile(filepath.Join(other, ".claude", "iterate", "current"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recordHook(t, other, "s-other", "")

	rep, err := ReadRecordReport(project, "alpha", "")
	if err != nil || rep.Source != "record" || len(rep.Runs) != 1 {
		t.Fatalf("report: %+v %v", rep, err)
	}
	r := rep.Runs[0]
	if r.Marker != "2026-10-20T10:00:00Z" || r.Versions.Engine != "claude-code" || r.Versions.ExecutorVersion != "5.22.0" {
		t.Fatalf("identity: %+v", r)
	}
	if len(r.Sessions) != 1 || r.Sessions[0].SessionID != "s1" || len(r.Agents) != 1 || r.Agents[0].Team != "app" {
		t.Fatalf("sessions/agents: %+v %+v", r.Sessions, r.Agents)
	}
	if len(r.Lifecycle) != 2 || r.Lifecycle[0].State != "executing" || r.Lifecycle[1].State != "paused" || r.State != "paused" {
		t.Fatalf("lifecycle: %+v", r.Lifecycle)
	}
	otherRep, err := ReadRecordReport(other, "alpha", "")
	if err != nil || len(otherRep.Runs) != 1 || otherRep.Runs[0].RunID == r.RunID || otherRep.Runs[0].Sessions[0].SessionID != "s-other" {
		t.Fatalf("other project: %+v %v", otherRep, err)
	}

	// A completed plan file records completion into its run and opens none.
	writeRecordPlan(t, project, "alpha", "phase: complete\nExecuting: 2026-10-20T10:00:00Z\nFinished: 2026-10-20T12:00:00Z")
	recordHook(t, project, "s1", "")
	rep, _ = ReadRecordReport(project, "alpha", "")
	if len(rep.Runs) != 1 || rep.Runs[0].RunID != r.RunID || rep.Runs[0].State != "completed" {
		t.Fatalf("completion: %+v", rep.Runs)
	}
	fresh := t.TempDir()
	writeRecordPlan(t, fresh, "alpha", "phase: complete\nExecuting: 2026-10-20T10:00:00Z")
	if err := os.WriteFile(filepath.Join(fresh, ".claude", "iterate", "current"), []byte("alpha\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	recordHook(t, fresh, "s-late", "")
	if runs, _ := RecordStore().Runs(fresh, "alpha"); len(runs) != 0 {
		t.Fatalf("a completed plan opened a run: %+v", runs)
	}
}
