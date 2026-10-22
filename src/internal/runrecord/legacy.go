package runrecord

import (
	"fmt"
	"sort"
	"strconv"
	"time"
)

// Legacy sources a read-only adapter reconstructs history from when a plan
// has no record. Each event it produces carries provenance
// "legacy:<source>" and a stable id, so presenting it never writes and
// importing it twice is a no-op.
const (
	LegacyPlanFile      = "plan-file"
	LegacyHookLog       = "hook-events"
	LegacyConductorRuns = "conductor-runs"
)

// LegacyStep is one plan step as its file shows it.
type LegacyStep struct {
	Number            int
	Task, Validation  string
	TaskChecked       bool
	ValidationChecked bool
}

// LegacyEvidence is one prose evidence line copied from a plan file.
// Requirement is 0 when the line names no step.
type LegacyEvidence struct {
	Requirement int
	Text        string
}

// LegacyPlan is what a plan or archive file states.
type LegacyPlan struct {
	ProjectPath     string
	ProjectID       string
	Plan            string
	SourceFile      string
	ArchiveFile     string
	Phase           string
	State           string // the plan's state label: executing, paused, blocked, ...
	Status          string
	Harness         string
	ExecutorVersion string
	PlannerVersion  string
	Started         *time.Time
	Executing       *time.Time
	Finished        *time.Time
	ArchivedAt      *time.Time // parsed from the archive filename prefix
	ModTime         time.Time
	Steps           []LegacyStep
	Evidence        []LegacyEvidence
}

// LegacyHookEvent is one hook observation attributed to the plan.
type LegacyHookEvent struct {
	At        time.Time
	SessionID string
	AgentID   string
	Team      string
}

// LegacyAssignment is one Conductor assignment for the plan.
type LegacyAssignment struct {
	AssignmentID   string
	ConductorRunID string
	Kind           string
	Step           int
	Team           string
	Engine         string
	Model          string
	Started        time.Time
	Finished       *time.Time
	Outcome        string // conductor result outcome; "" when no result exists
	Blocker        string
	ValidationMet  bool
	Evidence       []string
	Usage          []LegacyUsage
}

// LegacyUsage is the last cumulative provider total for one session.
type LegacyUsage struct {
	SessionID    string
	At           time.Time
	InputTokens  int64
	OutputTokens int64
}

// LegacySources is everything the adapter reads.
type LegacySources struct {
	Plan LegacyPlan
	// HooksRead is false when the hook event log could not be read.
	HooksRead bool
	Hooks     []LegacyHookEvent
	// HookNote explains attribution limits of the hook log.
	HookNote string
	// ConductorRead is false when the Conductor store could not be read.
	ConductorRead bool
	Assignments   []LegacyAssignment
}

// LegacyMarker is the execution marker the adapter assigns.
func LegacyMarker(p LegacyPlan) string {
	switch {
	case p.ArchiveFile != "":
		return "archive:" + p.ArchiveFile
	case p.Executing != nil:
		return p.Executing.UTC().Format(time.RFC3339)
	}
	return "plan-file"
}

// LegacyRunID derives a stable run id for imported history.
func LegacyRunID(p LegacyPlan) string {
	return StableID("run-legacy", CanonicalProject(p.ProjectPath), p.Plan, LegacyMarker(p))
}

var conductorOutcomes = map[string]string{
	"completed": "passed", "retry": "retry", "blocked": "blocked",
	"partial": "partial", "interrupted": "interrupted",
}

