package runrecord

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Run is the folded, read-only view of one plan run. Lists are in event
// order. A field that no event established is listed in Unknown.
type Run struct {
	SchemaVersion int    `json:"schema_version"`
	ProjectPath   string `json:"project_path"`
	ProjectKey    string `json:"project_key"`
	ProjectID     string `json:"project_id,omitempty"`
	Plan          string `json:"plan"`
	RunID         string `json:"run_id"`
	// Provenance is "observed" when every event was a runtime
	// observation, "legacy:<sources>" when every event was reconstructed
	// from legacy sources, and "mixed" otherwise.
	Provenance string     `json:"provenance"`
	Sources    []string   `json:"sources"`
	Marker     string     `json:"marker,omitempty"`
	OpenedAt   *time.Time `json:"opened_at,omitempty"`
	LastAt     *time.Time `json:"last_event_at,omitempty"`
	// State is the latest lifecycle state, or "unknown".
	State       string `json:"state"`
	Archived    bool   `json:"archived"`
	ArchiveFile string `json:"archive_file,omitempty"`
	// Superseded marks a run left open when a later run of the same
	// codename began without an archive observation.
	Superseded bool `json:"superseded,omitempty"`

	Versions     Versions          `json:"versions"`
	Lifecycle    []LifecycleEntry  `json:"lifecycle"`
	Owner        *OwnerView        `json:"owner,omitempty"`
	Ownership    []OwnershipEntry  `json:"ownership"`
	Blockers     []BlockerView     `json:"blockers"`
	Admissions   []AdmissionEntry  `json:"admissions"`
	Sessions     []SessionView     `json:"sessions"`
	Agents       []AgentView       `json:"agents"`
	Requirements []RequirementView `json:"requirements"`
	Usage        UsageView         `json:"usage"`
	Intervals    []IntervalEntry   `json:"intervals"`
	Deliveries   []DeliveryEntry   `json:"deliveries"`
	Coverage     []CoverageEntry   `json:"coverage"`
	Unknown      []string          `json:"unknown"`

	Events int `json:"events"`
	// Duplicates counts lines whose id an earlier line already carried.
	Duplicates int `json:"duplicates"`
	// Torn reports a trailing line from an interrupted write, ignored.
	Torn bool `json:"torn"`
	// Unreadable counts other lines skipped as unparseable or newer-schema.
	Unreadable int `json:"unreadable"`

	firstAt     time.Time
	attemptKeys map[string]bool
}

func (r Run) hasAttemptKey(key string) bool { return r.attemptKeys[key] }

// Meta is the provenance every entry carries.
type Meta struct {
	EventID    string    `json:"event_id"`
	At         time.Time `json:"at"`
	Source     string    `json:"source"`
	Provenance string    `json:"provenance"`
}

func meta(e Event) Meta {
	return Meta{EventID: e.ID, At: e.At, Source: e.Source, Provenance: e.Provenance}
}

type LifecycleEntry struct {
	Meta
	Lifecycle
}

type OwnershipEntry struct {
	Meta
	Event string `json:"event"` // acquired | heartbeat | released
	Ownership
}

// OwnerView is the current owner: the latest acquisition with no later
// release.
type OwnerView struct {
	OwnerKind     string     `json:"owner_kind"`
	OwnerID       string     `json:"owner_id"`
	AcquiredAt    time.Time  `json:"acquired_at"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
}

type BlockerView struct {
	Blocker
	DetectedAt *time.Time `json:"detected_at,omitempty"`
	ClearedAt  *time.Time `json:"cleared_at,omitempty"`
	// ClearedBy and ClearReason are the clearance's writer and reason;
	// Blocker keeps the detection's payload.
	ClearedBy   string `json:"cleared_by,omitempty"`
	ClearReason string `json:"clear_reason,omitempty"`
	Source      string `json:"source"`
	Provenance  string `json:"provenance"`
}

type AdmissionEntry struct {
	Meta
	Admission
}

type SessionView struct {
	SessionID string    `json:"session_id"`
	AgentID   string    `json:"agent_id,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Versions  Versions  `json:"versions"`
	Source    string    `json:"source"`
}

