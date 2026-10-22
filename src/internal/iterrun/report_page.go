package iterrun

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// PlanPageOptions are the plan page's surroundings.
type PlanPageOptions struct {
	// HomeURL adds a back link when the page has no application chrome.
	HomeURL string
	// Chrome is trusted application navigation rendered first on the page.
	// A page with chrome is the dashboard's view of recorded observations;
	// a plan that is not archived also gets the Conductor activity region.
	Chrome string
	// HistoryURL, when set, is where the page fetches tool history after it
	// renders. A status line says so and offers a retry when it fails.
	HistoryURL string
}

// reportNavItem is one entry in the page's "On this page" list.
type reportNavItem struct{ id, label string }

// RenderPlanPage renders a plan's page in reading order: application
// navigation, the plan header, the outcome, progress and evidence summary,
// the execution timeline, the breakdowns, and the Conductor activity in a
// bounded region last. Everything that tool history can change sits inside
// #plan-recorded-content, which the history loader replaces as one unit.
func RenderPlanPage(in ReportInput, opt PlanPageOptions) string {
	plan := in.Plan
	var rep *PlanReport
	if plan.Name != "" {
		r := BuildPlanReport(in)
		rep = &r
	}
	recorded := plan.Archived || opt.Chrome != ""
	live := opt.Chrome != "" && !plan.Archived

	title := "iterate-run plan report"
	if plan.Name != "" {
		title = plan.Name + " — iterate-run plan report"
	}

	var b strings.Builder
	b.WriteString(timelineHeadCSS(title))
	b.WriteString(planPageCSS)
	b.WriteString(`</style></head><body><a class="skip-link" href="#main">Skip to the plan report</a>`)
	if opt.Chrome != "" {
		b.WriteString(opt.Chrome)
	} else if opt.HomeURL != "" {
		b.WriteString(`<nav class="plan-back" aria-label="Dashboard"><a class="back" href="` + html.EscapeString(opt.HomeURL) + `">&larr; Dashboard</a></nav>`)
	}
	b.WriteString(`<main id="main" tabindex="-1">`)
	writePlanHeader(&b, plan)

	var nav []reportNavItem
	if rep != nil {
		nav = append(nav, reportNavItem{"report-progress", "Progress"}, reportNavItem{"report-evidence", "Evidence"})
	}
	nav = append(nav, reportNavItem{"plan-timeline", "Timeline"})
	if rep != nil {
		nav = append(nav, reportNavItem{"report-time", "Time"}, reportNavItem{"report-ownership", "Ownership"},
			reportNavItem{"report-rework", "Attempts"}, reportNavItem{"report-usage", "Usage"},
			reportNavItem{"report-coverage", "Coverage"}, reportNavItem{"report-delivery", "Delivery"})
	}
	if live {
		nav = append(nav, reportNavItem{"conductor-live-detail", "Conductor activity"})
	}
	b.WriteString(`<nav class="report-toc" aria-labelledby="report-toc-h"><h2 id="report-toc-h">On this page</h2><ol>`)
	for _, n := range nav {
		fmt.Fprintf(&b, `<li><a href="#%s">%s</a></li>`, n.id, n.label)
	}
	b.WriteString(`</ol></nav>`)

	if opt.HistoryURL != "" {
		u := html.EscapeString(opt.HistoryURL)
		b.WriteString(`<p id="plan-history-status" role="status" data-history-url="` + u + `">Plan details are ready. Loading tool history and token usage…</p><noscript><p><a href="` + u + `">Load complete tool history and token usage</a></p></noscript>`)
	}

	b.WriteString(`<div id="plan-recorded-content">`)
	reqs := map[int]ProgressRequirement{}
	if rep != nil {
		writePlanElapsed(&b, rep.Time)
		writeReportSummary(&b, *rep)
		for _, r := range rep.Progress.RequirementItems {
			reqs[r.Number] = r
		}
	}
	writeTimelineSection(&b, in.Rows, recorded, reqs)
	if rep != nil {
		writeReportBreakdowns(&b, *rep)
	}
	if live {
		b.WriteString(conductorActivityHTML)
	}
	b.WriteString(`<footer>Built from what's already on disk — each team's own log file plus any iterate-run-wrapped unit's registry timestamps, merged with hook-derived tool-call data wherever it's been captured, and the plan's run record.</footer>`)
	b.WriteString(`</div></main>`)
	b.WriteString(reportPageScript)
	b.WriteString(`</body></html>`)
	return b.String()
}

