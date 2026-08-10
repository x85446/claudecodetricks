package iterrun

import (
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// NotableGap is a gap between two tool calls on the same row long enough to
// be worth flagging. Two tiers: Notable (worth a look) and Severe (the
// "where did the time actually go" kind — dolphin sitting for 12 hours,
// the app-team's 10.5-minute stall, that class of thing).
const (
	NotableGap = 60 * time.Second
	SevereGap  = 5 * time.Minute
)

// span is one busy interval on a row: a tool call from its pre to its post.
type span struct {
	start, end time.Time
	tool       string
	summary    string
}

// gap is idle time between two spans on the same row.
type gap struct {
	start, end time.Time
}

func (g gap) dur() time.Duration { return g.end.Sub(g.start) }

// StepDetail is one numbered Steps/Validation pair from the plan file,
// attached to whichever team's Teams-table row claims that step number —
// the "1a/1b" pairing shown in chat, now shown on click in the dashboard.
type StepDetail struct {
	Num        int
	Step       string
	Validation string
}

// Row is one agent's (or the coordinator's) full activity picture. Status
// and Steps are only ever populated for filesystem-derived rows, straight
// from the plan's own Teams table — Status is "done", "in-progress",
// "pending", or a "blocked(...)" reason; both are empty for hook-only rows
// (there's no Teams row to read them from) and for a plain, unteamed
// plan's coordinator row.
type Row struct {
	key    string // agent_id, or "" for coordinator
	label  string
	status string
	depth  int // position in the Depends-on chain — 0 for a team with no dependency, for display indentation only
	steps  []StepDetail
	spans  []span
	gaps   []gap
}

// computeGaps finds the idle stretches between consecutive (already sorted)
// spans that clear the NotableGap threshold — shared by every row source,
// hook-derived or filesystem-derived alike.
func computeGaps(spans []span) []gap {
	var gaps []gap
	for i := 1; i < len(spans); i++ {
		g := gap{start: spans[i-1].end, end: spans[i].start}
		if g.dur() >= NotableGap {
			gaps = append(gaps, g)
		}
	}
	return gaps
}

// BuildRows groups events into per-agent rows, pairs pre/post events into
// spans by tool_use_id, and computes the gaps between consecutive spans —
// the gaps are the actual point of this whole thing.
func BuildRows(events []Event, labels map[string]string) []Row {
	byKey := map[string][]Event{}
	for _, e := range events {
		byKey[e.AgentID] = append(byKey[e.AgentID], e)
	}

	var rows []Row
	for key, evs := range byKey {
		pre := map[string]Event{}
		var spans []span
		for _, e := range evs {
			if e.Hook == "pre" {
				pre[e.ToolUseID] = e
				continue
			}
			if p, ok := pre[e.ToolUseID]; ok {
				spans = append(spans, span{start: p.TS, end: e.TS, tool: e.ToolName, summary: e.Summary})
				delete(pre, e.ToolUseID)
			}
		}
		sort.Slice(spans, func(i, j int) bool { return spans[i].start.Before(spans[j].start) })
		gaps := computeGaps(spans)

		label := key
		if key == "" {
			label = "coordinator"
		} else if l, ok := labels[key]; ok {
			label = l
		}

		rows = append(rows, Row{key: key, label: label, spans: spans, gaps: gaps})
	}

	sort.Slice(rows, func(i, j int) bool {
		if rows[i].key == "" {
			return true // coordinator always first
		}
		if rows[j].key == "" {
			return false
		}
		return rows[i].label < rows[j].label
	})
	return rows
}

// BuildRowsFromHookEvents is BuildRows scoped to one plan and keyed by team
// name instead of raw agent_id, so its output merges cleanly with
// BuildRowsFromFilesystem's rows (MergeRows) under the same key per team.
// Events without a resolved Plan (labeling hadn't caught up yet, or this
// call happened outside any plan) are excluded — not every tool call on
// this machine belongs to the timeline being asked for.
func BuildRowsFromHookEvents(events []Event, labels map[string]string, plan string) []Row {
	filtered := events[:0:0]
	for _, e := range events {
		if e.Plan == plan {
			filtered = append(filtered, e)
		}
	}

	byKey := map[string][]Event{}
	for _, e := range filtered {
		byKey[e.Team] = append(byKey[e.Team], e)
	}

	var rows []Row
	for key, evs := range byKey {
		pre := map[string]Event{}
		var spans []span
		for _, e := range evs {
			if e.Hook == "pre" {
				pre[e.ToolUseID] = e
				continue
			}
			if p, ok := pre[e.ToolUseID]; ok {
				spans = append(spans, span{start: p.TS, end: e.TS, tool: e.ToolName, summary: e.Summary})
				delete(pre, e.ToolUseID)
			}
		}
		sort.Slice(spans, func(i, j int) bool { return spans[i].start.Before(spans[j].start) })

		label := key
		if key == "" {
			label = "coordinator"
		} else if l, ok := labels[plan+"-"+key]; ok {
			label = l
		}

		rows = append(rows, Row{key: key, label: label, spans: spans, gaps: computeGaps(spans)})
	}
	return rows
}

// MergeRows unions two row sets by key (team name, "" for coordinator),
// concatenating spans and recomputing gaps over the combined, re-sorted
// list — this is what lets `timeline --plan X` show BOTH the coarse
// team-log/registry spans (always available) and fine-grained hook-derived
// spans (only once hooks are wired and have had time to accumulate) in one
// row per team, rather than forcing a choice between them.
func MergeRows(a, b []Row) []Row {
	byKey := map[string]*Row{}
	order := func(rs []Row) {
		for i := range rs {
			r := rs[i]
			if existing, ok := byKey[r.key]; ok {
				existing.spans = append(existing.spans, r.spans...)
				if existing.label == "" || existing.label == existing.key {
					existing.label = r.label
				}
				continue
			}
			cp := r
			byKey[r.key] = &cp
		}
	}
	order(a)
	order(b)

	var rows []Row
	for _, r := range byKey {
		sort.Slice(r.spans, func(i, j int) bool { return r.spans[i].start.Before(r.spans[j].start) })
		r.gaps = computeGaps(r.spans)
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if len(rows[i].spans) == 0 || len(rows[j].spans) == 0 {
			return rows[i].label < rows[j].label
		}
		return rows[i].spans[0].start.Before(rows[j].spans[0].start)
	})
	return rows
}

