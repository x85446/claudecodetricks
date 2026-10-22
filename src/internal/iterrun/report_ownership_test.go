package iterrun

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// Lifecycle report fixtures: an isolated HOME, record store and Conductor
// store, a canonical project with a current-plan pointer, and a bounded
// clock the record's observations use.

type lcClock struct{ now time.Time }

func lcUseClock(t *testing.T, start time.Time) *lcClock {
	t.Helper()
	c := &lcClock{now: start}
	prev := recordNow
	recordNow = func() time.Time { return c.now }
	t.Cleanup(func() { recordNow = prev })
	return c
}

func (c *lcClock) set(t time.Time) time.Time { c.now = t; return t }

func lcEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ITERATE_RUN_RECORD_DIR", t.TempDir())
	t.Setenv("ITERATE_CONDUCTOR_DIR", t.TempDir())
	t.Setenv("ITERATE_LIVE_SECS", "")
}

func lcProject(t *testing.T, plan string) string {
	t.Helper()
	dir, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, ".claude", "iterate", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".claude", "iterate", "current"), []byte(plan+"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	RegisterProject(dir)
	return dir
}

// lcHook delivers one PostToolUse hook from a session working in project.
func lcHook(t *testing.T, project, session string) {
	t.Helper()
	payload, _ := json.Marshal(map[string]any{"cwd": project, "session_id": session, "tool_name": "Edit", "tool_use_id": "t-" + session + "-" + recordNow().Format(time.RFC3339Nano), "tool_input": map[string]string{"file_path": "plan.md"}})
	HandleHook("post", strings.NewReader(string(payload)))
}

// lcSnapshot hashes every file under the record store, so a test can prove
// a report read wrote nothing.
func lcSnapshot(t *testing.T) map[string]string {
	t.Helper()
	out := map[string]string{}
	root := RecordDir()
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, _ := os.ReadFile(path)
		sum := sha256.Sum256(b)
		out[strings.TrimPrefix(path, root)] = hex.EncodeToString(sum[:])
		return nil
	})
	return out
}

func lcSameSnapshot(t *testing.T, before, after map[string]string) {
	t.Helper()
	if len(before) != len(after) {
		t.Fatalf("a report read changed the record store: %d files before, %d after", len(before), len(after))
	}
	for k, v := range before {
		if after[k] != v {
			t.Fatalf("a report read changed %s", k)
		}
	}
}

func lcReport(t *testing.T, project, plan, archive string, history bool, now time.Time) (PlanReport, string) {
	t.Helper()
	in, err := LoadReportInput(context.Background(), project, plan, archive, history, now)
	if err != nil {
		t.Fatalf("load report: %v", err)
	}
	rep := BuildPlanReport(in)
	return rep, RenderPlanReport(rep)
}

// lcAPI reads the report through the dashboard route.
func lcAPI(t *testing.T, query url.Values) map[string]any {
	t.Helper()
	srv := httptest.NewServer(ReportAPI())
	defer srv.Close()
	resp, err := http.Get(srv.URL + "/api/report?" + query.Encode())
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("report API status %d", resp.StatusCode)
	}
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

// lcPath walks a decoded JSON value by keys and indexes.
func lcPath(v any, keys ...any) any {
	for _, k := range keys {
		switch key := k.(type) {
		case string:
			m, _ := v.(map[string]any)
			v = m[key]
		case int:
			list, _ := v.([]any)
			if key >= len(list) {
				return nil
			}
			v = list[key]
		}
	}
	return v
}

func lcSection(html, id string) string {
	start := strings.Index(html, `id="`+id+`"`)
	if start < 0 {
		return ""
	}
	end := strings.Index(html[start:], `</section>`)
	return html[start : start+end]
}

func lcDecision(at time.Time, project, plan string, c conductor.Candidate) conductor.Decision {
	c.Project = model.Project{ID: "p1", Path: project}
	c.Plan = conductor.Plan{Name: plan, ProjectPath: project, State: "unblocked"}
	return conductor.Decision{At: at, Candidates: []conductor.Candidate{c}, Queues: []conductor.Queue{{ProjectID: "p1", RunningOwners: map[string]string{}}}}
}

