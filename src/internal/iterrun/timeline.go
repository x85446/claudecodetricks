package iterrun

import (
	"encoding/json"
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
	// VStatus and VNote come from the team's own ##ITERATE-VALIDATION##
	// marker for this step, if it wrote one — "met", "partial", "not-met",
	// or "" if the team hasn't reported on this validation at all. This is
	// the team's own real-time assessment, not something reconstructed
	// after the fact — a team can be in-progress overall with several of
	// its own validations already reported met.
	VStatus string
	VNote   string
}

// Row is one agent's (or the coordinator's) full activity picture. Status
// and Steps are only ever populated for filesystem-derived rows, straight
// from the plan's own Teams table — Status is "done", "in-progress",
// "pending", or a "blocked(...)" reason; both are empty for hook-only rows
// (there's no Teams row to read them from) and for a plain, unteamed
// plan's coordinator row.
type Row struct {
	key       string // agent_id, or "" for coordinator
	label     string
	status    string
	depth     int      // position in the Depends-on chain — 0 for a team with no dependency, for display indentation only
	dependsOn []string // this team's own Depends-on list, straight from the Teams table — nil for a team not found there (a flat plan's team, or a dispatch name that doesn't match any Teams-table row)
	steps     []StepDetail
	spans     []span
	gaps      []gap
}

// sortRowsForDisplay is the one row ordering used everywhere rows get
// displayed: active rows (anything with at least one span) first, sorted
// by when they first started; queued rows (nothing recorded yet) last,
// sorted alphabetically. This has to be a total, transitively consistent
// order — three separate call sites each had their own subtly different
// comparator that fell through to comparing labels whenever EITHER side
// was queued, even when comparing an active row against a queued one.
// That's not a valid strict weak ordering, and Go's sort has no defined
// behavior for an invalid one: the actual output depended on the input's
// starting order, which came from ranging over a map — randomized on
// every call — so the displayed order visibly reshuffled on every single
// page refresh even though nothing about the underlying data had changed.
func sortRowsForDisplay(rows []Row) {
	sort.SliceStable(rows, func(i, j int) bool {
		iq, jq := len(rows[i].spans) == 0, len(rows[j].spans) == 0
		if iq != jq {
			return !iq // active before queued
		}
		if !iq {
			return rows[i].spans[0].start.Before(rows[j].spans[0].start)
		}
		return rows[i].label < rows[j].label
	})
}

