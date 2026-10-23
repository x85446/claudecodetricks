package conductor

import (
	"context"
	"fmt"
	"strings"
	"testing"
)

func waitingOn(name, pred string, keys ...string) Plan {
	p := Plan{Name: name, Staged: true, State: "queued", Waiting: true, WaitingOn: []string{pred}, WaitReason: "waiting on " + pred, HeldBy: pred, Depth: 1}
	for _, k := range keys {
		switch k {
		case "executing":
			p.State = "executing"
		case "unblocked":
			p.State = "unblocked"
		}
	}
	return p
}

func candidateCodes(d Decision) map[string]string {
	out := map[string]string{}
	for _, x := range d.Candidates {
		out[x.Plan.Name] = x.Code + ": " + x.Reason
	}
	return out
}

func queuePlan(t *testing.T, d Decision, name string) Plan {
	t.Helper()
	for _, q := range d.Queues {
		for _, p := range q.Plans {
			if p.Name == name {
				return p
			}
		}
	}
	t.Fatalf("plan %s missing from queue %+v", name, d.Queues)
	return Plan{}
}

// TESTMASTER: id=dag-adm-1 tier=fast parallel=yes
func TestDagAdm1WaitingPlanIsDeniedNamingItsPredecessor(t *testing.T) {
	s, src, _ := fixture(t, "")
	src.plans = []Plan{waitingOn("bravo", "alpha")}
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment != nil || src.checkpoints != 0 {
		t.Fatalf("a waiting plan was admitted: %+v %v", out, e)
	}
	if got := candidateCodes(out.Decision)["bravo"]; got != "plan_waiting: waiting on alpha" {
		t.Fatalf("bravo candidate = %q", got)
	}
	if p := queuePlan(t, out.Decision, "bravo"); p.WaitReason != "waiting on alpha" || !p.Waiting {
		t.Fatalf("queue lost the wait reason: %+v", p)
	}
}

// TESTMASTER: id=dag-adm-2 tier=fast parallel=yes
func TestDagAdm2ParkedHeadHoldsTheWholeChain(t *testing.T) {
	s, src, _ := fixture(t, "")
	src.plans = []Plan{{Name: "alpha", Staged: true, State: "blocked", Blocked: true, Reason: "blocked-on-operator: ssh"}, waitingOn("bravo", "alpha"), waitingOn("charlie", "bravo")}
	src.plans[2].HeldBy = "alpha"
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment != nil {
		t.Fatalf("a held plan launched: %+v %v", out, e)
	}
	for _, name := range []string{"bravo", "charlie"} {
		if p := queuePlan(t, out.Decision, name); p.HeldBy != "alpha" {
			t.Fatalf("%s held_by = %q", name, p.HeldBy)
		}
	}
}

// TESTMASTER: id=dag-adm-3 tier=fast parallel=yes
func TestDagAdm3SiblingsOfADonePredecessorBothRunInGraphOrder(t *testing.T) {
	s, src, _ := fixture(t, "")
	src.plans = []Plan{{Name: "iiwi", Staged: true, State: "queued", Depth: 1, Milestone: "M1-05"}, {Name: "binturong", Staged: true, State: "queued", Depth: 1, Milestone: "M1-04"}, {Name: "root", Staged: true, State: "queued", Depth: 0}}
	d, e := s.Status(context.Background())
	if e != nil {
		t.Fatal(e)
	}
	order := []string{}
	for _, p := range d.Queues[0].Plans {
		order = append(order, p.Name)
	}
	if strings.Join(order, ",") != "root,binturong,iiwi" {
		t.Fatalf("queue order = %v, want depth then milestone", order)
	}
	codes := candidateCodes(d)
	for _, name := range []string{"binturong", "iiwi"} {
		if !strings.HasPrefix(codes[name], "eligible") {
			t.Fatalf("%s = %q, want eligible", name, codes[name])
		}
	}
}

