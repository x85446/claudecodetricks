package iterrun

import (
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// lcAttempt drives one Conductor assignment through the record adapter:
// admitted and started at start, usage observed, finished at end.
func lcAttempt(t *testing.T, clock *lcClock, project, plan string, step int, id string, start, end time.Time, in, out int64, result model.Result) model.Assignment {
	t.Helper()
	src := ConductorPlanSource{}
	a := model.Assignment{ID: id, RunID: "run-" + id, ProjectID: "p1", ProjectPath: project, Plan: plan, Step: step, Kind: "pair", Task: "step task", Validation: "step check"}
	clock.set(start)
	src.RecordAssignment(conductor.RecordAdmitted, a, nil, nil)
	src.RecordAssignment(conductor.RecordStarted, a, nil, nil)
	if in > 0 || out > 0 {
		clock.set(start.Add(end.Sub(start) / 2))
		src.RecordAssignment(conductor.RecordEvent, a, nil, &model.Event{Kind: "usage", SessionID: "c-" + id, Sequence: 1, InputTokens: in, OutputTokens: out, Cumulative: true})
	}
	clock.set(end)
	src.RecordAssignment(conductor.RecordFinished, a, &result, nil)
	return a
}

func lcRequirement(s ReworkSection, n int) ReworkRequirement {
	for _, r := range s.Rows {
		if r.Number == n {
			return r
		}
	}
	return ReworkRequirement{}
}

// TESTMASTER: id=kr-rework-retries tier=fast parallel=no
func TestKrReworkRetries(t *testing.T) {
	lcEnv(t)
	project := lcProject(t, "ibis")
	t0 := time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC)
	clock := lcUseClock(t, t0)
	writeRecordPlan(t, project, "ibis", lcExec)

	failed := model.Result{Outcome: "failed", Blocker: "validation failed: 2 cases red", Evidence: []string{"2 of 14 cases failed"}}
	a1 := lcAttempt(t, clock, project, "ibis", 1, "assignment-a1", t0, t0.Add(5*time.Minute), 1000, 100, failed)
	// Duplicate observations: the same finish and start replayed through
	// the adapter, and a second writer's start for the same attempt.
	clock.set(t0.Add(5*time.Minute + 30*time.Second))
	ConductorPlanSource{}.RecordAssignment(conductor.RecordFinished, a1, &failed, nil)
	ConductorPlanSource{}.RecordAssignment(conductor.RecordStarted, a1, nil, nil)
	runs, _ := RecordStore().Runs(project, "ibis")
	lcAppend(t, project, "ibis", runs[0].RunID, runrecord.Event{Type: runrecord.TypeAttemptStarted, Source: runrecord.SourceConductor, At: t0.Add(time.Second), Requirement: 1, AttemptKey: a1.ID, AssignmentID: a1.ID, AttemptInfo: &runrecord.Attempt{Kind: "pair"}})
	lcAttempt(t, clock, project, "ibis", 1, "assignment-a2", t0.Add(6*time.Minute), t0.Add(10*time.Minute), 2000, 200, model.Result{Outcome: "completed", ValidationMet: true, Evidence: []string{"14 of 14 cases pass"}})

	rep, page := lcReport(t, project, "ibis", "", false, t0.Add(11*time.Minute))
	s := rep.Rework
	if s.Status != sectionAvailable || s.Attempts != 2 || s.Requirements != 1 || s.Retries != 1 || s.Unsuccessful != 1 || s.Restarts != 0 || s.Reopened != 0 {
		t.Fatalf("counts: %+v", s)
	}
	if s.Recovered != (ReworkRatio{Count: 1, Of: 1}) || s.FirstPass != (ReworkRatio{Count: 0, Of: 1}) || s.RepeatedValidations != 1 {
		t.Fatalf("ratios: first %+v recovered %+v repeated %d", s.FirstPass, s.Recovered, s.RepeatedValidations)
	}
	rc, uc := s.RecoveryCost, s.UnsuccessfulCost
	if rc.Attempts != 1 || rc.DurationMS == nil || *rc.DurationMS != lcMin(4) || rc.Tokens == nil || *rc.Tokens != 2200 || rc.DurationUnknown+rc.TokensUnknown != 0 {
		t.Fatalf("recovery cost: %+v", rc)
	}
	if uc.Attempts != 1 || *uc.DurationMS != lcMin(5) || *uc.Tokens != 1100 {
		t.Fatalf("unsuccessful cost: %+v", uc)
	}
	row := lcRequirement(s, 1)
	if len(row.Attempts) != 2 || row.Attempts[0].Outcome != "failed" || row.Attempts[0].Recovery || row.Attempts[1].Outcome != "passed" || !row.Attempts[1].Recovery || !row.Recovered {
		t.Fatalf("requirement 1: %+v", row)
	}
	sec := lcSection(page, "report-rework")
	for _, frag := range []string{`aria-labelledby="report-rework-h"`, `data-outcome="failed"`, "1 of 1 requirement (100%)", "0 of 1 requirement (0%)", "2,200 tokens", "Definitions"} {
		if !strings.Contains(sec, frag) {
			t.Fatalf("rework HTML lacks %q", frag)
		}
	}
	body := lcAPI(t, url.Values{"project": {project}, "plan": {"ibis"}})
	if lcPath(body, "report", "rework", "attempts") != float64(2) || lcPath(body, "report", "rework", "recovered", "count") != float64(1) || lcPath(body, "report", "rework", "retries") != float64(1) {
		t.Fatalf("API rework: %v", lcPath(body, "report", "rework"))
	}
}

