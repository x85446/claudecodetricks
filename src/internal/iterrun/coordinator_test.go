package iterrun

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// daemon starts a long-lived process that stands in for a runtime host
// shared by several sessions, such as the Codex app-server.
func daemon(t *testing.T) int {
	t.Helper()
	c := exec.Command("sleep", "60")
	if e := c.Start(); e != nil {
		t.Fatal(e)
	}
	t.Cleanup(func() { _ = c.Process.Kill(); _ = c.Wait() })
	return c.Process.Pid
}

func coordFixture(t *testing.T) string {
	t.Helper()
	t.Setenv("ITERATE_RUN_SESSIONS_DIR", t.TempDir())
	p := t.TempDir()
	writeRecordPlan(t, p, "alpha", "phase: executing\ndepends: none\nrunning: false")
	return p
}

func planText(t *testing.T, p, name string) string {
	t.Helper()
	b, e := os.ReadFile(filepath.Join(p, ".claude", "iterate", "plans", name+".md"))
	if e != nil {
		t.Fatal(e)
	}
	return string(b)
}

func ownerOf(t *testing.T, p, name string, now time.Time) ExecutionOwner {
	t.Helper()
	o, e := readOwner(filepath.Join(p, ".claude", "iterate", "plans", name+".md"), now)
	if e != nil {
		t.Fatal(e)
	}
	return o
}

func tool(session, agent, phase, id string, now time.Time) {
	observeSessionHook(phase, hookInput{SessionID: session, AgentID: agent, ToolName: "Bash", ToolUseID: id}, now)
}

// TESTMASTER: id=coord-shared-daemon tier=? parallel=no
// Two sessions share one long-lived host process. The owning turn ends and
// the host stays alive: the claim is not driven, a new session reconciles
// and takes it, and two contenders cannot both win.
func TestCoordSharedDaemonTurnEndFreesTheClaim(t *testing.T) {
	p := coordFixture(t)
	pid, now := daemon(t), time.Now()
	if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: "thread-a", PID: pid}, now); e != nil {
		t.Fatal(e)
	}
	if o := ownerOf(t, p, "alpha", now); !o.CoordinatorLive || o.State != CoordCoordinating {
		t.Fatalf("fresh claim: %+v", o)
	}
	// Thread B, same host process, is refused while A's turn runs.
	if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: "thread-b", PID: pid}, now); e == nil || !strings.Contains(e.Error(), "project busy") {
		t.Fatalf("contender admitted during A's turn: %v", e)
	}
	// A's own later claim is the same coordinator.
	if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: "thread-a", PID: pid}, now); e != nil {
		t.Fatalf("owner refused its own claim: %v", e)
	}
	if d := DecideStop("thread-a", false, now); d.Block || d.Turn != TurnEnded {
		t.Fatalf("record-only stop: %+v", d)
	}
	later := now.Add(10 * time.Hour)
	o := ownerOf(t, p, "alpha", later)
	if o.Live || o.CoordinatorLive || o.State != CoordTurnEnded || !strings.Contains(o.Next, "reconcile alpha") {
		t.Fatalf("daemon alive, turn ended: %+v", o)
	}
	if ownerProcessIdentity(pid) == "" {
		t.Fatal("the shared process must still be alive for this case to mean anything")
	}
	if _, e := UpdatePlanOwnershipAs(p, "", "reconcile", Caller{Session: "thread-b"}, false, later); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(planText(t, p, "alpha"), "checkpoint-reconciliation: required") {
		t.Fatal("reconcile dropped the checkpoint marker")
	}
	var wg sync.WaitGroup
	wins := make(chan string, 2)
	for _, s := range []string{"thread-b", "thread-c"} {
		wg.Add(1)
		go func(s string) {
			defer wg.Done()
			if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: s, PID: pid}, later); e == nil {
				wins <- s
			}
		}(s)
	}
	wg.Wait()
	close(wins)
	if n := len(wins); n != 1 {
		t.Fatalf("%d contenders won the claim", n)
	}
}

