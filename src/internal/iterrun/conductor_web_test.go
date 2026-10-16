package iterrun

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

// TESTMASTER: id=indigobird-unified-detail-preservation tier=? parallel=yes
func TestConductorUnifiedDetailPreservesTimeline(t *testing.T) {
	start := time.Date(2026, 10, 6, 0, 0, 0, 0, time.UTC)
	rows := []Row{{label: "coordinator", status: "in-progress", spans: []span{{start: start, end: start.Add(time.Hour), tool: "recorded tool", summary: "recorded evidence"}}, steps: []StepDetail{{Num: 1, Step: "Task evidence", Validation: "Validation evidence"}}}}
	p := PlanSummary{Name: "gar", GoalFull: "Actual historical goal", Archived: true, Started: start.Format(time.RFC3339), Finished: start.Add(time.Hour).Format(time.RFC3339)}
	page := RenderTimelineWithChrome(rows, p, "/project?project=fixture", nil, conductorDetailHeader("/workspace/fixture", "gar"))
	for _, want := range []string{"Actual historical goal", "Requirements", "coordinator", "recorded evidence", "recorded open", "open at last recorded observation", "Conductor", "/assets/conductor/detail.js"} {
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
}

// TESTMASTER: id=indigobird-unified-identity-errors tier=? parallel=no
func TestConductorDetailIdentityAndMissingSource(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ITERATE_CONDUCTOR_DIR", t.TempDir())
	project := t.TempDir()
	plans := filepath.Join(project, ".claude", "iterate", "plans")
	if err := os.MkdirAll(plans, 0700); err != nil {
		t.Fatal(err)
	}
	svc := NewConductorService("")
	cfg, _ := svc.Config()
	cfg.Projects = []model.Project{{ID: "fixture", Name: "fixture", Path: project}}
	if _, e := svc.Apply(cfg.Revision, cfg); e != nil {
		t.Fatal(e)
	}
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

// TESTMASTER: id=indigobird-unenrolled-queue-discovery tier=? parallel=no
func TestConductorUnenrolledQueueDiscovery(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	store := t.TempDir()
	t.Setenv("ITERATE_CONDUCTOR_DIR", store)
	project := t.TempDir()
	plans := filepath.Join(project, ".claude", "iterate", "plans")
	if e := os.MkdirAll(plans, 0700); e != nil {
		t.Fatal(e)
	}
	if e := os.WriteFile(filepath.Join(plans, "finch.md"), []byte("# Finch\nphase: planned\nstatus: queued\nStarted: 2026-10-16T00:00:00Z\n\n## Goal\nQueued fixture\n\n## Steps\n- [ ] 1. Task\n## Validation\n- [ ] 1. Evidence\n"), 0600); e != nil {
		t.Fatal(e)
	}
	RegisterProject(project)
	r := httptest.NewRequest("GET", "/api/conductor/projects", nil)
	w := httptest.NewRecorder()
	conductorProjects(w, r)
	var response struct {
		Data []conductorProjectView `json:"data"`
	}
	if e := json.Unmarshal(w.Body.Bytes(), &response); e != nil {
		t.Fatal(e)
	}
	if len(response.Data) != 1 || response.Data[0].Registered || response.Data[0].Queue.Depth != 1 || len(response.Data[0].Queue.Plans) != 1 || response.Data[0].Queue.Plans[0].Name != "finch" {
		t.Fatalf("discovery queue mismatch: %s", w.Body)
	}
	if _, e := os.Stat(filepath.Join(store, "config.json")); !os.IsNotExist(e) {
		t.Fatal("read-only discovery persisted enrollment")
	}
}