// writePlanHeader writes the plan's identity: name, harness and launch
// command, the operator banner when the plan waits on a person, and the goal.
func writePlanHeader(b *strings.Builder, plan PlanSummary) {
	b.WriteString(`<header class="plan-head">`)
	if plan.Archived {
		b.WriteString(`<div class="eyebrow">iterate plan <span class="archived-chip">archived</span></div>`)
	} else {
		b.WriteString(`<div class="eyebrow">iterate plan</div>`)
	}
	b.WriteString(`<h1>` + html.EscapeString(plan.Name) + `</h1>`)
	b.WriteString(planMetaLine(plan))
	if plan.Blocked() && plan.NextAttempt != "" {
		// The most urgent thing on the page: a plan can have every team done
		// and still wait on one act only a person can take.
		b.WriteString(`<div class="blocked-banner"><div class="blocked-label">Needs you</div><p>` + html.EscapeString(plan.NextAttempt) + `</p></div>`)
	}
	if plan.GoalFull != "" {
		b.WriteString(`<div class="goal"><div class="goal-label">Goal</div><p>` + html.EscapeString(plan.GoalFull) + `</p></div>`)
	}
	b.WriteString(`</header>`)
}

// writePlanElapsed writes the page's one elapsed figure. It is the Time
// section's clock, so the summary and the breakdown never disagree.
func writePlanElapsed(b *strings.Builder, t TimeSection) {
	if t.Clock == nil || t.WallMS == nil {
		b.WriteString(`<p class="plan-elapsed" id="plan-elapsed"><span class="plan-elapsed-label">Elapsed</span> <strong>unknown</strong> <span>no execution start is recorded · <a href="#report-time">Time</a></span></p>`)
		return
	}
	c := t.Clock
	span := "from " + lcTime(c.Start) + " to " + lcTime(c.End)
	if c.Open {
		span = "from " + lcTime(c.Start) + ", still open at " + lcTime(c.End)
	}
	fmt.Fprintf(b, `<p class="plan-elapsed" id="plan-elapsed"><span class="plan-elapsed-label">Elapsed</span> <strong>%s</strong> <span>%s · <a href="#report-time">Time breakdown</a></span></p>`, lcDur(*t.WallMS), span)
}

