package iterrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/plangraph"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type ConductorPlanSource struct{}

func NewConductorService(dir string) *conductor.Service {
	return conductor.NewService(dir, ConductorPlanSource{})
}
func (ConductorPlanSource) List(ctx context.Context, p model.Project) ([]conductor.Plan, error) {
	ps, e := ListPlans(p.Path)
	if e != nil {
		return nil, e
	}
	graph, graphErr := ProjectGraph(p.Path)
	out := []conductor.Plan{}
	for _, v := range ps {
		started, _ := v.StartedAt()
		state := v.StateLabel()
		running := conductorManualPlanRunning(p.Path, v, time.Now())
		plan := conductor.Plan{Running: running, Name: v.Name, ProjectPath: p.Path, ID: conductor.PlanID(p.Path, v.Name), State: state, Staged: v.IsQueued() || v.Phase == "executing" || state == "unblocked", Blocked: state == "blocked" || state == "paused", Reason: v.Status, Release: v.TypedOnly, Started: started, Complete: v.Phase == "complete" || v.Phase == "done", Milestone: v.Milestone}
		if graphErr != nil && len(v.Depends) > 0 {
			plan.Blocked, plan.State, plan.Reason = true, "blocked", "Plan dependencies unreadable: "+graphErr.Error()
		} else if n, ok := graph.Node(v.Name); ok && graphErr == nil {
			plan.Depth = n.Depth
			switch n.Status {
			case plangraph.StatusBlocked:
				if !plan.Blocked {
					plan.Blocked, plan.State, plan.Reason = true, "blocked", "Plan dependencies: "+n.Reason
				}
			case plangraph.StatusWaiting:
				plan.Waiting, plan.WaitingOn, plan.WaitReason, plan.HeldBy = true, n.WaitingOn, n.Reason, n.HeldBy
			}
		}
		if plan.Staged && !plan.Blocked && !plan.Running && !plan.Complete {
			if _, err := (ConductorPlanSource{}).Pair(ctx, p, v.Name); err != nil {
				plan.Blocked, plan.State, plan.Reason = true, "blocked", "Plan cannot run: "+err.Error()
			}
		}
		out = append(out, plan)
	}
	return out, nil
}
func safePlanPath(project, name string) (string, error) {
	if name == "" || filepath.Base(name) != name || strings.ContainsAny(name, "/\\\x00") {
		return "", errors.New("invalid plan name")
	}
	base := filepath.Join(project, ".claude", "iterate", "plans")
	path := filepath.Join(base, name+".md")
	resolved, e := filepath.EvalSymlinks(path)
	if e != nil {
		return "", e
	}
	canonicalBase, e := filepath.EvalSymlinks(base)
	if e != nil {
		return "", e
	}
	if filepath.Dir(resolved) != canonicalBase {
		return "", errors.New("plan resolves outside project plan directory")
	}
	return path, nil
}

type pairLines struct {
	step, taskLine, validationLine int
	task, validation               string
	done                           bool
	team                           string
}

