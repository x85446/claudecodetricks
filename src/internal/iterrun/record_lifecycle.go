package iterrun

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// recordNow is the run record's observation clock. Tests replace it with a
// bounded clock.
var recordNow = func() time.Time { return time.Now().UTC() }

// cliLockStale is /iterate's own stale-lock rule: a running: timestamp
// older than this belongs to a run that died, and the next run may take
// the lock.
const cliLockStale = 90 * time.Second

// clearanceAtAdmission begins the reason of a blocker clearance the record
// first saw when the Conductor admitted the plan again: the plan was no
// longer blocked, but when the operator cleared it was not observed.
const clearanceAtAdmission = "clearance not observed; plan admitted by the Conductor"

var (
	reRecordRunning        = regexp.MustCompile(`(?m)^running:[ \t]*([^\r\n]*)`)
	reRecordConductorOwner = regexp.MustCompile(`(?m)^conductor-assignment:[ \t]*([^\r\n]*)`)
)

// cliLock is /iterate's concurrency lock as the plan file states it. The
// CLI writes running: <timestamp> when it takes the lock and at every step
// boundary, and running: false on every exit path. The Conductor writes the
// same field while it holds a lease, together with conductor-assignment:.
type cliLock struct {
	held      bool      // running: carries a timestamp
	at        time.Time // that timestamp, the latest heartbeat written
	released  bool      // running: false
	conductor bool      // conductor-assignment: names a lease, so running: is the Conductor's
}

func readCLILock(raw []byte) cliLock {
	var l cliLock
	if m := reRecordConductorOwner.FindSubmatch(raw); m != nil && strings.TrimSpace(string(m[1])) != "" {
		l.conductor = true
	}
	m := reRecordRunning.FindSubmatch(raw)
	if m == nil {
		return l
	}
	value := strings.TrimSpace(string(m[1]))
	if value == "false" {
		l.released = true
		return l
	}
	if t, err := time.Parse(time.RFC3339, value); err == nil {
		l.held, l.at = true, t.UTC()
	}
	return l
}

// cliOwnershipEvents turns the plan's running: lock into ownership events.
// The lock is the CLI's authority, so an acquisition is a fresh lock no
// owner held, a heartbeat is a newer lock timestamp from the same owner,
// and a release is running: false. A lock the Conductor holds is the
// Conductor's, observed through its lease instead. A stale lock nobody
// refreshes is never turned into an acquisition by whichever session
// happens to read it.
func cliOwnershipEvents(run runrecord.Run, lock cliLock, session string, now time.Time) []runrecord.Event {
	if lock.conductor {
		return nil
	}
	owner := run.Owner
	cli := owner != nil && owner.OwnerKind == "cli"
	if session == "" {
		session = "cli"
	}
	acquire := func(at time.Time, reason string) runrecord.Event {
		return runrecord.Event{
			Type: runrecord.TypeOwnershipAcquired, ID: runrecord.StableID("cli-acquired", run.RunID, session, at.Format(time.RFC3339Nano)),
			At: at, SessionID: session, Ownership: &runrecord.Ownership{OwnerKind: "cli", OwnerID: session, Reason: reason},
		}
	}
	release := func(at time.Time, reason string) runrecord.Event {
		return runrecord.Event{
			Type: runrecord.TypeOwnershipReleased, ID: runrecord.StableID("cli-released", run.RunID, owner.OwnerID, owner.AcquiredAt.Format(time.RFC3339Nano)),
			At: at, Ownership: &runrecord.Ownership{OwnerKind: "cli", OwnerID: owner.OwnerID, Reason: reason},
		}
	}
	fresh := lock.held && now.Sub(lock.at) < cliLockStale && !lock.at.After(now.Add(cliLockStale))
	switch {
	case lock.released:
		if cli {
			return []runrecord.Event{release(now, "running: false")}
		}
	case lock.held && !cli:
		if fresh {
			return []runrecord.Event{acquire(lock.at, "running: lock taken")}
		}
	case lock.held:
		last := owner.AcquiredAt
		if owner.LastHeartbeat != nil {
			last = *owner.LastHeartbeat
		}
		if !lock.at.After(last) {
			return nil
		}
		if session != owner.OwnerID && fresh && lock.at.Sub(last) >= cliLockStale {
			// /iterate's stale-lock rule: another session took a lock its
			// owner stopped refreshing. The takeover follows the release by
			// one microsecond so the (time, id) fold keeps that order.
			return []runrecord.Event{
				release(lock.at, "stale running: lock taken over by session "+session),
				acquire(lock.at.Add(time.Microsecond), "took over a stale running: lock"),
			}
		}
		return []runrecord.Event{{
			Type: runrecord.TypeOwnershipBeat, ID: runrecord.StableID("cli-heartbeat", run.RunID, owner.OwnerID, lock.at.Format(time.RFC3339Nano)),
			At: lock.at, Ownership: &runrecord.Ownership{OwnerKind: "cli", OwnerID: owner.OwnerID, Reason: "running: heartbeat"},
		}}
	}
	return nil
}

