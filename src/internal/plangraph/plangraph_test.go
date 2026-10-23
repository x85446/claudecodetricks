package plangraph

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// project is a fixture project directory with live plans and an archive.
type project struct {
	t   *testing.T
	dir string
}

func newProject(t *testing.T) project {
	t.Helper()
	dir := t.TempDir()
	for _, sub := range []string{"plans", "archive"} {
		if err := os.MkdirAll(filepath.Join(dir, ".claude", "iterate", sub), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return project{t: t, dir: dir}
}

func planText(name string, keys ...string) string {
	return "# Iterate Task — " + name + "\n\nname: " + name + "\n" + strings.Join(keys, "\n") + "\n\n## Goal\nfixture\n\n## Steps\n- [ ] 1. depends: not-a-key-line\n"
}

func (p project) plan(name string, keys ...string) {
	p.t.Helper()
	path := filepath.Join(p.dir, ".claude", "iterate", "plans", name+".md")
	if err := os.WriteFile(path, []byte(planText(name, keys...)), 0o644); err != nil {
		p.t.Fatal(err)
	}
}

func (p project) archive(file, name string, keys ...string) {
	p.t.Helper()
	path := filepath.Join(p.dir, ".claude", "iterate", "archive", file)
	if err := os.WriteFile(path, []byte(planText(name, keys...)), 0o644); err != nil {
		p.t.Fatal(err)
	}
}

func (p project) graph() Graph {
	p.t.Helper()
	g, err := Load(p.dir)
	if err != nil {
		p.t.Fatal(err)
	}
	return g
}

func node(t *testing.T, g Graph, name string) Node {
	t.Helper()
	n, ok := g.Node(name)
	if !ok {
		t.Fatalf("no node %s in %+v", name, g.Nodes)
	}
	return n
}

func wantStatus(t *testing.T, g Graph, name, status string, waitingOn ...string) Node {
	t.Helper()
	n := node(t, g, name)
	if n.Status != status {
		t.Fatalf("%s status = %q (%s), want %q", name, n.Status, n.Reason, status)
	}
	if waitingOn == nil {
		waitingOn = []string{}
	}
	if !slices.Equal(n.WaitingOn, waitingOn) {
		t.Fatalf("%s waiting_on = %v, want %v", name, n.WaitingOn, waitingOn)
	}
	return n
}

func TestDagSat1DoneArchiveSatisfies(t *testing.T) {
	for _, file := range []string{"20261013T014252Z-alpha-done.md", "2026-10-10T060037Z-alpha-done.md", "2026-10-09T05-24-17Z-alpha-done.md"} {
		t.Run(file, func(t *testing.T) {
			p := newProject(t)
			p.archive(file, "alpha", "phase: executing")
			p.plan("bravo", "phase: planned", "status: queued: 2026-10-22T00:00:00Z by /ip stage", "depends: alpha")
			g := p.graph()
			wantStatus(t, g, "bravo", StatusReady)
			if a := node(t, g, "alpha"); !a.Archived || a.Status != StatusDone {
				t.Fatalf("alpha = %+v, want an archived done node", a)
			}
		})
	}
}

func TestDagSat2LandedArchiveSatisfies(t *testing.T) {
	p := newProject(t)
	p.archive("20261013T014252Z-alpha-landed-rolled-to-charlie.md", "alpha")
	p.plan("charlie", "phase: planned", "status: blocked-on-operator: needs a token")
	p.plan("bravo", "phase: planned", "depends: alpha")
	wantStatus(t, p.graph(), "bravo", StatusReady)
}

func TestDagSat3RolledArchiveForwards(t *testing.T) {
	p := newProject(t)
	p.archive("20261013T014252Z-alpha-rolled-to-charlie.md", "alpha")
	p.plan("charlie", "phase: planned")
	p.plan("bravo", "phase: planned", "depends: alpha")
	g := p.graph()
	n := wantStatus(t, g, "bravo", StatusWaiting, "charlie")
	if n.HeldBy != "charlie" {
		t.Fatalf("held_by = %q, want charlie", n.HeldBy)
	}
	p.archive("20261014T014252Z-charlie-done.md", "charlie")
	os.Remove(filepath.Join(p.dir, ".claude", "iterate", "plans", "charlie.md"))
	wantStatus(t, p.graph(), "bravo", StatusReady)
}

func TestDagSat4ClosedArchiveHolds(t *testing.T) {
	p := newProject(t)
	p.archive("20261013T014252Z-alpha-closed.md", "alpha")
	p.plan("bravo", "phase: planned", "depends: alpha")
	g := p.graph()
	n := wantStatus(t, g, "bravo", StatusWaiting, "alpha")
	if !strings.Contains(n.Reason, "alpha (closed unfinished)") {
		t.Fatalf("reason = %q, want it to name alpha as closed unfinished", n.Reason)
	}
	if a := node(t, g, "alpha"); a.Status != StatusHeld {
		t.Fatalf("alpha status = %q, want held", a.Status)
	}
}

func TestDagSat5UnfinishedPredecessorNeverSatisfies(t *testing.T) {
	for _, keys := range [][]string{
		{"phase: planned"},
		{"phase: planned", "status: queued: 2026-10-22T00:00:00Z by /ip stage"},
		{"phase: executing"},
		{"phase: executing", "status: blocked-on-operator: token"},
		{"phase: executing", "status: paused: 2026-10-22T00:00:00Z at step 2/9"},
		{"phase: executing", "status: unblocked"},
	} {
		t.Run(strings.Join(keys, "+"), func(t *testing.T) {
			p := newProject(t)
			p.plan("alpha", keys...)
			p.plan("bravo", "phase: planned", "depends: alpha")
			wantStatus(t, p.graph(), "bravo", StatusWaiting, "alpha")
		})
	}
}

func TestDagSat6MilestoneIDResolves(t *testing.T) {
	p := newProject(t)
	p.archive("20261022T192922Z-gazelle-done.md", "gazelle", "milestone: M1-03")
	p.plan("binturong", "phase: planned", "milestone: M1-04", "depends: M1-03")
	p.plan("iiwi", "phase: planned", "milestone: M1-05", "depends: M1-03")
	p.plan("eider", "phase: planned", "milestone: M1-06", "depends: M1-04, M1-05")
	g := p.graph()
	wantStatus(t, g, "binturong", StatusReady)
	wantStatus(t, g, "iiwi", StatusReady)
	n := wantStatus(t, g, "eider", StatusWaiting, "binturong", "iiwi")
	if n.Depth != 2 {
		t.Fatalf("eider depth = %d, want 2", n.Depth)
	}
}

func TestDagSat7NoDependsLineAddsNothing(t *testing.T) {
	p := newProject(t)
	p.plan("alpha", "phase: planned")
	p.plan("bravo", "phase: planned")
	g := p.graph()
	for _, name := range []string{"alpha", "bravo"} {
		n := wantStatus(t, g, name, StatusReady)
		if n.Declared || len(n.Edges) != 0 || n.Depth != 0 {
			t.Fatalf("%s = %+v, want undeclared with no edges at depth 0", name, n)
		}
	}
}

func TestDagSat8JoinWaitsOnEveryEdge(t *testing.T) {
	p := newProject(t)
	p.archive("20261013T014252Z-alpha-done.md", "alpha")
	p.plan("delta", "phase: executing")
	p.plan("bravo", "phase: planned", "depends: alpha, delta")
	wantStatus(t, p.graph(), "bravo", StatusWaiting, "delta")
}

func TestDagSat9DependsNoneDeclaresNoEdges(t *testing.T) {
	p := newProject(t)
	p.plan("alpha", "phase: planned", "depends: none")
	n := wantStatus(t, p.graph(), "alpha", StatusReady)
	if !n.Declared || len(n.Edges) != 0 {
		t.Fatalf("alpha = %+v, want declared with no edges", n)
	}
}

func TestDagErr1UnknownDependency(t *testing.T) {
	p := newProject(t)
	p.plan("bravo", "phase: planned", "depends: ghost")
	g := p.graph()
	n := wantStatus(t, g, "bravo", StatusBlocked)
	if !slices.Contains(n.Errors, "unknown dependency ghost") || !slices.Contains(g.Errors, "unknown dependency ghost") {
		t.Fatalf("errors = %v / %v, want unknown dependency ghost", n.Errors, g.Errors)
	}
}

func TestDagErr2CycleNamesBothPlans(t *testing.T) {
	p := newProject(t)
	p.plan("alpha", "phase: planned", "depends: bravo")
	p.plan("bravo", "phase: planned", "depends: alpha")
	g := p.graph()
	for _, name := range []string{"alpha", "bravo"} {
		n := wantStatus(t, g, name, StatusBlocked, func() []string {
			if name == "alpha" {
				return []string{"bravo"}
			}
			return []string{"alpha"}
		}()...)
		if !strings.Contains(n.Reason, "cycle alpha → bravo → alpha") {
			t.Fatalf("%s reason = %q, want the cycle named", name, n.Reason)
		}
	}
}

func TestDagErr3SelfDependency(t *testing.T) {
	p := newProject(t)
	p.plan("alpha", "phase: planned", "depends: alpha")
	n := wantStatus(t, p.graph(), "alpha", StatusBlocked)
	if !slices.Contains(n.Errors, "alpha depends on itself") {
		t.Fatalf("errors = %v", n.Errors)
	}
}

func TestDagErr4AmbiguousMilestone(t *testing.T) {
	p := newProject(t)
	p.plan("alpha", "phase: planned", "milestone: M1-03")
	p.plan("charlie", "phase: planned", "milestone: M1-03")
	p.plan("bravo", "phase: planned", "depends: M1-03")
	n := wantStatus(t, p.graph(), "bravo", StatusBlocked)
	if !slices.Contains(n.Errors, "ambiguous milestone M1-03: carried by alpha and charlie") {
		t.Fatalf("errors = %v", n.Errors)
	}
}

func TestUnknownArchiveSuffixUsesTheNameKey(t *testing.T) {
	p := newProject(t)
	p.archive("20261013T014252Z-alpha-abandoned.md", "alpha")
	p.plan("bravo", "phase: planned", "depends: alpha")
	n := wantStatus(t, p.graph(), "bravo", StatusWaiting, "alpha")
	if !strings.Contains(n.Reason, "alpha (archived unfinished)") {
		t.Fatalf("reason = %q", n.Reason)
	}
}

func TestAPlanCanCarrySeveralMilestones(t *testing.T) {
	p := newProject(t)
	p.plan("genet", "phase: planned", "milestone: M1-06", "milestone: M1-07", "depends: M1-05")
	p.plan("alexandrite", "phase: planned", "milestone: M1-05")
	p.plan("hog", "phase: planned", "milestone: M1-08, M1-09", "depends: M1-06")
	p.plan("indri", "phase: planned", "milestone: M1-10", "depends: M1-09")
	g := p.graph()
	if n := node(t, g, "genet"); !slices.Equal(n.Milestones, []string{"M1-06", "M1-07"}) || n.Milestone != "M1-06" {
		t.Fatalf("genet milestones = %v / %q", n.Milestones, n.Milestone)
	}
	wantStatus(t, g, "hog", StatusWaiting, "genet")
	wantStatus(t, g, "indri", StatusWaiting, "hog")
	if len(g.Errors) != 0 {
		t.Fatalf("errors = %v", g.Errors)
	}
}

func TestImpliedEdgesStillGateButLeaveTheReason(t *testing.T) {
	p := newProject(t)
	p.plan("haddock", "phase: executing", "status: unblocked")
	p.plan("ibex", "phase: executing", "status: unblocked", "depends: haddock")
	p.plan("jabiru", "phase: planned", "depends: ibex, haddock")
	g := p.graph()
	n := wantStatus(t, g, "jabiru", StatusWaiting, "ibex", "haddock")
	if n.Reason != "waiting on ibex (unblocked)" {
		t.Fatalf("reason = %q", n.Reason)
	}
	implied := 0
	for _, e := range n.Edges {
		if e.Implied {
			implied++
			if e.Target != "haddock" {
				t.Fatalf("implied edge = %+v", e)
			}
		}
	}
	if implied != 1 {
		t.Fatalf("implied = %d", implied)
	}
	// Out of order: ibex finished first, so the implied predecessor is what holds.
	p.archive("20261023T000000Z-ibex-done.md", "ibex")
	os.Remove(filepath.Join(p.dir, ".claude", "iterate", "plans", "ibex.md"))
	n = wantStatus(t, p.graph(), "jabiru", StatusWaiting, "haddock")
	if n.Reason != "waiting on haddock (unblocked)" {
		t.Fatalf("reason = %q", n.Reason)
	}
}

func TestMilestoneCarriedThroughARollResolvesToTheSource(t *testing.T) {
	p := newProject(t)
	p.archive("20261013T014252Z-alpha-rolled-to-charlie.md", "alpha", "milestone: M1-03")
	p.plan("charlie", "phase: planned", "milestone: M1-03")
	p.plan("bravo", "phase: planned", "depends: M1-03")
	wantStatus(t, p.graph(), "bravo", StatusWaiting, "charlie")
}

func TestHeldByIsTheRootOfAChain(t *testing.T) {
	p := newProject(t)
	p.plan("caiman", "phase: executing", "status: blocked-on-operator: ssh")
	p.plan("dace", "phase: executing", "status: unblocked", "depends: caiman")
	p.plan("earthworm", "phase: executing", "status: unblocked", "depends: dace")
	p.plan("fairywren", "phase: executing", "status: unblocked", "depends: earthworm")
	g := p.graph()
	for _, name := range []string{"dace", "earthworm", "fairywren"} {
		if n := node(t, g, name); n.HeldBy != "caiman" {
			t.Fatalf("%s held_by = %q, want caiman", name, n.HeldBy)
		}
	}
	if n := node(t, g, "fairywren"); n.Depth != 3 {
		t.Fatalf("fairywren depth = %d, want 3", n.Depth)
	}
}

func TestReadyOrderIsDepthMilestoneStartedName(t *testing.T) {
	p := newProject(t)
	p.archive("20261013T014252Z-root-done.md", "root")
	p.plan("zeta", "phase: planned", "Started: 2026-10-01T00:00:00Z (planned)", "milestone: M1-10", "depends: root")
	p.plan("yak", "phase: planned", "Started: 2026-10-02T00:00:00Z (planned)", "milestone: M1-09", "depends: root")
	p.plan("xray", "phase: planned", "Started: 2026-09-01T00:00:00Z (planned)")
	p.plan("whiskey", "phase: planned", "Started: 2026-09-01T00:00:00Z (planned)")
	got := []string{}
	for _, n := range p.graph().Ready() {
		got = append(got, n.Name)
	}
	want := []string{"whiskey", "xray", "yak", "zeta"}
	if !slices.Equal(got, want) {
		t.Fatalf("ready order = %v, want %v", got, want)
	}
}

func TestDependsInTheBodyIsNotAKey(t *testing.T) {
	p := newProject(t)
	p.plan("alpha", "phase: planned")
	if n := node(t, p.graph(), "alpha"); n.Declared {
		t.Fatalf("a depends: line below the first heading was read as a key")
	}
}

func TestParseArchiveName(t *testing.T) {
	cases := []struct {
		file, name string
		outcome    Outcome
		rolledTo   string
		ok         bool
	}{
		{"20261013T014252Z-jackrabbit-done.md", "jackrabbit", OutcomeDone, "", true},
		{"2026-10-10T060037Z-hartebeest-done.md", "hartebeest", OutcomeDone, "", true},
		{"2026-10-09T05-24-17Z-baboon-done.md", "baboon", OutcomeDone, "", true},
		{"20261013T014252Z-alpha-closed.md", "alpha", OutcomeClosed, "", true},
		{"20261013T014252Z-alpha-rolled-to-quail2.md", "alpha", OutcomeRolled, "quail2", true},
		{"20261013T014252Z-alpha-landed-rolled-to-bravo.md", "alpha", OutcomeLanded, "bravo", true},
		{"20261013T014252Z-alpha-abandoned.md", "abandoned", OutcomeUnknown, "", true},
		{"20260812T012354Z.md", "", "", "", false},
		{"2026-08-12T01-23-54Z.md", "", "", "", false},
	}
	for _, c := range cases {
		name, outcome, rolledTo, ok := ParseArchiveName(c.file)
		if name != c.name || outcome != c.outcome || rolledTo != c.rolledTo || ok != c.ok {
			t.Errorf("%s = (%q, %q, %q, %v), want (%q, %q, %q, %v)", c.file, name, outcome, rolledTo, ok, c.name, c.outcome, c.rolledTo, c.ok)
		}
	}
}

func TestParseDepends(t *testing.T) {
	for in, want := range map[string][]string{
		"caiman":            {"caiman"},
		"M1-04, M1-05":      {"M1-04", "M1-05"},
		"`dace`; earthworm": {"dace", "earthworm"},
		"caiman, caiman":    {"caiman"},
		"none":              nil,
		"—":                 nil,
		"":                  nil,
	} {
		got, none := ParseDepends(in)
		if !slices.Equal(got, want) || none != (len(want) == 0) {
			t.Errorf("ParseDepends(%q) = %v, %v; want %v", in, got, none, want)
		}
	}
}

func TestMilestoneLessReadsNumbers(t *testing.T) {
	ordered := []string{"M1-01", "M1-09", "M1-10", "M2-01", "M10-01"}
	for i := 0; i+1 < len(ordered); i++ {
		if !MilestoneLess(ordered[i], ordered[i+1]) || MilestoneLess(ordered[i+1], ordered[i]) {
			t.Errorf("%s should sort before %s", ordered[i], ordered[i+1])
		}
	}
}