func parseConductorPairs(text string) ([]pairLines, error) {
	lines := strings.Split(text, "\n")
	section := ""
	pairs := []pairLines{}
	index := map[int]int{}
	for n, line := range lines {
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
		}
		item, ok := parsePlanItem(strings.TrimSpace(line))
		if !ok {
			continue
		}
		step := item.Num
		switch section {
		case "Steps":
			if _, exists := index[step]; exists {
				return nil, fmt.Errorf("step %d occurs more than once", step)
			}
			index[step] = len(pairs)
			pairs = append(pairs, pairLines{step: step, taskLine: n, validationLine: -1, task: item.Text, done: item.Checked})
		case "Validation":
			if i, ok := index[step]; ok {
				if pairs[i].validationLine >= 0 {
					return nil, fmt.Errorf("validation %d occurs more than once", step)
				}
				pairs[i].validationLine = n
				pairs[i].validation = item.Text
				pairs[i].done = pairs[i].done && item.Checked
			}
		}
	}
	for _, p := range pairs {
		if p.validationLine < 0 {
			return nil, fmt.Errorf("step %d has no validation pair", p.step)
		}
	}
	return pairs, nil
}
func pairDigest(p pairLines) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%d\n%s\n%s", p.step, p.task, p.validation)))
	return hex.EncodeToString(h[:])
}
func (ConductorPlanSource) Pair(ctx context.Context, p model.Project, name string) (conductor.Pair, error) {
	path, e := safePlanPath(p.Path, name)
	if e != nil {
		return conductor.Pair{}, e
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return conductor.Pair{}, e
	}
	pairs, e := parseConductorPairs(string(b))
	if e != nil {
		return conductor.Pair{}, e
	}
	if len(pairs) == 0 {
		return conductor.Pair{}, errors.New("plan has no task/validation pairs")
	}
	teams := conductorTeams(string(b))
	for i := range pairs {
		for name, team := range teams {
			for _, step := range team.steps {
				if step == pairs[i].step {
					pairs[i].team = name
				}
			}
		}
	}
	for _, v := range pairs {
		if !v.done {
			ready := true
			if team, ok := teams[v.team]; ok {
				for _, dep := range team.dependencies {
					parent, found := teams[dep]
					if !found {
						ready = false
						continue
					}
					for _, step := range parent.steps {
						for _, pair := range pairs {
							if pair.step == step && !pair.done {
								ready = false
							}
						}
					}
				}
			}
			if !ready {
				continue
			}
			return conductor.Pair{Step: v.step, Team: v.team, Task: v.task, Validation: v.validation, Digest: pairDigest(v)}, nil
		}
	}
	for _, p := range pairs {
		if !p.done {
			return conductor.Pair{}, errors.New("waiting for team dependencies")
		}
	}
	h := sha256.Sum256(b)
	return conductor.Pair{Digest: hex.EncodeToString(h[:])}, nil
}
func (ConductorPlanSource) Checkpoint(ctx context.Context, a model.Assignment, r model.Result) error {
	path, e := safePlanPath(a.ProjectPath, a.Plan)
	if e != nil {
		return e
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	pairs, e := parseConductorPairs(string(b))
	if e != nil {
		return e
	}
	lines := strings.Split(string(b), "\n")
	for _, p := range pairs {
		if p.step != a.Step {
			continue
		}
		if pairDigest(p) != a.SourceDigest {
			return errors.New("task/validation content changed during assignment")
		}
		if p.done {
			return nil
		}
		lines[p.taskLine] = fmt.Sprintf("- [x] %d. %s", p.step, p.task)
		lines[p.validationLine] = fmt.Sprintf("- [x] %d. %s", p.step, p.validation)
		text := strings.Join(lines, "\n")
		text = conductorSetField(text, "phase", "executing")
		text = conductorSetField(text, "status", "executing")
		text += "\n<!-- conductor checkpoint " + a.ID + " -->\n"
		for _, ev := range r.Evidence {
			text += "- Validation evidence: " + strings.ReplaceAll(ev, "\n", " ") + "\n"
		}
		return conductor.AtomicWrite(path, []byte(text))
	}
	return errors.New("assignment step is absent")
}
func conductorSetField(text, key, value string) string {
	re := regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(key) + `:.*$`)
	if re.MatchString(text) {
		return re.ReplaceAllStringFunc(text, func(string) string { return key + ": " + value })
	}
	return key + ": " + value + "\n" + text
}

