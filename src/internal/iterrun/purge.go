package iterrun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// PurgeResult reports what PurgePlan removed (or, in dry-run mode, what it
// would remove) so the CLI can print an honest accounting either way.
type PurgeResult struct {
	Plan          string
	EventsRemoved int
	LabelsRemoved int
	RegistryFiles []string
}

// PurgeEligible returns every plan, across every known project, whose
// Teams table shows every row terminal (done or blocked) — the only
// plans --all-completed is allowed to touch. A flat plan (no Teams table)
// is never eligible here regardless of how done it looks, since there's
// no reliable signal for that short of a human saying so.
func PurgeEligible() ([]PlanSummary, error) {
	projects, err := ListProjects()
	if err != nil {
		return nil, err
	}
	var eligible []PlanSummary
	for _, proj := range projects {
		plans, err := ListPlans(proj)
		if err != nil {
			continue
		}
		for _, p := range plans {
			if p.IsCompleted() {
				eligible = append(eligible, p)
			}
		}
	}
	return eligible, nil
}

// FindPlan locates a plan by name across every known project. Errors
// loudly on zero or multiple matches rather than silently picking one —
// a purge command guessing which of two same-named plans you meant is
// exactly the wrong place to guess.
func FindPlan(name string) (PlanSummary, error) {
	projects, err := ListProjects()
	if err != nil {
		return PlanSummary{}, err
	}
	var matches []PlanSummary
	for _, proj := range projects {
		plans, err := ListPlans(proj)
		if err != nil {
			continue
		}
		for _, p := range plans {
			if p.Name == name {
				matches = append(matches, p)
			}
		}
	}
	switch len(matches) {
	case 0:
		return PlanSummary{}, fmt.Errorf("no plan named %q found across %d known project(s)", name, len(projects))
	case 1:
		return matches[0], nil
	default:
		var dirs []string
		for _, m := range matches {
			dirs = append(dirs, m.ProjectDir)
		}
		return PlanSummary{}, fmt.Errorf("plan %q exists in more than one project (%s) — purge acts on tracking data for that plan NAME across every known project, so confirm that's really what you want before forcing this", name, strings.Join(dirs, ", "))
	}
}

// PurgePlan removes this plan's TRACKING data — central hook events tagged
// to it, its agent labels, and its iterate-run registry entries wherever
// they landed (a team can and does work in a directory far from the
// plan's own project, so every known project is checked, not just the
// plan's own). It never touches the plan's own .md file, its team
// log.md files, or any project source — those are authored content, not
// observability exhaust, and purge's whole job is cleaning up the exhaust.
func PurgePlan(name string, dryRun bool) (PurgeResult, error) {
	res := PurgeResult{Plan: name}

	events, err := ReadEvents()
	if err != nil {
		return res, err
	}
	var kept []Event
	for _, e := range events {
		if e.Plan == name {
			res.EventsRemoved++
			continue
		}
		kept = append(kept, e)
	}
	if !dryRun && res.EventsRemoved > 0 {
		if err := rewriteEvents(kept); err != nil {
			return res, err
		}
	}

	labels, err := ReadLabels()
	if err != nil {
		return res, err
	}
	prefix := name + "-"
	for agentID, label := range labels {
		if label == name || strings.HasPrefix(label, prefix) {
			res.LabelsRemoved++
			if !dryRun {
				delete(labels, agentID)
			}
		}
	}
	if !dryRun && res.LabelsRemoved > 0 {
		if err := writeLabels(labels); err != nil {
			return res, err
		}
	}

	projects, err := ListProjects()
	if err != nil {
		return res, err
	}
	for _, proj := range projects {
		dir := RegistryDir(proj)
		matches, _ := filepath.Glob(filepath.Join(dir, name+".*.json"))
		for _, m := range matches {
			res.RegistryFiles = append(res.RegistryFiles, m)
			logFile := strings.TrimSuffix(m, ".json") + ".log"
			if !dryRun {
				_ = os.Remove(m)
				_ = os.Remove(logFile)
			} else if _, err := os.Stat(logFile); err == nil {
				res.RegistryFiles = append(res.RegistryFiles, logFile)
			}
		}
	}

	return res, nil
}

func rewriteEvents(events []Event) error {
	path := EventsPath()
	tmp := path + ".tmp"
	f, err := os.Create(tmp)
	if err != nil {
		return err
	}
	for _, e := range events {
		data, err := json.Marshal(e)
		if err != nil {
			continue
		}
		if _, err := f.Write(append(data, '\n')); err != nil {
			f.Close()
			return err
		}
	}
	if err := f.Close(); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func writeLabels(labels map[string]string) error {
	path := LabelsPath()
	data, err := json.MarshalIndent(labels, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}
