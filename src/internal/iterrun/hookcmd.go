package iterrun

import (
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"
)

// hookInput mirrors the JSON Claude Code sends to PreToolUse/PostToolUse
// hooks on stdin. tool_input/tool_response vary in shape by tool, so they're
// decoded generically and picked apart per-tool in summarize().
type hookInput struct {
	SessionID  string          `json:"session_id"`
	CWD        string          `json:"cwd"`
	AgentID    string          `json:"agent_id"`
	AgentType  string          `json:"agent_type"`
	ToolName   string          `json:"tool_name"`
	ToolUseID  string          `json:"tool_use_id"`
	ToolInput  json.RawMessage `json:"tool_input"`
	ToolResp   json.RawMessage `json:"tool_response"`
	DurationMs *int            `json:"duration_ms"`
}

// HandleHook is the entry point for `iterate-run hook
// pre|post|subagent-start|subagent-stop`. It reads one HookInput JSON object
// from r and records an Event for pre/post. For a dispatch-tool PostToolUse
// (Claude Code's Agent tool, or Codex's spawn_agent) it resolves the new
// subagent's agent_id to the human label we gave it at dispatch time, so
// later events from that agent_id can be shown under a real team name
// instead of an opaque ID — see resolveDispatchLabel and
// PendingSpawnLabelsPath's doc comments for why the two platforms need two
// different mechanisms for the same outcome. Never fails loudly: a hook
// that errors blocks nothing by design (best-effort observability), so all
// errors are swallowed after an attempt — this must never be the thing that
// breaks a real tool call.
func HandleHook(phase string, r io.Reader) {
	data, err := io.ReadAll(r)
	if err != nil {
		return
	}
	var in hookInput
	if json.Unmarshal(data, &in) != nil {
		return
	}
	if in.CWD == "" {
		return
	}

	RegisterProject(in.CWD)

	// SubagentStart/SubagentStop carry no tool_name/tool_input — they're
	// Codex-only lifecycle events (Claude Code has no equivalent hook this
	// binary needs to act on), not a PreToolUse/PostToolUse observation, so
	// they never reach the Event-recording path below.
	if phase == "subagent-start" {
		handleCodexSubagentStart(in)
		return
	}
	if phase == "subagent-stop" {
		return
	}

	summary := summarize(in.ToolName, in.ToolInput)
	plan, team := resolvePlanTeam(in.CWD, in.AgentID)

	e := Event{
		TS:        time.Now().UTC(),
		Hook:      phase,
		SessionID: in.SessionID,
		AgentID:   in.AgentID,
		AgentType: in.AgentType,
		ToolName:  in.ToolName,
		ToolUseID: in.ToolUseID,
		Summary:   summary,
		Plan:      plan,
		Team:      team,
		CWD:       in.CWD,
	}

	if phase == "post" {
		e.DurationMs = in.DurationMs
		if success, ok := parseSuccess(in.ToolResp); ok {
			e.Success = &success
		}
	}

	_ = AppendEvent(e)

	// Resolve agent_id -> team label the moment a dispatch confirms it,
	// so subsequent events from that agent_id render under a real name.
	if phase == "post" && in.ToolName == "Agent" {
		if agentID, label, ok := resolveDispatchLabel(in.ToolInput, in.ToolResp); ok {
			_ = SetLabel(agentID, label)
		}
	}

	// Codex's own dispatch tool: same intent as the Claude Code path above,
	// but Codex's spawn_agent PostToolUse response never carries the new
	// agent's id (confirmed empirically — see PendingSpawnLabelsPath), so
	// the label has to be queued now and claimed later, at SubagentStart.
	if phase == "post" && isCodexSpawnTool(in.ToolName) {
		if label, ok := extractCodexSpawnTaskName(in.ToolInput); ok {
			_ = PushPendingSpawnLabel(in.SessionID, label)
		}
	}
}

// handleCodexSubagentStart claims this session's oldest queued spawn label
// (pushed by the PostToolUse handler above) the moment Codex confirms a
// subagent has actually started, and assigns it to the now-known agent_id.
func handleCodexSubagentStart(in hookInput) {
	if in.SessionID == "" || in.AgentID == "" {
		return
	}
	if label, ok, _ := PopPendingSpawnLabel(in.SessionID); ok {
		_ = SetLabel(in.AgentID, label)
	}
}

// isCodexSpawnTool reports whether toolName is Codex's subagent-dispatch
// tool. Confirmed empirically (Codex CLI 0.147.0): its hook payload reports
// tool_name as "collaborationspawn_agent" (no separator) for a spawn_agent
// call — not "Agent", the way Claude Code names its own dispatch tool.
// Matched with Contains rather than an exact string so a future Codex
// version renaming or re-prefixing this tool doesn't silently stop working.
func isCodexSpawnTool(toolName string) bool {
	return strings.Contains(toolName, "spawn_agent")
}

// extractCodexSpawnTaskName pulls the human label out of a Codex
// spawn_agent call's own tool_input. task_name is the only plain-text label
// field available — Codex encrypts tool_input.message before it reaches
// hooks (confirmed empirically: a Fernet-token-shaped blob, not the actual
// prompt text), so there is nothing else usable here.
func extractCodexSpawnTaskName(raw json.RawMessage) (string, bool) {
	if len(raw) == 0 {
		return "", false
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return "", false
	}
	name, _ := m["task_name"].(string)
	if name == "" {
		return "", false
	}
	return name, true
}

