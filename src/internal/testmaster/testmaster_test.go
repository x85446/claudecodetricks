package testmaster

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func project(t *testing.T, tests map[string]*Entry) *Project {
	t.Helper()
	p, err := Open(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for id, e := range tests {
		if e.Tier == "" {
			e.Tier = "?"
		}
		p.Reg.Tests[id] = e
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	return p
}

func runAll(t *testing.T, p *Project, opt RunOptions) *Report {
	t.Helper()
	if opt.Jobs == 0 {
		opt.Jobs = 4
	}
	if opt.Timeout == 0 {
		opt.Timeout = time.Minute
	}
	ids, label, err := p.Select([]string{"all"}, false)
	if err != nil {
		t.Fatal(err)
	}
	rep, err := p.Run(context.Background(), ids, label, opt)
	if err != nil {
		t.Fatal(err)
	}
	return rep
}

func statuses(rep *Report) map[string]string {
	out := map[string]string{}
	for _, r := range rep.Problems {
		out[r.ID] = r.Status
	}
	return out
}

func history(t *testing.T, p *Project) []map[string]any {
	t.Helper()
	f, err := os.Open(filepath.Join(p.Root, StateDir, "history.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	var out []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("history line %q: %v", sc.Text(), err)
		}
		out = append(out, m)
	}
	return out
}

func need(t *testing.T, tools ...string) {
	t.Helper()
	for _, tool := range tools {
		if _, err := exec.LookPath(tool); err != nil {
			t.Skipf("%s not on PATH: this adapter is not exercised here", tool)
		}
	}
}

// A registry written before the runner existed loads, gets kinds, and keeps
// every field the runner does not know about.
func TestRegistryRoundTripKeepsUnknownFieldsAndNormalizesLegacyEntries(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, StateDir+"/registry.json", `{"schema":1,"owner":"someone","tests":{
	  "git.TestX":{"name":"TestX","pkg":"./src/internal/git","cmd":"go test ./src/internal/git -run '^TestX$'","runs":3,"avg_ms":12,"tier":"fast","covers_hint":"x.go"},
	  "dotfiles.check":{"name":"check","cmd":"make statusline-check","runner":"make","runs":1,"avg_ms":4000,"tier":"fast"}}}`)
	p, err := Open(dir)
	if err != nil {
		t.Fatal(err)
	}
	if k := p.Reg.Tests["git.TestX"].Kind; k != "go" {
		t.Errorf("go test entry kind = %q, want go", k)
	}
	if k := p.Reg.Tests["dotfiles.check"].Kind; k != "shell" {
		t.Errorf("make entry kind = %q, want shell", k)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, StateDir, "registry.json"))
	for _, want := range []string{`"owner": "someone"`, `"covers_hint": "x.go"`, `"kind": "shell"`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("saved registry lost %s:\n%s", want, b)
		}
	}
	if strings.Contains(string(b), `"runner"`) {
		t.Errorf("legacy runner key survived next to kind:\n%s", b)
	}
}