var lcProjects = func(project string) []model.Project { return []model.Project{{ID: "p1", Path: project}} }

const lcExec = "phase: executing\nExecuting: 2026-01-07T09:00:00Z\nharness: claude-code"

func lcRunning(at time.Time) string { return "\nrunning: " + at.UTC().Format(time.RFC3339) }

// TESTMASTER: id=kr-handoff-resume tier=fast parallel=no
func TestKrHandoffResume(t *testing.T) {
	lcEnv(t)
	project := lcProject(t, "heron")
	t0 := time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC)
	clock := lcUseClock(t, t0)
	at := func(d time.Duration) time.Time { return clock.set(t0.Add(d)) }

	// The CLI takes the lock, hits a blocker, clears it and releases.
	writeRecordPlan(t, project, "heron", lcExec+lcRunning(t0))
	lcHook(t, project, "s1")
	at(time.Minute)
	writeRecordPlan(t, project, "heron", lcExec+lcRunning(clock.now))
	lcHook(t, project, "s1")
	at(2 * time.Minute)
	writeRecordPlan(t, project, "heron", lcExec+"\nstatus: blocked-on-operator: needs a signing token"+lcRunning(clock.now))
	lcHook(t, project, "s1")
	at(10 * time.Minute)
	writeRecordPlan(t, project, "heron", lcExec+"\nstatus: unblocked: token supplied"+lcRunning(clock.now))
	lcHook(t, project, "s1")
	at(12 * time.Minute)
	writeRecordPlan(t, project, "heron", lcExec+"\nstatus: unblocked: token supplied\nrunning: false")
	lcHook(t, project, "s1")

	// The Conductor's next tick admits the eligible plan, then claims it.
	src := ConductorPlanSource{}
	src.RecordAdmission(lcDecision(at(13*time.Minute), project, "heron", conductor.Candidate{Eligible: true, Selected: true, Code: "eligible", Reason: "Eligible"}), lcProjects(project))
	a := model.Assignment{ID: "assignment-heron-1", RunID: "run-c1", ProjectID: "p1", ProjectPath: project, Plan: "heron", Step: 1, Kind: "pair", Task: "Write receipt", Validation: "Read receipt"}
	at(13*time.Minute + time.Second)
	src.RecordAssignment(conductor.RecordAdmitted, a, nil, nil)
	src.RecordAssignment(conductor.RecordStarted, a, nil, nil)
	// While the Conductor holds the marker, a CLI hook is not an owner.
	writeRecordPlan(t, project, "heron", lcExec+lcRunning(clock.now)+"\nconductor-assignment: "+a.ID)
	lcHook(t, project, "s2")
	at(20 * time.Minute)
	src.RecordAssignment(conductor.RecordFinished, a, &model.Result{Outcome: "completed", ValidationMet: true, Evidence: []string{"receipt read back"}}, nil)
	writeRecordPlan(t, project, "heron", lcExec+"\nrunning: false\nconductor-assignment: ")

	before := lcSnapshot(t)
	rep, page := lcReport(t, project, "heron", "", false, t0.Add(21*time.Minute))
	o := rep.Ownership
	if o.Status != sectionAvailable || o.Holds != 2 {
		t.Fatalf("status %s holds %d: %s", o.Status, o.Holds, o.StatusNote)
	}

	// Ordered transitions: CLI acquired, blocker detected and cleared, CLI
	// released, admission, Conductor acquired, Conductor released.
	want := []string{"ownership.acquired/CLI", "blocker.detected", "blocker.cleared", "ownership.released/CLI", "admission.considered", "ownership.acquired/Conductor", "ownership.released/Conductor"}
	var got []string
	var last time.Time
	for _, e := range o.Transitions {
		if e.At.Before(last) {
			t.Fatalf("transitions out of order at %+v", e)
		}
		last = e.At
		key := e.Event
		if strings.HasPrefix(key, "ownership.") {
			key += "/" + e.Actor
		}
		if strings.HasPrefix(key, "lifecycle.") {
			continue
		}
		got = append(got, key)
	}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("transitions\n got %v\nwant %v", got, want)
	}
	for _, e := range o.Transitions {
		if e.Event == "ownership.acquired" && strings.Contains(e.Summary, "s2") {
			t.Fatalf("a CLI hook during Conductor ownership acquired the plan: %+v", e)
		}
	}

	// The handoff carries the measured delays.
	if len(o.Handoffs) != 2 {
		t.Fatalf("handoffs: %+v", o.Handoffs)
	}
	h := o.Handoffs[0]
	if h.FromKind != "cli" || h.ToKind != "conductor" || h.ReleaseToResumeMS == nil || *h.ReleaseToResumeMS != 61000 ||
		h.ClearToReleaseMS == nil || *h.ClearToReleaseMS != 120000 || !h.ClearanceObserved || h.BlockerReason != "blocked-on-operator: needs a signing token" {
		t.Fatalf("handoff: %+v", h)
	}
	if o.Current != nil || o.Pending == nil || o.Pending.WaitingMS != 60000 || !strings.Contains(o.Pending.Reason, "No admission decision") {
		t.Fatalf("current/pending: %+v %+v", o.Current, o.Pending)
	}
	if len(o.Blockers) != 1 || o.Blockers[0].DurationMS == nil || *o.Blockers[0].DurationMS != 480000 || o.Blockers[0].ClearedBy != "CLI" {
		t.Fatalf("blockers: %+v", o.Blockers)
	}

	sec := lcSection(page, "report-ownership")
	for _, s := range []string{`aria-labelledby="report-ownership-h"`, `<h2 id="report-ownership-h">`, `Released to resumed: <span class="ro-delay">1m 1s</span>`, `Blocker cleared to released: <span class="ro-delay">2m 0s</span>`, "Definitions"} {
		if !strings.Contains(sec, s) {
			t.Fatalf("ownership HTML lacks %q", s)
		}
	}

	body := lcAPI(t, url.Values{"project": {project}, "plan": {"heron"}})
	if v := lcPath(body, "report", "ownership", "handoffs", 0, "release_to_resume_ms"); v != float64(61000) {
		t.Fatalf("API release_to_resume_ms = %v", v)
	}
	if v := lcPath(body, "report", "ownership", "handoffs", 0, "clear_to_release_ms"); v != float64(120000) {
		t.Fatalf("API clear_to_release_ms = %v", v)
	}
	lcSameSnapshot(t, before, lcSnapshot(t))
}

