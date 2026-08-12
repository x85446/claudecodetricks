package iterrun

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"path/filepath"
	"strings"
)

// Serve starts the read-only dashboard: every known project's plans, tabbed
// by project plus an "All" overview, each plan linking to its live
// timeline. Deliberately read-only — no delete button here. Purge is a
// terminal command (see purge.go) precisely because a stray click on a
// page anyone on the machine can reach is not where a destructive action
// belongs.
func Serve(addr string) error {
	mux := http.NewServeMux()
	mux.HandleFunc("/", handleIndex)
	mux.HandleFunc("/plan", handlePlan)
	mux.HandleFunc("/archive", handleArchive)
	fmt.Printf("iterate-run dashboard: http://%s\n", addr)
	return http.ListenAndServe(addr, mux)
}

func handleIndex(w http.ResponseWriter, _ *http.Request) {
	projects, _ := ListProjects()

	var b strings.Builder
	b.WriteString(dashboardHead("iterate-run"))
	b.WriteString(`<div class="eyebrow">iterate-run</div><h1>Dashboard</h1><div class="sub">every project seen on this machine — one click to any plan's timeline</div>`)

	if len(projects) == 0 {
		b.WriteString(`<p class="empty">No projects registered yet — run any iterate-run command (status, timeline, or a hook-covered session) from a project with .claude/iterate/plans/ and it'll show up here.</p></div></body></html>`)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(b.String()))
		return
	}

	type projPlans struct {
		dir      string
		short    string
		plans    []PlanSummary
		archived []PlanSummary
	}
	var all []projPlans
	totalPlans := 0
	for _, proj := range projects {
		plans, _ := ListPlans(proj)
		archived, _ := ListArchivedPlans(proj)
		totalPlans += len(plans)
		all = append(all, projPlans{dir: proj, short: filepath.Base(proj), plans: plans, archived: archived})
	}

	b.WriteString(`<nav class="tabs">`)
	fmt.Fprintf(&b, `<button class="tab active" data-tab="all" onclick="showTab('all')">All <span class="tcount">%d</span></button>`, totalPlans)
	for i, pp := range all {
		fmt.Fprintf(&b, `<button class="tab" data-tab="p%d" onclick="showTab('p%d')" title="%s">%s <span class="tcount">%d</span></button>`,
			i, i, html.EscapeString(pp.dir), html.EscapeString(pp.short), len(pp.plans))
	}
	b.WriteString(`</nav>`)

	b.WriteString(`<div id="tab-all" class="tabpanel">`)
	for _, pp := range all {
		writeProjectSection(&b, pp.dir, pp.short, pp.plans, pp.archived)
	}
	b.WriteString(`</div>`)

	for i, pp := range all {
		fmt.Fprintf(&b, `<div id="tab-p%d" class="tabpanel" style="display:none">`, i)
		writeProjectSection(&b, pp.dir, pp.short, pp.plans, pp.archived)
		b.WriteString(`</div>`)
	}

	b.WriteString(`<script>
function showTab(id){
  document.querySelectorAll('.tabpanel').forEach(function(el){ el.style.display = (el.id === 'tab-'+id) ? '' : 'none'; });
  document.querySelectorAll('.tab').forEach(function(el){ el.classList.toggle('active', el.dataset.tab === id); });
}
</script>`)

	b.WriteString(`</div></body></html>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

func writeProjectSection(b *strings.Builder, dir, short string, plans, archived []PlanSummary) {
	fmt.Fprintf(b, `<section class="project"><h2 title="%s">%s</h2>`, html.EscapeString(dir), html.EscapeString(short))
	if len(plans) == 0 {
		b.WriteString(`<p class="empty">no plans found</p>`)
	} else {
		writeLivePlans(b, dir, plans)
	}
	writeArchivedPlans(b, dir, archived)
	b.WriteString(`</section>`)
}

// writeArchivedPlans renders every finished/given-up run for this project
// as a collapsed <details> block — completed history you can still open
// and review (Goal, Requirements burndown, per-team timeline), just not
// competing for space with the live plans a visitor actually came to
// check on. Confirmed live as a real gap: before this existed, a plan
// simply vanished from the dashboard the moment /iterate archived it —
// right when someone would most want to look back at what it did.
func writeArchivedPlans(b *strings.Builder, dir string, archived []PlanSummary) {
	if len(archived) == 0 {
		return
	}
	fmt.Fprintf(b, `<details class="archived"><summary>Archived <span class="tcount">%d</span></summary><div class="plans">`, len(archived))
	for _, p := range archived {
		badge, badgeClass := "archived", "b-archived"
		if p.Blocked() {
			badge, badgeClass = "needs you", "b-blocked"
		} else if p.Phase != "" {
			badge = p.Phase
		}
		fmt.Fprintf(b, `<a class="plan-card" href="/archive?project=%s&file=%s"><span class="badge %s">%s</span><span class="pname">%s</span><span class="pgoal">%s</span><span class="pmeta">started %s</span></a>`,
			url.QueryEscape(dir), url.QueryEscape(p.ArchiveFile), badgeClass, html.EscapeString(badge),
			html.EscapeString(p.Name), html.EscapeString(p.Goal), html.EscapeString(p.Started))
	}
	b.WriteString(`</div></details>`)
}

func writeLivePlans(b *strings.Builder, dir string, plans []PlanSummary) {
	b.WriteString(`<div class="plans">`)
	for _, p := range plans {
		// Order matters: IsCompleted() (team-verified) beats a bare
		// declared phase, but an honestly-labeled phase beats silently
		// defaulting to "planned" — confirmed live: five plans declaring
		// "phase: complete" with zero team rows actually done (never-
		// executed, likely superseded/archived, not verified-finished)
		// were showing as "planned" simply because only "executing" was
		// ever specially recognized; everything else fell through to the
		// default, indistinguishable from a plan nothing had touched yet.
		// Blocked always wins last: a plan can be 100% done by its Teams
		// table and still be sitting here waiting on a human.
		badge, badgeClass := "planned", "b-planned"
		switch {
		case p.Phase == "executing":
			badge, badgeClass = "executing", "b-executing"
		case p.IsCompleted():
			badge, badgeClass = "completed", "b-done"
		case p.Phase != "" && p.Phase != "planned":
			badge, badgeClass = p.Phase, "b-archived"
		}
		if p.Blocked() {
			badge, badgeClass = "needs you", "b-blocked"
		}
		teamsInfo := ""
		if p.HasTeams {
			teamsInfo = fmt.Sprintf(" &middot; teams %d/%d", p.TeamsDone, p.TeamsTotal)
		}
		fmt.Fprintf(b, `<a class="plan-card" href="/plan?project=%s&name=%s"><span class="badge %s">%s</span><span class="pname">%s</span><span class="pgoal">%s</span><span class="pmeta">started %s%s</span></a>`,
			url.QueryEscape(dir), url.QueryEscape(p.Name), badgeClass, badge,
			html.EscapeString(p.Name), html.EscapeString(p.Goal), html.EscapeString(p.Started), teamsInfo)
	}
	b.WriteString(`</div></section>`)
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

	plan, err := GetPlanSummary(proj, name)
	if err != nil {
		plan = PlanSummary{Name: name, ProjectDir: proj}
	}
	planStarted, _ := plan.StartedAt()

	if events, err := ReadEvents(); err == nil {
		labels, _ := ReadLabels()
		rows = MergeRows(rows, BuildRowsFromHookEvents(events, labels, name, proj, planStarted))
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(RenderTimelineHTML(rows, plan, "/")))
}

func dashboardHead(title string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><title>` + html.EscapeString(title) + `</title><style>
:root{
  --bg:#f7f4ef; --surface:#fff; --surface-2:#fbf9f5; --border:#e4ddd0;
  --text:#1c1a17; --text-dim:#6b6459; --text-faint:#a29b8d;
  --good:#059669; --good-bg:#e3f6ee;
  --warn:#b45309; --warn-bg:#fbecd5;
  --queued:#a29b8d; --queued-bg:#f0ece2;
  --accent:#0e7490; --accent-bg:#dff3f6;
  --danger:#dc2626; --danger-bg:#fbe1e1;
  --archived:#6d5aa8; --archived-bg:#ece8f7;
}
@media (prefers-color-scheme:dark){:root{
  --bg:#0f1215; --surface:#171b1f; --surface-2:#1d2227; --border:#262c31;
  --text:#e8e6e1; --text-dim:#8b9198; --text-faint:#565d64;
  --good:#34d399; --good-bg:#0d2b21;
  --warn:#f59e0b; --warn-bg:#3a2705;
  --queued:#565d64; --queued-bg:#1d2227;
  --accent:#22d3ee; --accent-bg:#123b42;
  --danger:#ef4444; --danger-bg:#2c0a0a;
  --archived:#a78bda; --archived-bg:#241f38;
}}
*{box-sizing:border-box}
body{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;background:var(--bg);color:var(--text);padding:28px 20px 50px;margin:0}
body>div{max-width:960px;margin:0 auto}
.eyebrow{font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:var(--accent);font-weight:600}
h1{font-size:24px;font-weight:700;margin:2px 0 8px;letter-spacing:-.01em}
h2{font-size:13px;color:var(--text-dim);margin:0;font-weight:600;text-transform:uppercase;letter-spacing:.04em}
.sub{color:var(--text-dim);font-size:12.5px;margin-bottom:22px}
.empty{color:var(--text-faint);font-size:13px;font-style:italic}
.tabs{display:flex;flex-wrap:wrap;gap:6px;border-bottom:1px solid var(--border);padding-bottom:0;margin-bottom:22px}
.tab{font:inherit;font-size:13px;background:none;border:none;border-bottom:2px solid transparent;color:var(--text-dim);padding:8px 12px;cursor:pointer;border-radius:6px 6px 0 0}
.tab:hover{background:var(--surface-2);color:var(--text)}
.tab.active{color:var(--accent);border-bottom-color:var(--accent);font-weight:600}
.tcount{font-size:10.5px;color:var(--text-faint);margin-left:2px}
.tab.active .tcount{color:var(--accent)}
.project{margin-bottom:28px}
.project h2{border-bottom:1px solid var(--border);padding-bottom:8px;margin-bottom:10px}
.plans{display:flex;flex-direction:column;gap:8px}
.plan-card{display:grid;grid-template-columns:92px 90px 1fr 230px;gap:12px;align-items:center;padding:10px 12px;background:var(--surface);border:1px solid var(--border);border-radius:8px;text-decoration:none;color:var(--text);font-size:12.5px}
.plan-card:hover{border-color:var(--accent)}
.badge{font-size:10px;text-transform:uppercase;letter-spacing:.04em;padding:3px 7px;border-radius:4px;text-align:center;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap;min-width:0}
.b-planned{background:var(--queued-bg);color:var(--queued)}
.b-executing{background:var(--warn-bg);color:var(--warn)}
.b-done{background:var(--good-bg);color:var(--good)}
.b-blocked{background:var(--danger-bg);color:var(--danger)}
.b-archived{background:var(--archived-bg);color:var(--archived)}
.pname{min-width:0;font-weight:600;overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.pgoal{min-width:0;color:var(--text-dim);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.pmeta{color:var(--text-faint);text-align:right;font-size:11px;min-width:0;white-space:normal;line-height:1.5;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
</style></head><body><div>
`
}
