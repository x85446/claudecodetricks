package iterrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TESTMASTER: id=planitem-three-formats tier=fast parallel=yes
//
// The iterate family writes plan items in three shapes because its own
// skills disagree: `/iterate-planner`'s template emits `1. <task>`, while
// `/iterate`'s emits `- [ ] 1. <step>` and `- [ ] check 1: <criterion>`.
// Reading only the planner's form left 31 of this machine's 136 plan files
// with no Requirements row at all.
func TestParsePlanItemReadsEveryFormatTheFamilyWrites(t *testing.T) {
	cases := []struct {
		name, line         string
		num                int
		text               string
		boxed, checked, ok bool
	}{
		{"planner plain", "1. Verify the izcr git remote is reachable", 1, "Verify the izcr git remote is reachable", false, false, true},
		{"executor unchecked", "- [ ] 2. Download every missing period", 2, "Download every missing period", true, false, true},
		{"executor checked", "- [x] 1. Verify the signed-in sessions", 1, "Verify the signed-in sessions", true, true, true},
		{"executor checked caps", "- [X] 7. Run the suite", 7, "Run the suite", true, true, true},
		{"asterisk bullet", "* [x] 3. Stage the files", 3, "Stage the files", true, true, true},
		{"check form checked", "- [x] check 1: all 18 instances STOPPED", 1, "all 18 instances STOPPED", true, true, true},
		{"check form unchecked", "- [ ] check 12: the gate reads CLEAN", 12, "the gate reads CLEAN", true, false, true},
		{"check form bare", "check 4: exits 0", 4, "exits 0", false, false, true},
		{"paren separator", "5) Prove it", 5, "Prove it", false, false, true},
		// A partial marker is not a tick. This must never turn an
		// unproven step green.
		{"tilde is not done", "- [~] 4. half way", 4, "half way", true, false, true},
		{"slash is not done", "- [/] 4. started", 4, "started", true, false, true},

		// Not items: nothing to pair a validation against.
		{"unnumbered checkbox", "- [ ] CLEANUP: restart gate daemon", 0, "", false, false, false},
		{"prose", "This plan depends on step 3 landing first", 0, "", false, false, false},
		{"blank", "", 0, "", false, false, false},
		{"heading", "## Steps", 0, "", false, false, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := parsePlanItem(c.line)
			if ok != c.ok {
				t.Fatalf("parsePlanItem(%q) ok=%v, want %v", c.line, ok, c.ok)
			}
			if !c.ok {
				return
			}
			if got.Num != c.num || got.Text != c.text {
				t.Errorf("parsePlanItem(%q) = %d/%q, want %d/%q", c.line, got.Num, got.Text, c.num, c.text)
			}
			if got.Boxed != c.boxed || got.Checked != c.checked {
				t.Errorf("parsePlanItem(%q) boxed=%v checked=%v, want %v/%v",
					c.line, got.Boxed, got.Checked, c.boxed, c.checked)
			}
		})
	}
}

// TESTMASTER: id=planitem-box-vstatus tier=fast parallel=yes
//
// A plan's two checkboxes for one requirement routinely disagree, and the
// disagreement is the interesting part. halibut has step 1 ticked and
// validation 1 unticked, which is exactly true: the step ran and named the
// sites behind login walls, and its validation — "the sites with gaps are
// signed in" — is what the plan is blocked on. Reporting that as proven
// would be the worst possible answer.
func TestCheckboxStatusNeverClaimsAnUnprovenStepIsMet(t *testing.T) {
	cases := []struct {
		name string
		box  boxState
		want string
	}{
		{"nothing known", boxState{}, ""},
		{"validation ticked", boxState{ValidationBoxed: true, ValidationChecked: true}, "met"},
		{"step ticked, validation explicitly not", boxState{StepBoxed: true, StepChecked: true, ValidationBoxed: true}, "partial"},
		{"step ticked, no validation box at all", boxState{StepBoxed: true, StepChecked: true}, "met"},
		{"both ticked", boxState{StepBoxed: true, StepChecked: true, ValidationBoxed: true, ValidationChecked: true}, "met"},
		{"boxes present, none ticked", boxState{StepBoxed: true, ValidationBoxed: true}, ""},
	}
	for _, c := range cases {
		if got := c.box.vstatus(); got != c.want {
			t.Errorf("%s: vstatus() = %q, want %q", c.name, got, c.want)
		}
	}
}

