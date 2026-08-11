package iterrun

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
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

// HandleHook is the entry point for `iterate-run hook pre|post`. It reads
// one HookInput JSON object from r, records an Event, and — specifically for
// an Agent-tool PostToolUse — resolves the new subagent's agent_id to the
// human label we gave it at dispatch time (its own "description"), so later
// events from that agent_id can be shown under a real team name instead of
// an opaque ID. Never fails loudly: a hook that errors blocks nothing by
// design (best-effort observability), so all errors are swallowed after an
// attempt — this must never be the thing that breaks a real tool call.
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
}

// resolvePlanTeam figures out which plan (and, for a team member, which
// team) this event belongs to. Two paths, tried in order:
//
//  1. cwd carries a .claude/iterate/current pointer — true for the
//     coordinator, which always runs from the plan's own project, and for
//     any team that happens to still be working there. plan comes from the
//     pointer file directly; team is empty (this is coordinator-level work).
//  2. agentID is already labeled — true for a team working in a directory
//     with no relation to the plan's project at all (confirmed live: a team
//     can and does clone an entirely separate workspace). The label is
//     always "<plan>-<team>" per /iterate's own dispatch convention, and
//     plan names are always a single word, so splitting on the first
//     hyphen is unambiguous.
//
// Neither may resolve yet (a label lags its own dispatch's first events) —
// that just means this one event goes untagged, not an error.
func resolvePlanTeam(cwd, agentID string) (plan, team string) {
	if cwd != "" {
		if data, err := os.ReadFile(filepath.Join(cwd, ".claude", "iterate", "current")); err == nil {
			if p := strings.TrimSpace(string(data)); p != "" {
				return p, ""
			}
		}
	}
	if agentID == "" {
		return "", ""
	}
	labels, err := ReadLabels()
	if err != nil {
		return "", ""
	}
	label, ok := labels[agentID]
	if !ok {
		return "", ""
	}
	p, t, found := strings.Cut(label, "-")
	if !found {
		return label, ""
	}
	return p, t
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