// TESTMASTER: id=coord-helper-heartbeat tier=? parallel=no
// A helper keeps renewing after its coordinator's turn ends. Its heartbeat
// establishes nothing, cannot refresh a successor's claim, and its late
// release cannot clear that successor.
func TestCoordHelperCannotKeepOrClearAClaim(t *testing.T) {
	p := coordFixture(t)
	pid, now := daemon(t), time.Now()
	if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: "thread-a", PID: pid}, now); e != nil {
		t.Fatal(e)
	}
	DecideStop("thread-a", false, now)
	// The helper inherited A's session id from the shell that started it.
	helper := Caller{Session: "thread-a"}
	at := now.Add(time.Minute)
	if _, e := UpdatePlanOwnershipAs(p, "alpha", "heartbeat", helper, false, at); e == nil {
		t.Fatal("a helper heartbeat revived an ended turn")
	}
	if o := ownerOf(t, p, "alpha", at); o.Live {
		t.Fatalf("helper kept the claim live: %+v", o)
	}
	if _, e := UpdatePlanOwnershipAs(p, "alpha", "reconcile", Caller{}, false, at); e != nil {
		t.Fatal(e)
	}
	if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: "thread-b", PID: pid}, at); e != nil {
		t.Fatal(e)
	}
	before := ownerOf(t, p, "alpha", at).Heartbeat
	late := at.Add(time.Minute)
	for _, c := range []Caller{helper, {}} {
		if _, e := UpdatePlanOwnershipAs(p, "alpha", "heartbeat", c, false, late); e == nil {
			t.Fatalf("caller %+v refreshed the successor's claim", c)
		}
		if _, e := UpdatePlanOwnershipAs(p, "alpha", "release", c, false, late); e == nil {
			t.Fatalf("caller %+v released the successor's claim", c)
		}
	}
	o := ownerOf(t, p, "alpha", late)
	if !o.CoordinatorLive || o.Session != "thread-b" || !o.Heartbeat.Equal(before) {
		t.Fatalf("successor disturbed: %+v", o)
	}
	// The operator's scoped recovery is explicit and leaves a line.
	if _, e := UpdatePlanOwnershipAs(p, "alpha", "release", Caller{}, true, late); e != nil {
		t.Fatal(e)
	}
	if !strings.Contains(planText(t, p, "alpha"), "released by an operator with --force") {
		t.Fatal("forced release left no record")
	}
}