type AgentView struct {
	AgentID   string    `json:"agent_id"`
	Team      string    `json:"team,omitempty"`
	SessionID string    `json:"session_id,omitempty"`
	FirstSeen time.Time `json:"first_seen"`
	LastSeen  time.Time `json:"last_seen"`
	Source    string    `json:"source"`
}

type RequirementView struct {
	Number            int             `json:"number"`
	Task              string          `json:"task,omitempty"`
	Validation        string          `json:"validation,omitempty"`
	TaskChecked       *bool           `json:"task_checked,omitempty"`
	ValidationChecked *bool           `json:"validation_checked,omitempty"`
	StateSource       string          `json:"state_source,omitempty"`
	Attempts          []AttemptView   `json:"attempts"`
	Evidence          []EvidenceEntry `json:"evidence"`
}

type AttemptView struct {
	Number         int        `json:"number"`
	Key            string     `json:"key"`
	AssignmentID   string     `json:"assignment_id,omitempty"`
	ConductorRunID string     `json:"conductor_run_id,omitempty"`
	SessionID      string     `json:"session_id,omitempty"`
	AgentID        string     `json:"agent_id,omitempty"`
	Team           string     `json:"team,omitempty"`
	StartedAt      *time.Time `json:"started_at,omitempty"`
	FinishedAt     *time.Time `json:"finished_at,omitempty"`
	// Outcome is the finished outcome, or "open" when no finish was
	// observed.
	Outcome    string          `json:"outcome"`
	Kind       string          `json:"kind,omitempty"`
	DurationMS *int64          `json:"duration_ms,omitempty"`
	Reason     string          `json:"reason,omitempty"`
	Engine     string          `json:"engine,omitempty"`
	Provider   string          `json:"provider,omitempty"`
	Model      string          `json:"model,omitempty"`
	Evidence   []EvidenceEntry `json:"evidence"`
	Usage      UsageTotals     `json:"usage"`
	Source     string          `json:"source"`
}

type EvidenceEntry struct {
	Meta
	Attempt int `json:"attempt,omitempty"`
	Evidence
}

// UsageTotals sums known values. A total stays nil until at least one
// request reported that field; Partial names fields some requests lacked.
type UsageTotals struct {
	Requests         int      `json:"requests"`
	InputTokens      *int64   `json:"input_tokens,omitempty"`
	OutputTokens     *int64   `json:"output_tokens,omitempty"`
	ReasoningTokens  *int64   `json:"reasoning_tokens,omitempty"`
	CacheReadTokens  *int64   `json:"cache_read_tokens,omitempty"`
	CacheWriteTokens *int64   `json:"cache_write_tokens,omitempty"`
	CostUSD          *float64 `json:"cost_usd,omitempty"`
	Partial          []string `json:"partial,omitempty"`
}

