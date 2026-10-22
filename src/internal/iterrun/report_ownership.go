package iterrun

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// OwnershipSection is the plan report's step-9 section: CLI and Conductor ownership, handoffs, blockers and admission decisions, with their delays.
//
// Every figure comes from recorded events: ownership acquired, heartbeat
// and released, blocker detected and cleared, admission considered and
// lifecycle states. Liveness is never derived from a plan checkbox, an
// open terminal or the plan file read at report time, and reading the
// report never releases a stale owner.
type OwnershipSection struct {
	Status     string `json:"status"`
	StatusNote string `json:"status_note"`
	// Current is the owner holding the plan, nil when none holds it.
	Current     *OwnershipHolder `json:"current,omitempty"`
	CurrentNote string           `json:"current_note"`
	// LastActivity is the latest recorded sign of work: a heartbeat, a
	// session or agent observation, an attempt or an ownership event.
	LastActivity     *time.Time         `json:"last_activity,omitempty"`
	LastActivityWhat string             `json:"last_activity_what,omitempty"`
	Pending          *OwnershipPending  `json:"pending,omitempty"`
	Holds            int                `json:"holds"`
	Handoffs         []OwnershipHandoff `json:"handoffs"`
	Blockers         []OwnershipBlocker `json:"blockers"`
	Transitions      []OwnershipEvent   `json:"transitions"`
	TransitionsTotal int                `json:"transitions_total"`
	Terms            []LifecycleTerm    `json:"definitions"`
	Unknown          []string           `json:"unknown"`
}

// OwnershipHolder is the owner currently holding the plan.
type OwnershipHolder struct {
	Kind          string     `json:"kind"`
	ID            string     `json:"id"`
	Label         string     `json:"label"`
	AcquiredAt    time.Time  `json:"acquired_at"`
	LastHeartbeat *time.Time `json:"last_heartbeat,omitempty"`
	LastActivity  time.Time  `json:"last_activity"`
	// Stale marks an owner with no heartbeat or activity for the live
	// window and no recorded release. Only the owner's own authority
	// reconciles it.
	Stale     bool   `json:"stale"`
	StaleNote string `json:"stale_note,omitempty"`
}

// OwnershipPending explains a plan that no owner holds and that has not
// ended: since when it waits and the latest reason recorded for it.
type OwnershipPending struct {
	Since     time.Time  `json:"since"`
	SinceWhat string     `json:"since_what"`
	WaitingMS int64      `json:"waiting_ms"`
	Decision  string     `json:"decision,omitempty"`
	Gate      string     `json:"gate,omitempty"`
	Reason    string     `json:"reason"`
	DecidedAt *time.Time `json:"decided_at,omitempty"`
}

// OwnershipHandoff is one release and the acquisition that resumed the
// plan, with the delays between blocker clearance, release and
// resumption.
type OwnershipHandoff struct {
	From       string     `json:"from"`
	FromKind   string     `json:"from_kind"`
	ReleasedAt *time.Time `json:"released_at,omitempty"`
	// ReleaseNote explains a release the record lacks (superseded).
	ReleaseReason string     `json:"release_reason,omitempty"`
	To            string     `json:"to,omitempty"`
	ToKind        string     `json:"to_kind,omitempty"`
	ResumedAt     *time.Time `json:"resumed_at,omitempty"`
	// ReleaseToResumeMS is nil when either end is not recorded.
	ReleaseToResumeMS *int64     `json:"release_to_resume_ms,omitempty"`
	BlockerClearedAt  *time.Time `json:"blocker_cleared_at,omitempty"`
	BlockerReason     string     `json:"blocker_reason,omitempty"`
	// ClearanceObserved is false when the clearance was first seen at a
	// Conductor admission, so its time is an upper bound.
	ClearanceObserved bool   `json:"clearance_observed"`
	ClearToReleaseMS  *int64 `json:"clear_to_release_ms,omitempty"`
	ClearToResumeMS   *int64 `json:"clear_to_resume_ms,omitempty"`
	// Gates lists the admission decisions that held the plan while it
	// waited.
	Gates []string `json:"gates"`
}