var teamTerminalLine = regexp.MustCompile(`^TEAM (DONE|BLOCKED):`)

// BuildRowsFromFilesystem builds one row per team of a plan directly from
// what's already on disk — no hook wiring required, and it works for a run
// already in progress, which hook-derived events never can (they only start
// accumulating from the moment they're wired in). Two sources per team,
// merged into the same row:
//
//   - The team's own log file at <homeDir>/.claude/iterate/plans/<plan>.teams/
//     <team>.log.md: one coarse span from the file's birth time (when the
//     team started) to its last-modified time (its last known write —
//     NOT stretched to "now", since anything after the last write is
//     genuinely unknown, not confirmed activity).
//   - Any `iterate-run run` registry entries tagged with this plan, found by
//     scanning scanDirs (a team can — and did, in practice — work in a
//     directory far from the plan's own project, e.g. a team cloning a
//     side workspace) — these carry exact Started/Finished timestamps.
//
// homeDir is where the plan file and team logs live; scanDirs additionally
// searches for registry entries (homeDir is always scanned too).
func BuildRowsFromFilesystem(plan, homeDir string, scanDirs []string) ([]Row, error) {
	byTeam := map[string]*Row{}
	get := func(team string) *Row {
		if r, ok := byTeam[team]; ok {
			return r
		}
		label := team
		if team == "" {
			label = "coordinator"
		}
		r := &Row{key: team, label: label}
		byTeam[team] = r
		return r
	}

	teamsDir := filepath.Join(homeDir, ".claude", "iterate", "plans", plan+".teams")
	logFiles, _ := filepath.Glob(filepath.Join(teamsDir, "*.log.md"))
	for _, lf := range logFiles {
		team := strings.TrimSuffix(filepath.Base(lf), ".log.md")
		fi, err := os.Stat(lf)
		if err != nil {
			continue
		}
		start, ok := birthTime(fi)
		if !ok {
			start = fi.ModTime()
		}
		end := fi.ModTime()
		if end.Before(start) {
			end = start
		}
		summary := "still running as of last write"
		if data, err := os.ReadFile(lf); err == nil {
			if m := teamTerminalLine.FindString(string(data)); m != "" {
				summary = m
			} else if last := lastNonEmptyLine(string(data)); last != "" {
				summary = truncateSummary(last)
			}
		}
		r := get(team)
		r.spans = append(r.spans, span{start: start, end: end, tool: "team-log", summary: summary})
	}

	seenDirs := map[string]bool{}
	dirs := append([]string{homeDir}, scanDirs...)
	for _, d := range dirs {
		abs, err := filepath.Abs(d)
		if err != nil || seenDirs[abs] {
			continue
		}
		seenDirs[abs] = true
		entries, err := ScanRegistry(RegistryDir(abs))
		if err != nil {
			continue
		}
		for _, e := range entries {
			if e.Plan != plan {
				continue
			}
			end := time.Now()
			if e.Finished != nil {
				end = *e.Finished
			} else if !e.LastHeartbeat.IsZero() {
				end = e.LastHeartbeat
			}
			r := get(e.Team)
			r.spans = append(r.spans, span{start: e.Started, end: end, tool: "iterate-run:" + e.Unit, summary: e.Status + " " + e.LastMessage})
		}
	}

	// Enrich with each team's status and Steps/Validation detail from the
	// plan's own Teams table, and pick up any team that hasn't started at
	// all yet — no log file, no registry entry, so byTeam wouldn't
	// otherwise know it exists. A queued team being invisible is exactly
	// the wrong failure mode for a view meant to show the whole picture,
	// not just the parts that happened.
	teams, steps, validations := readPlanTeamsAndSteps(homeDir, plan)
	depths := teamDepths(teams)
	for team, meta := range teams {
		r := get(team)
		r.status = meta.status
		r.depth = depths[team]
		for _, n := range meta.stepNums {
			sd := StepDetail{Num: n, Step: steps[n], Validation: validations[n]}
			if sd.Step == "" && sd.Validation == "" {
				continue // step number in the table but not found in either list — skip rather than show a blank pair
			}
			r.steps = append(r.steps, sd)
		}
		sort.Slice(r.steps, func(i, j int) bool { return r.steps[i].Num < r.steps[j].Num })
	}

	var rows []Row
	for _, r := range byTeam {
		sort.Slice(r.spans, func(i, j int) bool { return r.spans[i].start.Before(r.spans[j].start) })
		r.gaps = computeGaps(r.spans)
		rows = append(rows, *r)
	}
	sort.Slice(rows, func(i, j int) bool {
		if len(rows[i].spans) == 0 || len(rows[j].spans) == 0 {
			return rows[i].label < rows[j].label
		}
		return rows[i].spans[0].start.Before(rows[j].spans[0].start)
	})
	return rows, nil
}

