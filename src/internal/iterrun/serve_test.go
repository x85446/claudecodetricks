package iterrun

import (
	"net/http"
	"net/http/httptest"
	"net/url"
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

// TestDashboardSeparatesActiveAndArchivedTabs reproduces two things found
// live in the same session: a completed plan simply vanished from the
// dashboard once /iterate archived it (no ListArchivedPlans call
// anywhere), and there was no way to hide badge categories a visitor
// doesn't care about. Drives the actual rendering functions handleIndex
// calls (writeTagFilter, writeLiveProjectSection,
// writeArchivedProjectAccordion) directly against a real live+archived
// plan pair on disk — NOT through handleIndex/ListProjects, which read
// the real machine-wide project registry
// (~/.claude/iterate-run/projects.json) and would pollute it with this
// test's temp directory. Confirms: a data-tag on every card (live and
// archived), a checkbox for each distinct tag actually in play, live
// plans render in the project section (not archived ones), and archived
// plans render as their own collapsed per-project accordion row instead.
func TestDashboardSeparatesActiveAndArchivedTabs(t *testing.T) {
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

	var active, archivedTab strings.Builder
	writeLiveProjectSection(&active, dir, "proj", plans)
	writeArchivedProjectAccordion(&archivedTab, dir, "proj", archived)

	var filter strings.Builder
	writeTagFilter(&filter, tagSet)

	if !strings.Contains(active.String(), `data-tag="executing"`) {
		t.Errorf("expected the live executing plan's card to carry data-tag=\"executing\"; output:\n%s", active.String())
	}
	if strings.Contains(active.String(), "antelope") {
		t.Errorf("archived plan antelope leaked into the Active section's rendering; output:\n%s", active.String())
	}
	if !strings.Contains(archivedTab.String(), `data-tag="complete"`) {
		t.Errorf("expected the archived plan's card to carry data-tag=\"complete\" (its own declared phase); output:\n%s", archivedTab.String())
	}
	if !strings.Contains(archivedTab.String(), `<details class="archived-project">`) {
		t.Errorf("expected antelope's project to render as a collapsed archived-project row; output:\n%s", archivedTab.String())
	}
	if !strings.Contains(archivedTab.String(), "antelope") {
		t.Errorf("archived plan antelope missing from the Archived tab entirely; output:\n%s", archivedTab.String())
	}
	if !strings.Contains(filter.String(), `<input type="checkbox" class="tf-cb" data-tag="executing"`) {
		t.Errorf("expected a filter checkbox for the \"executing\" tag; output:\n%s", filter.String())
	}
	if !strings.Contains(filter.String(), `<input type="checkbox" class="tf-cb" data-tag="complete"`) {
		t.Errorf("expected a filter checkbox for the \"complete\" tag; output:\n%s", filter.String())
	}
}

// TestArchivedProjectAccordionOmittedWhenNoArchivedPlans confirms a
// project with zero archived runs contributes no row at all to the
// Archived tab — an always-visible empty accordion per project would be
// pure noise in a tab meant purely for browsing finished history.
func TestArchivedProjectAccordionOmittedWhenNoArchivedPlans(t *testing.T) {
	var b strings.Builder
	writeArchivedProjectAccordion(&b, "/some/project", "proj", nil)
	if b.Len() != 0 {
		t.Errorf("expected no output for a project with no archived plans, got:\n%s", b.String())
	}
}

// TestHarnessBadgeCoversAllThreeValues confirms harnessBadge never
// invents a fourth label — every PlanSummary.Harness value it can ever
// see (claude-code, codex, or the resolved unknown) maps to its own
// exact badge.
func TestHarnessBadgeCoversAllThreeValues(t *testing.T) {
	cases := []struct {
		harness   string
		wantLabel string
		wantClass string
	}{
		{"claude-code", "claude-code", "h-claude"},
		{"codex", "codex", "h-codex"},
		{"unknown", "unknown", "h-unknown"},
	}
	for _, c := range cases {
		label, class := harnessBadge(c.harness)
		if label != c.wantLabel || class != c.wantClass {
			t.Errorf("harnessBadge(%q) = (%q, %q), want (%q, %q)", c.harness, label, class, c.wantLabel, c.wantClass)
		}
	}
}

// TestLivePlanCardsCarryHarnessBadge confirms every plan card on the
// dashboard renders its own harness as a badge, and that a
// claude-code-only project's card list doesn't accidentally show codex
// (or vice versa) — each plan's own value, not a project-wide guess.
func TestLivePlanCardsCarryHarnessBadge(t *testing.T) {
	plans := []PlanSummary{
		{Name: "claudeplan", Harness: "claude-code"},
		{Name: "codexplan", Harness: "codex"},
		{Name: "oldplan", Harness: "unknown"},
	}
	var b strings.Builder
	writeLivePlans(&b, "/some/project", plans)
	out := b.String()
	if !strings.Contains(out, `<span class="hbadge h-claude">claude-code</span>`) {
		t.Errorf("missing claude-code harness badge; output:\n%s", out)
	}
	if !strings.Contains(out, `<span class="hbadge h-codex">codex</span>`) {
		t.Errorf("missing codex harness badge; output:\n%s", out)
	}
	if !strings.Contains(out, `<span class="hbadge h-unknown">unknown</span>`) {
		t.Errorf("missing unknown harness badge; output:\n%s", out)
	}
}

// TestHarnessRollupOnlyWhenMixed confirms the project-header rollup stays
// silent when every live plan shares one harness, and names both when a
// project genuinely mixes them — the whole point of the rollup is
// surfacing that mix, so showing it unconditionally would just be noise.
func TestHarnessRollupOnlyWhenMixed(t *testing.T) {
	uniform := []PlanSummary{{Name: "a", Harness: "claude-code"}, {Name: "b", Harness: "claude-code"}}
	if got := harnessRollup(uniform); got != "" {
		t.Errorf("harnessRollup(uniform claude-code) = %q, want empty", got)
	}
	mixed := []PlanSummary{{Name: "a", Harness: "claude-code"}, {Name: "b", Harness: "codex"}}
	if got := harnessRollup(mixed); got != "claude-code + codex" {
		t.Errorf("harnessRollup(mixed) = %q, want %q", got, "claude-code + codex")
	}
}

// TestWriteLiveProjectSectionRendersConductorStatus confirms an enabled
// conductor.md sitting next to a project's plans surfaces its tick state
// on the dashboard, driven through the real writeLiveProjectSection
// entry point (not just the lower-level ConductorStatusLine helper) so a
// regression in how the two are wired together would actually be caught.
func TestWriteLiveProjectSectionRendersConductorStatus(t *testing.T) {
	dir := t.TempDir()
	writeConductorFixture(t, dir, `---
enabled: true
cron:
tick: stood-down
sweeps: 3
---

## Sweep log
- [2026-09-10T09:00:00Z] stood down, nothing left to run
`)
	var b strings.Builder
	writeLiveProjectSection(&b, dir, "proj", nil)
	if !strings.Contains(b.String(), "stood down") {
		t.Errorf("expected the project section to surface the conductor's stood-down state; output:\n%s", b.String())
	}
}

func TestPlanRouteFollowsAPlanIntoTheArchive(t *testing.T) {
	proj := t.TempDir()
	plans := filepath.Join(proj, ".claude", "iterate", "plans")
	arch := filepath.Join(proj, ".claude", "iterate", "archive")
	for _, d := range []string{plans, arch} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	// Two runs of the same codename; the bare name means the most recent.
	body := "# Iterate Task — t\n\nname: galago\nStarted: 2026-09-12 (planned)\nExecuting: 2026-09-15T08:35:38Z\nFinished: 2026-09-15T19:22:10Z\nphase: executing\n\n## Goal\ng\n"
	for _, f := range []string{"20260901T000000Z-galago-done.md", "20260915T192210Z-galago-done.md"} {
		if err := os.WriteFile(filepath.Join(arch, f), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	req := httptest.NewRequest("GET", "/plan?project="+url.QueryEscape(proj)+"&name=galago", nil)
	rec := httptest.NewRecorder()
	handlePlan(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("status = %d, want %d — an archived plan's own page must not degrade to a summary-less shell", rec.Code, http.StatusFound)
	}
	loc := rec.Header().Get("Location")
	if !strings.Contains(loc, "/archive?") {
		t.Fatalf("Location = %q, want the archive route", loc)
	}
	if !strings.Contains(loc, "20260915T192210Z-galago-done.md") {
		t.Errorf("Location = %q, want the most recent run of this codename", loc)
	}

	// A live plan still renders in place.
	if err := os.WriteFile(filepath.Join(plans, "civet.md"), []byte("# Iterate Task — t\n\nname: civet\nphase: executing\n\n## Goal\ng\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	req = httptest.NewRequest("GET", "/plan?project="+url.QueryEscape(proj)+"&name=civet", nil)
	rec = httptest.NewRecorder()
	handlePlan(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("live plan status = %d, want 200", rec.Code)
	}

	// A name that never existed is not a redirect loop.
	req = httptest.NewRequest("GET", "/plan?project="+url.QueryEscape(proj)+"&name=nosuch", nil)
	rec = httptest.NewRecorder()
	handlePlan(rec, req)
	if rec.Code == http.StatusFound {
		t.Error("an unknown plan name redirected; want it rendered or errored, never bounced")
	}
}
