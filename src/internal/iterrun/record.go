package iterrun

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// BuildVersion is the iterate-run version stamped into records it opens.
// main sets it from its -ldflags version.
var BuildVersion = "dev"

// RecordDir is the run record store: $ITERATE_RUN_RECORD_DIR when set,
// otherwise <StoreDir>/records.
func RecordDir() string {
	if d := os.Getenv("ITERATE_RUN_RECORD_DIR"); d != "" {
		return d
	}
	return filepath.Join(StoreDir(), "records")
}

var recordStores sync.Map // dir -> *runrecord.Store

// RecordStore returns this process's handle on the run record store, one
// per directory so its accepted-id index stays warm. Opening creates
// nothing.
func RecordStore() *runrecord.Store {
	dir := RecordDir()
	if s, ok := recordStores.Load(dir); ok {
		return s.(*runrecord.Store)
	}
	s, _ := recordStores.LoadOrStore(dir, runrecord.Open(dir))
	return s.(*runrecord.Store)
}

var (
	reRecordExecutorVersion = regexp.MustCompile(`(?m)^executor-version:\s*(\S+)`)
	reRecordPlannerVersion  = regexp.MustCompile(`(?m)^planner-version:\s*(\S+)`)
	reRecordEvidence        = regexp.MustCompile(`^\s*-\s*(?:Validation|Delivery) evidence:\s*(.+)$`)
	reArchiveStamp          = regexp.MustCompile(`^(\d{8}T\d{6}Z|\d{4}-\d{2}-\d{2}T\d{6}Z)-`)
)

// recordMarker normalizes a plan's Executing: value so the CLI, the
// Conductor and the legacy adapter name one execution identically.
func recordMarker(ps PlanSummary) string {
	if t, ok := ps.ExecutingAt(); ok {
		return t.UTC().Format(time.RFC3339)
	}
	return strings.TrimSpace(ps.Executing)
}

func recordEngine(harness string) string {
	switch harness {
	case "claude-code", "codex":
		return harness
	case "claude":
		return "claude-code"
	}
	return ""
}

// recordVersions reads the version markers of a plan file.
func recordVersions(path string, ps PlanSummary) *runrecord.Versions {
	v := &runrecord.Versions{Engine: recordEngine(ps.Harness), IterateRun: BuildVersion}
	if b, err := os.ReadFile(path); err == nil {
		if m := reRecordExecutorVersion.FindSubmatch(b); m != nil {
			v.ExecutorVersion = string(m[1])
		}
		if m := reRecordPlannerVersion.FindSubmatch(b); m != nil {
			v.PlannerVersion = string(m[1])
		}
	}
	return v
}

// recordLifecycleState maps a plan's state label to a lifecycle state, or
// "" for a label that is not one (planned).
func recordLifecycleState(ps PlanSummary) string {
	if ps.Phase == "complete" || ps.Phase == "done" {
		return "completed"
	}
	switch s := ps.StateLabel(); s {
	case "executing", "paused", "blocked", "unblocked", "queued":
		return s
	}
	return ""
}

