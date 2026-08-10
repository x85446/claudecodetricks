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
	// Plan and Team are resolved best-effort at hook time — Plan from the
	// event's own cwd (via .claude/iterate/current, when the call happens
	// inside the plan's own project) or from the agent's <plan>-<team>
	// dispatch label (when it doesn't — confirmed live: a team can and does
	// work in a directory with no relation to the plan's own project, so
	// cwd alone can't be trusted). Team is only ever derived from the label.
	// Both are empty until label resolution catches up, which can lag the
	// very first events from a freshly dispatched subagent.
	Plan string `json:"plan,omitempty"`
	Team string `json:"team,omitempty"`
}

// StoreDir is the one global, cwd-independent directory every iterate-run
// hook invocation reads and writes — deliberately NOT under any project's
// own .claude/, because a dispatched team can work in a directory with no
// relation to the plan's project at all (confirmed live: win-media and
// win-provision run entirely inside a separate checkout). Centralizing here
// is what lets `timeline --plan <name>` find every team's activity in one
// place regardless of where each team actually worked.
func StoreDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		home = "."
	}
	return filepath.Join(home, ".claude", "iterate-run")
}

// EventsPath is the shared append-only log every hook invocation writes to.
func EventsPath() string {
	return filepath.Join(StoreDir(), "events.jsonl")
}

// AppendEvent appends one event as a compact JSON line.
func AppendEvent(e Event) error {
	path := EventsPath()
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
func ReadEvents() ([]Event, error) {
	path := EventsPath()
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
