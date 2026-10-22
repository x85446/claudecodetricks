package conductor

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

// failingSource proves the preview never reads projects, plans or gates.
type failingSource struct{}

func (failingSource) List(context.Context, model.Project) ([]Plan, error) {
	return nil, errors.New("preview must not list plans")
}
func (failingSource) Pair(context.Context, model.Project, string) (Pair, error) {
	return Pair{}, errors.New("preview must not read pairs")
}
func (failingSource) Checkpoint(context.Context, model.Assignment, model.Result) error {
	return errors.New("preview must not checkpoint")
}
func (failingSource) Gate(context.Context, model.Project, time.Time) error {
	return errors.New("preview must not read gates")
}
func (failingSource) Outcome(context.Context, model.Assignment, model.Result) error {
	return errors.New("preview must not record outcomes")
}

func previewService(t *testing.T) (*Service, time.Time) {
	t.Helper()
	s := NewService(filepath.Join(t.TempDir(), "store"), failingSource{})
	now := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC) // Monday
	s.Clock = func() time.Time { return now }
	sub := func(id, name string, seqs ...string) model.Subscription {
		x := model.Subscription{ID: id, Name: name, Adapter: "codex", ResultTransport: "prompt_file", Executable: "missing-" + id + "-executable", PowerBandID: "universal", Concurrency: 1, FreshnessSeconds: 300}
		for _, q := range seqs {
			x.Sequences = append(x.Sequences, model.Sequence{ID: id + "-" + q, Name: q})
		}
		return x
	}
	c := model.Config{Version: 2, PowerBands: []model.PowerBand{budget.Universal()},
		Projects:      []model.Project{{ID: "disabled", Name: "Disabled", Path: t.TempDir(), Paused: true}},
		Subscriptions: []model.Subscription{sub("codex", "Codex", "build", "fast"), sub("claude", "Claude Code", "build")},
		TimeWindows:   []model.TimeWindow{{ID: "weekdays", Name: "Weekdays", Enabled: true, StartMode: "weekly", Recurrence: model.Recurrence{Days: []int{1, 2, 3, 4, 5}, Start: "09:00", End: "17:00", Timezone: "UTC", EveryWeeks: 1}}}}
	if _, e := s.Apply(0, c); e != nil {
		t.Fatal(e)
	}
	return s, now
}

func rows(entries ...[2]string) model.Automation {
	a := model.Automation{ID: "draft", Name: "Draft", Mode: "keep_working"}
	for i, e := range entries {
		a.Entries = append(a.Entries, model.Entry{ID: "e" + string(rune('0'+i)), SequenceID: e[0], MaxBand: model.Band(e[1])})
	}
	return a
}

func preview(t *testing.T, s *Service, req AutomationPreviewRequest) AutomationPreview {
	t.Helper()
	p, e := s.PreviewAutomation(context.Background(), req)
	if e != nil {
		t.Fatal(e)
	}
	return p
}

// TESTMASTER: id=killdeer-preview-bands tier=fast parallel=yes
func TestKilldeerPreviewBands(t *testing.T) {
	s, _ := previewService(t)
	yellow := rows([2]string{"codex-build", "yellow"})
	for band, want := range map[model.Band]string{model.Green: "launch", model.Yellow: "launch", model.Red: "wait"} {
		p := preview(t, s, AutomationPreviewRequest{Automation: yellow, Band: band})
		if p.Outcome != want {
			t.Fatalf("Green + Yellow with %s sample: %s (%s), want %s", band, p.Outcome, p.Reason, want)
		}
		if want == "launch" && (p.Selected == nil || p.Selected.Label != "Codex · build" || p.Selected.Reason != "Green + Yellow permits "+BandTitle(band)) {
			t.Fatalf("selected row = %+v", p.Selected)
		}
	}
	p := preview(t, s, AutomationPreviewRequest{Automation: yellow, Band: model.Red})
	if p.Reason != "Waits: no row permits Red; the highest row permits Green + Yellow" || p.Maximum != model.Yellow {
		t.Fatalf("red reason %q maximum %q", p.Reason, p.Maximum)
	}
	if len(p.Bands) != 3 || p.Bands[0].Selected != 0 || p.Bands[1].Selected != 0 || p.Bands[2].Selected != -1 {
		t.Fatalf("band summary %+v", p.Bands)
	}
	green := rows([2]string{"codex-build", "green"})
	if p := preview(t, s, AutomationPreviewRequest{Automation: green, Band: model.Yellow}); p.Outcome != "wait" || p.Rows[0].Reason != "Green only does not permit Yellow" {
		t.Fatalf("green-only yellow sample: %+v", p)
	}
}

