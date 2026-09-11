package iterrun

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// ConductorState is a project's ./.claude/iterate/conductor.md frontmatter,
// exactly as /iterate-conductor writes it — never a live query into
// whatever mechanism actually arms the ticking (CronList, launchd, an OS
// cron entry, a harness-specific scheduler — see the skill's own
// "Degraded mode" section), only what the file on disk currently says.
// Confirmed live against a real running conductor (symmail's
// .claude/iterate/conductor.md): enabled, cron, current, tick,
// watch-ticks, watch-bound, stood-down, notified, last-cycle,
// imported-issues, sweeps all coexist in one frontmatter block, plus a
// prose "## Sweep log" below it.
type ConductorState struct {
	Enabled bool
	// Cron is the armed job id (e.g. "5435e2e7"), or empty when nothing
	// fires this conductor on its own — either it was never started, or
	// the harness that started it (Codex today, per the skill's own
	// "Degraded mode" section) has no scheduler of its own to arm.
	Cron string
	// Tick is "working" (normal 5-minute cadence), "watching" (wound
	// down to a 60-minute cadence after a sweep found nothing
	// startable), or "stood-down" (cron cancelled outright — fully
	// terminal until a human clears something and restarts it). Empty
	// on a plain/older file that predates this field.
	Tick string
	// LastSweep/HasLastSweep is the timestamp of the most recent "## Sweep
	// log" entry, when one could be parsed — conductor.md never stores a
	// machine-clean "fires at" instant of its own, so this is the anchor
	// the dashboard computes "next tick" from instead.
	LastSweep    time.Time
	HasLastSweep bool
}

var (
	reConductorEnabled = regexp.MustCompile(`^enabled:\s*(.+)$`)
	reConductorCron    = regexp.MustCompile(`^cron:\s*(.*)$`)
	reConductorTick    = regexp.MustCompile(`^tick:\s*(.*)$`)
	// reSweepLogStamp matches one "## Sweep log" bullet's leading
	// bracketed UTC timestamp. Real entries are inconsistent about
	// seconds ("[2026-09-09T07:10:43Z]" vs "[2026-09-10T23:45Z]") — the
	// seconds group is optional and defaults to :00 when absent.
	reSweepLogStamp = regexp.MustCompile(`^-\s*\[(\d{4}-\d{2}-\d{2})T(\d{2}):(\d{2})(?::(\d{2}))?Z\]`)
)

// ReadConductorState reads <projectDir>/.claude/iterate/conductor.md. ok
// is false when the file doesn't exist at all — true for most projects,
// since the conductor is opt-in per project.
func ReadConductorState(projectDir string) (cs ConductorState, ok bool) {
	path := filepath.Join(projectDir, ".claude", "iterate", "conductor.md")
	f, err := os.Open(path)
	if err != nil {
		return ConductorState{}, false
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	inFront, frontDone := false, false
	for scanner.Scan() {
		line := scanner.Text()
		if strings.TrimSpace(line) == "---" {
			if !inFront && !frontDone {
				inFront = true
				continue
			}
			if inFront {
				inFront, frontDone = false, true
				continue
			}
		}
		if inFront {
			if m := reConductorEnabled.FindStringSubmatch(line); m != nil {
				cs.Enabled = strings.TrimSpace(m[1]) == "true"
			}
			if m := reConductorCron.FindStringSubmatch(line); m != nil {
				cs.Cron = strings.TrimSpace(m[1])
			}
			if m := reConductorTick.FindStringSubmatch(line); m != nil {
				cs.Tick = strings.TrimSpace(m[1])
			}
			continue
		}
		if m := reSweepLogStamp.FindStringSubmatch(line); m != nil {
			sec := "00"
			if m[4] != "" {
				sec = m[4]
			}
			ts := fmt.Sprintf("%sT%s:%s:%sZ", m[1], m[2], m[3], sec)
			if t, err := time.Parse(time.RFC3339, ts); err == nil {
				// Sweep log is append-only, oldest first — keep
				// overwriting so the LAST match wins (the newest entry).
				cs.LastSweep, cs.HasLastSweep = t, true
			}
		}
	}
	return cs, true
}

// Tick cadence per the skill's own documented rule: "arm a recurring cron
// firing /iterate-conductor — every 5 minutes, not every minute" for the
// normal working state; a sweep that finds nothing startable winds that
// down to 60 minutes (the "watching" state) before giving up entirely and
// standing down.
const (
	conductorWorkingInterval  = 5 * time.Minute
	conductorWatchingInterval = 60 * time.Minute
)

// ConductorStatusLine renders one project's conductor state into the
// three-way display the dashboard shows: "stood down" (cron cancelled,
// no next-tick figure — the fully degraded terminal state, checked
// first since a stood-down conductor's cron field is also empty and
// would otherwise misreport as NO TRIGGER), "NO TRIGGER" (enabled but
// nothing can ever fire it on its own — the harness-degraded shape,
// enabled: true with an empty cron:), or its tick label plus a "next
// tick Xm" figure computed from the last recorded sweep plus that
// tick's own interval. ok is false when there's no conductor.md at all,
// or it exists but was never enabled — most projects, and not worth a
// dashboard line.
func ConductorStatusLine(cs ConductorState, now time.Time) (label string, ok bool) {
	if !cs.Enabled {
		return "", false
	}
	if strings.TrimSpace(cs.Tick) == "stood-down" {
		return "stood down", true
	}
	if strings.TrimSpace(cs.Cron) == "" {
		return "NO TRIGGER", true
	}
	tick := strings.TrimSpace(cs.Tick)
	if tick == "" {
		tick = "working"
	}
	if !cs.HasLastSweep {
		return tick, true
	}
	interval := conductorWorkingInterval
	if tick == "watching" {
		interval = conductorWatchingInterval
	}
	remaining := cs.LastSweep.Add(interval).Sub(now)
	if remaining < 0 {
		remaining = 0
	}
	mins := int((remaining + 30*time.Second) / time.Minute)
	return fmt.Sprintf("%s &middot; next tick %dm", tick, mins), true
}
