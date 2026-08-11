package iterrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
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
