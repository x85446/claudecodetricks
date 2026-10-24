package iterrun

import (
	"crypto/sha256"
	"fmt"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// layoutPages are the plan pages the layout cases render: a live executing
// plan, a completed plan that is not archived yet, and two archived
// executions, each through the dashboard's own handler.
type layoutPage struct {
	name, path string
	live       bool
}

// layoutProject copies the sanitized Waterbuck, Gemsbok and Albatross
// records into one registered project, adds a completed plan, and returns
// the project and its pages.
func layoutProject(t *testing.T) (string, []layoutPage) {
	t.Helper()
	project := integrityProject(t)
	copyIntegrityFixture(t, "waterbuck", integrityPlans(project))
	copyIntegrityFixture(t, "gemsbok", integrityArchive(project))
	copyIntegrityFixture(t, "albatross", integrityArchive(project))
	completed := "# Iterate Task — completed fixture\n\nname: dunnock\nphase: complete\nExecuting: 2026-10-21T08:00:00Z\nFinished: 2026-10-21T09:30:00Z\nharness: codex\n\n## Goal\nShip the dunnock change.\n\n## Steps\n- [x] 1. Write the change.\n- [x] 2. Merge it.\n\n## Validation\n- [x] 1. Its cases pass.\n- [x] 2. main holds the merge.\n"
	if err := os.WriteFile(filepath.Join(integrityPlans(project), "dunnock.md"), []byte(completed), 0o644); err != nil {
		t.Fatal(err)
	}
	q := func(k, v string) string {
		return url.Values{"project": {project}, k: {v}}.Encode()
	}
	return project, []layoutPage{
		{"live waterbuck", "/plan?" + q("name", "waterbuck"), true},
		{"completed dunnock", "/plan?" + q("name", "dunnock"), true},
		{"archived gemsbok", "/archive?" + q("file", "20261020T094038Z-gemsbok.md"), false},
		{"archived albatross", "/archive?" + q("file", "2026-09-11T02-27-19Z-albatross-done.md"), false},
	}
}

// getPage renders one dashboard page through its handler.
func getPage(t *testing.T, path string) string {
	t.Helper()
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	if strings.HasPrefix(path, "/archive") {
		handleArchive(rec, req)
	} else {
		handlePlan(rec, req)
	}
	if rec.Code != http.StatusOK {
		t.Fatalf("GET %s: %d %s", path, rec.Code, rec.Body.String())
	}
	return rec.Body.String()
}

// inOrder fails unless every marker appears, each after the one before.
func inOrder(t *testing.T, where, page string, markers ...string) {
	t.Helper()
	at := -1
	for _, m := range markers {
		i := strings.Index(page, m)
		if i < 0 {
			t.Errorf("%s: %q is missing", where, m)
			continue
		}
		if i < at {
			t.Errorf("%s: %q comes before the marker that should precede it", where, m)
		}
		at = i
	}
}

var reInPageLink = regexp.MustCompile(`href="#([^"]+)"`)

// TESTMASTER: id=kr-report-navigation tier=fast parallel=no
//
// The page reads top-down: application navigation, the plan header, the
// outcome, progress and evidence summary, the execution timeline, the
// breakdowns, and the Conductor activity last in a bounded scroller. Every
// in-page link resolves, every section states its availability, and the
// page carries no unlabelled stat card or second clock, for live, completed and archived
// plans, before and after tool history loads, with a large hook log.
func TestKrReportNavigation(t *testing.T) {
	project, pages := layoutProject(t)
	// A large hook log: a long-running live plan's history.
	start := time.Date(2026, 10, 21, 0, 0, 0, 0, time.UTC)
	for i := 0; i < 1500; i++ {
		at := start.Add(time.Duration(i) * 30 * time.Second)
		id := fmt.Sprintf("bulk-%d", i)
		for _, e := range []Event{
			{TS: at, Hook: "pre", SessionID: "wb-coord", Plan: "waterbuck", CWD: project, ToolName: "Bash", ToolUseID: id},
			{TS: at.Add(10 * time.Second), Hook: "post", SessionID: "wb-coord", ToolName: "Bash", ToolUseID: id, Summary: "step " + id},
		} {
			if err := AppendEvent(e); err != nil {
				t.Fatal(err)
			}
		}
	}

	for _, p := range pages {
		for _, history := range []bool{false, true} {
			path := p.path
			label := p.name + " (initial)"
			if history {
				path += "&history=1"
				label = p.name + " (history)"
			}
			page := getPage(t, path)

			// Reading order.
			order := []string{`class="skip-link" href="#main"`, `class="dashboard-detail-header"`, `aria-label="Breadcrumb"`,
				`<main id="main"`, `<h1>`, `class="report-toc"`, `<div id="plan-recorded-content">`, `id="plan-elapsed"`,
				`id="report-progress"`, `id="report-evidence"`, `id="plan-timeline"`, `id="report-time"`, `id="report-ownership"`,
				`id="report-rework"`, `id="report-usage"`, `id="report-coverage"`, `id="report-delivery"`}
			inOrder(t, label, page, append(order, `<footer>`, `</main>`)...)
			if n := strings.Count(page, "<main"); n != 1 {
				t.Errorf("%s: %d main landmarks, want 1", label, n)
			}
			if strings.Index(page, `<a `) != strings.Index(page, `<a class="skip-link"`) {
				t.Errorf("%s: the skip link is not the first link on the page", label)
			}

			// The history loader replaces #plan-recorded-content as one unit,
			// so everything history can change sits inside it.
			if history == strings.Contains(page, "data-history-url=") {
				t.Errorf("%s: history status line present=%v, want %v", label, !history, !history)
			}
			content := page[strings.Index(page, `<div id="plan-recorded-content">`):]
			for _, id := range []string{"plan-elapsed", "report-progress", "report-usage", "report-coverage", "plan-timeline"} {
				if !strings.Contains(content, `id="`+id+`"`) {
					t.Errorf("%s: #%s is outside the content the history loader replaces", label, id)
				}
			}

			// Every in-page link resolves: the section list, the requirement
			// grid, the gantt's evidence links and the progress list.
			for _, m := range reInPageLink.FindAllStringSubmatch(page, -1) {
				if !strings.Contains(page, `id="`+m[1]+`"`) {
					t.Errorf("%s: link #%s has no target", label, m[1])
				}
			}
			if !strings.Contains(page, `<a class="bd-cell bd-`) {
				t.Errorf("%s: no requirement grid links", label)
			}

			// Each section states its availability.
			for _, s := range []struct{ id, state string }{
				{"report-progress", `class="rs-state rs-`}, {"report-evidence", `class="rs-state rs-`},
				{"report-time", `data-status="`}, {"report-ownership", `data-status="`}, {"report-rework", `data-status="`},
				{"report-usage", `class="ra-badge`}, {"report-coverage", `class="ra-badge`}, {"report-delivery", `class="ra-badge`},
			} {
				sec := page[strings.Index(page, `id="`+s.id+`"`):]
				sec = sec[:strings.Index(sec, "</section>")]
				if !strings.Contains(sec, s.state) {
					t.Errorf("%s: %s has no explicit availability state", label, s.id)
				}
			}

			// No unlabelled stat cards, severe-gap judgments, token panel,
			// second clock or click-only control.
			for _, gone := range []string{`class="stats"`, `class="runbox"`, `gapmark`, `class="gaplist"`, "Where the time actually went",
				"severe gap", "Token spend", "tk-top", "note-modal", "bdShow", "onclick=", "Running for", "Recorded execution span"} {
				if strings.Contains(page, gone) {
					t.Errorf("%s: the page carries %q, which the report sections replace", label, gone)
				}
			}
		}
	}
}

// fixtureDigest hashes every file under the given roots.
func fixtureDigest(t *testing.T, roots ...string) map[string]string {
	t.Helper()
	out := map[string]string{}
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return nil
			}
			b, err := os.ReadFile(p)
			if err == nil {
				out[p] = fmt.Sprintf("%x", sha256.Sum256(b))
			}
			return nil
		})
	}
	return out
}