// writeTimelineSection writes the execution timeline: the coordinator's lane,
// then the teams' lanes nested by dependency, on one shared clock axis.
func writeTimelineSection(b *strings.Builder, rows []Row, recorded bool, reqs map[int]ProgressRequirement) {
	b.WriteString(`<section class="report-section" id="plan-timeline" aria-labelledby="plan-timeline-h"><h2 id="plan-timeline-h">Execution timeline</h2>`)
	if len(rows) == 0 {
		b.WriteString(`<p class="empty">No activity recorded yet.</p></section>`)
		return
	}

	var minT, maxT time.Time
	for _, r := range rows {
		for _, s := range r.spans {
			if minT.IsZero() || s.start.Before(minT) {
				minT = s.start
			}
			if s.end.After(maxT) {
				maxT = s.end
			}
		}
	}
	// Rows can exist with no confirmed span at all (a flat plan's
	// coordinator owns its steps before any hook data exists). The zero
	// time then only feeds bar positioning, never the page's text.
	hasActivity := !minT.IsZero()
	total := maxT.Sub(minT)
	if total <= 0 {
		total = time.Second
	}
	pct := func(t time.Time) float64 {
		return 100 * float64(t.Sub(minT)) / float64(total)
	}
	if hasActivity {
		fmt.Fprintf(b, `<p class="sub">Bars run from the first to the last recorded activity, %s to %s (%s): tool calls, wrapped units and team-log entries. A gap between bars is not a failure; <a href="#report-time">Time</a> accounts for every interval.</p>`,
			lcTime(minT), lcTime(maxT), lcDur(lcMS(maxT.Sub(minT))))
	} else {
		b.WriteString(`<p class="sub">No confirmed activity yet: this plan has no hook or iterate-run-wrapped registry data on this machine.</p>`)
	}

	// The coordinator dispatches and merges everyone else, so it gets its
	// own block above the teams rather than sorting in among them.
	var coordRow *Row
	var teamRows []Row
	for i := range rows {
		if rows[i].key == "" {
			r := rows[i]
			coordRow = &r
		} else {
			teamRows = append(teamRows, rows[i])
		}
	}
	teamRows = orderTeamRows(teamRows)

	if coordRow != nil {
		b.WriteString(`<h3>Coordinator</h3><div class="gantt">`)
		writeGanttRow(b, *coordRow, pct, false, recorded, reqs)
		if hasActivity {
			writeTimeAxis(b, minT.Local(), maxT.Local(), pct)
		}
		b.WriteString(`</div>`)
	}

	// The divider separates root-level dependency groups only, never a
	// parent from its own nested children. An unteamed plan has only the
	// coordinator, so it gets no empty team block.
	if len(teamRows) > 0 {
		b.WriteString(`<h3>Activity by team</h3><div class="gantt">`)
		firstRoot := true
		for _, r := range teamRows {
			divider := false
			if r.depth == 0 {
				divider = !firstRoot
				firstRoot = false
			}
			writeGanttRow(b, r, pct, divider, recorded, reqs)
		}
		if hasActivity {
			writeTimeAxis(b, minT.Local(), maxT.Local(), pct)
			fmt.Fprintf(b, `<div class="axis"><span>%s</span><span>%s</span></div>`,
				minT.Local().Format("15:04:05"), maxT.Local().Format("15:04:05")+" (latest)")
		}
		b.WriteString(`</div>`)
	}

	openLabel := "still running"
	if recorded {
		openLabel = "open at last recorded observation"
	}
	b.WriteString(`<div class="legend"><span><span class="sw" aria-hidden="true"></span>confirmed activity</span><span><span class="sw sw-open" aria-hidden="true"></span>` + openLabel + `</span></div>`)
	b.WriteString(`</section>`)
}

// conductorActivityHTML is the live Conductor region. detail.js fills it and
// keeps the node across history loads; its event log scrolls inside a fixed
// maximum height so a long log never pushes the report down.
const conductorActivityHTML = `<section id="conductor-live-detail" aria-labelledby="conductor-activity-title"><h2 id="conductor-activity-title">Conductor activity</h2><div data-status aria-live="polite"><p>Loading activity…</p></div><details class="conductor-events" open><summary>Event log</summary><div class="conductor-event-scroll" role="region" aria-label="Conductor event log" tabindex="0"><p>Loading events…</p></div></details><details class="conductor-usage"><summary>Usage observations</summary><div data-usage></div></details></section>`

// reportPageScript marks the requirement cell whose evidence the address
// points at. It sits outside #plan-recorded-content, so it keeps working
// after the history loader replaces that content.
const reportPageScript = `<script>(function(){function m(){var h='';try{h=decodeURIComponent(location.hash.slice(1))}catch(e){}document.querySelectorAll('a.bd-cell').forEach(function(a){a.classList.toggle('bd-selected',h!==''&&a.getAttribute('href')==='#'+h)})}window.addEventListener('hashchange',m);document.addEventListener('plan-report-replaced',m);m()})()</script>`

