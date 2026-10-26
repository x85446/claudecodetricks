package iterrun

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

// TESTMASTER: id=nene-order-3 tier=? parallel=no
func TestNeneOrderLabelsAndStablePositions(t *testing.T) {
	p := t.TempDir()
	for i := 0; i < 16; i++ {
		depends := "none"
		if i > 0 {
			depends = fmt.Sprintf("animal%02d", i-1)
		}
		writeRecordPlan(t, p, fmt.Sprintf("animal%02d", i), fmt.Sprintf("phase: planned\ndepends: %s\nrelease: v%d\nposition: %d", depends, i/6+1, i+1))
	}
	v, e := BuildGraphView(p)
	if e != nil || len(v.Errors) > 0 {
		t.Fatalf("graph: %+v %v", v, e)
	}
	if v.Next != "animal00" {
		t.Fatalf("next=%s", v.Next)
	}
	for i, n := range v.Nodes {
		if n.Position != i+1 || n.Letter != "a" || n.Release == "" {
			t.Fatalf("node %d: %+v", i, n)
		}
		if i > 0 && n.WaitingReason == "" {
			t.Fatalf("missing waiting reason: %+v", n)
		}
	}
	writeRecordPlan(t, p, "animal00", "phase: executing\nstatus: blocked-on-operator\ndepends: none\nrelease: v1\nposition: 1")
	changed, e := BuildGraphView(p)
	if e != nil {
		t.Fatal(e)
	}
	for i, n := range changed.Nodes {
		if n.Position != v.Nodes[i].Position || n.Release != v.Nodes[i].Release {
			t.Fatalf("status changed order: %+v", n)
		}
	}
}

// TESTMASTER: id=nene-order-7 tier=? parallel=no
func TestNeneOrderRefusesCollisionOverflowAndDependencyReverse(t *testing.T) {
	p := ownerFixture(t)
	writeRecordPlan(t, p, "beta", "phase: planned\ndepends: alpha")
	before, _ := os.ReadFile(p + "/.claude/iterate/plans/beta.md")
	if e := SetPlanOrder(p, []string{"beta", "alpha"}); e == nil {
		t.Fatal("reversed prerequisite accepted")
	}
	after, _ := os.ReadFile(p + "/.claude/iterate/plans/beta.md")
	if string(before) != string(after) {
		t.Fatal("denied ordering changed plan")
	}
	if e := SetPlanOrder(p, []string{"alpha", "beta"}); e != nil {
		t.Fatal(e)
	}
	writeRecordPlan(t, p, "beta", "phase: planned\ndepends: alpha\nposition: 1\nlabel: A")
	v, _ := BuildGraphView(p)
	if len(v.Errors) == 0 {
		t.Fatal("duplicate position accepted")
	}
	if strings.Contains(strings.Join(v.Errors, " "), "label") {
		t.Fatalf("a label line is an error: %+v", v.Errors)
	}
}

// TESTMASTER: id=letters-6 tier=? parallel=no
func TestLettersNoPlanCap(t *testing.T) {
	p := t.TempDir()
	for i := 0; i < 27; i++ {
		writeRecordPlan(t, p, fmt.Sprintf("plan%02d", i), "phase: planned\ndepends: none")
	}
	v, _ := BuildGraphView(p)
	if len(v.Errors) > 0 {
		t.Fatalf("27 plans: %+v", v.Errors)
	}
	for _, n := range v.Nodes {
		if n.Letter != "p" {
			t.Fatalf("letter %q for %s", n.Letter, n.Name)
		}
	}
}

// TESTMASTER: id=letters-5 tier=? parallel=no
func TestLettersPlanOrderSetStripsLabel(t *testing.T) {
	p := ownerFixture(t)
	writeRecordPlan(t, p, "beta", "phase: planned\ndepends: alpha\nlabel: B\nposition: 2")
	v, _ := BuildGraphView(p)
	if len(v.Errors) > 0 {
		t.Fatalf("label line reported: %+v", v.Errors)
	}
	if e := SetPlanOrder(p, []string{"alpha", "beta"}); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(p + "/.claude/iterate/plans/beta.md")
	if strings.Contains(string(b), "label:") || !strings.Contains(string(b), "position: 2") {
		t.Fatalf("plan after order set:\n%s", b)
	}
}

// TESTMASTER: id=nene-order-5 tier=? parallel=no
func TestNeneOrderUnknownAndCyclicClaimsRefused(t *testing.T) {
	for _, dependency := range []string{"ghost", "beta"} {
		p := ownerFixture(t)
		writeRecordPlan(t, p, "alpha", "phase: planned\ndepends: "+dependency)
		writeRecordPlan(t, p, "beta", "phase: planned\ndepends: alpha")
		if _, e := ClaimPlan(p, "alpha", "cli", os.Getpid(), time.Now()); e == nil {
			t.Fatalf("accepted dependency %s", dependency)
		}
	}
}