// TESTMASTER: id=kr-report-legacy tier=fast parallel=no
//
// Copied Albatross, Gemsbok and Waterbuck records render every detail the
// old page carried, in its new place, and mark what is partial: the plan
// header, each requirement's task and validation text, the team lanes and
// their bars, the time axis, the requirement grid and evidence, and the one
// elapsed figure. Rendering changes no plan file, no record and no
// Conductor state.
func TestKrReportLegacy(t *testing.T) {
	project, pages := layoutProject(t)
	roots := []string{filepath.Join(project, ".claude"), RecordDir(), os.Getenv("ITERATE_LEGACY_RUN_DIR")}
	before := fixtureDigest(t, roots...)

	for _, p := range pages {
		if p.name == "completed dunnock" {
			continue
		}
		page := getPage(t, p.path+"&history=1")
		switch p.name {
		case "archived albatross":
			mustContain(t, p.name, page,
				`<span class="archived-chip">archived</span>`, "Activity by team", "boot-chain", "stage-metal", "build-system",
				`class="bar`, `aria-label="Requirement 20, Complete`, `id="evidence-req-20"`,
				"Lane by lane", "Elapsed</span> <strong>1h 18m</strong>")
		case "archived gemsbok":
			mustContain(t, p.name, page,
				"Completed and archived", "PR 28", `aria-label="Requirement 4, Complete`, `id="evidence-req-4"`,
				"Elapsed</span> <strong>41m 15s</strong>", "/iterate gemsbok")
		case "live waterbuck":
			mustContain(t, p.name, page,
				"still open at", `aria-label="Requirement 8, `, `id="evidence-req-8"`,
				"Make every macOS bake delete its container on every outcome.",
				"Cases bake.cleanup.failed, bake.cleanup.green and bake.cleanup.interrupted pass.")
		}
		// Partial data is identified as partial, never as complete or zero.
		mustContain(t, p.name, page, "This execution has no run record", `data-status="partial"`)
		// The step and validation text of every requirement survives in the
		// lane disclosures, beside a link to its evidence.
		if !strings.Contains(page, `<a class="step-ev" href="#evidence-req-1">Evidence for requirement 1</a>`) {
			t.Errorf("%s: requirement 1's lane detail does not link to its evidence", p.name)
		}
		if strings.Count(page, `<span class="stepnum">`) < 2 {
			t.Errorf("%s: the lane disclosures lost the Na/Nb text", p.name)
		}
	}

	after := fixtureDigest(t, roots...)
	for path := range after {
		if _, ok := before[path]; !ok {
			t.Errorf("rendering created %s", path)
		}
	}
	for path, sum := range before {
		if after[path] != sum {
			t.Errorf("rendering modified %s", path)
		}
	}
}
