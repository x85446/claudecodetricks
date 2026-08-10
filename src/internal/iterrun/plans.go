package iterrun

import (
	"bufio"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// PlanSummary is what the dashboard and purge commands need to know about
// one plan, parsed straight from its markdown file — never a separate
// database, so it can never drift from what a human reading the file sees.
type PlanSummary struct {
	Name       string
	ProjectDir string
	Phase      string
	Teamed     bool
	Started    string
	Goal       string
	HasTeams   bool
	TeamsTotal int
	TeamsDone  int
}

// IsCompleted reports whether every team in this plan's Teams table has
// reached a terminal status (done or blocked). Plans with no Teams table
// (flat plans) are never reported completed here — there's no reliable
// signal for "done" on a flat plan short of the coordinator saying so, and
// guessing wrong is the one place that matters for a purge command.
func (p PlanSummary) IsCompleted() bool {
	return p.HasTeams && p.TeamsTotal > 0 && p.TeamsDone == p.TeamsTotal
}

var (
	reName    = regexp.MustCompile(`^name:\s*(.+)$`)
	rePhase   = regexp.MustCompile(`^phase:\s*(.+)$`)
	reStarted = regexp.MustCompile(`^Started:\s*(.+)$`)
	reTeamed  = regexp.MustCompile(`^teamed:\s*true\s*$`)
)

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

	for scanner.Scan() {
		line := scanner.Text()

		if m := reName.FindStringSubmatch(line); m != nil {
			ps.Name = strings.TrimSpace(m[1])
		}
		if m := rePhase.FindStringSubmatch(line); m != nil {
			ps.Phase = strings.TrimSpace(m[1])
		}
		if m := reStarted.FindStringSubmatch(line); m != nil {
			ps.Started = strings.TrimSpace(m[1])
		}
		if reTeamed.MatchString(line) {
			ps.Teamed = true
		}

		trimmed := strings.TrimSpace(line)
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

	ps.Goal = strings.Join(goalLines, " ")
	if len(ps.Goal) > 220 {
		ps.Goal = ps.Goal[:220] + "…"
	}
	return ps, nil
}

// parseTeamRow pulls (team name, status) out of one Teams-table row,
// rejecting the header row ("| Team | ... |") and the separator row
// ("|---|...|") rather than trying to special-case their exact text.
func parseTeamRow(line string) (team, status string, ok bool) {
	if !strings.HasPrefix(strings.TrimSpace(line), "|") {
		return "", "", false
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
		return "", "", false
	}
	team = cells[0]
	status = cells[len(cells)-1]
	if team == "" || team == "Team" || strings.HasPrefix(team, "---") || strings.HasPrefix(team, ":--") {
		return "", "", false
	}
	return team, status, true
}
