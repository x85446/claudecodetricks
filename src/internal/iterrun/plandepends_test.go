package iterrun

import (
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func writePlanFixture(t *testing.T, dir, name, body string) string {
	t.Helper()
	path := filepath.Join(dir, ".claude", "iterate", "plans", name+".md")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestPlanSummaryReadsDependsAndMilestoneFromTheKeyBlock(t *testing.T) {
	dir := t.TempDir()
	writePlanFixture(t, dir, "eider", "# Iterate Task — eider\n\nname: eider\nphase: planned\nmilestone: M1-06\ndepends: caiman, M1-03\n\n## Goal\ng\n\n## Steps\n- [ ] 1. x\n")
	writePlanFixture(t, dir, "fennec", "# Iterate Task — fennec\n\nname: fennec\nphase: planned\ndepends: none\n\n## Goal\ng\n")
	writePlanFixture(t, dir, "gecko", "# Iterate Task — gecko\n\nname: gecko\nphase: planned\n\n## Goal\ng\n\ndepends: body-text\nmilestone: M9-99\n")
	plans, err := ListPlans(dir)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]PlanSummary{}
	for _, p := range plans {
		got[p.Name] = p
	}
	if e := got["eider"]; !slices.Equal(e.Depends, []string{"caiman", "M1-03"}) || !e.DependsDeclared || e.Milestone != "M1-06" {
		t.Fatalf("eider = %v %v %q", e.Depends, e.DependsDeclared, e.Milestone)
	}
	if f := got["fennec"]; len(f.Depends) != 0 || !f.DependsDeclared {
		t.Fatalf("fennec = %v %v, want declared with no edges", f.Depends, f.DependsDeclared)
	}
	if g := got["gecko"]; len(g.Depends) != 0 || g.DependsDeclared || g.Milestone != "" {
		t.Fatalf("gecko read body lines as keys: %v %v %q", g.Depends, g.DependsDeclared, g.Milestone)
	}
}

func TestProjectGraphNeverWritesAPlan(t *testing.T) {
	dir := t.TempDir()
	paths := []string{
		writePlanFixture(t, dir, "caiman", "# Iterate Task — caiman\n\nname: caiman\nphase: executing\nstatus: blocked-on-operator: ssh\n\n## Goal\ng\n"),
		writePlanFixture(t, dir, "dace", "# Iterate Task — dace\n\nname: dace\nphase: executing\nstatus: unblocked\ndepends: caiman\n\n## Goal\ng\n"),
	}
	before := [][]byte{}
	for _, p := range paths {
		b, _ := os.ReadFile(p)
		before = append(before, b)
	}
	g, err := ProjectGraph(dir)
	if err != nil {
		t.Fatal(err)
	}
	if n, ok := g.Node("dace"); !ok || n.Status != "waiting" || n.HeldBy != "caiman" {
		t.Fatalf("dace = %+v", n)
	}
	for i, p := range paths {
		after, _ := os.ReadFile(p)
		if !bytes.Equal(before[i], after) {
			t.Fatalf("%s changed after the graph was read", p)
		}
	}
}