// observeRecordHook links a hook's session and agent to the plan run they
// worked on, and records the plan file's state when it differs from the
// run's latest lifecycle state. Only an executing plan whose file lives in
// the hook's project is observed, so drafting activity opens no run and a
// same-named plan in another project is never touched. Best-effort: a
// record failure never affects the tool call.
func observeRecordHook(root, plan, team string, in hookInput) {
	if plan == "" || !runrecord.ValidName(plan) {
		return
	}
	path := filepath.Join(root, ".claude", "iterate", "plans", plan+".md")
	ps, err := parsePlanFile(path, root)
	if err != nil || (ps.Phase != "executing" && ps.Executing == "") || recordLifecycleState(ps) == "completed" {
		return
	}
	now := time.Now().UTC()
	bucket := now.Truncate(5 * time.Minute).Format(time.RFC3339)
	req := runrecord.OpenRequest{
		Key:    runrecord.PlanKey{ProjectPath: root, Plan: plan},
		Marker: recordMarker(ps), Source: runrecord.SourceHook,
		Versions: recordVersions(path, ps), At: now,
	}
	_, _, _ = RecordStore().Observe(req, func(run runrecord.Run) []runrecord.Event {
		var out []runrecord.Event
		if in.SessionID != "" {
			out = append(out, runrecord.Event{
				Type: runrecord.TypeSessionObserved, ID: runrecord.StableID("hook-session", run.RunID, in.SessionID, bucket),
				At: now, SessionID: in.SessionID, AgentID: in.AgentID,
				Versions: &runrecord.Versions{Engine: recordEngine(ps.Harness)},
			})
		}
		if in.AgentID != "" {
			out = append(out, runrecord.Event{
				Type: runrecord.TypeAgentObserved, ID: runrecord.StableID("hook-agent", run.RunID, in.AgentID, bucket),
				At: now, SessionID: in.SessionID, AgentID: in.AgentID, Team: team,
			})
		}
		if state := recordLifecycleState(ps); state != "" && state != run.State {
			previous := ""
			if n := len(run.Lifecycle); n > 0 {
				previous = run.Lifecycle[n-1].EventID
			}
			out = append(out, runrecord.Event{
				Type: runrecord.TypeLifecycle, ID: runrecord.StableID("hook-lifecycle", run.RunID, previous, state),
				At: now, Lifecycle: &runrecord.Lifecycle{State: state, Reason: ps.Status, ObservedVia: "plan-file"},
			})
		}
		return out
	})
}

