package iterrun

import (
	"bufio"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// lcOpenRun opens the run of a plan execution through the store and
// returns its id.
func lcOpenRun(t *testing.T, project, plan, marker string, at time.Time) string {
	t.Helper()
	id, _, err := RecordStore().Observe(runrecord.OpenRequest{Key: runrecord.PlanKey{ProjectPath: project, Plan: plan}, Marker: marker, Source: runrecord.SourceCLI, At: at}, nil)
	if err != nil || id == "" {
		t.Fatalf("open run: %q %v", id, err)
	}
	return id
}

// lcAppend appends events to one run through the store.
func lcAppend(t *testing.T, project, plan, runID string, events ...runrecord.Event) {
	t.Helper()
	for i := range events {
		e := &events[i]
		e.ProjectPath, e.Plan, e.RunID = project, plan, runID
		if e.Source == "" {
			e.Source = runrecord.SourceCLI
		}
	}
	if _, err := RecordStore().AppendAll(events); err != nil {
		t.Fatalf("append: %v", err)
	}
}

// lcCall appends one tool call's pre and post hook events.
func lcCall(t *testing.T, project, plan, agent, id string, start time.Time, d time.Duration) {
	t.Helper()
	base := Event{SessionID: "s1", AgentID: agent, ToolName: "Bash", ToolUseID: id, Plan: plan, CWD: project}
	pre, post := base, base
	pre.TS, pre.Hook = start, "pre"
	post.TS, post.Hook = start.Add(d), "post"
	if err := AppendEvent(pre); err != nil {
		t.Fatal(err)
	}
	if err := AppendEvent(post); err != nil {
		t.Fatal(err)
	}
}

func lcShares(s TimeSection) map[string]int64 {
	out := map[string]int64{}
	var sum int64
	for _, p := range s.Partition {
		out[p.Cause] = p.MS
		sum += p.MS
	}
	out["sum"] = sum
	return out
}

func lcMin(n float64) int64 { return int64(n * 60000) }

// TESTMASTER: id=kr-time-partition tier=fast parallel=no
func TestKrTimePartition(t *testing.T) {
	lcEnv(t)
	project := lcProject(t, "kite")
	t0 := time.Date(2026, 1, 7, 9, 0, 0, 0, time.UTC)
	m := func(n float64) time.Time { return t0.Add(time.Duration(n * float64(time.Minute))) }
	writeRecordPlan(t, project, "kite", lcExec)

	// Two lanes overlap for 30 minutes: the coordinator calls every minute
	// for 50 s from 0 to 60, a team calls back to back from 30 to 90.
	for i := 0; i < 60; i++ {
		lcCall(t, project, "kite", "", "c"+time.Duration(i).String(), m(float64(i)), 50*time.Second)
	}
	for i := 30; i < 90; i++ {
		lcCall(t, project, "kite", "akite-api-0123456789abcdef", "a"+time.Duration(i).String(), m(float64(i)), time.Minute)
	}

	// Recorded waits, one of them overlapping the team's work.
	run := lcOpenRun(t, project, "kite", "2026-01-07T09:00:00Z", t0)
	lcAppend(t, project, "kite", run,
		runrecord.Event{Type: runrecord.TypeBlockerDetected, At: m(80), Blocker: &runrecord.Blocker{Key: "b1", Kind: "operator", Reason: "needs an API key"}},
		runrecord.Event{Type: runrecord.TypeBlockerCleared, At: m(160), Blocker: &runrecord.Blocker{Key: "b1"}},
		runrecord.Event{Type: runrecord.TypeLifecycle, At: m(170), Lifecycle: &runrecord.Lifecycle{State: "paused", Reason: "paused: operator"}},
		runrecord.Event{Type: runrecord.TypeLifecycle, At: m(230), Lifecycle: &runrecord.Lifecycle{State: "resumed"}},
		runrecord.Event{Type: runrecord.TypeAdmission, Source: runrecord.SourceConductor, At: m(240), Admission: &runrecord.Admission{Decision: "gated", Gate: "schedule", Reason: "Outside automation time window"}},
		runrecord.Event{Type: runrecord.TypeAdmission, Source: runrecord.SourceConductor, At: m(300), Admission: &runrecord.Admission{Decision: "gated", Gate: "budget", Reason: "Weekly allowance exhausted"}},
		runrecord.Event{Type: runrecord.TypeOwnershipAcquired, Source: runrecord.SourceConductor, At: m(330), Ownership: &runrecord.Ownership{OwnerKind: "conductor", OwnerID: "assignment-k1"}},
		runrecord.Event{Type: runrecord.TypeAttemptStarted, Source: runrecord.SourceConductor, At: m(330), Requirement: 1, AttemptKey: "assignment-k1", AssignmentID: "assignment-k1", AttemptInfo: &runrecord.Attempt{Kind: "pair"}},
		runrecord.Event{Type: runrecord.TypeAttemptFinished, Source: runrecord.SourceConductor, At: m(390), Requirement: 1, AttemptKey: "assignment-k1", AssignmentID: "assignment-k1", AttemptInfo: &runrecord.Attempt{Kind: "pair", Outcome: "passed"}},
		runrecord.Event{Type: runrecord.TypeOwnershipReleased, Source: runrecord.SourceConductor, At: m(390), Ownership: &runrecord.Ownership{OwnerKind: "conductor", OwnerID: "assignment-k1"}},
	)

	want := map[string]int64{"execution": lcMin(150), "blocked": lcMin(70), "paused": lcMin(60), "schedule": lcMin(60), "queued": lcMin(30), "unobserved": lcMin(230), "sum": lcMin(600)}
	check := func(label string, s TimeSection, want map[string]int64) {
		t.Helper()
		got := lcShares(s)
		for k, v := range want {
			if got[k] != v {
				t.Fatalf("%s: %s = %s, want %s (partition %+v)", label, k, lcDur(got[k]), lcDur(v), s.Partition)
			}
		}
		if s.WallMS == nil || *s.WallMS != got["sum"] {
			t.Fatalf("%s: partition sums to %d, wall is %v", label, got["sum"], s.WallMS)
		}
	}

	rep, page := lcReport(t, project, "kite", "", true, m(600))
	s := rep.Time
	check("live", s, want)
	if s.Status != sectionAvailable || s.Clock == nil || !s.Clock.Open || s.Clock.StartBasis != "Executing: in the plan file" {
		t.Fatalf("clock: %+v %s", s.Clock, s.StatusNote)
	}
	// Lane-time keeps parallel lanes apart and never feeds elapsed time.
	lanes := map[string]TimeLaneTotal{}
	for _, l := range s.LaneTime {
		lanes[l.Kind] = l
	}
	if l := lanes["tool"]; l.MS == nil || *l.MS != lcMin(110) || l.Lanes != 2 || l.Spans != 120 {
		t.Fatalf("tool lane-time: %+v", l)
	}
	if l := lanes["between"]; l.MS == nil || *l.MS != 59*10000 {
		t.Fatalf("between-call lane-time: %+v", l)
	}
	if l := lanes["attempt"]; l.MS == nil || *l.MS != lcMin(60) {
		t.Fatalf("attempt lane-time: %+v", l)
	}
	if *lanes["tool"].MS+*lanes["attempt"].MS <= lcShares(s)["execution"] {
		t.Fatal("lane-time should exceed execution when lanes overlap")
	}
	// Every listed interval is a non-execution cause with explicit bounds.
	causes := map[string]bool{}
	for _, iv := range s.Intervals {
		if iv.Cause == "execution" || !iv.End.After(iv.Start) || iv.MS != iv.End.Sub(iv.Start).Milliseconds() {
			t.Fatalf("interval: %+v", iv)
		}
		causes[iv.Cause] = true
	}
	for _, c := range []string{"blocked", "paused", "schedule", "queued", "unobserved"} {
		if !causes[c] {
			t.Fatalf("no %s interval listed: %+v", c, s.Intervals)
		}
	}
	sec := lcSection(page, "report-time")
	for _, frag := range []string{`aria-labelledby="report-time-h"`, `role="img"`, `<caption class="vh">Elapsed time by cause`, "Lane-time, not part of elapsed", "Precedence", `data-cause="unobserved"`} {
		if !strings.Contains(sec, frag) {
			t.Fatalf("time HTML lacks %q", frag)
		}
	}
	if strings.Contains(sec, "severe") || strings.Contains(sec, "var(--danger)") {
		t.Fatal("the time section judges severity")
	}
	body := lcAPI(t, url.Values{"project": {project}, "plan": {"kite"}, "history": {"1"}})
	var sum float64
	parts, _ := lcPath(body, "report", "time", "partition").([]any)
	for _, p := range parts {
		sum += lcPath(p, "ms").(float64)
	}
	if wall, _ := lcPath(body, "report", "time", "wall_ms").(float64); len(parts) != 6 || sum != wall {
		t.Fatalf("API partition sums to %v of %v", sum, lcPath(body, "report", "time", "wall_ms"))
	}

	// Interrupted, paused and archived views keep the same clock rules.
	lcAppend(t, project, "kite", run, runrecord.Event{Type: runrecord.TypeLifecycle, Source: runrecord.SourceConductor, At: m(390), Lifecycle: &runrecord.Lifecycle{State: "interrupted"}})
	rep, _ = lcReport(t, project, "kite", "", true, m(600))
	check("interrupted", rep.Time, want)
	lcAppend(t, project, "kite", run, runrecord.Event{Type: runrecord.TypeLifecycle, At: m(395), Lifecycle: &runrecord.Lifecycle{State: "paused"}})
	paused := map[string]int64{"execution": lcMin(150), "paused": lcMin(60 + 205), "unobserved": lcMin(25), "sum": lcMin(600)}
	rep, _ = lcReport(t, project, "kite", "", true, m(600))
	check("paused", rep.Time, paused)

	archive := "20260107T190000Z-kite-done.md"
	dir := filepath.Join(project, ".claude", "iterate", "archive")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	body2 := "name: kite\nphase: complete\nExecuting: 2026-01-07T09:00:00Z\nFinished: 2026-01-07T19:00:00Z\nharness: claude-code\n## Goal\nFixture only\n## Steps\n- [x] 1. Write receipt\n## Validation\n- [x] 1. Read receipt\n"
	if err := os.WriteFile(filepath.Join(dir, archive), []byte(body2), 0o644); err != nil {
		t.Fatal(err)
	}
	lcAppend(t, project, "kite", run, runrecord.Event{Type: runrecord.TypeLifecycle, At: m(600), Lifecycle: &runrecord.Lifecycle{State: "archived", ArchiveFile: archive}})
	rep, _ = lcReport(t, project, "", archive, true, m(1800))
	check("archived", rep.Time, paused)
	if c := rep.Time.Clock; c == nil || c.Open || c.EndBasis != "Finished: in the plan file" {
		t.Fatalf("archived clock: %+v", c)
	}
}

// lcLoadFixture copies a shaped hook-event fixture into the isolated
// event log, pointing its working directories at project.
func lcLoadFixture(t *testing.T, name, project string) {
	t.Helper()
	f, err := os.Open(filepath.Join("testdata", "reports", "lifecycle", name))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var e Event
		if err := json.Unmarshal([]byte(strings.ReplaceAll(sc.Text(), "{{PROJECT}}", project)), &e); err != nil {
			t.Fatal(err)
		}
		if err := AppendEvent(e); err != nil {
			t.Fatal(err)
		}
	}
}

