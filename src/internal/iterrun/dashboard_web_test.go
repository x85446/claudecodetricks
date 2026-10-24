package iterrun

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TESTMASTER: id=dashboard-detail-independent-history tier=? parallel=no
func TestDashboardDetailIndependentHistory(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ITERATE_LEGACY_RUN_DIR", t.TempDir())
	project, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	RegisterProject(project)
	body := "# Iterate Task\nname: finch\nphase: executing\nStarted: 2026-10-20T00:00:00Z\nExecuting: 2026-10-20T00:00:00Z\n\n## Goal\nReadable plan goal\n\n## Steps\n- [ ] 1. Real task\n\n## Validation\n- [ ] 1. Real proof\n"
	for _, file := range []string{"plans/finch.md", "archive/20261021T000000Z-finch-done.md"} {
		path := filepath.Join(project, ".claude", "iterate", file)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(body), 0600); err != nil {
			t.Fatal(err)
		}
	}
	RegisterProject(project)
	start := time.Date(2026, 10, 20, 1, 0, 0, 0, time.UTC)
	for _, e := range []Event{
		{TS: start, Hook: "pre", Plan: "finch", CWD: project, ToolUseID: "call", ToolName: "Bash"},
		{TS: start.Add(time.Minute), Hook: "post", ToolUseID: "call", ToolName: "Bash", Summary: "Historical completed call"},
	} {
		if err := AppendEvent(e); err != nil {
			t.Fatal(err)
		}
	}
	for _, tc := range []struct {
		path    string
		handler http.HandlerFunc
	}{
		{"/plan?" + url.Values{"project": {project}, "name": {"finch"}}.Encode(), handlePlan},
		{"/archive?" + url.Values{"project": {project}, "file": {"20261021T000000Z-finch-done.md"}}.Encode(), handleArchive},
	} {
		t.Run(tc.path, func(t *testing.T) {
			// A blocked history scan must not block ordinary navigation.
			detailHistorySlot <- struct{}{}
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			rec := httptest.NewRecorder()
			tc.handler(rec, httptest.NewRequest("GET", tc.path, nil).WithContext(ctx))
			<-detailHistorySlot
			cancel()
			for _, want := range []string{"Readable plan goal", "Real task", "Real proof", "data-history-url=", "plan-recorded-content", `<main id="main"`, `href="#evidence-req-1"`, `id="evidence-req-1"`} {
				if !strings.Contains(rec.Body.String(), want) {
					t.Fatalf("initial page missing %q: %s", want, rec.Body)
				}
			}
			if rec.Code != 200 || strings.Contains(rec.Body.String(), "Historical completed call") {
				t.Fatal("initial page depends on history")
			}
			if rec.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("detail response can cache stale state")
			}
			rec = httptest.NewRecorder()
			tc.handler(rec, httptest.NewRequest("GET", tc.path+"&history=1", nil))
			if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Historical completed call") || strings.Contains(rec.Body.String(), "data-history-url=") {
				t.Fatalf("history enrichment lost unlabelled completion or recurses: %d %s", rec.Code, rec.Body)
			}
			// Canceled enrichment does not wait for another reader.
			detailHistorySlot <- struct{}{}
			ctx, cancel = context.WithCancel(context.Background())
			cancel()
			rec = httptest.NewRecorder()
			tc.handler(rec, httptest.NewRequest("GET", tc.path+"&history=1", nil).WithContext(ctx))
			<-detailHistorySlot
			if rec.Body.Len() != 0 {
				t.Fatal("canceled request rendered history")
			}
		})
	}
	// A history read error leaves the initial page usable and reports failure
	// to the enrichment client, which can offer a retry.
	if err := os.Remove(EventsPath()); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(EventsPath(), 0700); err != nil {
		t.Fatal(err)
	}
	path := "/plan?" + url.Values{"project": {project}, "name": {"finch"}}.Encode()
	rec := httptest.NewRecorder()
	handlePlan(rec, httptest.NewRequest("GET", path+"&history=1", nil))
	if rec.Code != 503 {
		t.Fatalf("history error status %d", rec.Code)
	}
	rec = httptest.NewRecorder()
	handlePlan(rec, httptest.NewRequest("GET", path, nil))
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "Readable plan goal") {
		t.Fatal("history failure hid plan")
	}
	if err := os.Remove(filepath.Join(project, ".claude", "iterate", "plans", "finch.md")); err != nil {
		t.Fatal(err)
	}
	rec = httptest.NewRecorder()
	handlePlan(rec, httptest.NewRequest("GET", path+"&history=1", nil))
	location := rec.Header().Get("Location")
	if rec.Code != 302 || !strings.HasPrefix(location, "/archive?") || !strings.Contains(location, "history=1") {
		t.Fatalf("archive handoff lost history request: %d %s", rec.Code, location)
	}
}