// TESTMASTER: id=coord-legacy-claims tier=? parallel=no
// A claim written before session binding names a shared process. With an
// old heartbeat it is not driven; with a helper's fresh heartbeat it is
// held and reported as unverified, never as coordination, and a session
// caller's heartbeat on it is accepted and refreshes nothing.
func TestCoordLegacyClaimIsNeverReportedAsCoordination(t *testing.T) {
	p := coordFixture(t)
	pid, now := daemon(t), time.Now()
	start := ownerProcessIdentity(pid)
	stale := now.Add(-10 * time.Hour).UTC().Format(time.RFC3339)
	fresh := now.Add(-20 * time.Second).UTC().Format(time.RFC3339)
	writeRecordPlan(t, p, "alpha", "phase: executing\ndepends: none\nowner: cli\nowner-pid: "+itoa(pid)+"\nowner-start: "+start+"\nheartbeat: "+stale+"\nrunning: "+stale)
	writeRecordPlan(t, p, "beta", "phase: executing\ndepends: none\nowner: cli\nowner-pid: "+itoa(pid)+"\nowner-start: "+start+"\nheartbeat: "+fresh+"\nrunning: "+fresh)
	writeRecordPlan(t, p, "marmot", "phase: executing\ndepends: none\nstatus: blocked-on-operator\nrunning: false")
	writeRecordPlan(t, p, "numbat", "phase: planned\ndepends: marmot\nrunning: false")

	a, b := ownerOf(t, p, "alpha", now), ownerOf(t, p, "beta", now)
	if a.Live || a.State != CoordUnverified || !a.EvidenceAt.Equal(a.Heartbeat) || !strings.Contains(a.Evidence, "unidentified caller") {
		t.Fatalf("old heartbeat on a shared process: %+v", a)
	}
	if !b.Live || b.State != CoordUnverified || !strings.Contains(b.Evidence, "unidentified caller") {
		t.Fatalf("fresh helper heartbeat: %+v", b)
	}
	claims, e := ProjectClaims(p, now)
	if e != nil || len(claims) != 2 {
		t.Fatalf("claims: %+v %v", claims, e)
	}
	// A helper carrying a session id heartbeats the legacy claim: accepted
	// so it keeps running, and nothing is refreshed.
	r, e := UpdatePlanOwnershipAs(p, "beta", "heartbeat", Caller{Session: "worker-thread"}, false, now)
	if e != nil || r.Note == "" {
		t.Fatalf("legacy heartbeat: %+v %v", r, e)
	}
	if got := ownerOf(t, p, "beta", now).Heartbeat; got.UTC().Format(time.RFC3339) != fresh {
		t.Fatalf("legacy heartbeat refreshed the claim to %s", got)
	}
	// Past the idle window the unrefreshed claim is no longer held.
	if o := ownerOf(t, p, "beta", now.Add(IdleWindow())); o.Live {
		t.Fatalf("unrefreshed legacy claim still held: %+v", o)
	}
	// Operator and dependency holds keep their own states.
	v, e := BuildGraphView(p)
	if e != nil {
		t.Fatal(e)
	}
	got := map[string]string{}
	for _, n := range v.Nodes {
		got[n.Name] = n.Status
	}
	if got["numbat"] != "waiting" {
		t.Fatalf("dependency wait lost: %v", got)
	}
	if hold, why := planHolds(planText(t, p, "marmot")); !hold || !strings.Contains(why, "blocked-on-operator") {
		t.Fatalf("operator block lost: %v %q", hold, why)
	}
}

func itoa(n int) string { b, _ := json.Marshal(n); return string(b) }

// TESTMASTER: id=coord-long-operation tier=? parallel=no
// A coordinator is alive through a long operation, then a mutating child
// outlives it. Contenders are refused throughout, and the report tells the
// coordinator, the surviving child and the eventual completion apart.
func TestCoordLongOperationThenSurvivingChild(t *testing.T) {
	p := coordFixture(t)
	now := time.Now()
	host := daemon(t)
	if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: "sess", PID: host}, now); e != nil {
		t.Fatal(e)
	}
	tool("sess", "", "pre", "call-1", now)
	mid := now.Add(IdleWindow() + time.Minute)
	if o := ownerOf(t, p, "alpha", mid); !o.CoordinatorLive || !strings.Contains(o.Evidence, "work in flight") {
		t.Fatalf("a tool call in flight past the idle window: %+v", o)
	}
	if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: "other", PID: host}, mid); e == nil {
		t.Fatal("contender admitted during a long operation")
	}
	child := exec.Command("sleep", "60")
	if e := child.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	if e := AttachPlanOwnerChildAs(p, "alpha", child.Process.Pid, Caller{Session: "other"}, mid); e == nil {
		t.Fatal("a stranger attached a child to the claim")
	}
	if e := AttachPlanOwnerChildAs(p, "alpha", child.Process.Pid, Caller{Session: "sess"}, mid); e != nil {
		t.Fatal(e)
	}
	tool("sess", "", "post", "call-1", mid)
	DecideStop("sess", false, mid)
	o := ownerOf(t, p, "alpha", mid)
	if o.CoordinatorLive || !o.ChildLive || !o.Live || o.State != CoordTurnEnded || !strings.Contains(o.Next, "child pid") {
		t.Fatalf("coordinator gone, child running: %+v", o)
	}
	if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: "other", PID: host}, mid); e == nil || !strings.Contains(e.Error(), "child pid") {
		t.Fatalf("contender admitted beside a mutating child: %v", e)
	}
	// The claiming session adopts its own child.
	tool("sess", "", "pre", "call-2", mid)
	if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: "sess", PID: host}, mid); e != nil {
		t.Fatalf("owner could not adopt its child: %v", e)
	}
	if o := ownerOf(t, p, "alpha", mid); o.ChildPID != child.Process.Pid {
		t.Fatalf("adoption dropped the child: %+v", o)
	}
	DecideStop("sess", false, mid)
	_ = child.Process.Kill()
	_ = child.Wait()
	if o := ownerOf(t, p, "alpha", mid); o.Live || o.ChildLive {
		t.Fatalf("child finished: %+v", o)
	}
}

