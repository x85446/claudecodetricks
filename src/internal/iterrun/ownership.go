package iterrun

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/plangraph"
)

// ExecutionOwner describes a plan's execution claim and the evidence behind
// it. Live is what excludes another executor: the coordinator can still
// advance the plan, or a child it started can still cause side effects.
// The two are reported apart, because only the first means the plan is
// being driven.
type ExecutionOwner struct {
	Plan      string    `json:"plan"`
	Owner     string    `json:"owner"`
	PID       int       `json:"pid"`
	Identity  string    `json:"identity"`
	Session   string    `json:"session,omitempty"`
	Heartbeat time.Time `json:"heartbeat"`
	Live      bool      `json:"live"`
	// Claimed is true while the claim is recorded and not released.
	Claimed bool `json:"claimed"`
	// CoordinatorLive is true when the claiming coordinator can still
	// advance the plan.
	CoordinatorLive bool `json:"coordinator_live"`
	// State names the coordinator's condition; see the Coord constants.
	State string `json:"state,omitempty"`
	// Evidence is the latest substantive sign behind State, EvidenceAt its
	// time.
	Evidence   string    `json:"evidence,omitempty"`
	EvidenceAt time.Time `json:"evidence_at,omitempty"`
	// ChildPID is an owned child that outlives its coordinator, ChildLive
	// whether it still runs.
	ChildPID  int  `json:"child_pid,omitempty"`
	ChildLive bool `json:"child_live,omitempty"`
	// Wait is the coordinator's declared wait, when one is in force.
	Wait string `json:"wait,omitempty"`
	// Next is the action that moves a claim nobody is driving.
	Next string `json:"next,omitempty"`
}

