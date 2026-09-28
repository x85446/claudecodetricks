package iterrun

import (
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Package iterrun, heartbeat: proof that a plan marked executing is
// actually being driven by something.
//
// `phase: executing` is written into a plan file by /iterate when a run
// starts and rewritten when it ends. A run that is killed, crashes, or
// has its terminal closed never gets to write the ending, so the plan
// stays "executing" forever and the status line stays green forever --
// green meant "a file says so", not "work is happening". The heartbeat
// makes the second claim checkable: every hook invocation stamps the
// project's file, and anything reading it can compare the age against a
// window.
//
// It is per-project, not per-plan, on purpose. The hook knows its cwd for
// nothing; resolving which plan a given tool call belongs to would mean
// trusting .claude/iterate/current, which a teamed run's subagents do not
// all agree on. Per-project is exactly true for the normal case (the
// conductor drives one plan at a time) and honestly weaker for the rare
// one: two plans executing in one project are both marked live when
// either is.

// HeartbeatDir holds one file per project, named by the project's
// absolute path with separators turned into dashes -- the same encoding
// Claude Code uses under ~/.claude/projects. It is deliberately not a
// hash: the status line is a shell script and has to be able to build
// this path with ${DIR//\//-} and no tooling.
func HeartbeatDir() string { return filepath.Join(StoreDir(), "heartbeats") }

// HeartbeatPath is the heartbeat file for one project directory.
func HeartbeatPath(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	return filepath.Join(HeartbeatDir(), strings.ReplaceAll(abs, string(os.PathSeparator), "-"))
}

// TouchHeartbeat records that this project just did something. Called on
// every hook invocation, so it must stay cheap and must never fail
// loudly -- a hook that errors blocks a real tool call, and liveness is
// observability, not correctness. The mtime is the signal; the contents
// are the same timestamp in seconds, for anything that would rather read
// than stat.
func TouchHeartbeat(dir string) {
	if dir == "" {
		return
	}
	path := HeartbeatPath(dir)
	if os.MkdirAll(filepath.Dir(path), 0o755) != nil {
		return
	}
	_ = os.WriteFile(path, []byte(strconv.FormatInt(time.Now().Unix(), 10)+"\n"), 0o644)
}

// HeartbeatAge is how long ago this project last showed activity, and
// whether there was a heartbeat to read at all. A missing file is not an
// error: it means nothing has run since this was installed, which reads
// the same as stale.
func HeartbeatAge(dir string, now time.Time) (time.Duration, bool) {
	info, err := os.Stat(HeartbeatPath(dir))
	if err != nil {
		return 0, false
	}
	return now.Sub(info.ModTime()), true
}

// DefaultLiveWindow is how recent a heartbeat has to be for a run to
// count as live. Fifteen minutes is deliberately generous: an /iterate
// lane can sit inside one long build, test suite or /loop wait without
// touching a tool, and calling that dead would be worse than calling a
// dead run live a few minutes too long. Override with ITERATE_LIVE_SECS.
const DefaultLiveWindow = 15 * time.Minute

// LiveWindow is DefaultLiveWindow unless ITERATE_LIVE_SECS overrides it.
func LiveWindow() time.Duration {
	if v := os.Getenv("ITERATE_LIVE_SECS"); v != "" {
		if secs, err := strconv.Atoi(v); err == nil && secs > 0 {
			return time.Duration(secs) * time.Second
		}
	}
	return DefaultLiveWindow
}

// IsLive reports whether dir has shown activity inside the live window.
func IsLive(dir string, now time.Time) bool {
	age, ok := HeartbeatAge(dir, now)
	return ok && age <= LiveWindow()
}