// TESTMASTER: id=coord-conductor-path tier=? parallel=no
// The Conductor claims as its own process. That process is its coordinator,
// a session cannot touch its claim, and its child keeps exclusion after it.
func TestCoordConductorClaimStaysProcessBound(t *testing.T) {
	p := coordFixture(t)
	now := time.Now()
	if _, e := ClaimPlan(p, "alpha", "conductor", os.Getpid(), now); e != nil {
		t.Fatal(e)
	}
	late := now.Add(24 * time.Hour)
	if o := ownerOf(t, p, "alpha", late); !o.CoordinatorLive || o.State != CoordDedicated {
		t.Fatalf("a running Conductor with a delayed heartbeat: %+v", o)
	}
	if _, e := UpdatePlanOwnershipAs(p, "alpha", "release", Caller{Session: "stray"}, false, late); e == nil {
		t.Fatal("a session released the Conductor's claim")
	}
	if e := UpdatePlanOwnership(p, "alpha", "heartbeat", late); e != nil {
		t.Fatalf("the Conductor's own heartbeat: %v", e)
	}
	if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: "stray"}, late); e == nil || !strings.Contains(e.Error(), "conductor") {
		t.Fatalf("cli admitted over the Conductor: %v", e)
	}
}

// TESTMASTER: id=coord-stop-hook tier=? parallel=no
// The stop hook is the loop: a session with runnable work continues its
// turn, a session whose agents are in flight ends its turn and stays the
// coordinator, and every real ending lets the turn end.
func TestCoordStopHookContinuesOnlyRunnableWork(t *testing.T) {
	p := coordFixture(t)
	now := time.Now()
	if d := DecideStop("nobody", true, now); d.Block {
		t.Fatal("a session with no claim was continued")
	}
	if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: "sess", PID: os.Getpid()}, now); e != nil {
		t.Fatal(e)
	}
	d := DecideStop("sess", true, now)
	if !d.Block || !strings.Contains(d.Reason, "alpha") || !strings.Contains(d.Reason, "plan release alpha") {
		t.Fatalf("runnable work: %+v", d)
	}
	// Agents in flight: the turn ends, the claim stays driven.
	observeSessionHook("subagent-start", hookInput{SessionID: "sess", AgentID: "worker-1"}, now)
	if d = DecideStop("sess", true, now); d.Block || d.Turn != TurnWaiting {
		t.Fatalf("agents in flight: %+v", d)
	}
	if o := ownerOf(t, p, "alpha", now.Add(time.Hour)); !o.CoordinatorLive || o.State != CoordWaiting || !strings.Contains(o.Evidence, "1 agent") {
		t.Fatalf("waiting on an agent: %+v", o)
	}
	// The agent reports. Its result starts a turn, and with nothing in
	// flight the next stop continues: completions are collected without a
	// new prompt.
	observeSessionHook("subagent-stop", hookInput{SessionID: "sess", AgentID: "worker-1"}, now)
	if o := ownerOf(t, p, "alpha", now); o.CoordinatorLive {
		t.Fatalf("no agent, no turn: %+v", o)
	}
	tool("sess", "", "pre", "c1", now)
	tool("sess", "", "post", "c1", now)
	if d = DecideStop("sess", true, now); !d.Block {
		t.Fatalf("after the agent returned: %+v", d)
	}
	// A turn that will not work is not continued forever.
	n := 1
	for ; DecideStop("sess", true, now).Block; n++ {
		if n > MaxStopBlocks+1 {
			t.Fatal("stop hook continued without bound")
		}
	}
	if n != MaxStopBlocks {
		t.Fatalf("continued %d times, want %d", n, MaxStopBlocks)
	}
	// A declared wait ends the turn and holds the claim until it expires.
	tool("sess", "", "pre", "c2", now)
	until, e := DeclareWait(p, "alpha", "CI job 42", 30*time.Minute, Caller{Session: "sess"}, now)
	if e != nil {
		t.Fatal(e)
	}
	if d = DecideStop("sess", true, now); d.Block || d.Turn != TurnWaiting {
		t.Fatalf("declared wait: %+v", d)
	}
	if o := ownerOf(t, p, "alpha", until.Add(-time.Second)); !o.CoordinatorLive || o.Wait != "CI job 42" {
		t.Fatalf("inside the wait: %+v", o)
	}
	if o := ownerOf(t, p, "alpha", until.Add(time.Second)); o.CoordinatorLive || !strings.Contains(o.Evidence, "expired") {
		t.Fatalf("past the wait: %+v", o)
	}
	if _, e = DeclareWait(p, "alpha", "x", time.Minute, Caller{Session: "stray"}, now); e == nil {
		t.Fatal("a stranger declared a wait")
	}
	// Releasing without a terminal state is not an ending.
	tool("sess", "", "pre", "c3", now)
	r, e := UpdatePlanOwnershipAs(p, "alpha", "release", Caller{Session: "sess"}, false, now)
	if e != nil || r.Terminal {
		t.Fatalf("release of a runnable plan: %+v %v", r, e)
	}
	if d = DecideStop("sess", true, now); !d.Block {
		t.Fatalf("released with runnable work: %+v", d)
	}
	// A pause is.
	path := filepath.Join(p, ".claude", "iterate", "plans", "alpha.md")
	_ = os.WriteFile(path, []byte(setPlanField(planText(t, p, "alpha"), "status", "paused")), 0o644)
	tool("sess", "", "pre", "c4", now)
	if d = DecideStop("sess", true, now); d.Block || d.Turn != TurnEnded {
		t.Fatalf("paused plan: %+v", d)
	}
	if r, e = UpdatePlanOwnershipAs(p, "alpha", "release", Caller{Session: "sess"}, false, now); e != nil || !r.Terminal || r.State != "status: paused" {
		t.Fatalf("release of a paused plan: %+v %v", r, e)
	}
	// So is the plan leaving the project.
	_ = os.WriteFile(path, []byte(dropPlanField(planText(t, p, "alpha"), "status")), 0o644)
	_ = os.Remove(path)
	tool("sess", "", "pre", "c5", now)
	if d = DecideStop("sess", true, now); d.Block {
		t.Fatalf("archived plan: %+v", d)
	}
	if st := LoadSession("sess"); st == nil || len(st.Claims) != 0 {
		t.Fatalf("engagement outlived the plan: %+v", st)
	}
}