// TESTMASTER: id=kr-handoff-gated tier=fast parallel=no
func TestKrHandoffGated(t *testing.T) {
	t.Run("live CLI owner", func(t *testing.T) {
		lcEnv(t)
		project := lcProject(t, "heron")
		t0 := time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC)
		clock := lcUseClock(t, t0)
		at := func(d time.Duration) time.Time { return clock.set(t0.Add(d)) }
		writeRecordPlan(t, project, "heron", lcExec+lcRunning(t0))
		lcHook(t, project, "s1")
		at(2 * time.Minute)
		writeRecordPlan(t, project, "heron", lcExec+"\nstatus: blocked-on-operator: needs a token"+lcRunning(clock.now))
		lcHook(t, project, "s1")
		at(5 * time.Minute)
		writeRecordPlan(t, project, "heron", lcExec+"\nstatus: unblocked"+lcRunning(clock.now))
		lcHook(t, project, "s1")
		// The Conductor sees the CLI holding the plan and defers to it.
		d := conductor.Decision{At: at(6 * time.Minute), Queues: []conductor.Queue{{ProjectID: "p1", RunningOwners: map[string]string{"heron": "cli"}}}}
		ConductorPlanSource{}.RecordAdmission(d, lcProjects(project))

		rep, page := lcReport(t, project, "heron", "", false, t0.Add(7*time.Minute))
		o := rep.Ownership
		if o.Current == nil || o.Current.Kind != "cli" || o.Current.ID != "s1" || o.Current.Stale || o.Pending != nil {
			t.Fatalf("live owner: %+v pending %+v", o.Current, o.Pending)
		}
		if o.Current.LastHeartbeat == nil || !o.Current.LastHeartbeat.Equal(t0.Add(5*time.Minute)) {
			t.Fatalf("heartbeat: %+v", o.Current.LastHeartbeat)
		}
		gated := false
		for _, e := range o.Transitions {
			gated = gated || (e.Event == "admission.considered" && strings.Contains(e.Summary, "gate ownership"))
		}
		if !gated || len(o.Handoffs) != 0 {
			t.Fatalf("admission not explained: %+v %+v", o.Transitions, o.Handoffs)
		}
		if !strings.Contains(lcSection(page, "report-ownership"), "CLI session s1, since") {
			t.Fatal("current owner not rendered")
		}

		// Thirty-five quiet minutes later the owner is labelled stale, and
		// reading the report twice — page and API — frees nothing.
		before := lcSnapshot(t)
		for i := 0; i < 2; i++ {
			rep, page = lcReport(t, project, "heron", "", false, t0.Add(40*time.Minute))
			o = rep.Ownership
			if o.Current == nil || o.Current.ID != "s1" || !o.Current.Stale || !strings.Contains(o.Current.StaleNote, "does not release it") {
				t.Fatalf("stale owner: %+v", o.Current)
			}
		}
		if !strings.Contains(lcSection(page, "report-ownership"), `<span class="ro-stale">Stale.</span>`) {
			t.Fatal("stale owner not labelled")
		}
		lcAPI(t, url.Values{"project": {project}, "plan": {"heron"}})
		lcSameSnapshot(t, before, lcSnapshot(t))
		runs, _ := RecordStore().Runs(project, "heron")
		if len(runs) != 1 || runs[0].Owner == nil || runs[0].Owner.OwnerID != "s1" {
			t.Fatalf("the record's owner changed: %+v", runs)
		}
	})

	t.Run("automation, schedule and budget gates", func(t *testing.T) {
		lcEnv(t)
		project := lcProject(t, "egret")
		t0 := time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC)
		clock := lcUseClock(t, t0)
		at := func(d time.Duration) time.Time { return clock.set(t0.Add(d)) }
		writeRecordPlan(t, project, "egret", lcExec+lcRunning(t0))
		lcHook(t, project, "s1")
		at(time.Minute)
		writeRecordPlan(t, project, "egret", lcExec+"\nstatus: blocked-on-operator: approve the spend"+lcRunning(clock.now))
		lcHook(t, project, "s1")
		at(3 * time.Minute)
		writeRecordPlan(t, project, "egret", lcExec+"\nstatus: unblocked"+lcRunning(clock.now))
		lcHook(t, project, "s1")
		at(4 * time.Minute)
		writeRecordPlan(t, project, "egret", lcExec+"\nstatus: unblocked\nrunning: false")
		lcHook(t, project, "s1")

		src := ConductorPlanSource{}
		tick := func(d time.Duration, c conductor.Candidate) {
			src.RecordAdmission(lcDecision(at(d), project, "egret", c), lcProjects(project))
		}
		pending := func(now time.Duration) *OwnershipPending {
			rep, page := lcReport(t, project, "egret", "", false, t0.Add(now))
			if rep.Ownership.Current != nil || rep.Ownership.Pending == nil {
				t.Fatalf("released plan: current %+v pending %+v", rep.Ownership.Current, rep.Ownership.Pending)
			}
			if !strings.Contains(lcSection(page, "report-ownership"), "Waiting to resume") {
				t.Fatal("pending reason not rendered")
			}
			return rep.Ownership.Pending
		}

		tick(5*time.Minute, conductor.Candidate{Code: "automation_disabled", Reason: "Automation disabled"})
		if p := pending(6 * time.Minute); p.Gate != "automation_disabled" || p.Reason != "Automation disabled" || p.WaitingMS != 120000 || p.Decision != "gated" {
			t.Fatalf("disabled: %+v", p)
		}
		tick(6*time.Minute, conductor.Candidate{Code: "automation_disabled", Reason: "Automation disabled"})
		tick(10*time.Minute, conductor.Candidate{Code: "automation_window", Reason: "Outside automation time window"})
		if p := pending(11 * time.Minute); p.Gate != "schedule" || p.Reason != "Outside automation time window" {
			t.Fatalf("schedule: %+v", p)
		}
		tick(20*time.Minute, conductor.Candidate{Code: "weekly_exhausted", Reason: "Weekly allowance exhausted", Budget: model.BudgetDecision{Allowed: false, Code: "weekly_exhausted"}})
		if p := pending(21 * time.Minute); p.Gate != "budget" || p.Reason != "Weekly allowance exhausted" {
			t.Fatalf("budget: %+v", p)
		}
		tick(25*time.Minute, conductor.Candidate{Code: "legacy_gate", Reason: "project launch policy: outside the launch schedule"})
		if p := pending(26 * time.Minute); p.Gate != "schedule" {
			t.Fatalf("launch policy: %+v", p)
		}
		runs, _ := RecordStore().Runs(project, "egret")
		if len(runs) != 1 || len(runs[0].Admissions) != 4 {
			t.Fatalf("a repeated decision added an admission: %+v", runs[0].Admissions)
		}

		// The waits land in the time partition by gate.
		rep, _ := lcReport(t, project, "egret", "", false, t0.Add(30*time.Minute))
		shares := map[string]int64{}
		for _, p := range rep.Time.Partition {
			shares[p.Cause] = p.MS
		}
		if shares["queued"] != 10*60000 || shares["schedule"] != 15*60000 || shares["blocked"] != 2*60000 {
			t.Fatalf("partition: %+v", rep.Time.Partition)
		}
		if h := rep.Ownership.Handoffs; len(h) != 1 || len(h[0].Gates) != 4 || h[0].ResumedAt != nil {
			t.Fatalf("waiting handoff: %+v", h)
		}
	})
}

