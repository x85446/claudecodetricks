package iterrun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Package iterrun, coordinator: evidence that the session holding a plan's
// claim can still advance it.
//
// A process that is alive proves nothing about coordination. Codex runs
// every thread inside one app-server daemon, so a claim keyed to that PID
// stays "live" for as long as the daemon runs, hours after the thread that
// took it sent its final answer. A heartbeat proves nothing either: any
// helper can call it in a loop. What does identify a coordinator is the
// session (Claude Code) or thread (Codex) that claimed the plan, and what
// shows it can still act is its own turn: the runtime's hooks report every
// tool call it makes, every subagent it starts, and the moment its turn
// ends.
//
// One file per claiming session records that evidence. It exists only for
// sessions that hold a claim, so a hook for any other session costs one
// stat.

// Turn states a coordinator session moves through.
const (
	TurnActive      = "active"      // the coordinator's own turn is running
	TurnWaiting     = "waiting"     // the turn ended while its agents or a declared wait continue
	TurnEnded       = "ended"       // the turn ended with nothing of its own in flight
	TurnInterrupted = "interrupted" // the runtime reported an interruption
)

// Coordinator states reported for a claim.
const (
	CoordCoordinating = "coordinating"
	CoordWaiting      = "waiting"
	CoordTurnEnded    = "turn-ended"
	CoordInterrupted  = "interrupted"
	CoordIdle         = "idle"
	CoordProcessGone  = "process-gone"
	CoordUnverified   = "unverified"
	CoordDedicated    = "dedicated-process"
	CoordReleased     = "released"
)

// DefaultIdleWindow is how long an active turn may go without one observed
// event before the coordinator counts as idle. It covers the gap between
// two tool calls, never a whole step.
const DefaultIdleWindow = 15 * time.Minute

// DefaultBusyWindow bounds a turn that has a tool call or an agent in
// flight, and a declared wait.
const DefaultBusyWindow = 2 * time.Hour

func windowEnv(key string, def time.Duration) time.Duration {
	if v := os.Getenv(key); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return def
}

// IdleWindow is DefaultIdleWindow unless ITERATE_IDLE_SECS overrides it.
func IdleWindow() time.Duration { return windowEnv("ITERATE_IDLE_SECS", DefaultIdleWindow) }

// BusyWindow is DefaultBusyWindow unless ITERATE_BUSY_SECS overrides it.
func BusyWindow() time.Duration { return windowEnv("ITERATE_BUSY_SECS", DefaultBusyWindow) }

// Caller identifies whoever invokes an ownership verb.
type Caller struct {
	// Session is the Claude Code session id or Codex thread id, empty for a
	// process that runs outside both.
	Session string
	// PID is the caller's long-lived process, 0 when unknown.
	PID int
}

// CallerFromEnv reads the identity the runtime exports to every command a
// session runs. ITERATE_RUN_SESSION overrides it for a wrapper that knows
// better.
func CallerFromEnv() Caller {
	var c Caller
	for _, k := range []string{"ITERATE_RUN_SESSION", "CLAUDE_CODE_SESSION_ID", "CODEX_THREAD_ID"} {
		if v := strings.TrimSpace(os.Getenv(k)); v != "" {
			c.Session = v
			break
		}
	}
	c.PID, _ = strconv.Atoi(strings.TrimSpace(os.Getenv("CLAUDE_PID")))
	return c
}

// SessionClaim names one plan a session has claimed.
type SessionClaim struct {
	Project string `json:"project"`
	Plan    string `json:"plan"`
}

// SessionWait is a pause the coordinator declared: work it started
// continues elsewhere and nothing is runnable until it reports.
type SessionWait struct {
	Plan  string    `json:"plan"`
	For   string    `json:"for"`
	Until time.Time `json:"until"`
}

// SessionState is the observed evidence for one coordinator session.
type SessionState struct {
	Session        string               `json:"session"`
	Turn           string               `json:"turn"`
	TurnAt         time.Time            `json:"turn_at"`
	LastEvent      time.Time            `json:"last_event"`
	LastEventWhat  string               `json:"last_event_what,omitempty"`
	LastAgentEvent time.Time            `json:"last_agent_event,omitempty"`
	OpenTools      map[string]time.Time `json:"open_tools,omitempty"`
	OpenAgents     map[string]time.Time `json:"open_agents,omitempty"`
	Wait           *SessionWait         `json:"wait,omitempty"`
	Claims         []SessionClaim       `json:"claims,omitempty"`
	Blocks         int                  `json:"blocks,omitempty"`
}

var sessionFileUnsafe = regexp.MustCompile(`[^A-Za-z0-9._-]`)

// SessionsDir holds one evidence file per claiming session.
// ITERATE_RUN_SESSIONS_DIR overrides it, and every test binary sets it.
func SessionsDir() string {
	if v := os.Getenv("ITERATE_RUN_SESSIONS_DIR"); v != "" {
		return v
	}
	return filepath.Join(StoreDir(), "sessions")
}

