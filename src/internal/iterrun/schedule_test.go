package iterrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// 2026-10-02 is a Friday, 10-03 Saturday, 10-05 Monday.
func at(day, hour, min int) time.Time {
	return time.Date(2026, 10, day, hour, min, 0, 0, time.UTC)
}

func mustSchedule(t *testing.T, name string, lines ...string) Schedule {
	t.Helper()
	s, err := ParseSchedule(name, lines...)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// TESTMASTER: id=iterrun.TestScheduleWeeknightWindowWrapsMidnightAndKeepsOpeningDay tier=? parallel=yes
func TestScheduleWeeknightWindowWrapsMidnightAndKeepsOpeningDay(t *testing.T) {
	s := mustSchedule(t, "launch-schedule", "allow mon-fri 22:00-06:00")
	// sch-1: Friday 23:30 is inside the window.
	if ok, _, _ := s.Allowed(at(2, 23, 30)); !ok {
		t.Error("sch-1: Fri 23:30 refused; want allowed")
	}
	// sch-2: Saturday 02:00 belongs to Friday's window.
	if ok, _, _ := s.Allowed(at(3, 2, 0)); !ok {
		t.Error("sch-2: Sat 02:00 refused; the window opened Friday 22:00")
	}
	// sch-3: Saturday 23:00 is refused, next window Monday 22:00.
	ok, rule, next := s.Allowed(at(3, 23, 0))
	if ok {
		t.Fatal("sch-3: Sat 23:00 allowed; want refused")
	}
	if rule != "launch-schedule: no allow rule matches" {
		t.Errorf("sch-3: rule = %q", rule)
	}
	if want := at(5, 22, 0); !next.Equal(want) {
		t.Errorf("sch-3: next = %v, want %v", next, want)
	}
	// Boundary: 06:00 is outside (end exclusive), 05:59 inside.
	if ok, _, _ := s.Allowed(at(3, 6, 0)); ok {
		t.Error("Sat 06:00 allowed; end is exclusive")
	}
	if ok, _, _ := s.Allowed(at(3, 5, 59)); !ok {
		t.Error("Sat 05:59 refused; want allowed")
	}
}

// TESTMASTER: id=iterrun.TestScheduleDenyBeatsAllow tier=? parallel=yes
func TestScheduleDenyBeatsAllow(t *testing.T) {
	// sch-4
	s := mustSchedule(t, "launch-schedule", "allow daily", "deny sat,sun")
	ok, rule, _ := s.Allowed(at(3, 12, 0))
	if ok || rule != "launch-schedule: deny sat,sun" {
		t.Errorf("sch-4: Saturday = (%v, %q); want refused by the deny line", ok, rule)
	}
	if ok, _, _ := s.Allowed(at(5, 12, 0)); !ok {
		t.Error("sch-4: Monday refused; want allowed")
	}
}

// TESTMASTER: id=iterrun.TestScheduleOneAllowMakesDefaultDeny tier=? parallel=yes
func TestScheduleOneAllowMakesDefaultDeny(t *testing.T) {
	// sch-5
	s := mustSchedule(t, "launch-schedule", "allow mon 10:00-11:00")
	if ok, _, _ := s.Allowed(at(6, 10, 30)); ok {
		t.Error("sch-5: Tuesday 10:30 allowed; one allow rule must make everything else refused")
	}
	if ok, _, _ := s.Allowed(at(5, 10, 30)); !ok {
		t.Error("sch-5: Monday 10:30 refused; want allowed")
	}
}

// TESTMASTER: id=iterrun.TestScheduleNoRulesAlwaysAllows tier=? parallel=yes
func TestScheduleNoRulesAlwaysAllows(t *testing.T) {
	// sch-6
	s := mustSchedule(t, "launch-schedule")
	ok, rule, next := s.Allowed(at(3, 3, 3))
	if !ok || rule != "" || !next.IsZero() {
		t.Errorf("sch-6: empty schedule = (%v, %q, %v); want (true, \"\", zero)", ok, rule, next)
	}
}

// TESTMASTER: id=iterrun.TestConductorScheduleOnlyNarrows tier=? parallel=yes
func TestConductorScheduleOnlyNarrows(t *testing.T) {
	launch := mustSchedule(t, "launch-schedule", "allow daily 22:00-06:00")
	// sch-7: a wider conductor schedule cannot open the day.
	wide := mustSchedule(t, "conductor-schedule", "allow daily")
	ok, rule, _ := AllowedAll(at(5, 12, 0), launch, wide)
	if ok || !strings.HasPrefix(rule, "launch-schedule:") {
		t.Errorf("sch-7: noon = (%v, %q); want refused by the launch schedule", ok, rule)
	}
	// sch-8: a narrower one narrows, and next is the first minute BOTH allow.
	narrow := mustSchedule(t, "conductor-schedule", "allow daily 23:00-05:00")
	ok, rule, next := AllowedAll(at(5, 22, 30), launch, narrow)
	if ok || rule != "conductor-schedule: no allow rule matches" {
		t.Errorf("sch-8: 22:30 = (%v, %q); want refused by the conductor schedule", ok, rule)
	}
	if want := at(5, 23, 0); !next.Equal(want) {
		t.Errorf("sch-8: next = %v, want %v", next, want)
	}
	if ok, _, _ := AllowedAll(at(5, 23, 30), launch, narrow); !ok {
		t.Error("sch-8: 23:30 refused; both schedules allow it")
	}
}

// TESTMASTER: id=iterrun.TestScheduleMalformedLineFailsClosedAndIsNamed tier=? parallel=yes
func TestScheduleMalformedLineFailsClosedAndIsNamed(t *testing.T) {
	// sch-9
	_, err := ParseSchedule("launch-schedule", "allow mon-fri 22:00-06:00", "allow monday-ish 25:00")
	if err == nil {
		t.Fatal("sch-9: malformed rule parsed")
	}
	if !strings.Contains(err.Error(), "allow monday-ish 25:00") || !strings.Contains(err.Error(), "launch-schedule") {
		t.Errorf("sch-9: error does not name the line and schedule: %v", err)
	}
	for _, bad := range []string{"permit daily", "allow 22:00-25:00", "allow 2026-13-01", "allow mon mon", "allow 2026-12-26..2026-12-24"} {
		if _, err := ParseSchedule("x", bad); err == nil {
			t.Errorf("%q parsed; want an error", bad)
		}
	}
}

// TESTMASTER: id=iterrun.TestScheduleDatesMatchOpeningDay tier=? parallel=yes
func TestScheduleDatesMatchOpeningDay(t *testing.T) {
	// A date-only deny covers those calendar days: the small hours of the
	// 27th are the 27th's own (untimed) window, so they are allowed again.
	s := mustSchedule(t, "launch-schedule", "allow daily 22:00-06:00", "deny 2026-12-24..2026-12-26")
	if ok, _, _ := s.Allowed(time.Date(2026, 12, 25, 23, 0, 0, 0, time.UTC)); ok {
		t.Error("Dec 25 23:00 allowed; want denied by the date rule")
	}
	if ok, _, _ := s.Allowed(time.Date(2026, 12, 27, 2, 0, 0, 0, time.UTC)); !ok {
		t.Error("Dec 27 02:00 refused; an untimed date deny ends at midnight")
	}
	// A timed deny on a date follows the opening-day rule: the window that
	// opened Dec 26 22:00 still covers Dec 27 02:00.
	s = mustSchedule(t, "launch-schedule", "allow daily 22:00-06:00", "deny 2026-12-26 22:00-06:00")
	if ok, _, _ := s.Allowed(time.Date(2026, 12, 27, 2, 0, 0, 0, time.UTC)); ok {
		t.Error("Dec 27 02:00 allowed; that window opened Dec 26, which is denied")
	}
	if ok, _, _ := s.Allowed(time.Date(2026, 12, 27, 23, 0, 0, 0, time.UTC)); !ok {
		t.Error("Dec 27 23:00 refused; the deny covered only the night of the 26th")
	}
}

// TESTMASTER: id=iterrun.TestReadSchedulesFromProjectFiles tier=? parallel=yes
func TestReadSchedulesFromProjectFiles(t *testing.T) {
	dir := t.TempDir()
	it := filepath.Join(dir, ".claude", "iterate")
	if err := os.MkdirAll(it, 0o755); err != nil {
		t.Fatal(err)
	}
	policy := "---\nrequire-launch-keyword: permission\nlaunch-schedule:\n  - allow mon-fri 22:00-06:00   # weeknights\n  - deny 2026-12-24..2026-12-26\n---\n\n# Iterate policy\n\nlaunch-schedule:\n  - allow daily\n"
	if err := os.WriteFile(filepath.Join(it, "policy.md"), []byte(policy), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err := ReadLaunchSchedule(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rules) != 2 || s.Rules[0].Raw != "allow mon-fri 22:00-06:00" {
		t.Errorf("launch rules = %+v; want the two frontmatter rules only (body text ignored, comment stripped)", s.Rules)
	}

	conductor := "---\nenabled: true\npaused: false\nconductor-schedule:         # optional\n  - allow daily 23:00-05:00 #   intersected\ntick: working\n---\n\n## Sweep log\n"
	if err := os.WriteFile(filepath.Join(it, "conductor.md"), []byte(conductor), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := ReadConductorSchedule(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(c.Rules) != 1 || c.Rules[0].Raw != "allow daily 23:00-05:00" {
		t.Errorf("conductor rules = %+v", c.Rules)
	}

	// launch-window shorthand, used only when launch-schedule is absent.
	if err := os.WriteFile(filepath.Join(it, "policy.md"), []byte("---\nlaunch-window: 22:00-06:00\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s, err = ReadLaunchSchedule(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(s.Rules) != 1 || s.Rules[0].Raw != "allow daily 22:00-06:00" {
		t.Errorf("launch-window shorthand = %+v", s.Rules)
	}

	// No files at all: empty, always allowed.
	empty, err := ReadLaunchSchedule(t.TempDir())
	if err != nil || !empty.Empty() {
		t.Errorf("missing policy.md = (%+v, %v); want empty, nil", empty, err)
	}
}
