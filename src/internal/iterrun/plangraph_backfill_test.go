package iterrun

import (
	"crypto/sha256"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

var backfillNow = time.Date(2026, 10, 23, 3, 0, 0, 0, time.UTC)

func backfillProject(t *testing.T, plans map[string]string, milestones string) string {
	t.Helper()
	dir := t.TempDir()
	for name, body := range plans {
		writePlanFixture(t, dir, name, body)
	}
	if milestones != "" {
		if err := os.MkdirAll(filepath.Join(dir, "docs"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "docs", "milestones.md"), []byte(milestones), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func bfPlan(name string, keys []string, constraints ...string) string {
	return "# Iterate Task — " + name + "\n\nname: " + name + "\n" + strings.Join(keys, "\n") + "\n\n## Goal\ng\n\n## Steps\n- [ ] 1. x\n\n## Validation\n- [ ] 1. y\n\n## Constraints\n" + strings.Join(constraints, "\n") + "\n"
}

func rowFor(t *testing.T, r BackfillReport, plan string) BackfillRow {
	t.Helper()
	for _, row := range r.Rows {
		if row.Plan == plan {
			return row
		}
	}
	t.Fatalf("no row for %s in %+v", plan, r.Rows)
	return BackfillRow{}
}

func hashTree(t *testing.T, dir string) map[string][32]byte {
	t.Helper()
	out := map[string][32]byte{}
	files, _ := filepath.Glob(filepath.Join(dir, ".claude", "iterate", "plans", "*"))
	for _, f := range files {
		b, _ := os.ReadFile(f)
		out[f] = sha256.Sum256(b)
	}
	return out
}

const kingtutMilestones = `# Milestones

### M1-05 — The whole step machine

**Depends on:** M1-03

### M1-06 — Values, secrets and files

**Depends on:** M1-04, M1-05; Papyrus spec 1.0 defining every v1 element (external)
`

// TESTMASTER: id=dag-bf-1 tier=fast parallel=yes
func TestDagBf1MilestonesDocProposesInternalIDs(t *testing.T) {
	dir := backfillProject(t, map[string]string{"eider": bfPlan("eider", []string{"phase: planned", "milestone: M1-06"}, "- Depends on: the companion and the relay")}, kingtutMilestones)
	r, err := BackfillDepends(dir, false, backfillNow)
	if err != nil {
		t.Fatal(err)
	}
	if row := rowFor(t, r, "eider"); row.Action != "propose" || row.Line != "depends: M1-04, M1-05" {
		t.Fatalf("eider = %+v", row)
	}
}

// TESTMASTER: id=dag-bf-2 tier=fast parallel=yes
func TestDagBf2FromPlanProseProposesThePlan(t *testing.T) {
	dir := backfillProject(t, map[string]string{"dace": bfPlan("dace", []string{"phase: executing", "status: unblocked"}, "- Depends on: the artifacts named in `docs/implementation/phase_3.md` `## Exit criteria` (from plan caiman) — Phase 3's exit artifacts")}, "")
	r, _ := BackfillDepends(dir, false, backfillNow)
	if row := rowFor(t, r, "dace"); row.Line != "depends: caiman" {
		t.Fatalf("dace = %+v", row)
	}
}

// TESTMASTER: id=dag-bf-3 tier=fast parallel=yes
func TestDagBf3DryRunChangesNothing(t *testing.T) {
	dir := backfillProject(t, map[string]string{
		"dace":  bfPlan("dace", []string{"phase: planned"}, "- Depends on: x (from plan caiman)"),
		"eider": bfPlan("eider", []string{"phase: planned", "milestone: M1-06"}),
	}, kingtutMilestones)
	before := hashTree(t, dir)
	if _, err := BackfillDepends(dir, false, backfillNow); err != nil {
		t.Fatal(err)
	}
	if after := hashTree(t, dir); len(after) != len(before) {
		t.Fatal("dry run created or removed files")
	} else {
		for f, h := range before {
			if after[f] != h {
				t.Fatalf("dry run changed %s", f)
			}
		}
	}
}

// TESTMASTER: id=dag-bf-4 tier=fast parallel=yes
func TestDagBf4AnExistingDependsLineIsKept(t *testing.T) {
	body := bfPlan("dace", []string{"phase: planned", "depends: baboon"}, "- Depends on: x (from plan caiman)")
	dir := backfillProject(t, map[string]string{"dace": body}, "")
	r, _ := BackfillDepends(dir, true, backfillNow)
	if row := rowFor(t, r, "dace"); row.Action != "keep" || row.Line != "depends: baboon" {
		t.Fatalf("dace = %+v", row)
	}
	after, _ := os.ReadFile(filepath.Join(dir, ".claude", "iterate", "plans", "dace.md"))
	if string(after) != body {
		t.Fatal("a hand-written depends: line was rewritten")
	}
}

// TESTMASTER: id=dag-bf-5 tier=fast parallel=yes
func TestDagBf5ALivePlanIsSkipped(t *testing.T) {
	dir := backfillProject(t, map[string]string{
		"caiman": bfPlan("caiman", []string{"conductor-assignment: assignment-f6b3", "phase: executing", "running: 2026-10-23T02:58:12Z"}, "- Depends on: x (from plan baboon)"),
		"dace":   bfPlan("dace", []string{"phase: executing", "running: 2026-10-23T02:59:50Z"}, "- Depends on: x (from plan caiman)"),
		"eider":  bfPlan("eider", []string{"phase: executing", "running: true"}, "- Depends on: x (from plan dace)"),
		"fennec": bfPlan("fennec", []string{"phase: executing", "running: 2026-10-22T02:59:50Z"}, "- Depends on: x (from plan eider)"),
	}, "")
	r, _ := BackfillDepends(dir, true, backfillNow)
	for _, name := range []string{"caiman", "dace", "eider"} {
		if row := rowFor(t, r, name); row.Action != "skip" {
			t.Fatalf("%s = %+v, want skip", name, row)
		}
	}
	if row := rowFor(t, r, "fennec"); row.Action != "applied" {
		t.Fatalf("a stale heartbeat is not a live run: %+v", row)
	}
}

// TESTMASTER: id=dag-bf-6 tier=fast parallel=yes
func TestDagBf6ApplyAddsExactlyOneLine(t *testing.T) {
	body := bfPlan("iiwi", []string{"phase: planned", "status: queued: 2026-10-22T22:25:34Z by /ip stage", "milestone: M1-05", "teamed: true"}, "- Depends on: the step machine")
	dir := backfillProject(t, map[string]string{"iiwi": body}, kingtutMilestones)
	path := filepath.Join(dir, ".claude", "iterate", "plans", "iiwi.md")
	orig := filepath.Join(t.TempDir(), "iiwi.md")
	if err := os.WriteFile(orig, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := BackfillDepends(dir, true, backfillNow); err != nil {
		t.Fatal(err)
	}
	diff, _ := exec.Command("diff", orig, path).CombinedOutput()
	if strings.TrimSpace(string(diff)) != "6a7\n> depends: M1-03" {
		t.Fatalf("diff =\n%s", diff)
	}
	after, _ := os.ReadFile(path)
	if !strings.Contains(string(after), "status: queued: 2026-10-22T22:25:34Z by /ip stage\n") || !strings.Contains(string(after), "- [ ] 1. x\n") {
		t.Fatal("staging or a step changed")
	}
	if ps, _ := ListPlans(dir); len(ps) != 1 || !slices.Equal(ps[0].Depends, []string{"M1-03"}) {
		t.Fatalf("inserted line is not read as a key: %+v", ps)
	}
}

// TESTMASTER: id=dag-bf-7 tier=fast parallel=yes
func TestDagBf7AnInterruptedApplyResumesCleanly(t *testing.T) {
	dir := backfillProject(t, map[string]string{
		"dace":  bfPlan("dace", []string{"phase: planned"}, "- Depends on: x (from plan caiman)"),
		"eider": bfPlan("eider", []string{"phase: planned"}, "- Depends on: x (from plan dace)"),
	}, "")
	// The first write landed, then the process died before the second.
	first := filepath.Join(dir, ".claude", "iterate", "plans", "dace.md")
	raw, _ := os.ReadFile(first)
	if err := insertKeyLine(first, raw, "depends: caiman"); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(dir, ".claude", "iterate", "plans", ".eider.md.tmp-123"), []byte("partial"), 0o600)
	r, err := BackfillDepends(dir, true, backfillNow)
	if err != nil {
		t.Fatal(err)
	}
	if rowFor(t, r, "dace").Action != "keep" || rowFor(t, r, "eider").Action != "applied" {
		t.Fatalf("rerun = %+v", r.Rows)
	}
	for _, name := range []string{"dace", "eider"} {
		b, _ := os.ReadFile(filepath.Join(dir, ".claude", "iterate", "plans", name+".md"))
		if strings.Count(string(b), "\ndepends: ") != 1 || !strings.HasSuffix(string(b), "\n") {
			t.Fatalf("%s:\n%s", name, b)
		}
	}
}

// TESTMASTER: id=dag-bf-8 tier=fast parallel=yes
func TestDagBf8FromMilestoneProseProposesTheID(t *testing.T) {
	dir := backfillProject(t, map[string]string{"herring": bfPlan("herring", []string{"phase: planned", "milestone: M3-01"}, "- Depends on: the released v2 product (from milestone M2-10)")}, "")
	r, _ := BackfillDepends(dir, false, backfillNow)
	if row := rowFor(t, r, "herring"); row.Line != "depends: M2-10" {
		t.Fatalf("herring = %+v", row)
	}
}

// TESTMASTER: id=dag-bf-9 tier=fast parallel=yes
func TestDagBf9DependsOnNothingProposesNone(t *testing.T) {
	dir := backfillProject(t, map[string]string{"tern": bfPlan("tern", []string{"phase: planned"}, "- Depends on: nothing. Stage 2's packages, its catalogue, and the Windows credential proof are all on main.")}, "")
	r, _ := BackfillDepends(dir, false, backfillNow)
	if row := rowFor(t, r, "tern"); row.Line != "depends: none" {
		t.Fatalf("tern = %+v", row)
	}
}

// TESTMASTER: id=dag-bf-10 tier=fast parallel=yes
func TestDagBf10UnparseableProseIsUnresolved(t *testing.T) {
	dir := backfillProject(t, map[string]string{
		"stoat": bfPlan("stoat", []string{"phase: planned"}, "- Depends on: stage 1's packages merged to main"),
		"wren":  bfPlan("wren", []string{"phase: planned"}),
	}, "")
	r, _ := BackfillDepends(dir, true, backfillNow)
	row := rowFor(t, r, "stoat")
	if row.Action != "unresolved" || row.Line != "" || row.Prose != "stage 1's packages merged to main" {
		t.Fatalf("stoat = %+v", row)
	}
	if rowFor(t, r, "wren").Action != "none" {
		t.Fatal("a plan with no source needs no line")
	}
	b, _ := os.ReadFile(filepath.Join(dir, ".claude", "iterate", "plans", "stoat.md"))
	if strings.Contains(string(b), "\ndepends:") {
		t.Fatal("an unresolved plan got a line")
	}
}