// resolvePlanTeam figures out which plan (and, for a team member, which
// team) this event belongs to.
//
// **A non-empty agentID is a subagent, never the coordinator.** That is the
// whole ordering rule here, and getting it backwards is what made every
// team's work show up as the coordinator's: teams share the plan's working
// tree by design (they never switch branches), so a cwd-first resolver
// matched `.claude/iterate/current` for team events too and returned them as
// coordinator-level work. Confirmed live on plan galago: 498 of the 881
// spans filed under its coordinator were actually its five teams', and the
// team rows — left with only their log file and their `iterate-run run`
// wrapped commands — showed 45 minutes of "severe downtime" during which
// reporting alone made 77 tool calls.
//
// Resolution order:
//
//  1. agentID non-empty → a team. The id is self-describing,
//     "a<plan>-<team>-<16 hex>", because /iterate names each dispatch
//     `<plan>-<team>`; parse it. A re-dispatch of the same team arrives as
//     `<team>-2`, and that trailing counter is dropped so the second
//     dispatch lands in the same row as the first — one team, one lane.
//  2. Still a team but the id does not parse → the label map, which is how
//     the older generation of agent ids was resolved.
//  3. Neither → keep the plan (from cwd when it is there) but key the row by
//     the raw agentID. An unrecognized subagent gets its own honest lane;
//     what it must never get is team "", which is the coordinator's key.
//  4. agentID empty → the coordinator. plan comes from cwd's
//     `.claude/iterate/current` pointer.
func resolvePlanTeam(cwd, agentID string) (plan, team string) {
	planFromCWD := func() string { return CurrentPlanName(cwd) }

	if agentID == "" {
		return planFromCWD(), ""
	}

	if p, t := parseAgentID(agentID); t != "" {
		return p, t
	}
	if labels, err := ReadLabels(); err == nil {
		if label, ok := labels[agentID]; ok {
			if p, t, found := strings.Cut(label, "-"); found {
				return p, t
			}
			return label, ""
		}
	}
	return planFromCWD(), agentID
}

// agentIDPattern matches the self-describing subagent id the harness builds
// from /iterate's dispatch name: a leading "a", the Agent call's own
// "<plan>-<team>" name, then a 16-hex-digit suffix.
var agentIDPattern = regexp.MustCompile(`^a([a-z0-9]+)-(.+)-[0-9a-f]{16}$`)

// redispatchSuffix matches the "-2", "-3" a re-dispatch of the same team
// carries, so both dispatches resolve to one team.
var redispatchSuffix = regexp.MustCompile(`-[0-9]+$`)

// parseAgentID pulls the plan and team out of a subagent id, returning
// empty strings for anything that is not one. Both must be non-empty for
// the result to be usable — a caller checks team.
func parseAgentID(agentID string) (plan, team string) {
	m := agentIDPattern.FindStringSubmatch(agentID)
	if m == nil {
		return "", ""
	}
	team = redispatchSuffix.ReplaceAllString(m[2], "")
	if team == "" {
		return "", ""
	}
	return m[1], team
}

// summarize extracts a short, tool-appropriate description from tool_input.
// Best-effort: an unrecognized or malformed tool_input just yields the tool
// name alone, never an error.
func summarize(toolName string, raw json.RawMessage) string {
	if len(raw) == 0 {
		return ""
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return ""
	}
	str := func(k string) string {
		if v, ok := m[k].(string); ok {
			return v
		}
		return ""
	}
	trunc := func(s string, n int) string {
		s = strings.ReplaceAll(s, "\n", " ")
		if len(s) > n {
			return s[:n] + "…"
		}
		return s
	}

	switch toolName {
	case "Bash":
		return trunc(str("command"), 100)
	case "Edit", "Write", "Read", "NotebookEdit":
		return str("file_path")
	case "Grep":
		return trunc(str("pattern"), 60)
	case "Glob":
		return trunc(str("pattern"), 60)
	case "Agent":
		if d := str("description"); d != "" {
			return d
		}
		return str("subagent_type")
	case "WebFetch", "WebSearch":
		if u := str("url"); u != "" {
			return u
		}
		return trunc(str("query"), 60)
	default:
		return ""
	}
}

// parseSuccess makes a best-effort read of whether a tool_response indicates
// success. Shapes vary by tool; absence of a clear signal returns ok=false
// rather than guessing.
func parseSuccess(raw json.RawMessage) (bool, bool) {
	if len(raw) == 0 {
		return false, false
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return false, false
	}
	if interrupted, ok := m["interrupted"].(bool); ok && interrupted {
		return false, true
	}
	if _, hasStderr := m["stderr"]; hasStderr {
		if s, _ := m["stderr"].(string); s != "" {
			return false, true
		}
		return true, true
	}
	if status, ok := m["status"].(string); ok {
		return status == "completed", true
	}
	return false, false
}

// resolveDispatchLabel pulls the new subagent's agent_id out of an Agent
// tool's PostToolUse response, and the human label out of the SAME call's
// tool_input.description (the label we chose when dispatching it, e.g.
// "badger-app" for a team dispatch named that way per iterate's convention).
func resolveDispatchLabel(toolInput, toolResp json.RawMessage) (agentID, label string, ok bool) {
	var resp struct {
		AgentID string `json:"agentId"`
	}
	if json.Unmarshal(toolResp, &resp) != nil || resp.AgentID == "" {
		return "", "", false
	}
	var in struct {
		Description  string `json:"description"`
		SubagentType string `json:"subagent_type"`
	}
	_ = json.Unmarshal(toolInput, &in)
	label = in.Description
	if label == "" {
		label = in.SubagentType
	}
	if label == "" {
		label = resp.AgentID
	}
	return resp.AgentID, label, true
}

// ExitQuiet is a small helper the hook subcommands use — a hook must never
// block a real tool call over an observability failure, so it always exits
// 0 regardless of what HandleHook encountered internally.
func ExitQuiet() {
	fmt.Print("") // no output; hooks that emit nothing are treated as pass-through
}