type teamMeta struct {
	status    string
	stepNums  []int
	dependsOn []string
}

var reNumberedItem = regexp.MustCompile(`^(\d+)\.\s+(.*)$`)

// readPlanTeamsAndSteps reads the plan file once and returns everything
// the dashboard needs from it beyond raw activity: each team's status and
// step-number list from the Teams table, plus the Steps and Validation
// sections themselves keyed by number — the "Na./Nb." pairing shown in
// chat, now available for the dashboard to show the same way on click.
// Returns nils (not an error) if the plan file can't be read.
func readPlanTeamsAndSteps(homeDir, plan string) (teams map[string]teamMeta, steps, validations map[int]string) {
	data, err := os.ReadFile(filepath.Join(homeDir, ".claude", "iterate", "plans", plan+".md"))
	if err != nil {
		return nil, nil, nil
	}
	teams = map[string]teamMeta{}
	steps = map[int]string{}
	validations = map[int]string{}
	section := ""
	for line := range strings.SplitSeq(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			switch trimmed {
			case "## Steps":
				section = "steps"
			case "## Validation":
				section = "validation"
			case "## Teams":
				section = "teams"
			default:
				section = ""
			}
			continue
		}
		switch section {
		case "steps":
			if m := reNumberedItem.FindStringSubmatch(trimmed); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil {
					steps[n] = m[2]
				}
			}
		case "validation":
			if m := reNumberedItem.FindStringSubmatch(trimmed); m != nil {
				if n, err := strconv.Atoi(m[1]); err == nil {
					validations[n] = m[2]
				}
			}
		case "teams":
			if cells, ok := parseTeamRowCells(line); ok {
				var stepsCol, dependsCol string
				if len(cells) >= 2 {
					stepsCol = cells[1]
				}
				if len(cells) >= 4 {
					dependsCol = cells[3]
				}
				teams[cells[0]] = teamMeta{status: cells[len(cells)-1], stepNums: parseIntList(stepsCol), dependsOn: parseTeamList(dependsCol)}
			}
		}
	}
	return teams, steps, validations
}

