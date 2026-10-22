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

// teamLogTool marks the one span kind that is a bounding box rather than
// work: a team's log file existed from its birth to its last write, which
// says nothing about whether the agent was running in between.
const teamLogTool = "team-log"

// dropCoarseSpans removes the log-file bounding box from a row that has real
// per-call data, and leaves it alone on a row that has nothing else.
//
// Keeping both is what let a team claim more busy time than its span is long
// and, worse, hid real outages: ledger's log file spanned the six hours its
// model was exhausted, so counting that file's existence as work erased the
// gap the coordinator correctly reported for the same window. A coarse span
// is the fallback for a team with no instrumentation, never a supplement to
// a team that has it.
func dropCoarseSpans(spans []span) []span {
	fine := false
	for _, s := range spans {
		if s.tool != teamLogTool {
			fine = true
			break
		}
	}
	if !fine {
		return spans
	}
	out := spans[:0:0]
	for _, s := range spans {
		if s.tool != teamLogTool {
			out = append(out, s)
		}
	}
	return out
}

// mergeSpans unions overlapping spans from an already start-sorted list.
// A row routinely holds spans at two resolutions — one coarse span covering
// the whole life of a team's log file, plus every fine-grained tool call
// inside it — and treating those as separate work double-counts the same
// minutes. Confirmed live: reporting on plan galago reported "1h48m41s
// busy" inside a 93-minute span, which cannot be true of one agent.
func mergeSpans(spans []span) []span {
	if len(spans) == 0 {
		return nil
	}
	out := []span{spans[0]}
	for _, s := range spans[1:] {
		last := &out[len(out)-1]
		if !s.start.After(last.end) {
			if s.end.After(last.end) {
				last.end = s.end
			}
			continue
		}
		out = append(out, s)
	}
	return out
}

// busyDuration is the wall-clock time a row was actually working: the length
// of the union of its spans, never the sum of their lengths.
func busyDuration(spans []span) time.Duration {
	var d time.Duration
	for _, s := range mergeSpans(spans) {
		d += s.end.Sub(s.start)
	}
	return d
}

