package runrecord

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// ErrNotFound reports that no record (and no legacy source) exists for the
// requested identity.
var ErrNotFound = errors.New("run record not found")

// Store is the record directory. Layout:
//
//	<Dir>/<project-key>/project.json        canonical project path
//	<Dir>/<project-key>/<plan>/.lock        writer lock for the plan
//	<Dir>/<project-key>/<plan>/<run-id>.jsonl
//
// The key is the canonical project path, never the plan file's location,
// so archiving a plan file moves nothing in the store.
type Store struct {
	Dir string
	Now func() time.Time

	mu    sync.Mutex
	index map[string]*idIndex
}

// Open returns a store rooted at dir. It creates nothing.
func Open(dir string) *Store { return &Store{Dir: dir, Now: time.Now} }

func (s *Store) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC()
	}
	return time.Now().UTC()
}

// CanonicalProject resolves a project path to its absolute, symlink-free
// form. A path that no longer exists keeps its cleaned absolute form, so a
// deleted project's history stays addressable.
func CanonicalProject(path string) string {
	if path == "" {
		return ""
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	if resolved, err := filepath.EvalSymlinks(abs); err == nil {
		return filepath.Clean(resolved)
	}
	return filepath.Clean(abs)
}

// ProjectKey is the stable directory key for a canonical project path.
func ProjectKey(canonical string) string {
	sum := sha256.Sum256([]byte(canonical))
	return hex.EncodeToString(sum[:8])
}

// ValidName reports whether s is safe as one path element.
func ValidName(s string) bool {
	return s != "" && s != "." && s != ".." && !strings.ContainsAny(s, "/\\\x00") && !strings.HasPrefix(s, ".") && len(s) <= 200
}

// NewRunID returns a fresh, sortable, globally unique run id.
func NewRunID(at time.Time) string {
	return "run-" + at.UTC().Format("20060102T150405Z") + "-" + randomHex(4)
}

// NewEventID returns a fresh, globally unique event id.
func NewEventID() string { return "ev-" + randomHex(12) }

// StableID derives a deterministic event id from its parts, so a writer that
// replays the same observation produces the same id and the append is a
// no-op.
func StableID(prefix string, parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		h.Write([]byte(p))
		h.Write([]byte{0})
	}
	return prefix + "-" + hex.EncodeToString(h.Sum(nil)[:12])
}

func randomHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("%x", time.Now().UnixNano())
	}
	return hex.EncodeToString(b)
}

func (s *Store) planDir(projectKey, plan string) string {
	return filepath.Join(s.Dir, projectKey, plan)
}

func (s *Store) runPath(projectKey, plan, runID string) string {
	return filepath.Join(s.planDir(projectKey, plan), runID+".jsonl")
}

// PlanKey identifies one plan codename inside one project.
type PlanKey struct {
	ProjectPath string
	ProjectID   string
	Plan        string
}

func (k PlanKey) canonical() (PlanKey, string, error) {
	if k.ProjectPath == "" {
		return k, "", errors.New("project path is required")
	}
	if !ValidName(k.Plan) {
		return k, "", fmt.Errorf("plan name %q is not a valid codename", k.Plan)
	}
	k.ProjectPath = CanonicalProject(k.ProjectPath)
	return k, ProjectKey(k.ProjectPath), nil
}

