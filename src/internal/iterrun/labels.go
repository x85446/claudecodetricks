package iterrun

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// LabelsPath holds the agent_id -> human label map, populated as the hook
// observes each Agent-tool dispatch complete (its PostToolUse response
// carries the new subagent's agentId; its PreToolUse carried our own
// description, e.g. "badger-app" for a team dispatch named that way).
func LabelsPath(cwd string) string {
	return filepath.Join(cwd, ".claude", "iterate", "registry", "agent-labels.json")
}

// ReadLabels returns the current agent_id -> label map, empty if none yet.
func ReadLabels(cwd string) (map[string]string, error) {
	data, err := os.ReadFile(LabelsPath(cwd))
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]string{}, nil
		}
		return nil, err
	}
	var m map[string]string
	if err := json.Unmarshal(data, &m); err != nil {
		return map[string]string{}, nil // corrupt file: start fresh rather than fail the hook
	}
	return m, nil
}

// SetLabel records agentID -> label, read-modify-write via a temp file +
// rename so a crash mid-write never leaves a half-written map on disk.
// Concurrent SetLabel calls can still race (last-write-wins) — acceptable
// here since collisions require two Agent-tool dispatches completing in the
// same instant, and losing one label just means that agent's timeline row
// falls back to its raw agent_id instead of a team name, not lost data.
func SetLabel(cwd, agentID, label string) error {
	path := LabelsPath(cwd)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	m, err := ReadLabels(cwd)
	if err != nil {
		return err
	}
	m[agentID] = label

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