// writePlan drops a plan file into a temp project and returns the project dir.
func writePlan(t *testing.T, name, body string) string {
	t.Helper()
	proj := t.TempDir()
	dir := filepath.Join(proj, ".claude", "iterate", "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, name+".md"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return proj
}

// TESTMASTER: id=planitem-boxed-plan-requirements tier=fast parallel=yes
//
// The whole user-visible bug: an unteamed plan written in the executor's
// checkbox format produced NO rows from the filesystem, so its steps never
// reached the page and the Requirements row vanished entirely. Confirmed
// live on halibut, filemaster's wombat (56 items) and izbooter's badger
// (52 items) — all three live plans at the time.
func TestABoxedUnteamedPlanStillGetsItsRequirements(t *testing.T) {
	proj := writePlan(t, "halibut", `# Iterate Task — signed-in sweeps

name: halibut
phase: executing
Executing: 2026-09-15T19:39:14Z
teamed: false

## Goal
The two sweeps only a signed-in session can run.

## Steps
- [x] 1. Verify the signed-in sessions every later step depends on.
- [ ] 2. Download every missing period the trackers name.
- [ ] 3. The E*TRADE deposit-image sweep.

## Validation
- [ ] 1. Every site reads signed in, or is named for the operator.
- [ ] 2. v_missing_statements returns 0 rows.
- [ ] 3. deposit_image_audit.py reads slip-only 0.
`)

	rows, err := BuildRowsFromFilesystem("halibut", proj, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 coordinator row: %+v", len(rows), rows)
	}
	if rows[0].key != "" {
		t.Errorf("row key = %q, want the coordinator's empty key", rows[0].key)
	}
	if len(rows[0].steps) != 3 {
		t.Fatalf("coordinator steps = %d, want 3", len(rows[0].steps))
	}

	burn := collectSteps(rows)
	if len(burn) != 3 {
		t.Fatalf("burndown steps = %d, want 3 — this is the Requirements row", len(burn))
	}
	// Step 1 ticked, its validation explicitly not: attempted, not proven.
	if burn[0].VStatus != "partial" {
		t.Errorf("req 1 vstatus = %q, want partial (step done, validation unmet)", burn[0].VStatus)
	}
	if burn[1].VStatus != "" || burn[2].VStatus != "" {
		t.Errorf("reqs 2-3 should carry no status yet, got %q/%q", burn[1].VStatus, burn[2].VStatus)
	}
	if !strings.Contains(burn[0].Step, "Verify the signed-in sessions") {
		t.Errorf("req 1 step text = %q", burn[0].Step)
	}
	if !strings.Contains(burn[0].Validation, "named for the operator") {
		t.Errorf("req 1 validation text = %q", burn[0].Validation)
	}
}

// TESTMASTER: id=planitem-mixed-plan tier=fast parallel=yes
//
// Eight plan files mix both formats. That case is worse than an unreadable
// plan, because a partial Requirements row looks correct.
func TestAMixedFormatPlanGetsEveryRequirementNotSomeOfThem(t *testing.T) {
	proj := writePlan(t, "nightjar", `# Iterate Task — mixed

name: nightjar
phase: executing
Executing: 2026-09-09T05:32:39Z

## Steps
1. A planner-format step.
- [x] 2. An executor-format step, done.
- [ ] 3. An executor-format step, pending.
4. Another planner-format step.

## Validation
1. Check one.
- [x] check 2: check two, proven.
- [ ] check 3: check three.
4. Check four.
`)

	rows, err := BuildRowsFromFilesystem("nightjar", proj, nil)
	if err != nil {
		t.Fatal(err)
	}
	burn := collectSteps(rows)
	if len(burn) != 4 {
		t.Fatalf("burndown steps = %d, want all 4 — a partial row is worse than none", len(burn))
	}
	if burn[1].VStatus != "met" {
		t.Errorf("req 2 vstatus = %q, want met (its `check 2:` box is ticked)", burn[1].VStatus)
	}
	if burn[0].VStatus != "" || burn[3].VStatus != "" {
		t.Errorf("plain-format items carry no box, so no status: got %q/%q", burn[0].VStatus, burn[3].VStatus)
	}
}

// TESTMASTER: id=planitem-statuslog-wins tier=fast parallel=yes
//
// An explicit "step N DONE: <note>" line in the Status/Log is more
// specific than a checkbox and carries the note explaining a partial or a
// blocker, so it must win.
func TestAnExplicitStatusLogMarkOverridesTheCheckbox(t *testing.T) {
	proj := writePlan(t, "badger", `# Iterate Task — override

name: badger
phase: executing
Executing: 2026-09-15T08:00:00Z

## Steps
- [x] 1. Do the thing.

## Validation
- [x] 1. The thing is done.

## Status / Log
- step 1 partial: the perf clause fails on a laptop, accepted.
`)

	rows, err := BuildRowsFromFilesystem("badger", proj, nil)
	if err != nil {
		t.Fatal(err)
	}
	burn := collectSteps(rows)
	if len(burn) != 1 {
		t.Fatalf("burndown steps = %d, want 1", len(burn))
	}
	if burn[0].VStatus != "partial" {
		t.Errorf("vstatus = %q, want partial from the Status/Log, not met from the box", burn[0].VStatus)
	}
	if !strings.Contains(burn[0].VNote, "perf clause") {
		t.Errorf("note = %q, want the Status/Log's explanation", burn[0].VNote)
	}
}

// TESTMASTER: id=planitem-plain-plan-unchanged tier=fast parallel=yes
//
// 102 plan files use the planner's plain format and read correctly today.
// Accepting two more formats must not change any of them.
func TestPlainFormatPlansAreUnaffected(t *testing.T) {
	proj := writePlan(t, "ichneumon", `# Iterate Task — plain

name: ichneumon
phase: executing
Executing: 2026-09-15T08:00:00Z

## Steps
1. First step.
2. Second step.

## Validation
1. First check.
2. Second check.
`)

	rows, err := BuildRowsFromFilesystem("ichneumon", proj, nil)
	if err != nil {
		t.Fatal(err)
	}
	burn := collectSteps(rows)
	if len(burn) != 2 {
		t.Fatalf("burndown steps = %d, want 2", len(burn))
	}
	for _, b := range burn {
		if b.VStatus != "" {
			t.Errorf("req %d vstatus = %q — a plain plan has no boxes, so no status may be invented", b.Num, b.VStatus)
		}
		if b.State != "queued" {
			t.Errorf("req %d state = %q, want queued", b.Num, b.State)
		}
	}
}

// TESTMASTER: id=timeline-no-empty-team-section tier=fast parallel=yes
//
// An unteamed plan has exactly one row — the coordinator — already
// rendered in its own section. Emitting "Activity by team" anyway left a
// heading with nothing under it, which reads as missing data.
func TestUnteamedPlanOmitsTheEmptyTeamSection(t *testing.T) {
	base := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	coordOnly := []Row{{key: "", label: "coordinator",
		spans: []span{{start: base, end: base.Add(30 * time.Minute)}}}}
	out := RenderTimelineHTML(coordOnly, PlanSummary{Name: "halibut"}, "")
	if strings.Contains(out, "Activity by team") {
		t.Error("an unteamed plan must not render an empty 'Activity by team' section")
	}
	if !strings.Contains(out, "Coordinator") {
		t.Error("the coordinator section must still render")
	}

	withTeam := append(coordOnly, Row{key: "app", label: "app",
		spans: []span{{start: base.Add(5 * time.Minute), end: base.Add(20 * time.Minute)}}})
	out = RenderTimelineHTML(withTeam, PlanSummary{Name: "galago"}, "")
	if !strings.Contains(out, "Activity by team") {
		t.Error("a teamed plan must still render the team section")
	}
}
