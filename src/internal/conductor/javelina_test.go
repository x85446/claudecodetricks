package conductor

import (
	"context"
	"encoding/json"
	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

// TESTMASTER: id=javelina-shared-automation-migration tier=? parallel=yes
func TestJavelinaMigration(t *testing.T) {
	s, _, c := fixture(t, "")
	legacy := c
	legacy.Version = 2
	legacy.Attachments = nil
	legacy.Automations[0].ProjectID = "project"
	legacy.Automations[0].Priority = -2
	legacy.Automations[0].Enabled = false
	custom := model.TimeWindow{ID: "custom", Name: "Custom", Enabled: true, Recurrence: model.Recurrence{Days: []int{1}, Start: "22:00", End: "06:00", Timezone: "UTC", EveryWeeks: 1}}
	legacy.TimeWindows = []model.TimeWindow{custom}
	b, _ := json.Marshal(legacy)
	if e := os.WriteFile(filepath.Join(s.Dir, "config.json"), b, 0600); e != nil {
		t.Fatal(e)
	}
	got, e := s.Config()
	if e != nil {
		t.Fatal(e)
	}
	if len(got.Attachments) != 1 || got.Attachments[0].Enabled || got.Attachments[0].Priority != -2 || got.Attachments[0].ID != "automation" || got.Automations[0].ProjectID != "" {
		t.Fatalf("migration: %+v", got)
	}
	if len(got.TimeWindows) != 3 || !reflect.DeepEqual(got.TimeWindows[0], custom) {
		t.Fatal("custom/default windows", got.TimeWindows)
	}
	got, e = s.Apply(got.Revision, got)
	if e != nil {
		t.Fatal(e)
	}
	again, e := s.Config()
	if e != nil || !reflect.DeepEqual(got, again) {
		t.Fatal("non-idempotent migration", e)
	}
	for _, p := range got.PowerBands {
		if len(p.ShortGreenCurve) != 6 || p.ShortGreenCurve[5] != p.ShortGreen {
			t.Fatal("legacy curve changed", p)
		}
	}
	stale := again
	stale.Attachments[0].Enabled = true
	if _, e = s.Apply(again.Revision-1, stale); e != ErrConflict {
		t.Fatal("stale write", e)
	}
	bad := again
	bad.Automations = nil
	if _, e = s.Apply(again.Revision, bad); e == nil {
		t.Fatal("referenced deletion allowed")
	}
	bad = again
	bad.Attachments = append(append([]model.AutomationAttachment{}, again.Attachments...), again.Attachments[0])
	bad.Attachments[1].ID = "duplicate"
	if _, e = s.Apply(again.Revision, bad); e == nil {
		t.Fatal("duplicate attachment allowed")
	}
	bad = again
	bad.Attachments[0].DefinitionID = "missing"
	if _, e = s.Apply(again.Revision, bad); e == nil {
		t.Fatal("dangling attachment allowed")
	}
}

// TESTMASTER: id=javelina-shared-automation-isolation tier=? parallel=yes
func TestJavelinaSharedExecution(t *testing.T) {
	s, src, c := fixture(t, "")
	c.Projects = append(c.Projects, model.Project{ID: "other", Name: "Other", Path: t.TempDir(), Enabled: true})
	c.Attachments = append(c.Attachments, model.AutomationAttachment{ID: "other-attachment", DefinitionID: "automation", ProjectID: "other", Enabled: true})
	c.TimeWindows = []model.TimeWindow{{ID: "window", Name: "Window", Enabled: true, Recurrence: model.Recurrence{Days: []int{5}, Start: "11:00", End: "13:00", Timezone: "UTC", EveryWeeks: 1}}}
	c.Automations[0].Mode = "scheduled"
	c.Automations[0].WindowID = "window"
	c.Automations[0].RunLimit = "one_plan"
	var e error
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	first, e := s.TickProject(context.Background(), "project")
	if e != nil || first.Assignment == nil {
		t.Fatal(first, e)
	}
	second, e := s.TickProject(context.Background(), "other")
	if e != nil || second.Assignment == nil {
		t.Fatal(second, e)
	}
	if first.Assignment.Automation.ID == second.Assignment.Automation.ID || first.Assignment.Step != 1 || second.Assignment.Step != 1 || len(src.done) != 2 {
		t.Fatal("shared progress or identity", first, second, src.done)
	}
	r, _ := s.Runtime()
	if len(r.Occurrences) != 2 {
		t.Fatal("shared occurrence", r.Occurrences)
	}
	c.Attachments[0].Enabled = false
	c.Automations[0].Name = "Shared edit"
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	d, e := s.Status(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	for _, a := range d.Candidates {
		if a.Project.ID == "project" && a.Eligible {
			t.Fatal("disabled attachment eligible")
		}
		if a.Project.ID == "other" && a.Automation.Name != "Shared edit" {
			t.Fatal("shared edit missing")
		}
	}
	if first.Assignment.Automation.Name == "Shared edit" {
		t.Fatal("active snapshot mutated")
	}
	c.Attachments = c.Attachments[1:]
	c, e = s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	restarted := NewService(s.Dir, src)
	restarted.Clock = s.Clock
	restarted.CollectUsage = s.CollectUsage
	d, e = restarted.Status(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	for _, a := range d.Candidates {
		if a.Project.ID == "project" {
			t.Fatal("detached candidate")
		}
	}
}

// TESTMASTER: id=javelina-full-day-windows tier=? parallel=yes
func TestJavelinaFullDay(t *testing.T) {
	loc, _ := time.LoadLocation("America/Chicago")
	for _, date := range []string{"2026-10-23", "2026-03-08", "2026-11-01"} {
		day, _ := time.ParseInLocation("2006-01-02", date, loc)
		for _, n := range []int{0, 23*3600 + 59*60, 24*3600 - 1} {
			h, m, sec := n/3600, n/60%60, n%60
			at := time.Date(day.Year(), day.Month(), day.Day(), h, m, sec, 0, loc)
			count := 0
			for _, w := range defaultTimeWindows() {
				o, on, e := budget.WindowAt(w.Recurrence, at)
				if e != nil {
					t.Fatal(e)
				}
				if on {
					count++
					if !o.End.Equal(day.AddDate(0, 0, 1)) {
						t.Fatal("wrong civil end", o)
					}
				}
			}
			if count != 1 {
				t.Fatal(date, n, count)
			}
		}
	}
}

// TESTMASTER: id=javelina-overdrive-achievement tier=? parallel=yes
func TestJavelinaAchievement(t *testing.T) {
	s, _, c := fixture(t, "")
	c.Subscriptions[0].OverdriveEnabled = true
	for _, target := range []float64{60, 100} {
		c.Subscriptions[0].OverdriveWindows = []model.OverdriveWindow{{ID: "boost", Name: "Boost", Enabled: true, StartMode: "weekly", EndMode: "recurrence", TargetUsedPercent: target, Recurrence: model.Recurrence{Days: []int{5}, Start: "00:00", End: "00:00", Timezone: "UTC", EveryWeeks: 1}}}
		var e error
		c, e = s.Apply(c.Revision, c)
		if e != nil {
			t.Fatal(e)
		}
		for _, used := range []float64{target - 1, target, 100, target - 1} {
			out, e := s.BudgetPreview(context.Background(), BudgetPreviewRequest{SubscriptionID: "account", HonorOverdrive: true, Sample: &SampleAccount{WeeklyUsed: &used}})
			if e != nil || out.Achieved != (used >= target) || out.Target != target {
				t.Fatal(target, used, out, e)
			}
			if used == 100 && out.Allowed {
				t.Fatal("exhaustion admitted")
			}
		}
	}
	c.Subscriptions[0].OverdriveWindows[0].Enabled = false
	if _, e := s.Apply(c.Revision, c); e != nil {
		t.Fatal(e)
	}
	used := 100.0
	out, e := s.BudgetPreview(context.Background(), BudgetPreviewRequest{SubscriptionID: "account", HonorOverdrive: true, Sample: &SampleAccount{WeeklyUsed: &used}})
	if e != nil || out.Achieved {
		t.Fatal("disabled window achieved", out, e)
	}
	c, _ = s.Config()
	c.Subscriptions[0].OverdriveWindows[0].Enabled = true
	shared := model.Automation{ID: "unattached-rules", Name: "Reusable boost", Mode: "overdrive", Participants: []string{"sequence"}, HonorOverdrive: true, OnlyOverdrive: true}
	c.Automations = append(c.Automations, shared)
	if _, e = s.Apply(c.Revision, c); e != nil {
		t.Fatal(e)
	}
	options, e := s.OverdriveOptions(context.Background(), shared.ID)
	if e != nil || len(options) == 0 {
		t.Fatal("unattached definition has no preview options", options, e)
	}
}

// TESTMASTER: id=javelina-hourly-persistence tier=? parallel=yes
func TestJavelinaHourlyPersistence(t *testing.T) {
	s, _, c := fixture(t, "")
	c.PowerBands[0].ShortGreenCurve = []float64{0, 10, 20, 30, 40, 50}
	c.PowerBands[0].ShortRedCurve = []float64{0, 20, 40, 60, 80, 100}
	saved, e := s.Apply(c.Revision, c)
	if e != nil {
		t.Fatal(e)
	}
	read, e := s.Config()
	if e != nil || !reflect.DeepEqual(saved.PowerBands, read.PowerBands) {
		t.Fatal("hourly persistence", e)
	}
	for _, invalid := range [][]float64{{0, 1}, {0, 10, 20, 15, 40, 50}, {0, 10, 20, 30, 40, 101}} {
		bad := read
		bad.PowerBands = append([]model.PowerBand{}, read.PowerBands...)
		bad.PowerBands[0].ShortGreenCurve = invalid
		if _, e = s.Apply(read.Revision, bad); e == nil {
			t.Fatal("invalid curve persisted", invalid)
		}
	}
	final, _ := s.Config()
	if final.Revision != read.Revision || !reflect.DeepEqual(final.PowerBands, read.PowerBands) {
		t.Fatal("failed write changed saved policy")
	}
}
