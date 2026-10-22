package main

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/x85446/claudecodetricks/src/internal/iterrun"
	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// recordEnv isolates one kr-record test: a temporary HOME whose default
// record store (~/.claude/iterate-run/records) is the one under test.
type recordEnv struct {
	t    *testing.T
	home string
}

func newRecordEnv(t *testing.T) *recordEnv {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ITERATE_RUN_RECORD_DIR", "")
	return &recordEnv{t: t, home: home}
}

func (e *recordEnv) storeDir() string {
	return filepath.Join(e.home, ".claude", "iterate-run", "records")
}

// cli runs `iterate-run record` in this process.
func (e *recordEnv) cli(input string, args ...string) (int, string) {
	e.t.Helper()
	var out, diag bytes.Buffer
	code := runRecordCLI(args, strings.NewReader(input), &out, &diag)
	return code, out.String() + diag.String()
}

// restart runs `iterate-run record` as a fresh process.
func (e *recordEnv) restart(input string, args ...string) (int, string) {
	e.t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "ITERATE_RUN_RECORD_CLI_HELPER=1", "HOME="+e.home, "ITERATE_RUN_RECORD_DIR=")
	cmd.Stdin = strings.NewReader(input)
	b, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		ee, ok := err.(*exec.ExitError)
		if !ok {
			e.t.Fatal(err)
		}
		code = ee.ExitCode()
	}
	return code, string(b)
}

