package iterrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestParsePlanFileBlockedOnOperator reproduces the exact shape found live
// in badger.md: a coordinator that finished everything it could and hit an
// external, human-only blocker (GitHub Actions billing) writes a "Next
// attempt" blockquote banner at the top of the file plus a "status:
// blocked-on-operator (...)" frontmatter line — both invented ad hoc by
// that coordinator, since nothing standardized this before. Confirms both
// get parsed and that Blocked() recognizes them.
func TestParsePlanFileBlockedOnOperator(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, ".claude", "iterate", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := `# Iterate Task — Flame on two platforms

> **Next attempt (one operator action):** GitHub Actions refused the final CI run for BILLING —
> "recent account payments have failed". Fix payment at github.com/settings/billing, then:
> ` + "`gh run rerun 31519284965`" + `

name: badger
Started: 2026-08-10 (planned)
phase: executing
running: false
status: blocked-on-operator (Actions billing) — all else green
teamed: true

## Goal
Bring Windows level with macOS.
`
	if err := os.WriteFile(filepath.Join(plansDir, "badger.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ps, err := GetPlanSummary(dir, "badger")
	if err != nil {
		t.Fatal(err)
	}

	if !ps.Blocked() {
		t.Errorf("Blocked() = false, want true for status %q", ps.Status)
	}
	if !strings.Contains(ps.NextAttempt, "GitHub Actions refused") {
		t.Errorf("NextAttempt = %q, want it to contain the banner text", ps.NextAttempt)
	}
	if !strings.Contains(ps.NextAttempt, "gh run rerun 31519284965") {
		t.Errorf("NextAttempt = %q, want the exact rerun command carried through", ps.NextAttempt)
	}
	if ps.Phase != "executing" {
		t.Errorf("Phase = %q, want %q — blocked-on-operator doesn't change phase, it's an additional signal", ps.Phase, "executing")
	}
}

// TestParsePlanFileNotBlocked confirms an ordinary plan (no status: line
// at all) never reads as blocked — Blocked() must default false, not
// panic or match on an empty string.
func TestParsePlanFileNotBlocked(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, ".claude", "iterate", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "name: finch\nStarted: 2026-08-09\nphase: executing\n\n## Goal\nSomething.\n"
	if err := os.WriteFile(filepath.Join(plansDir, "finch.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ps, err := GetPlanSummary(dir, "finch")
	if err != nil {
		t.Fatal(err)
	}
	if ps.Blocked() {
		t.Errorf("Blocked() = true for a plan with no status: line at all")
	}
	if ps.NextAttempt != "" {
		t.Errorf("NextAttempt = %q, want empty — no banner in this file", ps.NextAttempt)
	}
}

// TestParsePlanFileGoalTruncation confirms the card-list Goal field
// truncates at 160 chars (tightened from 220 after the dashboard card's
// own CSS was found to overflow at the old length) while GoalFull stays
// untruncated for the plan detail page.
func TestParsePlanFileGoalTruncation(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, ".claude", "iterate", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	long := strings.Repeat("a very long goal sentence describing the work ", 10) // well over 160 chars
	content := "name: otter\n\n## Goal\n" + long + "\n"
	if err := os.WriteFile(filepath.Join(plansDir, "otter.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ps, err := GetPlanSummary(dir, "otter")
	if err != nil {
		t.Fatal(err)
	}
	if len(ps.Goal) > 160+len("…") { // "…" is 3 bytes in UTF-8
		t.Errorf("Goal length = %d, want <= %d (160 chars + ellipsis)", len(ps.Goal), 160+len("…"))
	}
	if !strings.HasSuffix(ps.Goal, "…") {
		t.Errorf("Goal = %q, want it to end with an ellipsis after truncation", ps.Goal)
	}
	if len(ps.GoalFull) <= len(ps.Goal) {
		t.Errorf("GoalFull should be longer than the truncated Goal, got GoalFull=%d Goal=%d", len(ps.GoalFull), len(ps.Goal))
	}
}

// TestParsePlanFileExecutingDistinctFromStarted confirms Started: (drafting
// time, set once by /iterate-planner) and Executing: (real execution-start
// time, set once by /iterate at the planned->executing transition) parse
// independently — the dashboard's "running for" figure depends on these
// never getting conflated.
func TestParsePlanFileExecutingDistinctFromStarted(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, ".claude", "iterate", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "name: aardvark\nStarted: 2026-08-11 13:59:24 (planned)\nExecuting: 2026-08-11 16:32:55\nphase: executing\n\n## Goal\nSomething.\n"
	if err := os.WriteFile(filepath.Join(plansDir, "aardvark.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ps, err := GetPlanSummary(dir, "aardvark")
	if err != nil {
		t.Fatal(err)
	}

	started, ok := ps.StartedAt()
	if !ok || !started.Equal(time.Date(2026, 8, 11, 13, 59, 24, 0, time.Local)) {
		t.Errorf("StartedAt() = %v, %v; want 2026-08-11 13:59:24", started, ok)
	}
	executing, ok := ps.ExecutingAt()
	if !ok || !executing.Equal(time.Date(2026, 8, 11, 16, 32, 55, 0, time.Local)) {
		t.Errorf("ExecutingAt() = %v, %v; want 2026-08-11 16:32:55", executing, ok)
	}
}

// TestParsePlanFileExecutingAbsent confirms ExecutingAt() reports ok=false
// (not a zero-value false positive) for a plan that hasn't started
// executing yet, or predates the field entirely — callers must fall back
// to a different signal rather than trusting a zero time.
func TestParsePlanFileExecutingAbsent(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, ".claude", "iterate", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "name: mole\nStarted: 2026-08-11 (planned)\nphase: planned\n\n## Goal\nSomething.\n"
	if err := os.WriteFile(filepath.Join(plansDir, "mole.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	ps, err := GetPlanSummary(dir, "mole")
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := ps.ExecutingAt(); ok {
		t.Errorf("ExecutingAt() ok = true for a plan with no Executing: line, want false")
	}
}

// TestListArchivedPlans reproduces a real gap found live: a completed run
// (moved by /iterate to .claude/iterate/archive/<timestamp>-<name>-done.md
// on success, per its own SKILL.md) simply vanished from the dashboard —
// ListPlans only ever looked at plans/, never archive/. Confirms archived
// runs are discoverable, tagged Archived, and keep their real declared
// name (not the timestamped filename) plus the exact archive filename
// needed to re-locate them.
func TestListArchivedPlans(t *testing.T) {
	dir := t.TempDir()
	archiveDir := filepath.Join(dir, ".claude", "iterate", "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "name: aardvark\nStarted: 2026-08-11T13:59:24Z (planned)\nphase: complete\n\n## Goal\nShip the thing.\n"
	if err := os.WriteFile(filepath.Join(archiveDir, "20260812T012354Z-aardvark-done.md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	archived, err := ListArchivedPlans(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 {
		t.Fatalf("ListArchivedPlans returned %d entries, want 1", len(archived))
	}
	ps := archived[0]
	if !ps.Archived {
		t.Error("Archived = false, want true")
	}
	if ps.Name != "aardvark" {
		t.Errorf("Name = %q, want %q (from the frontmatter, not the timestamped filename)", ps.Name, "aardvark")
	}
	if ps.ArchiveFile != "20260812T012354Z-aardvark-done.md" {
		t.Errorf("ArchiveFile = %q, want the exact archived filename", ps.ArchiveFile)
	}
}