// OwnershipBlocker is one blocker and its recorded span.
type OwnershipBlocker struct {
	Key               string     `json:"key,omitempty"`
	Kind              string     `json:"kind,omitempty"`
	Reason            string     `json:"reason,omitempty"`
	DetectedAt        *time.Time `json:"detected_at,omitempty"`
	ClearedAt         *time.Time `json:"cleared_at,omitempty"`
	ClearanceObserved bool       `json:"clearance_observed"`
	ClearedBy         string     `json:"cleared_by,omitempty"`
	ClearReason       string     `json:"clear_reason,omitempty"`
	DurationMS        *int64     `json:"duration_ms,omitempty"`
	Source            string     `json:"source"`
}

// OwnershipEvent is one ordered transition.
type OwnershipEvent struct {
	At         time.Time `json:"at"`
	Actor      string    `json:"actor"`
	Event      string    `json:"event"`
	Summary    string    `json:"summary"`
	Provenance string    `json:"provenance"`
}

const (
	// handoffMinGap separates a handoff from back-to-back holds by the
	// same kind of owner (one Conductor assignment after another).
	handoffMinGap = time.Minute
	// ownershipTransitionCap bounds the transitions listed.
	ownershipTransitionCap = 60
)

var ownershipTerms = []LifecycleTerm{
	{"Owner", "The CLI /iterate session holding the plan's running: lock, or the Conductor assignment holding its lease."},
	{"Heartbeat", "A newer running: timestamp written by the CLI owner, or provider output from the Conductor assignment (at most one per five minutes)."},
	{"Stale owner", "An owner with no heartbeat or activity for the live window and no recorded release. The owner's own authority reconciles it — /iterate's stale-lock rule or the Conductor's lease reconciliation. Reading this report never releases it."},
	{"Handoff", "An owner released the plan and an owner acquired it again after at least a minute, or after a blocker cleared."},
	{"Blocker cleared to released", "From the recorded blocker clearance to the owner's release of the plan."},
	{"Released to resumed", "From the release to the next recorded acquisition: the actual resumption delay."},
	{"Admission decision", "The Conductor's verdict on a waiting plan and the gate that held it: automation_disabled, schedule, budget, ownership, queue or other."},
}

func actorName(source string) string {
	switch {
	case source == runrecord.SourceConductor:
		return "Conductor"
	case strings.HasPrefix(source, "cli"):
		return "CLI"
	case source == runrecord.SourceImport:
		return "import"
	}
	return "unknown"
}

type ownershipHold struct {
	kind, id      string
	acquired      time.Time
	beat          *time.Time
	released      *time.Time
	releaseReason string
	superseded    bool
}

