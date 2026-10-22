// Package runrecord is the durable, versioned run record that iterate-run
// owns for both execution owners: the CLI (/iterate driven by hooks and
// `iterate-run record append`) and the Conductor supervisor. Every report,
// skill and web page reads the same record.
//
// A record is an append-only JSONL log per plan run. Each line is one Event
// carrying a globally unique ID, the full identity it belongs to and exactly
// one typed payload. Reading folds the log into a Run view deterministically:
// events sort by event time, then ID; a duplicate ID is a no-op; a torn
// trailing line from an interrupted write is ignored.
//
// Missing capture is unknown, never zero. Numeric observations are pointers
// so an absent value stays absent, and the folded view lists every unknown
// field by name.
package runrecord

import (
	"time"
)

// SchemaVersion is stamped on every event and on every folded view. A
// reader accepts events at or below this version and reports a newer one
// as unreadable rather than guessing at its meaning.
const SchemaVersion = 1

// Event types. The dotted prefix names the payload family.
const (
	TypeRunOpened         = "run.opened"           // Versions; Marker
	TypeLifecycle         = "run.lifecycle"        // Lifecycle
	TypeSessionObserved   = "session.observed"     // Versions (engine/provider/model when known)
	TypeAgentObserved     = "agent.observed"       // Team on the envelope
	TypeOwnershipAcquired = "ownership.acquired"   // Ownership
	TypeOwnershipBeat     = "ownership.heartbeat"  // Ownership
	TypeOwnershipReleased = "ownership.released"   // Ownership
	TypeBlockerDetected   = "blocker.detected"     // Blocker
	TypeBlockerCleared    = "blocker.cleared"      // Blocker
	TypeAdmission         = "admission.considered" // Admission
	TypeRequirement       = "requirement.observed" // RequirementState
	TypeAttemptStarted    = "attempt.started"      // Attempt
	TypeAttemptFinished   = "attempt.finished"     // Attempt
	TypeEvidence          = "evidence.observed"    // Evidence
	TypeUsage             = "usage.observed"       // Usage
	TypeInterval          = "interval.observed"    // Interval
	TypeDelivery          = "delivery.observed"    // Delivery
	TypeCoverage          = "coverage.observed"    // Coverage
)

// KnownTypes lists every event type this schema version defines.
var KnownTypes = []string{
	TypeRunOpened, TypeLifecycle, TypeSessionObserved, TypeAgentObserved,
	TypeOwnershipAcquired, TypeOwnershipBeat, TypeOwnershipReleased,
	TypeBlockerDetected, TypeBlockerCleared, TypeAdmission, TypeRequirement,
	TypeAttemptStarted, TypeAttemptFinished, TypeEvidence, TypeUsage,
	TypeInterval, TypeDelivery, TypeCoverage,
}

// Lifecycle states a run.lifecycle event may carry.
var LifecycleStates = []string{
	"queued", "admitted", "executing", "paused", "resumed", "interrupted",
	"blocked", "unblocked", "completed", "archived",
}

// Sources name which owner wrote an event.
const (
	SourceConductor = "conductor"
	SourceHook      = "cli:hook"
	SourceCLI       = "cli:record"
	SourceImport    = "import"
)