// TESTMASTER: id=kr-time-unknown tier=fast parallel=no
func TestKrTimeUnknown(t *testing.T) {
	lcEnv(t)
	project := lcProject(t, "waterbuck")
	t0 := time.Date(2026, 1, 5, 18, 24, 14, 0, time.UTC)
	end := t0.Add(22 * time.Hour)
	writeRecordPlan(t, project, "waterbuck", "phase: complete\nExecuting: 2026-01-05T18:24:14Z\nFinished: 2026-01-06T16:24:14Z\nharness: claude-code")
	lcLoadFixture(t, "waterbuck-shaped.events.jsonl", project)
	silenceFrom, silenceTo := time.Date(2026, 1, 5, 18, 24, 48, 0, time.UTC), time.Date(2026, 1, 6, 1, 46, 18, 0, time.UTC)

	// No record explains the overnight silence: it stays unknown.
	rep, page := lcReport(t, project, "waterbuck", "", true, end.Add(48*time.Hour))
	s := rep.Time
	if s.WallMS == nil || *s.WallMS != (22 * time.Hour).Milliseconds() || s.Clock.Open {
		t.Fatalf("wall: %v clock %+v", s.WallMS, s.Clock)
	}
	if len(s.Intervals) == 0 {
		t.Fatal("no intervals")
	}
	first := s.Intervals[0]
	if first.Cause != "unobserved" || !first.Start.Equal(silenceFrom) || !first.End.Equal(silenceTo) || first.MS != (7*time.Hour+21*time.Minute+30*time.Second).Milliseconds() {
		t.Fatalf("longest interval: %+v", first)
	}
	if !strings.Contains(strings.Join(first.Evidence, " "), "unknown, not a failure") || first.Owner != "ownership not recorded" {
		t.Fatalf("unknown interval explained as %+v", first)
	}
	quiet := false
	for _, iv := range s.Intervals {
		quiet = quiet || (iv.Cause == "unobserved" && iv.MS == lcMin(45))
	}
	if !quiet {
		t.Fatalf("the 45-minute quiet stretch is missing: %+v", s.Intervals)
	}
	got := lcShares(s)
	if got["sum"] != *s.WallMS || got["blocked"]+got["paused"]+got["schedule"]+got["queued"] != 0 {
		t.Fatalf("partition without causal evidence: %+v", s.Partition)
	}
	// The overlapping team lane adds lane-time, not elapsed time.
	for _, l := range s.LaneTime {
		if l.Kind == "tool" && (l.Lanes != 2 || l.MS == nil) {
			t.Fatalf("tool lanes: %+v", l)
		}
	}
	sec := lcSection(page, "report-time")
	if strings.Contains(sec, "severe") || strings.Contains(sec, "var(--danger)") || !strings.Contains(sec, "unknown, not a failure") {
		t.Fatal("the unknown interval is rendered as a failure or not explained")
	}

	// A recorded blocker over the silence explains it, and only that part.
	run := lcOpenRun(t, project, "waterbuck", "2026-01-05T18:24:14Z", t0)
	lcAppend(t, project, "waterbuck", run,
		runrecord.Event{Type: runrecord.TypeBlockerDetected, At: time.Date(2026, 1, 5, 18, 30, 0, 0, time.UTC), Blocker: &runrecord.Blocker{Key: "b1", Kind: "operator", Reason: "waiting for operator approval"}},
		runrecord.Event{Type: runrecord.TypeBlockerCleared, At: time.Date(2026, 1, 6, 1, 40, 0, 0, time.UTC), Blocker: &runrecord.Blocker{Key: "b1"}},
	)
	rep, _ = lcReport(t, project, "waterbuck", "", true, end.Add(48*time.Hour))
	s = rep.Time
	first = s.Intervals[0]
	if first.Cause != "blocked" || first.MS != (7*time.Hour+10*time.Minute).Milliseconds() || !strings.Contains(strings.Join(first.Evidence, " "), "waiting for operator approval") {
		t.Fatalf("explained interval: %+v", first)
	}
	unknown := map[int64]bool{}
	for _, iv := range s.Intervals {
		if iv.Cause == "unobserved" {
			unknown[iv.MS] = true
		}
	}
	if !unknown[(5*time.Minute+12*time.Second).Milliseconds()] || !unknown[(6*time.Minute+18*time.Second).Milliseconds()] {
		t.Fatalf("the unexplained edges of the silence: %+v", s.Intervals)
	}
	if got := lcShares(s); got["blocked"] != (7*time.Hour+10*time.Minute).Milliseconds() || got["sum"] != *s.WallMS {
		t.Fatalf("partition with a blocker: %+v", s.Partition)
	}
}