// TESTMASTER: id=kr-admission-summary tier=fast parallel=no
func TestRecordAdmissionGatesFromCandidates(t *testing.T) {
	project := "/p"
	cases := []struct {
		c        conductor.Candidate
		cli      bool
		decision string
		gate     string
	}{
		{conductor.Candidate{Selected: true, Eligible: true}, false, "admitted", ""},
		{conductor.Candidate{Eligible: true}, false, "gated", "queue"},
		{conductor.Candidate{Code: "disabled"}, false, "gated", "automation_disabled"},
		{conductor.Candidate{Code: "project_window"}, false, "gated", "schedule"},
		{conductor.Candidate{Code: "project_busy"}, true, "gated", "ownership"},
		{conductor.Candidate{Code: "project_busy"}, false, "gated", "queue"},
		{conductor.Candidate{Code: "legacy_gate", Reason: "manual iterate run lock owns this project"}, false, "gated", "ownership"},
		{conductor.Candidate{Code: "short_exhausted", Budget: model.BudgetDecision{Code: "short_exhausted"}}, false, "gated", "budget"},
		{conductor.Candidate{Code: "plan_blocked"}, false, "gated", "other"},
	}
	for _, tc := range cases {
		d := lcDecision(time.Now(), project, "kite", tc.c)
		if tc.cli {
			d.Queues[0].RunningOwners["other"] = "cli"
		}
		got := summarizeAdmission(d, []model.Project{{ID: "p1", Path: project}})
		var o admissionObservation
		for _, x := range got {
			if x.Plan == "kite" {
				o = x
			}
		}
		if o.Decision != tc.decision || o.Gate != tc.gate {
			t.Fatalf("%s: got %s/%s want %s/%s", tc.c.Code, o.Decision, o.Gate, tc.decision, tc.gate)
		}
	}
	_ = runrecord.SchemaVersion
}