// LegacyEvents builds the adapter's events. Times that the sources do not
// state are not invented: a state whose transition time is unknown is
// placed at the file's modification time and says so in its reason.
func LegacyEvents(src LegacySources) []Event {
	p := src.Plan
	run := LegacyRunID(p)
	base := Event{
		SchemaVersion: SchemaVersion, Source: SourceImport,
		ProjectPath: CanonicalProject(p.ProjectPath), ProjectID: p.ProjectID, Plan: p.Plan, RunID: run,
	}
	base.ProjectKey = ProjectKey(base.ProjectPath)
	var out []Event
	add := func(source string, e Event, idParts ...string) {
		x := base
		x.Type, x.At = e.Type, e.At.UTC()
		x.ID = StableID("legacy", append([]string{run, source, e.Type}, idParts...)...)
		x.Provenance = "legacy:" + source
		x.RecordedAt = x.At
		x.SessionID, x.AgentID, x.Team = e.SessionID, e.AgentID, e.Team
		x.Requirement, x.Attempt, x.AttemptKey = e.Requirement, e.Attempt, e.AttemptKey
		x.AssignmentID, x.ConductorRunID, x.Marker = e.AssignmentID, e.ConductorRunID, e.Marker
		x.Versions, x.Lifecycle, x.RequirementState = e.Versions, e.Lifecycle, e.RequirementState
		x.AttemptInfo, x.Evidence, x.Usage, x.Coverage = e.AttemptInfo, e.Evidence, e.Usage, e.Coverage
		x.Blocker, x.Ownership = e.Blocker, e.Ownership
		out = append(out, x)
	}

	opened := p.ModTime
	switch {
	case p.Executing != nil:
		opened = *p.Executing
	case p.Started != nil:
		opened = *p.Started
	}
	v := &Versions{ExecutorVersion: p.ExecutorVersion, PlannerVersion: p.PlannerVersion}
	if p.Harness == "claude-code" || p.Harness == "codex" {
		v.Engine = p.Harness
	}
	add(LegacyPlanFile, Event{Type: TypeRunOpened, At: opened, Marker: LegacyMarker(p), Versions: v}, "opened")
	add(LegacyPlanFile, Event{Type: TypeCoverage, At: opened, Coverage: &Coverage{Telemetry: "plan_file", Kind: "plan", Key: p.SourceFile, State: CoverageCaptured, Note: "plan file read; steps and prose evidence only"}}, "coverage")

	if p.Executing != nil {
		add(LegacyPlanFile, Event{Type: TypeLifecycle, At: *p.Executing, Lifecycle: &Lifecycle{State: "executing", ObservedVia: "plan-file", Reason: "Executing: field"}}, "executing")
	}
	state := ""
	switch p.State {
	case "executing", "paused", "blocked", "unblocked", "queued":
		state = p.State
	}
	if p.Phase == "complete" || p.Phase == "done" || p.Finished != nil {
		state = "completed"
	}
	if state != "" && !(state == "executing" && p.Executing != nil) {
		at, reason, via := p.ModTime, "transition time unknown; observed at the file modification time", "plan-file-mtime"
		if state == "completed" && p.Finished != nil {
			at, reason, via = *p.Finished, "Finished: field", "plan-file"
		}
		if p.Status != "" && state != "completed" {
			reason = p.Status + "; " + reason
		}
		add(LegacyPlanFile, Event{Type: TypeLifecycle, At: at, Lifecycle: &Lifecycle{State: state, Reason: reason, ObservedVia: via}}, "state", state)
	}
	if p.ArchiveFile != "" {
		at, reason := p.ModTime, "archive time unknown; observed at the file modification time"
		if p.ArchivedAt != nil {
			at, reason = *p.ArchivedAt, "archive filename timestamp"
		}
		add(LegacyPlanFile, Event{Type: TypeLifecycle, At: at, Lifecycle: &Lifecycle{State: "archived", ArchiveFile: p.ArchiveFile, Reason: reason, ObservedVia: "archive-file"}}, "archived")
	}
	for _, s := range p.Steps {
		tc, vc := s.TaskChecked, s.ValidationChecked
		add(LegacyPlanFile, Event{Type: TypeRequirement, At: opened, Requirement: s.Number, RequirementState: &RequirementState{Task: s.Task, Validation: s.Validation, TaskChecked: &tc, ValidationChecked: &vc}}, "step", strconv.Itoa(s.Number))
	}
	for i, ev := range p.Evidence {
		add(LegacyPlanFile, Event{Type: TypeEvidence, At: p.ModTime, Requirement: ev.Requirement, Evidence: &Evidence{Result: "unknown", Summary: ev.Text, Imported: true}}, "evidence", strconv.Itoa(i), ev.Text)
	}

	// Hook observations: sessions and agents, first and last sighting.
	if !src.HooksRead {
		add(LegacyHookLog, Event{Type: TypeCoverage, At: opened, Coverage: &Coverage{Telemetry: "hook", Kind: "session", State: CoverageUnavailable, Note: "hook event log unreadable"}}, "coverage")
	} else if len(src.Hooks) == 0 {
		add(LegacyHookLog, Event{Type: TypeCoverage, At: opened, Coverage: &Coverage{Telemetry: "hook", Kind: "session", State: CoverageMissing, Note: "no hook events attributed to this plan run"}}, "coverage")
	} else {
		hooks := append([]LegacyHookEvent(nil), src.Hooks...)
		sort.SliceStable(hooks, func(i, j int) bool { return hooks[i].At.Before(hooks[j].At) })
		type span struct{ first, last LegacyHookEvent }
		sessions, agents := map[string]*span{}, map[string]*span{}
		var sessionOrder, agentOrder []string
		for _, h := range hooks {
			if h.SessionID != "" {
				if s, ok := sessions[h.SessionID]; ok {
					s.last = h
				} else {
					sessions[h.SessionID] = &span{h, h}
					sessionOrder = append(sessionOrder, h.SessionID)
				}
			}
			if h.AgentID != "" {
				if s, ok := agents[h.AgentID]; ok {
					s.last = h
				} else {
					agents[h.AgentID] = &span{h, h}
					agentOrder = append(agentOrder, h.AgentID)
				}
			}
		}
		for _, id := range sessionOrder {
			s := sessions[id]
			add(LegacyHookLog, Event{Type: TypeSessionObserved, At: s.first.At, SessionID: id}, id, "first")
			add(LegacyHookLog, Event{Type: TypeSessionObserved, At: s.last.At, SessionID: id}, id, "last")
		}
		for _, id := range agentOrder {
			s := agents[id]
			add(LegacyHookLog, Event{Type: TypeAgentObserved, At: s.first.At, AgentID: id, Team: s.first.Team, SessionID: s.first.SessionID}, id, "first")
			add(LegacyHookLog, Event{Type: TypeAgentObserved, At: s.last.At, AgentID: id, Team: s.last.Team, SessionID: s.last.SessionID}, id, "last")
		}
		state := CoverageCaptured
		if src.HookNote != "" {
			state = CoveragePartial
		}
		add(LegacyHookLog, Event{Type: TypeCoverage, At: hooks[0].At, Coverage: &Coverage{Telemetry: "hook", Kind: "session", State: state, From: hooks[0].At, To: hooks[len(hooks)-1].At, Note: fmt.Sprintf("%d sessions, %d agents from tool hooks; %s", len(sessionOrder), len(agentOrder), src.HookNote)}}, "coverage")
	}

	// Conductor assignments become attempts.
	requests := 0
	if !src.ConductorRead {
		add(LegacyConductorRuns, Event{Type: TypeCoverage, At: opened, Coverage: &Coverage{Telemetry: "conductor_runs", Kind: "request", State: CoverageUnavailable, Note: "conductor store unreadable"}}, "coverage")
	} else {
		for _, a := range src.Assignments {
			key := a.AssignmentID
			info := &Attempt{Kind: a.Kind, Engine: a.Engine, Model: a.Model}
			add(LegacyConductorRuns, Event{Type: TypeAttemptStarted, At: a.Started, Requirement: a.Step, AttemptKey: key, AssignmentID: a.AssignmentID, ConductorRunID: a.ConductorRunID, Team: a.Team, AttemptInfo: info}, key, "started")
			if a.Outcome != "" {
				outcome := conductorOutcomes[a.Outcome]
				if outcome == "" {
					outcome = a.Outcome
				}
				at := a.Started
				if a.Finished != nil {
					at = *a.Finished
				}
				fin := &Attempt{Kind: a.Kind, Outcome: outcome, Reason: a.Blocker, Engine: a.Engine, Model: a.Model}
				if a.Finished == nil {
					fin.Reason = "finish time unknown; " + a.Blocker
				}
				add(LegacyConductorRuns, Event{Type: TypeAttemptFinished, At: at, Requirement: a.Step, AttemptKey: key, AssignmentID: a.AssignmentID, ConductorRunID: a.ConductorRunID, AttemptInfo: fin}, key, "finished")
				for i, text := range a.Evidence {
					result := "unknown"
					if a.Outcome == "completed" && a.ValidationMet {
						result = "passed"
					}
					add(LegacyConductorRuns, Event{Type: TypeEvidence, At: at, Requirement: a.Step, AttemptKey: key, AssignmentID: a.AssignmentID, Evidence: &Evidence{Result: result, Summary: text, Imported: true}}, key, "evidence", strconv.Itoa(i))
				}
			}
			for _, u := range a.Usage {
				in, outTok := u.InputTokens, u.OutputTokens
				requests++
				add(LegacyConductorRuns, Event{Type: TypeUsage, At: u.At, Requirement: a.Step, AttemptKey: key, SessionID: u.SessionID, AssignmentID: a.AssignmentID, Usage: &Usage{
					RequestID: a.AssignmentID + ":" + u.SessionID, Scope: ScopeExact, Cumulative: true, Model: a.Model,
					InputTokens: &in, OutputTokens: &outTok, Note: "conductor stream total; input includes cache read and creation tokens",
				}}, key, "usage", u.SessionID)
			}
		}
		state, note := CoverageCaptured, fmt.Sprintf("%d conductor assignments", len(src.Assignments))
		if len(src.Assignments) == 0 {
			state, note = CoverageMissing, "no conductor assignments recorded for this plan"
		}
		add(LegacyConductorRuns, Event{Type: TypeCoverage, At: opened, Coverage: &Coverage{Telemetry: "conductor_runs", Kind: "request", State: state, Note: note}}, "coverage")
	}
	if requests == 0 {
		add(LegacyConductorRuns, Event{Type: TypeCoverage, At: opened, Coverage: &Coverage{Telemetry: "usage", Kind: "request", State: CoverageUnavailable, Note: "no per-request usage in legacy sources"}}, "usage-coverage")
	}
	sortEvents(out)
	return out
}

// Legacy folds the adapter's events into a read-only view.
func Legacy(src LegacySources) Run {
	return Fold(LegacyEvents(src), fileRead{})
}