type UsageEntry struct {
	Meta
	Requirement int    `json:"requirement,omitempty"`
	Attempt     int    `json:"attempt,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	AgentID     string `json:"agent_id,omitempty"`
	// Replaced counts earlier observations of the same request that this
	// one superseded.
	Replaced int `json:"replaced,omitempty"`
	Usage
}

// UsageView keeps scopes apart: an exact provider total, an estimated
// allocation, a whole-session cost and an allowance never add together.
type UsageView struct {
	Requests []UsageEntry           `json:"requests"`
	ByScope  map[string]UsageTotals `json:"by_scope"`
}

type IntervalEntry struct {
	Meta
	Interval
}

type DeliveryEntry struct {
	Meta
	Delivery
}

type CoverageEntry struct {
	Meta
	Coverage
}

// sortEvents orders events by event time, then id — the deterministic
// fold order that makes out-of-order arrival irrelevant.
func sortEvents(events []Event) {
	sort.SliceStable(events, func(i, j int) bool {
		if !events[i].At.Equal(events[j].At) {
			return events[i].At.Before(events[j].At)
		}
		return events[i].ID < events[j].ID
	})
}

func addPtr(dst **int64, v *int64) {
	if v == nil {
		return
	}
	if *dst == nil {
		z := int64(0)
		*dst = &z
	}
	**dst += *v
}

func (t *UsageTotals) add(u Usage) {
	t.Requests++
	addPtr(&t.InputTokens, u.InputTokens)
	addPtr(&t.OutputTokens, u.OutputTokens)
	addPtr(&t.ReasoningTokens, u.ReasoningTokens)
	addPtr(&t.CacheReadTokens, u.CacheReadTokens)
	addPtr(&t.CacheWriteTokens, u.CacheWriteTokens)
	if u.CostUSD != nil {
		if t.CostUSD == nil {
			z := 0.0
			t.CostUSD = &z
		}
		*t.CostUSD += *u.CostUSD
	}
	missing := func(name string, absent bool) {
		if absent && !contains(t.Partial, name) {
			t.Partial = append(t.Partial, name)
		}
	}
	missing("input_tokens", u.InputTokens == nil)
	missing("output_tokens", u.OutputTokens == nil)
	missing("cost_usd", u.CostUSD == nil)
}

func ptrTime(t time.Time) *time.Time { return &t }

// Fold builds the read-only view of one run from its events. Duplicate ids
// are counted and dropped; events fold in (time, id) order.
func Fold(events []Event, info fileRead) Run {
	r := Run{SchemaVersion: SchemaVersion, State: "unknown", Torn: info.Torn, Unreadable: info.Unreadable,
		Lifecycle: []LifecycleEntry{}, Ownership: []OwnershipEntry{}, Blockers: []BlockerView{},
		Admissions: []AdmissionEntry{}, Sessions: []SessionView{}, Agents: []AgentView{},
		Requirements: []RequirementView{}, Intervals: []IntervalEntry{}, Deliveries: []DeliveryEntry{},
		Coverage: []CoverageEntry{}, Unknown: []string{}, attemptKeys: map[string]bool{},
		Usage: UsageView{Requests: []UsageEntry{}, ByScope: map[string]UsageTotals{}}}
	seen := map[string]bool{}
	var list []Event
	for _, e := range events {
		if seen[e.ID] {
			r.Duplicates++
			continue
		}
		seen[e.ID] = true
		list = append(list, e)
	}
	sortEvents(list)
	r.Events = len(list)

	sessions := map[string]int{}
	agents := map[string]int{}
	reqs := map[int]int{}
	attempts := map[string][2]int{} // "<req>\x00<key>" -> (requirement index, attempt index)
	usage := map[string]int{}       // "<scope>\x00<request>" -> index in r.Usage.Requests
	sources := map[string]bool{}
	provenances := map[string]bool{}
	legacySources := map[string]bool{}

	requirement := func(n int) *RequirementView {
		i, ok := reqs[n]
		if !ok {
			i = len(r.Requirements)
			reqs[n] = i
			r.Requirements = append(r.Requirements, RequirementView{Number: n, Attempts: []AttemptView{}, Evidence: []EvidenceEntry{}})
		}
		return &r.Requirements[i]
	}
	attemptKey := func(e Event) string {
		if e.AttemptKey != "" {
			return e.AttemptKey
		}
		if e.Attempt > 0 {
			return fmt.Sprintf("n:%d", e.Attempt)
		}
		return ""
	}
	attempt := func(e Event) *AttemptView {
		key := attemptKey(e)
		if e.Requirement < 0 || key == "" {
			return nil
		}
		k := fmt.Sprintf("%d\x00%s", e.Requirement, key)
		if at, ok := attempts[k]; ok {
			return &r.Requirements[at[0]].Attempts[at[1]]
		}
		req := requirement(e.Requirement)
		number := e.Attempt
		if number <= 0 {
			for _, a := range req.Attempts {
				if a.Number > number {
					number = a.Number
				}
			}
			number++
		}
		req.Attempts = append(req.Attempts, AttemptView{Number: number, Key: key, Outcome: "open", Evidence: []EvidenceEntry{}, Source: e.Source})
		attempts[k] = [2]int{reqs[e.Requirement], len(req.Attempts) - 1}
		r.attemptKeys[key] = true
		return &req.Attempts[len(req.Attempts)-1]
	}

	for _, e := range list {
		if r.ProjectPath == "" {
			r.ProjectPath, r.ProjectKey, r.Plan, r.RunID = e.ProjectPath, e.ProjectKey, e.Plan, e.RunID
			r.firstAt = e.At
		}
		if e.ProjectID != "" {
			r.ProjectID = e.ProjectID
		}
		sources[e.Source] = true
		provenances[e.Provenance] = true
		if strings.HasPrefix(e.Provenance, "legacy:") {
			legacySources[strings.TrimPrefix(e.Provenance, "legacy:")] = true
		}
		if r.OpenedAt == nil || e.At.Before(*r.OpenedAt) {
			r.OpenedAt = ptrTime(e.At)
		}
		r.LastAt = ptrTime(e.At)
		mergeVersions(&r.Versions, e.Versions)

		switch e.Type {
		case TypeRunOpened:
			r.Marker = e.Marker
			r.OpenedAt = ptrTime(e.At)
		case TypeLifecycle:
			l := *e.Lifecycle
			r.Lifecycle = append(r.Lifecycle, LifecycleEntry{Meta: meta(e), Lifecycle: l})
			// Archived is terminal for a run: a later execution of the
			// codename opens a new run.
			if !r.Archived {
				r.State = l.State
			}
			if l.State == "archived" {
				r.State = "archived"
				r.Archived = true
				if l.ArchiveFile != "" {
					r.ArchiveFile = l.ArchiveFile
				}
			}
		case TypeSessionObserved:
			i, ok := sessions[e.SessionID]
			if !ok {
				i = len(r.Sessions)
				sessions[e.SessionID] = i
				r.Sessions = append(r.Sessions, SessionView{SessionID: e.SessionID, AgentID: e.AgentID, FirstSeen: e.At, Source: e.Source})
			}
			r.Sessions[i].LastSeen = e.At
			mergeVersions(&r.Sessions[i].Versions, e.Versions)
		case TypeAgentObserved:
			i, ok := agents[e.AgentID]
			if !ok {
				i = len(r.Agents)
				agents[e.AgentID] = i
				r.Agents = append(r.Agents, AgentView{AgentID: e.AgentID, Team: e.Team, SessionID: e.SessionID, FirstSeen: e.At, Source: e.Source})
			}
			r.Agents[i].LastSeen = e.At
			if e.Team != "" {
				r.Agents[i].Team = e.Team
			}
		case TypeOwnershipAcquired, TypeOwnershipBeat, TypeOwnershipReleased:
			kind := strings.TrimPrefix(e.Type, "ownership.")
			r.Ownership = append(r.Ownership, OwnershipEntry{Meta: meta(e), Event: kind, Ownership: *e.Ownership})
			switch e.Type {
			case TypeOwnershipAcquired:
				r.Owner = &OwnerView{OwnerKind: e.Ownership.OwnerKind, OwnerID: e.Ownership.OwnerID, AcquiredAt: e.At}
			case TypeOwnershipBeat:
				if r.Owner != nil && r.Owner.OwnerID == e.Ownership.OwnerID {
					r.Owner.LastHeartbeat = ptrTime(e.At)
				}
			case TypeOwnershipReleased:
				if r.Owner != nil && r.Owner.OwnerID == e.Ownership.OwnerID {
					r.Owner = nil
				}
			}
		case TypeBlockerDetected:
			r.Blockers = append(r.Blockers, BlockerView{Blocker: *e.Blocker, DetectedAt: ptrTime(e.At), Source: e.Source, Provenance: e.Provenance})
		case TypeBlockerCleared:
			cleared := false
			for i := len(r.Blockers) - 1; i >= 0; i-- {
				b := &r.Blockers[i]
				if b.ClearedAt == nil && (e.Blocker.Key == "" || b.Key == e.Blocker.Key) {
					b.ClearedAt = ptrTime(e.At)
					b.ClearedBy, b.ClearReason = e.Source, e.Blocker.Reason
					cleared = true
					break
				}
			}
			if !cleared {
				// A clearance with no observed detection keeps its
				// detection time unknown.
				r.Blockers = append(r.Blockers, BlockerView{Blocker: *e.Blocker, ClearedAt: ptrTime(e.At), ClearedBy: e.Source, ClearReason: e.Blocker.Reason, Source: e.Source, Provenance: e.Provenance})
			}
		case TypeAdmission:
			r.Admissions = append(r.Admissions, AdmissionEntry{Meta: meta(e), Admission: *e.Admission})
		case TypeRequirement:
			req := requirement(e.Requirement)
			st := e.RequirementState
			if st.Task != "" {
				req.Task = st.Task
			}
			if st.Validation != "" {
				req.Validation = st.Validation
			}
			if st.TaskChecked != nil {
				req.TaskChecked = st.TaskChecked
			}
			if st.ValidationChecked != nil {
				req.ValidationChecked = st.ValidationChecked
			}
			req.StateSource = e.Provenance
		case TypeAttemptStarted, TypeAttemptFinished:
			a := attempt(e)
			if e.AssignmentID != "" {
				a.AssignmentID = e.AssignmentID
			}
			if e.ConductorRunID != "" {
				a.ConductorRunID = e.ConductorRunID
			}
			if e.SessionID != "" {
				a.SessionID = e.SessionID
			}
			if e.AgentID != "" {
				a.AgentID = e.AgentID
			}
			if e.Team != "" {
				a.Team = e.Team
			}
			if info := e.AttemptInfo; info != nil {
				if info.Kind != "" {
					a.Kind = info.Kind
				}
				if info.Engine != "" {
					a.Engine = info.Engine
				}
				if info.Provider != "" {
					a.Provider = info.Provider
				}
				if info.Model != "" {
					a.Model = info.Model
				}
			}
			if e.Type == TypeAttemptStarted {
				if a.StartedAt == nil {
					a.StartedAt = ptrTime(e.At)
				}
				continue
			}
			if a.FinishedAt == nil || !e.At.Before(*a.FinishedAt) {
				a.FinishedAt = ptrTime(e.At)
				a.Outcome = e.AttemptInfo.Outcome
				a.Reason = e.AttemptInfo.Reason
				a.DurationMS = e.AttemptInfo.DurationMS
				if a.DurationMS == nil && a.StartedAt != nil {
					d := a.FinishedAt.Sub(*a.StartedAt).Milliseconds()
					a.DurationMS = &d
				}
			}
		case TypeEvidence:
			entry := EvidenceEntry{Meta: meta(e), Evidence: *e.Evidence}
			// Requirement 0 holds plan-level evidence (delivery, imported
			// prose that names no step).
			if a := attempt(e); a != nil {
				entry.Attempt = a.Number
				a.Evidence = append(a.Evidence, entry)
				continue
			}
			req := requirement(e.Requirement)
			req.Evidence = append(req.Evidence, entry)
		case TypeUsage:
			u := *e.Usage
			entry := UsageEntry{Meta: meta(e), Requirement: e.Requirement, SessionID: e.SessionID, AgentID: e.AgentID, Usage: u}
			if a := attempt(e); a != nil {
				entry.Attempt = a.Number
			}
			k := u.Scope + "\x00" + u.RequestID
			if i, ok := usage[k]; ok {
				entry.Replaced = r.Usage.Requests[i].Replaced + 1
				r.Usage.Requests[i] = entry
			} else {
				usage[k] = len(r.Usage.Requests)
				r.Usage.Requests = append(r.Usage.Requests, entry)
			}
		case TypeInterval:
			r.Intervals = append(r.Intervals, IntervalEntry{Meta: meta(e), Interval: *e.Interval})
		case TypeDelivery:
			r.Deliveries = append(r.Deliveries, DeliveryEntry{Meta: meta(e), Delivery: *e.Delivery})
		case TypeCoverage:
			r.Coverage = append(r.Coverage, CoverageEntry{Meta: meta(e), Coverage: *e.Coverage})
		}
	}

	// Usage totals come from the deduplicated request list only.
	for _, u := range r.Usage.Requests {
		t := r.Usage.ByScope[u.Scope]
		t.add(u.Usage)
		r.Usage.ByScope[u.Scope] = t
		if u.Attempt > 0 {
			if at, ok := attempts[fmt.Sprintf("%d\x00%s", u.Requirement, attemptKeyFor(r, u.Requirement, u.Attempt))]; ok {
				r.Requirements[at[0]].Attempts[at[1]].Usage.add(u.Usage)
			}
		}
	}
	sort.SliceStable(r.Requirements, func(i, j int) bool { return r.Requirements[i].Number < r.Requirements[j].Number })

	for s := range sources {
		r.Sources = append(r.Sources, s)
	}
	sort.Strings(r.Sources)
	switch {
	case len(provenances) == 1 && provenances["observed"]:
		r.Provenance = "observed"
	case len(legacySources) > 0 && len(provenances) == len(legacySources):
		var names []string
		for s := range legacySources {
			names = append(names, s)
		}
		sort.Strings(names)
		r.Provenance = "legacy:" + strings.Join(names, ",")
	case len(provenances) == 0:
		r.Provenance = "unknown"
	default:
		r.Provenance = "mixed"
	}
	r.Unknown = unknowns(r)
	return r
}

// attemptKeyFor finds the key of attempt number n of requirement req.
func attemptKeyFor(r Run, req, n int) string {
	for _, q := range r.Requirements {
		if q.Number != req {
			continue
		}
		for _, a := range q.Attempts {
			if a.Number == n {
				return a.Key
			}
		}
	}
	return ""
}

func mergeVersions(dst *Versions, src *Versions) {
	if src == nil {
		return
	}
	set := func(d *string, s string) {
		if s != "" {
			*d = s
		}
	}
	set(&dst.Engine, src.Engine)
	set(&dst.EngineVersion, src.EngineVersion)
	set(&dst.ExecutorVersion, src.ExecutorVersion)
	set(&dst.PlannerVersion, src.PlannerVersion)
	set(&dst.IterateRun, src.IterateRun)
	set(&dst.Provider, src.Provider)
	set(&dst.Model, src.Model)
}

// unknowns lists what no event established. Unknown is never reported as
// zero.
func unknowns(r Run) []string {
	out := []string{}
	add := func(cond bool, name string) {
		if cond {
			out = append(out, name)
		}
	}
	model := r.Versions.Model != ""
	provider := r.Versions.Provider != ""
	for _, s := range r.Sessions {
		model = model || s.Versions.Model != ""
		provider = provider || s.Versions.Provider != ""
	}
	evidence := false
	for _, q := range r.Requirements {
		evidence = evidence || len(q.Evidence) > 0
		for _, a := range q.Attempts {
			model = model || a.Model != ""
			provider = provider || a.Provider != ""
			evidence = evidence || len(a.Evidence) > 0
		}
	}
	for _, u := range r.Usage.Requests {
		model = model || u.Model != ""
		provider = provider || u.Provider != ""
	}
	add(r.Versions.Engine == "", "versions.engine")
	add(r.Versions.ExecutorVersion == "", "versions.executor_version")
	add(r.Versions.IterateRun == "", "versions.iterate_run")
	add(!provider, "versions.provider")
	add(!model, "versions.model")
	add(len(r.Lifecycle) == 0, "lifecycle")
	add(len(r.Ownership) == 0, "ownership")
	add(len(r.Sessions) == 0, "sessions")
	add(len(r.Agents) == 0, "agents")
	add(!evidence, "evidence")
	add(len(r.Usage.Requests) == 0, "usage")
	add(len(r.Intervals) == 0, "intervals")
	add(len(r.Deliveries) == 0, "delivery")
	add(len(r.Coverage) == 0, "coverage")
	return out
}