// openBlockers lists the run's blockers with an observed detection and no
// clearance.
func openBlockers(run runrecord.Run) []runrecord.BlockerView {
	var out []runrecord.BlockerView
	for _, b := range run.Blockers {
		if b.ClearedAt == nil && b.DetectedAt != nil {
			out = append(out, b)
		}
	}
	return out
}

func blockerKind(status string) string {
	switch {
	case strings.HasPrefix(status, "blocked-on-operator"), strings.HasPrefix(status, "awaiting-human-gate"):
		return "operator"
	case strings.Contains(status, "quota"):
		return "quota"
	}
	return "unknown"
}

// cliBlockerEvents observes the plan's blocked status. A blocked plan with
// no open blocker records a detection; a plan that is no longer blocked
// clears every open blocker, but only when the plan file changed after the
// detection, so a Conductor blocker recorded a moment before its status
// reaches the plan file is never cleared by a stale read.
func cliBlockerEvents(run runrecord.Run, ps PlanSummary, modTime, now time.Time) []runrecord.Event {
	state := recordLifecycleState(ps)
	open := openBlockers(run)
	if state == "blocked" {
		if len(open) > 0 {
			return nil
		}
		n := fmt.Sprint(len(run.Blockers))
		return []runrecord.Event{{
			Type: runrecord.TypeBlockerDetected, ID: runrecord.StableID("cli-blocker", run.RunID, n, ps.Status),
			At: now, Blocker: &runrecord.Blocker{Key: "plan-status-" + n, Kind: blockerKind(ps.Status), Reason: ps.Status},
		}}
	}
	if state == "" {
		return nil
	}
	var out []runrecord.Event
	for _, b := range open {
		if !modTime.After(*b.DetectedAt) {
			continue
		}
		out = append(out, runrecord.Event{
			Type: runrecord.TypeBlockerCleared, ID: runrecord.StableID("cli-blocker-cleared", run.RunID, b.Key, b.DetectedAt.Format(time.RFC3339Nano)),
			At: now, Blocker: &runrecord.Blocker{Key: b.Key, Kind: b.Kind, Reason: "plan status " + state},
		})
	}
	return out
}

// sameLifecycle reports whether a plan-file state adds nothing to the
// run's latest state: resumed is executing again.
func sameLifecycle(fileState, runState string) bool {
	return fileState == runState || (fileState == "executing" && runState == "resumed")
}

// admissionClearances clears the blockers still open when the Conductor
// admits the plan again: admission means the plan was no longer blocked,
// and the reason says the clearance itself went unobserved.
func admissionClearances(run runrecord.Run, at time.Time, assignment string) []runrecord.Event {
	var out []runrecord.Event
	for _, b := range openBlockers(run) {
		out = append(out, runrecord.Event{
			Type: runrecord.TypeBlockerCleared, ID: runrecord.StableID("conductor-blocker-cleared", run.RunID, b.Key, assignment),
			At: at, Blocker: &runrecord.Blocker{Key: b.Key, Kind: b.Kind, Reason: clearanceAtAdmission},
		})
	}
	return out
}