func buildOwnershipSection(in ReportInput) OwnershipSection {
	s := OwnershipSection{Handoffs: []OwnershipHandoff{}, Blockers: []OwnershipBlocker{}, Transitions: []OwnershipEvent{}, Terms: ownershipTerms, Unknown: []string{}}
	if in.RecordErr != nil || len(in.Record.Runs) == 0 {
		s.Status = sectionUnavailable
		s.StatusNote = "No run record exists for this execution, so ownership, blockers and admission decisions are unknown."
		if in.RecordErr != nil {
			s.StatusNote = "The run record could not be read (" + in.RecordErr.Error() + "), so ownership is unknown."
		}
		s.CurrentNote = "Unknown."
		s.Unknown = append(s.Unknown, "ownership", "blockers", "admission")
		return s
	}

	var ownership []runrecord.OwnershipEntry
	var admissions []runrecord.AdmissionEntry
	var lifecycle []runrecord.LifecycleEntry
	var blockers []runrecord.BlockerView
	ended := false
	var last time.Time
	lastWhat := ""
	seen := func(t time.Time, what string) {
		if t.After(last) {
			last, lastWhat = t, what
		}
	}
	for _, r := range in.Record.Runs {
		ownership = append(ownership, r.Ownership...)
		admissions = append(admissions, r.Admissions...)
		lifecycle = append(lifecycle, r.Lifecycle...)
		blockers = append(blockers, r.Blockers...)
		if r.State == "completed" || r.State == "archived" {
			ended = true
		}
		for _, x := range r.Sessions {
			seen(x.LastSeen, "session "+x.SessionID+" observed")
		}
		for _, x := range r.Agents {
			seen(x.LastSeen, "agent "+x.AgentID+" observed")
		}
		for _, q := range r.Requirements {
			for _, a := range q.Attempts {
				if a.StartedAt != nil {
					seen(*a.StartedAt, fmt.Sprintf("attempt %d of requirement %d started", a.Number, q.Number))
				}
				if a.FinishedAt != nil {
					seen(*a.FinishedAt, fmt.Sprintf("attempt %d of requirement %d finished", a.Number, q.Number))
				}
			}
		}
	}
	if in.Plan.Archived || in.Plan.Phase == "complete" || in.Plan.Phase == "done" {
		ended = true
	}
	sort.SliceStable(ownership, func(i, j int) bool { return ownership[i].At.Before(ownership[j].At) })
	sort.SliceStable(admissions, func(i, j int) bool { return admissions[i].At.Before(admissions[j].At) })
	sort.SliceStable(lifecycle, func(i, j int) bool { return lifecycle[i].At.Before(lifecycle[j].At) })

	// Holds: each acquisition to its release. An acquisition while another
	// hold is open supersedes it; no release is invented for it.
	var holds []*ownershipHold
	var open *ownershipHold
	for _, o := range ownership {
		label := ownerName(o.OwnerKind, o.OwnerID)
		seen(o.At, label+" "+o.Event)
		switch o.Event {
		case "acquired":
			if open != nil {
				open.superseded = true
			}
			open = &ownershipHold{kind: o.OwnerKind, id: o.OwnerID, acquired: o.At}
			holds = append(holds, open)
		case "heartbeat":
			if open != nil && open.id == o.OwnerID {
				at := o.At
				open.beat = &at
			}
		case "released":
			if open != nil && open.id == o.OwnerID {
				at := o.At
				open.released, open.releaseReason = &at, o.Reason
				open = nil
			}
		}
	}
	s.Holds = len(holds)
	if !last.IsZero() {
		l := last
		s.LastActivity, s.LastActivityWhat = &l, lastWhat
	}

	// Blockers.
	for _, b := range blockers {
		ob := OwnershipBlocker{Key: b.Key, Kind: b.Kind, Reason: b.Reason, DetectedAt: b.DetectedAt, ClearedAt: b.ClearedAt, Source: actorName(b.Source), ClearanceObserved: true, ClearReason: b.ClearReason}
		if b.ClearedAt != nil {
			ob.ClearedBy = actorName(lcOr(b.ClearedBy, b.Source))
		}
		if b.ClearedAt != nil && strings.HasPrefix(b.ClearReason, clearanceAtAdmission) {
			ob.ClearanceObserved = false
		}
		if b.DetectedAt != nil && b.ClearedAt != nil {
			d := lcMS(b.ClearedAt.Sub(*b.DetectedAt))
			ob.DurationMS = &d
		}
		s.Blockers = append(s.Blockers, ob)
	}
	sort.SliceStable(s.Blockers, func(i, j int) bool {
		return lcFirst(s.Blockers[i].DetectedAt, s.Blockers[i].ClearedAt).Before(lcFirst(s.Blockers[j].DetectedAt, s.Blockers[j].ClearedAt))
	})

	// Handoffs between consecutive holds.
	clearedIn := func(after, through time.Time) *OwnershipBlocker {
		var found *OwnershipBlocker
		for i := range s.Blockers {
			b := &s.Blockers[i]
			if b.ClearedAt != nil && b.ClearedAt.After(after) && !b.ClearedAt.After(through) {
				found = b
			}
		}
		return found
	}
	gatesIn := func(after, before time.Time) []string {
		out := []string{}
		for _, a := range admissions {
			if a.Decision == "gated" && a.At.After(after) && (before.IsZero() || a.At.Before(before)) {
				g := a.Gate
				if a.Reason != "" {
					g += " (" + a.Reason + ")"
				}
				out = append(out, g+" at "+a.At.UTC().Format(time.RFC3339))
			}
		}
		return out
	}
	for i := 0; i+1 < len(holds); i++ {
		h, next := holds[i], holds[i+1]
		end := next.acquired
		if h.released != nil {
			end = *h.released
		}
		blocker := clearedIn(h.acquired, next.acquired)
		gap := next.acquired.Sub(end)
		if h.released != nil && h.kind == next.kind && gap < handoffMinGap && blocker == nil {
			continue
		}
		ho := OwnershipHandoff{From: ownerName(h.kind, h.id), FromKind: h.kind, To: ownerName(next.kind, next.id), ToKind: next.kind, ResumedAt: lcPtr(next.acquired), Gates: gatesIn(end, next.acquired)}
		if h.released != nil {
			ho.ReleasedAt, ho.ReleaseReason = h.released, h.releaseReason
			d := lcMS(next.acquired.Sub(*h.released))
			ho.ReleaseToResumeMS = &d
		} else {
			ho.ReleaseReason = "no release recorded; superseded by the next acquisition"
		}
		attachClearance(&ho, blocker)
		s.Handoffs = append(s.Handoffs, ho)
	}

	// The current owner, or why none holds the plan.
	now := in.Now.UTC()
	if n := len(holds); n > 0 && holds[n-1].released == nil {
		h := holds[n-1]
		activity := h.acquired
		if h.beat != nil && h.beat.After(activity) {
			activity = *h.beat
		}
		for _, r := range in.Record.Runs {
			for _, x := range r.Sessions {
				if h.kind == "cli" && x.SessionID == h.id && x.LastSeen.After(activity) {
					activity = x.LastSeen
				}
			}
			for _, q := range r.Requirements {
				for _, a := range q.Attempts {
					if h.kind == "conductor" && a.AssignmentID == h.id {
						for _, t := range []*time.Time{a.StartedAt, a.FinishedAt} {
							if t != nil && t.After(activity) {
								activity = *t
							}
						}
					}
				}
			}
		}
		cur := &OwnershipHolder{Kind: h.kind, ID: h.id, Label: ownerName(h.kind, h.id), AcquiredAt: h.acquired, LastHeartbeat: h.beat, LastActivity: activity}
		switch {
		case ended:
			cur.Stale = true
			cur.StaleNote = "The execution ended with no recorded release from this owner."
		case now.Sub(activity) > LiveWindow():
			cur.Stale = true
			cur.StaleNote = fmt.Sprintf("No heartbeat or activity recorded for %s, and no release. The owner's own authority reconciles it: /iterate's stale-lock rule or the Conductor's lease reconciliation. Reading this report does not release it.", lcDur(lcMS(now.Sub(activity))))
		}
		s.Current = cur
		s.CurrentNote = cur.Label + " holds the plan."
	} else if len(holds) == 0 {
		s.CurrentNote = "No ownership is recorded for this execution."
		s.Unknown = append(s.Unknown, "ownership")
	} else {
		h := holds[len(holds)-1]
		s.CurrentNote = fmt.Sprintf("No owner holds the plan; %s released it at %s.", ownerName(h.kind, h.id), h.released.UTC().Format(time.RFC3339))
		if !ended {
			s.Pending = ownershipPending(*h.released, h, s.Blockers, admissions, now)
			ho := OwnershipHandoff{From: ownerName(h.kind, h.id), FromKind: h.kind, ReleasedAt: h.released, ReleaseReason: h.releaseReason, Gates: gatesIn(*h.released, time.Time{})}
			attachClearance(&ho, clearedIn(h.acquired, now))
			s.Handoffs = append(s.Handoffs, ho)
		}
	}

	// Ordered transitions.
	var events []OwnershipEvent
	for _, o := range ownership {
		if o.Event == "heartbeat" {
			continue
		}
		summary := ownerName(o.OwnerKind, o.OwnerID) + " " + o.Event + " the plan"
		if o.Event == "acquired" {
			summary = ownerName(o.OwnerKind, o.OwnerID) + " acquired the plan"
		}
		if o.Reason != "" {
			summary += " (" + o.Reason + ")"
		}
		events = append(events, OwnershipEvent{At: o.At, Actor: lcOwnerActor(o.OwnerKind), Event: "ownership." + o.Event, Summary: summary, Provenance: o.Provenance})
	}
	for _, b := range blockers {
		if b.DetectedAt != nil {
			events = append(events, OwnershipEvent{At: *b.DetectedAt, Actor: actorName(b.Source), Event: "blocker.detected", Summary: "blocker detected: " + lcOr(b.Reason, b.Kind), Provenance: b.Provenance})
		}
		if b.ClearedAt != nil {
			events = append(events, OwnershipEvent{At: *b.ClearedAt, Actor: actorName(lcOr(b.ClearedBy, b.Source)), Event: "blocker.cleared", Summary: "blocker cleared: " + lcOr(b.ClearReason, b.Key), Provenance: b.Provenance})
		}
	}
	for _, a := range admissions {
		summary := "admission " + a.Decision
		if a.Gate != "" {
			summary += " by gate " + a.Gate
		}
		if a.Reason != "" {
			summary += ": " + a.Reason
		}
		events = append(events, OwnershipEvent{At: a.At, Actor: "Conductor", Event: "admission.considered", Summary: summary, Provenance: a.Provenance})
	}
	for _, l := range lifecycle {
		summary := "state " + l.State
		if l.Reason != "" {
			summary += " (" + l.Reason + ")"
		}
		events = append(events, OwnershipEvent{At: l.At, Actor: actorName(l.Source), Event: "lifecycle." + l.State, Summary: summary, Provenance: l.Provenance})
	}
	sort.SliceStable(events, func(i, j int) bool { return events[i].At.Before(events[j].At) })
	s.TransitionsTotal = len(events)
	if len(events) > ownershipTransitionCap {
		events = events[len(events)-ownershipTransitionCap:]
	}
	if events != nil {
		s.Transitions = events
	}

	if len(admissions) == 0 {
		s.Unknown = append(s.Unknown, "admission decisions")
	}
	switch {
	case in.Record.Source == "legacy":
		s.Status = sectionPartial
		s.StatusNote = "This execution has no run record. Its legacy history carries plan states and Conductor attempts but no ownership, heartbeat or admission events."
	case len(holds) == 0:
		s.Status = sectionPartial
		s.StatusNote = "The run record holds no ownership events for this execution, so who held it and when it was handed off are unknown."
	default:
		s.Status = sectionAvailable
		s.StatusNote = "Ownership, blockers and admission decisions come from recorded events."
	}
	return s
}

