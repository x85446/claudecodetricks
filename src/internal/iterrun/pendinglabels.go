package iterrun

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// PendingSpawnLabelsPath holds a FIFO queue, per session_id, of human labels
// waiting to be assigned to the next subagent that starts in that session.
//
// This exists because Codex's spawn-agent tool response carries no agent id
// (confirmed empirically — its PostToolUse tool_response is just
// {"task_name": "..."}, nothing else), unlike Claude Code's Agent tool,
// whose response embeds the new subagent's agentId directly (see
// resolveDispatchLabel). Codex only reveals the agent_id once that subagent
// actually starts, via a separate SubagentStart hook event carrying
// agent_id + agent_type but no label. Correlating the two requires a queue:
// push the label when the coordinator's spawn call completes, pop it when
// the next SubagentStart in that same session arrives. Confirmed live (a
// real two-subagent parallel dispatch under Codex 0.147.0): each spawn's
// PostToolUse always completes, in call order, before the corresponding
// SubagentStart fires — so FIFO-per-session is a safe correlation key, not
// a guess. It's still a heuristic rather than a guaranteed unique join (Codex
// doesn't hand us one), so a queue desync just means a label falls back to
// the raw agent_id — the same graceful degradation SetLabel already has.
func PendingSpawnLabelsPath() string {
	return filepath.Join(StoreDir(), "pending-spawn-labels.json")
}

func readPendingSpawnLabels() (map[string][]string, error) {
	data, err := os.ReadFile(PendingSpawnLabelsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return map[string][]string{}, nil
		}
		return nil, err
	}
	var m map[string][]string
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string][]string{}, nil // corrupt file: start fresh rather than fail the hook
	}
	return m, nil
}

func writePendingSpawnLabels(m map[string][]string) error {
	path := PendingSpawnLabelsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// PushPendingSpawnLabel appends label to sessionID's queue.
func PushPendingSpawnLabel(sessionID, label string) error {
	if sessionID == "" || label == "" {
		return nil
	}
	m, err := readPendingSpawnLabels()
	if err != nil {
		return err
	}
	m[sessionID] = append(m[sessionID], label)
	return writePendingSpawnLabels(m)
}

// PopPendingSpawnLabel removes and returns the oldest queued label for
// sessionID. ok is false if the queue is empty or the session is unknown.
func PopPendingSpawnLabel(sessionID string) (label string, ok bool, err error) {
	if sessionID == "" {
		return "", false, nil
	}
	m, err := readPendingSpawnLabels()
	if err != nil {
		return "", false, err
	}
	q, exists := m[sessionID]
	if !exists || len(q) == 0 {
		return "", false, nil
	}
	label = q[0]
	if len(q) == 1 {
		delete(m, sessionID)
	} else {
		m[sessionID] = q[1:]
	}
	if err := writePendingSpawnLabels(m); err != nil {
		return "", false, err
	}
	return label, true, nil
}