// Passing tests leave nothing behind but a count and timing; each failure
// leaves a log with the full output and the command that reruns it; a test
// that passes again has its stale log removed.
func TestShellPassIsSilentAndFailureLeavesALog(t *testing.T) {
	p := project(t, map[string]*Entry{
		"ok":   {Kind: "shell", Cmd: "echo fine"},
		"bad":  {Kind: "shell", Cmd: "echo the detail the AI needs; echo on stderr >&2; exit 3"},
		"skip": {Kind: "shell", Cmd: "exit 77"},
	})
	rep := runAll(t, p, RunOptions{})
	if rep.Counts[Pass] != 1 || rep.Counts[Fail] != 1 || rep.Counts[Skip] != 1 {
		t.Fatalf("counts = %v", rep.Counts)
	}
	if rep.ExitCode() != 1 {
		t.Errorf("exit = %d, want 1", rep.ExitCode())
	}
	log, err := os.ReadFile(p.LogPath("bad"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"the detail the AI needs", "on stderr", "exit:    3", "rerun:   echo the detail"} {
		if !strings.Contains(string(log), want) {
			t.Errorf("failure log missing %q:\n%s", want, log)
		}
	}
	if _, err := os.Stat(p.LogPath("ok")); err == nil {
		t.Error("a passing test wrote a failure log")
	}
	var out strings.Builder
	rep.Print(&out, p, p.Root)
	if strings.Contains(out.String(), "\tok") || strings.Count(out.String(), "\n") != 2 {
		t.Errorf("report should be one summary line plus one line for the failure:\n%s", out.String())
	}

	p.Reg.Tests["bad"].Cmd = "true"
	rep = runAll(t, p, RunOptions{})
	if rep.ExitCode() != 0 {
		t.Fatalf("all green should exit 0, counts %v", rep.Counts)
	}
	if _, err := os.Stat(p.LogPath("bad")); err == nil {
		t.Error("a test that passes again kept its old failure log")
	}
	e := p.Reg.Tests["ok"]
	if e.Runs != 2 || e.AvgMs == nil || e.Tier != "fast" || e.LastResult != Pass {
		t.Errorf("registry not updated from measurement: %+v", e)
	}
	if p.Reg.LastRun == nil || p.Reg.LastRun.Passed != 2 {
		t.Errorf("last_run = %+v", p.Reg.LastRun)
	}
	h := history(t, p)
	var skipHasMs bool
	for _, line := range h {
		if line["id"] == "skip" {
			_, skipHasMs = line["ms"]
		}
	}
	if skipHasMs {
		t.Error("a skip wrote a duration into history: only real executions are timing data")
	}
	if last := h[len(h)-1]; last["batch"] != "all" {
		t.Errorf("last history line should be the batch record, got %v", last)
	}
}

// Independent tests all run however many fail; a blocking test that fails
// stops the run before anything else starts.
func TestBlockingFailureStopsTheRunButIndependentFailuresDoNot(t *testing.T) {
	tests := map[string]*Entry{}
	for i := 0; i < 30; i++ {
		cmd := "true"
		if i%3 == 0 {
			cmd = "false"
		}
		tests["t"+string(rune('a'+i/10))+string(rune('0'+i%10))] = &Entry{Kind: "shell", Cmd: cmd}
	}
	p := project(t, tests)
	rep := runAll(t, p, RunOptions{})
	if rep.Counts[Pass] != 20 || rep.Counts[Fail] != 10 {
		t.Fatalf("all 30 independent tests should run: %v", rep.Counts)
	}

	marker := filepath.Join(p.Root, "ran")
	p.Reg.Tests["build"] = &Entry{Kind: "shell", Cmd: "exit 1", Blocking: true, Tier: "?"}
	p.Reg.Tests["after"] = &Entry{Kind: "shell", Cmd: "touch " + marker, Tier: "?"}
	rep = runAll(t, p, RunOptions{})
	if rep.BlockedBy != "build" || rep.Counts[NotRun] != 31 || rep.Counts[Fail] != 1 {
		t.Fatalf("blocked run: blockedBy=%q counts=%v", rep.BlockedBy, rep.Counts)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("a test ran after its blocking test failed")
	}
	if p.Reg.Tests["after"].LastResult != "" {
		t.Error("a test that never ran got a result")
	}
}

// parallel=no tests never overlap anything; parallel tests do overlap.
func TestSerialTestsRunAlone(t *testing.T) {
	dir := t.TempDir()
	lock := filepath.Join(dir, "busy")
	guard := "if [ -e " + lock + " ]; then echo overlap; exit 1; fi; touch " + lock + "; sleep 0.3; rm " + lock
	f := false
	p := project(t, map[string]*Entry{
		"s1": {Kind: "shell", Cmd: guard, ParallelSafe: &f},
		"s2": {Kind: "shell", Cmd: guard, ParallelSafe: &f},
		"s3": {Kind: "shell", Cmd: guard, ParallelSafe: &f},
	})
	if rep := runAll(t, p, RunOptions{Jobs: 8}); rep.Counts[Pass] != 3 {
		t.Fatalf("serial tests overlapped: %v %v", rep.Counts, statuses(rep))
	}
	start := time.Now()
	p2 := project(t, map[string]*Entry{
		"p1": {Kind: "shell", Cmd: "sleep 0.5"}, "p2": {Kind: "shell", Cmd: "sleep 0.5"}, "p3": {Kind: "shell", Cmd: "sleep 0.5"},
	})
	runAll(t, p2, RunOptions{Jobs: 8})
	if d := time.Since(start); d > 1400*time.Millisecond {
		t.Errorf("parallel-safe tests took %s: they did not run concurrently", d)
	}
}

// A test whose measured average crosses a boundary is announced, and the
// slow tier drops out of the default selection while unmeasured tests stay in.
func TestTierChangeIsReportedAndDefaultSelection(t *testing.T) {
	slow := int64(200_000)
	fast := int64(5)
	p := project(t, map[string]*Entry{
		"was-fast": {Kind: "shell", Cmd: "true", Runs: 1, AvgMs: &fast, Tier: "fast"},
		"slow":     {Kind: "shell", Cmd: "true", Runs: 1, AvgMs: &slow, Tier: "slow"},
		"new":      {Kind: "shell", Cmd: "true"},
	})
	ids, label, err := p.Select(nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(ids, ",") != "new,was-fast" || label != "fast+standard" {
		t.Errorf("default selection = %v (%s), want new,was-fast", ids, label)
	}
	if _, _, err := p.Select([]string{"was-fats"}, false); err == nil || !strings.Contains(err.Error(), "did you mean was-fast") {
		t.Errorf("unknown id error = %v", err)
	}
	if ids, _, _ := p.Select([]string{"was-*"}, false); len(ids) != 1 {
		t.Errorf("glob selected %v", ids)
	}

	// One measured run at 15s (standard); a 50ms run halves the average to ~7.5s (fast).
	prev := int64(15_000)
	e := p.Reg.Tests["was-fast"]
	e.Cmd, e.Runs, e.AvgMs, e.Tier = "sleep 0.05", 1, &prev, "standard"
	rep, err := p.Run(context.Background(), []string{"was-fast"}, "x", RunOptions{Jobs: 1, Timeout: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if len(rep.TierChanges) != 1 || rep.TierChanges[0].From != "standard" || rep.TierChanges[0].To != "fast" {
		t.Errorf("tier change not reported: %+v", rep.TierChanges)
	}

	p.Reg.Tests["was-fast"].LastResult = Fail
	if ids, label, _ := p.Select(nil, true); strings.Join(ids, ",") != "was-fast" || label != "failed" {
		t.Errorf("--failed selected %v (%s)", ids, label)
	}
}

func TestShellTimeoutKillsTheWholeProcessGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "survived")
	p := project(t, map[string]*Entry{
		"hang": {Kind: "shell", Cmd: "(sleep 2; touch " + marker + ") & sleep 60"},
	})
	rep := runAll(t, p, RunOptions{Timeout: 300 * time.Millisecond})
	if rep.Counts[Timeout] != 1 {
		t.Fatalf("counts = %v", rep.Counts)
	}
	time.Sleep(2500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Error("a grandchild outlived the timeout")
	}
	if p.Reg.Tests["hang"].Runs != 0 {
		t.Error("a timeout was recorded as a measured duration")
	}
}

const goFixture = `package fx

import (
	"os"
	"testing"
	"time"
)

func TestPass(t *testing.T) {}
func TestFail(t *testing.T) { t.Log("why it failed"); t.Fatal("boom") }
func TestSkip(t *testing.T) { t.Skip("not here") }
func TestSub(t *testing.T) {
	t.Run("ok", func(t *testing.T) {})
	t.Run("bad", func(t *testing.T) { t.Error("subtest broke") })
}
func TestCrash(t *testing.T) { panic("crashed the binary") }
func TestZAfterCrash(t *testing.T) {}
func TestHang(t *testing.T) {
	if os.Getenv("FX_HANG") != "" {
		time.Sleep(time.Hour)
	}
}
`

// One go test invocation per package; a panic that kills the test binary
// does not take the results of the package's other tests with it.
func TestGoAdapterBatchesAPackageAndSurvivesACrash(t *testing.T) {
	need(t, "go")
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "go.mod", "module fx\n\ngo 1.21\n")
	write(t, dir, "fx/fx_test.go", goFixture)
	p, _ := Open(dir)
	reps, err := p.Discover(context.Background(), nil, false)
	if err != nil || len(reps) != 1 || reps[0].Err != nil {
		t.Fatalf("discover: %v %+v", err, reps)
	}
	if len(reps[0].New) != 7 || p.Reg.Tests["fx.TestPass"] == nil || p.Reg.Tests["fx.TestPass"].File != "fx/fx_test.go" {
		t.Fatalf("discover found %v", reps[0].New)
	}
	p.Reg.Tests["fx.TestGone"] = &Entry{Kind: "go", Pkg: "./fx", Name: "TestGone", Tier: "?"}

	rep := runAll(t, p, RunOptions{})
	want := map[string]string{
		"fx.TestFail": Fail, "fx.TestSkip": Skip, "fx.TestSub": Fail, "fx.TestCrash": Fail, "fx.TestGone": Missing,
	}
	got := statuses(rep)
	for id, st := range want {
		if got[id] != st {
			t.Errorf("%s = %q, want %q (all: %v)", id, got[id], st, got)
		}
	}
	if rep.Counts[Pass] != 3 {
		t.Errorf("TestPass, TestHang and TestZAfterCrash should pass despite the crash: %v", got)
	}
	log, _ := os.ReadFile(p.LogPath("fx.TestSub"))
	if !strings.Contains(string(log), "subtest broke") || strings.Contains(string(log), "why it failed") {
		t.Errorf("a test's log should hold its own output (subtests included) and nobody else's:\n%s", log)
	}
	log, _ = os.ReadFile(p.LogPath("fx.TestCrash"))
	if !strings.Contains(string(log), "crashed the binary") {
		t.Errorf("crash log lacks the panic:\n%s", log)
	}

	again, _ := p.Discover(context.Background(), nil, false)
	if len(again[0].New) != 0 || strings.Join(again[0].Missing, ",") != "fx.TestGone" {
		t.Errorf("second discover: new %v missing %v", again[0].New, again[0].Missing)
	}
}

func TestGoTimeoutNamesTheHungTestAndRunsTheRest(t *testing.T) {
	need(t, "go")
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "go.mod", "module fx\n\ngo 1.21\n")
	write(t, dir, "fx/fx_test.go", goFixture)
	p := project(t, nil)
	p.Root = dir
	for _, n := range []string{"TestHang", "TestPass"} {
		p.Reg.Tests["fx."+n] = &Entry{Kind: "go", Pkg: "./fx", Name: n, Tier: "?"}
	}
	p.Reg.Runners = map[string][]string{"go": {"env", "FX_HANG=1", "go"}}
	rep := runAll(t, p, RunOptions{Timeout: 2 * time.Second})
	if got := statuses(rep); got["fx.TestHang"] != Timeout || rep.Counts[Pass] != 1 {
		t.Errorf("want TestHang timeout and TestPass passing, got %v %v", got, rep.Counts)
	}
}

