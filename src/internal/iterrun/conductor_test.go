package iterrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// writeConductorFixture writes a minimal conductor.md under
// <dir>/.claude/iterate/ for a test to read back via ReadConductorState.
func writeConductorFixture(t *testing.T, dir, content string) {
	t.Helper()
	iterateDir := filepath.Join(dir, ".claude", "iterate")
	if err := os.MkdirAll(iterateDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(iterateDir, "conductor.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestConductorStatusLineStoodDown confirms a tick: stood-down conductor
// renders as "stood down" with no next-tick figure at all, regardless of
// whatever cron/sweep data happens to still be on the file — a
// stood-down conductor is fully terminal, and showing a stale next-tick
// estimate would misreport it as still ticking.
func TestConductorStatusLineStoodDown(t *testing.T) {
	dir := t.TempDir()
	writeConductorFixture(t, dir, `---
enabled: true
cron:
tick: stood-down
sweeps: 9
---

## Sweep log
- [2026-09-10T10:00:00Z] wound down to watching after sweep 8 found nothing startable
- [2026-09-10T11:00:00Z] stood down after 12 watching ticks with no change
`)
	cs, ok := ReadConductorState(dir)
	if !ok {
		t.Fatal("ReadConductorState ok = false, want true")
	}
	label, show := ConductorStatusLine(cs, time.Date(2026, 9, 10, 11, 5, 0, 0, time.UTC))
	if !show {
		t.Fatal("ConductorStatusLine show = false, want true for an enabled conductor")
	}
	if label != "stood down" {
		t.Errorf("label = %q, want %q", label, "stood down")
	}
	if strings.Contains(label, "tick") || strings.Contains(label, "m") {
		t.Errorf("label = %q, want no next-tick figure at all for a stood-down conductor", label)
	}
}

// TestConductorStatusLineNoTrigger confirms the harness-degraded shape —
// enabled: true with an empty cron: (nothing can ever fire this
// conductor on its own, e.g. Codex with no scheduler of its own) — reads
// as "NO TRIGGER" rather than silently showing a bogus next-tick figure.
func TestConductorStatusLineNoTrigger(t *testing.T) {
	dir := t.TempDir()
	writeConductorFixture(t, dir, `---
enabled: true
cron:
tick: working
sweeps: 1
---

## Sweep log
- [2026-09-10T10:00:00Z] sweep 1: queue held 1 plan
`)
	cs, ok := ReadConductorState(dir)
	if !ok {
		t.Fatal("ReadConductorState ok = false, want true")
	}
	label, show := ConductorStatusLine(cs, time.Date(2026, 9, 10, 10, 2, 0, 0, time.UTC))
	if !show {
		t.Fatal("ConductorStatusLine show = false, want true for an enabled conductor")
	}
	if label != "NO TRIGGER" {
		t.Errorf("label = %q, want %q", label, "NO TRIGGER")
	}
}

// TestConductorStatusLineWorkingShowsNextTickMinutes confirms a live,
// working conductor with an armed cron shows its next tick in minutes,
// computed from the last recorded sweep plus the 5-minute working
// cadence the skill itself documents.
func TestConductorStatusLineWorkingShowsNextTickMinutes(t *testing.T) {
	dir := t.TempDir()
	writeConductorFixture(t, dir, `---
enabled: true
cron: 5435e2e7
current: tern
tick: working
sweeps: 5
---

## Sweep log
- [2026-09-10T10:00:00Z] sweep 5: tern in flight, no terminal lines
`)
	cs, ok := ReadConductorState(dir)
	if !ok {
		t.Fatal("ReadConductorState ok = false, want true")
	}
	// 2 minutes after the last sweep, on a 5-minute working cadence:
	// 3 minutes remain until the next tick.
	label, show := ConductorStatusLine(cs, time.Date(2026, 9, 10, 10, 2, 0, 0, time.UTC))
	if !show {
		t.Fatal("ConductorStatusLine show = false, want true for an enabled conductor")
	}
	if !strings.Contains(label, "working") {
		t.Errorf("label = %q, want it to name the working tick state", label)
	}
	if !strings.Contains(label, "next tick 3m") {
		t.Errorf("label = %q, want a \"next tick 3m\" figure (5m cadence - 2m elapsed)", label)
	}
}

// TestConductorStatusLineNotEnabledHidden confirms a conductor.md that
// exists but was never enabled (the common case — most projects that
// have ever run /ic at all still have a conductor.md sitting at
// enabled: false from being stopped) produces no dashboard line at all,
// rather than reporting a stale tick state as if it still applied.
func TestConductorStatusLineNotEnabledHidden(t *testing.T) {
	dir := t.TempDir()
	writeConductorFixture(t, dir, `---
enabled: false
cron:
tick: working
sweeps: 22
---

## Sweep log
- [2026-09-10T08:01:19Z] STOPPED. cron cancelled and verified gone.
`)
	cs, ok := ReadConductorState(dir)
	if !ok {
		t.Fatal("ReadConductorState ok = false, want true")
	}
	_, show := ConductorStatusLine(cs, time.Now())
	if show {
		t.Error("ConductorStatusLine show = true, want false for a disabled conductor")
	}
}

// TestReadConductorStateMissingFile confirms a project with no
// conductor.md at all (the overwhelming majority) reads back ok=false
// rather than an error or a zero-value state that looks like a real,
// disabled conductor.
func TestReadConductorStateMissingFile(t *testing.T) {
	dir := t.TempDir()
	if _, ok := ReadConductorState(dir); ok {
		t.Error("ReadConductorState ok = true, want false for a project with no conductor.md")
	}
}
