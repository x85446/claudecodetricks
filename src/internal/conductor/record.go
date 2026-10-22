package conductor

import (
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

// Assignment phases a RecordSource observes.
const (
	RecordAdmitted = "admitted" // lease claimed and plan marker acquired
	RecordStarted  = "started"  // owned child process started
	RecordEvent    = "event"    // a provider event was durably accepted
	RecordFinished = "finished" // the validated result was persisted
)

// RecordSource is an optional PlanSource extension that mirrors assignment
// lifecycle into iterate-run's durable run record. The Conductor's leases,
// checkpoints and events stay authoritative: a record write never gates
// admission, alters a result or fails an assignment.
type RecordSource interface {
	RecordAssignment(phase string, a model.Assignment, result *model.Result, event *model.Event)
}

func (s *Service) record(phase string, a model.Assignment, result *model.Result, event *model.Event) {
	if r, ok := s.Source.(RecordSource); ok {
		r.RecordAssignment(phase, a, result, event)
	}
}