// RecordAssignment mirrors Conductor assignment lifecycle into the run
// record. The Conductor's leases and checkpoints stay authoritative; the
// record observes them.
func (ConductorPlanSource) RecordAssignment(phase string, a model.Assignment, result *model.Result, ev *model.Event) {
	if !runrecord.ValidName(a.Plan) || a.ProjectPath == "" {
		return
	}
	engine := recordEngine(a.Subscription.Adapter)
	if engine == "" {
		engine = a.Subscription.Adapter
	}
	now := time.Now().UTC()
	base := runrecord.Event{
		Source: runrecord.SourceConductor, At: now, ProjectID: a.ProjectID,
		Requirement: a.Step, AttemptKey: a.ID, AssignmentID: a.ID, ConductorRunID: a.RunID, Team: a.Team,
	}
	id := func(parts ...string) string {
		return runrecord.StableID("conductor", append([]string{a.ID}, parts...)...)
	}
	var events []runrecord.Event
	push := func(e runrecord.Event) { events = append(events, e) }
	owner := &runrecord.Ownership{OwnerKind: "conductor", OwnerID: a.ID, PID: os.Getpid()}
	switch phase {
	case conductor.RecordAdmitted:
		e := base
		e.Type, e.ID = runrecord.TypeLifecycle, id("admitted")
		e.Lifecycle = &runrecord.Lifecycle{State: "admitted", ObservedVia: "lease", Reason: a.Kind + " assignment " + a.ID}
		push(e)
		e = base
		e.Type, e.ID, e.Ownership = runrecord.TypeOwnershipAcquired, id("acquired"), owner
		push(e)
	case conductor.RecordStarted:
		e := base
		e.Type, e.ID = runrecord.TypeAttemptStarted, id("started")
		e.AttemptInfo = &runrecord.Attempt{Kind: a.Kind, Engine: engine, Model: a.Sequence.Model}
		push(e)
		e = base
		e.Type, e.ID = runrecord.TypeLifecycle, id("executing")
		e.Lifecycle = &runrecord.Lifecycle{State: "executing", ObservedVia: "lease"}
		push(e)
	case conductor.RecordEvent:
		if ev == nil {
			return
		}
		switch {
		case ev.Kind == "session" && ev.SessionID != "":
			e := base
			e.Type, e.ID, e.SessionID = runrecord.TypeSessionObserved, id("session", ev.SessionID), ev.SessionID
			e.Versions = &runrecord.Versions{Engine: engine, Model: a.Sequence.Model}
			push(e)
		case (ev.Kind == "terminal" || ev.Kind == "usage") && (ev.InputTokens > 0 || ev.OutputTokens > 0):
			in, out := ev.InputTokens, ev.OutputTokens
			e := base
			e.Type, e.ID, e.SessionID = runrecord.TypeUsage, id("usage", fmt.Sprint(ev.Sequence), ev.ProviderID), ev.SessionID
			e.Usage = &runrecord.Usage{
				RequestID: a.ID + ":" + ev.SessionID, Scope: runrecord.ScopeExact, Cumulative: ev.Cumulative,
				Model: a.Sequence.Model, InputTokens: &in, OutputTokens: &out,
				Note: "conductor stream total; input includes cache read and creation tokens",
			}
			push(e)
		default:
			return
		}
	case conductor.RecordFinished:
		if result == nil {
			return
		}
		outcome := result.Outcome
		if outcome == "completed" {
			outcome = "passed"
		}
		e := base
		e.Type, e.ID = runrecord.TypeAttemptFinished, id("finished")
		e.AttemptInfo = &runrecord.Attempt{Kind: a.Kind, Outcome: outcome, Reason: result.Blocker, Engine: engine, Model: a.Sequence.Model}
		push(e)
		revision := gitHead(a.ProjectPath)
		for i, text := range result.Evidence {
			r := "unknown"
			if result.Outcome == "completed" && result.ValidationMet {
				r = "passed"
			}
			e = base
			e.Type, e.ID = runrecord.TypeEvidence, id("evidence", fmt.Sprint(i))
			e.Evidence = &runrecord.Evidence{Result: r, Summary: text, Revision: revision, Imported: true, ArtifactPath: a.ResultPath}
			push(e)
		}
		if result.Outcome == "completed" && a.Kind == "pair" && a.Step > 0 {
			yes := true
			e = base
			e.Type, e.ID = runrecord.TypeRequirement, id("requirement")
			e.RequirementState = &runrecord.RequirementState{Task: a.Task, Validation: a.Validation, TaskChecked: &yes, ValidationChecked: &yes}
			push(e)
		}
		if result.Outcome == "blocked" || result.Outcome == "partial" {
			e = base
			e.Type, e.ID = runrecord.TypeBlockerDetected, id("blocker")
			e.Blocker = &runrecord.Blocker{Key: a.ID, Kind: "operator", Reason: result.Blocker}
			push(e)
			e = base
			e.Type, e.ID = runrecord.TypeLifecycle, id("blocked")
			e.Lifecycle = &runrecord.Lifecycle{State: "blocked", Reason: result.Blocker, ObservedVia: "lease"}
			push(e)
		}
		if result.Outcome == "interrupted" {
			e = base
			e.Type, e.ID = runrecord.TypeLifecycle, id("interrupted")
			e.Lifecycle = &runrecord.Lifecycle{State: "interrupted", Reason: result.Blocker, ObservedVia: "lease"}
			push(e)
		}
		if a.Kind == "delivery" && result.Outcome == "completed" {
			if archive := conductorDeliveryArchive(a); archive != "" {
				e = base
				e.Type, e.ID = runrecord.TypeLifecycle, id("completed")
				e.Lifecycle = &runrecord.Lifecycle{State: "completed", ObservedVia: "lease"}
				push(e)
				// The archive move follows completion; one microsecond keeps
				// that order under the (time, id) fold.
				e = base
				e.At = now.Add(time.Microsecond)
				e.Type, e.ID = runrecord.TypeLifecycle, id("archived")
				e.Lifecycle = &runrecord.Lifecycle{State: "archived", ArchiveFile: archive, ObservedVia: "lease"}
				push(e)
			}
		}
		e = base
		e.Type, e.ID, e.Ownership = runrecord.TypeOwnershipReleased, id("released"), owner
		e.Ownership.Reason = result.Outcome
		push(e)
	default:
		return
	}
	var marker string
	var versions *runrecord.Versions
	if path, err := safePlanPath(a.ProjectPath, a.Plan); err == nil {
		if ps, err := parsePlanFile(path, a.ProjectPath); err == nil {
			marker = recordMarker(ps)
			versions = recordVersions(path, ps)
		}
	}
	if versions == nil {
		versions = &runrecord.Versions{IterateRun: BuildVersion}
	}
	req := runrecord.OpenRequest{
		Key:    runrecord.PlanKey{ProjectPath: a.ProjectPath, ProjectID: a.ProjectID, Plan: a.Plan},
		Marker: marker, Source: runrecord.SourceConductor, Versions: versions, At: now, AttemptKey: a.ID,
	}
	_, _, _ = RecordStore().Observe(req, func(runrecord.Run) []runrecord.Event { return events })
}