func TestGoBuildFailureCollapsesToOneLine(t *testing.T) {
	need(t, "go")
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "go.mod", "module fx\n\ngo 1.21\n")
	write(t, dir, "fx/fx_test.go", "package fx\n\nimport \"testing\"\n\nfunc TestA(t *testing.T) { undefinedThing() }\nfunc TestB(t *testing.T) {}\n")
	p := project(t, nil)
	p.Root = dir
	for _, n := range []string{"TestA", "TestB"} {
		p.Reg.Tests["fx."+n] = &Entry{Kind: "go", Pkg: "./fx", Name: n, Tier: "?"}
	}
	rep := runAll(t, p, RunOptions{})
	if rep.Counts[Fail] != 2 {
		t.Fatalf("both tests fail when their package does not build: %v", rep.Counts)
	}
	var out strings.Builder
	rep.Print(&out, p, dir)
	if lines := strings.Split(strings.TrimSpace(out.String()), "\n"); len(lines) != 2 || !strings.Contains(lines[1], "2 tests in ./fx") {
		t.Errorf("a shared cause should report once:\n%s", out.String())
	}
	log, _ := os.ReadFile(p.LogPath("fx.TestA"))
	if !strings.Contains(string(log), "undefinedThing") {
		t.Errorf("build log missing from failure log:\n%s", log)
	}
}