// Event is one durable record line.
type Event struct {
	SchemaVersion int       `json:"schema_version"`
	ID            string    `json:"id"`
	Type          string    `json:"type"`
	At            time.Time `json:"at"`
	RecordedAt    time.Time `json:"recorded_at"`
	// Source is the writer: conductor, cli:hook, cli:record or import.
	Source string `json:"source"`
	// Provenance is "observed" for a runtime observation, or
	// "legacy:<source>" for history reconstructed from a plan file, the
	// hook event log or conductor run events.
	Provenance string `json:"provenance"`

	ProjectPath  string `json:"project_path"`
	ProjectKey   string `json:"project_key"`
	ProjectID    string `json:"project_id,omitempty"`
	Plan         string `json:"plan"`
	RunID        string `json:"run_id"`
	SessionID    string `json:"session_id,omitempty"`
	AgentID      string `json:"agent_id,omitempty"`
	Team         string `json:"team,omitempty"`
	Requirement  int    `json:"requirement,omitempty"`
	Attempt      int    `json:"attempt,omitempty"`
	AttemptKey   string `json:"attempt_key,omitempty"`
	AssignmentID string `json:"assignment_id,omitempty"`
	// ConductorRunID is the Conductor's per-assignment run directory id,
	// distinct from RunID, which names one execution of the plan.
	ConductorRunID string `json:"conductor_run_id,omitempty"`

	// Marker identifies the plan execution a run.opened event begins: the
	// plan file's Executing: value, or the archive filename for imported
	// history.
	Marker string `json:"marker,omitempty"`

	Versions         *Versions         `json:"versions,omitempty"`
	Lifecycle        *Lifecycle        `json:"lifecycle,omitempty"`
	Ownership        *Ownership        `json:"ownership,omitempty"`
	Blocker          *Blocker          `json:"blocker,omitempty"`
	Admission        *Admission        `json:"admission,omitempty"`
	RequirementState *RequirementState `json:"requirement_state,omitempty"`
	AttemptInfo      *Attempt          `json:"attempt_info,omitempty"`
	Evidence         *Evidence         `json:"evidence,omitempty"`
	Usage            *Usage            `json:"usage,omitempty"`
	Interval         *Interval         `json:"interval,omitempty"`
	Delivery         *Delivery         `json:"delivery,omitempty"`
	Coverage         *Coverage         `json:"coverage,omitempty"`
}

// Versions names the software that produced a run, session or request.
// An empty field is unknown.
type Versions struct {
	Engine          string `json:"engine,omitempty"` // claude-code | codex
	EngineVersion   string `json:"engine_version,omitempty"`
	ExecutorVersion string `json:"executor_version,omitempty"`
	PlannerVersion  string `json:"planner_version,omitempty"`
	IterateRun      string `json:"iterate_run,omitempty"`
	Provider        string `json:"provider,omitempty"`
	Model           string `json:"model,omitempty"`
}

// Lifecycle is one plan state transition.
type Lifecycle struct {
	State       string `json:"state"`
	Reason      string `json:"reason,omitempty"`
	ArchiveFile string `json:"archive_file,omitempty"`
	// ObservedVia names how the transition was seen: "plan-file" when a
	// hook read the plan's phase/status, "lease" for a Conductor lease,
	// "explicit" for a direct append.
	ObservedVia string `json:"observed_via,omitempty"`
}

// Ownership is a lease observation by an owner.
type Ownership struct {
	OwnerKind string `json:"owner_kind"` // cli | conductor
	OwnerID   string `json:"owner_id"`
	PID       int    `json:"pid,omitempty"`
	Reason    string `json:"reason,omitempty"`
}

// Blocker is a detected or cleared obstacle.
type Blocker struct {
	Key    string `json:"key,omitempty"`
	Kind   string `json:"kind,omitempty"` // operator | quota | dependency | validation | unknown
	Reason string `json:"reason,omitempty"`
}

// Admission is one admission decision and the gate that decided it.
type Admission struct {
	Decision string `json:"decision"`       // admitted | gated
	Gate     string `json:"gate,omitempty"` // automation_disabled | schedule | budget | ownership | queue | other
	Reason   string `json:"reason,omitempty"`
}

// RequirementState is the observed state of one plan step's task and
// validation checkboxes. A checked box is not evidence.
type RequirementState struct {
	Task              string `json:"task,omitempty"`
	Validation        string `json:"validation,omitempty"`
	TaskChecked       *bool  `json:"task_checked,omitempty"`
	ValidationChecked *bool  `json:"validation_checked,omitempty"`
}

// Attempt is one try at a requirement.
type Attempt struct {
	Kind       string `json:"kind,omitempty"`    // pair | diagnosis | delivery | intake | cli
	Outcome    string `json:"outcome,omitempty"` // passed | failed | interrupted | retry | blocked | partial
	DurationMS *int64 `json:"duration_ms,omitempty"`
	Reason     string `json:"reason,omitempty"`
	Provider   string `json:"provider,omitempty"`
	Model      string `json:"model,omitempty"`
	Engine     string `json:"engine,omitempty"`
}

