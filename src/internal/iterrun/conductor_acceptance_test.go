package iterrun

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

func acceptanceData(t *testing.T, h http.Handler, method, path string, body, result any) {
	t.Helper()
	w := conductorRequest(t, h, method, path, "http://localhost:8420", "localhost:8420", body)
	if w.Code != 200 {
		t.Fatalf("%s %s: %d %s", method, path, w.Code, w.Body)
	}
	var envelope struct {
		OK   bool            `json:"ok"`
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &envelope); err != nil || !envelope.OK {
		t.Fatalf("response: %s (%v)", w.Body, err)
	}
	if result != nil {
		if err := json.Unmarshal(envelope.Data, result); err != nil {
			t.Fatal(err)
		}
	}
}

func acceptanceMutation(t *testing.T, s *conductor.Service, noun, action, id, parent string, definition any) map[string]any {
	t.Helper()
	c, err := s.Config()
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	acceptanceData(t, ConductorAPI(s), "POST", noun+"/"+id, map[string]any{"expected_revision": c.Revision, "action": action, "subscription_id": parent, "definition": definition}, &result)
	after, err := NewConductorService(s.Dir).Config()
	if err != nil || after.Revision != c.Revision+1 {
		t.Fatalf("durable revision: %d -> %d (%v)", c.Revision, after.Revision, err)
	}
	return result
}

func acceptanceRecurrence() model.Recurrence {
	return model.Recurrence{Days: []int{1, 2, 3, 4, 5}, Start: "22:00", End: "06:00", Timezone: "America/Chicago", EveryWeeks: 1}
}

// TESTMASTER: id=indigobird-api-resource-lifecycle tier=fast parallel=yes
func TestConductorAPIResourceLifecycle(t *testing.T) {
	s := NewConductorService(t.TempDir())
	c, _ := s.Config()
	policy := c.PowerBands[0]
	policy.ID = "policy-test"
	policy.Name = "Personal"
	sub := model.Subscription{ID: "sub-test", Name: "Tiger", Adapter: "codex", Executable: "codex"}
	resources := []struct {
		noun, id, parent string
		definition       any
	}{
		{"power-band", policy.ID, "", policy},
		{"time-window", "window-test", "", model.TimeWindow{ID: "window-test", Name: "Nights", Enabled: true, Recurrence: acceptanceRecurrence()}},
		{"subscription", sub.ID, "", sub},
		{"preset", "preset-test", sub.ID, model.Preset{ID: "preset-test", Name: "Custom", Args: []string{"--add-dir", "a b", "--add-dir", "$(touch never)"}}},
		{"sequence", "sequence-test", sub.ID, model.Sequence{ID: "sequence-test", Name: "build", Model: "custom-model", Reasoning: "high", PresetID: "preset-test", Args: []string{"--config", "key=\"a b\""}}},
		{"overdrive", "overdrive-test", sub.ID, model.OverdriveWindow{ID: "overdrive-test", Name: "Thursday", Enabled: true, Recurrence: acceptanceRecurrence(), EndMode: "explicit", EndDay: 6, EndTime: "12:00", TargetUsedPercent: 60}},
		{"project", "project-test", "", model.Project{ID: "project-test", Name: "fixture", Path: t.TempDir(), Enabled: true}},
		{"automation", "automation-test", "", model.Automation{ID: "automation-test", Name: "Work", ProjectID: "project-test", Mode: "scheduled", WindowID: "window-test", Entries: []model.Entry{{ID: "entry-test", SequenceID: "sequence-test", MaxBand: model.Yellow}}}},
	}
	for _, r := range resources {
		t.Run(r.noun, func(t *testing.T) {
			acceptanceMutation(t, s, r.noun, "create", "", r.parent, r.definition)
			path := r.noun + "/" + r.id + "?subscription_id=" + r.parent
			var got map[string]any
			acceptanceData(t, ConductorAPI(NewConductorService(s.Dir)), "GET", path, nil, &got)
			if got["id"] != r.id {
				t.Fatal(got)
			}
			if r.noun == "project" && got["enabled"] != false {
				t.Fatal("registration armed project")
			}
			if r.noun == "subscription" && got["power_band_id"] != "universal" {
				t.Fatal("missing universal default", got)
			}
			acceptanceMutation(t, s, r.noun, "edit", r.id, r.parent, map[string]any{"id": "must-not-replace-id", "name": "Edited " + r.noun})
			patch := map[string]any{"name": "Copy " + r.noun}
			if r.noun == "project" {
				patch["path"] = t.TempDir()
			}
			copy := acceptanceMutation(t, s, r.noun, "duplicate", r.id, r.parent, patch)
			copyID, _ := copy["id"].(string)
			if copyID == "" || copyID == r.id {
				t.Fatal("duplicate reused ID", copy)
			}
			acceptanceMutation(t, s, r.noun, "delete", copyID, r.parent, nil)
			acceptanceData(t, ConductorAPI(s), "GET", path, nil, &got)
			if got["id"] != r.id || got["name"] != "Edited "+r.noun {
				t.Fatal("rename changed identity", got)
			}
		})
	}
	// Duplicating a populated subscription remaps children and their local preset reference.
	copy := acceptanceMutation(t, s, "subscription", "duplicate", sub.ID, "", nil)
	copyID := copy["id"].(string)
	fresh, _ := NewConductorService(s.Dir).Config()
	for _, v := range fresh.Subscriptions {
		if v.ID == copyID {
			if v.Enabled || len(v.Sequences) != 1 || len(v.Presets) != 1 || len(v.OverdriveWindows) != 1 {
				t.Fatal("incomplete duplicate", v)
			}
			if v.Sequences[0].ID == "sequence-test" || v.Presets[0].ID == "preset-test" || v.OverdriveWindows[0].ID == "overdrive-test" || v.Sequences[0].PresetID != v.Presets[0].ID {
				t.Fatal("duplicate identity/reference collision", v)
			}
			if !reflect.DeepEqual(v.Presets[0].Args, []string{"--add-dir", "a b", "--add-dir", "$(touch never)"}) {
				t.Fatal("literal argv changed", v.Presets)
			}
		}
	}
	acceptanceMutation(t, s, "subscription", "delete", copyID, "", nil)
	for i := len(resources) - 1; i >= 0; i-- {
		r := resources[i]
		acceptanceMutation(t, s, r.noun, "delete", r.id, r.parent, nil)
	}
}