func parseIntList(s string) []int {
	var nums []int
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if n, err := strconv.Atoi(part); err == nil {
			nums = append(nums, n)
		}
	}
	return nums
}

// parseTeamList parses a Depends-on cell ("engine, storage, proforma" or
// "—" for none) into team names.
func parseTeamList(s string) []string {
	var names []string
	for part := range strings.SplitSeq(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" || part == "—" || part == "-" {
			continue
		}
		names = append(names, part)
	}
	return names
}

// teamDepths computes each team's depth in the dependency graph — 0 for a
// team with no dependencies, otherwise one more than the deepest of its
// dependencies. Used purely for display indentation (see RenderTimelineHTML)
// so the two independent chains in a plan like finch — engine's Rust track
// and win-media's Windows track — read as visually distinct hierarchies
// instead of one flat list that hides which teams could actually run in
// parallel and which were waiting on another.
func teamDepths(teams map[string]teamMeta) map[string]int {
	depth := map[string]int{}
	var resolve func(name string, seen map[string]bool) int
	resolve = func(name string, seen map[string]bool) int {
		if d, ok := depth[name]; ok {
			return d
		}
		if seen[name] {
			return 0 // dependency cycle — shouldn't happen, never hang on it
		}
		seen[name] = true
		meta, ok := teams[name]
		if !ok || len(meta.dependsOn) == 0 {
			depth[name] = 0
			return 0
		}
		max := 0
		for _, dep := range meta.dependsOn {
			if d := resolve(dep, seen); d+1 > max {
				max = d + 1
			}
		}
		depth[name] = max
		return max
	}
	for name := range teams {
		resolve(name, map[string]bool{})
	}
	return depth
}

func lastNonEmptyLine(s string) string {
	lines := strings.Split(strings.TrimRight(s, "\n"), "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			return strings.TrimSpace(lines[i])
		}
	}
	return ""
}

func truncateSummary(s string) string {
	if len(s) > 120 {
		return s[:120] + "…"
	}
	return s
}

// PrintTimelineSummary writes a plain-text per-agent summary: busy time,
// call count, and every notable gap with its duration — the quick terminal
// answer to "where did the time go," no browser required.
func PrintTimelineSummary(w io.Writer, rows []Row) {
	if len(rows) == 0 {
		fmt.Fprintln(w, "no events recorded yet")
		return
	}
	for _, r := range rows {
		var busy time.Duration
		for _, s := range r.spans {
			busy += s.end.Sub(s.start)
		}
		fmt.Fprintf(w, "%s — %d tool calls, %s busy", r.label, len(r.spans), busy.Round(time.Second))
		if len(r.spans) > 0 {
			fmt.Fprintf(w, ", active %s to %s", r.spans[0].start.Local().Format("15:04:05"), r.spans[len(r.spans)-1].end.Local().Format("15:04:05"))
		}
		fmt.Fprintln(w)
		for _, g := range r.gaps {
			marker := "  gap"
			if g.dur() >= SevereGap {
				marker = "  SEVERE GAP"
			}
			fmt.Fprintf(w, "%s: %s (%s -> %s)\n", marker, g.dur().Round(time.Second), g.start.Local().Format("15:04:05"), g.end.Local().Format("15:04:05"))
		}
	}
}