// TESTMASTER: id=killdeer-preview-first-eligible tier=fast parallel=yes
func TestKilldeerPreviewFirstEligibleRow(t *testing.T) {
	s, _ := previewService(t)
	a := rows([2]string{"claude-build", "green"}, [2]string{"codex-fast", "yellow"}, [2]string{"codex-build", "red"})
	cases := map[model.Band]int{model.Green: 0, model.Yellow: 1, model.Red: 2}
	for band, want := range cases {
		p := preview(t, s, AutomationPreviewRequest{Automation: a, Band: band})
		if p.Selected == nil || p.Selected.Index != want {
			t.Fatalf("%s sample selected %+v, want row %d", band, p.Selected, want+1)
		}
		selected := 0
		for _, r := range p.Rows {
			if r.Selected {
				selected++
			}
			if r.Index > want && r.Eligible && r.Code != "secondary" {
				t.Fatalf("later eligible row %d is %s", r.Index+1, r.Code)
			}
		}
		if selected != 1 {
			t.Fatalf("%s sample selected %d rows", band, selected)
		}
	}
	reordered := rows([2]string{"codex-build", "red"}, [2]string{"claude-build", "green"})
	if p := preview(t, s, AutomationPreviewRequest{Automation: reordered, Band: model.Green}); p.Selected.Index != 0 || p.Rows[1].Code != "secondary" {
		t.Fatalf("order not preserved: %+v", p.Rows)
	}
}

// TESTMASTER: id=killdeer-preview-isolation tier=fast parallel=yes
func TestKilldeerPreviewIgnoresLiveState(t *testing.T) {
	s, now := previewService(t)
	// Live state that admission would deny on: paused project, failing plan source,
	// disabled Conductor and subscriptions, missing executables, no bound account,
	// stale and exhausted usage, an active lease and a parked plan.
	stale := model.Usage{SubscriptionID: "codex", Source: "real", ObservedAt: now.Add(-48 * time.Hour), Supported: true, HardBlocked: true, Weekly: &model.Allowance{UsedPercent: 100, ResetsAt: now.Add(-time.Hour)}}
	r, _ := s.Runtime()
	r.Usage["codex"] = stale
	r.Leases = map[string]Lease{"busy": {Assignment: model.Assignment{ProjectID: "disabled", ProjectPath: "/elsewhere", Plan: "running"}}}
	r.Progress = map[string]Progress{"parked": {Parked: true, Reason: "operator"}}
	if e := s.saveRuntime(&r); e != nil {
		t.Fatal(e)
	}
	before := snapshot(t, s.Dir)
	a := rows([2]string{"codex-build", "yellow"})
	for _, band := range []model.Band{model.Green, model.Yellow} {
		if p := preview(t, s, AutomationPreviewRequest{Automation: a, Band: band}); p.Outcome != "launch" {
			t.Fatalf("live state changed the %s sample: %s", band, p.Reason)
		}
	}
	if after := snapshot(t, s.Dir); !bytes.Equal(before, after) {
		t.Fatal("preview changed stored configuration or runtime")
	}
}

