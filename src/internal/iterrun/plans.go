package iterrun

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// PlanSummary is what the dashboard and purge commands need to know about
// one plan, parsed straight from its markdown file — never a separate
// database, so it can never drift from what a human reading the file sees.
type PlanSummary struct {
	Name        string
	ProjectDir  string
	Phase       string
	Status      string // freeform "status:" frontmatter line — "blocked-on-operator: <reason>" is the one standardized value the dashboard specifically surfaces; anything else is just carried through unused
	NextAttempt string // the "> **Next attempt ...**" blockquote banner at the top of the file, if present — the coordinator's own exact instructions for clearing a blocked-on-operator state
	Teamed      bool
	Started     string
	// Executing is this plan's own declared "Executing:" value — when THIS
	// execution attempt actually began (set once, at the moment /iterate
	// transitions the plan from phase: planned to phase: executing; never
	// touched again by heartbeats or resumes). Deliberately distinct from
	// Started, which is drafting time and can predate real work by hours
	// when the plan went through /iterate-planner first — conflating the
	// two is what made the dashboard's "running for" figure show elapsed
	// planning time instead of elapsed execution time. Empty for plans
	// written before this field existed, or a plan still sitting at
	// phase: planned.
	Executing  string
	Goal       string // truncated to ~160 chars — for the dashboard card list
	GoalFull   string // untruncated — for the plan detail page
	HasTeams   bool
	TeamsTotal int
	TeamsDone  int
	// Archived is true for a plan read from .claude/iterate/archive/ (see
	// ListArchivedPlans) instead of the live .claude/iterate/plans/ — a
	// completed or given-up run, moved there by /iterate on exit per its
	// own SKILL.md. ArchiveFile is the archived file's exact base name
	// (e.g. "20260812T012354Z-aardvark-done.md", NOT just "<name>.md" —
	// the timestamp prefix is what the live plans/ directory drops), the
	// one piece of information needed to re-locate this exact run's file
	// and its sibling ".teams/" dir later (BuildRowsFromArchive).
	Archived    bool
	ArchiveFile string
}

// IsCompleted reports whether every team in this plan's Teams table has
// reached a terminal status (done or blocked). Plans with no Teams table
// (flat plans) are never reported completed here — there's no reliable
// signal for "done" on a flat plan short of the coordinator saying so, and
// guessing wrong is the one place that matters for a purge command.
func (p PlanSummary) IsCompleted() bool {
	return p.HasTeams && p.TeamsTotal > 0 && p.TeamsDone == p.TeamsTotal
}

// Blocked reports whether this plan is stuck on something only a human can
// clear — the standardized "status: blocked-on-operator: <reason>" a
// coordinator writes once every step it can act on is done and the one
// remaining thing (billing, an external approval, physical access) is
// outside any agent's reach. Takes priority over phase/IsCompleted for
// display: a plan can look 100% done by its Teams table and still be
// sitting here waiting on you.
func (p PlanSummary) Blocked() bool {
	return strings.HasPrefix(p.Status, "blocked-on-operator")
}

// StartedAt is this plan's own declared Started: value, parsed into the
// instant it names (see parsePlanStarted). Returns the zero time and
// ok=false if it can't be determined — callers treat that as "unknown,
// don't filter on it" rather than guessing.
func (p PlanSummary) StartedAt() (time.Time, bool) {
	return parsePlanStarted(p.Started)
}

// ExecutingAt is this plan's own declared Executing: value, parsed the same
// way as StartedAt. Returns ok=false when absent (plan predates the field,
// or hasn't started executing yet) — callers fall back to a different
// signal rather than guessing.
func (p PlanSummary) ExecutingAt() (time.Time, bool) {
	return parsePlanStarted(p.Executing)
}