// ownershipPending explains a released plan that has not ended.
func ownershipPending(released time.Time, h *ownershipHold, blockers []OwnershipBlocker, admissions []runrecord.AdmissionEntry, now time.Time) *OwnershipPending {
	p := &OwnershipPending{Since: released, SinceWhat: ownerName(h.kind, h.id) + " released the plan"}
	for _, b := range blockers {
		if b.DetectedAt != nil && b.ClearedAt == nil {
			p.Decision, p.Gate = "blocked", "blocker"
			p.Reason = "a blocker is open: " + lcOr(b.Reason, b.Kind)
			p.WaitingMS = lcMS(now.Sub(released))
			return p
		}
		if b.ClearedAt != nil && b.ClearedAt.After(p.Since) {
			p.Since, p.SinceWhat = *b.ClearedAt, "the blocker cleared"
		}
	}
	p.WaitingMS = max(lcMS(now.Sub(p.Since)), 0)
	var latest *runrecord.AdmissionEntry
	for i := range admissions {
		if admissions[i].At.After(released) {
			latest = &admissions[i]
		}
	}
	switch {
	case latest == nil:
		p.Reason = "No admission decision is recorded since the release: the Conductor may be stood down, may not cover this project, or has not ticked."
	case latest.Decision == "admitted":
		p.Decision = "admitted"
		p.Reason = "admitted; no ownership acquisition is recorded yet"
		p.DecidedAt = lcPtr(latest.At)
	default:
		p.Decision, p.Gate = latest.Decision, latest.Gate
		p.Reason = lcOr(latest.Reason, "held by gate "+latest.Gate)
		p.DecidedAt = lcPtr(latest.At)
	}
	return p
}

