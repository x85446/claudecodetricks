package plangraph

import "testing"

func liveOrder(g Graph) []string {
	out := []string{}
	for _, n := range g.Live() {
		out = append(out, n.Name)
	}
	return out
}

// TESTMASTER: id=letters-1 tier=? parallel=yes
func TestLettersOrderFollowsDepends(t *testing.T) {
	p := newProject(t)
	p.plan("cobra", "phase: planned", "depends: bison", "Started: 2026-01-01T00:00:00Z")
	p.plan("bison", "phase: planned", "depends: aphid", "Started: 2026-01-02T00:00:00Z")
	p.plan("aphid", "phase: planned", "depends: none", "Started: 2026-01-03T00:00:00Z")
	g := p.graph()
	got := liveOrder(g)
	want := []string{"aphid", "bison", "cobra"}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
		if n := node(t, g, want[i]); n.Position != i+1 {
			t.Fatalf("%s position = %d, want %d", n.Name, n.Position, i+1)
		}
	}
}

// TESTMASTER: id=letters-2 tier=? parallel=yes
func TestLettersIndependentPlansOrderByStarted(t *testing.T) {
	p := newProject(t)
	p.plan("zebu", "phase: planned", "depends: none", "Started: 2026-01-01T00:00:00Z")
	p.plan("adder", "phase: planned", "depends: none", "Started: 2026-01-02T00:00:00Z")
	g := p.graph()
	if got := liveOrder(g); got[0] != "zebu" || got[1] != "adder" {
		t.Fatalf("order = %v, want [zebu adder]", got)
	}
	for _, name := range []string{"zebu", "adder"} {
		wantStatus(t, g, name, StatusReady)
	}
}

// TESTMASTER: id=letters-3 tier=? parallel=yes
func TestLettersTeamedIsUpperCase(t *testing.T) {
	p := newProject(t)
	p.plan("falcon", "phase: complete", "teamed: true", "depends: none")
	n := node(t, p.graph(), "falcon")
	if n.Letter != "F" || !n.Teamed {
		t.Fatalf("falcon letter=%q teamed=%v, want F true", n.Letter, n.Teamed)
	}
}

// TESTMASTER: id=letters-4 tier=? parallel=yes
func TestLettersFlatIsLowerCase(t *testing.T) {
	p := newProject(t)
	p.plan("jackdaw", "phase: executing", "status: blocked-on-operator: x", "teamed: false", "depends: none")
	p.plan("mackerel", "phase: planned", "depends: none")
	g := p.graph()
	for name, want := range map[string]string{"jackdaw": "j", "mackerel": "m"} {
		if n := node(t, g, name); n.Letter != want || n.Teamed {
			t.Fatalf("%s letter=%q teamed=%v, want %s false", name, n.Letter, n.Teamed, want)
		}
	}
}

// TESTMASTER: id=letters-10 tier=? parallel=yes
func TestLettersArchivedPredecessorHasNoLetter(t *testing.T) {
	p := newProject(t)
	p.archive("20260101T000000Z-otter-done.md", "otter", "phase: complete", "teamed: true")
	p.plan("panda", "phase: planned", "depends: otter")
	g := p.graph()
	wantStatus(t, g, "panda", StatusReady)
	if a := node(t, g, "otter"); a.Letter != "" {
		t.Fatalf("archived otter letter = %q, want empty", a.Letter)
	}
	if n := node(t, g, "panda"); n.Letter != "p" || n.Position != 1 {
		t.Fatalf("panda = %+v", n)
	}
}

func TestLetterOfEmptyName(t *testing.T) {
	if Letter("", true) != "" {
		t.Fatal("empty name has a letter")
	}
}
