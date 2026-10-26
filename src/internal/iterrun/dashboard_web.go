package iterrun

import (
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"html"
	"io/fs"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

//go:embed dashboard_web/*
var dashboardAssets embed.FS

func safeDetailIdentity(project, name, folder string) (string, error) {
	if project == "" || name == "" || name == "." || name == ".." || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
		return "", fmt.Errorf("a canonical project and exact safe plan identity are required")
	}
	canonical, e := filepath.EvalSymlinks(project)
	if e != nil {
		return "", fmt.Errorf("project is unavailable: %w", e)
	}
	known := false
	paths, _ := ListProjects()
	for _, p := range paths {
		c, e := filepath.EvalSymlinks(p)
		if e == nil && c == canonical {
			known = true
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

func dashboardDetailHeader(project, title string, returnURLs ...string) string {
	p := url.QueryEscape(project)
	_ = returnURLs
	return `<header class="dashboard-detail-header" aria-label="Dashboard navigation" data-project="` + html.EscapeString(project) + `" data-plan="` + html.EscapeString(title) + `"><a href="/">Dashboard</a></header><nav class="dashboard-detail-nav" aria-label="Breadcrumb"><a href="/">All projects</a> / <a href="/project?project=` + p + `&tab=plans">` + html.EscapeString(filepath.Base(project)) + `</a> / Plans / ` + html.EscapeString(title) + `<p>` + html.EscapeString(project) + `</p><a href="/project?project=` + p + `&tab=plans">Plans</a> · <a href="/project?project=` + p + `&tab=graph">Graph</a> · <a href="/project?project=` + p + `&tab=runs">Runs</a></nav><script src="/assets/dashboard/detail.js" defer></script>`
}
func dashboardDetailError(w http.ResponseWriter, r *http.Request, status int, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><title>Plan unavailable</title><link rel="stylesheet" href="/assets/dashboard/app.css"></head><body>`+dashboardDetailHeader(r.URL.Query().Get("project"), "Unavailable")+`<main><h1>Plan unavailable</h1><p>`+html.EscapeString(message)+`</p></main></body></html>`)
}

func dashboardShell(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" && r.URL.Path != "/project" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	fmt.Fprint(w, `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>iterate-run dashboard</title><link rel="stylesheet" href="/assets/dashboard/app.css"></head><body><div id="app"><p>Loading projects…</p></div><div id="notices" role="status" aria-live="polite"></div><script src="/assets/dashboard/app.js" defer></script></body></html>`)
}
func registerDashboardWeb(mux *http.ServeMux) {
	assets, _ := fs.Sub(dashboardAssets, "dashboard_web")
	mux.Handle("/assets/dashboard/", http.StripPrefix("/assets/dashboard/", http.FileServer(http.FS(assets))))
	mux.HandleFunc("/project", dashboardShell)
	mux.HandleFunc("/api/projects", dashboardProjects)
	mux.HandleFunc("/api/graph", dashboardGraph)
}
func dashboardLocalHost(host string) bool {
	if h, _, e := net.SplitHostPort(host); e == nil {
		host = h
	}
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "[::1]"
}
func knownProjectPath(want string) (string, bool) {
	paths, _ := ListProjects()
	for _, p := range paths {
		canonical, e := filepath.EvalSymlinks(p)
		if e != nil {
			canonical = filepath.Clean(p)
		}
		sum := sha256.Sum256([]byte(canonical))
		if want == "discovered-"+hex.EncodeToString(sum[:12]) || want == canonical || sameProjectDirectory(canonical, want) {
			return canonical, true
		}
	}
	return "", false
}
func dashboardGraph(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	path, ok := knownProjectPath(r.URL.Query().Get("project"))
	if !ok {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(404)
		json.NewEncoder(w).Encode(map[string]any{"ok": false, "error": fmt.Sprintf("unknown project %q; name a registered project path", r.URL.Query().Get("project"))})
		return
	}
	graph, e := BuildGraphView(path)
	if e != nil {
		http.Error(w, e.Error(), 500)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(graph)
}
func dashboardProjects(w http.ResponseWriter, r *http.Request) {
	if r.Method != "GET" {
		http.Error(w, "method not allowed", 405)
		return
	}
	paths, _ := ListProjects()
	out := []map[string]any{}
	seen := map[string]bool{}
	for _, path := range paths {
		canonical, e := filepath.EvalSymlinks(path)
		if e != nil {
			canonical = filepath.Clean(path)
		}
		if seen[canonical] {
			continue
		}
		seen[canonical] = true
		plans, e := ListPlans(canonical)
		reason := ""
		if e != nil {
			reason = e.Error()
		}
		if plans == nil {
			plans = []PlanSummary{}
		}
		archives, _ := ListArchivedPlans(canonical)
		if archives == nil {
			archives = []PlanSummary{}
		}
		graph, err := BuildGraphView(canonical)
		queuePlans := []map[string]any{}
		running := []string{}
		owners := map[string]string{}
		if err == nil {
			for _, node := range graph.Nodes {
				if node.Archived {
					continue
				}
				if node.State == "queued" || node.State == "executing" || node.State == "unblocked" || node.Status == "waiting" || node.Status == "blocked" {
					queuePlans = append(queuePlans, map[string]any{"name": node.Name, "waiting": node.Status == "waiting", "blocked": node.Status == "blocked", "reason": node.Reason, "wait_reason": node.Reason, "waiting_on": node.WaitingOn, "held_by": node.HeldBy})
				}
			}
			if owner := graph.ExecutionOwner; owner != nil && owner.Live {
				running = append(running, owner.Plan)
				kind := owner.Owner
				if kind == "" {
					kind = "cli"
				}
				owners[owner.Plan] = kind
			}
		}
		out = append(out, map[string]any{"id": canonical, "name": filepath.Base(canonical), "path": canonical, "plans": plans, "archived": archives, "error": reason, "queue": map[string]any{"plans": queuePlans, "running_plans": running, "running_owners": owners}})
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"schema_version": 1, "ok": true, "data": out})
}