func sessionPath(session string) string {
	return filepath.Join(SessionsDir(), sessionFileUnsafe.ReplaceAllString(session, "_")+".json")
}

// LoadSession returns the evidence for session, or nil when it holds no
// claim and so has no file.
func LoadSession(session string) *SessionState {
	if session == "" {
		return nil
	}
	b, err := os.ReadFile(sessionPath(session))
	if err != nil {
		return nil
	}
	var st SessionState
	if json.Unmarshal(b, &st) != nil || st.Session == "" {
		return nil
	}
	return &st
}

// updateSession applies fn to session's evidence under an exclusive lock:
// a coordinator and its agents invoke hooks concurrently with one session
// id. With create false it does nothing for a session that has no file.
func updateSession(session string, create bool, fn func(*SessionState)) error {
	if session == "" {
		return nil
	}
	path := sessionPath(session)
	if !create {
		if _, err := os.Stat(path); err != nil {
			return nil
		}
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	lock, err := os.OpenFile(path+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer lock.Close()
	if err = ownerLockFile(lock); err != nil {
		return err
	}
	defer ownerUnlockFile(lock)
	st := LoadSession(session)
	if st == nil {
		if !create {
			return nil
		}
		st = &SessionState{Session: session}
	}
	fn(st)
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".session-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err = tmp.Write(b); err != nil {
		tmp.Close()
		return err
	}
	if err = tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), path)
}

func (st *SessionState) addClaim(project, plan string) {
	for _, c := range st.Claims {
		if c.Project == project && c.Plan == plan {
			return
		}
	}
	st.Claims = append(st.Claims, SessionClaim{Project: project, Plan: plan})
}

// markActive records an event from the coordinator's own turn.
func (st *SessionState) markActive(now time.Time, what string) {
	if st.Turn != TurnActive {
		st.Turn, st.TurnAt = TurnActive, now
	}
	st.LastEvent, st.LastEventWhat = now, what
	st.Wait = nil
}

// openAgentsFresh counts agents that started, have not stopped, and showed
// activity inside the busy window.
func (st *SessionState) openAgentsFresh(now time.Time) int {
	n := 0
	for _, started := range st.OpenAgents {
		last := started
		if st.LastAgentEvent.After(last) {
			last = st.LastAgentEvent
		}
		if now.Sub(last) < BusyWindow() {
			n++
		}
	}
	return n
}

func (st *SessionState) waitActive(now time.Time) bool {
	return st.Wait != nil && now.Before(st.Wait.Until)
}

// observeSessionHook folds one hook event into the session's evidence.
// agentID is empty for the coordinator's own turn.
func observeSessionHook(phase string, in hookInput, now time.Time) {
	_ = updateSession(in.SessionID, false, func(st *SessionState) {
		switch phase {
		case "subagent-start":
			if in.AgentID != "" {
				if st.OpenAgents == nil {
					st.OpenAgents = map[string]time.Time{}
				}
				st.OpenAgents[in.AgentID] = now
				st.LastAgentEvent = now
			}
			return
		case "subagent-stop":
			delete(st.OpenAgents, in.AgentID)
			st.LastAgentEvent = now
			return
		}
		if in.AgentID != "" {
			st.LastAgentEvent = now
			return
		}
		st.markActive(now, in.ToolName)
		st.Blocks = 0
		if in.ToolUseID == "" {
			return
		}
		if phase == "pre" {
			if st.OpenTools == nil {
				st.OpenTools = map[string]time.Time{}
			}
			st.OpenTools[in.ToolUseID] = now
		} else {
			delete(st.OpenTools, in.ToolUseID)
		}
	})
}