func (e *recordEnv) mustAppend(project, plan string, restart bool, event string, wantStatus string) string {
	e.t.Helper()
	args := []string{"append", "--project", project, "--json", "--plan", plan}
	var code int
	var out string
	if restart {
		code, out = e.restart(event, args...)
	} else {
		code, out = e.cli(event, args...)
	}
	var res struct {
		Status string `json:"status"`
		RunID  string `json:"run_id"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &res) != nil || res.Status != wantStatus {
		e.t.Fatalf("append %s: exit %d status %q (want %s): %s", event, code, res.Status, wantStatus, out)
	}
	return res.RunID
}

func (e *recordEnv) show(restart bool, args ...string) iterrun.RecordReport {
	e.t.Helper()
	args = append([]string{"show", "--json"}, args...)
	var code int
	var out string
	if restart {
		code, out = e.restart("", args...)
	} else {
		code, out = e.cli("", args...)
	}
	var rep iterrun.RecordReport
	if code != 0 || json.Unmarshal([]byte(out), &rep) != nil {
		e.t.Fatalf("show %v: exit %d: %s", args, code, out)
	}
	return rep
}

func writePlan(t *testing.T, project, name, executing string) string {
	t.Helper()
	dir := filepath.Join(project, ".claude", "iterate", "plans")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, name+".md")
	body := "name: " + name + "\nphase: executing\nExecuting: " + executing + "\nharness: claude-code\n\n## Goal\nFixture\n## Steps\n- [x] 1. Build it\n## Validation\n- [x] 1. Prove it\n- Validation evidence: go test ./... passed (imported prose)\n"
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func archivePlan(t *testing.T, project, name, archive string) {
	t.Helper()
	dir := filepath.Join(project, ".claude", "iterate", "archive")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(filepath.Join(project, ".claude", "iterate", "plans", name+".md"), filepath.Join(dir, archive)); err != nil {
		t.Fatal(err)
	}
}

func apiReport(t *testing.T, query url.Values) iterrun.RecordReport {
	t.Helper()
	w := httptest.NewRecorder()
	iterrun.RecordAPI().ServeHTTP(w, httptest.NewRequest("GET", "/api/record?"+query.Encode(), nil))
	var rep iterrun.RecordReport
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &rep) != nil {
		t.Fatalf("GET /api/record?%s: %d %s", query.Encode(), w.Code, w.Body)
	}
	return rep
}

func onlyRun(t *testing.T, rep iterrun.RecordReport) runrecord.Run {
	t.Helper()
	if len(rep.Runs) != 1 {
		t.Fatalf("want one run, got %d: %+v", len(rep.Runs), rep)
	}
	return rep.Runs[0]
}

func storeSnapshot(t *testing.T, dir string) string {
	t.Helper()
	var b strings.Builder
	_ = filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err == nil {
			b.WriteString(path)
			if !info.IsDir() {
				data, _ := os.ReadFile(path)
				b.Write(data)
			}
		}
		return nil
	})
	return b.String()
}

// TESTMASTER: id=kr-record-replay tier=fast parallel=no
//
// Given accepted run events and an interrupted write, when the CLI restarts
// and replays duplicates, then durable history is recovered once with its
// original identities.
func TestKrRecordReplay(t *testing.T) {
	env := newRecordEnv(t)
	project := t.TempDir()
	writePlan(t, project, "alpha", "2026-10-20T10:00:00Z")

	accepted := []string{
		`{"id":"e-exec","type":"run.lifecycle","at":"2026-10-20T10:00:00Z","lifecycle":{"state":"executing"}}`,
		`{"id":"e-start","type":"attempt.started","at":"2026-10-20T10:01:00Z","requirement":1,"attempt_key":"k1","session_id":"s1"}`,
		`{"id":"e-u1","type":"usage.observed","at":"2026-10-20T10:02:00Z","requirement":1,"attempt_key":"k1","usage":{"request_id":"req-1","scope":"exact","input_tokens":100,"output_tokens":10,"cumulative":true}}`,
		`{"id":"e-ev","type":"evidence.observed","at":"2026-10-20T10:03:00Z","requirement":1,"attempt_key":"k1","evidence":{"result":"passed","command":"go test ./...","exit_status":0,"revision":"abc123","environment":"darwin/arm64"}}`,
		`{"id":"e-fin","type":"attempt.finished","at":"2026-10-20T10:04:00Z","requirement":1,"attempt_key":"k1","attempt_info":{"outcome":"passed"}}`,
	}
	run := ""
	for _, e := range accepted {
		id := env.mustAppend(project, "alpha", false, e, "accepted")
		if run != "" && id != run {
			t.Fatalf("events split across runs %s and %s", run, id)
		}
		run = id
	}

	// A writer dies mid-line: the record holds a torn trailing fragment.
	runFile := filepath.Join(env.storeDir(), runrecord.ProjectKey(runrecord.CanonicalProject(project)), "alpha", run+".jsonl")
	f, err := os.OpenFile(runFile, os.O_APPEND|os.O_WRONLY, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = f.WriteString(`{"schema_version":1,"id":"e-lost","type":"usage.obs`)
	f.Close()

	// Restart: a fresh process reads accepted history and ignores the tear,
	// without modifying the file.
	before, _ := os.ReadFile(runFile)
	r := onlyRun(t, env.show(true, "--project", project, "--plan", "alpha"))
	if after, _ := os.ReadFile(runFile); !bytes.Equal(before, after) {
		t.Fatal("show modified the record")
	}
	if r.RunID != run || !r.Torn || r.Events != 6 || r.State != "executing" {
		t.Fatalf("after restart: run=%s torn=%v events=%d state=%s", r.RunID, r.Torn, r.Events, r.State)
	}

	// Replay every accepted event from a fresh process: each is a duplicate.
	for _, e := range accepted {
		if id := env.mustAppend(project, "alpha", true, e, "duplicate"); id != run {
			t.Fatalf("replay landed in %s", id)
		}
	}
	// A streaming rewrite of the same request and an out-of-order event.
	env.mustAppend(project, "alpha", true, `{"id":"e-u2","type":"usage.observed","at":"2026-10-20T10:02:30Z","requirement":1,"attempt_key":"k1","usage":{"request_id":"req-1","scope":"exact","input_tokens":150,"output_tokens":12,"cumulative":true}}`, "accepted")
	env.mustAppend(project, "alpha", false, `{"id":"e-queued","type":"run.lifecycle","at":"2026-10-20T09:00:00Z","lifecycle":{"state":"queued"}}`, "accepted")
	env.mustAppend(project, "alpha", false, `{"id":"e-start-again","type":"attempt.started","at":"2026-10-20T10:01:05Z","requirement":1,"attempt_key":"k1"}`, "accepted")

	r = onlyRun(t, env.show(true, "--project", project, "--plan", "alpha"))
	if r.RunID != run || r.Torn || r.Duplicates != 0 || r.Events != 9 {
		t.Fatalf("after replay: run=%s torn=%v dup=%d events=%d", r.RunID, r.Torn, r.Duplicates, r.Events)
	}
	if len(r.Lifecycle) != 2 || r.Lifecycle[0].State != "queued" || r.Lifecycle[0].EventID != "e-queued" || r.Lifecycle[1].EventID != "e-exec" {
		t.Fatalf("out-of-order fold: %+v", r.Lifecycle)
	}
	if len(r.Requirements) != 1 || len(r.Requirements[0].Attempts) != 1 {
		t.Fatalf("duplicates added attempts: %+v", r.Requirements)
	}
	a := r.Requirements[0].Attempts[0]
	if a.Outcome != "passed" || len(a.Evidence) != 1 || a.Evidence[0].EventID != "e-ev" || a.Evidence[0].Revision != "abc123" || *a.Evidence[0].ExitStatus != 0 {
		t.Fatalf("attempt: %+v", a)
	}
	exact := r.Usage.ByScope[runrecord.ScopeExact]
	if exact.Requests != 1 || *exact.InputTokens != 150 || *exact.OutputTokens != 12 {
		t.Fatalf("usage double counted: %+v", exact)
	}
	data, _ := os.ReadFile(runFile)
	if bytes.Contains(data, []byte("e-lost")) {
		t.Fatal("torn fragment kept after repair")
	}
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
		if !json.Valid([]byte(line)) {
			t.Fatalf("corrupt line: %q", line)
		}
	}

	// Archive the plan file; the record follows by identity, not path.
	archive := "20261020T120000Z-alpha-done.md"
	archivePlan(t, project, "alpha", archive)
	if id := env.mustAppend(project, "alpha", false, `{"id":"e-arch","type":"run.lifecycle","lifecycle":{"state":"archived","archive_file":"`+archive+`"}}`, "accepted"); id != run {
		t.Fatalf("archive landed in %s", id)
	}
	byArchive := onlyRun(t, env.show(true, "--project", project, "--archive", archive))
	viaAPI := onlyRun(t, apiReport(t, url.Values{"project": {project}, "archive": {archive}}))
	for _, got := range []runrecord.Run{byArchive, viaAPI} {
		if got.RunID != run || !got.Archived || got.ArchiveFile != archive || got.Events != 10 || got.Plan != "alpha" || got.ProjectPath != runrecord.CanonicalProject(project) {
			t.Fatalf("archived reload: %+v", got)
		}
	}

	// A re-run of the codename opens a new run; the archived one is intact.
	writePlan(t, project, "alpha", "2026-10-21T08:00:00Z")
	rerun := env.mustAppend(project, "alpha", true, `{"id":"e-exec","type":"run.lifecycle","lifecycle":{"state":"executing"}}`, "accepted")
	if rerun == run {
		t.Fatal("re-run reused the archived run id")
	}
	code, out := env.restart("", "show", run, "--json")
	var old runrecord.Run
	if code != 0 || json.Unmarshal([]byte(out), &old) != nil || old.Events != 10 || !old.Archived {
		t.Fatalf("archived run after re-run: %d %s", code, out)
	}
}

// TESTMASTER: id=kr-record-isolation tier=fast parallel=no
//
// Given two projects with the same plan name, when their reports are read
// and archived, then events and evidence stay with their own project and run.
func TestKrRecordIsolation(t *testing.T) {
	env := newRecordEnv(t)
	a, b, legacy := t.TempDir(), t.TempDir(), t.TempDir()
	writePlan(t, a, "finch", "2026-10-20T10:00:00Z")
	writePlan(t, b, "finch", "2026-10-20T10:00:00Z")

	// Identical event ids in both projects: each project's record accepts
	// its own copy and never sees the other's.
	for _, p := range []struct{ dir, proof, tokens string }{{a, "A proof", "111"}, {b, "B proof", "222"}} {
		env.mustAppend(p.dir, "finch", false, `{"id":"same-id-1","type":"evidence.observed","requirement":1,"attempt":1,"evidence":{"result":"passed","summary":"`+p.proof+`"}}`, "accepted")
		env.mustAppend(p.dir, "finch", true, `{"id":"same-id-2","type":"usage.observed","requirement":1,"attempt":1,"usage":{"request_id":"req","scope":"exact","input_tokens":`+p.tokens+`}}`, "accepted")
	}
	check := func(rep iterrun.RecordReport, project, proof string, tokens int64) runrecord.Run {
		t.Helper()
		r := onlyRun(t, rep)
		if r.ProjectPath != runrecord.CanonicalProject(project) || len(r.Requirements) != 1 || len(r.Requirements[0].Attempts) != 1 {
			t.Fatalf("%s: %+v", project, r)
		}
		ev := r.Requirements[0].Attempts[0].Evidence
		if len(ev) != 1 || ev[0].Summary != proof || *r.Usage.ByScope[runrecord.ScopeExact].InputTokens != tokens {
			t.Fatalf("%s mixed evidence or usage: %+v %+v", project, ev, r.Usage.ByScope)
		}
		return r
	}
	ra := check(env.show(true, "--plan", "finch", "--project", a), a, "A proof", 111)
	rb := check(apiReport(t, url.Values{"project": {b}, "plan": {"finch"}}), b, "B proof", 222)
	if ra.RunID == rb.RunID || ra.ProjectKey == rb.ProjectKey {
		t.Fatal("same-named plans share a run or project key")
	}

	// Flags anywhere, same answer.
	_, first := env.cli("", "--json", "list", "--plan", "finch")
	_, second := env.cli("", "list", "--plan", "finch", "--json")
	var listing struct {
		Runs []iterrun.RunSummary `json:"runs"`
	}
	if first != second || json.Unmarshal([]byte(first), &listing) != nil || len(listing.Runs) != 2 || listing.Runs[0].ProjectPath == listing.Runs[1].ProjectPath {
		t.Fatalf("list: %s / %s", first, second)
	}

	// Both archive under the same archive filename and stay apart.
	archive := "20261020T120000Z-finch-done.md"
	for _, p := range []string{a, b} {
		archivePlan(t, p, "finch", archive)
		env.mustAppend(p, "finch", false, `{"id":"arch","type":"run.lifecycle","lifecycle":{"state":"archived","archive_file":"`+archive+`"}}`, "accepted")
	}
	if r := check(env.show(true, "--project", a, "--archive", archive), a, "A proof", 111); r.RunID != ra.RunID || !r.Archived {
		t.Fatalf("archived A: %+v", r)
	}
	if r := check(apiReport(t, url.Values{"project": {b}, "archive": {archive}}), b, "B proof", 222); r.RunID != rb.RunID || !r.Archived {
		t.Fatalf("archived B: %+v", r)
	}

	// A third project with the same codename and no record: the legacy view
	// is read-only, carries provenance and states its unknowns.
	writePlan(t, legacy, "finch", "2026-09-01T10:00:00Z")
	archivePlan(t, legacy, "finch", "20260901T130000Z-finch-done.md")
	before := storeSnapshot(t, env.storeDir())
	rep := env.show(true, "--project", legacy, "--plan", "finch")
	lr := onlyRun(t, rep)
	if rep.Source != "legacy" || !strings.HasPrefix(lr.Provenance, "legacy:") || lr.ProjectPath != runrecord.CanonicalProject(legacy) || !lr.Archived {
		t.Fatalf("legacy view: %+v", rep)
	}
	for _, want := range []string{"usage", "versions.model", "ownership"} {
		found := false
		for _, u := range lr.Unknown {
			found = found || u == want
		}
		if !found {
			t.Fatalf("legacy unknowns %v lack %s", lr.Unknown, want)
		}
	}
	imported := false
	for _, q := range lr.Requirements {
		for _, e := range q.Evidence {
			imported = imported || (e.Imported && e.Result == "unknown" && strings.Contains(e.Summary, "imported prose"))
		}
	}
	if !imported {
		t.Fatalf("prose evidence not marked imported: %+v", lr.Requirements)
	}
	if after := storeSnapshot(t, env.storeDir()); after != before {
		t.Fatal("a legacy read wrote to the record store")
	}

	// Explicit import persists it with provenance; importing again is a no-op.
	code, out := env.restart("", "import", "--project", legacy, "--plan", "finch", "--json")
	var res struct {
		RunID    string `json:"run_id"`
		Accepted int    `json:"accepted"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &res) != nil || res.Accepted != lr.Events || res.RunID != lr.RunID {
		t.Fatalf("import: %d %s", code, out)
	}
	code, out = env.restart("", "import", "--project", legacy, "--plan", "finch", "--json")
	if code != 0 || json.Unmarshal([]byte(out), &res) != nil || res.Accepted != 0 {
		t.Fatalf("re-import: %d %s", code, out)
	}
	rep = env.show(false, "--project", legacy, "--plan", "finch")
	if r := onlyRun(t, rep); rep.Source != "record" || r.RunID != lr.RunID || r.Provenance != lr.Provenance {
		t.Fatalf("imported: %+v", rep)
	}
	// A and B are untouched by the third project's import.
	check(env.show(false, "--project", a, "--archive", archive), a, "A proof", 111)
	if dirs, _ := filepath.Glob(filepath.Join(env.storeDir(), "*", "finch")); len(dirs) != 3 {
		t.Fatalf("want three project keys for finch, got %v", dirs)
	}
}

// TESTMASTER: id=record-cli-usage-errors tier=fast parallel=yes
func TestRecordCLIUsageErrors(t *testing.T) {
	env := newRecordEnv(t)
	project := t.TempDir()
	cases := []struct {
		args  []string
		input string
		code  int
	}{
		{[]string{"bogus"}, "", 2},
		{[]string{"show", "--nope"}, "", 2},
		{[]string{"append", "--project", project, "--plan", "alpha"}, `{}`, 2},
		{[]string{"append", "--project", project, "--plan", "alpha", "--type", "run.lifecycle"}, `{"lifecycle":{"state":"sleeping"}}`, 2},
		{[]string{"append", "--project", project, "--plan", "alpha", "--type", "run.lifecycle"}, `{"lifecycle":{"state":"paused"},"surprise":1}`, 2},
		{[]string{"show", "--project", project, "--plan", "alpha"}, "", 3},
		{[]string{"show", "run-missing"}, "", 3},
		{[]string{"--help"}, "", 0},
		{[]string{"types", "--json"}, "", 0},
	}
	for _, c := range cases {
		if code, out := env.cli(c.input, c.args...); code != c.code {
			t.Fatalf("%v: exit %d, want %d: %s", c.args, code, c.code, out)
		}
	}
	if _, err := os.Stat(env.storeDir()); !os.IsNotExist(err) {
		t.Fatal("rejected commands wrote to the record store")
	}
	if args, ok := recordInvocation([]string{"--json", "record", "list"}); !ok || strings.Join(args, " ") != "--json list" {
		t.Fatalf("pre-noun flags: %v %v", args, ok)
	}
}