var (
	reName             = regexp.MustCompile(`^name:\s*(.+)$`)
	rePhase            = regexp.MustCompile(`^phase:\s*(.+)$`)
	reStarted          = regexp.MustCompile(`^Started:\s*(.+)$`)
	reExecuting        = regexp.MustCompile(`^Executing:\s*(.+)$`)
	reTeamed           = regexp.MustCompile(`^teamed:\s*true\s*$`)
	reStatus           = regexp.MustCompile(`^status:\s*(.+)$`)
	reLeadingTimestamp = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})(?:[ T](\d{2}:\d{2}:\d{2})(Z)?)?`)
)

// parsePlanStarted best-effort parses a plan's freeform Started:/Executing:
// value into the instant it names. Two formats are in real use and must
// parse to the SAME instant they actually name, not just the same digits:
//   - space-separated, no zone ("2026-08-10 05:39:52", "2026-08-10",
//     "2026-08-10 (planned)") — written by hand or by older code, and
//     genuinely local wall-clock time.
//   - ISO 8601 UTC ("2026-08-11T13:59:24Z") — what /iterate and
//     /iterate-planner actually write (`date -u +%Y-%m-%dT%H:%M:%SZ`).
//
// A trailing "Z" is the signal to parse as UTC instead of local — confirmed
// live as a real, silent bug: the old version matched only the digit
// groups, discarded "Z" as ignorable trailing text same as "(planned)",
// and fed the bare digits to time.ParseInLocation(..., time.Local) —
// reinterpreting a UTC clock reading as already-local wall-clock time.
// That doesn't just mislabel the displayed "since" timestamp, it silently
// shifts the parsed INSTANT by the local UTC offset (5h understated on a
// UTC-5 machine, confirmed by hand-tracing the exact figure a live "Running
// for 2h36m40s" bug reproduced). Trailing freeform text (like "(planned)")
// is otherwise ignored; ok is false if no leading date could be found at
// all, which callers treat as "unknown, don't filter" rather than guessing.
func parsePlanStarted(s string) (time.Time, bool) {
	m := reLeadingTimestamp.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return time.Time{}, false
	}
	layout, value := "2006-01-02", m[1]
	if m[2] != "" {
		layout, value = "2006-01-02 15:04:05", m[1]+" "+m[2]
	}
	loc := time.Local
	if m[3] == "Z" {
		loc = time.UTC
	}
	t, err := time.ParseInLocation(layout, value, loc)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}

// ListPlans scans <projectDir>/.claude/iterate/plans/*.md, newest first.
func ListPlans(projectDir string) ([]PlanSummary, error) {
	dir := filepath.Join(projectDir, ".claude", "iterate", "plans")
	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	var out []PlanSummary
	for _, m := range matches {
		ps, err := parsePlanFile(m, projectDir)
		if err != nil {
			continue
		}
		out = append(out, ps)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Started > out[j].Started })
	return out, nil
}

// ListArchivedPlans scans <projectDir>/.claude/iterate/archive/*.md — every
// run /iterate has ever finished or given up on in this project, newest
// first (the UTC-timestamp filename prefix sorts correctly as a plain
// string). Confirmed live as a real gap: without this, the dashboard had
// no way at all to see a plan once it archived — the exact moment a run
// finishes is when someone most wants to review it, and it would just
// silently vanish from the "0 plans" project listing.
func ListArchivedPlans(projectDir string) ([]PlanSummary, error) {
	dir := filepath.Join(projectDir, ".claude", "iterate", "archive")
	matches, err := filepath.Glob(filepath.Join(dir, "*.md"))
	if err != nil {
		return nil, err
	}
	var out []PlanSummary
	for _, m := range matches {
		ps, err := parsePlanFile(m, projectDir)
		if err != nil {
			continue
		}
		ps.Archived = true
		ps.ArchiveFile = filepath.Base(m)
		out = append(out, ps)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ArchiveFile > out[j].ArchiveFile })
	return out, nil
}

// GetArchivedPlanSummary reads one specific archived plan file directly, by
// its exact archive filename (see PlanSummary.ArchiveFile) — the detail-page
// counterpart to GetPlanSummary.
func GetArchivedPlanSummary(homeDir, archiveFile string) (PlanSummary, error) {
	path := filepath.Join(homeDir, ".claude", "iterate", "archive", archiveFile)
	ps, err := parsePlanFile(path, homeDir)
	if err != nil {
		return ps, err
	}
	ps.Archived = true
	ps.ArchiveFile = archiveFile
	return ps, nil
}

func parsePlanFile(path, projectDir string) (PlanSummary, error) {
	f, err := os.Open(path)
	if err != nil {
		return PlanSummary{}, err
	}
	defer f.Close()

	ps := PlanSummary{ProjectDir: projectDir, Name: strings.TrimSuffix(filepath.Base(path), ".md")}

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	inGoal, inTeams := false, false
	var goalLines []string
	// The "Next attempt" banner is a blockquote ("> ...") near the top of
	// the file — the coordinator's own exact instructions for clearing a
	// blocked-on-operator state. Collected as one contiguous run of ">"
	// lines starting from wherever "Next attempt" first appears; a plan
	// with no such banner just never sets inNextAttempt and this stays
	// empty.
	inNextAttempt := false
	var nextAttemptLines []string

	for scanner.Scan() {
		line := scanner.Text()
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, ">") {
			if !inNextAttempt && strings.Contains(trimmed, "Next attempt") {
				inNextAttempt = true
			}
			if inNextAttempt {
				nextAttemptLines = append(nextAttemptLines, strings.TrimSpace(strings.TrimPrefix(trimmed, ">")))
			}
			continue
		}
		inNextAttempt = false // blockquote block ended (a non-">" line, including a blank one)

		if m := reName.FindStringSubmatch(line); m != nil {
			ps.Name = strings.TrimSpace(m[1])
		}
		if m := rePhase.FindStringSubmatch(line); m != nil {
			ps.Phase = strings.TrimSpace(m[1])
		}
		if m := reStarted.FindStringSubmatch(line); m != nil {
			ps.Started = strings.TrimSpace(m[1])
		}
		if m := reExecuting.FindStringSubmatch(line); m != nil {
			ps.Executing = strings.TrimSpace(m[1])
		}
		if m := reStatus.FindStringSubmatch(line); m != nil {
			ps.Status = strings.TrimSpace(m[1])
		}
		if reTeamed.MatchString(line) {
			ps.Teamed = true
		}

		if strings.HasPrefix(trimmed, "## ") {
			inGoal = trimmed == "## Goal"
			inTeams = trimmed == "## Teams"
			continue
		}
		if inGoal && trimmed != "" {
			goalLines = append(goalLines, trimmed)
		}
		if inTeams {
			if team, status, ok := parseTeamRow(line); ok {
				ps.HasTeams = true
				ps.TeamsTotal++
				if strings.HasPrefix(status, "done") || strings.HasPrefix(status, "blocked") {
					ps.TeamsDone++
				}
				_ = team
			}
		}
	}

	ps.GoalFull = strings.Join(goalLines, " ")
	ps.Goal = ps.GoalFull
	if len(ps.Goal) > 160 {
		ps.Goal = ps.Goal[:160] + "…"
	}
	ps.NextAttempt = strings.TrimSpace(strings.Join(nextAttemptLines, " "))
	return ps, nil
}

// GetPlanSummary reads and parses one specific plan file directly, for
// callers that already know its name and don't need the whole directory
// listing (the plan detail page, mainly).
func GetPlanSummary(homeDir, name string) (PlanSummary, error) {
	path := filepath.Join(homeDir, ".claude", "iterate", "plans", name+".md")
	return parsePlanFile(path, homeDir)
}

// parseTeamRowCells splits one Teams-table row into its trimmed cells,
// rejecting the header row ("| Team | ... |") and the separator row
// ("|---|...|") rather than trying to special-case their exact text. The
// schema is Team | Steps | Focus | Depends on | Agent | Status, so
// cells[0] is always the team name and cells[len-1] is always the status.
func parseTeamRowCells(line string) ([]string, bool) {
	if !strings.HasPrefix(strings.TrimSpace(line), "|") {
		return nil, false
	}
	fields := strings.Split(line, "|")
	var cells []string
	for _, f := range fields {
		cells = append(cells, strings.TrimSpace(f))
	}
	if len(cells) > 0 && cells[0] == "" {
		cells = cells[1:]
	}
	if len(cells) > 0 && cells[len(cells)-1] == "" {
		cells = cells[:len(cells)-1]
	}
	if len(cells) < 2 {
		return nil, false
	}
	team := cells[0]
	if team == "" || team == "Team" || strings.HasPrefix(team, "---") || strings.HasPrefix(team, ":--") {
		return nil, false
	}
	return cells, true
}

// parseTeamRow pulls just (team name, status) out of a Teams-table row.
func parseTeamRow(line string) (team, status string, ok bool) {
	cells, ok := parseTeamRowCells(line)
	if !ok {
		return "", "", false
	}
	return cells[0], cells[len(cells)-1], true
}
