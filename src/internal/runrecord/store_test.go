package runrecord

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

func i64(v int64) *int64 { return &v }

func testStore(t *testing.T) (*Store, string) {
	t.Helper()
	return Open(filepath.Join(t.TempDir(), "records")), t.TempDir()
}

func openRun(t *testing.T, s *Store, project, plan, marker string) string {
	t.Helper()
	id, _, err := s.Observe(OpenRequest{Key: PlanKey{ProjectPath: project, Plan: plan}, Marker: marker, Source: SourceCLI}, nil)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func ev(project, plan, run, id, typ string, at time.Time) Event {
	return Event{ID: id, Type: typ, At: at, Source: SourceCLI, ProjectPath: project, Plan: plan, RunID: run}
}

func runFile(t *testing.T, s *Store, project, plan, run string) string {
	t.Helper()
	return s.runPath(ProjectKey(CanonicalProject(project)), plan, run)
}

// TESTMASTER: id=runrecord-fold-order tier=fast parallel=yes
func TestFoldOrdersOutOfOrderEventsAndDropsDuplicates(t *testing.T) {
	s, project := testStore(t)
	run := openRun(t, s, project, "alpha", "m1")
	t0 := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	late := ev(project, "alpha", run, "b", TypeLifecycle, t0.Add(2*time.Minute))
	late.Lifecycle = &Lifecycle{State: "executing"}
	early := ev(project, "alpha", run, "a", TypeLifecycle, t0.Add(time.Minute))
	early.Lifecycle = &Lifecycle{State: "queued"}
	for _, e := range []Event{late, early, late, early} {
		if _, err := s.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	runs, err := s.Runs(project, "alpha")
	if err != nil || len(runs) != 1 {
		t.Fatal(runs, err)
	}
	r := runs[0]
	if r.Events != 3 || len(r.Lifecycle) != 2 || r.Lifecycle[0].State != "queued" || r.State != "executing" {
		t.Fatalf("fold: %+v", r)
	}
	// Duplicate lines that bypassed append (a concatenated copy) count once.
	path := runFile(t, s, project, "alpha", run)
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, append(data, data...), 0o600); err != nil {
		t.Fatal(err)
	}
	r, err = s.Find(run)
	if err != nil || r.Events != 3 || r.Duplicates != 3 || len(r.Lifecycle) != 2 {
		t.Fatalf("duplicate lines: %+v %v", r, err)
	}
}

// TESTMASTER: id=runrecord-torn-tail tier=fast parallel=yes
func TestTornTailIgnoredOnReadRepairedOnAppend(t *testing.T) {
	s, project := testStore(t)
	run := openRun(t, s, project, "alpha", "")
	path := runFile(t, s, project, "alpha", run)
	f, err := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"schema_version":1,"id":"torn","type":"run.lifecy`)
	f.Close()
	before, _ := os.ReadFile(path)
	r, err := s.Find(run)
	if err != nil || !r.Torn || r.Events != 1 || r.Unreadable != 0 {
		t.Fatalf("torn read: %+v %v", r, err)
	}
	if after, _ := os.ReadFile(path); !bytes.Equal(before, after) {
		t.Fatal("a read modified the run file")
	}
	e := ev(project, "alpha", run, "next", TypeLifecycle, time.Time{})
	e.Lifecycle = &Lifecycle{State: "executing"}
	if ok, err := s.Append(e); !ok || err != nil {
		t.Fatal(ok, err)
	}
	data, _ := os.ReadFile(path)
	if bytes.Contains(data, []byte("run.lifecy`")) || bytes.Contains(data, []byte(`"torn"`)) {
		t.Fatalf("torn fragment survived repair: %s", data)
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if !json.Valid([]byte(line)) {
			t.Fatalf("invalid line after repair: %q", line)
		}
	}
	r, _ = s.Find(run)
	if r.Torn || r.Events != 2 || r.State != "executing" {
		t.Fatalf("after repair: %+v", r)
	}
}

// TESTMASTER: id=runrecord-unterminated-line tier=fast parallel=yes
func TestUnterminatedCompleteLineIsKept(t *testing.T) {
	s, project := testStore(t)
	run := openRun(t, s, project, "alpha", "")
	path := runFile(t, s, project, "alpha", run)
	data, _ := os.ReadFile(path)
	if err := os.WriteFile(path, bytes.TrimRight(data, "\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	e := ev(project, "alpha", run, "next", TypeLifecycle, time.Time{})
	e.Lifecycle = &Lifecycle{State: "paused"}
	if ok, err := s.Append(e); !ok || err != nil {
		t.Fatal(ok, err)
	}
	r, _ := s.Find(run)
	if r.Events != 2 || r.Marker != "" || r.OpenedAt == nil {
		t.Fatalf("complete unterminated line lost: %+v", r)
	}
}

// TESTMASTER: id=runrecord-usage-idempotent tier=fast parallel=yes
func TestUsageIdempotentPerRequestAndScopesSeparate(t *testing.T) {
	s, project := testStore(t)
	run := openRun(t, s, project, "alpha", "")
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	add := func(id string, u Usage, offset time.Duration) {
		e := ev(project, "alpha", run, id, TypeUsage, at.Add(offset))
		e.Usage = &u
		if _, err := s.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	add("u1", Usage{RequestID: "r1", Scope: ScopeExact, Cumulative: true, InputTokens: i64(100), OutputTokens: i64(5)}, 0)
	add("u2", Usage{RequestID: "r1", Scope: ScopeExact, Cumulative: true, InputTokens: i64(140), OutputTokens: i64(9)}, time.Second)
	add("u2", Usage{RequestID: "r1", Scope: ScopeExact, InputTokens: i64(999)}, 2*time.Second) // replayed id: no-op
	add("u3", Usage{RequestID: "r2", Scope: ScopeExact, OutputTokens: i64(1)}, 3*time.Second)
	cost := 784.89
	add("u4", Usage{RequestID: "session", Scope: ScopeSessionCost, CostUSD: &cost}, 4*time.Second)
	r, _ := s.Find(run)
	exact := r.Usage.ByScope[ScopeExact]
	if exact.Requests != 2 || *exact.InputTokens != 140 || *exact.OutputTokens != 10 {
		t.Fatalf("exact totals: %+v", exact)
	}
	if exact.CostUSD != nil || !contains(exact.Partial, "input_tokens") || !contains(exact.Partial, "cost_usd") {
		t.Fatalf("unknown must stay unknown: %+v", exact)
	}
	session := r.Usage.ByScope[ScopeSessionCost]
	if session.InputTokens != nil || *session.CostUSD != cost {
		t.Fatalf("session cost mixed into tokens: %+v", session)
	}
	if r.Usage.Requests[0].Replaced != 1 {
		t.Fatalf("streaming rewrite not recorded as replacement: %+v", r.Usage.Requests[0])
	}
}

// TESTMASTER: id=runrecord-attempts-dedupe tier=fast parallel=yes
func TestAttemptsCountDistinctAttemptsOnly(t *testing.T) {
	s, project := testStore(t)
	run := openRun(t, s, project, "alpha", "")
	at := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	step := func(id, typ, key, outcome string, offset time.Duration) {
		e := ev(project, "alpha", run, id, typ, at.Add(offset))
		e.Requirement, e.AttemptKey = 3, key
		if outcome != "" {
			e.AttemptInfo = &Attempt{Outcome: outcome}
		}
		if _, err := s.Append(e); err != nil {
			t.Fatal(err)
		}
	}
	step("s1", TypeAttemptStarted, "k1", "", 0)
	step("s1-again", TypeAttemptStarted, "k1", "", time.Second) // same attempt, new event id
	step("f1", TypeAttemptFinished, "k1", "failed", time.Minute)
	step("s2", TypeAttemptStarted, "k2", "", 2*time.Minute)
	step("f2", TypeAttemptFinished, "k2", "passed", 3*time.Minute)
	ex := ev(project, "alpha", run, "e2", TypeEvidence, at.Add(3*time.Minute))
	ex.Requirement, ex.AttemptKey = 3, "k2"
	ex.Evidence = &Evidence{Result: "passed", Command: "go test ./...", ExitStatus: new(int), Revision: "abc123"}
	if _, err := s.Append(ex); err != nil {
		t.Fatal(err)
	}
	r, _ := s.Find(run)
	if len(r.Requirements) != 1 {
		t.Fatal(r.Requirements)
	}
	q := r.Requirements[0]
	if len(q.Attempts) != 2 || q.Attempts[0].Outcome != "failed" || q.Attempts[1].Outcome != "passed" || q.Attempts[1].Number != 2 {
		t.Fatalf("attempts: %+v", q.Attempts)
	}
	if *q.Attempts[0].DurationMS != time.Minute.Milliseconds() || len(q.Attempts[1].Evidence) != 1 || q.Attempts[1].Evidence[0].Attempt != 2 {
		t.Fatalf("attempt detail: %+v", q.Attempts)
	}
}

// TESTMASTER: id=runrecord-concurrent-writers tier=fast parallel=yes
func TestConcurrentWritersNeverInterleave(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "records")
	project := t.TempDir()
	run := openRun(t, Open(dir), project, "alpha", "")
	var wg sync.WaitGroup
	for w := 0; w < 8; w++ {
		wg.Add(1)
		go func(w int) {
			defer wg.Done()
			s := Open(dir) // a separate writer, as a second process would be
			for i := 0; i < 10; i++ {
				for _, id := range []string{fmt.Sprintf("w%d-%d", w, i), fmt.Sprintf("w%d-%d", (w+1)%8, i)} {
					e := ev(project, "alpha", run, id, TypeSessionObserved, time.Time{})
					e.SessionID = id
					if _, err := s.Append(e); err != nil {
						t.Error(err)
						return
					}
				}
			}
		}(w)
	}
	wg.Wait()
	r, err := Open(dir).Find(run)
	if err != nil || r.Events != 81 || r.Duplicates != 0 || r.Torn || r.Unreadable != 0 {
		t.Fatalf("concurrent append: events=%d dup=%d torn=%v unreadable=%d err=%v", r.Events, r.Duplicates, r.Torn, r.Unreadable, err)
	}
}

// TESTMASTER: id=runrecord-run-identity tier=fast parallel=yes
func TestRunIdentityAcrossArchiveAndRerun(t *testing.T) {
	s, project := testStore(t)
	first := openRun(t, s, project, "alpha", "2026-10-01T00:00:00Z")
	if again := openRun(t, s, project, "alpha", "2026-10-01T00:00:00Z"); again != first {
		t.Fatal("same execution opened a second run")
	}
	archived := ev(project, "alpha", first, "arch", TypeLifecycle, time.Time{})
	archived.Lifecycle = &Lifecycle{State: "archived", ArchiveFile: "20261001T010000Z-alpha-done.md"}
	if _, err := s.Append(archived); err != nil {
		t.Fatal(err)
	}
	rerun := openRun(t, s, project, "alpha", "2026-10-02T00:00:00Z")
	if rerun == first {
		t.Fatal("re-run reused the archived run")
	}
	// A late observation of an archived run's attempt stays with that run.
	if _, err := s.Append(Event{ID: "start", Type: TypeAttemptStarted, Source: SourceConductor, ProjectPath: project, Plan: "alpha", RunID: first, Requirement: 1, AttemptKey: "assignment-1"}); err != nil {
		t.Fatal(err)
	}
	late, _, err := s.Observe(OpenRequest{Key: PlanKey{ProjectPath: project, Plan: "alpha"}, Source: SourceConductor, AttemptKey: "assignment-1"}, func(Run) []Event {
		return []Event{{ID: "late", Type: TypeAttemptFinished, Requirement: 1, AttemptKey: "assignment-1", AttemptInfo: &Attempt{Outcome: "passed"}}}
	})
	if err != nil || late != first {
		t.Fatalf("late attempt went to %s, want %s (%v)", late, first, err)
	}
	// A marker change without an archive observation also opens a new run
	// and leaves the old one flagged superseded.
	third := openRun(t, s, project, "alpha", "2026-10-03T00:00:00Z")
	runs, _ := s.Runs(project, "alpha")
	if third == rerun || len(runs) != 3 || !runs[1].Superseded || runs[0].Superseded || runs[2].Superseded {
		t.Fatalf("runs: %+v", runs)
	}
}

// TESTMASTER: id=runrecord-reads-pure tier=fast parallel=yes
func TestReadsNeverCreateState(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "records")
	s := Open(dir)
	if runs, err := s.List("", ""); err != nil || len(runs) != 0 {
		t.Fatal(runs, err)
	}
	if _, err := s.Runs(t.TempDir(), "alpha"); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Find("run-x"); err != ErrNotFound {
		t.Fatal(err)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatal("a read created the record directory")
	}
}

// TESTMASTER: id=runrecord-validation tier=fast parallel=yes
func TestAppendRejectsInvalidEvents(t *testing.T) {
	s, project := testStore(t)
	run := openRun(t, s, project, "alpha", "")
	bad := []Event{
		{Type: "nonsense"},
		{Type: TypeLifecycle, Lifecycle: &Lifecycle{State: "sleeping"}},
		{Type: TypeUsage, Usage: &Usage{RequestID: "r", Scope: "total"}},
		{Type: TypeEvidence, Evidence: &Evidence{Result: "green"}},
		{Type: TypeAttemptStarted, Requirement: 1},
		{Type: TypeLifecycle, SchemaVersion: SchemaVersion + 1, Lifecycle: &Lifecycle{State: "paused"}},
	}
	for _, e := range bad {
		e.Source, e.ProjectPath, e.Plan, e.RunID = SourceCLI, project, "alpha", run
		if ok, err := s.Append(e); ok || err == nil {
			t.Fatalf("accepted invalid event %+v", e)
		}
	}
	if _, err := s.Append(Event{Type: TypeLifecycle, Source: SourceCLI, ProjectPath: project, Plan: "../escape", RunID: run, Lifecycle: &Lifecycle{State: "paused"}}); err == nil {
		t.Fatal("accepted a path-escaping plan name")
	}
}

// TESTMASTER: id=runrecord-batch-append tier=fast parallel=yes
func TestAppendAllValidatesWholeBatchFirst(t *testing.T) {
	s, project := testStore(t)
	run := openRun(t, s, project, "alpha", "")
	good := ev(project, "alpha", run, "g1", TypeLifecycle, time.Time{})
	good.Lifecycle = &Lifecycle{State: "executing"}
	bad := ev(project, "alpha", run, "b1", TypeLifecycle, time.Time{})
	bad.Lifecycle = &Lifecycle{State: "nope"}
	if n, err := s.AppendAll([]Event{good, bad}); n != 0 || err == nil {
		t.Fatalf("partial batch written: %d %v", n, err)
	}
	other := ev(project, "beta", "run-other", "g1", TypeLifecycle, time.Time{})
	other.Lifecycle = &Lifecycle{State: "queued"}
	if n, err := s.AppendAll([]Event{good, good, other}); n != 2 || err != nil {
		t.Fatalf("batch: %d %v", n, err)
	}
	if r, _ := s.Find(run); r.Events != 2 || r.Duplicates != 0 {
		t.Fatalf("run after batch: %+v", r)
	}
	if r, _ := s.Find("run-other"); r.Events != 1 || r.Plan != "beta" {
		t.Fatalf("second run in batch: %+v", r)
	}
}

// TESTMASTER: id=runrecord-unknowns tier=fast parallel=yes
func TestUnknownsListedNeverZero(t *testing.T) {
	s, project := testStore(t)
	run := openRun(t, s, project, "alpha", "")
	r, _ := s.Find(run)
	for _, want := range []string{"versions.engine", "versions.model", "usage", "evidence", "ownership", "coverage"} {
		if !contains(r.Unknown, want) {
			t.Fatalf("missing unknown %s in %v", want, r.Unknown)
		}
	}
	if len(r.Usage.ByScope) != 0 {
		t.Fatal("usage invented for a run with no usage")
	}
}

// TESTMASTER: id=runrecord-legacy-provenance tier=fast parallel=yes
func TestLegacyEventsCarryProvenanceAndStableIDs(t *testing.T) {
	exec := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	fin := exec.Add(3 * time.Hour)
	src := LegacySources{
		Plan: LegacyPlan{ProjectPath: t.TempDir(), Plan: "gemsbok", SourceFile: "a.md", ArchiveFile: "20260901T130500Z-gemsbok-done.md",
			Phase: "complete", Harness: "unknown", Executing: &exec, Finished: &fin, ModTime: fin.Add(time.Minute),
			Steps:    []LegacyStep{{Number: 1, Task: "t", Validation: "v", TaskChecked: true, ValidationChecked: true}, {Number: 2, Task: "t2", Validation: "v2"}},
			Evidence: []LegacyEvidence{{Text: "24 PDFs verified"}}},
		HooksRead: true, Hooks: []LegacyHookEvent{{At: exec.Add(time.Minute), SessionID: "s1"}, {At: exec.Add(time.Hour), SessionID: "s1", AgentID: "a1", Team: "app"}},
		ConductorRead: true,
	}
	a, b := LegacyEvents(src), LegacyEvents(src)
	if len(a) != len(b) {
		t.Fatal("non-deterministic legacy events")
	}
	for i := range a {
		if a[i].ID != b[i].ID || !strings.HasPrefix(a[i].Provenance, "legacy:") {
			t.Fatalf("event %d: %+v", i, a[i])
		}
	}
	r := Legacy(src)
	if !strings.HasPrefix(r.Provenance, "legacy:") || !r.Archived || r.State != "archived" || len(r.Requirements) != 3 {
		t.Fatalf("legacy view: %+v", r)
	}
	if r.Versions.Engine != "" || !contains(r.Unknown, "versions.engine") || !contains(r.Unknown, "usage") {
		t.Fatalf("legacy unknowns: %+v %v", r.Versions, r.Unknown)
	}
	for _, q := range r.Requirements {
		for _, e := range q.Evidence {
			if !e.Imported || e.Result != "unknown" {
				t.Fatalf("prose evidence not marked imported: %+v", e)
			}
		}
	}
	if len(r.Sessions) != 1 || len(r.Agents) != 1 || r.Sessions[0].LastSeen != exec.Add(time.Hour) {
		t.Fatalf("hook sessions: %+v %+v", r.Sessions, r.Agents)
	}
}