// TESTMASTER: id=indigobird-api-reference-integrity tier=fast parallel=yes
func TestConductorAPIReferenceIntegrity(t *testing.T) {
	s := NewConductorService(t.TempDir())
	c, _ := s.Config()
	p := c.PowerBands[0]
	p.ID = "policy-ref"
	p.Name = "Referenced"
	c.PowerBands = append(c.PowerBands, p)
	c.Subscriptions[0].PowerBandID = p.ID
	q := c.Subscriptions[0].Sequences[0]
	sub := c.Subscriptions[0]
	if q.PresetID == "" {
		t.Fatal("fixture requires shipped preset")
	}
	c.TimeWindows = []model.TimeWindow{{ID: "window-ref", Name: "Nights", Enabled: true, Recurrence: acceptanceRecurrence()}}
	c.Projects = []model.Project{{ID: "project-ref", Name: "fixture", Path: t.TempDir(), WindowID: "window-ref"}}
	c.Automations = []model.Automation{{ID: "automation-ref", Name: "Work", ProjectID: "project-ref", Mode: "scheduled", WindowID: "window-ref", Entries: []model.Entry{{ID: "entry-ref", SequenceID: q.ID, MaxBand: model.Green}}}}
	c, err := s.Apply(0, c)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	for _, r := range []struct{ noun, id, parent string }{{"subscription", sub.ID, ""}, {"sequence", q.ID, sub.ID}, {"preset", q.PresetID, sub.ID}, {"power-band", p.ID, ""}, {"power-band", "universal", ""}, {"time-window", "window-ref", ""}, {"project", "project-ref", ""}} {
		w := conductorRequest(t, ConductorAPI(s), "POST", r.noun+"/"+r.id, "http://localhost:8420", "localhost:8420", map[string]any{"expected_revision": c.Revision, "action": "delete", "subscription_id": r.parent})
		if w.Code == 200 {
			t.Fatalf("deleted referenced %s %s", r.noun, r.id)
		}
		after, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
		if !bytes.Equal(before, after) {
			t.Fatal("failed deletion wrote partial config")
		}
	}
	// A single transaction can reassign references and remove their former definitions.
	c.Subscriptions[0].PowerBandID = "universal"
	c.PowerBands = c.PowerBands[:1]
	c.Automations = nil
	c.Projects = nil
	c.TimeWindows = nil
	acceptanceData(t, ConductorAPI(s), "POST", "config", map[string]any{"expected_revision": c.Revision, "config": c}, nil)
}