// TESTMASTER: id=coord-interrupted-resume tier=? parallel=no
// An interrupted coordinator stops being the owner at once. Its next turn
// claims the plan again and keeps what the plan recorded; a second
// recovery changes nothing.
func TestCoordInterruptedTurnResumesItsOwnClaim(t *testing.T) {
	p := coordFixture(t)
	now := time.Now()
	c := Caller{Session: "sess", PID: os.Getpid()}
	if _, e := ClaimPlanAs(p, "alpha", "cli", c, now); e != nil {
		t.Fatal(e)
	}
	tool("sess", "", "pre", "c1", now)
	MarkInterrupted("sess", now)
	if o := ownerOf(t, p, "alpha", now); o.Live || o.State != CoordInterrupted {
		t.Fatalf("interrupted: %+v", o)
	}
	for i := 0; i < 2; i++ {
		if _, e := UpdatePlanOwnershipAs(p, "alpha", "reconcile", c, false, now); e != nil {
			t.Fatal(e)
		}
		tool("sess", "", "pre", "c2", now)
		if _, e := ClaimPlanAs(p, "alpha", "cli", c, now); e != nil {
			t.Fatalf("resume %d: %v", i, e)
		}
	}
	text := planText(t, p, "alpha")
	if n := strings.Count(text, "reconciled a claim nobody was driving"); n != 1 {
		t.Fatalf("recovery recorded %d times", n)
	}
	if o := ownerOf(t, p, "alpha", now); !o.CoordinatorLive || o.Session != "sess" {
		t.Fatalf("resumed: %+v", o)
	}
}

