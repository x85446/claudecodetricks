package iterrun

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/runrecord"
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
	if r.Marker != "2026-10-20T10:00:00Z" || r.Versions.Engine != "codex" || r.Versions.ExecutorVersion != "5.22.0" {
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

// recordAgentScript is a /bin/sh fixture agent: it streams a session and
// two cumulative usage totals, then completes the assignment by echoing the
// identity template from its prompt with a completed outcome.
const recordAgentScript = `printf '%s\n' '{"type":"session","kind":"session","session_id":"sess-1","provider_id":"session-1"}'
printf '%s\n' '{"type":"usage","kind":"usage","session_id":"sess-1","provider_id":"usage-1","input_tokens":100,"output_tokens":7,"cumulative":true}'
printf '%s\n' '{"type":"usage","kind":"usage","session_id":"sess-1","provider_id":"usage-2","input_tokens":120,"output_tokens":9,"cumulative":true}'
printf '%s\n' "$1" | sed -n 's/.*identity fields in this template: \(.*\)\. The evidence field.*/\1/p' |
  sed -e 's/"outcome":"partial"/"outcome":"completed"/' -e 's/"task_complete":false/"task_complete":true/' \
      -e 's/"validation_met":false/"validation_met":true/' -e 's/Concrete verification command and observed result/receipt read back: 1 line/' > "$2"
`

// TESTMASTER: id=record-conductor-ingestion tier=standard parallel=no
func TestConductorAssignmentsLandInTheRunRecord(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ITERATE_RUN_RECORD_DIR", t.TempDir())
	project := t.TempDir()
	writeRecordPlan(t, project, "alpha", "phase: planned\nstatus: queued\nStarted: 2026-10-01T00:00:00Z")
	script := filepath.Join(t.TempDir(), "agent.sh")
	if err := os.WriteFile(script, []byte(recordAgentScript), 0o600); err != nil {
		t.Fatal(err)
	}
	s := NewConductorService(t.TempDir())
	t.Setenv("ITERATE_CONDUCTOR_DIR", s.Dir)
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	s.Clock = func() time.Time { return now }
	s.PollInterval = 20 * time.Millisecond
	canonical, _ := conductor.Canonical(project)
	c := model.Config{Version: 2, Enabled: true, PowerBands: []model.PowerBand{budget.Universal()},
		Projects:      []model.Project{{ID: "project", Name: "fixture", Path: canonical, Enabled: true, MaxAttempts: 5, NoProgressSeconds: 3600}},
		Subscriptions: []model.Subscription{{ID: "account", Name: "Fixture", Enabled: true, Adapter: "jsonl", ResultTransport: "argv_file", Executable: "/bin/sh", Args: []string{script, "{prompt}", "{result}"}, PoolID: "pool", AccountID: "identity", PowerBandID: "universal", Sequences: []model.Sequence{{ID: "sequence", Name: "build"}}}},
		Automations:   []model.Automation{{ID: "rule", Name: "Work", ProjectID: "project", Enabled: true, Mode: "keep_working", Entries: []model.Entry{{ID: "entry", SequenceID: "sequence", MaxBand: model.Red}}}}}
	c, err := s.Apply(0, c)
	if err != nil {
		t.Fatal(err)
	}
	usage := func(_ context.Context, sub model.Subscription) (model.Usage, error) {
		return model.Usage{SubscriptionID: sub.ID, PoolID: sub.PoolID, AccountID: sub.AccountID, Source: "fixture", ObservedAt: now, Supported: true,
			Weekly: &model.Allowance{UsedPercent: 10, StartsAt: now.Add(-72 * time.Hour), ResetsAt: now.Add(96 * time.Hour), ResetTrusted: true, Epoch: "w"},
			Short:  &model.Allowance{UsedPercent: 0, StartsAt: now.Add(-time.Hour), ResetsAt: now.Add(4 * time.Hour), ResetTrusted: true, Epoch: "s"}}, nil
	}
	s.CollectUsage = usage
	u, _ := usage(context.Background(), c.Subscriptions[0])
	if err := s.PutUsage(u); err != nil {
		t.Fatal(err)
	}

	pair, err := s.Tick(context.Background())
	if err != nil || pair.Result == nil || pair.Result.Outcome != "completed" || pair.Assignment.Kind != "pair" {
		t.Fatalf("pair tick: %+v %v", pair, err)
	}
	delivery, err := s.Tick(context.Background())
	if err != nil || delivery.Result == nil || delivery.Result.Outcome != "completed" || delivery.Assignment.Kind != "delivery" {
		t.Fatalf("delivery tick: %+v %v", delivery, err)
	}

	runs, err := RecordStore().Runs(project, "alpha")
	if err != nil || len(runs) != 1 {
		t.Fatalf("runs: %d %v", len(runs), err)
	}
	r := runs[0]
	if !r.Archived || !strings.HasSuffix(r.ArchiveFile, "-alpha-done.md") || r.State != "archived" || r.Provenance != "observed" {
		t.Fatalf("archive: %+v", r)
	}
	states := []string{}
	for _, l := range r.Lifecycle {
		states = append(states, l.State)
	}
	if got := strings.Join(states, ","); got != "admitted,executing,admitted,executing,completed,archived" {
		t.Fatalf("lifecycle %s", got)
	}
	if r.Owner != nil || len(r.Ownership) != 4 {
		t.Fatalf("ownership: %+v %+v", r.Owner, r.Ownership)
	}
	var step1, plan *runrecord.RequirementView
	for i := range r.Requirements {
		switch r.Requirements[i].Number {
		case 1:
			step1 = &r.Requirements[i]
		case 0:
			plan = &r.Requirements[i]
		}
	}
	if step1 == nil || plan == nil || len(step1.Attempts) != 1 || len(plan.Attempts) != 1 {
		t.Fatalf("requirements: %+v", r.Requirements)
	}
	a := step1.Attempts[0]
	if a.Outcome != "passed" || a.AssignmentID != pair.Assignment.ID || a.Model != "" || a.StartedAt == nil || len(a.Evidence) != 1 || !a.Evidence[0].Imported || a.Evidence[0].Result != "passed" {
		t.Fatalf("attempt: %+v", a)
	}
	if step1.TaskChecked == nil || !*step1.TaskChecked || plan.Attempts[0].Kind != "delivery" || plan.Attempts[0].Outcome != "passed" {
		t.Fatalf("checkpoint/delivery: %+v %+v", step1, plan.Attempts[0])
	}
	exact := r.Usage.ByScope[runrecord.ScopeExact]
	// Cumulative totals per request: the later 120/9 replaces 100/7, once
	// per assignment.
	if exact.Requests != 2 || *exact.InputTokens != 240 || *exact.OutputTokens != 18 || a.Usage.Requests != 1 {
		t.Fatalf("usage: %+v attempt %+v", exact, a.Usage)
	}
	if len(r.Sessions) != 1 || r.Sessions[0].SessionID != "sess-1" {
		t.Fatalf("sessions: %+v", r.Sessions)
	}

	// Archive selection finds the same run; a later execution of the same
	// codename opens a new one.
	rep, err := ReadRecordReport(project, "", r.ArchiveFile)
	if err != nil || rep.Source != "record" || len(rep.Runs) != 1 || rep.Runs[0].RunID != r.RunID {
		t.Fatalf("archive report: %+v %v", rep, err)
	}
	writeRecordPlan(t, project, "alpha", "phase: executing\nExecuting: 2026-10-21T09:00:00Z")
	recordHook(t, project, "s-rerun", "")
	runs, _ = RecordStore().Runs(project, "alpha")
	if len(runs) != 2 || runs[1].RunID == r.RunID || runs[1].Archived || runs[0].RunID != r.RunID {
		t.Fatalf("re-run: %+v", runs)
	}
}
