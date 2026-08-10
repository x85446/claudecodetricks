package iterrun

import (
	"fmt"
	"html"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
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

// Row is one agent's (or the coordinator's) full activity picture.
type Row struct {
	key   string // agent_id, or "" for coordinator
	label string
	spans []span
	gaps  []gap
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

// WriteTimelineHTML renders a self-contained Gantt-style timeline: one row
// per agent, busy spans as bars, gaps left visibly blank (and colored when
// severe) so downtime is something you SEE, not something you reconstruct
// from timestamps by hand.
func WriteTimelineHTML(cwd string, rows []Row) (string, error) {
	if len(rows) == 0 {
		return "", fmt.Errorf("no events to render")
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

	var b strings.Builder
	b.WriteString(`<!doctype html><html><head><meta charset="utf-8"><title>iterate-run timeline</title><style>
body{font-family:-apple-system,sans-serif;background:#111;color:#eee;padding:24px;margin:0}
h1{font-size:16px;font-weight:600;margin:0 0 4px}
.sub{color:#888;font-size:12px;margin-bottom:20px}
.row{display:flex;align-items:center;margin-bottom:10px}
.label{width:200px;flex:0 0 auto;font-size:12px;color:#ccc;padding-right:12px;text-align:right;white-space:nowrap;overflow:hidden;text-overflow:ellipsis}
.track{position:relative;flex:1 1 auto;height:22px;background:#1c1c1c;border-radius:3px}
.bar{position:absolute;top:0;height:100%;background:#3b82f6;border-radius:2px}
.bar:hover{background:#60a5fa}
.gapmark{position:absolute;top:0;height:100%;background:repeating-linear-gradient(45deg,#7f1d1d,#7f1d1d 4px,#450a0a 4px,#450a0a 8px);border-radius:2px}
.legend{margin-top:20px;font-size:11px;color:#888}
.legend span{display:inline-block;width:10px;height:10px;margin-right:4px;vertical-align:middle;border-radius:2px}
</style></head><body>
`)
	fmt.Fprintf(&b, "<h1>iterate-run timeline</h1><div class=\"sub\">%s &rarr; %s (%s total)</div>\n",
		minT.Local().Format("2006-01-02 15:04:05"), maxT.Local().Format("15:04:05"), total.Round(time.Second))

	for _, r := range rows {
		var busy time.Duration
		for _, s := range r.spans {
			busy += s.end.Sub(s.start)
		}
		fmt.Fprintf(&b, "<div class=\"row\"><div class=\"label\" title=\"%s\">%s (%s busy)</div><div class=\"track\">\n",
			html.EscapeString(r.label), html.EscapeString(r.label), busy.Round(time.Second))
		for _, s := range r.spans {
			left := pct(s.start)
			width := pct(s.end) - left
			if width < 0.15 {
				width = 0.15 // keep even instant calls visible
			}
			fmt.Fprintf(&b, "<div class=\"bar\" style=\"left:%.3f%%;width:%.3f%%\" title=\"%s: %s\"></div>\n",
				left, width, html.EscapeString(s.tool), html.EscapeString(s.summary))
		}
		for _, g := range r.gaps {
			if g.dur() < SevereGap {
				continue // only the severe tier gets a visible downtime marker; short gaps are normal think-time
			}
			left := pct(g.start)
			width := pct(g.end) - left
			fmt.Fprintf(&b, "<div class=\"gapmark\" style=\"left:%.3f%%;width:%.3f%%\" title=\"downtime: %s\"></div>\n",
				left, width, g.dur().Round(time.Second))
		}
		b.WriteString("</div></div>\n")
	}

	b.WriteString(`<div class="legend"><span style="background:#3b82f6"></span>tool active &nbsp; <span style="background:#7f1d1d"></span>downtime 5m+ (unmarked gaps under 5m are normal think-time between calls)</div>
</body></html>`)

	path := filepath.Join(cwd, ".claude", "iterate", "timeline.html")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
