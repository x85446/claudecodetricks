package iterrun

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

var integrityNow = time.Date(2026, 10, 22, 18, 0, 0, 0, time.UTC)

// integrityProject isolates HOME, the run record and the Conductor store,
// and registers an empty project so the report API accepts it.
func integrityProject(t *testing.T) string {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("ITERATE_RUN_RECORD_DIR", t.TempDir())
	t.Setenv("ITERATE_CONDUCTOR_DIR", t.TempDir())
	project := t.TempDir()
	for _, d := range []string{"plans", "archive"} {
		if err := os.MkdirAll(filepath.Join(project, ".claude", "iterate", d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	RegisterProject(project)
	return runrecord.CanonicalProject(project)
}

// copyIntegrityFixture copies testdata/reports/integrity/<name> into dst.
func copyIntegrityFixture(t *testing.T, name, dst string) {
	t.Helper()
	src := filepath.Join("testdata", "reports", "integrity", name)
	err := filepath.WalkDir(src, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(src, p)
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, b, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
}

func integrityArchive(project string) string {
	return filepath.Join(project, ".claude", "iterate", "archive")
}
func integrityPlans(project string) string {
	return filepath.Join(project, ".claude", "iterate", "plans")
}

// apiReport reads the report through GET /api/report.
func apiReport(t *testing.T, query url.Values) PlanReport {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/api/report?"+query.Encode(), nil)
	rec := httptest.NewRecorder()
	ReportAPI().ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /api/report?%s: %d %s", query.Encode(), rec.Code, rec.Body.String())
	}
	var body struct {
		Report PlanReport `json:"report"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	return body.Report
}

func loadReport(t *testing.T, project, plan, archive string) (ReportInput, PlanReport) {
	t.Helper()
	in, err := LoadReportInput(context.Background(), project, plan, archive, false, integrityNow)
	if err != nil {
		t.Fatalf("LoadReportInput: %v", err)
	}
	return in, BuildPlanReport(in)
}

func sectionHTML(t *testing.T, rep PlanReport, id string) string {
	t.Helper()
	page := RenderPlanReport(rep)
	start := strings.Index(page, `<section class="report-section" id="`+id+`"`)
	if start < 0 {
		t.Fatalf("section %s missing from the rendered report", id)
	}
	end := strings.Index(page[start:], `</section>`)
	return page[start : start+end+len(`</section>`)]
}

func mustContain(t *testing.T, where, text string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(text, w) {
			t.Errorf("%s lacks %q", where, w)
		}
	}
}

func observeIntegrity(t *testing.T, project, plan, marker string, at time.Time, events ...runrecord.Event) {
	t.Helper()
	_, _, err := RecordStore().Observe(runrecord.OpenRequest{
		Key: runrecord.PlanKey{ProjectPath: project, Plan: plan}, Marker: marker, Source: runrecord.SourceCLI, At: at,
	}, func(runrecord.Run) []runrecord.Event { return events })
	if err != nil {
		t.Fatal(err)
	}
}

func intp(n int) *int { return &n }

// TESTMASTER: id=kr-progress-complete tier=fast parallel=no
func TestKrProgressComplete(t *testing.T) {
	project := integrityProject(t)
	copyIntegrityFixture(t, "gemsbok", integrityArchive(project))
	copyIntegrityFixture(t, "albatross", integrityArchive(project))

	t.Run("gemsbok", func(t *testing.T) {
		const archive = "20261020T094038Z-gemsbok.md"
		in, rep := loadReport(t, project, "", archive)
		p := rep.Progress
		if p.Outcome.State != "completed" || !p.Outcome.Terminal || p.Outcome.Label != "Completed and archived" {
			t.Fatalf("outcome: %+v", p.Outcome)
		}
		if m := p.Outcome.Merge; m == nil || m.Ref != "PR 28" || !strings.HasPrefix(m.Revision, "399c3bd") || !m.Imported {
			t.Fatalf("merge outcome: %+v", p.Outcome.Merge)
		}
		want := ProgressRequirementCounts{Total: 4, Complete: 4, TasksImplemented: 4, ValidationsPassed: 4}
		if p.Requirements != want {
			t.Fatalf("requirement counts: %+v, want %+v", p.Requirements, want)
		}
		if p.Lanes != (ProgressLaneCounts{Total: 1, Completed: 1}) {
			t.Fatalf("lane counts: %+v (a completed flat plan's coordinator closes with the plan)", p.Lanes)
		}
		// Detail agrees with the summary.
		complete := 0
		for _, r := range p.RequirementItems {
			if r.State == "complete" {
				complete++
			}
		}
		if complete != 4 || len(rep.Evidence.Requirements) != 4 {
			t.Fatalf("detail: %d complete items, %d evidence requirements", complete, len(rep.Evidence.Requirements))
		}
		if p.Attempts.Captured || p.Attempts.Note == "" {
			t.Fatalf("attempts are unknown for a plan-file execution, not zero: %+v", p.Attempts)
		}

		// The headline shows the outcome and four of four; no zero-done
		// card and no open lane.
		h := sectionHTML(t, rep, "report-progress")
		mustContain(t, "progress HTML", h, `aria-labelledby="report-progress-title"`, `<h2 id="report-progress-title">Verified progress</h2>`,
			"Completed and archived", "complete: merged PR28", "PR 28", "4 requirements", "4 of 4", "1 lane", "How progress is counted", `href="#evidence-req-1"`)
		if strings.Contains(h, ">0 of 4<") && strings.Index(h, ">0 of 4<") < strings.Index(h, ">4 of 4<") {
			t.Errorf("a zero count precedes the completed count")
		}
		if strings.Contains(strings.ToLower(h), "recorded open") {
			t.Errorf("progress still reports a recorded open lane")
		}

		// The API serves the same summary.
		api := apiReport(t, url.Values{"project": {project}, "archive": {archive}})
		if api.Progress.Requirements != want || api.Progress.Outcome.State != "completed" || api.Progress.Lanes.Completed != 1 {
			t.Fatalf("api progress: %+v %+v", api.Progress.Requirements, api.Progress.Outcome)
		}
		_ = in
	})

	t.Run("albatross", func(t *testing.T) {
		const archive = "2026-09-11T02-27-19Z-albatross-done.md"
		in, rep := loadReport(t, project, "", archive)
		p := rep.Progress
		if p.Outcome.State != "completed" || p.Outcome.Merge == nil || p.Outcome.Merge.Ref != "PR 19" || p.Outcome.Merge.Revision != "ea3929c" {
			t.Fatalf("outcome: %+v merge %+v", p.Outcome, p.Outcome.Merge)
		}
		if p.Requirements.Total != 20 || p.Requirements.Complete != 20 || p.Requirements.ValidationsPassed != 20 {
			t.Fatalf("requirements: %+v", p.Requirements)
		}
		if p.Lanes != (ProgressLaneCounts{Total: 6, Completed: 6}) {
			t.Fatalf("lanes: %+v %+v", p.Lanes, p.LaneItems)
		}
		// A further lane observation (a hook-only agent with activity and
		// no terminal line) changes lane counts, never requirement counts.
		in.Rows = append(in.Rows, Row{key: "a-split", label: "split-agent", spans: []span{{start: integrityNow.Add(-time.Hour), end: integrityNow}}})
		p2 := BuildPlanReport(in).Progress
		if p2.Requirements != p.Requirements {
			t.Fatalf("requirement counts moved with lanes: %+v -> %+v", p.Requirements, p2.Requirements)
		}
		if p2.Lanes != (ProgressLaneCounts{Total: 7, Completed: 6, Unknown: 1}) {
			t.Fatalf("extra lane: %+v", p2.Lanes)
		}
		h := sectionHTML(t, BuildPlanReport(in), "report-progress")
		mustContain(t, "albatross progress HTML", h, "20 requirements", "20 of 20", "7 lanes", "Unknown end", "no terminal observation before the plan ended")
		if strings.Contains(h, "Queued</dt><dd class=\"pg-n\">1<") {
			t.Errorf("a lane of a completed plan reads as queued")
		}
	})
}

// TESTMASTER: id=kr-progress-partial tier=fast parallel=no
func TestKrProgressPartial(t *testing.T) {
	project := integrityProject(t)
	copyIntegrityFixture(t, "kestrel", integrityPlans(project))
	marker := "2026-10-22T09:00:00Z"
	t0 := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	observeIntegrity(t, project, "kestrel", marker, t0,
		runrecord.Event{Type: runrecord.TypeLifecycle, ID: "k-exec", At: t0, Lifecycle: &runrecord.Lifecycle{State: "executing", ObservedVia: "plan-file"}},
		runrecord.Event{Type: runrecord.TypeAttemptStarted, ID: "k-3-1s", At: t0.Add(5 * time.Minute), Requirement: 3, AttemptKey: "k3a1", Team: "beta", AttemptInfo: &runrecord.Attempt{Kind: "cli"}},
		runrecord.Event{Type: runrecord.TypeAttemptFinished, ID: "k-3-1f", At: t0.Add(9 * time.Minute), Requirement: 3, AttemptKey: "k3a1", AttemptInfo: &runrecord.Attempt{Outcome: "blocked", Reason: "no signing identity"}},
		runrecord.Event{Type: runrecord.TypeAttemptStarted, ID: "k-4-1s", At: t0.Add(10 * time.Minute), Requirement: 4, AttemptKey: "k4a1", Team: "gamma", AttemptInfo: &runrecord.Attempt{Kind: "cli"}},
	)
	query := url.Values{"project": {project}, "plan": {"kestrel"}}

	p := apiReport(t, query).Progress
	if p.Source != "record" || p.Outcome.State != "executing" || p.Outcome.Terminal {
		t.Fatalf("live outcome: source %s %+v", p.Source, p.Outcome)
	}
	wantReq := ProgressRequirementCounts{Total: 8, Complete: 2, TasksImplemented: 3, ValidationsPassed: 2, Blocked: 1, InProgress: 1, NotStarted: 4}
	if p.Requirements != wantReq {
		t.Fatalf("requirements: %+v, want %+v", p.Requirements, wantReq)
	}
	wantLanes := ProgressLaneCounts{Total: 5, Completed: 1, Active: 2, Blocked: 1, Queued: 1}
	if p.Lanes != wantLanes {
		t.Fatalf("lanes: %+v, want %+v (%+v)", p.Lanes, wantLanes, p.LaneItems)
	}
	if !p.Attempts.Captured || p.Attempts.Total != 2 || p.Attempts.ByOutcome["blocked"] != 1 || p.Attempts.ByOutcome["open"] != 1 {
		t.Fatalf("attempts: %+v", p.Attempts)
	}
	states := map[int]string{}
	for _, r := range p.RequirementItems {
		states[r.Number] = r.State
	}
	if states[3] != "blocked" || states[4] != "in-progress" || states[5] != "not-started" || states[8] != "not-started" {
		t.Fatalf("requirement states: %v", states)
	}

	// Refresh: requirement 4 passes with observed proof.
	t1 := t0.Add(40 * time.Minute)
	observeIntegrity(t, project, "kestrel", marker, t1,
		runrecord.Event{Type: runrecord.TypeAttemptFinished, ID: "k-4-1f", At: t1, Requirement: 4, AttemptKey: "k4a1", AttemptInfo: &runrecord.Attempt{Outcome: "passed"}},
		runrecord.Event{Type: runrecord.TypeEvidence, ID: "k-4-ev", At: t1, Requirement: 4, AttemptKey: "k4a1", Evidence: &runrecord.Evidence{Result: "passed", Command: "go test ./cli/...", ExitStatus: intp(0)}},
	)
	planPath := filepath.Join(integrityPlans(project), "kestrel.md")
	text, _ := os.ReadFile(planPath)
	os.WriteFile(planPath, []byte(strings.Replace(string(text), "- [ ] 4. CLI cases pass.", "- [x] 4. CLI cases pass.", 1)), 0o644)
	p = apiReport(t, query).Progress
	wantReq = ProgressRequirementCounts{Total: 8, Complete: 3, TasksImplemented: 3, ValidationsPassed: 3, ValidationsObserved: 1, Blocked: 1, NotStarted: 4}
	if p.Requirements != wantReq || p.Lanes != wantLanes {
		t.Fatalf("after refresh: %+v lanes %+v", p.Requirements, p.Lanes)
	}

	// The plan completes while gamma and delta never wrote a terminal
	// line: they are unknown, not queued or running; the coordinator
	// closes with the completion event.
	t2 := t0.Add(2 * time.Hour)
	text, _ = os.ReadFile(planPath)
	done := strings.Replace(string(text), "phase: executing", "phase: complete\nFinished: 2026-10-22T11:00:00Z", 1)
	os.WriteFile(planPath, []byte(done), 0o644)
	observeIntegrity(t, project, "kestrel", marker, t2,
		runrecord.Event{Type: runrecord.TypeLifecycle, ID: "k-done", At: t2, Lifecycle: &runrecord.Lifecycle{State: "completed", ObservedVia: "plan-file"}})
	rep := apiReport(t, query)
	p = rep.Progress
	if p.Outcome.State != "completed" || p.Outcome.Completed == nil || p.Outcome.Completed.Provenance != "observed" {
		t.Fatalf("completed outcome: %+v", p.Outcome)
	}
	if p.Requirements != wantReq {
		t.Fatalf("requirements changed with the outcome: %+v", p.Requirements)
	}
	if want := (ProgressLaneCounts{Total: 5, Completed: 2, Blocked: 1, Unknown: 2}); p.Lanes != want {
		t.Fatalf("ended lanes: %+v, want %+v (%+v)", p.Lanes, want, p.LaneItems)
	}
	for _, l := range p.LaneItems {
		if (l.Name == "gamma" || l.Name == "delta") && (l.State != "unknown" || !strings.Contains(l.Basis, "no terminal observation")) {
			t.Fatalf("lane %s: %+v", l.Name, l)
		}
	}

	in, err := LoadReportInput(context.Background(), project, "kestrel", "", false, integrityNow)
	if err != nil {
		t.Fatal(err)
	}
	h := sectionHTML(t, BuildPlanReport(in), "report-progress")
	mustContain(t, "partial progress HTML", h, "8 requirements", "3 of 8", "1 with observed proof", "5 lanes", "Unknown end", "Blocked or not met",
		"Completed</dt>", "Active</dt>", "Queued</dt>", `<span class="pg-unit">2 attempts</span>`, "Teams table status: blocked: no signing identity on this host")
}

// TESTMASTER: id=kr-evidence-binding tier=fast parallel=no
func TestKrEvidenceBinding(t *testing.T) {
	project := integrityProject(t)
	copyIntegrityFixture(t, "waterbuck", integrityPlans(project))
	marker := "2026-10-20T23:24:14Z"
	t0 := time.Date(2026, 10, 21, 7, 0, 0, 0, time.UTC)
	observeIntegrity(t, project, "waterbuck", marker, t0,
		runrecord.Event{Type: runrecord.TypeAttemptStarted, ID: "w4a1s", At: t0, Requirement: 4, AttemptKey: "w4a1", AttemptInfo: &runrecord.Attempt{Kind: "cli"}},
		runrecord.Event{Type: runrecord.TypeEvidence, ID: "w4a1e", At: t0.Add(time.Minute), Requirement: 4, AttemptKey: "w4a1",
			Evidence: &runrecord.Evidence{Result: "failed", Command: "make test-bake CASE=bake.cleanup.green", ExitStatus: intp(1), Summary: "exceeded 30s under load"}},
		runrecord.Event{Type: runrecord.TypeAttemptFinished, ID: "w4a1f", At: t0.Add(time.Minute), Requirement: 4, AttemptKey: "w4a1", AttemptInfo: &runrecord.Attempt{Outcome: "failed"}},
		runrecord.Event{Type: runrecord.TypeAttemptStarted, ID: "w4a2s", At: t0.Add(5 * time.Minute), Requirement: 4, AttemptKey: "w4a2", AttemptInfo: &runrecord.Attempt{Kind: "cli"}},
		runrecord.Event{Type: runrecord.TypeEvidence, ID: "w4a2e", At: t0.Add(6 * time.Minute), Requirement: 4, AttemptKey: "w4a2",
			Evidence: &runrecord.Evidence{Result: "passed", Command: "make test-bake CASE=bake.cleanup.green", ExitStatus: intp(0),
				ArtifactURL: "https://ci.example.invalid/runs/39654071327", Revision: "e59399e0c1d2", Environment: "linux-runner, project ci",
				CoveredFiles: []string{"scripts/build-macos-oci.sh"}, Summary: "bake.cleanup.green passed in 2.6s"}},
		runrecord.Event{Type: runrecord.TypeAttemptFinished, ID: "w4a2f", At: t0.Add(6 * time.Minute), Requirement: 4, AttemptKey: "w4a2", AttemptInfo: &runrecord.Attempt{Outcome: "passed"}},
		runrecord.Event{Type: runrecord.TypeEvidence, ID: "w6e", At: t0.Add(15 * time.Minute), Requirement: 6,
			Evidence: &runrecord.Evidence{Result: "passed", Command: "make publish-check", ExitStatus: intp(0), Summary: "publish.reproducible passed"}},
	)
	query := url.Values{"project": {project}, "plan": {"waterbuck"}}
	rep := apiReport(t, query)
	ev := rep.Evidence
	byNum := map[int]RequirementEvidence{}
	for _, r := range ev.Requirements {
		byNum[r.Number] = r
		// Every item binds to its own requirement and attempt.
		for _, it := range r.Items {
			if it.Requirement != r.Number || it.Attempt != 0 {
				t.Fatalf("requirement %d holds item for %d/%d: %+v", r.Number, it.Requirement, it.Attempt, it)
			}
		}
		for _, a := range r.Attempts {
			for _, it := range a.Items {
				if it.Requirement != r.Number || it.Attempt != a.Number {
					t.Fatalf("attempt %d of %d holds item for %d/%d", a.Number, r.Number, it.Requirement, it.Attempt)
				}
			}
		}
	}

	r4 := byNum[4]
	if len(r4.Attempts) != 2 || r4.Attempts[0].Outcome != "failed" || r4.Attempts[1].Outcome != "passed" || !r4.Attempts[1].Latest {
		t.Fatalf("requirement 4 attempts: %+v", r4.Attempts)
	}
	if a1 := r4.Attempts[0].Items; len(a1) != 1 || a1[0].Result != "failed" || *a1[0].ExitStatus != 1 {
		t.Fatalf("attempt 1 evidence: %+v", a1)
	}
	a2 := r4.Attempts[1].Items
	if len(a2) != 1 || a2[0].Result != "passed" || a2[0].Revision != "e59399e0c1d2" || a2[0].ArtifactURL == "" || a2[0].Environment == "" || a2[0].At == nil {
		t.Fatalf("attempt 2 evidence: %+v", a2)
	}
	if r4.Proof != "observed-passed" && r4.Proof != "observed-stale" {
		t.Fatalf("requirement 4 proof: %s", r4.Proof)
	}
	// The plan log line that reports steps 4, 6 and 7 is split three ways.
	stepText := func(r RequirementEvidence) []string {
		var out []string
		for _, it := range r.Items {
			if it.Imported {
				out = append(out, it.Summary)
			}
		}
		return out
	}
	for _, s := range stepText(r4) {
		if strings.Contains(s, "Step 6") || strings.Contains(s, "Step 7") || strings.Contains(s, "td.evidence.run") {
			t.Fatalf("requirement 4 carries another step's result: %q", s)
		}
	}
	if got := stepText(r4); len(got) != 2 || !strings.HasPrefix(got[0], "step 4 partial:") || !strings.HasPrefix(got[1], "step 4 bake.cleanup.green rerun passed") {
		t.Fatalf("requirement 4 imported reports: %q", got)
	}
	if got := stepText(byNum[6]); len(got) != 1 || got[0] != "Step 6 build DONE exit=0." {
		t.Fatalf("requirement 6 imported reports: %q", got)
	}
	if got := stepText(byNum[7]); len(got) != 1 || !strings.HasPrefix(got[0], "Step 7 fixture execution") || strings.Contains(got[0], "Step 6") {
		t.Fatalf("requirement 7 imported reports: %q", got)
	}
	if r6 := byNum[6]; len(r6.Items) != 2 || r6.Proof != "observed-passed" {
		t.Fatalf("requirement 6: %+v", r6)
	}
	for _, r := range []RequirementEvidence{byNum[6], byNum[7]} {
		for _, it := range r.Items {
			if it.Imported && (it.Result != "unknown" || it.Reported == "" && strings.Contains(it.Summary, "DONE")) {
				t.Fatalf("imported prose was promoted to a result: %+v", it)
			}
		}
	}
	if r7 := byNum[7]; r7.Proof != "imported-only" || !strings.HasPrefix(r7.ProofLabel, "Checked; imported reports only") {
		t.Fatalf("requirement 7 proof: %s %q", r7.Proof, r7.ProofLabel)
	}
	if r5 := byNum[5]; r5.Proof != "checked-without-proof" || r5.ProofLabel != "Checked, no proof recorded" {
		t.Fatalf("requirement 5 (checked, nothing behind it): %s %q", r5.Proof, r5.ProofLabel)
	}
	if r8 := byNum[8]; r8.Proof != "none" {
		t.Fatalf("requirement 8: %s", r8.Proof)
	}

	// A stale "partial" report never undoes requirement 4's checked boxes;
	// a log line naming requirement 8 with no result shows work, not proof.
	for _, r := range rep.Progress.RequirementItems {
		if r.Number == 4 && (r.State != "complete" || r.Reported != "met") {
			t.Fatalf("requirement 4 progress: %+v", r)
		}
		if r.Number == 8 && (r.State != "in-progress" || !strings.Contains(r.Basis, "plan log reports work on it")) {
			t.Fatalf("requirement 8 progress: %+v", r)
		}
	}

	in, err := LoadReportInput(context.Background(), project, "waterbuck", "", false, integrityNow)
	if err != nil {
		t.Fatal(err)
	}
	h := sectionHTML(t, BuildPlanReport(in), "report-evidence")
	mustContain(t, "evidence HTML", h, `aria-labelledby="report-evidence-title"`, `id="evidence-req-4"`, "Acceptance: what the validation must show",
		"Attempt 1 · failed · earlier attempt", "Attempt 2 · passed · latest", "make test-bake CASE=bake.cleanup.green", "Exit status", "e59399e0c1d2",
		`href="https://ci.example.invalid/runs/39654071327"`, "linux-runner, project ci", "Checked, no proof recorded", "Imported", "Result not observed",
		"Reports: partial", "What counts as evidence")

	t.Run("albatross reports keep earlier attempts", func(t *testing.T) {
		copyIntegrityFixture(t, "albatross", integrityArchive(project))
		_, rep := loadReport(t, project, "", "2026-09-11T02-27-19Z-albatross-done.md")
		for _, r := range rep.Evidence.Requirements {
			if r.Number != 4 {
				continue
			}
			var team []EvidenceItem
			for _, it := range r.Items {
				if strings.HasPrefix(it.Origin, "team build-system") {
					team = append(team, it)
				}
			}
			if len(team) != 2 || !team[0].Superseded || team[1].Superseded || !strings.Contains(team[1].Summary, "re-validated") {
				t.Fatalf("requirement 4 team reports: %+v", team)
			}
			return
		}
		t.Fatal("requirement 4 missing")
	})

	t.Run("gemsbok exposes recorded proof", func(t *testing.T) {
		copyIntegrityFixture(t, "gemsbok", integrityArchive(project))
		_, rep := loadReport(t, project, "", "20261020T094038Z-gemsbok.md")
		facts := map[string][]string{}
		texts := ""
		for _, it := range rep.Evidence.Plan {
			if !it.Imported || it.Result != "unknown" {
				t.Fatalf("plan-log prose must stay imported with no result: %+v", it)
			}
			texts += it.Summary + "\n"
			for _, f := range it.Facts {
				facts[f.Kind] = append(facts[f.Kind], f.Text)
			}
		}
		joined := func(k string) string { return strings.Join(facts[k], " | ") }
		mustContain(t, "gemsbok test facts", joined("tests"), "299 fast/standard cases", "259 tests", "0 failures")
		mustContain(t, "gemsbok revisions", joined("revision"), "2bcb15dcdb15b4ae0b59fcefd3225a92fcb31b1b", "b081248", "399c3bd2b330c2673fdb8cbf4db474324d42076e")
		mustContain(t, "gemsbok merges", joined("merge"), "PR 28")
		mustContain(t, "gemsbok plan evidence", texts, "24 PDFs")
		var r1 RequirementEvidence
		for _, r := range rep.Evidence.Requirements {
			if r.Number == 1 {
				r1 = r
			}
			if r.Number > 1 && r.Proof != "checked-without-proof" {
				t.Fatalf("requirement %d proof: %s", r.Number, r.Proof)
			}
		}
		if r1.Proof != "imported-only" || len(r1.Items) != 1 || !strings.HasPrefix(r1.Items[0].Summary, "step 1 acceptance passes") {
			t.Fatalf("requirement 1 evidence: %+v", r1)
		}
		h := sectionHTML(t, rep, "report-evidence")
		mustContain(t, "gemsbok evidence HTML", h, "24 PDFs", "Test count", "Revision or hash", "PR 28", "Plan-level evidence")
	})
}

// TESTMASTER: id=kr-evidence-freshness tier=fast parallel=no
func TestKrEvidenceFreshness(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
	project := integrityProject(t)
	git := func(args ...string) string {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", project, "-c", "user.name=fixture", "-c", "user.email=fixture@example.invalid", "-c", "commit.gpgsign=false"}, args...)...)
		cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v: %v %s", args, err, out)
		}
		return strings.TrimSpace(string(out))
	}
	write := func(rel, body string) {
		p := filepath.Join(project, rel)
		os.MkdirAll(filepath.Dir(p), 0o755)
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	git("init", "-q")
	write("cli/parse.go", "package cli\n")
	write("cli/print.go", "package cli\n")
	plan := "# heron\n\nname: heron\nphase: executing\nExecuting: 2026-10-22T09:00:00Z\n\n## Steps\n- [x] 1. Parse.\n- [x] 2. Print.\n\n## Validation\n- [x] 1. Parse cases pass.\n- [x] 2. Print cases pass.\n"
	write(".claude/iterate/plans/heron.md", plan)
	git("add", "-A")
	git("commit", "-q", "-m", "fixture")
	rev1 := git("rev-parse", "HEAD")

	marker := "2026-10-22T09:00:00Z"
	t0 := time.Date(2026, 10, 22, 9, 30, 0, 0, time.UTC)
	observeIntegrity(t, project, "heron", marker, t0,
		runrecord.Event{Type: runrecord.TypeEvidence, ID: "h1", At: t0, Requirement: 1, Evidence: &runrecord.Evidence{Result: "passed", Command: "go test ./cli -run Parse",
			ExitStatus: intp(0), Revision: rev1, CoveredFiles: []string{"cli/parse.go"}, Summary: "parse cases pass"}},
		runrecord.Event{Type: runrecord.TypeEvidence, ID: "h2", At: t0, Requirement: 2, Evidence: &runrecord.Evidence{Result: "passed", Command: "go test ./cli -run Print",
			ExitStatus: intp(0), Revision: rev1, CoveredFiles: []string{"cli/print.go"}, Summary: "print cases pass"}},
	)
	query := url.Values{"project": {project}, "plan": {"heron"}}
	proofs := func() map[int]RequirementEvidence {
		out := map[int]RequirementEvidence{}
		for _, r := range apiReport(t, query).Evidence.Requirements {
			out[r.Number] = r
		}
		return out
	}
	if p := proofs(); p[1].Proof != "observed-passed" || p[2].Proof != "observed-passed" || p[1].Items[0].Freshness != "current" {
		t.Fatalf("before the change: %+v", p)
	}

	// Covered code for requirement 1 changes after its proof.
	write("cli/parse.go", "package cli\n\nfunc Parse() {}\n")
	git("commit", "-q", "-am", "change parse")
	p := proofs()
	r1, r2 := p[1], p[2]
	if r1.Proof != "observed-stale" || !strings.HasPrefix(r1.ProofLabel, "Passed at "+rev1[:12]) {
		t.Fatalf("requirement 1 after its covered code changed: %s %q", r1.Proof, r1.ProofLabel)
	}
	it := r1.Items[0]
	if it.Freshness != "stale" || it.Result != "passed" || it.Revision != rev1 || len(it.ChangedFiles) != 1 || it.ChangedFiles[0] != "cli/parse.go" || it.Command == "" {
		t.Fatalf("stale proof must stay intact and name the change: %+v", it)
	}
	if r2.Proof != "observed-passed" || r2.Items[0].Freshness != "current" {
		t.Fatalf("requirement 2 covers unchanged code: %+v", r2)
	}
	in, err := LoadReportInput(context.Background(), project, "heron", "", false, integrityNow)
	if err != nil {
		t.Fatal(err)
	}
	if in.Now != integrityNow {
		t.Fatalf("clock not injected")
	}
	h := sectionHTML(t, BuildPlanReport(in), "report-evidence")
	mustContain(t, "freshness HTML", h, "Stale", "1 of 1 covered file changed since "+rev1[:12], "cli/parse.go. The proof stays as recorded.", "parse cases pass", "Current", "1</strong> stale")

	// An injected change source: a revision the project does not know
	// leaves freshness unknown, never current.
	orig := evidenceChangedFiles
	t.Cleanup(func() { evidenceChangedFiles = orig })
	evidenceChangedFiles = func(string, string) ([]string, error) {
		return nil, os.ErrNotExist
	}
	if p := proofs(); p[1].Items[0].Freshness != "unknown" || p[1].Proof != "observed-passed" {
		t.Fatalf("unknown change source: %+v", p[1].Items[0])
	}
	calls := 0
	evidenceChangedFiles = func(_, rev string) ([]string, error) {
		calls++
		return []string{"cli/print.go"}, nil
	}
	if p := proofs(); p[2].Proof != "observed-stale" || p[1].Proof != "observed-passed" || calls != 1 {
		t.Fatalf("injected change: r1 %s r2 %s calls %d", p[1].Proof, p[2].Proof, calls)
	}
}
