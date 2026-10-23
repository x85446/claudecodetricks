package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/x85446/claudecodetricks/src/internal/iterrun"
)

// iterateRun runs the whole iterate-run main as a fresh process, the way a
// user's shell does, and returns stdout, stderr and the exit code.
func iterateRun(t *testing.T, env []string, args ...string) (string, string, int) {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^$", "--"}, args...)...)
	base := []string{"ITERATE_RUN_MAIN_HELPER=1", "TERM=xterm-256color", "LANG=en_US.UTF-8"}
	for _, v := range os.Environ() {
		k, _, _ := strings.Cut(v, "=")
		switch k {
		case "TERM", "LANG", "LC_ALL", "LC_CTYPE", "NO_COLOR", "FORCE_COLOR", "CLICOLOR_FORCE", "CI", "ITERATE_RUN_COLOR", "ITERATE_RUN_NO_COLOR":
		default:
			base = append(base, v)
		}
	}
	cmd.Env = append(base, env...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	err := cmd.Run()
	code := 0
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		code = exit.ExitCode()
	} else if err != nil {
		t.Fatal(err)
	}
	return out.String(), errb.String(), code
}

func graphFixture(t *testing.T, plans map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	base := filepath.Join(dir, ".claude", "iterate", "plans")
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, keys := range plans {
		body := "# Iterate Task — " + name + "\n\nname: " + name + "\n" + keys + "\n\n## Goal\ng\n\n## Steps\n- [ ] 1. x\n\n## Validation\n- [ ] 1. y\n"
		if err := os.WriteFile(filepath.Join(base, name+".md"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func fanOut(t *testing.T) string {
	return graphFixture(t, map[string]string{
		"alpha":   "phase: planned\nstatus: queued: 2026-10-22T00:00:00Z by /ip stage\nmilestone: M1-01",
		"bravo":   "phase: planned\nmilestone: M1-02\ndepends: alpha",
		"charlie": "phase: planned\nmilestone: M1-03\ndepends: alpha",
	})
}

// TESTMASTER: id=dag-cli-1 tier=fast parallel=yes
func TestDagCli1TextNestsEachPlanUnderItsPredecessor(t *testing.T) {
	out, errOut, code := iterateRun(t, nil, "plan", "graph", "-C", fanOut(t))
	want := "● alpha  M1-01  queued · ready\n" +
		"├─ ◌ bravo  M1-02  planned · waiting on alpha (queued)\n" +
		"└─ ◌ charlie  M1-03  planned · waiting on alpha (queued)\n"
	if code != 0 || out != want {
		t.Fatalf("exit %d\n%s\nwant\n%s", code, out, want)
	}
	if !strings.Contains(errOut, "3 plans: 1 ready, 2 waiting, 0 blocked") {
		t.Fatalf("stderr summary = %q", errOut)
	}
	ascii, _, _ := iterateRun(t, []string{"LANG=C"}, "plan", "graph", "-C", fanOut(t))
	if !strings.Contains(ascii, `\- ~ charlie`) || strings.ContainsAny(ascii, "●◌├└") {
		t.Fatalf("ASCII fallback = %q", ascii)
	}
}

// TESTMASTER: id=dag-cli-2 tier=fast parallel=yes
func TestDagCli2MermaidHasOneArrowPerEdge(t *testing.T) {
	out, _, code := iterateRun(t, nil, "plan", "graph", "--format", "mermaid", "-C", fanOut(t))
	if code != 0 || !strings.HasPrefix(out, "flowchart") {
		t.Fatalf("exit %d: %q", code, out)
	}
	if arrows := strings.Count(out, "-->") + strings.Count(out, "-.->"); arrows != 2 {
		t.Fatalf("arrows = %d, want 2\n%s", arrows, out)
	}
	if strings.Count(out, "\"]:::") != 3 {
		t.Fatalf("want one node per plan:\n%s", out)
	}
}

// TESTMASTER: id=dag-cli-3 tier=fast parallel=yes
func TestDagCli3JSONIsTheGraphViewDocument(t *testing.T) {
	dir := fanOut(t)
	out, _, code := iterateRun(t, nil, "plan", "graph", "--json", "-C", dir)
	if code != 0 {
		t.Fatalf("exit %d", code)
	}
	var got iterrun.GraphView
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatal(err)
	}
	want, err := iterrun.BuildGraphView(dir)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := json.Marshal(got)
	b, _ := json.Marshal(want)
	if !bytes.Equal(a, b) {
		t.Fatalf("CLI JSON differs from the graph view\n%s\n%s", a, b)
	}
	same, _, _ := iterateRun(t, nil, "plan", "graph", "--format", "json", "-C", dir)
	if same != out {
		t.Fatal("--json and --format json differ")
	}
	if strings.Contains(out, "\x1b[") {
		t.Fatal("machine output carries escapes")
	}
}

// TESTMASTER: id=dag-cli-4 tier=fast parallel=yes
func TestDagCli4CheckFailsOnACycleAndAnUnknownName(t *testing.T) {
	bad := graphFixture(t, map[string]string{
		"alpha": "phase: planned\ndepends: bravo",
		"bravo": "phase: planned\ndepends: alpha",
		"delta": "phase: planned\ndepends: ghost",
	})
	out, _, code := iterateRun(t, nil, "plan", "graph", "--check", "-C", bad)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != 1 || len(lines) != 2 || !strings.Contains(out, "cycle alpha → bravo → alpha") || !strings.Contains(out, "unknown dependency ghost") {
		t.Fatalf("exit %d\n%s", code, out)
	}
	out, errOut, code := iterateRun(t, nil, "plan", "graph", "--check", "-C", fanOut(t))
	if code != 0 || out != "" || errOut != "" {
		t.Fatalf("clean check: exit %d out %q err %q", code, out, errOut)
	}
}

// TESTMASTER: id=dag-cli-5 tier=fast parallel=yes
func TestDagCli5PlansWithoutEdgesAreAllRoots(t *testing.T) {
	dir := graphFixture(t, map[string]string{"alpha": "phase: planned", "bravo": "phase: planned"})
	out, _, code := iterateRun(t, nil, "plan", "graph", "-C", dir)
	if code != 0 || out != "● alpha  planned · ready\n● bravo  planned · ready\n" {
		t.Fatalf("exit %d\n%q", code, out)
	}
}

// TESTMASTER: id=dag-cli-6 tier=fast parallel=yes
func TestDagCli6FlagsParseInEveryPosition(t *testing.T) {
	dir := fanOut(t)
	a, _, ca := iterateRun(t, nil, "-C", dir, "plan", "graph", "--format", "json")
	b, _, cb := iterateRun(t, nil, "plan", "-C", dir, "graph", "--format", "json")
	c, _, cc := iterateRun(t, nil, "plan", "graph", "--format", "json", "-C", dir)
	if ca != 0 || cb != 0 || cc != 0 || a != b || b != c || a == "" {
		t.Fatalf("positions disagree: %d %d %d\n%s\n%s\n%s", ca, cb, cc, a, b, c)
	}
	d, _, cd := iterateRun(t, nil, "plan", "depends", "--json", "backfill", "--project="+dir)
	e, _, ce := iterateRun(t, nil, "--json", "plan", "depends", "backfill", "-C", dir)
	if cd != 0 || ce != 0 || d != e {
		t.Fatalf("backfill positions disagree: %d %d\n%s\n%s", cd, ce, d, e)
	}
}

// TESTMASTER: id=dag-cli-help tier=fast parallel=yes
func TestPlanHelpAndUsageErrors(t *testing.T) {
	out, _, code := iterateRun(t, nil, "plan", "graph", "--help")
	if code != 0 || !strings.HasPrefix(out, "iterate-run plan graph — ") || strings.Index(out, "Examples:") > strings.Index(out, "Flags:") {
		t.Fatalf("graph help: exit %d\n%s", code, out)
	}
	if out, _, code = iterateRun(t, nil, "plan"); code != 0 || !strings.Contains(out, "depends backfill") {
		t.Fatalf("bare plan: exit %d\n%s", code, out)
	}
	_, errOut, code := iterateRun(t, nil, "plan", "grahp")
	if code != 2 || !strings.Contains(errOut, `Error: unknown plan command "grahp"`) || !strings.Contains(errOut, "Did you mean: iterate-run plan graph?") {
		t.Fatalf("typo: exit %d %q", code, errOut)
	}
	_, errOut, code = iterateRun(t, nil, "plan", "graph", "--format", "dot")
	if code != 2 || !strings.Contains(errOut, `--format received "dot"; it accepts text, mermaid or json`) {
		t.Fatalf("bad format: exit %d %q", code, errOut)
	}
	_, errOut, code = iterateRun(t, nil, "plan", "graph", "--apply")
	if code != 2 || !strings.Contains(errOut, "belongs to plan depends backfill") {
		t.Fatalf("misplaced flag: exit %d %q", code, errOut)
	}
	_, errOut, code = iterateRun(t, nil, "plan", "graph", "-C", t.TempDir())
	if code != 1 || !strings.Contains(errOut, ".claude/iterate/plans does not exist") {
		t.Fatalf("no plans dir: exit %d %q", code, errOut)
	}
	_, errOut, _ = iterateRun(t, nil)
	if !strings.Contains(errOut, "iterate-run plan graph") || !strings.Contains(errOut, "iterate-run plan depends backfill") {
		t.Fatalf("top-level usage does not list the plan noun:\n%s", errOut)
	}
}

// TESTMASTER: id=dag-cli-color tier=fast parallel=yes
func TestPlanGraphColorLadder(t *testing.T) {
	dir := fanOut(t)
	for _, tc := range []struct {
		name string
		env  []string
		args []string
		want bool
	}{
		{"pipe-auto", nil, nil, false},
		{"no-color", []string{"NO_COLOR=1"}, nil, false},
		{"dumb", []string{"TERM=dumb"}, nil, false},
		{"always-over-pipe", nil, []string{"--color=always"}, true},
		{"explicit-over-no-color", []string{"NO_COLOR=1"}, []string{"--color", "always"}, true},
		{"force", []string{"FORCE_COLOR=1"}, nil, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			out, errOut, _ := iterateRun(t, tc.env, append([]string{"plan", "graph", "-C", dir}, tc.args...)...)
			if got := strings.Contains(out, "\x1b["); got != tc.want {
				t.Fatalf("escapes = %v, want %v\n%q", got, tc.want, out)
			}
			if strings.Contains(errOut, "\x1b[") {
				t.Fatalf("stderr summary carries escapes: %q", errOut)
			}
			plain := strings.NewReplacer("\x1b[0m", "", "\x1b[1m", "", "\x1b[2m", "", "\x1b[31m", "", "\x1b[32m", "", "\x1b[33m", "").Replace(out)
			if !strings.Contains(plain, "◌ bravo") || !strings.Contains(plain, "waiting on alpha") {
				t.Fatalf("meaning lost without color: %q", plain)
			}
		})
	}
}