// TESTMASTER: id=kr-rework-interrupted tier=fast parallel=no
func TestKrReworkInterrupted(t *testing.T) {
	lcEnv(t)
	project := lcProject(t, "plover")
	t0 := time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC)
	clock := lcUseClock(t, t0)
	m := func(n int) time.Time { return t0.Add(time.Duration(n) * time.Minute) }
	writeRecordPlan(t, project, "plover", lcExec)

	// Requirement 2: interrupted, then resumed to success.
	lcAttempt(t, clock, project, "plover", 2, "assignment-b1", m(0), m(10), 1000, 200, model.Result{Outcome: "interrupted", Blocker: "the operator paused the Conductor"})
	lcAttempt(t, clock, project, "plover", 2, "assignment-b2", m(15), m(20), 500, 100, model.Result{Outcome: "completed", ValidationMet: true, Evidence: []string{"build green"}})
	// Requirement 3: passed, reopened by a failure, passed again.
	lcAttempt(t, clock, project, "plover", 3, "assignment-c1", m(21), m(23), 0, 0, model.Result{Outcome: "completed", ValidationMet: true, Evidence: []string{"first check"}})
	lcAttempt(t, clock, project, "plover", 3, "assignment-c2", m(24), m(26), 0, 0, model.Result{Outcome: "failed", Blocker: "regression found"})
	lcAttempt(t, clock, project, "plover", 3, "assignment-c3", m(27), m(30), 0, 0, model.Result{Outcome: "completed", ValidationMet: true, Evidence: []string{"regression fixed"}})

	// The plan completes; the CLI hook records it.
	clock.set(m(31))
	writeRecordPlan(t, project, "plover", "phase: complete\nExecuting: 2026-01-07T09:00:00Z\nFinished: 2026-01-07T09:31:00Z\nstatus: completed\nharness: claude-code")
	lcHook(t, project, "s1")

	rep, page := lcReport(t, project, "plover", "", false, m(32))
	s := rep.Rework
	if s.Attempts != 5 || s.Requirements != 2 || s.Retries != 3 || s.Restarts != 1 || s.Reopened != 1 || s.Unsuccessful != 2 {
		t.Fatalf("counts: %+v", s)
	}
	if s.Outcomes["interrupted"] != 1 || s.Outcomes["passed"] != 3 || s.Outcomes["failed"] != 1 {
		t.Fatalf("outcomes: %+v", s.Outcomes)
	}
	r2, r3 := lcRequirement(s, 2), lcRequirement(s, 3)
	b1 := r2.Attempts[0]
	if b1.Outcome != "interrupted" || b1.DurationMS == nil || *b1.DurationMS != lcMin(10) || b1.Tokens == nil || *b1.Tokens != 1200 || b1.Recovery {
		t.Fatalf("the interrupted attempt lost its own record: %+v", b1)
	}
	if r2.FirstPass == nil || *r2.FirstPass || !r2.Recovered || r2.Reopened {
		t.Fatalf("requirement 2: %+v", r2)
	}
	if r3.FirstPass == nil || !*r3.FirstPass || !r3.Reopened || !r3.Recovered || r3.Attempts[0].Outcome != "passed" || r3.Attempts[1].Outcome != "failed" {
		t.Fatalf("requirement 3: %+v", r3)
	}
	if s.FirstPass != (ReworkRatio{Count: 1, Of: 2}) || s.Recovered != (ReworkRatio{Count: 2, Of: 2}) {
		t.Fatalf("ratios: %+v %+v", s.FirstPass, s.Recovered)
	}
	rc, uc := s.RecoveryCost, s.UnsuccessfulCost
	if rc.Attempts != 2 || *rc.DurationMS != lcMin(8) || *rc.Tokens != 600 || rc.TokensUnknown != 1 {
		t.Fatalf("recovery cost: %+v", rc)
	}
	if uc.Attempts != 2 || *uc.DurationMS != lcMin(12) || *uc.Tokens != 1200 || uc.TokensUnknown != 1 {
		t.Fatalf("unsuccessful cost: %+v", uc)
	}
	if rep.Time.Clock == nil || rep.Time.Clock.Open || rep.Time.Clock.EndBasis != "Finished: in the plan file" {
		t.Fatalf("completed clock: %+v", rep.Time.Clock)
	}
	sec := lcSection(page, "report-rework")
	for _, frag := range []string{`data-outcome="interrupted"`, `<span class="rw-tag">reopened</span>`, "usage unknown", "Restart"} {
		if !strings.Contains(sec, frag) {
			t.Fatalf("rework HTML lacks %q", frag)
		}
	}
}