// planPageCSS completes timelineHeadCSS for the plan page: landmarks, the
// section list, the narrow-screen gantt and the Conductor region.
const planPageCSS = `
.vh{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0);clip-path:inset(50%);white-space:nowrap}
.skip-link{position:absolute;left:12px;top:-80px;z-index:200;background:var(--surface);color:var(--accent);border:2px solid var(--accent);border-radius:6px;padding:8px 12px;font-size:13px;font-weight:600;text-decoration:none}
.skip-link:focus{top:12px}
main,.plan-back,.conductor-detail-header,.conductor-detail-nav{max-width:900px;margin-left:auto;margin-right:auto}
main{overflow-wrap:anywhere}
main:focus{outline:none}
main a:focus-visible,main summary:focus-visible,.skip-link:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
.plan-head{margin:0 0 6px}
.report-toc{margin:0 0 16px;padding:8px 12px;background:var(--surface);border:1px solid var(--border);border-radius:8px;display:flex;flex-wrap:wrap;align-items:center;gap:4px 12px}
.report-toc h2{margin:0;font-size:11px}
.report-toc ol{display:flex;flex-wrap:wrap;gap:2px 4px;margin:0;padding:0;list-style:none}
.report-toc a{display:inline-flex;align-items:center;min-height:28px;padding:2px 8px;border-radius:6px;font-size:13px;color:var(--accent);text-decoration:none}
.report-toc a:hover{background:var(--surface-2);text-decoration:underline}
#plan-history-status{font-size:12.5px;color:var(--text-dim);margin:0 0 12px}
#plan-history-status button{font:inherit;margin-left:6px;min-height:28px;padding:2px 10px;border-radius:6px;border:1px solid var(--accent);background:var(--surface);color:var(--accent);cursor:pointer}
.plan-elapsed{display:flex;flex-wrap:wrap;align-items:baseline;gap:4px 10px;margin:0 0 4px;padding:10px 14px;background:var(--surface);border:1px solid var(--border);border-left:3px solid var(--accent);border-radius:8px;font-size:12.5px;color:var(--text-dim)}
.plan-elapsed-label{font-size:11px;letter-spacing:.06em;text-transform:uppercase;font-weight:600}
.plan-elapsed strong{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-variant-numeric:tabular-nums;font-size:16px;color:var(--text)}
.plan-elapsed a{color:var(--accent)}
#plan-timeline{margin:26px 0}
#plan-timeline .tick-lab{overflow-wrap:normal}
.step-pair .step-text{min-width:0;overflow-wrap:anywhere}
#plan-timeline h2{margin:0 0 8px}
#plan-timeline h3{font-size:12px;letter-spacing:.04em;text-transform:uppercase;color:var(--text-dim);font-weight:600;margin:16px 0 8px}
#plan-timeline .sub a{color:var(--accent)}
.step-ev{font-size:12px;color:var(--accent);align-self:flex-start;min-height:24px;display:inline-flex;align-items:center}
@media (prefers-reduced-motion:reduce){.vmark.v-working{animation:none}.chev{transition:none}}
.conductor-detail-header,.conductor-detail-nav{margin-bottom:20px;overflow-wrap:anywhere}
.conductor-detail-header{display:flex;justify-content:space-between}
.conductor-detail-header a,.conductor-detail-nav a{color:var(--accent)}
.conductor-detail-nav{font-size:14px;line-height:1.8;padding-bottom:16px;border-bottom:1px solid var(--border)}
.conductor-detail-nav p{font-size:12px;margin:2px 0;color:var(--text-dim)}
#conductor-live-detail{margin:26px 0;padding:16px;background:var(--surface);border:1px solid var(--border);border-radius:10px;font-size:13px;overflow-wrap:anywhere}
#conductor-live-detail h2{margin:0 0 10px}
#conductor-live-detail p{margin:6px 0;color:var(--text-dim)}
#conductor-live-detail summary{cursor:pointer;padding:8px 0;min-height:24px;color:var(--accent);font-weight:600}
.conductor-events{margin-top:10px;border-top:1px solid var(--border)}
.conductor-event-scroll{min-height:72px;max-height:min(260px,40vh);overflow-y:auto;overscroll-behavior:contain;scrollbar-gutter:stable;background:var(--surface-2);border:1px solid var(--border);border-radius:6px;padding:10px 12px}
.conductor-event-scroll:focus-visible,#conductor-live-detail summary:focus-visible{outline:2px solid var(--accent);outline-offset:3px}
.conductor-event-scroll h3{font-size:13px;margin:14px 0 8px}
.conductor-event-scroll h3:first-child{margin-top:0}
.conductor-event-scroll p{font-family:ui-monospace,SFMono-Regular,Menlo,monospace;font-size:12px;padding-bottom:6px;border-bottom:1px solid var(--border)}
.conductor-usage{font-size:12px}
@media (max-width:600px){
body{padding:20px 16px 40px}
.row{flex-wrap:wrap;gap:4px 10px;padding:8px}
.label-col{width:auto;flex:1 1 0;min-width:0}
.busy{width:auto}
.track{flex:1 0 100%;order:3}
.axis-ticks{padding:4px 8px 0;gap:0}
.axis-ticks .ax-pad,.axis-ticks .ax-pad-r{display:none}
.axis{padding:2px 8px}
.steps-detail{padding:6px 10px 12px 12px}
}
`