// TESTMASTER: id=dag-adm-4 tier=fast parallel=yes
func TestDagAdm4IndependentWorkRunsPastAHeldBranch(t *testing.T) {
	s, src, _ := fixture(t, "")
	src.plans = []Plan{{Name: "alpha", Staged: true, State: "blocked", Blocked: true, Reason: "blocked"}, waitingOn("bravo", "alpha"), {Name: "delta", Staged: true, State: "queued"}}
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment == nil || out.Assignment.Plan != "delta" {
		t.Fatalf("independent plan was not admitted: %+v %v", out.Assignment, e)
	}
}

// TESTMASTER: id=dag-adm-5 tier=fast parallel=yes
func TestDagAdm5ALandedPredecessorReleasesTheWaitWithoutAReStage(t *testing.T) {
	s, src, _ := fixture(t, "")
	src.plans = []Plan{waitingOn("bravo", "alpha")}
	if out, e := s.Tick(context.Background()); e != nil || out.Assignment != nil {
		t.Fatalf("waiting plan admitted: %+v %v", out, e)
	}
	src.mu.Lock()
	src.plans = []Plan{{Name: "bravo", Staged: true, State: "queued", Depth: 1}}
	src.mu.Unlock()
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment == nil || out.Assignment.Plan != "bravo" {
		t.Fatalf("released plan was not admitted: %+v %v", out, e)
	}
}

// TESTMASTER: id=dag-adm-6 tier=fast parallel=yes
func TestDagAdm6TheGraphNeverGrantsApproval(t *testing.T) {
	s, src, _ := fixture(t, "")
	src.plans = []Plan{{Name: "bravo", State: "planned", Depth: 1}}
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment != nil || len(out.Decision.Candidates) != 0 {
		t.Fatalf("an unstaged plan with satisfied edges was considered: %+v %v", out.Decision.Candidates, e)
	}
}

// TESTMASTER: id=dag-adm-7 tier=fast parallel=yes
func TestDagAdm7ResumesAreGatedLikeStarts(t *testing.T) {
	s, src, _ := fixture(t, "")
	src.plans = []Plan{waitingOn("bravo", "alpha", "executing"), waitingOn("charlie", "alpha", "unblocked")}
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment != nil {
		t.Fatalf("a waiting executing/unblocked plan resumed: %+v %v", out.Assignment, e)
	}
	codes := candidateCodes(out.Decision)
	for _, name := range []string{"bravo", "charlie"} {
		if !strings.HasPrefix(codes[name], "plan_waiting") {
			t.Fatalf("%s = %q", name, codes[name])
		}
	}
}

// TESTMASTER: id=dag-adm-9 tier=fast parallel=yes
func TestDagAdm9ReleasePlansStayTypedOnly(t *testing.T) {
	s, src, _ := fixture(t, "")
	src.plans = []Plan{{Name: "release", Staged: true, State: "queued", Release: true}}
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment != nil || len(out.Decision.Candidates) != 0 {
		t.Fatalf("a release plan entered admission: %+v %v", out, e)
	}
}

// TESTMASTER: id=dag-adm-10 tier=fast parallel=yes
func TestDagAdm10AnOperatorHeldRootStandsTheWatchDownNamingIt(t *testing.T) {
	s, src, _ := fixture(t, "")
	src.plans = []Plan{{Name: "caiman", Staged: true, State: "blocked", Blocked: true, Reason: "blocked-on-operator: ssh"}, waitingOn("dace", "caiman"), waitingOn("earthworm", "dace")}
	src.plans[2].HeldBy = "caiman"
	if _, e := s.Tick(context.Background()); e != nil {
		t.Fatal(e)
	}
	r, e := s.Runtime()
	if e != nil {
		t.Fatal(e)
	}
	if w := r.Watches["project"]; w.State != "stood_down" {
		t.Fatalf("watch = %+v, want stood_down", w)
	}
	found := false
	for _, n := range r.Notices {
		if strings.Contains(n.Message, "caiman holds 2 plans") {
			found = true
		}
	}
	if !found {
		t.Fatalf("no notice names the root: %+v", r.Notices)
	}

	s, src, _ = fixture(t, "")
	src.plans = []Plan{{Name: "caiman", Running: true, State: "executing"}, waitingOn("dace", "caiman")}
	if _, e := s.Tick(context.Background()); e != nil {
		t.Fatal(e)
	}
	r, _ = s.Runtime()
	if w := r.Watches["project"]; w.State != "watching" {
		t.Fatalf("watch = %+v, want watching while the root runs", w)
	}
}