// TESTMASTER: id=indigobird-api-window-controls-persistence tier=fast parallel=yes
func TestConductorAPIWindowControlsPersistence(t *testing.T) {
	s := NewConductorService(t.TempDir())
	c, _ := s.Config()
	c.Projects = []model.Project{{ID: "p", Name: "fixture", Path: t.TempDir()}}
	c.TimeWindows = []model.TimeWindow{{ID: "w", Name: "Nights", Enabled: true, Recurrence: acceptanceRecurrence()}}
	c.Automations = []model.Automation{{ID: "a", Name: "Work", ProjectID: "p", Mode: "scheduled", WindowID: "w", Entries: []model.Entry{{ID: "e", SequenceID: c.Subscriptions[0].Sequences[0].ID, MaxBand: model.Green}}}}
	c, e := s.Apply(0, c)
	if e != nil {
		t.Fatal(e)
	}
	if c.Automations[0].RunLimit != "drain_queue" || c.Automations[0].OnWindowEnd != "finish_checkpoint" {
		t.Fatal("wrong default controls", c.Automations[0])
	}
	for _, limit := range []string{"drain_queue", "one_plan"} {
		for _, end := range []string{"finish_checkpoint", "finish_plan", "interrupt"} {
			acceptanceMutation(t, s, "automation", "edit", "a", "", map[string]any{"run_limit": limit, "on_window_end": end})
			var got model.Automation
			acceptanceData(t, ConductorAPI(NewConductorService(s.Dir)), "GET", "automation/a", nil, &got)
			if got.RunLimit != limit || got.OnWindowEnd != end {
				t.Fatal("restart lost schedule policy", got)
			}
		}
	}
	c, _ = s.Config()
	before, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	for _, patch := range []map[string]any{{"run_limit": "forever"}, {"on_window_end": "kill-everything"}, {"mode": "scheduled", "window_id": ""}, {"only_during_overdrive": true, "honor_engine_overdrive": false}} {
		w := conductorRequest(t, ConductorAPI(s), "POST", "automation/a", "http://localhost:8420", "localhost:8420", map[string]any{"expected_revision": c.Revision, "action": "edit", "definition": patch})
		if w.Code == 200 {
			t.Fatal("invalid policy accepted", patch)
		}
	}
	acceptanceData(t, ConductorAPI(s), "POST", "automation/a", map[string]any{"expected_revision": c.Revision, "action": "edit", "dry_run": true, "definition": map[string]any{"run_limit": "drain_queue"}}, nil)
	after, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("invalid or draft save mutated config")
	}
}

func acceptanceSimulation(t *testing.T) (*conductor.Service, model.Config, time.Time) {
	t.Helper()
	s := NewConductorService(t.TempDir())
	c, _ := s.Config()
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	s.Clock = func() time.Time { return now }
	c.Enabled = true
	c.Projects = []model.Project{{ID: "p", Name: "fixture", Path: t.TempDir(), Enabled: true}}
	for i := range c.Subscriptions {
		c.Subscriptions[i].Enabled = false
	}
	for i := 0; i < 2; i++ {
		v := &c.Subscriptions[i]
		v.Enabled = true
		v.Executable = "/usr/bin/true"
		v.AccountID = v.ID + "-identity"
		v.PoolID = v.ID + "-pool"
	}
	c.Automations = []model.Automation{{ID: "a", Name: "Work", ProjectID: "p", Enabled: true, Mode: "keep_working", Entries: []model.Entry{{ID: "first", SequenceID: c.Subscriptions[0].Sequences[0].ID, MaxBand: model.Green}, {ID: "second", SequenceID: c.Subscriptions[1].Sequences[0].ID, MaxBand: model.Yellow}, {ID: "third", SequenceID: c.Subscriptions[0].Sequences[0].ID, MaxBand: model.Red}}}}
	c, e := s.Apply(0, c)
	if e != nil {
		t.Fatal(e)
	}
	return s, c, now
}

