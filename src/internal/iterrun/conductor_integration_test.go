package iterrun

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

// TESTMASTER: id=indigobird-stream-dedupe tier=? parallel=no
func TestConductorOwnedHookObservation(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	s := NewConductorService(t.TempDir())
	a := model.Assignment{ID: "assignment", RunID: "run", ProjectPath: t.TempDir(), Plan: "alpha"}
	runtime := conductor.Runtime{Leases: map[string]conductor.Lease{a.ID: {Assignment: a}}}
	b, _ := json.Marshal(runtime)
	if e := os.WriteFile(filepath.Join(s.Dir, "runtime.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	t.Setenv("ITERATE_CONDUCTOR_DIR", s.Dir)
	t.Setenv("ITERATE_CONDUCTOR_ASSIGNMENT_ID", a.ID)
	payload, _ := json.Marshal(map[string]any{"cwd": a.ProjectPath, "session_id": "session", "tool_name": "Bash", "tool_use_id": "tool-id", "tool_input": map[string]string{"command": "secret raw input"}})
	HandleHook("pre", strings.NewReader(string(payload)))
	HandleHook("pre", strings.NewReader(string(payload)))
	if e := s.RecordEvent(a, model.Event{ProviderID: "tool:tool-id:start", Kind: "tool", SessionID: "session"}); e != nil {
		t.Fatal(e)
	}
	HandleHook("post", strings.NewReader(string(payload)))
	p, e := s.Events(a.RunID, 0, 100)
	if e != nil || len(p.Events) != 2 {
		t.Fatalf("deduped hooks: %+v %v", p, e)
	}
	for _, event := range p.Events {
		if event.AssignmentID != a.ID || strings.Contains(event.Message, "secret") || len(event.Data) != 0 {
			t.Fatalf("unsafe observation: %+v", event)
		}
	}
	t.Setenv("ITERATE_CONDUCTOR_ASSIGNMENT_ID", "unknown")
	HandleHook("post", strings.NewReader(string(payload)))
	p, e = s.Events(a.RunID, 0, 100)
	if e != nil || len(p.Events) != 2 {
		t.Fatal("unknown assignment wrote events")
	}
}

// TESTMASTER: id=indigobird-execution-reactivation tier=? parallel=yes
func TestConductorQueueWakeAfterClosedWindow(t *testing.T) {
	s, c, _ := integrationFixture(t)
	started := 0
	start := func(dir, project string) (int, error) {
		if dir != s.Dir || project != "" {
			t.Fatal("wrong worker scope")
		}
		started++
		return 123, nil
	}
	c.Enabled = false
	c, _ = s.Apply(c.Revision, c)
	if e := wakeConductor(context.Background(), s, start); e != nil || started != 0 {
		t.Fatalf("disabled wake: %v %d", e, started)
	}
	c.Enabled = true
	c.Paused = true
	c, _ = s.Apply(c.Revision, c)
	_ = wakeConductor(context.Background(), s, start)
	if started != 0 {
		t.Fatal("paused wake")
	}
	c.Paused = false
	c, _ = s.Apply(c.Revision, c)
	// Waiting budgets still wake to refresh authenticated usage and reevaluate.
	if e := wakeConductor(context.Background(), s, start); e != nil || started != 1 {
		t.Fatalf("queued wake: %v %d", e, started)
	}
	if e := os.Remove(filepath.Join(c.Projects[0].Path, ".claude", "iterate", "plans", "alpha.md")); e != nil {
		t.Fatal(e)
	}
	_ = wakeConductor(context.Background(), s, start)
	if started != 1 {
		t.Fatal("empty queue started worker")
	}
	c.Projects[0].IntakeEnabled = true
	_, _ = s.Apply(c.Revision, c)
	if e := wakeConductor(context.Background(), s, start); e != nil || started != 2 {
		t.Fatalf("intake wake: %v %d", e, started)
	}
}

func integrationFixture(t *testing.T) (*conductor.Service, model.Config, time.Time) {
	t.Helper()
	dir := t.TempDir()
	plans := filepath.Join(dir, ".claude", "iterate", "plans")
	if e := os.MkdirAll(plans, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(plans, "alpha.md"), []byte("name: alpha\nphase: planned\nstatus: queued\nStarted: 2026-10-01T00:00:00Z\n\n## Goal\nFixture only\n## Steps\n- [ ] 1. Write receipt\n## Validation\n- [ ] 1. Read receipt\n"), 0600); e != nil {
		t.Fatal(e)
	}
	s := NewConductorService(t.TempDir())
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	s.Clock = func() time.Time { return now }
	c := model.Config{Version: 2, Enabled: true, PowerBands: []model.PowerBand{budget.Universal()}, Projects: []model.Project{{ID: "project", Name: "fixture", Path: dir, Enabled: true}}, Subscriptions: []model.Subscription{{ID: "first", Name: "First", Enabled: true, Adapter: "generic", Executable: "/usr/bin/true", Args: []string{"{prompt}", "{result}"}, PoolID: "pool", AccountID: "account", PowerBandID: "universal", Sequences: []model.Sequence{{ID: "sequence", Name: "build"}}}}, Automations: []model.Automation{{ID: "rule", Name: "Work", ProjectID: "project", Enabled: true, Mode: "keep_working", Entries: []model.Entry{{ID: "entry", SequenceID: "sequence", MaxBand: model.Green}}}}}
	var e error
	c, e = s.Apply(0, c)
	if e != nil {
		t.Fatal(e)
	}
	u := model.Usage{SubscriptionID: "first", PoolID: "pool", AccountID: "account", Source: "isolated acceptance fixture", ObservedAt: now, Supported: true, Weekly: &model.Allowance{UsedPercent: 10, StartsAt: now.Add(-3 * 24 * time.Hour), ResetsAt: now.Add(4 * 24 * time.Hour), ResetTrusted: true, Epoch: "fixture"}, Short: &model.Allowance{UsedPercent: 0, StartsAt: now.Add(-time.Hour), ResetsAt: now.Add(4 * time.Hour), ResetTrusted: true, Epoch: "short"}}
	if e = s.PutUsage(u); e != nil {
		t.Fatal(e)
	}
	return s, c, now
}

func decisionHTTP(t *testing.T, h http.Handler, request conductor.EvaluateRequest) conductor.Decision {
	t.Helper()
	w := conductorRequest(t, h, "POST", "evaluate", "http://localhost:8420", "localhost:8420", request)
	if w.Code != 200 {
		t.Fatalf("evaluation: %d %s", w.Code, w.Body)
	}
	var envelope struct {
		Data conductor.Decision `json:"data"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &envelope); e != nil {
		t.Fatal(e)
	}
	return envelope.Data
}

// TESTMASTER: id=indigobird-ordered-profiles-bands tier=? parallel=yes
func TestConductorIntegrationCumulativeBandsAndSampleIsolation(t *testing.T) {
	s, c, _ := integrationFixture(t)
	h := ConductorAPI(s)
	beforeConfig, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	beforeRuntime, _ := os.ReadFile(filepath.Join(s.Dir, "runtime.json"))
	bands := []model.Band{model.Green, model.Yellow, model.Red}
	for maxRank, max := range bands {
		for currentRank, current := range bands {
			draftBytes, _ := json.Marshal(c)
			var draft model.Config
			_ = json.Unmarshal(draftBytes, &draft)
			draft.Automations[0].Entries[0].MaxBand = max
			d := decisionHTTP(t, h, conductor.EvaluateRequest{ProjectID: "project", AutomationID: "rule", Draft: &draft, Samples: map[string]conductor.SampleAccount{"first": {Band: current, Availability: "ready"}}})
			if (d.Selected != nil) != (currentRank <= maxRank) {
				t.Fatalf("current %s max %s: %+v", current, max, d)
			}
		}
	}
	used := 100.0
	d := decisionHTTP(t, h, conductor.EvaluateRequest{Samples: map[string]conductor.SampleAccount{"first": {WeeklyUsed: &used, Availability: "ready"}}})
	if d.Selected != nil {
		t.Fatal("sample hard exhaustion launched")
	}
	afterConfig, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	afterRuntime, _ := os.ReadFile(filepath.Join(s.Dir, "runtime.json"))
	if !bytes.Equal(beforeConfig, afterConfig) || !bytes.Equal(beforeRuntime, afterRuntime) {
		t.Fatal("sample mutated persisted settings or runtime")
	}
}

// TESTMASTER: id=indigobird-settings-universal tier=? parallel=yes
func TestConductorIntegrationNamedPolicyScopeAndSharedPool(t *testing.T) {
	s, c, _ := integrationFixture(t)
	other := c.Subscriptions[0]
	other.ID = "second"
	other.Name = "Second"
	other.PoolID = "other-pool"
	other.AccountID = "other-account"
	other.Sequences = []model.Sequence{{ID: "second-sequence", Name: "build"}}
	custom := budget.Universal()
	custom.ID = "custom"
	custom.Name = "Custom"
	custom.ShortGreen = 30
	c.PowerBands = append(c.PowerBands, custom)
	other.PowerBandID = "custom"
	c.Subscriptions = append(c.Subscriptions, other)
	var e error
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	used := 25.
	max := model.Green
	first, e := s.BudgetPreview(context.Background(), conductor.BudgetPreviewRequest{SubscriptionID: "first", MaxBand: max, Sample: &conductor.SampleAccount{ShortUsed: &used, Availability: "ready"}})
	if e != nil {
		t.Fatal(e)
	}
	second, e := s.BudgetPreview(context.Background(), conductor.BudgetPreviewRequest{SubscriptionID: "second", MaxBand: max, Sample: &conductor.SampleAccount{ShortUsed: &used, Availability: "ready"}})
	if e != nil {
		t.Fatal(e)
	}
	if first.Allowed || !second.Allowed {
		t.Fatalf("independent assigned policies: first=%+v second=%+v", first, second)
	}
	c.PowerBands[0].ShortGreen = 30
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	loaded, e := conductor.NewService(s.Dir, s.Source).Config()
	if e != nil || loaded.Subscriptions[1].PowerBandID != "custom" || loaded.PowerBands[1].ShortGreen != 30 {
		t.Fatalf("restart policy isolation: %+v %v", loaded, e)
	}
	c.Subscriptions[1].AccountID = "account"
	c.Subscriptions[1].PoolID = "pool"
	c.Automations[0].Entries = append(c.Automations[0].Entries, model.Entry{ID: "second-entry", SequenceID: "second-sequence", MaxBand: model.Red})
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	used = 100
	d := decisionHTTP(t, ConductorAPI(s), conductor.EvaluateRequest{Samples: map[string]conductor.SampleAccount{"first": {WeeklyUsed: &used, Availability: "ready"}}})
	if d.Selected != nil || len(d.Candidates) != 2 {
		t.Fatalf("shared pool sample bypass: %+v", d)
	}
	for _, row := range d.Candidates {
		if row.Budget.WeeklyUsed == nil || *row.Budget.WeeklyUsed != 100 {
			t.Fatal("shared sample diverged", row)
		}
	}
}

// TESTMASTER: id=indigobird-overdrive-bar-normal tier=? parallel=yes
func TestConductorIntegrationOverdriveGeometryAndAvailability(t *testing.T) {
	s, c, _ := integrationFixture(t)
	sub := &c.Subscriptions[0]
	sub.OverdriveEnabled = true
	sub.OverdriveWindows = []model.OverdriveWindow{{ID: "window", Name: "Capacity", Enabled: true, TargetUsedPercent: 60, EndMode: "recurrence", Recurrence: model.Recurrence{Days: []int{5}, Start: "00:00", End: "00:00", Timezone: "UTC", EveryWeeks: 1}}}
	c.Automations[0].Mode = "overdrive"
	c.Automations[0].HonorOverdrive = true
	c.Automations[0].OnlyOverdrive = true
	c.Automations[0].Entries = nil
	c.Automations[0].Participants = []string{"sequence"}
	var e error
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	used := 56.
	day := 6.
	h := ConductorAPI(s)
	d := decisionHTTP(t, h, conductor.EvaluateRequest{Samples: map[string]conductor.SampleAccount{"first": {Day: &day, WeeklyUsed: &used, Availability: "ready"}}})
	if d.Selected == nil || !d.Selected.Budget.OverdriveActive || d.Selected.Budget.TargetUsedPercent != 60 || d.Selected.Budget.GreenEnd <= 60 {
		t.Fatalf("bright target plus normal green: %+v", d)
	}
	used = 60
	d = decisionHTTP(t, h, conductor.EvaluateRequest{Samples: map[string]conductor.SampleAccount{"first": {WeeklyUsed: &used, Availability: "ready"}}})
	if d.Selected != nil {
		t.Fatal("overdrive-only launched at used target")
	}
	c.Subscriptions[0].OverdriveWindows[0].Enabled = false
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	options, e := s.OverdriveOptions(context.Background(), "rule")
	if e != nil {
		t.Fatal(e)
	}
	if len(options) != 0 {
		t.Fatalf("disabled window offered: %+v", options)
	}
	// Unsaved form changes cannot invent an enabled subscription window.
	c.Subscriptions[0].OverdriveWindows[0].Enabled = true
	used = 56
	d = decisionHTTP(t, h, conductor.EvaluateRequest{Draft: &c, Samples: map[string]conductor.SampleAccount{"first": {WeeklyUsed: &used, Availability: "ready"}}})
	if d.Selected != nil {
		t.Fatal("draft enabled saved-disabled window")
	}
}