func TestPytestAdapter(t *testing.T) {
	need(t, "pytest")
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "pytest.ini", "[pytest]\n")
	write(t, dir, "tests/test_fx.py", `import pytest

def test_pass():
    pass

def test_fail():
    print("captured line")
    assert 1 == 2, "numbers differ"

@pytest.mark.skip(reason="not here")
def test_skip():
    pass

@pytest.mark.parametrize("n", [1, 2])
def test_param(n):
    assert n == 1

class TestGroup:
    def test_method(self):
        pass
`)
	p, _ := Open(dir)
	reps, err := p.Discover(context.Background(), nil, false)
	if err != nil || len(reps) != 1 || reps[0].Err != nil {
		t.Fatalf("discover: %v %+v", err, reps)
	}
	if len(reps[0].New) != 6 {
		t.Fatalf("discover found %v", reps[0].New)
	}
	p.Reg.Tests["tests/test_fx.py::test_gone"] = &Entry{Kind: "pytest", NodeID: "tests/test_fx.py::test_gone", Tier: "?"}
	p.Reg.Tests["whole-param"] = &Entry{Kind: "pytest", NodeID: "tests/test_fx.py::test_param", Tier: "?"}
	rep := runAll(t, p, RunOptions{})
	got := statuses(rep)
	want := map[string]string{
		"tests/test_fx.py::test_fail": Fail, "tests/test_fx.py::test_skip": Skip,
		"tests/test_fx.py::test_param[2]": Fail, "tests/test_fx.py::test_gone": Missing, "whole-param": Fail,
	}
	for id, st := range want {
		if got[id] != st {
			t.Errorf("%s = %q, want %q (all: %v)", id, got[id], st, got)
		}
	}
	if rep.Counts[Pass] != 3 {
		t.Errorf("want test_pass, test_param[1], TestGroup::test_method passing: %v", rep.Counts)
	}
	log, _ := os.ReadFile(p.LogPath("tests/test_fx.py::test_fail"))
	if !strings.Contains(string(log), "numbers differ") || !strings.Contains(string(log), "captured line") {
		t.Errorf("pytest failure log lacks the assertion or captured output:\n%s", log)
	}
}