// conductorBeats remembers the last heartbeat bucket recorded per
// assignment, so a stream of provider events costs one record write per
// five minutes.
var conductorBeats sync.Map

// conductorHeartbeat is the ownership heartbeat a provider event proves:
// the assignment's owned child is still producing output. At most one per
// five-minute bucket.
func conductorHeartbeat(assignment string, at time.Time) (runrecord.Event, bool) {
	bucket := at.Truncate(5 * time.Minute).Format(time.RFC3339)
	if prev, ok := conductorBeats.Load(assignment); ok && prev.(string) == bucket {
		return runrecord.Event{}, false
	}
	conductorBeats.Store(assignment, bucket)
	return runrecord.Event{
		Type: runrecord.TypeOwnershipBeat, ID: runrecord.StableID("conductor", assignment, "heartbeat", bucket),
		Ownership: &runrecord.Ownership{OwnerKind: "conductor", OwnerID: assignment, PID: os.Getpid(), Reason: "provider event"},
	}, true
}

// Admission gates. A Candidate's deny code maps to the gate the run record
// names; a code the map does not know is "other".
var admissionGates = map[string]string{
	"disabled": "automation_disabled", "paused": "automation_disabled", "project_disabled": "automation_disabled",
	"automation_disabled": "automation_disabled", "subscription_disabled": "automation_disabled",
	"project_window": "schedule", "automation_window": "schedule", "occurrence_completed": "schedule",
	"occurrence_claimed": "queue", "account_busy": "queue", "secondary": "queue",
	"plan_blocked": "other", "operator_parked": "other", "checkpoint_reconciliation": "other",
	"executable_unavailable": "other", "transport_unavailable": "other", "account_unbound": "other",
}

// admissionObservation is one plan's outcome in one supervisor decision.
type admissionObservation struct {
	ProjectPath, ProjectID, Plan string
	Decision, Gate, Reason       string
}

func admissionGate(c conductor.Candidate, cliOwned bool) string {
	switch c.Code {
	case "project_busy":
		if cliOwned {
			return "ownership"
		}
		return "queue"
	case "legacy_gate":
		switch {
		case strings.HasPrefix(c.Reason, "project launch policy"):
			return "schedule"
		case strings.Contains(c.Reason, "manual iterate run lock"):
			return "ownership"
		}
		return "other"
	}
	if g, ok := admissionGates[c.Code]; ok {
		return g
	}
	if !c.Budget.Allowed && c.Code == c.Budget.Code {
		return "budget"
	}
	return "other"
}

// summarizeAdmission reduces a decision to one observation per plan it
// considered: admitted when the plan was selected, otherwise gated by the
// first gate that denied it. A plan a CLI run holds is gated by ownership.
func summarizeAdmission(d conductor.Decision, projects []model.Project) []admissionObservation {
	type key struct{ project, plan string }
	byPlan := map[key]*admissionObservation{}
	var order []key
	cliOwned := map[string]bool{}
	projectIDs := map[string]string{}
	for _, p := range projects {
		projectIDs[p.ID] = p.Path
	}
	for _, q := range d.Queues {
		for _, owner := range q.RunningOwners {
			if owner == "cli" {
				cliOwned[q.ProjectID] = true
			}
		}
	}
	put := func(k key, o admissionObservation) {
		if _, ok := byPlan[k]; !ok {
			order = append(order, k)
		}
		byPlan[k] = &o
	}
	for _, c := range d.Candidates {
		if c.Plan.Kind == "intake" || c.Plan.Name == "" || c.Project.Path == "" {
			continue
		}
		projectIDs[c.Project.ID] = c.Project.Path
		k := key{c.Project.Path, c.Plan.Name}
		prev, seen := byPlan[k]
		switch {
		case c.Selected:
			put(k, admissionObservation{c.Project.Path, c.Project.ID, c.Plan.Name, "admitted", "", "selected for an assignment"})
		case seen && prev.Decision == "admitted":
		case c.Eligible:
			put(k, admissionObservation{c.Project.Path, c.Project.ID, c.Plan.Name, "gated", "queue", "eligible; another plan was admitted first"})
		case seen:
			// The first denial in automation priority order decides.
		default:
			put(k, admissionObservation{c.Project.Path, c.Project.ID, c.Plan.Name, "gated", admissionGate(c, cliOwned[c.Project.ID]), c.Reason})
		}
	}
	for _, q := range d.Queues {
		path := projectIDs[q.ProjectID]
		for plan, owner := range q.RunningOwners {
			if owner != "cli" || path == "" {
				continue
			}
			put(key{path, plan}, admissionObservation{path, q.ProjectID, plan, "gated", "ownership", "a CLI /iterate run holds this plan"})
		}
		for _, pl := range q.Plans {
			if pl.ProjectPath == "" || pl.Kind == "intake" {
				continue
			}
			k := key{pl.ProjectPath, pl.Name}
			if _, ok := byPlan[k]; !ok {
				put(k, admissionObservation{pl.ProjectPath, q.ProjectID, pl.Name, "gated", "automation_disabled", "no enabled automation covers this project"})
			}
		}
	}
	out := make([]admissionObservation, 0, len(order))
	for _, k := range order {
		out = append(out, *byPlan[k])
	}
	return out
}