// Evidence is one validation observation for a requirement and attempt.
type Evidence struct {
	Result       string   `json:"result"` // passed | failed | unknown
	Command      string   `json:"command,omitempty"`
	ExitStatus   *int     `json:"exit_status,omitempty"`
	ArtifactPath string   `json:"artifact_path,omitempty"`
	ArtifactURL  string   `json:"artifact_url,omitempty"`
	Revision     string   `json:"revision,omitempty"`
	Environment  string   `json:"environment,omitempty"`
	CoveredFiles []string `json:"covered_files,omitempty"`
	Summary      string   `json:"summary,omitempty"`
	// Imported marks prose evidence copied from a plan file or result
	// rather than observed by running a command.
	Imported bool `json:"imported,omitempty"`
}

// Usage scopes say what a token or cost figure covers.
const (
	ScopeExact       = "exact"        // exact provider total for this request
	ScopeEstimated   = "estimated"    // estimated allocation of a larger total
	ScopeSessionCost = "session_cost" // whole-session cost, not plan-attributable
	ScopeAllowance   = "allowance"    // subscription allowance observation
)

// Usage is one request's token and cost observation. It is idempotent per
// (Scope, RequestID): a later observation of the same request replaces an
// earlier one, so retries and streaming rewrites never double count.
type Usage struct {
	RequestID        string   `json:"request_id"`
	Scope            string   `json:"scope"`
	Provider         string   `json:"provider,omitempty"`
	Model            string   `json:"model,omitempty"`
	Lane             string   `json:"lane,omitempty"`
	InputTokens      *int64   `json:"input_tokens,omitempty"`
	OutputTokens     *int64   `json:"output_tokens,omitempty"`
	ReasoningTokens  *int64   `json:"reasoning_tokens,omitempty"`
	CacheReadTokens  *int64   `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int64   `json:"cache_write_tokens,omitempty"`
	CostUSD          *float64 `json:"cost_usd,omitempty"`
	PriceSource      string   `json:"price_source,omitempty"`
	Note             string   `json:"note,omitempty"`
	// Cumulative marks a running total for the request id; the latest
	// observation is the total.
	Cumulative bool `json:"cumulative,omitempty"`
}

// Interval causes.
var IntervalCauses = []string{
	"execution", "model", "tool", "build", "queue", "blocked-operator",
	"schedule-pause", "unobserved",
}

// Interval is one observed span with a cause. Concurrent spans stay
// separate; a reader never sums them into wall time.
type Interval struct {
	Cause string    `json:"cause"`
	Start time.Time `json:"start"`
	End   time.Time `json:"end"`
	Lane  string    `json:"lane,omitempty"`
	Note  string    `json:"note,omitempty"`
}

// Delivery stages.
var DeliveryStages = []string{
	"source_commit", "pr", "review", "ci", "merge", "install", "deploy",
	"runtime_verification",
}

// Delivery is one delivery-stage observation.
type Delivery struct {
	Stage        string `json:"stage"`
	Result       string `json:"result,omitempty"` // passed | failed | pending | unknown
	Ref          string `json:"ref,omitempty"`    // PR number, tag, artifact id
	Revision     string `json:"revision,omitempty"`
	Target       string `json:"target,omitempty"`
	ArtifactPath string `json:"artifact_path,omitempty"`
	URL          string `json:"url,omitempty"`
	Evidence     string `json:"evidence,omitempty"`
}

// Coverage states.
const (
	CoverageCaptured    = "captured"
	CoverageMissing     = "missing"
	CoveragePartial     = "partial"
	CoverageUnavailable = "unavailable"
)

// Coverage records whether a telemetry source captured a session, agent
// or request stream, and over which interval.
type Coverage struct {
	Telemetry string    `json:"telemetry"` // hook | conductor_stream | transcript | plan_file | conductor_runs
	Kind      string    `json:"kind"`      // session | agent | request | plan
	Key       string    `json:"key,omitempty"`
	State     string    `json:"state"` // captured | missing | partial | unavailable
	From      time.Time `json:"from,omitempty"`
	To        time.Time `json:"to,omitempty"`
	Note      string    `json:"note,omitempty"`
}