// conductorDeliveryArchive finds the archive file a delivery assignment
// wrote.
func conductorDeliveryArchive(a model.Assignment) string {
	files, _ := filepath.Glob(filepath.Join(a.ProjectPath, ".claude", "iterate", "archive", "*-"+a.Plan+"-done.md"))
	for _, f := range files {
		if b, err := os.ReadFile(f); err == nil && strings.Contains(string(b), "<!-- conductor delivery "+a.ID+" -->") {
			return filepath.Base(f)
		}
	}
	return ""
}

// gitHead reads the checked-out revision without running git.
func gitHead(project string) string {
	gitPath := filepath.Join(project, ".git")
	if b, err := os.ReadFile(gitPath); err == nil {
		// A worktree's .git is a file naming its git dir.
		if dir, ok := strings.CutPrefix(strings.TrimSpace(string(b)), "gitdir: "); ok {
			if !filepath.IsAbs(dir) {
				dir = filepath.Join(project, dir)
			}
			gitPath = dir
		}
	}
	head, err := os.ReadFile(filepath.Join(gitPath, "HEAD"))
	if err != nil {
		return ""
	}
	ref, ok := strings.CutPrefix(strings.TrimSpace(string(head)), "ref: ")
	if !ok {
		return strings.TrimSpace(string(head))
	}
	for _, dir := range []string{gitPath, commonGitDir(gitPath)} {
		if dir == "" {
			continue
		}
		if b, err := os.ReadFile(filepath.Join(dir, ref)); err == nil {
			return strings.TrimSpace(string(b))
		}
		if f, err := os.Open(filepath.Join(dir, "packed-refs")); err == nil {
			sc := bufio.NewScanner(f)
			for sc.Scan() {
				if sha, name, ok := strings.Cut(sc.Text(), " "); ok && name == ref {
					f.Close()
					return sha
				}
			}
			f.Close()
		}
	}
	return ""
}

func commonGitDir(gitDir string) string {
	b, err := os.ReadFile(filepath.Join(gitDir, "commondir"))
	if err != nil {
		return ""
	}
	dir := strings.TrimSpace(string(b))
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(gitDir, dir)
	}
	return dir
}

// RecordReport is what a report reads for one plan: recorded runs, or the
// read-only legacy view when the plan has no record.
type RecordReport struct {
	SchemaVersion int    `json:"schema_version"`
	ProjectPath   string `json:"project_path"`
	Plan          string `json:"plan"`
	ArchiveFile   string `json:"archive_file,omitempty"`
	// Source is "record" for durable runs and "legacy" for the adapter.
	Source string          `json:"source"`
	Runs   []runrecord.Run `json:"runs"`
}

// ErrRecordNotFound reports a plan with neither a record nor a legacy
// source.
var ErrRecordNotFound = runrecord.ErrNotFound

// RunSummary is one line of a run listing.
type RunSummary struct {
	RunID       string     `json:"run_id"`
	ProjectPath string     `json:"project_path"`
	ProjectKey  string     `json:"project_key"`
	Plan        string     `json:"plan"`
	State       string     `json:"state"`
	Archived    bool       `json:"archived"`
	ArchiveFile string     `json:"archive_file,omitempty"`
	Superseded  bool       `json:"superseded,omitempty"`
	OpenedAt    *time.Time `json:"opened_at,omitempty"`
	LastAt      *time.Time `json:"last_event_at,omitempty"`
	Provenance  string     `json:"provenance"`
	Events      int        `json:"events"`
	Torn        bool       `json:"torn,omitempty"`
}

// SummarizeRun reduces a run to its listing line.
func SummarizeRun(r runrecord.Run) RunSummary {
	return RunSummary{RunID: r.RunID, ProjectPath: r.ProjectPath, ProjectKey: r.ProjectKey, Plan: r.Plan, State: r.State,
		Archived: r.Archived, ArchiveFile: r.ArchiveFile, Superseded: r.Superseded, OpenedAt: r.OpenedAt, LastAt: r.LastAt,
		Provenance: r.Provenance, Events: r.Events, Torn: r.Torn}
}

