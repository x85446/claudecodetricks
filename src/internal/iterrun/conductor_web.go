package iterrun

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"html"
	"io/fs"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
)

//go:embed conductor_web/*
var conductorAssets embed.FS

func conductorShell(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/project" && r.URL.Path != "/settings" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>Conductor · iterate-run</title><link rel="stylesheet" href="/assets/conductor/app.css"></head><body><div id="app"><p>Loading Conductor…</p></div><div id="notices" role="status" aria-live="polite"></div><dialog id="editor" aria-labelledby="dialog-title"></dialog><script src="/assets/conductor/app.js" defer></script></body></html>`)
}

func registerConductorWeb(mux *http.ServeMux) {
	assets, _ := fs.Sub(conductorAssets, "conductor_web")
	mux.Handle("/assets/conductor/", http.StripPrefix("/assets/conductor/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("/project", conductorShell)
	mux.HandleFunc("/settings", conductorShell)
	mux.HandleFunc("/api/conductor/projects", conductorProjects)
}

type conductorProjectView struct {
	ID         string          `json:"id"`
	Name       string          `json:"name"`
	Path       string          `json:"path"`
	Registered bool            `json:"registered"`
	Plans      []PlanSummary   `json:"plans"`
	Archived   []PlanSummary   `json:"archived"`
	Error      string          `json:"error,omitempty"`
	Queue      conductor.Queue `json:"queue"`
}

func conductorProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	svc := NewConductorService("")
	cfg, e := svc.Config()
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	paths, _ := ListProjects()
	by := map[string]conductorProjectView{}
	for _, p := range cfg.Projects {
		by[p.Path] = conductorProjectView{ID: p.ID, Name: p.Name, Path: p.Path, Registered: true}
	}
	for _, p := range paths {
		canonical, e := conductor.Canonical(p)
		if e != nil {
			canonical = filepath.Clean(p)
		}
		if _, ok := by[canonical]; !ok {
			by[canonical] = conductorProjectView{Name: filepath.Base(canonical), Path: canonical}
		}
	}
	for path, view := range by {
		if !view.Registered {
			sum := sha256.Sum256([]byte(path))
			view.ID = "discovered-" + hex.EncodeToString(sum[:12])
			by[path] = view
			cfg.Projects = append(cfg.Projects, model.Project{ID: view.ID, Name: view.Name, Path: path, Enabled: false})
		}
	}
	decision, decisionErr := svc.Evaluate(r.Context(), conductor.EvaluateRequest{Draft: &cfg})
	queues := map[string]conductor.Queue{}
	for _, q := range decision.Queues {
		queues[q.ProjectID] = q
	}
	out := []conductorProjectView{}
	for _, p := range by {
		p.Queue = queues[p.ID]
		if decisionErr != nil {
			p.Error = decisionErr.Error()
		}
		p.Plans, e = ListPlans(p.Path)
		if e != nil {
			p.Error = e.Error()
		}
		p.Archived, e = ListArchivedPlans(p.Path)
		if e != nil {
			p.Error = e.Error()
		}
		if p.Plans == nil {
			p.Plans = []PlanSummary{}
		}
		if p.Archived == nil {
			p.Archived = []PlanSummary{}
		}
		out = append(out, p)
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"schema_version": 1, "ok": true, "revision": cfg.Revision, "data": out, "error": nil})
}

func safeDetailIdentity(project, name, folder string) (string, error) {
	if project == "" || name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
		return "", fmt.Errorf("a canonical project and exact safe plan identity are required")
	}
	canonical, e := conductor.Canonical(project)
	if e != nil {
		return "", fmt.Errorf("project is unavailable: %w", e)
	}
	known := false
	paths, _ := ListProjects()
	for _, p := range paths {
		c, e := conductor.Canonical(p)
		if e == nil && c == canonical {
			known = true
		}
	}
	cfg, e := NewConductorService("").Config()
	if e == nil {
		for _, p := range cfg.Projects {
			if p.Path == canonical {
				known = true
			}
		}
	}
	if !known {
		return "", fmt.Errorf("project is not registered in this dashboard")
	}
	base := filepath.Join(canonical, ".claude", "iterate", folder)
	file := name
	if folder == "plans" {
		file += ".md"
	}
	path := filepath.Join(base, file)
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil {
		return canonical, e
	}
	resolvedBase, e := filepath.EvalSymlinks(base)
	if e != nil {
		return canonical, e
	}
	if filepath.Dir(resolved) != resolvedBase {
		return canonical, fmt.Errorf("plan resolves outside its project directory")
	}
	info, e := os.Stat(resolved)
	if e != nil {
		return canonical, e
	}
	if !info.Mode().IsRegular() {
		return canonical, fmt.Errorf("plan is not a regular file")
	}
	return canonical, nil
}

func conductorDetailHeader(project, title string, returnURLs ...string) string {
	p := url.QueryEscape(project)
	returnURL := "/project?project=" + p + "&tab=plans"
	if len(returnURLs) > 0 {
		returnURL = returnURLs[0]
	}
	return `<header class="conductor-detail-header" aria-label="Conductor navigation" data-project="` + html.EscapeString(project) + `" data-plan="` + html.EscapeString(title) + `"><a href="/">All projects</a><a href="/settings?return=` + url.QueryEscape(returnURL) + `">Settings</a></header><nav class="conductor-detail-nav" aria-label="Breadcrumb"><a href="/">All projects</a> / <a href="/project?project=` + p + `&tab=plans">` + html.EscapeString(filepath.Base(project)) + `</a> / Plans / ` + html.EscapeString(title) + `<p>` + html.EscapeString(project) + `</p><a href="/project?project=` + p + `&tab=plans">Plans</a> · <a href="/project?project=` + p + `&tab=automations">Automations</a> · <a href="/project?project=` + p + `&tab=activity">Activity</a></nav><section id="conductor-live-detail" class="conductor-detail-nav" aria-live="polite"></section><script src="/assets/conductor/detail.js" defer></script>`
}
func conductorDetailError(w http.ResponseWriter, r *http.Request, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Plan unavailable</title><link rel="stylesheet" href="/assets/conductor/app.css"></head><body>`+conductorDetailHeader(r.URL.Query().Get("project"), "Unavailable")+`<main><h1>Plan unavailable</h1><p>`+html.EscapeString(message)+`</p></main></body></html>`)
}