func TestCargoAdapter(t *testing.T) {
	need(t, "cargo")
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "Cargo.toml", "[package]\nname = \"fx\"\nversion = \"0.1.0\"\nedition = \"2021\"\n")
	write(t, dir, "src/lib.rs", `pub fn add(a: i32, b: i32) -> i32 { a + b }

#[cfg(test)]
mod tests {
    #[test]
    fn adds() { assert_eq!(super::add(1, 2), 3); }
    #[test]
    fn breaks() { println!("captured"); assert_eq!(super::add(1, 1), 3, "math is off"); }
    #[test]
    #[ignore]
    fn ignored() {}
}
`)
	write(t, dir, "tests/integration.rs", "#[test]\nfn from_outside() { assert_eq!(fx::add(2, 2), 4); }\n")
	p, _ := Open(dir)
	reps, err := p.Discover(context.Background(), nil, false)
	if err != nil || len(reps) != 1 || reps[0].Err != nil {
		t.Fatalf("discover: %v %+v", err, reps)
	}
	if strings.Join(reps[0].New, ",") != "fx::tests::adds,fx::tests::breaks,fx::tests::ignored,integration::from_outside" {
		t.Fatalf("discover found %v", reps[0].New)
	}
	p.Reg.Tests["fx::tests::gone"] = &Entry{Kind: "cargo", Pkg: "fx", Target: "lib:fx", Name: "tests::gone", Tier: "?"}
	rep := runAll(t, p, RunOptions{})
	got := statuses(rep)
	want := map[string]string{"fx::tests::breaks": Fail, "fx::tests::ignored": Skip, "fx::tests::gone": Missing}
	for id, st := range want {
		if got[id] != st {
			t.Errorf("%s = %q, want %q (all: %v)", id, got[id], st, got)
		}
	}
	if rep.Counts[Pass] != 2 {
		t.Errorf("want adds and from_outside passing: %v", rep.Counts)
	}
	log, _ := os.ReadFile(p.LogPath("fx::tests::breaks"))
	if !strings.Contains(string(log), "math is off") || !strings.Contains(string(log), "cargo test -p fx --lib -- --exact tests::breaks") {
		t.Errorf("cargo failure log lacks the panic or the rerun command:\n%s", log)
	}
}