// ListRecordRuns lists recorded runs. It never writes.
func ListRecordRuns(project, plan string) ([]RunSummary, error) {
	runs, err := RecordStore().List(project, plan)
	out := []RunSummary{}
	for _, r := range runs {
		out = append(out, SummarizeRun(r))
	}
	return out, err
}

// FindRecordRun returns one recorded run by id. It never writes.
func FindRecordRun(runID string) (runrecord.Run, error) { return RecordStore().Find(runID) }

// legacyTarget locates the plan file a legacy view reads: the named
// archive, the live plan, or the newest archive of the codename.
func legacyTarget(project, plan, archiveFile string) (PlanSummary, string, error) {
	if archiveFile != "" {
		if filepath.Base(archiveFile) != archiveFile || !strings.HasSuffix(archiveFile, ".md") {
			return PlanSummary{}, "", fmt.Errorf("archive %q is not an archive filename", archiveFile)
		}
		path := filepath.Join(project, ".claude", "iterate", "archive", archiveFile)
		ps, err := GetArchivedPlanSummary(project, archiveFile)
		if err != nil {
			return ps, "", ErrRecordNotFound
		}
		if plan != "" && ps.Name != plan {
			return ps, "", fmt.Errorf("archive %s holds plan %q, not %q", archiveFile, ps.Name, plan)
		}
		return ps, path, nil
	}
	path := filepath.Join(project, ".claude", "iterate", "plans", plan+".md")
	if ps, err := parsePlanFile(path, project); err == nil {
		return ps, path, nil
	}
	archived, _ := ListArchivedPlans(project)
	for _, a := range archived {
		if a.Name == plan {
			return a, filepath.Join(project, ".claude", "iterate", "archive", a.ArchiveFile), nil
		}
	}
	return PlanSummary{}, "", ErrRecordNotFound
}

// legacyPlan reads what a plan or archive file states.
func legacyPlan(project, projectID string, ps PlanSummary, path string) runrecord.LegacyPlan {
	lp := runrecord.LegacyPlan{
		ProjectPath: project, ProjectID: projectID, Plan: ps.Name, SourceFile: path, ArchiveFile: ps.ArchiveFile,
		Phase: ps.Phase, State: ps.StateLabel(), Status: ps.Status, Harness: ps.Harness,
	}
	if v := recordVersions(path, ps); v != nil {
		lp.ExecutorVersion, lp.PlannerVersion = v.ExecutorVersion, v.PlannerVersion
	}
	if t, ok := ps.StartedAt(); ok {
		lp.Started = &t
	}
	if t, ok := ps.ExecutingAt(); ok {
		lp.Executing = &t
	}
	if t, ok := ps.FinishedAt(); ok {
		lp.Finished = &t
	}
	if m := reArchiveStamp.FindStringSubmatch(ps.ArchiveFile); m != nil {
		for _, layout := range []string{"20060102T150405Z", "2006-01-02T150405Z"} {
			if t, err := time.Parse(layout, m[1]); err == nil {
				lp.ArchivedAt = &t
				break
			}
		}
	}
	if info, err := os.Stat(path); err == nil {
		lp.ModTime = info.ModTime().UTC()
	}
	if b, err := os.ReadFile(path); err == nil {
		section := ""
		steps := map[int]*runrecord.LegacyStep{}
		var order []int
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "## ") {
				section = strings.TrimSpace(strings.TrimPrefix(line, "## "))
				continue
			}
			if m := reRecordEvidence.FindStringSubmatch(line); m != nil {
				lp.Evidence = append(lp.Evidence, runrecord.LegacyEvidence{Text: strings.TrimSpace(m[1])})
				continue
			}
			if section != "Steps" && section != "Validation" {
				continue
			}
			item, ok := parsePlanItem(strings.TrimSpace(line))
			if !ok || !item.Boxed {
				continue
			}
			s, ok := steps[item.Num]
			if !ok {
				s = &runrecord.LegacyStep{Number: item.Num}
				steps[item.Num] = s
				order = append(order, item.Num)
			}
			if section == "Steps" {
				s.Task, s.TaskChecked = item.Text, item.Checked
			} else {
				s.Validation, s.ValidationChecked = item.Text, item.Checked
			}
		}
		for _, n := range order {
			lp.Steps = append(lp.Steps, *steps[n])
		}
	}
	return lp
}