// TESTMASTER: id=indigobird-api-cumulative-draft-samples tier=fast parallel=yes
func TestConductorAPICumulativeDraftSamples(t *testing.T) {
	s, c, now := acceptanceSimulation(t)
	h := ConductorAPI(s)
	yes := true
	before, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	rtBefore, _ := s.Runtime()
	for _, max := range []model.Band{model.Green, model.Yellow, model.Red} {
		for _, current := range []model.Band{model.Green, model.Yellow, model.Red} {
			draft := c
			draft.Automations = append([]model.Automation(nil), c.Automations...)
			draft.Automations[0].Entries = []model.Entry{{ID: "sample-row", SequenceID: c.Subscriptions[0].Sequences[0].ID, MaxBand: max}}
			var got conductor.Decision
			acceptanceData(t, h, "POST", "evaluate", conductor.EvaluateRequest{ProjectID: "p", AutomationID: "a", Draft: &draft, Now: &now, QueueAvailable: &yes, Samples: map[string]conductor.SampleAccount{c.Subscriptions[0].ID: {Band: current, Availability: "ready"}}}, &got)
			rank := map[model.Band]int{model.Green: 0, model.Yellow: 1, model.Red: 2}
			want := rank[current] <= rank[max]
			if (got.Selected != nil) != want {
				t.Fatalf("current=%s max=%s want=%v decision=%+v", current, max, want, got)
			}
		}
	}
	for _, tc := range []struct{ availability, winner string }{{"ready", "second"}, {"stale", "third"}, {"blocked", "third"}, {"exhausted", "third"}} {
		var got conductor.Decision
		acceptanceData(t, h, "POST", "evaluate", conductor.EvaluateRequest{ProjectID: "p", AutomationID: "a", Now: &now, QueueAvailable: &yes, Samples: map[string]conductor.SampleAccount{c.Subscriptions[0].ID: {Band: model.Red, Availability: "ready"}, c.Subscriptions[1].ID: {Band: model.Green, Availability: tc.availability}}}, &got)
		if got.Selected == nil || got.Selected.EntryID != tc.winner {
			t.Fatalf("%s: %+v", tc.availability, got)
		}
		if len(got.Candidates) != 3 {
			t.Fatal("repeated sequence rows collapsed", got.Candidates)
		}
	}
	after, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	rtAfter, _ := s.Runtime()
	if !bytes.Equal(before, after) || !reflect.DeepEqual(rtBefore, rtAfter) {
		t.Fatal("samples mutated saved configuration or runtime")
	}
}