// coordinatorEvidence judges a session-bound claim. It returns whether the
// coordinator can still advance the plan, the state name, and the latest
// evidence with its time and source.
func coordinatorEvidence(st *SessionState, heartbeat time.Time, now time.Time) (live bool, state, evidence string, at time.Time) {
	if st == nil {
		// No hook has reported for this session: the claim's own heartbeat
		// is all there is, and it is bound to the session.
		live = !heartbeat.IsZero() && now.Sub(heartbeat) >= 0 && now.Sub(heartbeat) < IdleWindow()
		return live, CoordUnverified, "claim heartbeat, no hook evidence for this session", heartbeat
	}
	switch st.Turn {
	case TurnEnded:
		return false, CoordTurnEnded, "the coordinator's turn ended", st.TurnAt
	case TurnInterrupted:
		return false, CoordInterrupted, "the coordinator's turn was interrupted", st.TurnAt
	case TurnWaiting:
		if st.waitActive(now) {
			return true, CoordWaiting, "declared wait: " + st.Wait.For, st.TurnAt
		}
		if n := st.openAgentsFresh(now); n > 0 {
			at = st.TurnAt
			if st.LastAgentEvent.After(at) {
				at = st.LastAgentEvent
			}
			return true, CoordWaiting, strconv.Itoa(n) + " agent(s) in flight", at
		}
		if st.Wait != nil {
			return false, CoordIdle, "declared wait expired: " + st.Wait.For, st.Wait.Until
		}
		return false, CoordIdle, "the turn ended and no agent is in flight", st.TurnAt
	}
	at = st.LastEvent
	if heartbeat.After(at) {
		at = heartbeat
	}
	if st.LastAgentEvent.After(at) {
		at = st.LastAgentEvent
	}
	window := IdleWindow()
	evidence = "coordinator tool call"
	if st.LastEventWhat != "" {
		evidence += " (" + st.LastEventWhat + ")"
	}
	if len(st.OpenTools) > 0 || st.openAgentsFresh(now) > 0 {
		window = BusyWindow()
		evidence += ", work in flight"
	}
	if now.Sub(at) < window {
		return true, CoordCoordinating, evidence, at
	}
	return false, CoordIdle, "no coordinator activity inside the window; last " + evidence, at
}

// planHolds reports whether a plan's own state says nothing is runnable:
// it is not executing, or it waits on a human.
func planHolds(text string) (bool, string) {
	if phase := ownerField(text, "phase"); phase != "executing" {
		return true, "phase: " + phase
	}
	status := ownerField(text, "status")
	for _, p := range []string{"paused", "blocked", "awaiting-human-gate", "done", "closed", "complete"} {
		if strings.HasPrefix(status, p) {
			return true, "status: " + status
		}
	}
	return false, ""
}

// MaxStopBlocks is how many times in a row the stop hook continues a turn
// that made no tool call in between. Past it the turn ends, so a session
// that will not work cannot loop forever.
const MaxStopBlocks = 3

// StopDecision is what the stop hook tells the runtime.
type StopDecision struct {
	// Block is true when the turn continues.
	Block bool
	// Reason is the instruction handed back to the model when Block is set.
	Reason string
	// Turn is the state recorded for the session.
	Turn string
}

// DecideStop records that session's turn is ending and decides whether it
// may. With cont set, a session engaged on a plan that is executing, holds
// on nothing, and has nothing in flight continues its turn: this is the
// loop that carries a plan to a terminal state. Without cont it only
// records the ending.
func DecideStop(session string, cont bool, now time.Time) StopDecision {
	var d StopDecision
	_ = updateSession(session, false, func(st *SessionState) {
		var runnable []string
		kept := st.Claims[:0]
		for _, c := range st.Claims {
			path := filepath.Join(c.Project, ".claude", "iterate", "plans", c.Plan+".md")
			b, err := os.ReadFile(path)
			if err != nil {
				continue // archived or removed: the engagement is over
			}
			text := string(b)
			if other := ownerField(text, "owner-session"); other != "" && other != session && ownerField(text, "running") != "false" {
				continue // another coordinator holds it
			}
			if ownerField(text, "owner") == "conductor" && ownerField(text, "running") != "false" {
				continue
			}
			hold, _ := planHolds(text)
			if ownerField(text, "phase") == "executing" {
				kept = append(kept, c)
			}
			if !hold {
				runnable = append(runnable, c.Plan)
			}
		}
		st.Claims = kept
		waiting := st.waitActive(now) || st.openAgentsFresh(now) > 0
		switch {
		case len(runnable) > 0 && waiting:
			st.Turn, st.TurnAt = TurnWaiting, now
		case len(runnable) > 0 && cont && st.Blocks < MaxStopBlocks:
			st.Blocks++
			st.Turn = TurnActive
			st.LastEvent, st.LastEventWhat = now, "stop hook continued the turn"
			d.Block = true
			name := strings.Join(runnable, ", ")
			d.Reason = "iterate plan " + name + " is executing under this session and has runnable work. " +
				"Re-read the plan file and continue from the first unchecked step; claim it again if the claim was released. " +
				"A turn on this plan ends only one of three ways: the plan reaches a terminal state and `iterate-run plan release " + runnable[0] + "` prints it; " +
				"work this session started continues elsewhere and `iterate-run plan wait " + runnable[0] + " --for \"<what>\"` declares it; " +
				"or the plan carries `status: paused`."
		default:
			st.Turn, st.TurnAt = TurnEnded, now
			st.OpenTools = nil
		}
		d.Turn = st.Turn
	})
	return d
}

// MarkInterrupted records a runtime-reported interruption of session's turn.
func MarkInterrupted(session string, now time.Time) {
	_ = updateSession(session, false, func(st *SessionState) {
		st.Turn, st.TurnAt = TurnInterrupted, now
		st.OpenTools = nil
	})
}