// normalize fills defaults and validates one event for append.
func (s *Store) normalize(e *Event) error {
	if e.SchemaVersion == 0 {
		e.SchemaVersion = SchemaVersion
	}
	if e.SchemaVersion > SchemaVersion {
		return fmt.Errorf("event schema_version %d is newer than supported %d", e.SchemaVersion, SchemaVersion)
	}
	if e.ProjectPath == "" {
		return errors.New("event project_path is required")
	}
	canonical := CanonicalProject(e.ProjectPath)
	key := ProjectKey(canonical)
	if e.ProjectKey != "" && e.ProjectKey != key {
		return fmt.Errorf("event project_key %q does not match project_path %q", e.ProjectKey, canonical)
	}
	e.ProjectPath, e.ProjectKey = canonical, key
	if !ValidName(e.Plan) {
		return fmt.Errorf("event plan %q is not a valid codename", e.Plan)
	}
	if !ValidName(e.RunID) {
		return fmt.Errorf("event run_id %q is not a valid run id", e.RunID)
	}
	if e.ID == "" {
		e.ID = NewEventID()
	}
	if len(e.ID) > 200 || strings.ContainsAny(e.ID, "\n\r\x00") {
		return errors.New("event id is invalid")
	}
	if e.Source == "" {
		return errors.New("event source is required")
	}
	if e.Provenance == "" {
		e.Provenance = "observed"
	}
	now := s.now()
	if e.At.IsZero() {
		e.At = now
	}
	e.At = e.At.UTC()
	e.RecordedAt = now
	return validatePayload(*e)
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// ValidatePayload checks an event's type and payload without identity.
func ValidatePayload(e Event) error { return validatePayload(e) }

func validatePayload(e Event) error {
	need := func(ok bool, what string) error {
		if !ok {
			return fmt.Errorf("%s event requires %s", e.Type, what)
		}
		return nil
	}
	switch e.Type {
	case TypeRunOpened, TypeSessionObserved, TypeAgentObserved:
		if e.Type == TypeSessionObserved {
			return need(e.SessionID != "", "session_id")
		}
		if e.Type == TypeAgentObserved {
			return need(e.AgentID != "", "agent_id")
		}
		return nil
	case TypeLifecycle:
		return need(e.Lifecycle != nil && contains(LifecycleStates, e.Lifecycle.State), "lifecycle.state in "+strings.Join(LifecycleStates, "|"))
	case TypeOwnershipAcquired, TypeOwnershipBeat, TypeOwnershipReleased:
		return need(e.Ownership != nil && (e.Ownership.OwnerKind == "cli" || e.Ownership.OwnerKind == "conductor") && e.Ownership.OwnerID != "", "ownership.owner_kind (cli|conductor) and ownership.owner_id")
	case TypeBlockerDetected, TypeBlockerCleared:
		return need(e.Blocker != nil, "blocker")
	case TypeAdmission:
		return need(e.Admission != nil && (e.Admission.Decision == "admitted" || e.Admission.Decision == "gated"), "admission.decision (admitted|gated)")
	case TypeRequirement:
		return need(e.Requirement > 0 && e.RequirementState != nil, "requirement > 0 and requirement_state")
	case TypeAttemptStarted, TypeAttemptFinished:
		if err := need(e.Requirement >= 0 && (e.Attempt > 0 || e.AttemptKey != ""), "requirement >= 0 (0 is plan-level) and attempt or attempt_key"); err != nil {
			return err
		}
		if e.Type == TypeAttemptFinished {
			return need(e.AttemptInfo != nil && e.AttemptInfo.Outcome != "", "attempt_info.outcome")
		}
		return nil
	case TypeEvidence:
		return need(e.Evidence != nil && (e.Evidence.Result == "passed" || e.Evidence.Result == "failed" || e.Evidence.Result == "unknown"), "evidence.result (passed|failed|unknown)")
	case TypeUsage:
		return need(e.Usage != nil && e.Usage.RequestID != "" && contains([]string{ScopeExact, ScopeEstimated, ScopeSessionCost, ScopeAllowance}, e.Usage.Scope), "usage.request_id and usage.scope (exact|estimated|session_cost|allowance)")
	case TypeInterval:
		return need(e.Interval != nil && contains(IntervalCauses, e.Interval.Cause) && !e.Interval.Start.IsZero() && !e.Interval.End.Before(e.Interval.Start), "interval.cause, start and end >= start")
	case TypeDelivery:
		return need(e.Delivery != nil && contains(DeliveryStages, e.Delivery.Stage), "delivery.stage in "+strings.Join(DeliveryStages, "|"))
	case TypeCoverage:
		return need(e.Coverage != nil && e.Coverage.Telemetry != "" && e.Coverage.Kind != "" && contains([]string{CoverageCaptured, CoverageMissing, CoveragePartial, CoverageUnavailable}, e.Coverage.State), "coverage.telemetry, kind and state")
	}
	return fmt.Errorf("unknown event type %q", e.Type)
}

// withPlanLock serializes writers for one plan across processes.
func (s *Store) withPlanLock(projectKey, plan, canonical string, fn func() error) error {
	dir := s.planDir(projectKey, plan)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	meta := filepath.Join(s.Dir, projectKey, "project.json")
	if _, err := os.Stat(meta); os.IsNotExist(err) {
		data, _ := json.Marshal(map[string]any{"schema_version": SchemaVersion, "project_path": canonical, "project_key": projectKey})
		_ = atomicWrite(meta, data)
	}
	f, err := os.OpenFile(filepath.Join(dir, ".lock"), os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	if err := lockFile(f); err != nil {
		return err
	}
	defer unlockFile(f)
	return fn()
}

func atomicWrite(path string, data []byte) error {
	tmp, err := os.CreateTemp(filepath.Dir(path), ".tmp-*")
	if err != nil {
		return err
	}
	name := tmp.Name()
	_, err = tmp.Write(data)
	if err == nil {
		err = tmp.Sync()
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(name, path)
	}
	if err != nil {
		_ = os.Remove(name)
	}
	return err
}

// repairTail makes the run file end at a line boundary. A torn final line
// from an interrupted write was never accepted, so it is dropped; a final
// line that parses but lacks its newline is completed.
func repairTail(path string) error {
	f, err := os.OpenFile(path, os.O_RDWR, 0o600)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return err
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() == 0 {
		return err
	}
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}
	if data[len(data)-1] == '\n' {
		return nil
	}
	cut := bytes.LastIndexByte(data, '\n') + 1
	var e Event
	if json.Unmarshal(bytes.TrimSpace(data[cut:]), &e) == nil && e.ID != "" {
		if _, err = f.WriteAt([]byte{'\n'}, int64(len(data))); err != nil {
			return err
		}
		return f.Sync()
	}
	if err = f.Truncate(int64(cut)); err != nil {
		return err
	}
	return f.Sync()
}

type idIndex struct {
	size int64
	ids  map[string]bool
}

// acceptedIDs returns the ids already accepted in a run file. A writer
// calls it under the plan lock after repairTail, so the file ends at a line
// boundary; only bytes appended since this store last looked are parsed.
// The returned map belongs to the cache and is only touched under the plan
// lock.
func (s *Store) acceptedIDs(path string) (map[string]bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.index == nil {
		s.index = map[string]*idIndex{}
	}
	idx := s.index[path]
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		idx = &idIndex{ids: map[string]bool{}}
		s.index[path] = idx
		return idx.ids, nil
	}
	if err != nil {
		return nil, err
	}
	if idx == nil || info.Size() < idx.size {
		idx = &idIndex{ids: map[string]bool{}}
	}
	if info.Size() > idx.size {
		f, err := os.Open(path)
		if err != nil {
			return nil, err
		}
		defer f.Close()
		if _, err := f.Seek(idx.size, io.SeekStart); err != nil {
			return nil, err
		}
		data, err := io.ReadAll(f)
		if err != nil {
			return nil, err
		}
		for _, line := range bytes.Split(data, []byte{'\n'}) {
			var e struct {
				ID string `json:"id"`
			}
			if json.Unmarshal(bytes.TrimSpace(line), &e) == nil && e.ID != "" {
				idx.ids[e.ID] = true
			}
		}
		idx.size += int64(len(data))
	}
	s.index[path] = idx
	return idx.ids, nil
}