// orderTeamRows arranges rows into a genuine parent/child hierarchy for
// display: a team appears immediately after its primary dependency (the
// FIRST name in its own Depends-on list — a team can depend on more than
// one other team, but display nesting needs exactly one parent, so the
// first-listed dependency wins), recursively, so a real dependency chain
// reads as one visual group instead of landing wherever its own activity
// happened to start chronologically (confirmed live: windows-port, which
// depends on winvm, was rendering sandwiched between gui-macos2 and
// gui-windows — two teams it has no relationship to at all — purely
// because sortRowsForDisplay only ever looked at start time). A team
// whose named dependency isn't itself a row here (or that has no
// dependency at all — includes any dispatch name that doesn't match a
// Teams-table row, e.g. a split-off subagent) is a root. Sibling order at
// every level is sortRowsForDisplay's existing rule — active-first by
// start time, then queued alphabetically — just scoped to that level's
// siblings instead of the whole flat list.
func orderTeamRows(rows []Row) []Row {
	byKey := map[string]*Row{}
	for i := range rows {
		byKey[rows[i].key] = &rows[i]
	}

	children := map[string][]string{}
	var roots []string
	for _, r := range rows {
		parent := ""
		if len(r.dependsOn) > 0 {
			if _, ok := byKey[r.dependsOn[0]]; ok {
				parent = r.dependsOn[0]
			}
		}
		if parent == "" {
			roots = append(roots, r.key)
		} else {
			children[parent] = append(children[parent], r.key)
		}
	}

	siblingOrder := func(keys []string) []string {
		sub := make([]Row, len(keys))
		for i, k := range keys {
			sub[i] = *byKey[k]
		}
		sortRowsForDisplay(sub)
		ordered := make([]string, len(sub))
		for i, r := range sub {
			ordered[i] = r.key
		}
		return ordered
	}

	var out []Row
	seen := map[string]bool{}
	var walk func(key string)
	walk = func(key string) {
		if seen[key] {
			return // dependency cycle guard — shouldn't happen, never hang on it
		}
		seen[key] = true
		out = append(out, *byKey[key])
		for _, c := range siblingOrder(children[key]) {
			walk(c)
		}
	}
	for _, k := range siblingOrder(roots) {
		walk(k)
	}
	return out
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
// this machine belongs to the timeline being asked for. projectDir is this
// plan's own project (BuildRowsFromFilesystem's homeDir) — coordinator
// events (AgentID "") are additionally required to have run from there,
// since plan-name codenames are drawn from a small shared pool and DO get
// reused across unrelated projects; an event with no CWD recorded (written
// before that field existed) is untrusted rather than guessed, and
// excluded. Team-member events keep their own identity via AgentID/label
// and skip this check — a team can legitimately work in an unrelated
// directory. planStarted (this plan's own declared Started:, zero if
// unknown) additionally excludes any event older than that — the same
// codename pool gets reused within a single project over time too (a
// finished plan's name picked up again months later for unrelated work),
// and CWD alone can't tell those two runs apart since they share a project.
func BuildRowsFromHookEvents(events []Event, labels map[string]string, plan, projectDir string, planStarted time.Time) []Row {
	// Pair pre/post by tool_use_id FIRST, globally, before any plan
	// filtering — then classify the resulting span using ONLY the pre
	// event's own Plan/Team/CWD, never the post's. PreToolUse reliably
	// gets cwd from Claude Code, so resolvePlanTeam reliably tags it; a
	// PostToolUse hook payload often doesn't carry cwd at all (confirmed
	// live: 26 of 274 posts for one plan's coordinator alone came back
	// with no plan tag), so filtering pre AND post independently by plan
	// before pairing silently dropped real completed calls whose post
	// lost its tag — on the dashboard they looked permanently open,
	// which is a false "still stuck" signal, not a real one.
	preByTID := map[string]Event{}
	for _, e := range events {
		if e.Hook == "pre" {
			preByTID[e.ToolUseID] = e
		}
	}

	byKey := map[string][]span{}
	for _, e := range events {
		if e.Hook != "post" {
			continue
		}
		p, ok := preByTID[e.ToolUseID]
		if !ok {
			continue // no matching pre recorded at all — nothing to pair
		}
		if p.Plan != plan {
			continue
		}
		if p.AgentID == "" && !samePath(p.CWD, projectDir) {
			continue
		}
		if !planStarted.IsZero() && p.TS.Before(planStarted) {
			continue
		}
		byKey[p.Team] = append(byKey[p.Team], span{start: p.TS, end: e.TS, tool: e.ToolName, summary: e.Summary})
	}

	var rows []Row
	for key, spans := range byKey {
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

// samePath reports whether a and b resolve to the same directory. Either
// side being empty (an event with no recorded CWD, or no project given) is
// never a match — an unknown location is not the same as "anywhere."
func samePath(a, b string) bool {
	if a == "" || b == "" {
		return false
	}
	aa, errA := filepath.Abs(a)
	bb, errB := filepath.Abs(b)
	if errA != nil || errB != nil {
		return false
	}
	return filepath.Clean(aa) == filepath.Clean(bb)
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
	sortRowsForDisplay(rows)
	return rows
}

// (?m) is required — the terminal line is essentially never the first
// line of the file (it's written last, per protocol), and without
// multiline mode ^ only anchors to the very start of the whole string, so
// this silently matched nothing on any real log.
var teamTerminalLine = regexp.MustCompile(`(?m)^TEAM (DONE|BLOCKED):`)

const validationMarker = "##ITERATE-VALIDATION##"

type validationMark struct {
	status string
	note   string
}

// readValidationMarkers scans one team's log file for
// ##ITERATE-VALIDATION## {"step":N,"status":"...","note":"..."} lines — the
// team's own real-time per-validation report, per /iterate's SKILL.md.
// Later markers for the same step number win (a team correcting or
// updating an earlier assessment), consistent with everything else here
// treating the log as an append-only stream of ground truth. Malformed
// lines and a missing file are silently skipped, not an error — a team
// that hasn't adopted the convention yet just shows no per-validation
// status, same as before this existed.
func readValidationMarkers(logPath string) map[int]validationMark {
	data, err := os.ReadFile(logPath)
	if err != nil {
		return nil
	}
	marks := map[int]validationMark{}
	for line := range strings.SplitSeq(string(data), "\n") {
		_, after, found := strings.Cut(line, validationMarker)
		if !found {
			continue
		}
		var m struct {
			Step   int    `json:"step"`
			Status string `json:"status"`
			Note   string `json:"note"`
		}
		if json.Unmarshal([]byte(strings.TrimSpace(after)), &m) != nil {
			continue
		}
		if m.Status != "met" && m.Status != "partial" && m.Status != "not-met" {
			continue
		}
		marks[m.Step] = validationMark{status: m.Status, note: m.Note}
	}
	return marks
}

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
	planFilePath := filepath.Join(homeDir, ".claude", "iterate", "plans", plan+".md")
	teamsDir := filepath.Join(homeDir, ".claude", "iterate", "plans", plan+".teams")
	return buildRowsFrom(planFilePath, teamsDir, plan, homeDir, scanDirs)
}

// BuildRowsFromArchive is BuildRowsFromFilesystem's counterpart for a run
// /iterate has already finished and moved to .claude/iterate/archive/ —
// same two data sources (team logs + registry entries), just pointed at
// the archived file and its sibling "<archiveFile-without-.md>.teams/"
// dir instead of the live plans/ directory. Confirmed live as a real gap:
// without this, a plan's OWN page kept resolving after archiving (nothing
// 404s), but with no plan file to read, teams/steps/validations all come
// back empty — no Goal, no Requirements burndown, no team grouping — while
// registry entries and hook events for that plan name (both permanent,
// global logs, unaffected by archiving) still get pulled in unbounded,
// rendering as one large ungrouped dump instead of the plan's real
// structure. archiveFile is the exact archived filename (see
// PlanSummary.ArchiveFile); planName is that file's own declared "name:"
// (PlanSummary.Name) — needed separately because registry entries and hook
// events are keyed by the plan's real name, never by its archive filename.
func BuildRowsFromArchive(archiveFile, planName, homeDir string, scanDirs []string) ([]Row, error) {
	planFilePath := filepath.Join(homeDir, ".claude", "iterate", "archive", archiveFile)
	teamsDir := filepath.Join(homeDir, ".claude", "iterate", "archive", strings.TrimSuffix(archiveFile, ".md")+".teams")
	return buildRowsFrom(planFilePath, teamsDir, planName, homeDir, scanDirs)
}

func buildRowsFrom(planFilePath, teamsDir, plan, homeDir string, scanDirs []string) ([]Row, error) {
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

	// Read the plan file once, up front — besides Teams/Steps/Validation
	// detail, this is also where this plan instance's own declared
	// Started: lives, needed below to keep a registry entry left over from
	// an earlier, unrelated run that reused this same codename from
	// silently merging into this run's picture.
	teams, steps, validations, planStarted := readPlanTeamsAndSteps(planFilePath)

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
			if !planStarted.IsZero() && e.Started.Before(planStarted) {
				continue // a stale registry entry from an earlier, unrelated run that reused this same plan codename
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
	// plan's own Teams table (already read above), and pick up any team
	// that hasn't started at all yet — no log file, no registry entry, so
	// byTeam wouldn't otherwise know it exists. A queued team being
	// invisible is exactly the wrong failure mode for a view meant to show
	// the whole picture, not just the parts that happened.
	depths := teamDepths(teams)
	stepOwner := map[int]string{} // step number -> the Teams-table row that owns it
	for team, meta := range teams {
		r := get(team)
		r.status = meta.status
		r.depth = depths[team]
		r.dependsOn = meta.dependsOn
		marks := readValidationMarkers(filepath.Join(teamsDir, team+".log.md"))
		for _, n := range meta.stepNums {
			stepOwner[n] = team
			sd := StepDetail{Num: n, Step: steps[n], Validation: validations[n]}
			if sd.Step == "" && sd.Validation == "" {
				continue // step number in the table but not found in either list — skip rather than show a blank pair
			}
			if m, ok := marks[n]; ok {
				sd.VStatus, sd.VNote = m.status, m.note
			}
			r.steps = append(r.steps, sd)
		}
		sort.Slice(r.steps, func(i, j int) bool { return r.steps[i].Num < r.steps[j].Num })
	}

	// A team-log or registry name that ISN'T a literal Teams-table row is
	// either a flat (unteamed) plan's only row — teams is empty there,
	// nothing to match against — or, on a teamed plan, a split-off
	// sub-dispatch: a team can and does fan out into more than one
	// concurrently-running agent (confirmed live: "gui" owns steps 25-32,
	// but the actual work ran as three separate dispatches — app-macos,
	// gui-macos2, gui-windows — each scoped to its own log file so they
	// never race on a shared one). Nothing records that split anywhere
	// (no naming convention for it exists yet), so the only reliable,
	// non-guessing signal is which step numbers the orphan's own
	// ##ITERATE-VALIDATION## markers report against — those are real
	// data, not a name-similarity heuristic, and a step number belongs to
	// exactly one Teams-table row. A match nests it under that row exactly
	// like a real dependency (this package's only use of dependsOn is
	// display nesting, so borrowing the same field is safe); it also
	// picks up a real status from its OWN terminal line instead of
	// defaulting to "running" just because it has recorded activity.
	for team, r := range byTeam {
		if _, isTeamsRow := teams[team]; isTeamsRow || team == "" {
			continue
		}
		lf := filepath.Join(teamsDir, team+".log.md")
		for step := range readValidationMarkers(lf) {
			if owner, ok := stepOwner[step]; ok {
				r.dependsOn = []string{owner}
				r.depth = depths[owner] + 1 // teamDepths never saw this row (it's not in teams), so its depth has to be set here too, not just dependsOn
				break
			}
		}
		if data, err := os.ReadFile(lf); err == nil {
			switch teamTerminalLine.FindString(string(data)) {
			case "TEAM DONE:":
				r.status = "done"
			case "TEAM BLOCKED:":
				r.status = "blocked (see log)"
			}
		}
	}

	var rows []Row
	for _, r := range byTeam {
		sort.Slice(r.spans, func(i, j int) bool { return r.spans[i].start.Before(r.spans[j].start) })
		r.gaps = computeGaps(r.spans)
		rows = append(rows, *r)
	}
	sortRowsForDisplay(rows)
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
// step-number list from the Teams table, the Steps and Validation sections
// themselves keyed by number — the "Na./Nb." pairing shown in chat, now
// available for the dashboard to show the same way on click — this plan's
// own declared Started: time (zero if missing or unparseable), and the raw
// text of the Status / Log section (used to recover the coordinator's own
// inline "step N DONE:" progress reports on a flat/unteamed plan — see
// parseCoordinatorStepStatus). planFilePath is the exact file to read —
// the live plans/<name>.md path for a running plan, or an
// archive/<timestamp>-<name>-done.md path for a finished one (see
// BuildRowsFromArchive); this function doesn't care which. Returns nils
// (not an error) if the file can't be read.
func readPlanTeamsAndSteps(planFilePath string) (teams map[string]teamMeta, steps, validations map[int]string, started time.Time, statusLog string) {
	data, err := os.ReadFile(planFilePath)
	if err != nil {
		return nil, nil, nil, time.Time{}, ""
	}
	teams = map[string]teamMeta{}
	steps = map[int]string{}
	validations = map[int]string{}
	var statusLines []string
	section := ""
	for line := range strings.SplitSeq(string(data), "\n") {
		if m := reStarted.FindStringSubmatch(line); m != nil {
			if t, ok := parsePlanStarted(m[1]); ok {
				started = t
			}
		}
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "## ") {
			switch trimmed {
			case "## Steps":
				section = "steps"
			case "## Validation":
				section = "validation"
			case "## Teams":
				section = "teams"
			case "## Status / Log":
				section = "status"
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
		case "status":
			statusLines = append(statusLines, line)
		}
	}
	return teams, steps, validations, started, strings.Join(statusLines, "\n")
}

// reCoordStepStatus matches the coordinator's own inline per-step self
// report in a flat plan's Status/Log — "step 5 PARTIAL: ...", "steps 2-3
// DONE: ...", case-insensitive since real logs mix "DONE" and "partial"
// in the same file. This is a free-text convention, not a structured
// marker, because there's no per-team log file for the coordinator's own
// unassigned steps to write one into (##ITERATE-VALIDATION## is scoped to
// dispatched teams, per /iterate's own SKILL.md) — confirmed live as the
// actual, only signal a real flat-plan coordinator produces.
var reCoordStepStatus = regexp.MustCompile(`(?i)\bsteps?\s+(\d+)(?:-(\d+))?\s+(done|partial|blocked|not-met)\b`)

// parseCoordinatorStepStatus turns those inline reports into the same
// validationMark shape readValidationMarkers produces for a team log, so
// collectSteps' done/active/queued shading works identically regardless
// of source. A range ("steps 2-3 DONE") applies to every step in it.
// Later matches win for a given step number — Status/Log is append-only
// and chronological, so a later line is a correction/update, same rule
// readValidationMarkers already follows.
func parseCoordinatorStepStatus(statusLog string) map[int]validationMark {
	marks := map[int]validationMark{}
	for _, m := range reCoordStepStatus.FindAllStringSubmatch(statusLog, -1) {
		start, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}
		end := start
		if m[2] != "" {
			if e, err := strconv.Atoi(m[2]); err == nil {
				end = e
			}
		}
		status := strings.ToLower(m[3])
		if status == "blocked" {
			status = "not-met"
		}
		for n := start; n <= end; n++ {
			marks[n] = validationMark{status: status}
		}
	}
	return marks
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
	eyebrow := `<div class="eyebrow">iterate plan</div>`
	if plan.Archived {
		eyebrow = `<div class="eyebrow">iterate plan <span class="archived-chip">archived</span></div>`
	}
	header := backLink + eyebrow + `<h1>` + html.EscapeString(planName) + `</h1>`
	if plan.Blocked() && plan.NextAttempt != "" {
		// The most urgent thing on the page, so it goes first — a plan can
		// have every team done and still be sitting here waiting on one
		// thing only a human can do (confirmed live: GitHub Actions billing
		// on the account, which no agent may touch).
		header += `<div class="blocked-banner"><div class="blocked-label">Needs you</div><p>` + html.EscapeString(plan.NextAttempt) + `</p></div>`
	}
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

	var done, running, queued, severeGaps int
	var gapCallouts []struct {
		label string
		g     gap
	}
	for _, r := range rows {
		// Reuse statusPill's own classification instead of a second,
		// separately-maintained switch — the old duplicate had only three
		// cases and silently counted a row toward NONE of done/running/
		// queued whenever status=="" and it had activity, which is exactly
		// the coordinator's own permanent state (it never gets a status
		// field at all) plus any orphan dispatch before its terminal line
		// arrives. That made a plan with real work still in flight read as
		// fully accounted-for at the top even while its own coordinator
		// section showed hours of live activity.
		switch pillClass, _ := statusPill(r.status, len(r.spans) > 0); pillClass {
		case "p-done", "p-blocked":
			done++
		case "p-running":
			running++
		case "p-queued":
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
	fmt.Fprintf(&b, `<div class="sub">%s &rarr; %s (%s of recorded activity)</div>`+"\n",
		minT.Local().Format("2006-01-02 15:04:05"), maxT.Local().Format("15:04:05"), total.Round(time.Second))

	// A separate, explicitly-labeled "running for" figure — deliberately
	// NOT the same number as the activity span above. That span is the
	// earliest-to-latest timestamp across every row's recorded spans,
	// which can include a genuinely long-running plan's real early
	// history; conflating the two read as "the coordinator has been
	// silently stuck for days" when what actually happened was a plan
	// that's been going for a while. So this box anchors to an explicit
	// "execution began" instant instead of the activity span — but that
	// instant is Executing:, NOT Started:. Started: is drafting time (set
	// once by /iterate-planner and, per its own doc, "kept forever, never
	// reset on refinement") — for any plan that went through the planner
	// before execution actually began, Started: can predate real work by
	// hours (confirmed live: a plan drafted at 13:59, then not actually
	// run until 16:32, showed "running for 2h36m" the instant /iterate
	// was invoked — that's elapsed planning time, not elapsed execution
	// time). Executing: is set once, by /iterate itself, at the moment
	// phase flips from planned to executing, and is never touched again —
	// exactly the "timestamp the invocation" anchor this box needs. Plans
	// written before that field existed fall back to the earliest
	// recorded activity (minT) — real tool-call data already on disk,
	// still far closer to true execution start than a stale planning
	// timestamp — and only fall all the way back to Started: when there's
	// no activity at all yet to measure from.
	started, ok := plan.ExecutingAt()
	if !ok && !minT.IsZero() {
		started, ok = minT, true
	}
	if !ok {
		started, ok = plan.StartedAt()
	}
	if ok {
		// "Running for" implies still in flight — actively misleading for
		// an archived plan (exactly the class of bug this whole box exists
		// to fix elsewhere: language that reads as live when the thing it
		// describes is over). "Ran for" for a finished run, same figure.
		label := "Running for"
		if plan.Archived {
			label = "Ran for"
		}
		runDur := max(maxT.Sub(started), 0)
		fmt.Fprintf(&b, `<div class="runbox"><span class="runbox-label">%s</span><span class="runbox-dur">%s</span><span class="runbox-since">since %s</span></div>`+"\n",
			label, runDur.Round(time.Second), started.Local().Format("2006-01-02 15:04:05"))
	}

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
		b.WriteString(`<h2>Coordinator</h2><div class="gantt">`)
		writeGanttRow(&b, *coordRow, pct, false)
		b.WriteString(`</div>`)
	}

	writeBurndownChart(&b, rows)

	// The divider line goes ONLY between separate root-level groups, not
	// between a parent and its own nested children — a dependency chain
	// (e.g. cli-frame -> selection -> reports) is one visual unit now that
	// orderTeamRows nests it together, so a line splitting it back apart
	// would undo the point of the nesting.
	b.WriteString(`<h2>Activity by team</h2><div class="gantt">`)
	firstRoot := true
	for _, r := range teamRows {
		divider := false
		if r.depth == 0 {
			divider = !firstRoot
			firstRoot = false
		}
		writeGanttRow(&b, r, pct, divider)
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
// "Na./Nb." panel underneath (native <details>, no JS). divider draws the
// light top rule the caller uses to separate one root-level group (a
// dependency chain, nested together) from the next — never set within a
// group, only between them; the coordinator's own single-row block always
// passes false, having no siblings to separate from.
// currentWorkingStepNum returns the step number a row is presumed to be
// actively working on right now, or 0 if none. The mark is inferred, not
// self-reported — teams only file a ##ITERATE-VALIDATION## marker once a
// step is actually resolved (met/partial/not-met), so there's no "I just
// started this" signal to read directly. The best available proxy: for a
// row whose overall status is in-progress, the first step in Num order
// that hasn't reported anything yet is the one it's presumably on right
// now. Everything after that is just queued behind it, not "in progress."
// Shared by writeGanttRow's per-team panel and the burndown chart so both
// agree on which cell is "active" instead of drifting independently.
func currentWorkingStepNum(r Row) int {
	if r.status != "in-progress" {
		return 0
	}
	for _, sd := range r.steps {
		if sd.VStatus == "" {
			return sd.Num
		}
	}
	return 0
}

// burnStep is one numbered requirement's flattened view for the burndown
// chart — a step number can only belong to one team (per Teams-table row),
// so unlike Row.steps (grouped per-team), this is one entry per requirement
// across the WHOLE plan, sorted by Num, letting the chart answer "where do
// we stand on requirement N" without knowing which team owns it first.
type burnStep struct {
	Num        int
	Step       string
	Validation string
	Team       string
	DependsOn  []string // this step's owning team's OWN full Depends-on list from the Teams table, resolved to display labels — every team that must finish first, not just one. A team can list several (tech-debt genuinely depends on all seven of the others in a real plan); showing only a single "primary" parent (the shortcut orderTeamRows takes purely for indentation) would silently hide the rest.
	State      string   // "done" (validation reported met), "active" (currently being worked, or reported partial/not-met), "queued" (white — no signal yet)
}

// collectSteps flattens every row's Steps/Validation detail into one
// Num-ordered list spanning the whole plan, each carrying its owning row's
// label and full Depends-on list — the data the burndown chart and its
// click-through detail panel are built from. Rows with no steps (flat/
// unteamed plans have none at all — see BuildRowsFromFilesystem) contribute
// nothing, same limitation as the existing per-team panel.
func collectSteps(rows []Row) []burnStep {
	labelOf := map[string]string{}
	for _, r := range rows {
		labelOf[r.key] = r.label
	}
	resolveDeps := func(keys []string) []string {
		labels := make([]string, len(keys))
		for i, k := range keys {
			if l, ok := labelOf[k]; ok {
				labels[i] = l
			} else {
				labels[i] = k
			}
		}
		return labels
	}

	var out []burnStep
	for _, r := range rows {
		working := currentWorkingStepNum(r)
		deps := resolveDeps(r.dependsOn)
		for _, sd := range r.steps {
			state := "queued"
			switch {
			case sd.VStatus == "met":
				state = "done"
			case sd.VStatus == "partial", sd.VStatus == "not-met":
				state = "active" // reported, but not fully resolved — needs more work, not untouched
			case sd.Num == working:
				state = "active"
			}
			out = append(out, burnStep{Num: sd.Num, Step: sd.Step, Validation: sd.Validation, Team: r.label, DependsOn: deps, State: state})
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Num < out[j].Num })
	return out
}

// writeBurndownChart renders one numbered cell per requirement across the
// whole plan — white for untouched, yellow for active, green for done —
// between the Coordinator section and the per-team Activity breakdown.
// Clicking a cell (vanilla JS, no round trip — the detail data is embedded
// as JSON right below the grid) opens a panel showing that requirement's
// owning team, everything that team depends on, and its Step/Validation
// text: "what accomplishing this task means and what it depends on."
// Silently renders nothing for a plan with no per-team step detail (flat/
// unteamed plans — see collectSteps).
func writeBurndownChart(b *strings.Builder, rows []Row) {
	steps := collectSteps(rows)
	if len(steps) == 0 {
		return
	}

	b.WriteString(`<h2>Requirements</h2><div class="burndown">`)
	for _, s := range steps {
		cls := "bd-queued"
		switch s.State {
		case "done":
			cls = "bd-done"
		case "active":
			cls = "bd-active"
		}
		fmt.Fprintf(b, `<div class="bd-cell %s" data-num="%d" onclick="bdShow(%d)" title="%d &middot; %s">%d</div>`,
			cls, s.Num, s.Num, s.Num, html.EscapeString(s.Team), s.Num)
	}
	b.WriteString(`</div><div id="bd-detail" class="bd-detail" style="display:none"></div>`)

	type burnStepJSON struct {
		Step       string   `json:"step"`
		Validation string   `json:"validation"`
		Team       string   `json:"team"`
		DependsOn  []string `json:"dependsOn"`
		State      string   `json:"state"`
	}
	data := make(map[string]burnStepJSON, len(steps))
	for _, s := range steps {
		data[strconv.Itoa(s.Num)] = burnStepJSON{Step: s.Step, Validation: s.Validation, Team: s.Team, DependsOn: s.DependsOn, State: s.State}
	}
	// json.Marshal HTML-escapes '<', '>' and '&' by default — exactly what
	// keeps a plan's own step/validation text (arbitrary markdown, could
	// itself contain "</script>") from breaking out of this script block.
	encoded, err := json.Marshal(data)
	if err != nil {
		return
	}
	b.WriteString(`<script>var bdData=` + string(encoded) + `;
function bdEsc(s){var d=document.createElement('div');d.textContent=s==null?'':s;return d.innerHTML;}
function bdShow(num){
  var d=bdData[num];
  var el=document.getElementById('bd-detail');
  if(!d){el.style.display='none';return;}
  var stateLabel={done:'done',active:'active',queued:'not yet worked on'}[d.state]||d.state;
  var html='<div class="bd-detail-num">Requirement '+num+' &middot; '+bdEsc(d.team)+' &middot; '+bdEsc(stateLabel)+'</div>';
  html+='<div class="bd-detail-chain">depends on: '+bdEsc(d.dependsOn.length?d.dependsOn.join(', '):'none')+'</div>';
  if(d.step){html+='<div class="bd-detail-row"><span class="bd-detail-label">'+num+'a.</span><span>'+bdEsc(d.step)+'</span></div>';}
  if(d.validation){html+='<div class="bd-detail-row"><span class="bd-detail-label">'+num+'b.</span><span>'+bdEsc(d.validation)+'</span></div>';}
  el.innerHTML=html;
  el.style.display='block';
  document.querySelectorAll('.bd-cell').forEach(function(c){c.classList.toggle('bd-selected',c.dataset.num===String(num));});
}
</script>`)
}

func writeGanttRow(b *strings.Builder, r Row, pct func(time.Time) float64, divider bool) {
	pillClass, pillLabel := statusPill(r.status, len(r.spans) > 0)
	var busy time.Duration
	for _, s := range r.spans {
		busy += s.end.Sub(s.start)
	}

	hasSteps := len(r.steps) > 0
	dividerClass := ""
	if divider {
		dividerClass = " row-divider"
	}
	rowOpen, rowClose := `<div class="row`+dividerClass+`">`, `</div>`
	teamLabel := html.EscapeString(r.label)
	if hasSteps {
		rowOpen, rowClose = `<details class="row-d`+dividerClass+`"><summary class="row">`, `</summary>`
		teamLabel += `<span class="chev">&rsaquo;</span>`
	}

	pillStyle := ""
	if r.depth > 0 {
		pillStyle = fmt.Sprintf(` style="margin-left:%dpx"`, r.depth*14)
	}

	fmt.Fprintf(b, `%s<div class="label-col"><span class="pill %s" title="%s"%s></span><span class="team">%s</span></div><div class="track">`,
		rowOpen, pillClass, html.EscapeString(pillLabel), pillStyle, teamLabel)
	for i, s := range r.spans {
		left := pct(s.start)
		width := pct(s.end) - left
		if width < 0.15 {
			width = 0.15 // keep even instant calls visible
		}
		cls := "bar"
		// "still running" belongs to the LAST span of THIS row, not to
		// whichever single span across the ENTIRE plan happens to share
		// the plan-wide maxT timestamp — that old check only ever matched
		// one row (whoever's tool call finished most recently), so every
		// other genuinely in-progress team's bar just stopped short and
		// looked done even while its own status was still "in-progress"
		// (confirmed live: prd sitting on step 16 with no bar-open stripe
		// while audio-filters, whose last call happened to land on maxT,
		// was the only row ever marked still-running).
		if i == len(r.spans)-1 && r.status == "in-progress" {
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
		working := currentWorkingStepNum(r)

		b.WriteString(`<div class="steps-detail">`)
		for _, sd := range r.steps {
			// Each step's Na/Nb pair is its own group, set off from the
			// next step's pair by a light divider — the visual grouping
			// IS the "12a and 12b belong together, and 12 as a whole is
			// done" signal, so it has to read as one unit at a glance
			// instead of just more lines in a flat stack. The status
			// glyph sits in its own column at the left of the WHOLE
			// group, vertically centered against both lines — not
			// trailing after the Nb text, where it read as punctuation
			// rather than a status mark.
			glyph := validationGlyph(sd.VStatus, sd.VNote)
			if glyph == "" && sd.Num == working {
				glyph = `<span class="vmark v-working" title="in progress">&#9679;</span>`
			}
			fmt.Fprintf(b, `<div class="step-group"><div class="step-glyph">%s</div><div class="step-lines">`, glyph)
			if sd.Step != "" {
				fmt.Fprintf(b, `<div class="step-pair"><span class="stepnum">%da.</span><span class="step-text">%s</span></div>`, sd.Num, html.EscapeString(sd.Step))
			}
			if sd.Validation != "" {
				fmt.Fprintf(b, `<div class="step-pair val"><span class="stepnum">%db.</span><span class="step-text">%s</span></div>`,
					sd.Num, html.EscapeString(sd.Validation))
			}
			b.WriteString(`</div></div>`)
		}
		b.WriteString(`</div></details>` + "\n")
	}
}

// validationGlyph renders the team's own ##ITERATE-VALIDATION## report for
// this Nb, if it wrote one — nothing at all when it hasn't, since "not yet
// reported" and "reported not-met" are different facts and only one of
// them is worth a red mark.
func validationGlyph(status, note string) string {
	var cls, symbol, label string
	switch status {
	case "met":
		cls, symbol, label = "v-met", "&#10003;", "met"
	case "partial":
		cls, symbol, label = "v-partial", "&#8776;", "partial"
	case "not-met":
		cls, symbol, label = "v-not-met", "&#10007;", "not met"
	default:
		return ""
	}
	title := label
	if note != "" {
		title += ": " + note
	}
	return fmt.Sprintf(`<span class="vmark %s" title="%s">%s</span>`, cls, html.EscapeString(title), symbol)
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
.archived-chip{display:inline-block;margin-left:6px;padding:1px 6px;border-radius:4px;font-size:10px;font-weight:600;letter-spacing:.02em;background:var(--surface-2);color:var(--text-faint);border:1px solid var(--border)}
h1{font-size:24px;font-weight:700;margin:2px 0 8px;text-wrap:balance;letter-spacing:-.01em}
h2{font-size:13px;text-transform:uppercase;letter-spacing:.06em;color:var(--text-dim);font-weight:600;margin:26px 0 12px}
.sub{color:var(--text-dim);font-size:12px;margin-bottom:10px}
.runbox{display:flex;align-items:baseline;gap:8px;margin:0 0 18px;padding:10px 14px;background:var(--surface);border:1px solid var(--border);border-left:3px solid var(--accent);border-radius:8px}
.runbox-label{font-size:10.5px;letter-spacing:.06em;text-transform:uppercase;color:var(--text-dim);font-weight:600}
.runbox-dur{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-variant-numeric:tabular-nums;font-size:16px;font-weight:700;color:var(--accent)}
.runbox-since{font-size:11.5px;color:var(--text-faint)}
.back{display:inline-block;font-size:12.5px;color:var(--accent);text-decoration:none;margin-bottom:10px}
.back:hover{text-decoration:underline}
.goal{margin:0 0 18px;padding:14px 16px;background:var(--accent-bg);border:1px solid var(--border);border-radius:8px}
.goal-label{font-size:10px;letter-spacing:.08em;text-transform:uppercase;color:var(--accent);font-weight:700;margin-bottom:5px}
.goal p{color:var(--text-dim);font-size:13.5px;margin:0}
.blocked-banner{margin:0 0 18px;padding:14px 16px;background:var(--danger-b);border:1px solid var(--danger);border-radius:8px}
.blocked-label{font-size:10px;letter-spacing:.08em;text-transform:uppercase;color:var(--danger);font-weight:700;margin-bottom:5px}
.blocked-banner p{color:var(--text);font-size:13.5px;margin:0}
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
.row-divider,.row-divider>.row{border-top:1px solid var(--border)}
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
.steps-detail{padding:6px 14px 14px 46px;background:var(--surface-2);display:flex;flex-direction:column}
.step-group{display:flex;align-items:center;gap:10px;padding:8px 0}
.step-group:first-child{padding-top:0}
.step-group:last-child{padding-bottom:0}
.step-group+.step-group{border-top:1px solid var(--border)}
.step-glyph{flex:0 0 18px;display:flex;justify-content:center;align-items:center;font-size:13px}
.step-lines{flex:1 1 auto;min-width:0;display:flex;flex-direction:column;gap:6px}
.step-pair{display:flex;align-items:flex-start;gap:8px;font-size:12.5px;color:var(--text)}
.step-pair.val{color:var(--text-dim)}
.step-pair .stepnum{flex:0 0 auto;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;color:var(--accent);font-weight:600;min-width:28px}
.step-pair .step-text{flex:1 1 auto}
.vmark{display:inline-block;font-weight:700}
.vmark.v-met{color:var(--good)}
.vmark.v-partial{color:var(--warn)}
.vmark.v-not-met{color:var(--danger)}
.vmark.v-working{color:var(--warn);animation:vworking 1.4s ease-in-out infinite}
@keyframes vworking{0%,100%{opacity:1}50%{opacity:.3}}
.burndown{display:flex;flex-wrap:wrap;gap:6px;margin-bottom:10px}
.bd-cell{width:32px;height:32px;display:flex;align-items:center;justify-content:center;border-radius:6px;border:1px solid var(--border);font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:12px;font-weight:700;cursor:pointer;color:var(--text-dim);background:var(--surface);transition:transform .1s}
.bd-cell:hover{transform:translateY(-1px);border-color:var(--accent)}
.bd-cell.bd-queued{background:var(--surface);color:var(--text-faint)}
.bd-cell.bd-active{background:var(--warn-bg);color:var(--warn);border-color:var(--warn)}
.bd-cell.bd-done{background:var(--good-bg);color:var(--good);border-color:var(--good)}
.bd-cell.bd-selected{box-shadow:0 0 0 2px var(--accent)}
.bd-detail{margin:0 0 22px;padding:14px 16px;background:var(--surface);border:1px solid var(--border);border-left:3px solid var(--accent);border-radius:8px}
.bd-detail-num{font-size:11px;text-transform:uppercase;letter-spacing:.06em;color:var(--accent);font-weight:700;margin-bottom:4px}
.bd-detail-chain{font-size:12px;color:var(--text-dim);margin-bottom:10px;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
.bd-detail-row{display:flex;align-items:flex-start;gap:8px;font-size:13px;margin-bottom:6px;color:var(--text)}
.bd-detail-row .bd-detail-label{flex:0 0 auto;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;color:var(--accent);font-weight:600;min-width:28px}
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