// TESTMASTER: id=indigobird-api-overdrive-window-samples tier=fast parallel=yes
func TestConductorAPIOverdriveWindowSamples(t *testing.T) {
	s, c, now := acceptanceSimulation(t)
	for i := 0; i < 2; i++ {
		v := &c.Subscriptions[i]
		v.OverdriveEnabled = true
		zone := "UTC"
		start, end := "10:00", "18:00"
		if i == 1 {
			zone = "America/Chicago"
			start = "05:00"
			end = "13:00"
		}
		v.OverdriveWindows = []model.OverdriveWindow{
			{ID: v.ID + "-window", Name: "Available", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: start, End: end, Timezone: zone, EveryWeeks: 1}, EndMode: "explicit", EndDay: 5, EndTime: end, TargetUsedPercent: 60},
			{ID: v.ID + "-disabled", Name: "Disabled higher target", Enabled: false, Recurrence: model.Recurrence{Days: []int{5}, Start: start, End: end, Timezone: zone, EveryWeeks: 1}, EndMode: "explicit", EndDay: 5, EndTime: end, TargetUsedPercent: 100},
		}
	}
	a := &c.Automations[0]
	a.Mode = "overdrive"
	a.Entries = nil
	a.Participants = []string{c.Subscriptions[0].Sequences[0].ID, c.Subscriptions[1].Sequences[0].ID}
	a.HonorOverdrive = true
	a.OnlyOverdrive = true
	c, err := s.Apply(c.Revision, c)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		v := c.Subscriptions[i]
		err = s.PutUsage(model.Usage{SubscriptionID: v.ID, PoolID: v.PoolID, AccountID: v.AccountID, Source: "isolated fixture", Supported: true, ObservedAt: now, Weekly: &model.Allowance{UsedPercent: 50, StartsAt: now.Add(-4 * 24 * time.Hour), ResetsAt: now.Add(time.Duration(i+1) * 24 * time.Hour), ResetTrusted: true, Epoch: v.ID + "-epoch"}, Short: &model.Allowance{UsedPercent: 0, StartsAt: now.Add(-time.Hour), ResetsAt: now.Add(4 * time.Hour), ResetTrusted: true, Epoch: "short"}})
		if err != nil {
			t.Fatal(err)
		}
	}
	h := ConductorAPI(s)
	var options struct {
		Options          []conductor.OverdriveOption `json:"options"`
		OutsideTime      time.Time                   `json:"outside_time"`
		OutsideAvailable bool                        `json:"outside_available"`
	}
	acceptanceData(t, h, "POST", "overdrive-options", map[string]any{"automation": c.Automations[0]}, &options)
	if len(options.Options) != 2 || !options.OutsideAvailable {
		t.Fatalf("available options: %+v", options)
	}
	for _, o := range options.Options {
		if strings.Contains(o.WindowID, "disabled") || !strings.Contains(o.Label, "build") || o.TargetUsedPercent != 60 {
			t.Fatal(o)
		}
	}
	before, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	rtBefore, _ := s.Runtime()
	yes := true
	shared := options.Options[0].SampleTime
	for _, used := range []float64{59, 60, 61, 100} {
		samples := map[string]conductor.SampleAccount{}
		for i := 0; i < 2; i++ {
			samples[c.Subscriptions[i].ID] = conductor.SampleAccount{WeeklyUsed: &used, Availability: "ready"}
		}
		var got conductor.Decision
		acceptanceData(t, h, "POST", "evaluate", conductor.EvaluateRequest{ProjectID: "p", AutomationID: "a", Now: &shared, QueueAvailable: &yes, Samples: samples}, &got)
		if got.At != shared || (got.Selected != nil) != (used < 60) {
			t.Fatalf("used %v: %+v", used, got)
		}
		if used < 60 {
			if len(got.Candidates) != 2 || got.Selected.Subscription.ID != c.Subscriptions[0].ID {
				t.Fatal("common instant / reset ordering", got)
			}
			for _, row := range got.Candidates {
				if !row.Budget.OverdriveActive || row.Budget.TargetUsedPercent != 60 {
					t.Fatal("disabled overlap affected target", row)
				}
			}
		}
	}
	used := 50.0
	var outside conductor.Decision
	acceptanceData(t, h, "POST", "evaluate", conductor.EvaluateRequest{ProjectID: "p", AutomationID: "a", Now: &options.OutsideTime, QueueAvailable: &yes, Samples: map[string]conductor.SampleAccount{c.Subscriptions[0].ID: {WeeklyUsed: &used, Availability: "ready"}, c.Subscriptions[1].ID: {WeeklyUsed: &used, Availability: "ready"}}}, &outside)
	if outside.Selected != nil {
		t.Fatal("outside sample selected", outside)
	}
	// A draft cannot promote a saved disabled window or alter the effective target.
	raw, _ := json.Marshal(c)
	var draft model.Config
	_ = json.Unmarshal(raw, &draft)
	draft.Subscriptions[0].OverdriveWindows[1].Enabled = true
	var protected conductor.Decision
	acceptanceData(t, h, "POST", "evaluate", conductor.EvaluateRequest{ProjectID: "p", AutomationID: "a", Draft: &draft, Now: &shared, QueueAvailable: &yes, Samples: map[string]conductor.SampleAccount{c.Subscriptions[0].ID: {WeeklyUsed: &used, Availability: "ready"}, c.Subscriptions[1].ID: {WeeklyUsed: &used, Availability: "ready"}}}, &protected)
	if protected.Selected == nil || protected.Selected.Budget.TargetUsedPercent != 60 {
		t.Fatal("draft promoted disabled window", protected)
	}
	after, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	rtAfter, _ := s.Runtime()
	if !bytes.Equal(before, after) || !reflect.DeepEqual(rtBefore, rtAfter) {
		t.Fatal("overdrive examples mutated saved state")
	}
	// Saved master and account switches determine the available dropdown inventory.
	c.Subscriptions[0].OverdriveEnabled = false
	c.Subscriptions[1].Enabled = false
	_, err = s.Apply(c.Revision, c)
	if err != nil {
		t.Fatal(err)
	}
	acceptanceData(t, h, "POST", "overdrive-options", map[string]any{"automation": c.Automations[0]}, &options)
	if len(options.Options) != 0 {
		t.Fatal("disabled subscriptions exposed windows", options)
	}
}

