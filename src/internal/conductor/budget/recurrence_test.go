package budget_test

import (
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

// TESTMASTER: id=indigobird-schedule-recurring tier=fast parallel=yes
func TestIndigobirdRecurrence(t *testing.T) {
	r := model.Recurrence{Days: []int{1}, Start: "22:00", End: "06:00", Timezone: "America/Chicago", EveryWeeks: 1}
	for _, c := range []struct {
		at     string
		active bool
	}{{"2026-10-13T02:59:59Z", false}, {"2026-10-13T03:00:00Z", true}, {"2026-10-13T10:59:59Z", true}, {"2026-10-13T11:00:00Z", false}, {"2026-10-14T04:00:00Z", false}} {
		o, ok, err := budget.WindowAt(r, instant(c.at))
		if err != nil || ok != c.active {
			t.Fatalf("at %s: %v %v %+v", c.at, ok, err, o)
		}
	}
	r.Start = "00:00"
	r.End = "00:00"
	o, ok, err := budget.WindowAt(r, instant("2026-10-12T12:00:00Z"))
	if err != nil || !ok || o.End.Sub(o.Start) != 24*time.Hour {
		t.Fatal("equal endpoints not full day")
	}
	r.End = "23:59"
	_, ok, _ = budget.WindowAt(r, instant("2026-10-13T04:59:00Z"))
	if ok {
		t.Fatal("23:59 incorrectly all-day")
	}
	r.EveryWeeks = 2
	r.AnchorDate = "2026-10-12"
	for _, c := range []struct {
		at     string
		active bool
	}{{"2026-10-12T12:00:00Z", true}, {"2026-10-19T12:00:00Z", false}, {"2026-10-26T12:00:00Z", true}} {
		_, ok, err := budget.WindowAt(r, instant(c.at))
		if err != nil || ok != c.active {
			t.Fatalf("anchor %s: %v %v", c.at, ok, err)
		}
	}
	r.Days = nil
	if budget.ValidateRecurrence(r) == nil {
		t.Fatal("empty weekdays valid")
	}
	r.Days = []int{1}
	r.Timezone = "invalid"
	if budget.ValidateRecurrence(r) == nil {
		t.Fatal("invalid timezone valid")
	}
}

// TESTMASTER: id=indigobird-schedule-dst tier=fast parallel=yes
func TestIndigobirdDST(t *testing.T) {
	r := model.Recurrence{Days: []int{7}, Start: "01:00", End: "04:00", Timezone: "America/Chicago", EveryWeeks: 1}
	o, ok, err := budget.WindowAt(r, instant("2026-03-08T08:30:00Z"))
	if err != nil || !ok || o.End.Sub(o.Start) != 2*time.Hour {
		t.Fatalf("spring interval %+v %v", o, err)
	}
	r.Start = "02:30"
	o, ok, err = budget.WindowAt(r, instant("2026-03-08T08:00:00Z"))
	if err != nil || !ok || !o.Start.Equal(instant("2026-03-08T08:00:00Z")) {
		t.Fatalf("missing clock instant %+v %v", o, err)
	}
	r.Start = "01:00"
	r.End = "01:30"
	o, ok, err = budget.WindowAt(r, instant("2026-11-01T07:15:00Z"))
	if err != nil || !ok || o.End.Sub(o.Start) != 90*time.Minute {
		t.Fatalf("repeated clock interval %+v %v", o, err)
	}
}

// TESTMASTER: id=indigobird-overdrive-reset-relative tier=fast parallel=yes
func TestIndigobirdResetRelative(t *testing.T) {
	_, _, u, now := fixture()
	w := model.TimeWindow{ID: "before", Revision: 2, Enabled: true, StartMode: "reset_relative", HoursBeforeReset: 100}
	o, ok, err := budget.ResolveTimeWindow(w, u.Weekly, now)
	if err != nil || !ok || o.End.Sub(o.Start) != 100*time.Hour || o.ID == "" {
		t.Fatalf("relative %+v %v", o, err)
	}
	w.HoursBeforeReset = 24
	if _, ok, _ := budget.ResolveTimeWindow(w, u.Weekly, now); ok {
		t.Fatal("outside relative window accepted")
	}
	w.HoursBeforeReset = 100
	u.Weekly.ResetTrusted = false
	if _, ok, _ := budget.ResolveTimeWindow(w, u.Weekly, now); ok {
		t.Fatal("estimated reset accepted")
	}
	u.Weekly.ResetTrusted = true
	od := window("od", 60)
	od.StartMode = "reset_relative"
	od.HoursBeforeReset = 100
	o, ok, err = budget.OverdriveAt(od, u.Weekly, now)
	if err != nil || !ok || o.End.Sub(o.Start) != 100*time.Hour {
		t.Fatal(o, err)
	}
}

// TESTMASTER: id=indigobird-overdrive-custom-explicit tier=fast parallel=yes
func TestIndigobirdExplicitEndAndEpoch(t *testing.T) {
	p, s, u, now := fixture()
	w := window("w", 60)
	w.EndMode = "explicit"
	w.EndDay = 5
	w.EndTime = "00:00"
	s.OverdriveWindows = []model.OverdriveWindow{w}
	o, ok, err := budget.OverdriveAt(w, u.Weekly, now)
	if err != nil || !ok || !o.End.Equal(instant("2026-10-16T00:00:00Z")) {
		t.Fatal(o, ok, err)
	}
	if _, ok, _ := budget.OverdriveAt(w, u.Weekly, o.End); ok {
		t.Fatal("exclusive explicit end ignored")
	}
	d := budget.Evaluate(p, s, u, now, model.Green, true, true, nil)
	if !d.Allowed {
		t.Fatal(d)
	}
	u.Weekly.StartsAt = now.Add(-time.Hour)
	u.Weekly.Epoch = "manual-reset"
	u.Weekly.ResetsAt = now.Add(7 * 24 * time.Hour)
	d = budget.Evaluate(p, s, u, now, model.Green, true, true, d.Occurrences)
	if d.Allowed || len(d.Occurrences) != 1 || !d.Occurrences[0].Completed {
		t.Fatal("old occurrence crossed trusted reset", d)
	}
}
