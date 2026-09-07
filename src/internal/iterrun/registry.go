// Package iterrun implements the iterate-run wrapper: it runs a command,
// streams its progress and heartbeat into a per-call registry entry so any
// agent (or the standalone `iterate-run status` view) can see real state
// instead of guessing from timers.
package iterrun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// Status values for an Entry.
const (
	StatusProgressing = "alive-progressing"
	StatusQuiet       = "alive-quiet"
	StatusStalled     = "stalled"
	StatusDone        = "done"
	StatusFailed      = "failed"
)

// Entry is the single JSON snapshot written by one iterate-run invocation.
// Each invocation owns exactly one entry file and is the only writer of it,
// so concurrent teams never race on the same file.
type Entry struct {
	Plan          string     `json:"plan"`
	Team          string     `json:"team,omitempty"`
	Unit          string     `json:"unit"`
	Command       string     `json:"command"`
	PID           int        `json:"pid"`
	Started       time.Time  `json:"started"`
	LastHeartbeat time.Time  `json:"last_heartbeat"`
	LastActivity  time.Time  `json:"last_activity"`
	Status        string     `json:"status"`
	LastMessage   string     `json:"last_message,omitempty"`
	Done          *int       `json:"done,omitempty"`
	Total         *int       `json:"total,omitempty"`
	Pct           *float64   `json:"pct,omitempty"`
	ExitCode      *int       `json:"exit_code,omitempty"`
	Finished      *time.Time `json:"finished,omitempty"`
}

var unsafeFilenameChars = regexp.MustCompile(`[^a-zA-Z0-9._-]+`)

func sanitize(s string) string {
	if s == "" {
		return "-"
	}
	return unsafeFilenameChars.ReplaceAllString(s, "-")
}

// RegistryDir returns the flat directory all entries are scanned from.
func RegistryDir(cwd string) string {
	return filepath.Join(cwd, ".claude", "iterate", "registry")
}

// entryPath is deterministic per (plan, team, unit) so a re-run of the same
// unit overwrites its own prior entry rather than accumulating duplicates.
func entryPath(dir string, e *Entry) string {
	team := e.Team
	if team == "" {
		team = "-"
	}
	name := fmt.Sprintf("%s.%s.%s.json", sanitize(e.Plan), sanitize(team), sanitize(e.Unit))
	return filepath.Join(dir, name)
}

// Write atomically overwrites this entry's own file. Safe to call from a
// single writer repeatedly (every heartbeat tick); never touches any other
// entry's file.
func (e *Entry) Write(dir string) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(e, "", "  ")
	if err != nil {
		return err
	}
	path := entryPath(dir, e)
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// Remove deletes this entry's own file (used after the status grace window).
func (e *Entry) Remove(dir string) error {
	return os.Remove(entryPath(dir, e))
}

// ScanRegistry reads every entry file in dir. Malformed files are skipped,
// not fatal — a status view shouldn't die because one writer crashed mid-write.
func ScanRegistry(dir string) ([]Entry, error) {
	matches, err := filepath.Glob(filepath.Join(dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var entries []Entry
	for _, m := range matches {
		if strings.HasSuffix(m, ".tmp") {
			continue
		}
		data, err := os.ReadFile(m)
		if err != nil {
			continue
		}
		var e Entry
		if err := json.Unmarshal(data, &e); err != nil {
			continue
		}
		entries = append(entries, e)
	}
	return entries, nil
}