// TESTMASTER: id=coord-reader-not-driver tier=? parallel=no
// A session that only reads a project another session has claimed -- a
// status check, a triage -- leaves no sign that the plan is being driven.
func TestCoordReadingSessionDoesNotLookLikeTheDriver(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	p := coordFixture(t)
	now := time.Now()
	if e := os.WriteFile(filepath.Join(p, ".claude", "iterate", "current"), []byte("alpha\n"), 0o644); e != nil {
		t.Fatal(e)
	}
	if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: "owner", PID: os.Getpid()}, now); e != nil {
		t.Fatal(e)
	}
	DecideStop("owner", false, now)
	payload := func(session string) string {
		b, _ := json.Marshal(map[string]string{"session_id": session, "cwd": p, "tool_name": "Bash", "tool_use_id": "t1"})
		return string(b)
	}
	HandleHook("pre", strings.NewReader(payload("triage")))
	if _, ok := HeartbeatAge(ProjectRoot(p), time.Now()); ok {
		t.Fatal("a reading session stamped the project heartbeat")
	}
	if o := ownerOf(t, p, "alpha", time.Now()); o.Live {
		t.Fatalf("a reading session revived the claim: %+v", o)
	}
	HandleHook("pre", strings.NewReader(payload("owner")))
	if _, ok := HeartbeatAge(ProjectRoot(p), time.Now()); !ok {
		t.Fatal("the owning session's work left no project heartbeat")
	}
	if o := ownerOf(t, p, "alpha", time.Now()); !o.CoordinatorLive {
		t.Fatalf("the owning session's tool call: %+v", o)
	}
}

// TESTMASTER: id=coord-stop-hook-wire tier=? parallel=no
// The hook entry points speak the runtime's wire format and ignore an
// agent's own stop.
func TestCoordStopHookWireFormat(t *testing.T) {
	p := coordFixture(t)
	if _, e := ClaimPlanAs(p, "alpha", "cli", Caller{Session: "sess", PID: os.Getpid()}, time.Now()); e != nil {
		t.Fatal(e)
	}
	var out strings.Builder
	HandleStopHook(strings.NewReader(`{"session_id":"sess","agent_id":"worker"}`), &out, true)
	if out.Len() != 0 {
		t.Fatalf("an agent's stop was answered: %s", out.String())
	}
	HandleStopHook(strings.NewReader(`{"session_id":"sess","stop_hook_active":false}`), &out, true)
	var d struct{ Decision, Reason string }
	if e := json.Unmarshal([]byte(out.String()), &d); e != nil || d.Decision != "block" || d.Reason == "" {
		t.Fatalf("continue decision: %q %v", out.String(), e)
	}
	out.Reset()
	HandleStopHook(strings.NewReader(`{"session_id":"sess"}`), &out, false)
	if out.Len() != 0 || LoadSession("sess").Turn != TurnEnded {
		t.Fatalf("record-only stop wrote %q", out.String())
	}
	HandleInterruptHook(strings.NewReader(`{"session_id":"sess"}`))
	if LoadSession("sess").Turn != TurnInterrupted {
		t.Fatal("interrupt not recorded")
	}
}