// fileRead describes what a read skipped.
type fileRead struct {
	Torn       bool
	Unreadable int
}

// readFile parses a run file without modifying it. A torn trailing line is
// reported and skipped; any other unparseable or newer-schema line is
// counted as unreadable and skipped.
func readFile(path string) ([]Event, fileRead, error) {
	var info fileRead
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, info, nil
		}
		return nil, info, err
	}
	var out []Event
	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 64*1024), 8*1024*1024)
	lines := 0
	total := bytes.Count(data, []byte{'\n'})
	tornTail := len(data) > 0 && data[len(data)-1] != '\n'
	for sc.Scan() {
		lines++
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var e Event
		if json.Unmarshal(line, &e) != nil || e.ID == "" {
			if tornTail && lines == total+1 {
				info.Torn = true
			} else {
				info.Unreadable++
			}
			continue
		}
		if e.SchemaVersion > SchemaVersion {
			info.Unreadable++
			continue
		}
		out = append(out, e)
	}
	return out, info, sc.Err()
}

func appendLine(path string, e Event) error { return appendLines(path, []Event{e}) }

// appendLines writes whole lines in one append and syncs once. An
// interrupted write can tear only the final line, which the next writer
// repairs; every complete line before it stands.
func appendLines(path string, events []Event) error {
	if len(events) == 0 {
		return nil
	}
	var buf bytes.Buffer
	for _, e := range events {
		data, err := json.Marshal(e)
		if err != nil {
			return err
		}
		buf.Write(data)
		buf.WriteByte('\n')
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	if _, err = f.Write(buf.Bytes()); err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	return err
}

// freshEvents drops events whose id the run file or an earlier event in
// the batch already holds.
func freshEvents(ids map[string]bool, events []Event) []Event {
	var out []Event
	batch := map[string]bool{}
	for _, e := range events {
		if ids[e.ID] || batch[e.ID] {
			continue
		}
		batch[e.ID] = true
		out = append(out, e)
	}
	return out
}

// Append durably writes one event. It returns accepted=false, with no
// error, when an event with the same id is already recorded.
func (s *Store) Append(e Event) (bool, error) {
	n, err := s.AppendAll([]Event{e})
	return n == 1, err
}

// AppendAll writes events that may span several runs. Every event is
// validated before anything is written; each run's new events then land in
// one append under that plan's lock. Duplicates are skipped. It returns how
// many were accepted.
func (s *Store) AppendAll(events []Event) (int, error) {
	type group struct {
		projectKey, project, plan, run string
		events                         []Event
	}
	groups := map[string]*group{}
	var order []string
	for i := range events {
		e := events[i]
		if err := s.normalize(&e); err != nil {
			return 0, err
		}
		k := e.ProjectKey + "\x00" + e.Plan + "\x00" + e.RunID
		g, ok := groups[k]
		if !ok {
			g = &group{projectKey: e.ProjectKey, project: e.ProjectPath, plan: e.Plan, run: e.RunID}
			groups[k] = g
			order = append(order, k)
		}
		g.events = append(g.events, e)
	}
	accepted := 0
	for _, k := range order {
		g := groups[k]
		err := s.withPlanLock(g.projectKey, g.plan, g.project, func() error {
			path := s.runPath(g.projectKey, g.plan, g.run)
			if err := repairTail(path); err != nil {
				return err
			}
			ids, err := s.acceptedIDs(path)
			if err != nil {
				return err
			}
			fresh := freshEvents(ids, g.events)
			if err := appendLines(path, fresh); err != nil {
				return err
			}
			accepted += len(fresh)
			return nil
		})
		if err != nil {
			return accepted, err
		}
	}
	return accepted, nil
}

// OpenRequest describes how to resolve the run a writer appends to.
type OpenRequest struct {
	Key PlanKey
	// Marker identifies the plan execution (the plan file's Executing:
	// value). An open run with a different non-empty marker belongs to an
	// earlier execution, so a new run opens.
	Marker   string
	Source   string
	Versions *Versions
	At       time.Time
	// AttemptKey, when set, selects the run that already holds this
	// attempt, archived or not, so a late observation of an attempt (a
	// result recorded after delivery archived the plan) stays with it.
	AttemptKey string
	// NoOpen appends only to an existing run: with no matching run, Observe
	// writes nothing and returns an empty run id.
	NoOpen bool
}

// Observe resolves the open run for a plan (opening one when needed), folds
// it, and appends whatever events fn derives from that view, all under the
// plan's writer lock. It returns the run id and how many events were
// accepted. Event identity fields are filled from the run.
func (s *Store) Observe(req OpenRequest, fn func(Run) []Event) (string, int, error) {
	key, projectKey, err := req.Key.canonical()
	if err != nil {
		return "", 0, err
	}
	if req.NoOpen {
		if _, err := os.Stat(s.planDir(projectKey, key.Plan)); os.IsNotExist(err) {
			return "", 0, nil
		}
	}
	var runID string
	accepted := 0
	err = s.withPlanLock(projectKey, key.Plan, key.ProjectPath, func() error {
		runs, err := s.readPlanRuns(projectKey, key.Plan)
		if err != nil {
			return err
		}
		var open *Run
		if req.AttemptKey != "" {
			for i := len(runs) - 1; i >= 0 && open == nil; i-- {
				if runs[i].hasAttemptKey(req.AttemptKey) {
					open = &runs[i]
				}
			}
		}
		for i := len(runs) - 1; i >= 0 && open == nil; i-- {
			r := runs[i]
			if r.Archived {
				continue
			}
			if req.Marker != "" && r.Marker != "" && r.Marker != req.Marker {
				continue
			}
			open = &runs[i]
		}
		if open == nil && req.NoOpen {
			return nil
		}
		if open == nil {
			at := req.At
			if at.IsZero() {
				at = s.now()
			}
			id := NewRunID(at)
			opened := Event{
				Type: TypeRunOpened, ID: "open-" + id, At: at, Source: req.Source,
				ProjectPath: key.ProjectPath, ProjectID: key.ProjectID, Plan: key.Plan,
				RunID: id, Marker: req.Marker, Versions: req.Versions,
			}
			if err := s.normalize(&opened); err != nil {
				return err
			}
			if err := appendLine(s.runPath(projectKey, key.Plan, id), opened); err != nil {
				return err
			}
			fresh := Fold([]Event{opened}, fileRead{})
			open = &fresh
		}
		runID = open.RunID
		if fn == nil {
			return nil
		}
		path := s.runPath(projectKey, key.Plan, runID)
		if err := repairTail(path); err != nil {
			return err
		}
		ids, err := s.acceptedIDs(path)
		if err != nil {
			return err
		}
		derived := fn(*open)
		for i := range derived {
			e := &derived[i]
			e.ProjectPath, e.ProjectKey, e.Plan, e.RunID = key.ProjectPath, projectKey, key.Plan, runID
			if e.ProjectID == "" {
				e.ProjectID = key.ProjectID
			}
			if e.Source == "" {
				e.Source = req.Source
			}
			if err := s.normalize(e); err != nil {
				return err
			}
		}
		fresh := freshEvents(ids, derived)
		if err := appendLines(path, fresh); err != nil {
			return err
		}
		accepted = len(fresh)
		return nil
	})
	return runID, accepted, err
}

// readPlanRuns folds every run file of a plan, oldest first.
func (s *Store) readPlanRuns(projectKey, plan string) ([]Run, error) {
	files, err := filepath.Glob(filepath.Join(s.planDir(projectKey, plan), "*.jsonl"))
	if err != nil {
		return nil, err
	}
	var runs []Run
	for _, f := range files {
		events, info, err := readFile(f)
		if err != nil {
			return nil, err
		}
		if len(events) == 0 {
			continue
		}
		runs = append(runs, Fold(events, info))
	}
	sortRuns(runs)
	return runs, nil
}

func sortRuns(runs []Run) {
	sort.SliceStable(runs, func(i, j int) bool {
		a, b := runs[i].firstAt, runs[j].firstAt
		if !a.Equal(b) {
			return a.Before(b)
		}
		return runs[i].RunID < runs[j].RunID
	})
}

// markSuperseded flags open runs that a later run of the same plan
// replaced without an archive observation.
func markSuperseded(runs []Run) {
	for i := range runs {
		if !runs[i].Archived && i < len(runs)-1 {
			runs[i].Superseded = true
		}
	}
}

// Runs returns every recorded run of one plan in one project, oldest first.
// It never writes.
func (s *Store) Runs(projectPath, plan string) ([]Run, error) {
	key, projectKey, err := PlanKey{ProjectPath: projectPath, Plan: plan}.canonical()
	if err != nil {
		return nil, err
	}
	_ = key
	runs, err := s.readPlanRuns(projectKey, plan)
	if err != nil {
		return nil, err
	}
	markSuperseded(runs)
	return runs, nil
}

// List returns recorded runs, filtered by project and plan when given, in
// project, plan, then open-time order. It never writes.
func (s *Store) List(projectPath, plan string) ([]Run, error) {
	projectGlob := "*"
	if projectPath != "" {
		projectGlob = ProjectKey(CanonicalProject(projectPath))
	}
	planGlob := "*"
	if plan != "" {
		if !ValidName(plan) {
			return nil, fmt.Errorf("plan name %q is not a valid codename", plan)
		}
		planGlob = plan
	}
	dirs, err := filepath.Glob(filepath.Join(s.Dir, projectGlob, planGlob))
	if err != nil {
		return nil, err
	}
	sort.Strings(dirs)
	var out []Run
	for _, d := range dirs {
		if st, err := os.Stat(d); err != nil || !st.IsDir() {
			continue
		}
		runs, err := s.readPlanRuns(filepath.Base(filepath.Dir(d)), filepath.Base(d))
		if err != nil {
			return nil, err
		}
		markSuperseded(runs)
		out = append(out, runs...)
	}
	return out, nil
}

// Find returns one run by id. It never writes.
func (s *Store) Find(runID string) (Run, error) {
	if !ValidName(runID) {
		return Run{}, fmt.Errorf("run id %q is invalid", runID)
	}
	files, err := filepath.Glob(filepath.Join(s.Dir, "*", "*", runID+".jsonl"))
	if err != nil {
		return Run{}, err
	}
	if len(files) == 0 {
		return Run{}, ErrNotFound
	}
	plan := filepath.Dir(files[0])
	runs, err := s.readPlanRuns(filepath.Base(filepath.Dir(plan)), filepath.Base(plan))
	if err != nil {
		return Run{}, err
	}
	markSuperseded(runs)
	for _, r := range runs {
		if r.RunID == runID {
			return r, nil
		}
	}
	return Run{}, ErrNotFound
}