func attachClearance(ho *OwnershipHandoff, b *OwnershipBlocker) {
	if b == nil || b.ClearedAt == nil {
		return
	}
	ho.BlockerClearedAt, ho.BlockerReason, ho.ClearanceObserved = b.ClearedAt, b.Reason, b.ClearanceObserved
	if !b.ClearanceObserved {
		return
	}
	if ho.ReleasedAt != nil && !b.ClearedAt.After(*ho.ReleasedAt) {
		d := lcMS(ho.ReleasedAt.Sub(*b.ClearedAt))
		ho.ClearToReleaseMS = &d
	}
	if ho.ResumedAt != nil && (ho.ReleasedAt == nil || b.ClearedAt.After(*ho.ReleasedAt)) {
		d := lcMS(ho.ResumedAt.Sub(*b.ClearedAt))
		ho.ClearToResumeMS = &d
	}
}

func lcPtr(t time.Time) *time.Time { return &t }

func lcFirst(ts ...*time.Time) time.Time {
	for _, t := range ts {
		if t != nil {
			return *t
		}
	}
	return time.Time{}
}

func lcOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func lcOwnerActor(kind string) string {
	switch kind {
	case "cli":
		return "CLI"
	case "conductor":
		return "Conductor"
	}
	return "unknown"
}