func TestDetectSources(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "go.mod", "module x\n")
	write(t, dir, "py/pyproject.toml", "[tool.pytest.ini_options]\n")
	write(t, dir, "rs/Cargo.toml", "[package]\n")
	write(t, dir, ".hidden/go.mod", "module y\n")
	got := DetectSources(dir)
	var parts []string
	for _, s := range got {
		parts = append(parts, s.Kind+"@"+s.Dir)
	}
	if strings.Join(parts, " ") != "go@. pytest@py cargo@rs" {
		t.Errorf("sources = %v", parts)
	}
}

// A green run stamps every catalog case its test backs (by case id, or by a
// scenario case's `test` field) and leaves cases of red tests alone.
func TestGreenRunStampsCatalogCases(t *testing.T) {
	need(t, "git")
	p := project(t, map[string]*Entry{
		"ok":  {Kind: "shell", Cmd: "true && true"},
		"ok2": {Kind: "shell", Cmd: "test -d ."},
		"bad": {Kind: "shell", Cmd: "false"},
	})
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "x"}} {
		if out, err := exec.Command("git", append([]string{"-C", p.Root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	write(t, p.Root, StateDir+"/catalog.json", `{"schema":1,"requirements":[{"id":"r","statement":"→ kept","cases":[
	  {"id":"ok","covers":["a.go"]},
	  {"id":"sc-1","test":"ok","then":"it ends with 📥2"},
	  {"id":"both","test":["ok","ok2"]},
	  {"id":"half","test":["ok","bad"]},
	  {"id":"bad","covers":["b.go"]}]}]}`)
	runAll(t, p, RunOptions{})
	b, _ := os.ReadFile(filepath.Join(p.Root, StateDir, "catalog.json"))
	var cat struct {
		Requirements []struct {
			Statement string
			Cases     []map[string]any
		}
	}
	if err := json.Unmarshal(b, &cat); err != nil {
		t.Fatal(err)
	}
	stamped := map[string]bool{}
	for _, c := range cat.Requirements[0].Cases {
		stamped[c["id"].(string)] = c["last_validated_commit"] != nil
	}
	if !stamped["ok"] || !stamped["sc-1"] || !stamped["both"] || stamped["half"] || stamped["bad"] {
		t.Errorf("stamps = %v, want ok, sc-1 and both only", stamped)
	}
	unlinked, err := p.Unlinked()
	if err != nil || len(unlinked) != 0 {
		t.Errorf("Unlinked = %v, %v; every link names a registered test", unlinked, err)
	}
	if !strings.Contains(string(b), "📥2") || cat.Requirements[0].Statement != "→ kept" {
		t.Errorf("catalog text mangled:\n%s", b)
	}
	reg, _ := os.ReadFile(filepath.Join(p.Root, StateDir, "registry.json"))
	if !strings.Contains(string(reg), `"true && true"`) {
		t.Errorf("registry escaped the command:\n%s", reg)
	}
}