// TESTMASTER: id=dag-adm-11 tier=fast parallel=yes
func TestDagAdm11TheIzcrosCascadeCannotRecur(t *testing.T) {
	names := []string{"caiman", "dace", "earthworm", "fairywren", "gadwall", "haddock", "ibex", "jabiru"}
	t.Run("staged chain whose head parks", func(t *testing.T) {
		s, src, _ := fixture(t, "")
		src.plans = []Plan{{Name: names[0], Staged: true, State: "blocked", Blocked: true, Reason: "blocked-on-operator: owned assignment interrupted"}}
		for i := 1; i < len(names); i++ {
			p := waitingOn(names[i], names[i-1])
			p.HeldBy, p.Depth = names[0], i
			src.plans = append(src.plans, p)
		}
		for tick := 0; tick < 3; tick++ {
			out, e := s.Tick(context.Background())
			if e != nil || out.Assignment != nil {
				t.Fatalf("tick %d launched %+v (%v)", tick, out.Assignment, e)
			}
		}
		d, _ := s.Status(context.Background())
		held := 0
		for _, p := range d.Queues[0].Plans {
			if p.HeldBy == names[0] {
				held++
			}
		}
		if held != 7 {
			t.Fatalf("held = %d, want 7", held)
		}
		src.mu.Lock()
		src.plans = append(src.plans, Plan{Name: "independent", Staged: true, State: "queued"})
		src.mu.Unlock()
		out, e := s.Tick(context.Background())
		if e != nil || out.Assignment == nil || out.Assignment.Plan != "independent" {
			t.Fatalf("independent ninth plan was not claimed: %+v %v", out.Assignment, e)
		}
	})
	t.Run("dependents sit executing and unblocked with no owner", func(t *testing.T) {
		s, src, _ := fixture(t, "")
		src.plans = []Plan{{Name: names[0], Staged: true, State: "executing", Running: true}}
		for i := 1; i < len(names); i++ {
			p := waitingOn(names[i], names[i-1], "executing")
			p.State = "unblocked"
			p.HeldBy, p.Depth = names[0], i
			src.plans = append(src.plans, p)
		}
		out, e := s.Tick(context.Background())
		if e != nil || out.Assignment != nil {
			t.Fatalf("a parked dependent resumed: %+v %v", out.Assignment, e)
		}
		for _, x := range out.Decision.Candidates {
			if x.Eligible {
				t.Fatalf("%s eligible while waiting", x.Plan.Name)
			}
		}
	})
}

// TESTMASTER: id=dag-adm-12 tier=fast parallel=yes
func TestDagAdm12ALiveRunIsNeverInterrupted(t *testing.T) {
	s, src, _ := fixture(t, "")
	live := waitingOn("bravo", "alpha", "executing")
	live.Running = true
	src.plans = []Plan{live}
	out, e := s.Tick(context.Background())
	if e != nil || out.Assignment != nil {
		t.Fatalf("%+v %v", out, e)
	}
	q := out.Decision.Queues[0]
	if q.RunningOwners["bravo"] != "cli" || len(out.Decision.Candidates) != 0 {
		t.Fatalf("live run lost its owner or was reconsidered: %+v %+v", q.RunningOwners, out.Decision.Candidates)
	}
	if fmt.Sprint(q.RunningPlans) != "[bravo]" {
		t.Fatalf("running plans = %v", q.RunningPlans)
	}
}