// TESTMASTER: id=indigobird-api-invalid-folders-readiness tier=fast parallel=yes
func TestConductorAPIInvalidFoldersAndReadiness(t *testing.T) {
	s := NewConductorService(t.TempDir())
	h := ConductorAPI(s)
	c, _ := s.Config()
	acceptanceData(t, h, "POST", "config", map[string]any{"expected_revision": c.Revision, "config": c}, nil)
	c, _ = s.Config()
	before, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	root := t.TempDir()
	file := filepath.Join(root, "file")
	if err := os.WriteFile(file, []byte("untouched"), 0600); err != nil {
		t.Fatal(err)
	}
	denied := filepath.Join(root, "denied")
	if err := os.Mkdir(denied, 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(denied, 0); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(denied, 0700)
	paths := []string{"", filepath.Join(root, "missing"), file}
	if f, err := os.Open(denied); err != nil {
		paths = append(paths, denied)
	} else {
		f.Close()
	}
	for _, path := range paths {
		w := conductorRequest(t, h, "POST", "project", "http://localhost:8420", "localhost:8420", map[string]any{"expected_revision": c.Revision, "action": "create", "definition": map[string]any{"id": "bad", "name": "bad", "path": path}})
		var env ConductorEnvelope
		_ = json.Unmarshal(w.Body.Bytes(), &env)
		if w.Code != 400 || env.Error == nil || env.Error.Field != "project.path" || env.Error.Remedy == "" {
			t.Fatalf("invalid folder %q: %d %s", path, w.Code, w.Body)
		}
		draft := c
		draft.Projects = []model.Project{{ID: "bad", Name: "bad", Path: path}}
		w = conductorRequest(t, h, "POST", "config", "http://localhost:8420", "localhost:8420", map[string]any{"expected_revision": c.Revision, "config": draft})
		env = ConductorEnvelope{}
		_ = json.Unmarshal(w.Body.Bytes(), &env)
		if w.Code != 400 || env.Error == nil || env.Error.Field != "project.path" {
			t.Fatalf("full configuration bypass for %q: %d %s", path, w.Code, w.Body)
		}
	}
	var readiness model.Readiness
	acceptanceData(t, h, "POST", "probe", map[string]any{"subscription": model.Subscription{ID: "absent", Name: "Absent", Adapter: "codex", Executable: filepath.Join(root, "absent-cli")}}, &readiness)
	if readiness.Installed || readiness.Stream || readiness.ResultContract || readiness.Reason == "" {
		t.Fatal("missing CLI fabricated readiness", readiness)
	}
	for _, body := range []map[string]any{{"subscription_id": "missing"}, {"subscription_id": c.Subscriptions[0].ID, "sequence_id": "missing"}} {
		w := conductorRequest(t, h, "POST", "preview", "http://localhost:8420", "localhost:8420", body)
		if w.Code != 404 || !bytes.Contains(w.Body.Bytes(), []byte(`"code":"not_found"`)) {
			t.Fatal("missing reference fallback", w.Code, w.Body.String())
		}
	}
	w := conductorRequest(t, h, "POST", "preview", "http://localhost:8420", "localhost:8420", map[string]any{"subscription": model.Subscription{Adapter: "generic", Executable: "custom", Args: []string{"{prompt}", "{result}"}}, "sequence": model.Sequence{Model: "unsupported-structured-model"}})
	if w.Code != 400 || !bytes.Contains(w.Body.Bytes(), []byte("model/reasoning")) {
		t.Fatal("unsupported capability fallback", w.Code, w.Body.String())
	}
	after, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	got, _ := os.ReadFile(file)
	if !bytes.Equal(before, after) || string(got) != "untouched" {
		t.Fatal("invalid folders/probes mutated configuration or repository")
	}
}

// TESTMASTER: id=indigobird-api-field-errors-recurrence tier=fast parallel=yes
func TestConductorAPIFieldErrorsAndRecurrenceRestore(t *testing.T) {
	s := NewConductorService(t.TempDir())
	c, _ := s.Config()
	sub := c.Subscriptions[0]
	recurrence := model.Recurrence{Days: []int{2, 5, 7}, Start: "23:00", End: "05:30", Timezone: "America/Chicago", EveryWeeks: 2, AnchorDate: "2026-10-12"}
	acceptanceMutation(t, s, "time-window", "create", "", "", model.TimeWindow{ID: "cadence", Name: "Custom cadence", Enabled: true, Recurrence: recurrence})
	acceptanceMutation(t, s, "subscription", "edit", sub.ID, "", map[string]any{"tier": "Custom enterprise allowance", "typical_reset": model.ResetSchedule{Day: 7, Time: "00:30", Timezone: "America/Chicago"}})
	var window model.TimeWindow
	acceptanceData(t, ConductorAPI(NewConductorService(s.Dir)), "GET", "time-window/cadence", nil, &window)
	if !reflect.DeepEqual(window.Recurrence, recurrence) {
		t.Fatal("recurrence lost on restart", window)
	}
	c, _ = s.Config()
	if c.Subscriptions[0].Tier != "Custom enterprise allowance" || c.Subscriptions[0].TypicalReset.Time != "00:30" {
		t.Fatal("custom subscription metadata lost")
	}
	before, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	h := ConductorAPI(s)
	for _, tc := range []struct {
		path, parent, action, field string
		definition                  any
	}{
		{"time-window/cadence", "", "edit", "time_windows[cadence].recurrence", map[string]any{"recurrence": model.Recurrence{Days: nil, Start: "23:00", End: "05:30", Timezone: "UTC", EveryWeeks: 1}}},
		{"power-band/universal", "", "edit", "power_bands[universal]", map[string]any{"green_curve": []float64{0, 200}}},
		{"overdrive", sub.ID, "create", "overdrive.target_used_percent", map[string]any{"id": "bad-target", "name": "Blank target", "recurrence": recurrence}},
		{"overdrive", sub.ID, "create", "subscriptions[" + sub.ID + "].overdrive_windows[bad-target]", model.OverdriveWindow{ID: "bad-target", Name: "Above100", Recurrence: recurrence, TargetUsedPercent: 101}},
		{"overdrive", sub.ID, "create", "subscriptions[" + sub.ID + "].overdrive_windows[bad-target]", model.OverdriveWindow{ID: "bad-target", Name: "Negative", Recurrence: recurrence, TargetUsedPercent: -1}},
	} {
		w := conductorRequest(t, h, "POST", tc.path, "http://localhost:8420", "localhost:8420", map[string]any{"expected_revision": c.Revision, "action": tc.action, "subscription_id": tc.parent, "definition": tc.definition})
		var env ConductorEnvelope
		_ = json.Unmarshal(w.Body.Bytes(), &env)
		if w.Code != 400 || env.Error == nil || env.Error.Field != tc.field {
			t.Fatalf("%s field=%s: %d %s", tc.path, tc.field, w.Code, w.Body)
		}
	}
	acceptanceData(t, h, "POST", "time-window/cadence", map[string]any{"expected_revision": c.Revision, "action": "edit", "dry_run": true, "definition": map[string]any{"name": "Cancelled cadence"}}, nil)
	after, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	if !bytes.Equal(before, after) {
		t.Fatal("invalid/draft edits altered last committed definitions")
	}
}

// TESTMASTER: id=indigobird-api-cursor-expiry-recovery tier=? parallel=yes
func TestConductorAPICursorExpiryRecovery(t *testing.T) {
	s := NewConductorService(t.TempDir())
	project := t.TempDir()
	a := model.Assignment{ID: "assignment-retained", RunID: "retained-run", ProjectID: "project-retained", ProjectPath: project, Plan: "alpha", Subscription: model.Subscription{ID: "account", Name: "Fixture"}, Sequence: model.Sequence{ID: "sequence", Name: "build"}}
	lease := conductor.Lease{Assignment: a, Status: "completed"}
	raw, _ := json.Marshal(lease)
	runDir := filepath.Join(s.Dir, "runs", a.RunID)
	if err := conductor.AtomicWrite(filepath.Join(runDir, "run.json"), raw); err != nil {
		t.Fatal(err)
	}
	// A retained suffix reproduces cursor expiry without thousands of durable writes.
	var retained bytes.Buffer
	for seq := uint64(100); seq <= 104; seq++ {
		e := model.Event{Version: 1, RunID: a.RunID, AssignmentID: a.ID, ProjectPath: project, Plan: "alpha", Sequence: seq, Kind: "activity", Message: fmt.Sprintf("retained event %d", seq)}
		if err := json.NewEncoder(&retained).Encode(e); err != nil {
			t.Fatal(err)
		}
	}
	if err := conductor.AtomicWrite(filepath.Join(runDir, "events.jsonl"), retained.Bytes()); err != nil {
		t.Fatal(err)
	}
	read := func(service *conductor.Service, after uint64) conductor.EventPage {
		t.Helper()
		var page conductor.EventPage
		acceptanceData(t, ConductorAPI(service), "GET", fmt.Sprintf("events?run_id=%s&project_id=%s&plan=alpha&after=%d&limit=2", a.RunID, a.ProjectID, after), nil, &page)
		return page
	}
	first := read(s, 7)
	if !first.Expired || first.Cursor != 101 || len(first.Events) != 2 || first.Events[0].Sequence != 100 {
		t.Fatal("expired cursor did not recover retained suffix", first)
	}
	seen := map[uint64]bool{}
	page := first
	for {
		for _, event := range page.Events {
			if seen[event.Sequence] || event.RunID != a.RunID || event.ProjectPath != project {
				t.Fatal("duplicate or cross-run event", event)
			}
			seen[event.Sequence] = true
		}
		if len(page.Events) == 0 {
			break
		}
		previous := page.Cursor
		page = read(NewConductorService(s.Dir), previous)
		if page.Expired || page.Cursor < previous {
			t.Fatal("restart lost cursor", page)
		}
	}
	if len(seen) != 5 || page.Cursor != 104 {
		t.Fatal("pagination lost retained history", seen, page)
	}
	again := read(NewConductorService(s.Dir), 101)
	if again.Expired || len(again.Events) != 2 || again.Events[0].Sequence != 102 || again.Events[1].Sequence != 103 {
		t.Fatal("stable resume cursor changed", again)
	}
	for _, path := range []string{"events?run_id=retained-run&project_id=other", "events?run_id=retained-run&plan=other", "events?run_id=missing"} {
		w := conductorRequest(t, ConductorAPI(s), "GET", path, "", "localhost:8420", nil)
		if w.Code != 404 {
			t.Fatal("unscoped cursor read", w.Code, w.Body.String())
		}
	}
	after, _ := os.ReadFile(filepath.Join(runDir, "events.jsonl"))
	runAfter, _ := os.ReadFile(filepath.Join(runDir, "run.json"))
	rt, _ := s.Runtime()
	if !bytes.Equal(after, retained.Bytes()) || !bytes.Equal(runAfter, raw) || rt.Revision != 0 || len(rt.Leases) != 0 {
		t.Fatal("history reads mutated durable evidence or acquired execution")
	}
}

// TESTMASTER: id=indigobird-api-visibility-conversion tier=? parallel=yes
func TestConductorAPIVisibilityAndConversion(t *testing.T) {
	s, c, now := acceptanceSimulation(t)
	sub := &c.Subscriptions[0]
	sub.OverdriveWindows = []model.OverdriveWindow{{ID: "convert", Name: "Before reset", Enabled: true, StartMode: "weekly", EndMode: "next_reset", TargetUsedPercent: 60, Recurrence: model.Recurrence{Days: []int{6}, Start: "00:00", End: "00:00", Timezone: "UTC", EveryWeeks: 1}}}
	c, err := s.Apply(c.Revision, c)
	if err != nil {
		t.Fatal(err)
	}
	reset := time.Date(2026, 10, 18, 3, 30, 0, 0, time.UTC)
	u := model.Usage{SubscriptionID: sub.ID, AccountID: sub.AccountID, PoolID: sub.PoolID, Source: "authentic fixture", Supported: true, ObservedAt: now.Add(-45 * time.Second), Weekly: &model.Allowance{StartsAt: reset.AddDate(0, 0, -7), ResetsAt: reset, ResetTrusted: true}, ShortUnsupported: true}
	if err = s.PutUsage(u); err != nil {
		t.Fatal(err)
	}
	rt, _ := s.Runtime()
	snapshotSub := *sub
	snapshotSub.Name = "Recorded account"
	snapshotSeq := sub.Sequences[0]
	snapshotSeq.Name = "Recorded sequence"
	rt.Leases["visibility-run"] = conductor.Lease{Status: "running", Assignment: model.Assignment{ID: "visibility-assignment", RunID: "visibility-run", ProjectID: "p", ProjectPath: c.Projects[0].Path, Plan: "alpha", Subscription: snapshotSub, Sequence: snapshotSeq}}
	raw, _ := json.Marshal(rt)
	if err = conductor.AtomicWrite(filepath.Join(s.Dir, "runtime.json"), raw); err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	var views []conductorVisibilityView
	acceptanceData(t, ConductorAPI(s), "GET", "visibility", nil, &views)
	if len(views) != 1 || views[0].State != "running" || views[0].EffectiveSequence != "Recorded account · Recorded sequence" || !strings.Contains(views[0].NextOpportunity, "checkpoint") || views[0].Reason != "Assignment in progress: alpha" || len(views[0].Usage) != 2 || views[0].Usage[0].AgeSeconds != 45 || views[0].Usage[0].Source != u.Source || !views[0].Usage[0].Available {
		t.Fatalf("visibility %+v", views)
	}
	if views[0].Usage[1].Available || !views[0].Usage[1].ObservedAt.IsZero() {
		t.Fatal("missing candidate telemetry fabricated", views[0].Usage[1])
	}
	acceptanceData(t, ConductorAPI(s), "GET", "visibility?project=/another-project", nil, &views)
	if len(views) != 0 {
		t.Fatal("cross project visibility", views)
	}
	var converted conductor.OverdriveConversion
	acceptanceData(t, ConductorAPI(s), "POST", "overdrive-conversion", map[string]string{"subscription_id": sub.ID, "window_id": "convert"}, &converted)
	if converted.Revision != c.Revision || converted.Window.HoursBeforeReset != 27.5 || converted.Window.ID != "convert" || converted.Window.TargetUsedPercent != 60 || !converted.End.Equal(reset) {
		t.Fatal(converted)
	}
	bad := conductorRequest(t, ConductorAPI(s), "POST", "overdrive-conversion", "http://localhost:8420", "localhost:8420", map[string]string{"subscription_id": sub.ID, "window_id": "missing"})
	if bad.Code != 400 {
		t.Fatal(bad.Code, bad.Body.String())
	}
	after, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	runtimeAfter, _ := os.ReadFile(filepath.Join(s.Dir, "runtime.json"))
	if !bytes.Equal(before, after) || !bytes.Equal(raw, runtimeAfter) {
		t.Fatal("presentation mutated state")
	}
}
