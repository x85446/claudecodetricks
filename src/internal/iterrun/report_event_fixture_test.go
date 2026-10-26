package iterrun

import (
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/runrecord"
	"os"
	"testing"
	"time"
)

// Report fixtures supply public record events; they do not run an executor.
type recordFixtureAssignment struct {
	ID, RunID, ProjectID, ProjectPath, Plan, Kind, Task, Validation, Team string
	Step                                                                  int
}
type recordFixtureResult struct {
	Outcome, Blocker string
	ValidationMet    bool
	Evidence         []string
}
type recordFixtureUsage struct {
	Kind, SessionID, ProviderID string
	Sequence                    int64
	InputTokens, OutputTokens   int64
	Cumulative                  bool
}
type recordFixtureSource struct{}

func (recordFixtureSource) AppendPhase(phase string, a recordFixtureAssignment, result *recordFixtureResult, usage *recordFixtureUsage) {
	now := recordNow()
	var events []runrecord.Event
	add := func(kind, suffix string) *runrecord.Event {
		events = append(events, runrecord.Event{Type: kind, ID: runrecord.StableID("fixture", a.ID, suffix), At: now, Source: runrecord.SourceConductor, ProjectID: a.ProjectID, Requirement: a.Step, AttemptKey: a.ID, AssignmentID: a.ID, ConductorRunID: a.RunID, Team: a.Team})
		return &events[len(events)-1]
	}
	owner := func() *runrecord.Ownership {
		return &runrecord.Ownership{OwnerKind: "conductor", OwnerID: a.ID, PID: os.Getpid()}
	}
	switch phase {
	case "admitted":
		add(runrecord.TypeLifecycle, "admitted").Lifecycle = &runrecord.Lifecycle{State: "admitted", ObservedVia: "lease"}
		add(runrecord.TypeOwnershipAcquired, "acquired").Ownership = owner()
	case "started":
		add(runrecord.TypeAttemptStarted, "started").AttemptInfo = &runrecord.Attempt{Kind: a.Kind}
		add(runrecord.TypeLifecycle, "executing").Lifecycle = &runrecord.Lifecycle{State: "executing", ObservedVia: "lease"}
	case "event":
		if usage == nil {
			return
		}
		add(runrecord.TypeOwnershipBeat, "beat").Ownership = owner()
		event := add(runrecord.TypeUsage, "usage")
		event.SessionID = usage.SessionID
		event.Usage = &runrecord.Usage{RequestID: a.ID + ":" + usage.SessionID, Scope: runrecord.ScopeExact, InputTokens: &usage.InputTokens, OutputTokens: &usage.OutputTokens, Cumulative: usage.Cumulative}
	case "finished":
		if result == nil {
			return
		}
		outcome := result.Outcome
		if outcome == "completed" {
			outcome = "passed"
		}
		add(runrecord.TypeAttemptFinished, "finished").AttemptInfo = &runrecord.Attempt{Kind: a.Kind, Outcome: outcome, Reason: result.Blocker}
		for i, text := range result.Evidence {
			verdict := "unknown"
			if result.Outcome == "completed" && result.ValidationMet {
				verdict = "passed"
			}
			add(runrecord.TypeEvidence, fmt.Sprint("evidence", i)).Evidence = &runrecord.Evidence{Result: verdict, Summary: text, Imported: true, Revision: gitHead(a.ProjectPath)}
		}
		if result.Outcome == "completed" {
			checked := true
			add(runrecord.TypeRequirement, "requirement").RequirementState = &runrecord.RequirementState{Task: a.Task, Validation: a.Validation, TaskChecked: &checked, ValidationChecked: &checked}
		}
		if result.Outcome == "interrupted" {
			add(runrecord.TypeLifecycle, "interrupted").Lifecycle = &runrecord.Lifecycle{State: "interrupted", Reason: result.Blocker, ObservedVia: "lease"}
		}
		add(runrecord.TypeOwnershipReleased, "released").Ownership = owner()
	}
	marker := ""
	if p, e := GetPlanSummary(a.ProjectPath, a.Plan); e == nil {
		marker = recordMarker(p)
	}
	_, _, _ = RecordStore().Observe(runrecord.OpenRequest{Key: runrecord.PlanKey{ProjectPath: a.ProjectPath, ProjectID: a.ProjectID, Plan: a.Plan}, Marker: marker, Source: runrecord.SourceConductor, At: now, Versions: RecordPlanVersions(a.ProjectPath, a.Plan), AttemptKey: a.ID}, func(runrecord.Run) []runrecord.Event { return events })
}
func appendAdmissionFixture(t *testing.T, at time.Time, project, plan, decision, gate, reason string) {
	t.Helper()
	runs, e := RecordStore().Runs(project, plan)
	if e != nil || len(runs) == 0 {
		t.Fatal(runs, e)
	}
	run := runs[len(runs)-1]
	if n := len(run.Admissions); n > 0 {
		last := run.Admissions[n-1]
		if last.Decision == decision && last.Gate == gate && last.Reason == reason {
			return
		}
	}
	_, e = RecordStore().Append(runrecord.Event{RunID: run.RunID, ProjectPath: project, ProjectKey: run.ProjectKey, Plan: plan, Type: runrecord.TypeAdmission, ID: runrecord.StableID("fixture-admission", run.RunID, decision, gate, reason), At: at, Source: runrecord.SourceConductor, Admission: &runrecord.Admission{Decision: decision, Gate: gate, Reason: reason}})
	if e != nil {
		t.Fatal(e)
	}
}
