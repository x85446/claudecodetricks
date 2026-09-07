package iterrun

import (
	"fmt"
	"sort"
	"time"
)

// staleGrace is how long a done/failed entry stays visible in `status`
// before it's treated as gone — long enough to see "it just finished",
// short enough that the view doesn't accumulate history forever.
const staleGrace = 2 * time.Minute

// PrintStatus renders the human-facing tree view: every plan with a live
// (or recently-finished) entry, teams under it, units under each team.
func PrintStatus(cwd string, w fmtWriter) {
	dir := RegistryDir(cwd)
	entries, err := ScanRegistry(dir)
	if err != nil {
		fmt.Fprintf(w, "no registry yet at %s\n", dir)
		return
	}

	now := time.Now().UTC()
	var live []Entry
	for _, e := range entries {
		if e.Finished != nil && now.Sub(*e.Finished) > staleGrace {
			continue
		}
		live = append(live, e)
	}
	if len(live) == 0 {
		fmt.Fprintln(w, "no active or recently-finished iterate-run units")
		return
	}

	byPlan := map[string][]Entry{}
	for _, e := range live {
		byPlan[e.Plan] = append(byPlan[e.Plan], e)
	}

	plans := make([]string, 0, len(byPlan))
	for p := range byPlan {
		plans = append(plans, p)
	}
	sort.Strings(plans)

	for _, plan := range plans {
		es := byPlan[plan]
		active, done := 0, 0
		for _, e := range es {
			if e.Status == StatusDone || e.Status == StatusFailed {
				done++
			} else {
				active++
			}
		}
		fmt.Fprintf(w, "plan: %s\n", plan)
		fmt.Fprintf(w, "agents: %d active, %d done\n\n", active, done)

		byTeam := map[string][]Entry{}
		for _, e := range es {
			byTeam[e.Team] = append(byTeam[e.Team], e)
		}
		teams := make([]string, 0, len(byTeam))
		for t := range byTeam {
			teams = append(teams, t)
		}
		sort.Strings(teams)

		for ti, team := range teams {
			prefix := "├─"
			if ti == len(teams)-1 {
				prefix = "└─"
			}
			label := team
			if label == "" {
				label = "unassigned"
			}
			units := byTeam[team]
			sort.Slice(units, func(i, j int) bool { return units[i].Unit < units[j].Unit })
			for _, u := range units {
				fmt.Fprintf(w, "%s team: %s [%s] — %s\n", prefix, label, describeStatus(u, now), u.Unit)
			}
		}
		fmt.Fprintln(w)
	}
}

func describeStatus(e Entry, now time.Time) string {
	switch e.Status {
	case StatusDone:
		return "done"
	case StatusFailed:
		code := -1
		if e.ExitCode != nil {
			code = *e.ExitCode
		}
		return fmt.Sprintf("failed, exit=%d", code)
	case StatusStalled:
		age := now.Sub(e.LastActivity).Round(time.Second)
		return fmt.Sprintf("stalled, no activity for %s", age)
	default:
		age := now.Sub(e.LastHeartbeat).Round(time.Second)
		msg := e.LastMessage
		if e.Pct != nil {
			return fmt.Sprintf("%s, updated %s ago, %.0f%% — %s", e.Status, age, *e.Pct, msg)
		}
		if e.Done != nil && e.Total != nil {
			return fmt.Sprintf("%s, updated %s ago, %d/%d — %s", e.Status, age, *e.Done, *e.Total, msg)
		}
		return fmt.Sprintf("%s, updated %s ago — %s", e.Status, age, msg)
	}
}

// fmtWriter is the minimal io.Writer surface used here, to keep this file
// free of importing "io" just for a type name.
type fmtWriter interface {
	Write(p []byte) (n int, err error)
}
