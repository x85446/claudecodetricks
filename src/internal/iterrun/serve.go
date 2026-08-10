package iterrun

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"strings"
)

// Serve starts the read-only dashboard: a list of every known project's
// plans, each linking to its live timeline. Deliberately read-only — no
// delete button here. Purge is a terminal command (see purge.go) precisely
// because a stray click on a page anyone on the machine can hit is not
// where a destructive action belongs.
func Serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/plan", handlePlan)
	fmt.Printf("iterate-run dashboard: http://%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func handleIndex(w http.ResponseWriter, _ *http.Request) {
	projects, _ := ListProjects()

	var b strings.Builder
	b.WriteString(dashboardHead("iterate-run"))
	b.WriteString("<h1>iterate-run</h1><div class=\"sub\">plans across every project seen on this machine</div>\n")

	if len(projects) == 0 {
		b.WriteString(`<p class="empty">No projects registered yet — run any iterate-run command (status, timeline, or a hook-covered session) from a project with .claude/iterate/plans/ and it'll show up here.</p>`)
	}

	for _, proj := range projects {
		plans, _ := ListPlans(proj)
		fmt.Fprintf(&b, "<section class=\"project\"><h2>%s</h2>\n", html.EscapeString(proj))
		if len(plans) == 0 {
			b.WriteString(`<p class="empty">no plans found</p>`)
		}
		b.WriteString("<div class=\"plans\">\n")
		for _, p := range plans {
			badge, badgeClass := "planned", "b-planned"
			if p.Phase == "executing" {
				badge, badgeClass = "executing", "b-executing"
			}
			if p.IsCompleted() {
				badge, badgeClass = "completed", "b-done"
			}
			teamsInfo := ""
			if p.HasTeams {
				teamsInfo = fmt.Sprintf(" &middot; teams %d/%d", p.TeamsDone, p.TeamsTotal)
			}
			fmt.Fprintf(&b, `<a class="plan-card" href="/plan?project=%s&name=%s"><span class="badge %s">%s</span><span class="pname">%s</span><span class="pgoal">%s</span><span class="pmeta">started %s%s</span></a>`+"\n",
				url.QueryEscape(proj), url.QueryEscape(p.Name), badgeClass, badge,
				html.EscapeString(p.Name), html.EscapeString(p.Goal), html.EscapeString(p.Started), teamsInfo)
		}
		b.WriteString("</div></section>\n")
	}

	b.WriteString("</body></html>")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func handlePlan(w http.ResponseWriter, r *http.Request) {
	proj := r.URL.Query().Get("project")
	name := r.URL.Query().Get("name")
	if proj == "" || name == "" {
		http.Error(w, "missing project or name", http.StatusBadRequest)
		return
	}

	rows, err := BuildRowsFromFilesystem(name, proj, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	if events, err := ReadEvents(); err == nil {
		labels, _ := ReadLabels()
		rows = MergeRows(rows, BuildRowsFromHookEvents(events, labels, name))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(RenderTimelineHTML(rows)))
}

func dashboardHead(title string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><title>` + html.EscapeString(title) + `</title><style>
body{font-family:-apple-system,sans-serif;background:#111;color:#eee;padding:24px;margin:0 auto;max-width:960px}
h1{font-size:20px;font-weight:600;margin:0 0 4px}
h2{font-size:14px;color:#ccc;margin:26px 0 10px;border-bottom:1px solid #262c31;padding-bottom:6px;font-weight:500}
.sub{color:#888;font-size:12px;margin-bottom:20px}
.empty{color:#666;font-size:13px;font-style:italic}
.plans{display:flex;flex-direction:column;gap:8px}
.plan-card{display:grid;grid-template-columns:92px 90px 1fr 230px;gap:12px;align-items:center;padding:10px 12px;background:#171b1f;border:1px solid #262c31;border-radius:8px;text-decoration:none;color:#eee;font-size:12.5px}
.plan-card:hover{border-color:#3b82f6}
.badge{font-size:10px;text-transform:uppercase;letter-spacing:.04em;padding:3px 7px;border-radius:4px;text-align:center;font-weight:600}
.b-planned{background:#1d2227;color:#8b9198}
.b-executing{background:#3a2705;color:#f59e0b}
.b-done{background:#0d2b21;color:#34d399}
.pname{font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.pgoal{color:#aaa;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.pmeta{color:#666;text-align:right;font-size:11px;white-space:nowrap}
</style></head><body>
`
}