// conductorManualPlanRunning respects an explicit release before consulting activity.
// Generic project hooks provide a fallback only for an executing plan without a release.
func conductorManualPlanRunning(project string, plan PlanSummary, now time.Time) bool {
	path, err := safePlanPath(project, plan.Name)
	if err != nil {
		return false
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	marker := regexp.MustCompile(`(?m)^running:[ \t]*([^\r\n]*)`).FindSubmatch(raw)
	if len(marker) > 1 {
		value := strings.TrimSpace(string(marker[1]))
		if value == "false" {
			return false
		}
		if at, err := time.Parse(time.RFC3339, value); err == nil && now.Sub(at) >= 0 && now.Sub(at) < 90*time.Second {
			return true
		}
	}
	return plan.StateLabel() == "executing" && CurrentPlanName(project) == plan.Name && IsLive(project, now) && !conductorOwnTrail(project)
}

func (ConductorPlanSource) Gate(ctx context.Context, p model.Project, now time.Time) error {
	policy, e := ReadLaunchSchedule(p.Path)
	if e != nil {
		return e
	}
	schedule, e := ReadConductorSchedule(p.Path)
	if e != nil {
		return e
	}
	if ok, reason, _ := AllowedAll(now, policy, schedule); !ok {
		return fmt.Errorf("project launch policy: %s", reason)
	}
	plans, e := ListPlans(p.Path)
	if e != nil {
		return e
	}
	for _, plan := range plans {
		if conductorManualPlanRunning(p.Path, plan, now) {
			return errors.New("manual iterate run lock owns this project")
		}
	}
	return nil
}
func (ConductorPlanSource) Outcome(ctx context.Context, a model.Assignment, r model.Result) error {
	path, e := safePlanPath(a.ProjectPath, a.Plan)
	if os.IsNotExist(e) && a.Kind == "delivery" && r.Outcome == "completed" {
		files, _ := filepath.Glob(filepath.Join(a.ProjectPath, ".claude", "iterate", "archive", "*-"+a.Plan+"-done.md"))
		for _, file := range files {
			b, _ := os.ReadFile(file)
			if strings.Contains(string(b), "<!-- conductor delivery "+a.ID+" -->") {
				return nil
			}
		}
	}
	if e != nil {
		return e
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	text := string(b)
	switch {
	case a.Kind == "intake" && r.Outcome == "completed":
		pairs, e := parseConductorPairs(text)
		if e != nil {
			return e
		}
		if len(pairs) == 0 {
			return errors.New("intake result has no paired plan")
		}
		summary, e := GetPlanSummary(a.ProjectPath, a.Plan)
		if e != nil {
			return e
		}
		if summary.Phase != "planned" || summary.IsQueued() || summary.StateLabel() == "executing" {
			return errors.New("issue intake must leave an unstaged planned document")
		}
		if !strings.Contains(text, a.SourceDigest) {
			return errors.New("intake plan lacks source provenance")
		}
		return nil
	case r.Outcome == "blocked" || r.Outcome == "partial" || r.Outcome == "interrupted":
		text = conductorSetField(text, "status", "blocked-on-operator: "+strings.ReplaceAll(r.Blocker, "\n", " "))
		text += "\n<!-- conductor outcome " + a.ID + ": " + r.Outcome + " -->\n"
		for _, v := range r.Evidence {
			text += "- Delivery evidence: " + strings.ReplaceAll(v, "\n", " ") + "\n"
		}
		return conductor.AtomicWrite(path, []byte(text))
	case a.Kind == "delivery" && r.Outcome == "completed":
		pairs, e := parseConductorPairs(text)
		if e != nil {
			return e
		}
		for _, p := range pairs {
			if !p.done {
				return errors.New("delivery cannot complete unfinished pairs")
			}
		}
		text += "\n<!-- conductor delivery " + a.ID + " -->\n"
		text = conductorSetField(text, "phase", "complete")
		text = conductorSetField(text, "status", "completed")
		text = conductorSetField(text, "Finished", time.Now().UTC().Format(time.RFC3339))
		for _, v := range r.Evidence {
			text += "\n- Delivery evidence: " + strings.ReplaceAll(v, "\n", " ")
		}
		archive := filepath.Join(a.ProjectPath, ".claude", "iterate", "archive", time.Now().UTC().Format("2006-01-02T150405Z")+"-"+a.Plan+"-done.md")
		if e = conductor.AtomicWrite(archive, []byte(text)); e != nil {
			return e
		}
		return os.Remove(path)
	}
	return nil
}
func (ConductorPlanSource) Claim(ctx context.Context, a model.Assignment) error {
	path, e := safePlanPath(a.ProjectPath, a.Plan)
	if e != nil {
		return e
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	owner := regexp.MustCompile(`(?m)^conductor-assignment:[ \t]*(.*)$`).FindStringSubmatch(string(b))
	if len(owner) > 1 && strings.TrimSpace(owner[1]) != "" && strings.TrimSpace(owner[1]) != a.ID {
		return errors.New("another conductor assignment owns plan marker")
	}
	text := conductorSetField(string(b), "phase", "executing")
	text = conductorSetField(text, "status", "executing")
	if !regexp.MustCompile(`(?m)^Executing:`).MatchString(text) {
		text = conductorSetField(text, "Executing", time.Now().UTC().Format(time.RFC3339))
	}
	if e = conductor.AtomicWrite(path, []byte(text)); e != nil {
		return e
	}
	if e = conductor.AtomicWrite(filepath.Join(a.ProjectPath, ".claude", "iterate", "current"), []byte(a.Plan+"\n")); e != nil {
		return e
	}
	return conductorRunMarker(a, false)
}
func (ConductorPlanSource) Heartbeat(ctx context.Context, a model.Assignment) error {
	return conductorRunMarker(a, false)
}
func (ConductorPlanSource) Release(ctx context.Context, a model.Assignment) error {
	return conductorRunMarker(a, true)
}
func conductorRunMarker(a model.Assignment, release bool) error {
	path, e := safePlanPath(a.ProjectPath, a.Plan)
	if os.IsNotExist(e) && release {
		return conductorRecordTrail(a)
	}
	if e != nil {
		return e
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	text := string(b)
	owner := regexp.MustCompile(`(?m)^conductor-assignment:[ \t]*(.*)$`).FindStringSubmatch(text)
	if len(owner) > 1 && strings.TrimSpace(owner[1]) != "" && strings.TrimSpace(owner[1]) != a.ID {
		return errors.New("another conductor assignment owns plan marker")
	}
	value := time.Now().UTC().Format(time.RFC3339)
	if release {
		value = "false"
	}
	text = conductorSetField(text, "running", value)
	ownerID := a.ID
	if release {
		ownerID = ""
	}
	text = conductorSetField(text, "conductor-assignment", ownerID)
	if e = conductor.AtomicWrite(path, []byte(text)); e != nil {
		return e
	}
	if release {
		return conductorRecordTrail(a)
	}
	return nil
}

type conductorTeam struct {
	steps        []int
	dependencies []string
}

func conductorTeams(text string) map[string]conductorTeam {
	out := map[string]conductorTeam{}
	section := false
	stepsColumn, dependsColumn := 1, 2
	for _, line := range strings.Split(text, "\n") {
		if strings.HasPrefix(line, "## ") {
			section = strings.TrimSpace(line) == "## Teams"
			continue
		}
		if !section || !strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		cols := strings.Split(strings.Trim(strings.TrimSpace(line), "|"), "|")
		if len(cols) < 3 {
			continue
		}
		name := strings.Trim(strings.TrimSpace(cols[0]), "` ")
		if strings.EqualFold(name, "team") {
			dependsColumn = -1
			for i, col := range cols {
				switch strings.ToLower(strings.TrimSpace(col)) {
				case "steps":
					stepsColumn = i
				case "depends", "depends on", "depends-on":
					dependsColumn = i
				}
			}
			continue
		}
		if strings.Contains(name, "---") || stepsColumn >= len(cols) {
			continue
		}
		team := conductorTeam{}
		for _, piece := range strings.Split(strings.TrimSpace(cols[stepsColumn]), ",") {
			piece = strings.Trim(strings.TrimSpace(piece), "` ")
			bounds := strings.Split(piece, "-")
			lo, e := strconv.Atoi(bounds[0])
			if e != nil {
				continue
			}
			hi := lo
			if len(bounds) == 2 {
				hi, _ = strconv.Atoi(bounds[1])
			}
			for n := lo; n <= hi && n-lo < 1000; n++ {
				team.steps = append(team.steps, n)
			}
		}
		depends := ""
		if dependsColumn >= 0 && dependsColumn < len(cols) {
			depends = cols[dependsColumn]
		}
		for _, d := range strings.Split(strings.TrimSpace(depends), ",") {
			d = strings.Trim(strings.TrimSpace(d), "` ")
			if d != "" && d != "—" && d != "-" {
				team.dependencies = append(team.dependencies, d)
			}
		}
		out[name] = team
	}
	return out
}
func conductorOwnTrail(project string) bool {
	b, e := os.ReadFile(filepath.Join(project, ".claude", "iterate", "conductor-trail.json"))
	if e != nil {
		return false
	}
	var record struct {
		Heartbeat int64 `json:"heartbeat"`
	}
	if json.Unmarshal(b, &record) != nil || record.Heartbeat == 0 {
		return false
	}
	info, e := os.Stat(HeartbeatPath(project))
	return e == nil && info.ModTime().UnixNano() == record.Heartbeat
}

func conductorRecordTrail(a model.Assignment) error {
	var stamp int64
	if info, e := os.Stat(HeartbeatPath(a.ProjectPath)); e == nil {
		stamp = info.ModTime().UnixNano()
	}
	raw, _ := json.Marshal(map[string]any{"assignment": a.ID, "heartbeat": stamp})
	return conductor.AtomicWrite(filepath.Join(a.ProjectPath, ".claude", "iterate", "conductor-trail.json"), raw)
}