const ownershipCSS = `#report-ownership .ro-now{display:grid;grid-template-columns:minmax(0,max-content) minmax(0,1fr);gap:4px 14px;margin:0 0 12px;font-size:13px;background:var(--surface);border:1px solid var(--border);border-radius:8px;padding:10px 12px}
#report-ownership .ro-now dt{color:var(--text-dim);font-weight:600;margin:0}
#report-ownership .ro-now dd{margin:0;color:var(--text)}
#report-ownership .ro-stale{color:var(--warn);font-weight:600}
#report-ownership .ro-ho{padding:8px 10px;background:var(--surface);border:1px solid var(--border);border-radius:8px;list-style:none;margin:0 0 8px}
#report-ownership ol.ro-hos{padding:0}
#report-ownership .ro-ho ul{margin:4px 0 0;padding-left:18px;color:var(--text-dim);font-size:12.5px}
#report-ownership .ro-ho li{margin:0 0 2px}
#report-ownership .ro-delay{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;color:var(--text);font-weight:600}
#report-ownership ol.ro-log{font-size:12.5px}
@media (max-width:480px){#report-ownership .ro-now{grid-template-columns:1fr}#report-ownership .ro-now dd{margin-bottom:6px}}
`

func writeOwnershipSection(b *strings.Builder, s OwnershipSection) {
	b.WriteString(`<section class="report-section" id="report-ownership" aria-labelledby="report-ownership-h"><style>`)
	b.WriteString(lcSectionCSS("report-ownership", "ro") + ownershipCSS)
	b.WriteString(`</style><h2 id="report-ownership-h">Ownership and handoffs</h2>`)
	status := s.Status
	if status == "" {
		status = sectionUnavailable
	}
	writeLcStatus(b, "ro", status, s.StatusNote)

	b.WriteString(`<dl class="ro-now"><dt>Current owner</dt><dd>`)
	if c := s.Current; c != nil {
		fmt.Fprintf(b, `%s, since %s`, lcEsc(c.Label), lcTime(c.AcquiredAt))
		if c.Stale {
			fmt.Fprintf(b, ` <span class="ro-stale">Stale.</span> %s`, lcEsc(c.StaleNote))
		}
	} else {
		b.WriteString(lcEsc(lcOr(s.CurrentNote, "Unknown.")))
	}
	b.WriteString(`</dd><dt>Last heartbeat</dt><dd>`)
	if s.Current != nil && s.Current.LastHeartbeat != nil {
		b.WriteString(lcTime(*s.Current.LastHeartbeat))
	} else if s.Current != nil {
		b.WriteString(`none recorded since the acquisition`)
	} else {
		b.WriteString(`no owner`)
	}
	b.WriteString(`</dd><dt>Last activity</dt><dd>`)
	if s.LastActivity != nil {
		fmt.Fprintf(b, `%s <span class="ro-dim">(%s)</span>`, lcTime(*s.LastActivity), lcEsc(s.LastActivityWhat))
	} else {
		b.WriteString(`none recorded`)
	}
	b.WriteString(`</dd>`)
	if p := s.Pending; p != nil {
		fmt.Fprintf(b, `<dt>Waiting to resume</dt><dd><span class="ro-delay">%s</span> since %s (%s). %s`, lcDur(p.WaitingMS), lcTime(p.Since), lcEsc(p.SinceWhat), lcEsc(p.Reason))
		if p.DecidedAt != nil {
			fmt.Fprintf(b, ` <span class="ro-dim">Decided %s.</span>`, lcTime(*p.DecidedAt))
		}
		b.WriteString(`</dd>`)
	}
	fmt.Fprintf(b, `<dt>Holds</dt><dd>%s, %s</dd></dl>`, lcCount(s.Holds, "ownership hold"), lcCount(len(s.Blockers), "blocker"))

	b.WriteString(`<h3>Handoffs</h3>`)
	if len(s.Handoffs) == 0 {
		b.WriteString(`<p class="ro-dim">None recorded.</p>`)
	} else {
		b.WriteString(`<ol class="ro-hos">`)
		for _, h := range s.Handoffs {
			to := "not yet resumed"
			if h.To != "" {
				to = h.To
			}
			fmt.Fprintf(b, `<li class="ro-ho"><strong>%s &rarr; %s</strong><ul>`, lcEsc(h.From), lcEsc(to))
			if h.BlockerClearedAt != nil {
				note := ""
				if !h.ClearanceObserved {
					note = " (first seen at Conductor admission; the clearance itself was not observed)"
				}
				fmt.Fprintf(b, `<li>Blocker cleared %s%s: %s</li>`, lcTime(*h.BlockerClearedAt), lcEsc(note), lcEsc(lcOr(h.BlockerReason, "no reason recorded")))
			}
			if h.ReleasedAt != nil {
				fmt.Fprintf(b, `<li>Released %s`, lcTime(*h.ReleasedAt))
				if h.ReleaseReason != "" {
					fmt.Fprintf(b, ` (%s)`, lcEsc(h.ReleaseReason))
				}
				b.WriteString(`</li>`)
			} else {
				fmt.Fprintf(b, `<li>Release: %s</li>`, lcEsc(h.ReleaseReason))
			}
			if h.ResumedAt != nil {
				fmt.Fprintf(b, `<li>Resumed %s</li>`, lcTime(*h.ResumedAt))
			}
			if h.ClearToReleaseMS != nil {
				fmt.Fprintf(b, `<li>Blocker cleared to released: <span class="ro-delay">%s</span></li>`, lcDur(*h.ClearToReleaseMS))
			}
			if h.ClearToResumeMS != nil {
				fmt.Fprintf(b, `<li>Blocker cleared to resumed: <span class="ro-delay">%s</span></li>`, lcDur(*h.ClearToResumeMS))
			}
			if h.ReleaseToResumeMS != nil {
				fmt.Fprintf(b, `<li>Released to resumed: <span class="ro-delay">%s</span></li>`, lcDur(*h.ReleaseToResumeMS))
			}
			for _, g := range h.Gates {
				fmt.Fprintf(b, `<li>Held by %s</li>`, lcEsc(g))
			}
			b.WriteString(`</ul></li>`)
		}
		b.WriteString(`</ol>`)
	}

	if len(s.Blockers) > 0 {
		b.WriteString(`<h3>Blockers</h3><ul>`)
		for _, bl := range s.Blockers {
			fmt.Fprintf(b, `<li>%s`, lcEsc(lcOr(bl.Reason, lcOr(bl.Kind, "blocker"))))
			if bl.DetectedAt != nil {
				fmt.Fprintf(b, `; detected %s by %s`, lcTime(*bl.DetectedAt), lcEsc(bl.Source))
			} else {
				b.WriteString(`; detection not recorded`)
			}
			switch {
			case bl.ClearedAt == nil:
				b.WriteString(`; still open`)
			case bl.ClearanceObserved:
				fmt.Fprintf(b, `; cleared %s by %s`, lcTime(*bl.ClearedAt), lcEsc(bl.ClearedBy))
				if bl.ClearReason != "" {
					fmt.Fprintf(b, ` (%s)`, lcEsc(bl.ClearReason))
				}
			default:
				fmt.Fprintf(b, `; cleared by %s at the latest (the clearance was not observed)`, lcTime(*bl.ClearedAt))
			}
			if bl.DurationMS != nil {
				fmt.Fprintf(b, `; open for %s`, lcDur(*bl.DurationMS))
			}
			b.WriteString(`</li>`)
		}
		b.WriteString(`</ul>`)
	}

	fmt.Fprintf(b, `<details class="ro-terms"><summary>Transitions (%s)</summary>`, lcCount(s.TransitionsTotal, "event"))
	if s.TransitionsTotal > len(s.Transitions) {
		fmt.Fprintf(b, `<p class="ro-dim">The latest %d of %d.</p>`, len(s.Transitions), s.TransitionsTotal)
	}
	if len(s.Transitions) == 0 {
		b.WriteString(`<p class="ro-dim">None recorded.</p>`)
	} else {
		b.WriteString(`<ol class="ro-log">`)
		for _, e := range s.Transitions {
			fmt.Fprintf(b, `<li data-event="%s">%s <strong>%s</strong> %s</li>`, lcEsc(e.Event), lcTime(e.At), lcEsc(e.Actor), lcEsc(e.Summary))
		}
		b.WriteString(`</ol>`)
	}
	b.WriteString(`</details>`)
	writeLcTerms(b, "ro", s.Terms)
	b.WriteString(`</section>`)
}