// admissionSeen caches each plan's last recorded decision in this process,
// so an unchanged decision costs no record read.
var admissionSeen sync.Map

// RecordAdmission mirrors one supervisor tick's admission decisions into
// the runs of the plans it considered. The decision is already made; the
// record observes it. Only a plan whose current execution already has a
// run is recorded, so considering a plan that never executed opens nothing,
// and a decision that repeats the run's latest one appends nothing.
func (ConductorPlanSource) RecordAdmission(d conductor.Decision, projects []model.Project) {
	at := d.At.UTC()
	if at.IsZero() {
		at = recordNow()
	}
	for _, o := range summarizeAdmission(d, projects) {
		if !runrecord.ValidName(o.Plan) {
			continue
		}
		path, err := safePlanPath(o.ProjectPath, o.Plan)
		if err != nil {
			continue
		}
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		// Every ownership change rewrites the plan file (running:), so its
		// modification time keys the cache: a repeated decision about an
		// unchanged plan costs one stat.
		cacheKey := o.ProjectPath + "\x00" + o.Plan
		sig := strings.Join([]string{o.Decision, o.Gate, o.Reason, fmt.Sprint(info.ModTime().UnixNano())}, "\x00")
		if prev, ok := admissionSeen.Load(cacheKey); ok && prev.(string) == sig {
			continue
		}
		ps, err := parsePlanFile(path, o.ProjectPath)
		if err != nil {
			continue
		}
		marker := recordMarker(ps)
		if marker == "" {
			continue
		}
		req := runrecord.OpenRequest{
			Key:    runrecord.PlanKey{ProjectPath: o.ProjectPath, ProjectID: o.ProjectID, Plan: o.Plan},
			Marker: marker, Source: runrecord.SourceConductor, At: at, NoOpen: true,
		}
		_, _, err = RecordStore().Observe(req, func(run runrecord.Run) []runrecord.Event {
			prevID, lastOwnership := "", ""
			if n := len(run.Admissions); n > 0 {
				last := run.Admissions[n-1]
				prevID = last.EventID
				changed := false
				for _, own := range run.Ownership {
					if own.At.After(last.At) {
						changed = true
					}
				}
				if !changed && last.Decision == o.Decision && last.Gate == o.Gate && last.Reason == o.Reason {
					return nil
				}
			}
			if n := len(run.Ownership); n > 0 {
				lastOwnership = run.Ownership[n-1].EventID
			}
			return []runrecord.Event{{
				Type: runrecord.TypeAdmission, ID: runrecord.StableID("conductor-admission", run.RunID, prevID, lastOwnership, o.Decision, o.Gate, o.Reason),
				At: at, ProjectID: o.ProjectID, Admission: &runrecord.Admission{Decision: o.Decision, Gate: o.Gate, Reason: o.Reason},
			}}
		})
		if err == nil {
			admissionSeen.Store(cacheKey, sig)
		}
	}
}