// legacySources gathers what the read-only adapter reads. It never writes:
// the hook log and Conductor run directories are opened read-only.
func legacySources(project, projectID string, ps PlanSummary, path string) runrecord.LegacySources {
	lp := legacyPlan(project, projectID, ps, path)
	src := runrecord.LegacySources{Plan: lp}
	start, end := lp.Executing, lp.Finished
	if start == nil {
		start = lp.Started
	}
	if end == nil {
		end = lp.ArchivedAt
	}
	within := func(t time.Time) bool {
		return (start == nil || !t.Before(*start)) && (end == nil || !t.After(*end))
	}
	canonical := runrecord.CanonicalProject(project)

	if events, err := ReadEvents(); err == nil {
		src.HooksRead = true
		src.HookNote = "attributed by plan name, project directory and execution window; activity in other directories is not attributed"
		roots := map[string]bool{}
		for _, e := range events {
			if e.Plan != ps.Name || !within(e.TS) {
				continue
			}
			in, seen := roots[e.CWD]
			if !seen {
				in = runrecord.CanonicalProject(ProjectRoot(e.CWD)) == canonical
				roots[e.CWD] = in
			}
			if in {
				src.Hooks = append(src.Hooks, runrecord.LegacyHookEvent{At: e.TS, SessionID: e.SessionID, AgentID: e.AgentID, Team: e.Team})
			}
		}
	}

	src.ConductorRead = true
	runDirs, err := filepath.Glob(filepath.Join(conductor.DefaultDir(), "runs", "*"))
	if err != nil {
		src.ConductorRead = false
	}
	for _, dir := range runDirs {
		var lease conductor.Lease
		b, err := os.ReadFile(filepath.Join(dir, "run.json"))
		if err != nil || json.Unmarshal(b, &lease) != nil {
			continue
		}
		a := lease.Assignment
		if a.Plan != ps.Name || runrecord.CanonicalProject(a.ProjectPath) != canonical || !within(lease.Started) {
			continue
		}
		la := runrecord.LegacyAssignment{
			AssignmentID: a.ID, ConductorRunID: a.RunID, Kind: a.Kind, Step: a.Step, Team: a.Team,
			Engine: recordEngine(a.Subscription.Adapter), Model: a.Sequence.Model, Started: lease.Started,
		}
		if la.Engine == "" {
			la.Engine = a.Subscription.Adapter
		}
		resultPath := filepath.Join(dir, "validated-result.json")
		if b, err := os.ReadFile(resultPath); err == nil {
			var r model.Result
			if json.Unmarshal(b, &r) == nil {
				la.Outcome, la.Blocker, la.ValidationMet, la.Evidence = r.Outcome, r.Blocker, r.ValidationMet, r.Evidence
				if info, err := os.Stat(resultPath); err == nil {
					t := info.ModTime().UTC()
					la.Finished = &t
				}
			}
		}
		la.Usage = conductorRunUsage(filepath.Join(dir, "events.jsonl"))
		src.Assignments = append(src.Assignments, la)
	}
	return src
}

// conductorRunUsage returns each session's last cumulative total.
func conductorRunUsage(path string) []runrecord.LegacyUsage {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	last := map[string]runrecord.LegacyUsage{}
	var order []string
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 4096), 1<<20)
	for sc.Scan() {
		var ev model.Event
		if json.Unmarshal(sc.Bytes(), &ev) != nil || !ev.Cumulative || (ev.InputTokens == 0 && ev.OutputTokens == 0) {
			continue
		}
		if _, ok := last[ev.SessionID]; !ok {
			order = append(order, ev.SessionID)
		}
		last[ev.SessionID] = runrecord.LegacyUsage{SessionID: ev.SessionID, At: ev.At, InputTokens: ev.InputTokens, OutputTokens: ev.OutputTokens}
	}
	var out []runrecord.LegacyUsage
	for _, s := range order {
		out = append(out, last[s])
	}
	return out
}

