package iterrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestLiveBadgePrioritizesBlockedOverEverything confirms Blocked() wins
// last regardless of phase/completion — a plan can be 100% done by its
// Teams table and still be sitting there waiting on a human, which is the
// one state most worth surfacing.
func TestLiveBadgePrioritizesBlockedOverEverything(t *testing.T) {
	p := PlanSummary{Phase: "executing", Status: "blocked-on-operator (billing)", HasTeams: true, TeamsTotal: 3, TeamsDone: 3}
	label, class := liveBadge(p)
	if label != "needs you" || class != "b-blocked" {
		t.Errorf("liveBadge = (%q, %q), want (%q, %q)", label, class, "needs you", "b-blocked")
	}
}

// TestArchivedBadgeReflectsBlockedAndPhase confirms archivedBadge mirrors
// liveBadge's blocked-wins-last rule, and otherwise surfaces the archived
// file's own declared phase rather than a generic label — so a plan that
// gave up mid-run reads differently on the dashboard than one that
// actually finished.
func TestArchivedBadgeReflectsBlockedAndPhase(t *testing.T) {
	cases := []struct {
		name      string
		p         PlanSummary
		wantLabel string
		wantClass string
	}{
		{"blocked wins", PlanSummary{Phase: "complete", Status: "blocked-on-operator (x)"}, "needs you", "b-blocked"},
		{"phase surfaced", PlanSummary{Phase: "complete"}, "complete", "b-archived"},
		{"no phase falls back", PlanSummary{}, "archived", "b-archived"},
		{"stale executing phase falls back", PlanSummary{Phase: "executing"}, "archived", "b-archived"},
		{"stale planned phase falls back", PlanSummary{Phase: "planned"}, "archived", "b-archived"},
	}
	for _, c := range cases {
		label, class := archivedBadge(c.p)
		if label != c.wantLabel || class != c.wantClass {
			t.Errorf("%s: archivedBadge = (%q, %q), want (%q, %q)", c.name, label, class, c.wantLabel, c.wantClass)
		}
	}
}

// TestDashboardRendersTagFilterAndArchivedSection reproduces two things
// found live in the same session: a completed plan simply vanished from
// the dashboard once /iterate archived it (no ListArchivedPlans call
// anywhere), and there was no way to hide badge categories a visitor
// doesn't care about. Drives the actual rendering functions handleIndex
// calls (writeTagFilter, writeProjectSection) directly against a real
// live+archived plan pair on disk — NOT through handleIndex/ListProjects,
// which read the real machine-wide project registry
// (~/.claude/iterate-run/projects.json) and would pollute it with this
// test's temp directory. Confirms a data-tag on every card (live and
// archived), a checkbox for each distinct tag actually in play, and a
// rendered "Archived" section for a project whose only other plan has
// already finished.
func TestDashboardRendersTagFilterAndArchivedSection(t *testing.T) {
	dir := t.TempDir()
	plansDir := filepath.Join(dir, ".claude", "iterate", "plans")
	archiveDir := filepath.Join(dir, ".claude", "iterate", "archive")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(plansDir, "aardvark.md"), []byte("name: aardvark\nphase: executing\n\n## Goal\nWork.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(archiveDir, "20260812T012354Z-antelope-done.md"),
		[]byte("name: antelope\nphase: complete\n\n## Goal\nDone.\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	plans, err := ListPlans(dir)
	if err != nil {
		t.Fatal(err)
	}
	archived, err := ListArchivedPlans(dir)
	if err != nil {
		t.Fatal(err)
	}

	tagSet := map[string]bool{}
	for _, p := range plans {
		label, _ := liveBadge(p)
		tagSet[label] = true
	}
	for _, p := range archived {
		label, _ := archivedBadge(p)
		tagSet[label] = true
	}

	var b strings.Builder
	writeTagFilter(&b, tagSet)
	writeProjectSection(&b, dir, "proj", plans, archived)
	out := b.String()

	if !strings.Contains(out, `data-tag="executing"`) {
		t.Errorf("expected the live executing plan's card to carry data-tag=\"executing\"; output:\n%s", out)
	}
	if !strings.Contains(out, `data-tag="complete"`) {
		t.Errorf("expected the archived plan's card to carry data-tag=\"complete\" (its own declared phase); output:\n%s", out)
	}
	if !strings.Contains(out, `<input type="checkbox" class="tf-cb" data-tag="executing"`) {
		t.Errorf("expected a filter checkbox for the \"executing\" tag; output:\n%s", out)
	}
	if !strings.Contains(out, `<input type="checkbox" class="tf-cb" data-tag="complete"`) {
		t.Errorf("expected a filter checkbox for the \"complete\" tag; output:\n%s", out)
	}
	if !strings.Contains(out, `<details class="archived">`) {
		t.Errorf("expected an Archived section now that antelope has a card to show; output:\n%s", out)
	}
	if !strings.Contains(out, "antelope") {
		t.Errorf("archived plan antelope missing from the index entirely; output:\n%s", out)
	}
}