// TESTMASTER: id=indigobird-unified-detail-preservation tier=? parallel=yes
func TestDashboardUnifiedDetailPreservesTimeline(t *testing.T) {
	start := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	rows := []Row{{label: "coordinator", status: "in-progress", spans: []span{{start: start, end: start.Add(time.Hour), tool: "recorded tool", summary: "recorded evidence"}}, steps: []StepDetail{{Num: 1, Step: "Task evidence", Validation: "Validation evidence"}}}}
	p := PlanSummary{Name: "gar", GoalFull: "Actual historical goal", Archived: true, Started: start.Format(time.RFC3339), Finished: start.Add(time.Hour).Format(time.RFC3339)}
	chrome := PlanPageOptions{Chrome: dashboardDetailHeader("/workspace/fixture", "gar")}
	page := RenderPlanPage(ReportInput{Plan: p, Rows: rows, Now: start.Add(2 * time.Hour)}, chrome)
	for _, want := range []string{"Actual historical goal", "Requirements", "coordinator", "recorded evidence", "recorded open", "open at last recorded observation", "Dashboard", "/assets/dashboard/detail.js"} {
		if !strings.Contains(page, want) {
			t.Errorf("full detail missing %q", want)
		}
	}
	if strings.Contains(page, "<div class=\"l\">running</div>") || strings.Contains(page, "</span>still running</span>") {
		t.Fatal("archive implies current executor")
	}
	if strings.Count(page, "<!doctype html>") != 1 {
		t.Fatal("nested document instead of composed timeline")
	}
	for _, state := range []string{"blocked-on-operator: needs repair", "unblocked", "executing"} {
		p.Archived = false
		p.Status = state
		page = RenderPlanPage(ReportInput{Plan: p, Rows: rows, Now: start.Add(2 * time.Hour)}, chrome)
		for _, want := range []string{`id="plan-elapsed"`, "recorded open", "open at last recorded observation", "recorded evidence"} {
			if !strings.Contains(page, want) {
				t.Errorf("%s: historical detail missing %q", state, want)
			}
		}
		for _, misleading := range []string{"Running for", `class="l">running</div>`, `>running</span>`, ">still running</span>"} {
			if strings.Contains(page, misleading) {
				t.Errorf("%s: historical detail claims live execution: %q", state, misleading)
			}
		}
	}
}

// TESTMASTER: id=dashboard-unified-identity-errors tier=? parallel=no
func TestDashboardDetailIdentityAndMissingSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ITERATE_LEGACY_RUN_DIR", t.TempDir())
	project := t.TempDir()
	plans := filepath.Join(project, ".claude", "iterate", "plans")
	if err := os.MkdirAll(plans, 0700); err != nil {
		t.Fatal(err)
	}
	RegisterProject(project)
	for _, name := range []string{"../outside", "..", "a/b", "a\\b", "missing"} {
		r := httptest.NewRequest("GET", "/plan?"+url.Values{"project": {project}, "name": {name}}.Encode(), nil)
		w := httptest.NewRecorder()
		handlePlan(w, r)
		if w.Code != 404 {
			t.Fatalf("%q returned %d: %s", name, w.Code, w.Body)
		}
		if !strings.Contains(w.Body.String(), "Plan unavailable") || !strings.Contains(w.Body.String(), "All projects") {
			t.Fatalf("missing navigable error for %q", name)
		}
	}
	outside := filepath.Join(t.TempDir(), "secret.md")
	if e := os.WriteFile(outside, []byte("private fixture"), 0600); e != nil {
		t.Fatal(e)
	}
	if e := os.Symlink(outside, filepath.Join(plans, "escape.md")); e != nil {
		t.Fatal(e)
	}
	if _, e := safeDetailIdentity(project, "escape", "plans"); e == nil {
		t.Fatal("symlink escape accepted")
	}
}