// TESTMASTER: id=killdeer-preview-schedule tier=fast parallel=yes
func TestKilldeerPreviewScheduleAndOverdrive(t *testing.T) {
	s, now := previewService(t)
	a := rows([2]string{"codex-build", "green"})
	a.Mode, a.WindowID = "scheduled", "weekdays"
	if p := preview(t, s, AutomationPreviewRequest{Automation: a, Band: model.Green, Now: &now}); p.Outcome != "launch" {
		t.Fatalf("inside window: %s", p.Reason)
	}
	sunday := now.Add(6 * 24 * time.Hour)
	if p := preview(t, s, AutomationPreviewRequest{Automation: a, Band: model.Green, Now: &sunday}); p.Outcome != "wait" || p.Reason != "Waits: Outside the automation time window at the sample time" {
		t.Fatalf("outside window: %+v", p)
	}
	draftWindow := model.TimeWindow{ID: "draft-window", Name: "Every day", Enabled: true, StartMode: "weekly", Recurrence: model.Recurrence{Days: []int{1, 2, 3, 4, 5, 6, 7}, Start: "00:00", End: "23:59", AllDay: true, Timezone: "UTC", EveryWeeks: 1}}
	a.WindowID = draftWindow.ID
	if p := preview(t, s, AutomationPreviewRequest{Automation: a, Window: &draftWindow, Band: model.Green, Now: &sunday}); p.Outcome != "launch" {
		t.Fatalf("unsaved window ignored: %s", p.Reason)
	}
	only := rows([2]string{"codex-build", "green"})
	only.HonorOverdrive, only.OnlyOverdrive = true, true
	if p := preview(t, s, AutomationPreviewRequest{Automation: only, Band: model.Green}); p.Outcome != "wait" || p.Rows[0].Code != "outside_overdrive" {
		t.Fatalf("only-overdrive without overdrive: %+v", p)
	}
	if p := preview(t, s, AutomationPreviewRequest{Automation: only, Band: model.Red, OverdriveActive: true}); p.Outcome != "launch" || p.Selected.Code != "overdrive" {
		t.Fatalf("honored overdrive: %+v", p)
	}
	if p := preview(t, s, AutomationPreviewRequest{Automation: only, Band: model.Green, OverdriveActive: true}); p.Selected == nil || p.Selected.Code != "permitted" {
		t.Fatalf("band within the row needs no overdrive bypass: %+v", p.Selected)
	}
	if _, e := s.PreviewAutomation(context.Background(), AutomationPreviewRequest{Automation: only, Band: "purple"}); e == nil {
		t.Fatal("unknown band accepted")
	}
	if p := preview(t, s, AutomationPreviewRequest{Automation: rows(), Band: model.Green}); p.Outcome != "wait" || len(p.Rows) != 0 {
		t.Fatalf("empty rows: %+v", p)
	}
	missing := rows([2]string{"deleted-sequence", "red"})
	if p := preview(t, s, AutomationPreviewRequest{Automation: missing, Band: model.Green}); p.Rows[0].Code != "sequence_unavailable" {
		t.Fatalf("missing sequence: %+v", p.Rows[0])
	}
}

// TESTMASTER: id=killdeer-preview-overdrive tier=fast parallel=yes
func TestKilldeerPreviewOverdriveParticipants(t *testing.T) {
	s, now := previewService(t)
	c, _ := s.Config()
	c.Subscriptions[0].OverdriveEnabled = true
	c.Subscriptions[0].OverdriveWindows = []model.OverdriveWindow{{ID: "always", Name: "Always", Enabled: true, Revision: 1, StartMode: "weekly", EndMode: "recurrence", TargetUsedPercent: 80, Recurrence: model.Recurrence{Days: []int{1, 2, 3, 4, 5, 6, 7}, Start: "00:00", End: "23:59", Timezone: "UTC", EveryWeeks: 1}}}
	if _, e := s.Apply(c.Revision, c); e != nil {
		t.Fatal(e)
	}
	a := model.Automation{ID: "od", Name: "Overdrive", Mode: "overdrive", Participants: []string{"claude-build", "codex-build"}, HonorOverdrive: true, OnlyOverdrive: true}
	// Live usage without a short window, from an old interval, must not change the sample.
	r, _ := s.Runtime()
	r.Usage["codex"] = model.Usage{SubscriptionID: "codex", Source: "real", ObservedAt: now.Add(-72 * time.Hour), Supported: true, HardBlocked: true, Weekly: &model.Allowance{UsedPercent: 99, StartsAt: now.Add(-10 * 24 * time.Hour), ResetsAt: now.Add(-3 * 24 * time.Hour), ResetTrusted: true}, Models: map[string]model.Allowance{"gpt": {UsedPercent: 100, ResetsAt: now.Add(time.Hour)}}}
	if e := s.saveRuntime(&r); e != nil {
		t.Fatal(e)
	}
	used := 40.0
	p := preview(t, s, AutomationPreviewRequest{Automation: a, Now: &now, Samples: map[string]SampleAccount{"codex": {WeeklyUsed: &used}}})
	if p.Outcome != "launch" || p.Selected.Label != "Codex · build" || p.Selected.SampleBudget == nil {
		t.Fatalf("overdrive participant: %+v", p)
	}
	used = 90
	if p := preview(t, s, AutomationPreviewRequest{Automation: a, Now: &now, Samples: map[string]SampleAccount{"codex": {WeeklyUsed: &used}}}); p.Outcome != "wait" {
		t.Fatalf("target reached still launches: %+v", p)
	}
}

func snapshot(t *testing.T, dir string) []byte {
	t.Helper()
	var out []byte
	for _, name := range []string{"config.json", "runtime.json"} {
		b, e := os.ReadFile(filepath.Join(dir, name))
		if e != nil && !os.IsNotExist(e) {
			t.Fatal(e)
		}
		out = append(append(out, name...), b...)
	}
	return out
}