// ReadRecordReport reads one plan's runs. With archiveFile it selects the
// runs of that archived execution; otherwise the live plan's execution,
// or every run of the codename when no live plan file exists. A plan with
// no record gets the read-only legacy view. A read never writes.
func ReadRecordReport(project, plan, archiveFile string) (RecordReport, error) {
	if project == "" {
		return RecordReport{}, errors.New("project is required")
	}
	project = runrecord.CanonicalProject(project)
	rep := RecordReport{SchemaVersion: runrecord.SchemaVersion, ProjectPath: project, Plan: plan, ArchiveFile: archiveFile, Runs: []runrecord.Run{}}
	if archiveFile != "" && plan == "" {
		if ps, err := GetArchivedPlanSummary(project, archiveFile); err == nil {
			plan = ps.Name
			rep.Plan = plan
		}
	}
	if !runrecord.ValidName(plan) {
		return rep, fmt.Errorf("plan %q is not a valid codename", plan)
	}
	runs, err := RecordStore().Runs(project, plan)
	if err != nil {
		return rep, err
	}
	ps, path, targetErr := legacyTarget(project, plan, archiveFile)
	if targetErr != nil && !errors.Is(targetErr, ErrRecordNotFound) {
		return rep, targetErr
	}
	live := targetErr == nil && !ps.Archived
	marker := ""
	if targetErr == nil {
		marker = recordMarker(ps)
	}
	legacyID := ""
	if targetErr == nil {
		legacyID = runrecord.LegacyRunID(legacyPlan(project, "", ps, path))
	}
	for _, r := range runs {
		switch {
		case archiveFile != "":
			if r.ArchiveFile == archiveFile || r.RunID == legacyID || (r.ArchiveFile == "" && marker != "" && r.Marker == marker) {
				rep.Runs = append(rep.Runs, r)
			}
		case live:
			if !r.Archived && (r.Marker == "" || marker == "" || r.Marker == marker || r.RunID == legacyID) {
				rep.Runs = append(rep.Runs, r)
			}
		default:
			rep.Runs = append(rep.Runs, r)
		}
	}
	if len(rep.Runs) > 0 {
		rep.Source = "record"
		return rep, nil
	}
	if targetErr != nil {
		return rep, ErrRecordNotFound
	}
	rep.Source = "legacy"
	rep.ArchiveFile = ps.ArchiveFile
	rep.Runs = append(rep.Runs, runrecord.Legacy(legacySources(project, "", ps, path)))
	return rep, nil
}

// ImportLegacyRecord persists the legacy view of one plan execution with
// its provenance. Importing twice appends nothing. It refuses an execution
// that already has an observed record, so history is never duplicated.
func ImportLegacyRecord(project, plan, archiveFile string, dryRun bool) (runrecord.Run, int, error) {
	project = runrecord.CanonicalProject(project)
	ps, path, err := legacyTarget(project, plan, archiveFile)
	if err != nil {
		return runrecord.Run{}, 0, err
	}
	src := legacySources(project, "", ps, path)
	events := runrecord.LegacyEvents(src)
	runID := runrecord.LegacyRunID(src.Plan)
	existing, err := RecordStore().Runs(project, ps.Name)
	if err != nil {
		return runrecord.Run{}, 0, err
	}
	marker := recordMarker(ps)
	for _, r := range existing {
		if r.RunID == runID {
			continue
		}
		if (ps.ArchiveFile != "" && r.ArchiveFile == ps.ArchiveFile) || (marker != "" && r.Marker == marker) {
			return runrecord.Run{}, 0, fmt.Errorf("plan %s execution %s already has run %s in the record", ps.Name, runrecord.LegacyMarker(src.Plan), r.RunID)
		}
	}
	if dryRun {
		return runrecord.Legacy(src), 0, nil
	}
	accepted, err := RecordStore().AppendAll(events)
	if err != nil {
		return runrecord.Run{}, accepted, err
	}
	run, err := RecordStore().Find(runID)
	return run, accepted, err
}
