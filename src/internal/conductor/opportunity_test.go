package conductor

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

// TESTMASTER: id=indigobird-web-visibility tier=fast parallel=yes
func TestIndigobirdNextScheduleIntersection(t *testing.T) {
	s, _, c := fixture(t, "")
	c.TimeWindows = []model.TimeWindow{
		{ID: "outer", Name: "Project hours", Enabled: true, StartMode: "weekly", Recurrence: model.Recurrence{Days: []int{5}, Start: "14:00", End: "17:00", Timezone: "UTC", EveryWeeks: 1}},
		{ID: "inner", Name: "Automation hours", Enabled: true, StartMode: "weekly", Recurrence: model.Recurrence{Days: []int{5}, Start: "13:00", End: "15:00", Timezone: "UTC", EveryWeeks: 1}},
	}
	c.Projects[0].WindowID = "outer"
	c.Automations[0].Mode = "scheduled"
	c.Automations[0].WindowID = "inner"
	var err error
	c, err = s.Apply(c.Revision, c)
	if err != nil {
		t.Fatal(err)
	}
	before, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	stateBefore, _ := os.ReadFile(filepath.Join(s.Dir, "runtime.json"))
	out, err := s.NextOpportunity(context.Background(), "project")
	expected := s.now().Add(2 * time.Hour)
	if err != nil || out.At == nil || !out.At.Equal(expected) || out.AutomationID != "automation" || !strings.Contains(out.Reason, "budget") {
		t.Fatalf("intersection %+v %v", out, err)
	}
	after, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
	stateAfter, _ := os.ReadFile(filepath.Join(s.Dir, "runtime.json"))
	if string(before) != string(after) || string(stateBefore) != string(stateAfter) {
		t.Fatal("preview wrote state")
	}
	// Exclusive meeting endpoints do not constitute an opening.
	c.TimeWindows[1].Recurrence.End = "14:00"
	c, err = s.Apply(c.Revision, c)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.NextOpportunity(context.Background(), "project")
	if err != nil || out.At != nil || !strings.Contains(out.Reason, "14 days") {
		t.Fatalf("empty intersection %+v %v", out, err)
	}
	// Multiweek anchors outside the bounded horizon are reported without guessing.
	c.TimeWindows[1].Recurrence.End = "15:00"
	c.TimeWindows[1].Recurrence.EveryWeeks = 4
	c.TimeWindows[1].Recurrence.AnchorDate = "2026-11-02"
	c, err = s.Apply(c.Revision, c)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.NextOpportunity(context.Background(), "project")
	if err != nil || out.At != nil {
		t.Fatalf("outside horizon %+v %v", out, err)
	}
	// Spring-forward midnight to 04:00 uses the shared civil clock implementation.
	loc, _ := time.LoadLocation("America/Chicago")
	now := time.Date(2027, 3, 13, 23, 0, 0, 0, loc)
	s.Clock = func() time.Time { return now }
	c.TimeWindows[0].Recurrence = model.Recurrence{Days: []int{7}, Start: "02:30", End: "04:00", Timezone: loc.String(), EveryWeeks: 1}
	c.TimeWindows[1].Recurrence = model.Recurrence{Days: []int{6}, Start: "22:00", End: "03:30", Timezone: loc.String(), EveryWeeks: 1}
	c, err = s.Apply(c.Revision, c)
	if err != nil {
		t.Fatal(err)
	}
	out, err = s.NextOpportunity(context.Background(), "project")
	expected = time.Date(2027, 3, 14, 3, 0, 0, 0, loc)
	if err != nil || out.At == nil || !out.At.Equal(expected) {
		t.Fatalf("DST overnight intersection %+v %v expected %v", out, err, expected)
	}
}

// TESTMASTER: id=indigobird-web-visibility tier=fast parallel=yes
func TestIndigobirdNextResetOpportunity(t *testing.T) {
	s, _, c := fixture(t, "")
	c.Subscriptions[0].OverdriveEnabled = true
	c.Subscriptions[0].OverdriveWindows = []model.OverdriveWindow{{ID: "over", Name: "Before reset", Enabled: true, StartMode: "reset_relative", HoursBeforeReset: 24, EndMode: "next_reset", TargetUsedPercent: 100}}
	c.Automations[0].Mode = "overdrive"
	c.Automations[0].HonorOverdrive = true
	c.Automations[0].OnlyOverdrive = true
	c.Automations[0].Participants = []string{"sequence"}
	c.Automations[0].Entries = nil
	c.TimeWindows = []model.TimeWindow{{ID: "outer", Name: "Outer", Enabled: true, StartMode: "reset_relative", HoursBeforeReset: 12}}
	c.Projects[0].WindowID = "outer"
	var err error
	c, err = s.Apply(c.Revision, c)
	if err != nil {
		t.Fatal(err)
	}
	out, err := s.NextOpportunity(context.Background(), "project")
	expected := s.now().Add(60 * time.Hour)
	if err != nil || out.At == nil || !out.At.Equal(expected) {
		t.Fatalf("reset intersection %+v %v", out, err)
	}
	r, _ := s.Runtime()
	u := r.Usage["account"]
	u.Weekly.ResetTrusted = false
	if err = s.PutUsage(u); err != nil {
		t.Fatal(err)
	}
	out, err = s.NextOpportunity(context.Background(), "project")
	if err != nil || out.At != nil || !strings.Contains(out.Reason, "trusted reset") {
		t.Fatalf("untrusted reset guessed %+v %v", out, err)
	}
	u.Weekly.ResetTrusted = true
	u.Weekly.UsedPercent = 100
	if err = s.PutUsage(u); err != nil {
		t.Fatal(err)
	}
	out, err = s.NextOpportunity(context.Background(), "project")
	if err != nil || out.At != nil {
		t.Fatalf("exhausted override guessed %+v %v", out, err)
	}
}