// RenderTimelineHTML builds a self-contained Gantt-style timeline page: one
// row per agent, busy spans as bars, gaps left visibly blank (and colored
// when severe) so downtime is something you SEE, not something you
// reconstruct from timestamps by hand. Pure string building — no file I/O —
// so it's equally usable for iterate-run timeline's file output and the
// dashboard server's inline HTTP responses.
func RenderTimelineHTML(rows []Row, plan PlanSummary, homeURL string) string {
	planName := plan.Name
	title := "iterate-run timeline"
	if planName != "" {
		title = planName + " — iterate-run timeline"
	}
	backLink := ""
	if homeURL != "" {
		backLink = `<a class="back" href="` + html.EscapeString(homeURL) + `">&larr; Dashboard</a>`
	}
	header := backLink + `<div class="eyebrow">iterate plan</div><h1>` + html.EscapeString(planName) + `</h1>`
	if plan.GoalFull != "" {
		header += `<div class="goal"><div class="goal-label">Goal</div><p>` + html.EscapeString(plan.GoalFull) + `</p></div>`
	}
	if len(rows) == 0 {
		return timelineHead(title) + header + `<p class="empty">no activity recorded yet</p></div>`
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
	total := maxT.Sub(minT)
	if total <= 0 {
		total = time.Second
	}
	pct := func(t time.Time) float64 {
		return 100 * float64(t.Sub(minT)) / float64(total)
	}

	// Sort rows for display: anything with real spans first (chronological
	// by first activity), queued/not-yet-started teams last regardless of
	// what their zero-value key sort would otherwise do.
	display := make([]Row, len(rows))
	copy(display, rows)
	sort.SliceStable(display, func(i, j int) bool {
		iq, jq := len(display[i].spans) == 0, len(display[j].spans) == 0
		if iq != jq {
			return !iq
		}
		return false
	})

	var done, running, queued, severeGaps int
	var gapCallouts []struct {
		label string
		g     gap
	}
	for _, r := range rows {
		switch {
		case strings.HasPrefix(r.status, "done") || strings.HasPrefix(r.status, "blocked"):
			done++
		case r.status == "in-progress":
			running++
		case len(r.spans) == 0:
			queued++
		}
		for _, g := range r.gaps {
			if g.dur() >= SevereGap {
				severeGaps++
				gapCallouts = append(gapCallouts, struct {
					label string
					g     gap
				}{r.label, g})
			}
		}
	}
	sort.Slice(gapCallouts, func(i, j int) bool { return gapCallouts[i].g.dur() > gapCallouts[j].g.dur() })

	var b strings.Builder
	b.WriteString(timelineHead(title))
	b.WriteString(header)
	fmt.Fprintf(&b, `<div class="sub">%s &rarr; %s (%s so far)</div>`+"\n",
		minT.Local().Format("2006-01-02 15:04:05"), maxT.Local().Format("15:04:05"), total.Round(time.Second))

	b.WriteString(`<div class="stats">`)
	fmt.Fprintf(&b, `<div class="stat s-good"><div class="n">%d</div><div class="l">done</div></div>`, done)
	fmt.Fprintf(&b, `<div class="stat s-warn"><div class="n">%d</div><div class="l">running</div></div>`, running)
	fmt.Fprintf(&b, `<div class="stat"><div class="n">%d</div><div class="l">queued</div></div>`, queued)
	fmt.Fprintf(&b, `<div class="stat s-bad"><div class="n">%d</div><div class="l">severe gap%s</div></div>`, severeGaps, plural(severeGaps))
	b.WriteString(`</div>`)

	// The coordinator isn't a team — it's the orchestrator dispatching and
	// merging everyone else — so it gets its own section above the team
	// list rather than sorting in among them wherever its timestamps land.
	var coordRow *Row
	var teamRows []Row
	for i := range display {
		if display[i].key == "" {
			r := display[i]
			coordRow = &r
		} else {
			teamRows = append(teamRows, display[i])
		}
	}

	if coordRow != nil {
		b.WriteString(`<h2>Coordinator</h2><div class="gantt">`)
		writeGanttRow(&b, *coordRow, maxT, pct)
		b.WriteString(`</div>`)
	}

	b.WriteString(`<h2>Activity by team</h2><div class="gantt">`)
	for _, r := range teamRows {
		writeGanttRow(&b, r, maxT, pct)
	}
	fmt.Fprintf(&b, `<div class="axis"><span>%s</span><span>%s</span></div>`,
		minT.Local().Format("15:04:05"), maxT.Local().Format("15:04:05")+" (latest)")
	b.WriteString(`</div>`)

	b.WriteString(`<div class="legend"><span><span class="sw" style="background:var(--busy)"></span>confirmed activity</span><span><span class="sw sw-open"></span>still running</span><span><span class="sw" style="background:var(--danger)"></span>severe gap (5m+)</span><span>unmarked gaps under 5m are normal think-time</span></div>`)

	if len(gapCallouts) > 0 {
		b.WriteString(`<h2>Where the time actually went</h2><div class="gaplist">`)
		for _, gc := range gapCallouts {
			fmt.Fprintf(&b, `<div class="gap-item"><span class="dur">%s</span><span class="ctx">%s, %s &rarr; %s</span></div>`,
				gc.g.dur().Round(time.Second), html.EscapeString(gc.label),
				gc.g.start.Local().Format("15:04:05"), gc.g.end.Local().Format("15:04:05"))
		}
		b.WriteString(`</div>`)
	}

	b.WriteString(`<footer>Built from what's already on disk — each team's own log file plus any iterate-run-wrapped unit's registry timestamps, merged with hook-derived tool-call data wherever it's been captured.</footer>`)
	b.WriteString(`</div></body></html>`)

	return b.String()
}

// writeGanttRow renders one team (or the coordinator's) row: the status
// pill and label — indented by dependency depth, so a two-independent-
// track plan like finch (engine's Rust chain vs. win-media's Windows
// chain) reads as two visual hierarchies instead of one flat list — the
// activity track with its bars and severe-gap markers, and, if the plan's
// Teams table gave this team any Steps/Validation detail, an expandable
// "Na./Nb." panel underneath (native <details>, no JS).
func writeGanttRow(b *strings.Builder, r Row, maxT time.Time, pct func(time.Time) float64) {
	pillClass, pillLabel := statusPill(r.status, len(r.spans) > 0)
	var busy time.Duration
	for _, s := range r.spans {
		busy += s.end.Sub(s.start)
	}

	hasSteps := len(r.steps) > 0
	rowOpen, rowClose := `<div class="row">`, `</div>`
	teamLabel := html.EscapeString(r.label)
	if hasSteps {
		rowOpen, rowClose = `<details class="row-d"><summary class="row">`, `</summary>`
		teamLabel += `<span class="chev">&rsaquo;</span>`
	}

	pillStyle := ""
	if r.depth > 0 {
		pillStyle = fmt.Sprintf(` style="margin-left:%dpx"`, r.depth*14)
	}

	fmt.Fprintf(b, `%s<div class="label-col"><span class="pill %s" title="%s"%s></span><span class="team">%s</span></div><div class="track">`,
		rowOpen, pillClass, html.EscapeString(pillLabel), pillStyle, teamLabel)
	for _, s := range r.spans {
		left := pct(s.start)
		width := pct(s.end) - left
		if width < 0.15 {
			width = 0.15 // keep even instant calls visible
		}
		cls := "bar"
		if s.end.Equal(maxT) && r.status == "in-progress" {
			cls = "bar bar-open"
		}
		fmt.Fprintf(b, `<div class="%s" style="left:%.3f%%;width:%.3f%%" title="%s: %s"></div>`,
			cls, left, width, html.EscapeString(s.tool), html.EscapeString(s.summary))
	}
	for _, g := range r.gaps {
		if g.dur() < SevereGap {
			continue // only the severe tier gets a visible downtime marker; short gaps are normal think-time
		}
		left := pct(g.start)
		width := pct(g.end) - left
		fmt.Fprintf(b, `<div class="gapmark" style="left:%.3f%%;width:%.3f%%" title="downtime: %s"></div>`,
			left, width, g.dur().Round(time.Second))
	}
	if len(r.spans) == 0 && (strings.HasPrefix(r.status, "done") || strings.HasPrefix(r.status, "blocked")) {
		// Done with an empty track is a real, recurring shape here — a step
		// can be marked done in the Teams table while its actual work ran
		// inside another team's reused agent (confirmed live: cli's work
		// landed in engine's log). Say so instead of leaving a blank bar
		// that looks identical to "something's wrong."
		b.WriteString(`<div class="track-note">done — activity recorded under another team</div>`)
	}
	b.WriteString(`</div>`)
	if len(r.spans) > 0 {
		fmt.Fprintf(b, `<div class="busy">%s</div>`, busy.Round(time.Second))
	} else {
		b.WriteString(`<div class="busy">—</div>`)
	}
	b.WriteString(rowClose + "\n")

	if hasSteps {
		b.WriteString(`<div class="steps-detail">`)
		for _, sd := range r.steps {
			if sd.Step != "" {
				fmt.Fprintf(b, `<div class="step-pair"><span class="stepnum">%da.</span><span>%s</span></div>`, sd.Num, html.EscapeString(sd.Step))
			}
			if sd.Validation != "" {
				fmt.Fprintf(b, `<div class="step-pair val"><span class="stepnum">%db.</span><span>%s</span></div>`, sd.Num, html.EscapeString(sd.Validation))
			}
		}
		b.WriteString(`</div></details>` + "\n")
	}
}

func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

func statusPill(status string, hasActivity bool) (class, label string) {
	switch {
	case strings.HasPrefix(status, "done"):
		return "p-done", "done"
	case strings.HasPrefix(status, "blocked"):
		return "p-blocked", status
	case status == "in-progress":
		return "p-running", "running"
	case status == "":
		if hasActivity {
			return "p-running", "running"
		}
		return "p-queued", "queued"
	default:
		return "p-queued", status
	}
}

func timelineHead(title string) string {
	return `<!doctype html><html><head><meta charset="utf-8"><title>` + html.EscapeString(title) + `</title><style>
:root{
  --bg:#f7f4ef; --surface:#fff; --surface-2:#fbf9f5; --border:#e4ddd0;
  --text:#1c1a17; --text-dim:#6b6459; --text-faint:#a29b8d;
  --busy:#0891b2; --busy-dim:#cceff5;
  --good:#059669; --good-bg:#e3f6ee;
  --warn:#b45309; --warn-bg:#fbecd5;
  --queued:#a29b8d; --queued-bg:#f0ece2;
  --danger:#dc2626; --danger-a:#f3b8b8; --danger-b:#fbe1e1;
  --accent:#0e7490; --accent-bg:#e5f4f7;
}
@media (prefers-color-scheme:dark){:root{
  --bg:#0f1215; --surface:#171b1f; --surface-2:#1d2227; --border:#262c31;
  --text:#e8e6e1; --text-dim:#8b9198; --text-faint:#565d64;
  --busy:#2dd4bf; --busy-dim:#144e49;
  --good:#34d399; --good-bg:#0d2b21;
  --warn:#f59e0b; --warn-bg:#3a2705;
  --queued:#565d64; --queued-bg:#1d2227;
  --danger:#ef4444; --danger-a:#5a1414; --danger-b:#2c0a0a;
  --accent:#22d3ee; --accent-bg:#0f2e33;
}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;padding:28px 20px 50px;line-height:1.45}
.wrap,body>div{max-width:900px;margin:0 auto}
.mono,.n,.dur,.busy,.axis,.pmeta{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-variant-numeric:tabular-nums}
.eyebrow{font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:var(--accent);font-weight:600}
h1{font-size:24px;font-weight:700;margin:2px 0 8px;text-wrap:balance;letter-spacing:-.01em}
h2{font-size:13px;text-transform:uppercase;letter-spacing:.06em;color:var(--text-dim);font-weight:600;margin:26px 0 12px}
.sub{color:var(--text-dim);font-size:12px;margin-bottom:18px}
.back{display:inline-block;font-size:12.5px;color:var(--accent);text-decoration:none;margin-bottom:10px}
.back:hover{text-decoration:underline}
.goal{margin:0 0 18px;padding:14px 16px;background:var(--accent-bg);border:1px solid var(--border);border-radius:8px}
.goal-label{font-size:10px;letter-spacing:.08em;text-transform:uppercase;color:var(--accent);font-weight:700;margin-bottom:5px}
.goal p{color:var(--text-dim);font-size:13.5px;margin:0}
.empty{color:var(--text-faint);font-style:italic}
.stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(100px,1fr));gap:1px;background:var(--border);border:1px solid var(--border);border-radius:10px;overflow:hidden}
.stat{background:var(--surface);padding:12px 16px}
.stat .n{font-size:20px;font-weight:700}
.stat .l{font-size:11px;color:var(--text-dim);text-transform:uppercase;letter-spacing:.05em;margin-top:2px}
.s-good .n{color:var(--good)}
.s-warn .n{color:var(--warn)}
.s-bad .n{color:var(--danger)}
.gantt{border:1px solid var(--border);border-radius:10px;background:var(--surface);padding:6px}
.row{display:flex;align-items:center;gap:14px;padding:9px 10px}
.row-d,.row-d>.row{border-top:1px solid var(--border)}
.row-d:first-child,.row-d:first-child>.row{border-top:none}
.row+.row{border-top:1px solid var(--border)}
summary.row{cursor:pointer;list-style:none}
summary.row::-webkit-details-marker{display:none}
summary.row:hover{background:var(--surface-2)}
.label-col{width:172px;flex:0 0 auto;display:flex;align-items:center;gap:8px;overflow:hidden}
.team{font-size:13px;white-space:nowrap;overflow:hidden;text-overflow:ellipsis;display:flex;align-items:center;gap:4px}
.chev{color:var(--text-faint);font-size:14px;transition:transform .15s}
details[open]>summary .chev{transform:rotate(90deg)}
.pill{width:7px;height:7px;border-radius:50%;flex:0 0 auto}
.p-done{background:var(--good)}
.p-running{background:var(--warn);box-shadow:0 0 0 3px var(--warn-bg)}
.p-queued{background:var(--queued)}
.p-blocked{background:var(--danger)}
.track{position:relative;flex:1 1 auto;height:20px;background:var(--surface-2);border-radius:4px}
.bar{position:absolute;top:0;height:100%;background:var(--busy);border-radius:3px}
.bar:hover{background:var(--good);z-index:2}
.bar-open{background:repeating-linear-gradient(115deg,var(--busy),var(--busy) 7px,var(--busy-dim) 7px,var(--busy-dim) 14px)}
.gapmark{position:absolute;top:-3px;height:calc(100% + 6px);background:repeating-linear-gradient(45deg,var(--danger-a),var(--danger-a) 4px,var(--danger-b) 4px,var(--danger-b) 8px);border-radius:3px;border:1px solid var(--danger)}
.busy{width:60px;flex:0 0 auto;text-align:right;font-size:12px;color:var(--text-dim)}
.track-note{position:absolute;top:0;left:0;height:100%;display:flex;align-items:center;padding-left:8px;font-size:11px;font-style:italic;color:var(--text-faint)}
.axis{display:flex;justify-content:space-between;padding:8px 10px 2px;font-size:10.5px;color:var(--text-faint)}
.legend{display:flex;flex-wrap:wrap;gap:18px;margin-top:14px;font-size:12px;color:var(--text-dim)}
.legend .sw{display:inline-block;width:10px;height:10px;border-radius:2px;margin-right:6px;vertical-align:-1px;background:var(--busy)}
.legend .sw-open{background:repeating-linear-gradient(115deg,var(--busy),var(--busy) 3px,var(--busy-dim) 3px,var(--busy-dim) 6px)}
.steps-detail{padding:6px 14px 14px 46px;background:var(--surface-2);display:flex;flex-direction:column;gap:6px}
.step-pair{display:flex;gap:8px;font-size:12.5px;color:var(--text)}
.step-pair.val{color:var(--text-dim)}
.step-pair .stepnum{flex:0 0 auto;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;color:var(--accent);font-weight:600;min-width:28px}
.gaplist{display:flex;flex-direction:column;gap:8px}
.gap-item{display:flex;gap:10px;align-items:baseline;font-size:13px;padding:9px 12px;background:var(--surface);border:1px solid var(--border);border-left:3px solid var(--danger);border-radius:6px}
.gap-item .dur{font-weight:700;color:var(--danger)}
.gap-item .ctx{color:var(--text-dim);font-size:12px}
footer{margin-top:32px;font-size:11.5px;color:var(--text-faint);border-top:1px solid var(--border);padding-top:14px}
</style></head><body><div>
`
}

// WriteTimelineHTML renders and writes the timeline to
// <cwd>/.claude/iterate/timeline.html — the file-output counterpart to
// RenderTimelineHTML, kept separate since a live dashboard shouldn't write
// to disk on every request.
func WriteTimelineHTML(cwd string, rows []Row, plan PlanSummary) (string, error) {
	if len(rows) == 0 {
		return "", fmt.Errorf("no events to render")
	}
	path := filepath.Join(cwd, ".claude", "iterate", "timeline.html")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(RenderTimelineHTML(rows, plan, "")), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
