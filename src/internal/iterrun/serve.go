package iterrun

import (
	"fmt"
	"html"
	"net/http"
	"net/url"
	"path/filepath"
	"sort"
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
	totalPlans, totalArchived := 0, 0
	for _, proj := range projects {
		plans, _ := ListPlans(proj)
		archived, _ := ListArchivedPlans(proj)
		totalPlans += len(plans)
		totalArchived += len(archived)
		all = append(all, projPlans{dir: proj, short: filepath.Base(proj), plans: plans, archived: archived})
	}

	// The tag filter lets a visitor hide badge categories they don't care
	// about right now (e.g. "archived", "completed") — a settings popup,
	// not a server-side query, since it's a per-visitor display
	// preference persisted in the browser's own localStorage. tagSet is
	// every distinct badge label actually in play across every project
	// right now (live AND archived) — built from data, never a fixed
	// list, since a badge can be an arbitrary custom "phase:" string a
	// coordinator wrote (confirmed live: badges besides the five standard
	// ones do show up).
	tagSet := map[string]bool{}
	for _, pp := range all {
		for _, p := range pp.plans {
			label, _ := liveBadge(p)
			tagSet[label] = true
		}
		for _, p := range pp.archived {
			label, _ := archivedBadge(p)
			tagSet[label] = true
		}
	}
	b.WriteString(`<div class="toolbar-row">`)
	writeTagFilter(&b, tagSet)
	b.WriteString(`</div>`)

	// Top-level split: Active is everything still in flight (or planned,
	// or blocked-on-operator) — the existing per-project sub-tabs live
	// entirely inside it, unchanged. Archived is its own separate tab, not
	// a collapsible tucked inside each project's live section — a
	// finished run and a live one are different enough concerns that
	// mixing them in the same list made the "what's actually happening
	// right now" view noisier than it needed to be.
	fmt.Fprintf(&b, `<nav class="toptabs"><button class="toptab active" data-top="active" onclick="showTop('active')">Active <span class="tcount">%d</span></button><button class="toptab" data-top="archived" onclick="showTop('archived')">Archived <span class="tcount">%d</span></button></nav>`,
		totalPlans, totalArchived)

	b.WriteString(`<div id="top-active">`)
	b.WriteString(`<nav class="tabs">`)
	fmt.Fprintf(&b, `<button class="tab active" data-tab="all" onclick="showTab('all')">All <span class="tcount">%d</span></button>`, totalPlans)
	for i, pp := range all {
		fmt.Fprintf(&b, `<button class="tab" data-tab="p%d" onclick="showTab('p%d')" title="%s">%s <span class="tcount">%d</span></button>`,
			i, i, html.EscapeString(pp.dir), html.EscapeString(pp.short), len(pp.plans))
	}
	b.WriteString(`</nav>`)

	b.WriteString(`<div id="tab-all" class="tabpanel">`)
	for _, pp := range all {
		writeLiveProjectSection(&b, pp.dir, pp.short, pp.plans)
	}
	b.WriteString(`</div>`)

	for i, pp := range all {
		fmt.Fprintf(&b, `<div id="tab-p%d" class="tabpanel" style="display:none">`, i)
		writeLiveProjectSection(&b, pp.dir, pp.short, pp.plans)
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)

	// Archived: one collapsed row per project — just its name and how many
	// finished runs it has — expanding on click to the same plan cards the
	// Active tab uses. A project with zero archived runs doesn't get a row
	// at all; nothing to look back at yet.
	b.WriteString(`<div id="top-archived" style="display:none">`)
	if totalArchived == 0 {
		b.WriteString(`<p class="empty">no archived plans yet</p>`)
	} else {
		for _, pp := range all {
			writeArchivedProjectAccordion(&b, pp.dir, pp.short, pp.archived)
		}
	}
	b.WriteString(`</div>`)

	b.WriteString(`<script>
function showTab(id){
  document.querySelectorAll('.tabpanel').forEach(function(el){ el.style.display = (el.id === 'tab-'+id) ? '' : 'none'; });
  document.querySelectorAll('.tab').forEach(function(el){ el.classList.toggle('active', el.dataset.tab === id); });
}
function showTop(id){
  document.getElementById('top-active').style.display = (id === 'active') ? '' : 'none';
  document.getElementById('top-archived').style.display = (id === 'archived') ? '' : 'none';
  document.querySelectorAll('.toptab').forEach(function(el){ el.classList.toggle('active', el.dataset.top === id); });
}
</script>`)

	b.WriteString(`</div></body></html>`)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

// writeTagFilter renders the "⚙ Filter" settings popup — checkboxes for
// every badge/tag currently in play (see tagSet's own doc), letting a
// visitor hide categories they don't care about right now (e.g. archived,
// completed). This is a per-visitor DISPLAY preference, not a query — it
// hides/shows already-rendered .plan-card elements client-side and
// persists the hidden set in the browser's own localStorage (so it
// survives a reload, but is local to this browser, never sent to or
// stored by the server). tags is sorted with the handful of standard
// badges first in their natural priority order, then any custom "phase:"
// string a coordinator wrote alphabetically after.
func writeTagFilter(b *strings.Builder, tagSet map[string]bool) {
	priority := []string{"executing", "needs you", "planned", "completed", "archived"}
	seen := map[string]bool{}
	var tags []string
	for _, t := range priority {
		if tagSet[t] {
			tags = append(tags, t)
			seen[t] = true
		}
	}
	var rest []string
	for t := range tagSet {
		if !seen[t] {
			rest = append(rest, t)
		}
	}
	sort.Strings(rest)
	tags = append(tags, rest...)

	b.WriteString(`<div class="tagfilter"><button type="button" class="tagfilter-btn" onclick="tfToggle(event)">&#9881; Filter<span id="tf-count" class="tf-count"></span></button><div id="tf-panel" class="tagfilter-panel" style="display:none"><div class="tf-title">Show tags</div>`)
	for _, t := range tags {
		fmt.Fprintf(b, `<label class="tf-row"><input type="checkbox" class="tf-cb" data-tag="%s" checked onchange="tfOnToggle(this)"> %s</label>`,
			html.EscapeString(t), html.EscapeString(t))
	}
	b.WriteString(`<button type="button" class="tf-reset" onclick="tfReset()">Show all</button></div></div>
<script>
(function(){
var TF_KEY = 'iterate-run-hidden-tags';
function tfLoad(){ try { return JSON.parse(localStorage.getItem(TF_KEY) || '[]'); } catch(e){ return []; } }
function tfSave(list){ try { localStorage.setItem(TF_KEY, JSON.stringify(list)); } catch(e){} }
function tfApply(){
  var hidden = tfLoad();
  document.querySelectorAll('.plan-card').forEach(function(c){
    c.style.display = hidden.indexOf(c.dataset.tag) !== -1 ? 'none' : '';
  });
  document.querySelectorAll('.tf-cb').forEach(function(cb){
    cb.checked = hidden.indexOf(cb.dataset.tag) === -1;
  });
  var count = document.getElementById('tf-count');
  count.textContent = hidden.length ? ' (' + hidden.length + ' hidden)' : '';
}
window.tfOnToggle = function(cb){
  var hidden = tfLoad(), tag = cb.dataset.tag, idx = hidden.indexOf(tag);
  if (cb.checked) { if (idx !== -1) hidden.splice(idx, 1); }
  else if (idx === -1) { hidden.push(tag); }
  tfSave(hidden);
  tfApply();
};
window.tfReset = function(){ tfSave([]); tfApply(); };
window.tfToggle = function(e){
  e.stopPropagation();
  var p = document.getElementById('tf-panel');
  p.style.display = p.style.display === 'none' ? '' : 'none';
};
document.addEventListener('click', function(e){
  var tf = document.querySelector('.tagfilter');
  if (tf && !tf.contains(e.target)) { document.getElementById('tf-panel').style.display = 'none'; }
});
tfApply();
})();
</script>`)
}

// writeLiveProjectSection renders one project's LIVE plans only, for the
// Active tab — archived runs get their own separate top-level tab now
// (writeArchivedProjectAccordion), not a collapsible tucked in here.
func writeLiveProjectSection(b *strings.Builder, dir, short string, plans []PlanSummary) {
	fmt.Fprintf(b, `<section class="project"><h2 title="%s">%s</h2>`, html.EscapeString(dir), html.EscapeString(short))
	if len(plans) == 0 {
		b.WriteString(`<p class="empty">no plans found</p>`)
	} else {
		writeLivePlans(b, dir, plans)
	}
	b.WriteString(`</section>`)
}

// writeArchivedProjectAccordion renders one project as a single collapsed
// row in the Archived tab — just its name and how many finished runs it
// has — expanding on click to the same plan cards the Active tab uses
// (Goal, Requirements burndown, per-team timeline all still just a click
// away). A project with zero archived runs gets no row at all — nothing
// to look back at yet, and an always-visible empty row would just be
// noise in a tab meant purely for browsing history.
func writeArchivedProjectAccordion(b *strings.Builder, dir, short string, archived []PlanSummary) {
	if len(archived) == 0 {
		return
	}
	fmt.Fprintf(b, `<details class="archived-project"><summary title="%s">%s <span class="tcount">%d</span></summary><div class="plans">`,
		html.EscapeString(dir), html.EscapeString(short), len(archived))
	for _, p := range archived {
		badge, badgeClass := archivedBadge(p)
		fmt.Fprintf(b, `<a class="plan-card" data-tag="%s" href="/archive?project=%s&file=%s"><span class="badge %s">%s</span><span class="pname">%s</span><span class="pgoal">%s</span><span class="pmeta">started %s</span></a>`,
			html.EscapeString(badge), url.QueryEscape(dir), url.QueryEscape(p.ArchiveFile), badgeClass, html.EscapeString(badge),
			html.EscapeString(p.Name), html.EscapeString(p.Goal), html.EscapeString(p.Started))
	}
	b.WriteString(`</div></details>`)
}

// archivedBadge is an archived plan's card badge — "needs you" beats
// everything (a finished run can still leave a status:blocked-on-operator
// clause, same as a live plan can), otherwise its own declared phase
// (typically "complete"), falling back to the generic "archived" both when
// the file predates phase: being set on archive at all AND when phase
// still reads "executing" or "planned" — confirmed live: /iterate's own
// SKILL.md only requires MOVING the file on archive, not updating phase:
// first, so an archived run can carry a stale in-flight-sounding phase
// (aardvark's archived copy still says "phase: executing"). Showing that
// verbatim recreates exactly the "looks live when it isn't" problem this
// whole feature exists to avoid, AND collides in the tag filter with
// genuinely live plans sharing the same label — one "executing" checkbox
// would then hide/show two unrelated things together.
func archivedBadge(p PlanSummary) (label, class string) {
	if p.Blocked() {
		return "needs you", "b-blocked"
	}
	if p.Phase != "" && p.Phase != "executing" && p.Phase != "planned" {
		return p.Phase, "b-archived"
	}
	return "archived", "b-archived"
}

func writeLivePlans(b *strings.Builder, dir string, plans []PlanSummary) {
	b.WriteString(`<div class="plans">`)
	for _, p := range plans {
		badge, badgeClass := liveBadge(p)
		teamsInfo := ""
		if p.HasTeams {
			teamsInfo = fmt.Sprintf(" &middot; teams %d/%d", p.TeamsDone, p.TeamsTotal)
		}
		fmt.Fprintf(b, `<a class="plan-card" data-tag="%s" href="/plan?project=%s&name=%s"><span class="badge %s">%s</span><span class="pname">%s</span><span class="pgoal">%s</span><span class="pmeta">started %s%s</span></a>`,
			html.EscapeString(badge), url.QueryEscape(dir), url.QueryEscape(p.Name), badgeClass, badge,
			html.EscapeString(p.Name), html.EscapeString(p.Goal), html.EscapeString(p.Started), teamsInfo)
	}
	b.WriteString(`</div>`)
}

// liveBadge is a live (unarchived) plan's card badge. Order matters:
// IsCompleted() (team-verified) beats a bare declared phase, but an
// honestly-labeled phase beats silently defaulting to "planned" —
// confirmed live: five plans declaring "phase: complete" with zero team
// rows actually done (never-executed, likely superseded/archived, not
// verified-finished) were showing as "planned" simply because only
// "executing" was ever specially recognized; everything else fell through
// to the default, indistinguishable from a plan nothing had touched yet.
// Blocked always wins last: a plan can be 100% done by its Teams table
// and still be sitting here waiting on a human.
func liveBadge(p PlanSummary) (label, class string) {
	label, class = "planned", "b-planned"
	switch {
	case p.Phase == "executing":
		label, class = "executing", "b-executing"
	case p.IsCompleted():
		label, class = "completed", "b-done"
	case p.Phase != "" && p.Phase != "planned":
		label, class = p.Phase, "b-archived"
	}
	if p.Blocked() {
		label, class = "needs you", "b-blocked"
	}
	return label, class
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

// handleArchive is handlePlan's counterpart for a finished/given-up run —
// same rendering, pointed at .claude/iterate/archive/ instead of the live
// plans/ directory (see BuildRowsFromArchive). file is the plan's exact
// archived filename (PlanSummary.ArchiveFile), not its bare name — the
// archive directory can hold more than one timestamped file, and the bare
// name alone can't disambiguate them or locate the file at all.
func handleArchive(w http.ResponseWriter, r *http.Request) {
	proj := r.URL.Query().Get("project")
	file := r.URL.Query().Get("file")
	if proj == "" || file == "" {
		http.Error(w, "missing project or file", http.StatusBadRequest)
		return
	}

	plan, err := GetArchivedPlanSummary(proj, file)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	rows, err := BuildRowsFromArchive(file, plan.Name, proj, nil)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	planStarted, _ := plan.StartedAt()
	if events, err := ReadEvents(); err == nil {
		labels, _ := ReadLabels()
		rows = MergeRows(rows, BuildRowsFromHookEvents(events, labels, plan.Name, proj, planStarted))
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
.toolbar-row{display:flex;justify-content:flex-end;margin-bottom:8px}
.tagfilter{position:relative}
.tagfilter-btn{font:inherit;font-size:12.5px;background:var(--surface);border:1px solid var(--border);color:var(--text-dim);padding:7px 12px;border-radius:8px;cursor:pointer}
.tagfilter-btn:hover{border-color:var(--accent);color:var(--text)}
.tf-count{color:var(--accent);font-size:11px;margin-left:2px}
.tagfilter-panel{position:absolute;right:0;top:calc(100% + 6px);z-index:10;background:var(--surface);border:1px solid var(--border);border-radius:10px;padding:12px 14px;min-width:180px;box-shadow:0 8px 24px rgba(0,0,0,.12)}
.tf-title{font-size:10.5px;letter-spacing:.06em;text-transform:uppercase;color:var(--text-faint);font-weight:600;margin-bottom:8px}
.tf-row{display:flex;align-items:center;gap:8px;font-size:12.5px;color:var(--text);padding:4px 0;cursor:pointer}
.tf-row input{accent-color:var(--accent)}
.tf-reset{font:inherit;font-size:11.5px;background:none;border:none;color:var(--accent);cursor:pointer;padding:8px 0 0;margin-top:4px;border-top:1px solid var(--border);width:100%;text-align:left}
.tf-reset:hover{text-decoration:underline}
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
.toptabs{display:flex;gap:20px;margin-bottom:14px;border-bottom:1px solid var(--border)}
.toptab{font:inherit;font-size:14px;font-weight:600;background:none;border:none;border-bottom:2px solid transparent;color:var(--text-dim);padding:0 0 10px;cursor:pointer}
.toptab:hover{color:var(--text)}
.toptab.active{color:var(--accent);border-bottom-color:var(--accent)}
.toptab.active .tcount{color:var(--accent)}
.archived-project{margin-bottom:10px;border:1px solid var(--border);border-radius:8px;background:var(--surface);overflow:hidden}
.archived-project>summary{cursor:pointer;list-style:none;display:flex;align-items:baseline;gap:6px;padding:12px 14px;font-size:13.5px;font-weight:600;color:var(--text)}
.archived-project>summary::-webkit-details-marker{display:none}
.archived-project>summary:hover{background:var(--surface-2)}
.archived-project>summary::before{content:'\203A';color:var(--text-faint);font-size:16px;line-height:1;margin-right:2px;transition:transform .15s}
.archived-project[open]>summary::before{transform:rotate(90deg)}
.archived-project .plans{padding:0 14px 14px}
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