// computeGaps finds the idle stretches that clear the NotableGap threshold —
// shared by every row source, hook-derived or filesystem-derived alike.
//
// Gaps are the holes in the UNION of the spans, not the distance between
// consecutive ones: a row carrying both a coarse log-file span and the tool
// calls inside it would otherwise report idle time that a span it already
// holds proves was busy.
func computeGaps(spans []span) []gap {
	merged := mergeSpans(spans)
	var gaps []gap
	for i := 1; i < len(merged); i++ {
		g := gap{start: merged[i-1].end, end: merged[i].start}
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
		// Recover the team for events already on disk. Every event written
		// before resolvePlanTeam was fixed carries agent_id but no team —
		// the resolver matched cwd first and returned team "" for team
		// members, so a plan's whole history reads as coordinator work. The
		// id is self-describing, so the team is recoverable rather than
		// lost: 215,000 stored events keep their real owner instead of
		// needing the plan re-run to be readable.
		team := p.Team
		if team == "" && p.AgentID != "" {
			if _, t := parseAgentID(p.AgentID); t != "" {
				team = t
			} else {
				team = p.AgentID // its own lane; never the coordinator's
			}
		}
		byKey[team] = append(byKey[team], span{start: p.TS, end: e.TS, tool: e.ToolName, summary: e.Summary})
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
		r.spans = dropCoarseSpans(r.spans)
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
	teams, steps, validations, planStarted, statusLog, boxes := readPlanTeamsStepsAndBoxes(planFilePath)

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
		r.spans = append(r.spans, span{start: start, end: end, tool: teamLogTool, summary: summary})
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
			// The plan's own checkbox is the fallback when this team's log
			// has no ##ITERATE-VALIDATION## marker for the step — the
			// coordinator ticks a team's boxes when it merges that team's
			// log, so on a boxed plan this is often the only record.
			sd.VStatus = boxes[n].vstatus()
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

	// Any Steps/Validation pair no named team claimed belongs to the
	// coordinator's own "unassigned steps" (see /iterate's own SKILL.md:
	// "what the coordinator uses for the plan's unassigned steps"). The
	// common case is a flat plan with no ## Teams table at all — teams is
	// empty, so EVERY step lands here — but it also covers a genuinely
	// teamed plan that just never assigned every step to a team.
	// Confirmed live as a real gap: a plan drafted via /iterate-planner
	// but never teamified showed no Requirements burndown at all, despite
	// having real Steps/Validation detail and real per-step progress
	// already logged — the coordinator row simply never got a .steps
	// slice populated, since that only ever happened inside the
	// Teams-table loop above. Status comes from parseCoordinatorStepStatus
	// rather than readValidationMarkers — there's no per-team log file for
	// the coordinator's own steps to write a ##ITERATE-VALIDATION## marker
	// into, only its inline "step N DONE:" lines in the plan's own
	// Status/Log.
	var unassigned []int
	for n := range steps {
		if _, owned := stepOwner[n]; !owned {
			unassigned = append(unassigned, n)
		}
	}
	for n := range validations {
		if _, owned := stepOwner[n]; !owned {
			unassigned = append(unassigned, n)
		}
	}
	if len(unassigned) > 0 {
		sort.Ints(unassigned)
		coordMarks := parseCoordinatorStepStatus(statusLog)
		coord := get("")
		seen := map[int]bool{}
		for _, n := range unassigned {
			if seen[n] {
				continue // steps and validations can both name the same number
			}
			seen[n] = true
			sd := StepDetail{Num: n, Step: steps[n], Validation: validations[n]}
			if sd.Step == "" && sd.Validation == "" {
				continue
			}
			// The checkbox is the baseline; an explicit "step N DONE:"
			// line in the Status/Log wins, because it is more specific and
			// carries the note explaining a partial or a blocker.
			sd.VStatus = boxes[n].vstatus()
			if m, ok := coordMarks[n]; ok {
				sd.VStatus, sd.VNote = m.status, m.note
			}
			coord.steps = append(coord.steps, sd)
		}
		sort.Slice(coord.steps, func(i, j int) bool { return coord.steps[i].Num < coord.steps[j].Num })
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
		r.spans = dropCoarseSpans(r.spans)
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

// The iterate family writes plan items in three different shapes, and the
// dashboard has to read all of them because its own skills disagree:
// `/iterate-planner`'s template emits `1. <task>`, while `/iterate`'s
// emits `- [ ] 1. <step>` for Steps and `- [ ] check 1: <criterion>` for
// Validation. Measured across all 136 plan files on this machine: 3,193
// plain items, 422 boxed-numbered, 79 boxed `check N:`. Reading only the
// plain form left 31 plan files with no Requirements row at all — and 8
// MIXED files showing only *some* of their steps, which is worse, because
// a partial row looks correct.
var (
	rePlanItem      = regexp.MustCompile(`^(?:[-*]\s*\[([ xX~/-]?)\]\s*)?(\d+)[.)]\s+(.*)$`)
	rePlanCheckItem = regexp.MustCompile(`(?i)^(?:[-*]\s*\[([ xX~/-]?)\]\s*)?check\s+(\d+)\s*:\s*(.*)$`)
)

// planItem is one parsed `## Steps` / `## Validation` line.
type planItem struct {
	Num     int
	Text    string
	Boxed   bool // the line carried a checkbox at all
	Checked bool // ...and it was ticked
}

// parsePlanItem reads one line of a Steps or Validation section. Returns
// ok=false for anything that isn't a numbered item — prose, a bare
// unnumbered checkbox (`- [ ] CLEANUP: ...`), a blank line.
func parsePlanItem(line string) (planItem, bool) {
	for _, re := range []*regexp.Regexp{rePlanItem, rePlanCheckItem} {
		m := re.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		n, err := strconv.Atoi(m[2])
		if err != nil {
			continue
		}
		box := m[1]
		return planItem{
			Num:   n,
			Text:  m[3],
			Boxed: strings.Contains(line, "["),
			// Only a literal x counts as done. The other markers people
			// use in a box (~, /, -) mean "started" at best, and this
			// must never turn an unproven step green.
			Checked: box == "x" || box == "X",
		}, true
	}
	return planItem{}, false
}

// readPlanTeamsAndSteps reads the plan file once and returns everything
// the dashboard needs from it beyond raw activity: each team's status and
// step-number list from the Teams table, the Steps and Validation sections
// themselves keyed by number — the "Na./Nb." pairing shown in chat, now
// available for the dashboard to show the same way on click — this plan's
// own EffectiveStart (Executing: when present, Started: otherwise — zero
// if neither is parseable), and the raw text of the Status / Log section
// (used to recover the coordinator's own inline "step N DONE:" progress
// reports on a flat/unteamed plan — see parseCoordinatorStepStatus).
// started is the boundary buildRowsFrom uses to exclude a stale registry
// entry left over from an earlier, unrelated run that reused this same
// plan codename — it must be Executing:, not Started:, or genuine
// drafting-phase activity (tool calls made while planning, tagged with
// this plan's name because `current` already pointed at it) slips through
// as if it were part of THIS execution, showing up as a bogus "severe
// gap" between the drafting burst and real execution (confirmed live: a
// plan drafted at 17:11, not run until 17:24:51, showed exactly that).
// planFilePath is the exact file to read — the live plans/<name>.md path
// for a running plan, or an archive/<timestamp>-<name>-done.md path for a
// finished one (see BuildRowsFromArchive); this function doesn't care
// which. Returns nils (not an error) if the file can't be read.
func readPlanTeamsAndSteps(planFilePath string) (teams map[string]teamMeta, steps, validations map[int]string, started time.Time, statusLog string) {
	teams, steps, validations, started, statusLog, _ = readPlanTeamsStepsAndBoxes(planFilePath)
	return
}

// boxState records what a plan's own checkboxes say about one numbered
// requirement. Kept separate from the Step/Validation text because the two
// boxes routinely DISAGREE, and the disagreement is the interesting part:
// halibut has step 1 ticked and validation 1 unticked, which is exactly
// true — the step ran and named the sites behind login walls, and its
// validation ("the sites with gaps are signed in") is what the plan is
// blocked on. Collapsing them would report that step as proven.
type boxState struct {
	StepBoxed, StepChecked             bool
	ValidationBoxed, ValidationChecked bool
}

// vstatus maps a requirement's checkboxes onto the validation vocabulary
// the burndown already speaks. Nothing here can claim "met" without a
// ticked box, and a ticked STEP whose validation box is explicitly
// unticked is "partial" — attempted, not proven — never green.
func (b boxState) vstatus() string {
	switch {
	case b.ValidationChecked:
		return "met"
	case b.StepChecked && b.ValidationBoxed:
		return "partial"
	case b.StepChecked:
		return "met"
	}
	return ""
}

func readPlanTeamsStepsAndBoxes(planFilePath string) (teams map[string]teamMeta, steps, validations map[int]string, started time.Time, statusLog string, boxes map[int]boxState) {
	data, err := os.ReadFile(planFilePath)
	if err != nil {
		return nil, nil, nil, time.Time{}, "", nil
	}
	teams = map[string]teamMeta{}
	steps = map[int]string{}
	validations = map[int]string{}
	boxes = map[int]boxState{}
	var statusLines []string
	var executing time.Time
	section := ""
	for line := range strings.SplitSeq(string(data), "\n") {
		if m := reStarted.FindStringSubmatch(line); m != nil {
			if t, ok := parsePlanStarted(m[1]); ok {
				started = t
			}
		}
		if m := reExecuting.FindStringSubmatch(line); m != nil {
			if t, ok := parsePlanStarted(m[1]); ok {
				executing = t
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
			if it, ok := parsePlanItem(trimmed); ok {
				steps[it.Num] = it.Text
				bs := boxes[it.Num]
				bs.StepBoxed = bs.StepBoxed || it.Boxed
				bs.StepChecked = bs.StepChecked || it.Checked
				boxes[it.Num] = bs
			}
		case "validation":
			if it, ok := parsePlanItem(trimmed); ok {
				validations[it.Num] = it.Text
				bs := boxes[it.Num]
				bs.ValidationBoxed = bs.ValidationBoxed || it.Boxed
				bs.ValidationChecked = bs.ValidationChecked || it.Checked
				boxes[it.Num] = bs
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
	if !executing.IsZero() {
		started = executing
	}
	return teams, steps, validations, started, strings.Join(statusLines, "\n"), boxes
}

// reCoordStepStatus matches the coordinator's own inline per-step self
// report in a flat plan's Status/Log — "step 5 PARTIAL: Windows VM on
// bridge...", "steps 2-3 DONE: bridge up...", case-insensitive since real
// logs mix "DONE" and "partial" in the same file. This is a free-text
// convention, not a structured marker, because there's no per-team log
// file for the coordinator's own unassigned steps to write one into
// (##ITERATE-VALIDATION## is scoped to dispatched teams, per /iterate's
// own SKILL.md) — confirmed live as the actual, only signal a real
// flat-plan coordinator produces. Group 4 captures the rest of that same
// line (the actual explanation) as the note — RE2's "." already stops at
// a newline, so this only ever captures within one log line.
var reCoordStepStatus = regexp.MustCompile(`(?i)\bsteps?\s+(\d+)(?:-(\d+))?\s+(done|partial|blocked|not-met)\b:?\s*(.*)`)

// parseCoordinatorStepStatus turns those inline reports into the same
// validationMark shape readValidationMarkers produces for a team log —
// status AND note both — so collectSteps' shading and the burndown
// chart's click-through detail work identically regardless of source.
// Confirmed live as a real gap: the note text was being read out of the
// coordinator's own log then silently discarded, so a flat plan's
// unassigned steps never got the "what actually happened" explanation a
// teamed plan's steps already show. A range ("steps 2-3 DONE") applies
// the same status AND note to every step in it. Later matches win for a
// given step number — Status/Log is append-only and chronological, so a
// later line is a correction/update, same rule readValidationMarkers
// already follows.
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
		// Normalize to the same three-value vocabulary
		// (met/partial/not-met) validationMark and validationGlyph use
		// everywhere else — "done"/"blocked" are this convention's own
		// words, not the internal ones.
		status := strings.ToLower(m[3])
		switch status {
		case "done":
			status = "met"
		case "blocked", "not-met":
			status = "not-met"
		}
		note := strings.TrimSpace(m[4])
		for n := start; n <= end; n++ {
			marks[n] = validationMark{status: status, note: note}
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
		busy := busyDuration(r.spans)
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

// planMetaLine renders the plan detail page's harness/version/launch-command
// strip, right under the title. Three independent pieces of truth live
// here: which harness produced this plan (PlanSummary.Harness), which
// family version wrote or is running it (PlanSummary.Version, may be
// empty on a very old plan), and the exact command to resume it — in
// that harness's OWN syntax, since a dashboard that prints a command the
// reader's harness doesn't have is worse than printing none. An
// "unknown" plan (the field is genuinely absent — see PlanSummary.Harness)
// shows both forms, each labelled, rather than guessing.
func planMetaLine(plan PlanSummary) string {
	var b strings.Builder
	b.WriteString(`<div class="planmeta">`)

	hlabel, hclass := harnessBadge(plan.Harness)
	fmt.Fprintf(&b, `<span class="hbadge %s">%s</span>`, hclass, html.EscapeString(hlabel))

	if plan.Version != "" {
		fmt.Fprintf(&b, `<span class="planmeta-version">iterate family %s</span>`, html.EscapeString(plan.Version))
	}

	name := html.EscapeString(plan.Name)
	claudeCmd := `/iterate ` + name
	codexCmd := `$iterate ` + name
	switch plan.Harness {
	case "claude-code":
		fmt.Fprintf(&b, `<code class="launch-cmd">%s</code>`, claudeCmd)
	case "codex":
		fmt.Fprintf(&b, `<code class="launch-cmd">%s</code>`, codexCmd)
	default:
		fmt.Fprintf(&b, `<code class="launch-cmd"><span class="launch-tag">claude-code</span> %s</code>`, claudeCmd)
		fmt.Fprintf(&b, `<code class="launch-cmd"><span class="launch-tag">codex</span> %s</code>`, codexCmd)
	}

	b.WriteString(`</div>`)
	return b.String()
}

// RenderTimelineHTML renders the plan page from rows and the plan file
// alone, with no run record or tool history read. Pure string building, so
// tests and callers holding only rows can render it.
func RenderTimelineHTML(rows []Row, plan PlanSummary, homeURL string) string {
	return RenderPlanPage(ReportInput{Plan: plan, Rows: rows, Now: time.Now()}, PlanPageOptions{HomeURL: homeURL})
}

// tickIntervals are the only spacings a clock axis may use: each one is a
// span a person reads without arithmetic, and each divides an hour or a day
// evenly so the marks land on real wall-clock boundaries (:00, :15, 06:00)
// rather than at "1h17m after whenever this plan happened to start".
var tickIntervals = []time.Duration{
	time.Minute, 2 * time.Minute, 5 * time.Minute, 10 * time.Minute,
	15 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour,
	3 * time.Hour, 6 * time.Hour, 12 * time.Hour, 24 * time.Hour,
}

// axisTicks returns the wall-clock instants to mark between minT and maxT,
// in minT's own location, plus the interval chosen. It aims for 6-10 marks:
// fewer and the scale stops being readable, more and the labels collide at
// phone width.
//
// Boundaries are computed from local midnight with time.Date rather than by
// repeatedly adding the interval, because time.Date normalizes through a DST
// transition. Adding 1h across a spring-forward boundary walks the wall clock
// off by an hour and every later label is then wrong — on the one night a
// year when an overnight run is most likely to be the thing being read.
func axisTicks(minT, maxT time.Time) ([]time.Time, time.Duration) {
	total := maxT.Sub(minT)
	if total <= 0 {
		return nil, 0
	}
	interval := tickIntervals[len(tickIntervals)-1]
	for _, c := range tickIntervals {
		if int64(total/c) <= 10 {
			interval = c
			break
		}
	}
	loc := minT.Location()
	day := time.Date(minT.Year(), minT.Month(), minT.Day(), 0, 0, 0, 0, loc)
	step := int(interval / time.Minute)
	var ticks []time.Time
	// Start a full interval before the span so a span opening at 03:35 still
	// finds 03:30 as a candidate, and stop one past maxT for the same reason.
	for k := -1; ; k++ {
		t := time.Date(day.Year(), day.Month(), day.Day(), 0, step*k, 0, 0, loc)
		if t.After(maxT) {
			break
		}
		// A wall-clock time that does not exist — 02:00 on a spring-forward
		// morning — is normalized by time.Date back onto a time already
		// marked, so require strict advance. The skipped hour genuinely has
		// no mark, which is the truth about that morning.
		if !t.Before(minT) && (len(ticks) == 0 || t.After(ticks[len(ticks)-1])) {
			ticks = append(ticks, t)
		}
		if k > 4000 { // a runaway guard; 4000 one-minute steps is ~2.8 days
			break
		}
	}
	return ticks, interval
}

// tickLabel is the shortest thing that still says which instant this is at
// the chosen scale: bare hour numbers for an hour-or-coarser axis (the
// "10 11 12 13" a clock reads as), HH:MM when the marks are minutes apart,
// and a date once a mark can be a different day.
func tickLabel(t time.Time, interval time.Duration) string {
	switch {
	case interval >= 24*time.Hour:
		return t.Format("Jan 2")
	case interval >= time.Hour:
		if t.Hour() == 0 {
			return t.Format("Jan 2")
		}
		return t.Format("15")
	default:
		return t.Format("15:04")
	}
}

// writeTimeAxis draws the hash marks under a gantt. It repeats the row's own
// flex geometry — label-col spacer, track, duration-column spacer — so a mark
// at 40% sits exactly above the pixel a bar at 40% starts on; a plain
// full-width axis would be offset by the 172px label column and silently
// misreport every time by that much.
func writeTimeAxis(b *strings.Builder, minT, maxT time.Time, pct func(time.Time) float64) {
	ticks, interval := axisTicks(minT, maxT)
	if len(ticks) == 0 {
		return
	}
	b.WriteString(`<div class="axis-ticks"><div class="ax-pad"></div><div class="tick-track">`)
	for _, t := range ticks {
		p := pct(t)
		// A mark within a hair of either end has its label clipped by the
		// track edge and says nothing the endpoint line below does not.
		if p < 1.5 || p > 98.5 {
			continue
		}
		fmt.Fprintf(b, `<span class="tick" style="left:%.3f%%"><span class="tick-lab">%s</span></span>`,
			p, html.EscapeString(tickLabel(t, interval)))
	}
	b.WriteString(`</div><div class="ax-pad-r"></div></div>`)
}

// writeGanttRow renders one team's (or the coordinator's) row: the status
// pill and label, indented by dependency depth so two independent tracks
// read as two hierarchies, the activity track with its bars, and, when the
// plan gives the row any Steps/Validation detail, a native disclosure
// listing each Na/Nb pair. Each pair carries its requirement's state from
// the progress model and a link to its evidence. divider draws the top rule
// that separates one root-level dependency group from the next. recorded
// labels an open row "recorded open": the page shows recorded observations,
// not a live executor.
func writeGanttRow(b *strings.Builder, r Row, pct func(time.Time) float64, divider, recorded bool, reqs map[int]ProgressRequirement) {
	pillClass, pillLabel := statusPill(r.status, len(r.spans) > 0)
	if recorded && pillClass == "p-running" {
		pillLabel = "recorded open"
	}
	busy := busyDuration(r.spans)

	hasSteps := len(r.steps) > 0
	dividerClass := ""
	if divider {
		dividerClass = " row-divider"
	}
	rowOpen, rowClose := `<div class="row`+dividerClass+`">`, `</div>`
	teamLabel := html.EscapeString(r.label)
	if hasSteps {
		rowOpen, rowClose = `<details class="row-d`+dividerClass+`" data-key="lane:`+html.EscapeString(r.label)+`"><summary class="row">`, `</summary>`
		teamLabel += `<span class="chev" aria-hidden="true">&rsaquo;</span>`
	}

	pillStyle := ""
	if r.depth > 0 {
		pillStyle = fmt.Sprintf(` style="margin-left:%dpx"`, r.depth*14)
	}

	fmt.Fprintf(b, `%s<div class="label-col"><span class="pill %s" title="%s"%s></span><span class="vh">%s: </span><span class="team">%s</span></div><div class="track">`,
		rowOpen, pillClass, html.EscapeString(pillLabel), pillStyle, html.EscapeString(pillLabel), teamLabel)
	for i, s := range r.spans {
		left := pct(s.start)
		width := pct(s.end) - left
		if width < 0.15 {
			width = 0.15 // keep even instant calls visible
		}
		cls := "bar"
		// The open stripe belongs to the last span of each in-progress row,
		// not to whichever span shares the plan-wide latest timestamp.
		if i == len(r.spans)-1 && r.status == "in-progress" {
			cls = "bar bar-open"
		}
		fmt.Fprintf(b, `<div class="%s" style="left:%.3f%%;width:%.3f%%" title="%s: %s"></div>`,
			cls, left, width, html.EscapeString(s.tool), html.EscapeString(s.summary))
	}
	if len(r.spans) == 0 && (strings.HasPrefix(r.status, "done") || strings.HasPrefix(r.status, "blocked")) {
		// Done with an empty track is a real shape: a step's work can run
		// inside another team's reused agent. Say so instead of leaving a
		// blank bar that looks like missing data.
		b.WriteString(`<div class="track-note">done — activity recorded under another team</div>`)
	}
	b.WriteString(`</div>`)
	if len(r.spans) > 0 {
		fmt.Fprintf(b, `<div class="busy"><span class="vh">busy </span>%s</div>`, busy.Round(time.Second))
	} else {
		b.WriteString(`<div class="busy"><span class="vh">busy </span>—</div>`)
	}
	b.WriteString(rowClose + "\n")

	if hasSteps {
		b.WriteString(`<div class="steps-detail">`)
		for _, sd := range r.steps {
			// Each Na/Nb pair is one group with its state glyph at the left.
			// State comes from the progress model, so the lane and the
			// summary never disagree about a requirement.
			req, known := reqs[sd.Num]
			fmt.Fprintf(b, `<div class="step-group"><div class="step-glyph">%s</div><div class="step-lines">`, requirementGlyph(req.State))
			if sd.Step != "" {
				fmt.Fprintf(b, `<div class="step-pair"><span class="stepnum">%da.</span><span class="step-text">%s</span></div>`, sd.Num, html.EscapeString(sd.Step))
			}
			if sd.Validation != "" {
				fmt.Fprintf(b, `<div class="step-pair val"><span class="stepnum">%db.</span><span class="step-text">%s</span></div>`,
					sd.Num, html.EscapeString(sd.Validation))
			}
			if known {
				fmt.Fprintf(b, `<a class="step-ev" href="#%s">Evidence for requirement %d</a>`, html.EscapeString(req.EvidenceAnchor), sd.Num)
			}
			b.WriteString(`</div></div>`)
		}
		b.WriteString(`</div></details>` + "\n")
	}
}

// requirementGlyph marks a requirement's progress state with a shape as
// well as a colour, and names it for screen readers.
func requirementGlyph(state string) string {
	var cls, symbol string
	switch state {
	case "complete":
		cls, symbol = "v-met", "&#10003;"
	case "in-progress":
		cls, symbol = "v-working", "&#9679;"
	case "blocked":
		cls, symbol = "v-not-met", "&#10007;"
	case "not-started":
		return `<span class="vh">Not started</span>`
	default:
		return ""
	}
	return fmt.Sprintf(`<span class="vmark %s" aria-hidden="true">%s</span><span class="vh">%s</span>`, cls, symbol, requirementStateLabel(state))
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

func timelineHeadCSS(title string) string {
	return `<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width,initial-scale=1"><title>` + html.EscapeString(title) + `</title><style>
:root{
  --bg:#f7f4ef; --surface:#fff; --surface-2:#fbf9f5; --border:#e4ddd0;
  --text:#1c1a17; --text-dim:#6b6459; --text-faint:#736b5e;
  --busy:#0891b2; --busy-dim:#cceff5;
  --good:#059669; --good-bg:#e3f6ee; --good-text:#047857;
  --warn:#b45309; --warn-bg:#fbecd5;
  --queued:#a29b8d; --queued-bg:#f0ece2;
  --danger:#dc2626; --danger-a:#f3b8b8; --danger-b:#fbe1e1;
  --gaveup:#c2410c; --gaveup-bg:#fee3d1;
  --accent:#0e7490; --accent-bg:#e5f4f7;
}
@media (prefers-color-scheme:dark){:root{
  --bg:#0f1215; --surface:#171b1f; --surface-2:#1d2227; --border:#262c31;
  --text:#e8e6e1; --text-dim:#8b9198; --text-faint:#8a929a;
  --busy:#2dd4bf; --busy-dim:#144e49;
  --good:#34d399; --good-bg:#0d2b21; --good-text:#34d399;
  --warn:#f59e0b; --warn-bg:#3a2705;
  --queued:#565d64; --queued-bg:#1d2227;
  --danger:#ef4444; --danger-a:#5a1414; --danger-b:#2c0a0a;
  --gaveup:#fb923c; --gaveup-bg:#431407;
  --accent:#22d3ee; --accent-bg:#0f2e33;
}}
*{box-sizing:border-box}
body{margin:0;background:var(--bg);color:var(--text);font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif;padding:28px 20px 50px;line-height:1.45}
.wrap{max-width:900px;margin:0 auto}
.mono,.n,.dur,.busy,.axis,.pmeta{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-variant-numeric:tabular-nums}
.eyebrow{font-size:11px;letter-spacing:.08em;text-transform:uppercase;color:var(--accent);font-weight:600}
.archived-chip{display:inline-block;margin-left:6px;padding:1px 6px;border-radius:4px;font-size:10px;font-weight:600;letter-spacing:.02em;background:var(--surface-2);color:var(--text-faint);border:1px solid var(--border)}
h1{font-size:24px;font-weight:700;margin:2px 0 8px;text-wrap:balance;letter-spacing:-.01em}
.planmeta{display:flex;flex-wrap:wrap;align-items:center;gap:8px;margin:0 0 14px}
.hbadge{font-size:10px;letter-spacing:.02em;padding:3px 8px;border-radius:4px;text-align:center;font-weight:600;border:1px solid transparent}
.h-claude{background:var(--accent-bg);color:var(--accent)}
.h-codex{background:var(--good-bg);color:var(--good)}
.h-unknown{background:var(--queued-bg);color:var(--text-dim);border-color:var(--border)}
.planmeta-version{font-size:11px;color:var(--text-faint);font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
.launch-cmd{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:11.5px;background:var(--surface-2);border:1px solid var(--border);border-radius:5px;padding:2px 8px;color:var(--text)}
.launch-tag{color:var(--text-faint);margin-right:4px}
h2{font-size:13px;text-transform:uppercase;letter-spacing:.06em;color:var(--text-dim);font-weight:600;margin:26px 0 12px}
.sub{color:var(--text-dim);font-size:12px;margin-bottom:10px}
.back{display:inline-block;font-size:12.5px;color:var(--accent);text-decoration:none;margin-bottom:10px}
.back:hover{text-decoration:underline}
.goal{margin:0 0 18px;padding:14px 16px;background:var(--accent-bg);border:1px solid var(--border);border-radius:8px}
.goal-label{font-size:10px;letter-spacing:.08em;text-transform:uppercase;color:var(--accent);font-weight:700;margin-bottom:5px}
.goal p{color:var(--text-dim);font-size:13.5px;margin:0}
.blocked-banner{margin:0 0 18px;padding:14px 16px;background:var(--danger-b);border:1px solid var(--danger);border-radius:8px}
.blocked-label{font-size:10px;letter-spacing:.08em;text-transform:uppercase;color:var(--danger);font-weight:700;margin-bottom:5px}
.blocked-banner p{color:var(--text);font-size:13.5px;margin:0}
.empty{color:var(--text-faint);font-style:italic}
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
.busy{width:60px;flex:0 0 auto;text-align:right;font-size:12px;color:var(--text-dim)}
.track-note{position:absolute;top:0;left:0;height:100%;display:flex;align-items:center;padding-left:8px;font-size:11px;font-style:italic;color:var(--text-faint)}
.axis{display:flex;justify-content:space-between;padding:2px 10px 2px;font-size:10.5px;color:var(--text-faint)}
.axis-ticks{display:flex;align-items:flex-start;gap:14px;padding:4px 10px 0}
.axis-ticks .ax-pad{width:172px;flex:0 0 auto}
.axis-ticks .ax-pad-r{width:60px;flex:0 0 auto}
.tick-track{position:relative;flex:1 1 auto;height:20px}
.tick{position:absolute;top:0;width:1px;height:6px;background:var(--text-faint);opacity:.85}
.tick-lab{position:absolute;top:7px;left:50%;transform:translateX(-50%);font-size:10.5px;color:var(--text-faint);white-space:nowrap;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-variant-numeric:tabular-nums}
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
.vmark.v-met{color:var(--good-text)}
.vmark.v-partial{color:var(--warn)}
.vmark.v-not-met{color:var(--gaveup)}
.vmark.v-working{color:var(--warn);animation:vworking 1.4s ease-in-out infinite}
@keyframes vworking{0%,100%{opacity:1}50%{opacity:.3}}
footer{margin-top:32px;font-size:11.5px;color:var(--text-faint);border-top:1px solid var(--border);padding-top:14px}
`
}

// WriteTimelineHTML renders the plan page and writes it to
// <cwd>/.claude/iterate/timeline.html: the file-output counterpart to the
// dashboard page, kept separate since a live dashboard never writes to disk.
func WriteTimelineHTML(cwd string, in ReportInput) (string, error) {
	if len(in.Rows) == 0 {
		return "", fmt.Errorf("no events to render")
	}
	path := filepath.Join(cwd, ".claude", "iterate", "timeline.html")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(path, []byte(RenderPlanPage(in, PlanPageOptions{})), 0o644); err != nil {
		return "", err
	}
	return path, nil
}