func ownerField(text, key string) string {
	head, _, _ := strings.Cut(text, "\n## ")
	m := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:[ \t]*([^\r\n]*)`).FindStringSubmatch(head)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

func readOwner(path string, now time.Time) (ExecutionOwner, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return ExecutionOwner{}, err
	}
	s := string(b)
	o := ExecutionOwner{Plan: strings.TrimSuffix(filepath.Base(path), ".md"), Owner: ownerField(s, "owner"), Identity: ownerField(s, "owner-start"), Session: ownerField(s, "owner-session")}
	if o.Owner == "" && ownerField(s, "conductor-assignment") != "" {
		o.Owner = "conductor"
	}
	o.PID, _ = strconv.Atoi(ownerField(s, "owner-pid"))
	o.Heartbeat, _ = time.Parse(time.RFC3339, ownerField(s, "heartbeat"))
	if o.Heartbeat.IsZero() {
		o.Heartbeat, _ = time.Parse(time.RFC3339, ownerField(s, "running"))
	}
	if ownerField(s, "running") == "false" {
		o.State = CoordReleased
		return o, nil
	}
	o.Claimed = o.Owner != "" || o.PID > 0 || o.Session != "" || !o.Heartbeat.IsZero()
	processOK := true
	if o.PID > 0 {
		identity := ownerProcessIdentity(o.PID)
		processOK = identity != "" && o.Identity != "" && identity == o.Identity
	}
	fresh := !o.Heartbeat.IsZero() && now.Sub(o.Heartbeat) >= 0
	switch {
	case !o.Claimed:
	case !processOK:
		o.State, o.Evidence, o.EvidenceAt = CoordProcessGone, "the owning process is gone", o.Heartbeat
	case o.Session != "":
		// A session claimed it: the session's own turn is the evidence.
		o.CoordinatorLive, o.State, o.Evidence, o.EvidenceAt = coordinatorEvidence(LoadSession(o.Session), o.Heartbeat, now)
		if st := LoadSession(o.Session); st != nil && st.waitActive(now) {
			o.Wait = st.Wait.For
		}
	case o.Owner == "conductor" && o.PID > 0:
		// The Conductor claims with its own dedicated process, so that
		// process is the coordinator. A delayed heartbeat does not end it.
		o.CoordinatorLive, o.State, o.Evidence, o.EvidenceAt = true, CoordDedicated, "the Conductor process is running", o.Heartbeat
	case o.PID > 0:
		// A claim with a process and no session cannot say which
		// invocation made it: a shared daemon outlives every turn it
		// hosted. The heartbeat is all it offers, and nothing ties that
		// heartbeat to a coordinator.
		o.CoordinatorLive = fresh && now.Sub(o.Heartbeat) < IdleWindow()
		o.State, o.Evidence, o.EvidenceAt = CoordUnverified, "heartbeat from an unidentified caller; the claim names a process and no session", o.Heartbeat
	default:
		o.CoordinatorLive = fresh && now.Sub(o.Heartbeat) < 90*time.Second
		o.State, o.Evidence, o.EvidenceAt = CoordUnverified, "heartbeat only; the claim names no process and no session", o.Heartbeat
	}
	o.ChildPID, _ = strconv.Atoi(ownerField(s, "owner-child-pid"))
	if o.ChildPID > 0 {
		identity := ownerProcessIdentity(o.ChildPID)
		o.ChildLive = identity != "" && identity == ownerField(s, "owner-child-start")
	}
	o.Live = o.CoordinatorLive || o.ChildLive
	switch {
	case !o.Claimed, o.CoordinatorLive:
	case o.ChildLive:
		o.Next = fmt.Sprintf("child pid %d still runs; adopt it from the claiming session or wait for it before redispatch", o.ChildPID)
	default:
		o.Next = "iterate-run plan reconcile " + o.Plan + ", then claim; inspect retained evidence before replaying a side effect"
	}
	return o, nil
}

// ProjectExecutionOwner reads live ownership without changing lease evidence.
func ProjectExecutionOwner(project string, now time.Time) (ExecutionOwner, error) {
	paths, err := filepath.Glob(filepath.Join(project, ".claude", "iterate", "plans", "*.md"))
	if err != nil {
		return ExecutionOwner{}, err
	}
	for _, path := range paths {
		o, e := readOwner(path, now)
		if e != nil {
			return o, e
		}
		if o.Live {
			return o, nil
		}
	}
	return ExecutionOwner{}, nil
}

// ProjectClaims lists every recorded claim in a project, driven or not,
// without changing lease evidence. A status report reads this: a claim
// nobody is driving is exactly what it has to name.
func ProjectClaims(project string, now time.Time) ([]ExecutionOwner, error) {
	paths, err := filepath.Glob(filepath.Join(project, ".claude", "iterate", "plans", "*.md"))
	if err != nil {
		return nil, err
	}
	out := []ExecutionOwner{}
	for _, path := range paths {
		o, e := readOwner(path, now)
		if e != nil {
			return out, e
		}
		if o.Claimed {
			out = append(out, o)
		}
	}
	return out, nil
}

func ownershipLocked(project string, fn func() error) error {
	path := filepath.Join(project, ".claude", "iterate", "execution.lock")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err = ownerLockFile(f); err != nil {
		return err
	}
	defer ownerUnlockFile(f)
	return fn()
}

func writeOwnerPlan(path, text string) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".owner-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	if st, e := os.Stat(path); e == nil {
		_ = f.Chmod(st.Mode().Perm())
	}
	if _, err = f.WriteString(text); err != nil {
		f.Close()
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	return os.Rename(f.Name(), path)
}

// describeHold renders a live claim for a refused contender.
func describeHold(o ExecutionOwner, now time.Time) string {
	s := fmt.Sprintf("project busy: %s owns %s (pid %d", o.Owner, o.Plan, o.PID)
	if o.State != "" {
		s += ", " + o.State
	}
	if !o.EvidenceAt.IsZero() {
		s += fmt.Sprintf(", last evidence %s ago", now.Sub(o.EvidenceAt).Round(time.Second))
	}
	s += ")"
	if !o.CoordinatorLive && o.ChildLive {
		s += fmt.Sprintf("; its coordinator is gone and child pid %d still runs", o.ChildPID)
	}
	return s
}

// ClaimPlan claims for a caller that carries no session identity: the
// Conductor, or a process outside both runtimes.
func ClaimPlan(project, name, owner string, pid int, now time.Time) (ExecutionOwner, error) {
	return ClaimPlanAs(project, name, owner, Caller{PID: pid}, now)
}

// ClaimPlanAs atomically admits one ready plan and excludes every live
// project owner. A session that already holds the plan claims it again
// without contention: a later turn of the same coordinator is the same
// coordinator.
func ClaimPlanAs(project, name, owner string, caller Caller, now time.Time) (ExecutionOwner, error) {
	var result ExecutionOwner
	err := ownershipLocked(project, func() error {
		if owner != "cli" && owner != "conductor" {
			return fmt.Errorf("owner must be cli or conductor")
		}
		identity := ""
		if caller.PID > 0 {
			if identity = ownerProcessIdentity(caller.PID); identity == "" {
				return fmt.Errorf("owner pid %d has no observable process identity", caller.PID)
			}
		} else if caller.Session == "" {
			return fmt.Errorf("claim needs a session identity or an owner pid")
		}
		path, err := safePlanPath(project, name)
		if err != nil {
			return err
		}
		graph, err := plangraph.Load(project)
		if err != nil {
			return err
		}
		node, ok := graph.Node(name)
		if !ok || node.Archived {
			return fmt.Errorf("plan %s not found", name)
		}
		if len(graph.Errors) > 0 {
			return fmt.Errorf("plan graph: %s", strings.Join(graph.Errors, "; "))
		}
		if node.Status != plangraph.StatusReady {
			return fmt.Errorf("plan %s: %s", name, node.Reason)
		}
		held, err := ProjectExecutionOwner(project, now)
		if err != nil {
			return err
		}
		again := held.Live && held.Plan == name && caller.Session != "" && held.Session == caller.Session
		if held.Live && !again {
			return errors.New(describeHold(held, now))
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		text := string(b)
		pid := ""
		if caller.PID > 0 {
			pid = strconv.Itoa(caller.PID)
		}
		for k, v := range map[string]string{"owner": owner, "owner-pid": pid, "owner-start": identity, "owner-session": caller.Session, "heartbeat": now.UTC().Format(time.RFC3339), "running": now.UTC().Format(time.RFC3339)} {
			text = setPlanField(text, k, v)
		}
		if !again {
			// A fresh claim starts with no child. The same session
			// claiming again keeps the child it already owns.
			text = setPlanField(text, "owner-child-pid", "")
			text = setPlanField(text, "owner-child-start", "")
		}
		if owner == "cli" {
			text = setPlanField(text, "conductor-assignment", "")
		}
		if err = writeOwnerPlan(path, text); err != nil {
			return err
		}
		if caller.Session != "" {
			abs, e := filepath.Abs(project)
			if e != nil {
				abs = project
			}
			if e = updateSession(caller.Session, true, func(st *SessionState) {
				st.addClaim(abs, name)
				st.markActive(now, "plan claim")
				st.Blocks = 0
			}); e != nil {
				return e
			}
		}
		result, err = readOwner(path, now)
		return err
	})
	return result, err
}

// authorize decides whether caller may change the claim o. A claim belongs
// to whoever made it: a session-bound claim to that session, a Conductor
// claim to a caller outside every session. It returns skip when the call
// succeeds as a no-op: a claim that names a process and no session cannot
// tell its coordinator from a helper, so a heartbeat on it refreshes
// nothing, and a helper that loops on one keeps running.
func authorize(o ExecutionOwner, caller Caller, verb string) (skip bool, err error) {
	who := "an unidentified caller"
	if caller.Session != "" {
		who = "session " + caller.Session
	}
	switch {
	case o.Session != "":
		if caller.Session == o.Session {
			return false, nil
		}
		return false, fmt.Errorf("plan %s is claimed by session %s; %s cannot %s it", o.Plan, o.Session, who, verb)
	case o.Owner == "conductor":
		if caller.Session == "" || callerDescendsFrom(o.PID) {
			return false, nil
		}
		return false, fmt.Errorf("plan %s is claimed by the Conductor; %s cannot %s it", o.Plan, who, verb)
	case caller.Session == "" || (caller.PID > 0 && caller.PID == o.PID):
		return false, nil
	case verb == "heartbeat":
		return true, nil
	}
	return false, fmt.Errorf("plan %s carries a claim with no session; %s cannot %s it. Reconcile it with: iterate-run plan reconcile %s", o.Plan, who, verb, o.Plan)
}

// SetPlanOwnerChild retains exclusion while an owned child survives its supervisor.
func SetPlanOwnerChild(project, name string, pid int, identity string) error {
	return ownershipLocked(project, func() error {
		path, e := safePlanPath(project, name)
		if e != nil {
			return e
		}
		b, e := os.ReadFile(path)
		if e != nil {
			return e
		}
		text := setPlanField(string(b), "owner-child-pid", strconv.Itoa(pid))
		text = setPlanField(text, "owner-child-start", identity)
		return writeOwnerPlan(path, text)
	})
}

// AttachPlanOwnerChild observes a live child's identity for the heartbeat CLI.
func AttachPlanOwnerChild(project, name string, pid int) error {
	return AttachPlanOwnerChildAs(project, name, pid, Caller{}, time.Now())
}

// AttachPlanOwnerChildAs attaches a child to the claim caller holds.
func AttachPlanOwnerChildAs(project, name string, pid int, caller Caller, now time.Time) error {
	identity := ownerProcessIdentity(pid)
	if identity == "" {
		return fmt.Errorf("child pid %d has no observable process identity", pid)
	}
	path, e := safePlanPath(project, name)
	if e != nil {
		return e
	}
	o, e := readOwner(path, now)
	if e != nil {
		return e
	}
	if skip, e := authorize(o, caller, "attach a child to"); e != nil || skip {
		return e
	}
	return SetPlanOwnerChild(project, name, pid, identity)
}

// ReleaseReceipt is what a release reports: the plan and the state it was
// left in. A turn that ends on a plan ends on this line.
type ReleaseReceipt struct {
	Plan      string `json:"plan"`
	Operation string `json:"operation"`
	State     string `json:"state,omitempty"`
	Terminal  bool   `json:"terminal"`
	Note      string `json:"note,omitempty"`
}

// UpdatePlanOwnership refreshes, releases, or reconciles the named lease
// for a caller that carries no session identity.
func UpdatePlanOwnership(project, name, verb string, now time.Time) error {
	_, err := UpdatePlanOwnershipAs(project, name, verb, Caller{}, false, now)
	return err
}

// UpdatePlanOwnershipAs refreshes, releases, or reconciles the named lease.
// Heartbeat and release act only for the caller that holds the claim, so a
// helper that outlives its coordinator cannot refresh or clear a
// successor's claim. force lets an operator release a claim it does not
// hold; the release is logged in the plan.
func UpdatePlanOwnershipAs(project, name, verb string, caller Caller, force bool, now time.Time) (ReleaseReceipt, error) {
	receipt := ReleaseReceipt{Plan: name, Operation: verb}
	err := ownershipLocked(project, func() error {
		paths := []string{}
		if name != "" {
			p, e := safePlanPath(project, name)
			if e != nil {
				return e
			}
			paths = append(paths, p)
		} else {
			paths, _ = filepath.Glob(filepath.Join(project, ".claude", "iterate", "plans", "*.md"))
		}
		for _, path := range paths {
			o, err := readOwner(path, now)
			if err != nil {
				return err
			}
			b, err := os.ReadFile(path)
			if err != nil {
				return err
			}
			text := string(b)
			if verb == "reconcile" && o.Live {
				continue
			}
			if verb == "reconcile" && ownerField(text, "running") == "false" {
				continue
			}
			if verb == "heartbeat" {
				if !o.Claimed {
					return fmt.Errorf("plan %s has no live owner", o.Plan)
				}
				skip, err := authorize(o, caller, verb)
				if err != nil {
					return err
				}
				if skip {
					receipt.Note = "ignored: the claim names no session, so this heartbeat identifies no coordinator"
					continue
				}
				if !o.Live {
					return fmt.Errorf("plan %s has no live owner", o.Plan)
				}
				text = setPlanField(text, "heartbeat", now.UTC().Format(time.RFC3339))
				text = setPlanField(text, "running", now.UTC().Format(time.RFC3339))
				if caller.Session != "" {
					_ = updateSession(caller.Session, false, func(st *SessionState) {
						if st.Turn == TurnActive {
							st.LastEvent, st.LastEventWhat = now, "plan heartbeat"
						}
					})
				}
			} else {
				if verb == "release" && o.Claimed {
					if _, err := authorize(o, caller, verb); err != nil {
						if !force {
							return err
						}
						text += "\n- " + now.UTC().Format(time.RFC3339) + " — claim released by an operator with --force; it was held by " + ownerLabel(o) + ".\n"
					}
				}
				text = setPlanField(text, "running", "false")
				if verb == "release" {
					text = setPlanField(text, "conductor-assignment", "")
					hold, why := planHolds(text)
					receipt.Terminal = hold
					receipt.State = why
					if !hold {
						receipt.State = "phase: executing"
						receipt.Note = "not terminal: the plan is still executing with no hold"
					}
				}
				if verb == "reconcile" {
					if assignment := ownerField(text, "conductor-assignment"); assignment != "" {
						text = setPlanField(text, "reconciled-assignment", assignment)
						text = setPlanField(text, "conductor-assignment", "")
					}
				}
				if verb == "reconcile" && ownerField(text, "owner") != "" {
					text = setPlanField(text, "checkpoint-reconciliation", "required: inspect retained evidence before replaying side effects")
					text += "\n- " + now.UTC().Format(time.RFC3339) + " — reconciled a claim nobody was driving (" + ownerLabel(o) + ", " + o.State + "); checkpoint evidence retained for diagnosis.\n"
				}
			}
			if err = writeOwnerPlan(path, text); err != nil {
				return err
			}
		}
		return nil
	})
	return receipt, err
}

func ownerLabel(o ExecutionOwner) string {
	s := o.Owner
	if s == "" {
		s = "an unnamed owner"
	}
	if o.Session != "" {
		s += " session " + o.Session
	} else if o.PID > 0 {
		s += " pid " + strconv.Itoa(o.PID)
	}
	return s
}

// DeclareWait records that the coordinator holding name has started work
// that continues elsewhere, so its turn may end without the plan being
// abandoned. The claim stays live until the wait expires.
func DeclareWait(project, name, what string, d time.Duration, caller Caller, now time.Time) (time.Time, error) {
	if strings.TrimSpace(what) == "" {
		return time.Time{}, errors.New("a wait names what it waits for")
	}
	if d <= 0 || d > BusyWindow() {
		d = BusyWindow()
	}
	path, err := safePlanPath(project, name)
	if err != nil {
		return time.Time{}, err
	}
	o, err := readOwner(path, now)
	if err != nil {
		return time.Time{}, err
	}
	if o.Session == "" || caller.Session != o.Session || !o.Claimed {
		return time.Time{}, fmt.Errorf("plan %s is not claimed by this session", name)
	}
	until := now.Add(d)
	err = updateSession(caller.Session, true, func(st *SessionState) {
		st.Wait = &SessionWait{Plan: name, For: strings.TrimSpace(what), Until: until}
		st.LastEvent, st.LastEventWhat = now, "plan wait"
	})
	return until, err
}

func safePlanPath(project, name string) (string, error) {
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
		return "", errors.New("invalid plan name")
	}
	base := filepath.Join(project, ".claude", "iterate", "plans")
	path := filepath.Join(base, name+".md")
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil {
		return "", e
	}
	canonicalBase, e := filepath.EvalSymlinks(base)
	if e != nil {
		return "", e
	}
	if filepath.Dir(resolved) != canonicalBase {
		return "", errors.New("plan resolves outside project plan directory")
	}
	return path, nil
}

func setPlanField(text, key, value string) string {
	boundary := strings.Index(text, "\n## ")
	head, tail := text, ""
	if boundary >= 0 {
		head, tail = text[:boundary], text[boundary:]
	}
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:.*$`)
	if re.MatchString(head) {
		return re.ReplaceAllStringFunc(head, func(string) string { return key + ": " + value }) + tail
	}
	return key + ": " + value + "\n" + head + tail
}

// dropPlanField removes every key-block line for key, leaving the body alone.
func dropPlanField(text, key string) string {
	boundary := strings.Index(text, "\n## ")
	head, tail := text, ""
	if boundary >= 0 {
		head, tail = text[:boundary], text[boundary:]
	}
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:.*\n?`)
	return re.ReplaceAllString(head, "") + tail
}
