package iterrun

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Event is one PreToolUse/PostToolUse observation. Written append-only —
// concurrent writers (the coordinator and every subagent, each invoking the
// hook independently) are safe because a single JSON line is well under
// PIPE_BUF, so POSIX guarantees each os.O_APPEND write lands atomically.
type Event struct {
	TS         time.Time `json:"ts"`
	Hook       string    `json:"hook"` // "pre" | "post"
	SessionID  string    `json:"session_id"`
	AgentID    string    `json:"agent_id,omitempty"`
	AgentType  string    `json:"agent_type,omitempty"`
	ToolName   string    `json:"tool_name"`
	ToolUseID  string    `json:"tool_use_id"`
	Summary    string    `json:"summary,omitempty"`
	DurationMs *int      `json:"duration_ms,omitempty"`
	Success    *bool     `json:"success,omitempty"`
}

// EventsPath is the shared append-only log every hook invocation writes to.
func EventsPath(cwd string) string {
	return filepath.Join(cwd, ".claude", "iterate", "events.jsonl")
}

// AppendEvent appends one event as a compact JSON line.
func AppendEvent(cwd string, e Event) error {
	path := EventsPath(cwd)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()

	data, err := json.Marshal(e)
	if err != nil {
		return err
	}
	_, err = f.Write(append(data, '\n'))
	return err
}

// ReadEvents reads every event in the log. Malformed lines are skipped, not
// fatal — a timeline shouldn't die because one write was torn by a crash.
func ReadEvents(cwd string) ([]Event, error) {
	path := EventsPath(cwd)
	f, err := os.Open(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	var events []Event
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 4*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var e Event
		if err := json.Unmarshal([]byte(line), &e); err != nil {
			continue
		}
		events = append(events, e)
	}
	return events, nil
}
