package iterrun

import (
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/runrecord"
	"regexp"
	"strings"
	"sync/atomic"
	"time"
)

// recordClock replaces the run record's observation clock; tests store a
// bounded clock in it. Nil is the wall clock.
var recordClock atomic.Pointer[func() time.Time]

// recordNow is the run record's observation clock.
func recordNow() time.Time {
	if f := recordClock.Load(); f != nil {
		return (*f)().UTC()
	}
	return time.Now().UTC()
}

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
