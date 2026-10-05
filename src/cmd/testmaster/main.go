// Command testmaster runs a project's registered tests outside the model's
// context. Passing tests are a count; each failure is one line naming a log
// file the AI reads only when it needs the detail.
package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/signal"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"time"

	tm "github.com/x85446/claudecodetricks/src/internal/testmaster"
)

// Set via -ldflags at build time (see Makefile).
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildTime = "unknown"
)

type flagSpec struct {
	long, short string
	value       bool
}

var specs = []flagSpec{
	{"project", "C", true}, {"json", "", false}, {"verbose", "v", false}, {"help", "h", false}, {"version", "", false},
	{"jobs", "j", true}, {"timeout", "", true}, {"failed", "", false}, {"dry-run", "n", false},
	{"kind", "", true}, {"dir", "", true}, {"tier", "", true}, {"failing", "", false}, {"unmeasured", "", false},
	{"blocking", "", false}, {"no-blocking", "", false}, {"serial", "", false}, {"parallel", "", false},
	{"cmd", "", true}, {"pkg", "", true}, {"name", "", true}, {"nodeid", "", true}, {"target", "", true}, {"file", "", true},
}

var globalFlags = []string{"project", "json", "verbose", "help", "version"}

var localFlags = map[string][]string{
	"suite run":      {"jobs", "timeout", "failed", "dry-run"},
	"suite discover": {"kind", "dir", "dry-run"},
	"suite status":   {},
	"test list":      {"tier", "kind", "failing", "unmeasured", "blocking"},
	"test show":      {},
	"test add":       {"kind", "dir", "cmd", "pkg", "name", "nodeid", "target", "file", "blocking", "serial"},
	"test set":       {"blocking", "no-blocking", "serial", "parallel"},
	"test remove":    {},
	"runner list":    {},
	"runner set":     {},
	"runner clear":   {},
}

// parse accepts flags in every position; `--` ends flag parsing.
func parse(args []string) (pos []string, flags map[string]string, err error) {
	flags = map[string]string{}
	byName := map[string]flagSpec{}
	for _, s := range specs {
		byName["--"+s.long] = s
		if s.short != "" {
			byName["-"+s.short] = s
		}
	}
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			return append(pos, args[i+1:]...), flags, nil
		}
		if !strings.HasPrefix(a, "-") || a == "-" {
			pos = append(pos, a)
			continue
		}
		name, val, hasVal := strings.Cut(a, "=")
		s, ok := byName[name]
		if !ok {
			return nil, nil, fmt.Errorf("unknown flag %s", name)
		}
		switch {
		case s.value && hasVal:
			flags[s.long] = val
		case s.value:
			if i+1 >= len(args) {
				return nil, nil, fmt.Errorf("%s needs a value", name)
			}
			i++
			flags[s.long] = args[i]
		case hasVal:
			return nil, nil, fmt.Errorf("%s takes no value", name)
		default:
			flags[s.long] = "true"
		}
	}
	return pos, flags, nil
}

const rootHelp = `testmaster — run a project's registered tests outside the model; report only what failed

Usage:
  testmaster <noun> <verb> [operands] [flags]

Examples:
  testmaster suite run                 # fast + standard + unmeasured: one line, plus one per failure
  testmaster suite run slow            # a tier; also all, an id, or a glob like 'iterrun.*'
  testmaster suite run --failed        # rerun only what failed last time
  testmaster suite discover            # register new go / cargo / pytest tests
  testmaster test add lint --kind shell --cmd 'make lint' --blocking

Commands:
  suite run [tier|all|id|glob…]   run tests; passes are a count, each failure a log path
  suite discover                  ask each toolchain what tests exist; register new ones
  suite status                    tiers, last results, last run, slowest
  test list                       registered ids, one per line
  test show <id>                  one registry entry as JSON
  test add <id>                   register a test by hand (any language, via --kind shell)
  test set <id>…                  mark tests blocking or serial
  test remove <id>…               drop registry entries (history stays)
  runner set <kind> -- <argv…>    how this project invokes a toolchain (env, wrapper, venv)
  runner list | runner clear <kind>

Global flags:
  -C, --project <dir>   project root (default: nearest ancestor with a registry, else the git root)
      --json            machine output on stdout
  -v, --verbose         one stderr line per invocation
  -h, --help            help for this level
      --version         version, one line

State: .claude/testmaster/registry.json (tests, timing, results), history.jsonl (every
result), failures/<id>.log (one per currently failing test, ignored by git).

Exit: 0 all passed · 1 a test failed, timed out, is missing, or was blocked · 2 usage ·
      3 no tests registered · 130 interrupted
`

var cmdHelp = map[string]string{
	"suite run": `testmaster suite run — run tests and report only what did not pass

Usage:
  testmaster suite run [fast|standard|slow|all|<id>|<glob>…] [flags]

Examples:
  testmaster suite run                        # fast + standard + every unmeasured test
  testmaster suite run iterrun.TestStatus     # one test
  testmaster suite run 'iterrun.*' --failed   # the failing ones in a package

Blocking tests run first, one at a time; the first that fails stops the run.
Then parallel-safe tests run concurrently, then serial ones one at a time.
Each failure's full output goes to .claude/testmaster/failures/<id>.log.

Flags:
  -j, --jobs <n>         concurrent invocations (default: CPU count)
      --timeout <dur>    per-invocation floor; measured tests get 3x their average if larger (default 10m)
      --failed           only tests whose last result was fail or timeout
  -n, --dry-run          print the invocations, run nothing
`,
	"suite discover": `testmaster suite discover — register every test the toolchains can see

Usage:
  testmaster suite discover [flags]

Examples:
  testmaster suite discover                          # the recorded sources, or detect them
  testmaster suite discover --kind pytest --dir api  # add a source

Detects go.mod, Cargo.toml and pytest config at the root and one level down.
New tests are registered unmeasured; tests a toolchain no longer sees are
reported, never removed.

Flags:
      --kind <go|cargo|pytest>   discover this toolchain only, and remember it as a source
      --dir <path>               where that toolchain runs (default .)
  -n, --dry-run                  report, write nothing
`,
	"suite status": `testmaster suite status — one screen of registry state

Usage:
  testmaster suite status
`,
	"test list": `testmaster test list — registered ids, one per line

Usage:
  testmaster test list [flags]

Flags:
      --tier <fast|standard|slow|?>
      --kind <go|cargo|pytest|shell>
      --failing       last result fail or timeout
      --unmeasured    no measured timing yet
      --blocking      only blocking tests
`,
	"test show": `testmaster test show — one registry entry as JSON

Usage:
  testmaster test show <id>
`,
	"test add": `testmaster test add — register a test by hand

Usage:
  testmaster test add <id> --kind <kind> <identity flags> [flags]

Examples:
  testmaster test add e2e.login --kind shell --cmd 'npm run e2e -- login'
  testmaster test add build --kind shell --cmd 'make build' --blocking
  testmaster test add api.TestHealth --kind go --pkg ./api --name TestHealth

A shell test passes on exit 0, skips on exit 77, and fails otherwise.

Flags:
      --kind <go|cargo|pytest|shell>
      --cmd <command>      shell: the command, run with bash -c
      --pkg <pkg>          go: package path (./api); cargo: package name
      --name <name>        go: test function; cargo: test path
      --nodeid <id>        pytest: node id
      --target <kind:name> cargo: lib:foo, test:integration, bin:tool
      --dir <path>         where to run it (default .)
      --file <path>        the test's source file
      --blocking           a failure stops the run
      --serial             never run alongside other tests
`,
	"test set": `testmaster test set — change declared properties

Usage:
  testmaster test set <id>… [--blocking|--no-blocking] [--serial|--parallel]
`,
	"test remove": `testmaster test remove — drop registry entries and their failure logs

Usage:
  testmaster test remove <id>…
`,
	"runner set": `testmaster runner set — how this project invokes a toolchain

Usage:
  testmaster runner set <go|cargo|pytest|shell> -- <argv…>

Examples:
  testmaster runner set go -- env CGO_ENABLED=1 GOFLAGS=-tags=integration go
  testmaster runner set pytest -- uv run pytest
  testmaster runner set shell -- zsh -c

The argv replaces the default command prefix (go, cargo, pytest, bash -c);
the runner appends its own arguments after it.
`,
	"runner list": `testmaster runner list — the project's toolchain overrides, one per line

Usage:
  testmaster runner list
`,
	"runner clear": `testmaster runner clear — return a toolchain to its default command

Usage:
  testmaster runner clear <kind>
`,
}

func main() { os.Exit(run(os.Args[1:], os.Stdout, os.Stderr)) }

func usageErr(stderr io.Writer, cmd, msg string) int {
	fmt.Fprintf(stderr, "Error: %s\n\n", msg)
	if h, ok := cmdHelp[cmd]; ok {
		fmt.Fprint(stderr, h)
	} else {
		fmt.Fprint(stderr, rootHelp)
	}
	return 2
}

func fail(stderr io.Writer, msg string, next ...string) int {
	fmt.Fprintf(stderr, "Error: %s\n", msg)
	if len(next) > 0 {
		fmt.Fprintf(stderr, "\n  %s\n", strings.Join(next, "\n  "))
	}
	return 1
}

func run(args []string, stdout, stderr io.Writer) int {
	pos, flags, err := parse(args)
	if err != nil {
		return usageErr(stderr, "", err.Error())
	}
	if flags["version"] != "" {
		fmt.Fprintf(stdout, "testmaster %s (commit %s, built %s)\n", Version, Commit, BuildTime)
		return 0
	}
	if len(pos) < 2 {
		if len(pos) == 1 && pos[0] != "suite" && pos[0] != "test" && pos[0] != "runner" {
			return usageErr(stderr, "", fmt.Sprintf("unknown command %q", pos[0]))
		}
		fmt.Fprint(stdout, rootHelp)
		return 0
	}
	cmd := pos[0] + " " + pos[1]
	allowed, ok := localFlags[cmd]
	if !ok {
		return usageErr(stderr, "", fmt.Sprintf("unknown command %q", cmd))
	}
	if flags["help"] != "" {
		fmt.Fprint(stdout, cmdHelp[cmd])
		return 0
	}
	for f := range flags {
		if !contains(globalFlags, f) && !contains(allowed, f) {
			return usageErr(stderr, cmd, fmt.Sprintf("--%s does not apply to %s", f, cmd))
		}
	}
	operands := pos[2:]

	cwd, _ := os.Getwd()
	start := cwd
	if d := flags["project"]; d != "" {
		start = d
	}
	var p *tm.Project
	if flags["project"] != "" {
		p, err = tm.Open(start)
	} else {
		p, err = tm.Find(start)
	}
	if err != nil {
		return fail(stderr, err.Error())
	}

	switch cmd {
	case "suite run":
		return suiteRun(p, operands, flags, cwd, stdout, stderr)
	case "suite discover":
		return suiteDiscover(p, flags, stdout, stderr)
	case "test add":
		return mutate(p, cmd, operands, flags, stderr)
	case "runner list":
		for _, k := range tm.Kinds {
			if v, ok := p.Reg.Runners[k]; ok {
				fmt.Fprintf(stdout, "%s\t%s\n", k, strings.Join(v, " "))
			}
		}
		return 0
	case "runner set", "runner clear":
		return mutate(p, cmd, operands, flags, stderr)
	}

	if !p.Exists() || len(p.Reg.Tests) == 0 {
		return noRegistry(p, stderr)
	}
	switch cmd {
	case "suite status":
		p.Status(stdout, cwd)
	case "test list":
		for _, id := range p.List(tm.ListFilter{
			Tier: flags["tier"], Kind: flags["kind"], Failing: flags["failing"] != "",
			Unmeasured: flags["unmeasured"] != "", Blocking: flags["blocking"] != "",
		}) {
			fmt.Fprintln(stdout, id)
		}
	case "test show":
		if len(operands) != 1 {
			return usageErr(stderr, cmd, "test show takes exactly one id")
		}
		e := p.Reg.Tests[operands[0]]
		if e == nil {
			return fail(stderr, fmt.Sprintf("no test %q", operands[0]), "List them: testmaster test list")
		}
		b, _ := json.MarshalIndent(e, "", "  ")
		fmt.Fprintln(stdout, string(b))
	default:
		return mutate(p, cmd, operands, flags, stderr)
	}
	return 0
}

func noRegistry(p *tm.Project, stderr io.Writer) int {
	fmt.Fprintf(stderr, "Error: no tests registered in %s/%s/registry.json\n\n  Register them first:\n    testmaster suite discover\n", p.Root, tm.StateDir)
	return 3
}

func mutate(p *tm.Project, cmd string, operands []string, flags map[string]string, stderr io.Writer) int {
	release, err := p.Lock()
	if err != nil {
		return fail(stderr, err.Error())
	}
	defer release()
	switch cmd {
	case "test add":
		if len(operands) != 1 {
			return usageErr(stderr, cmd, "test add takes exactly one id")
		}
		e := tm.Entry{
			Kind: flags["kind"], Cmd: flags["cmd"], Pkg: flags["pkg"], Name: flags["name"], NodeID: flags["nodeid"],
			Target: flags["target"], Dir: flags["dir"], File: flags["file"], Blocking: flags["blocking"] != "",
		}
		if flags["serial"] != "" {
			f := false
			e.ParallelSafe = &f
		}
		if e.Kind == "" {
			return usageErr(stderr, cmd, "test add needs --kind")
		}
		err = p.Add(operands[0], e)
	case "test set":
		if len(operands) == 0 {
			return usageErr(stderr, cmd, "test set needs at least one id")
		}
		var blocking, parallel *bool
		t, f := true, false
		switch {
		case flags["blocking"] != "" && flags["no-blocking"] != "":
			return usageErr(stderr, cmd, "--blocking and --no-blocking contradict each other")
		case flags["blocking"] != "":
			blocking = &t
		case flags["no-blocking"] != "":
			blocking = &f
		}
		switch {
		case flags["serial"] != "" && flags["parallel"] != "":
			return usageErr(stderr, cmd, "--serial and --parallel contradict each other")
		case flags["serial"] != "":
			parallel = &f
		case flags["parallel"] != "":
			parallel = &t
		}
		if blocking == nil && parallel == nil {
			return usageErr(stderr, cmd, "test set needs --blocking, --no-blocking, --serial or --parallel")
		}
		err = p.Set(operands, blocking, parallel)
	case "test remove":
		if len(operands) == 0 {
			return usageErr(stderr, cmd, "test remove needs at least one id")
		}
		err = p.Remove(operands)
	case "runner set", "runner clear":
		if len(operands) == 0 || !contains(tm.Kinds, operands[0]) {
			return usageErr(stderr, cmd, "name a kind: go, cargo, pytest or shell")
		}
		if cmd == "runner set" && len(operands) < 2 {
			return usageErr(stderr, cmd, "runner set needs the command after --")
		}
		if p.Reg.Runners == nil {
			p.Reg.Runners = map[string][]string{}
		}
		if cmd == "runner set" {
			p.Reg.Runners[operands[0]] = operands[1:]
		} else {
			delete(p.Reg.Runners, operands[0])
		}
		err = p.Save()
	}
	if err != nil {
		return fail(stderr, err.Error())
	}
	return 0
}

func suiteRun(p *tm.Project, operands []string, flags map[string]string, cwd string, stdout, stderr io.Writer) int {
	if !p.Exists() || len(p.Reg.Tests) == 0 {
		return noRegistry(p, stderr)
	}
	opt := tm.RunOptions{Jobs: runtime.NumCPU(), Timeout: 10 * time.Minute, DryRun: flags["dry-run"] != "", Out: stdout}
	if v := flags["jobs"]; v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			return usageErr(stderr, "suite run", fmt.Sprintf("--jobs takes a positive number, got %q", v))
		}
		opt.Jobs = n
	}
	if v := flags["timeout"]; v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d <= 0 {
			return usageErr(stderr, "suite run", fmt.Sprintf("--timeout takes a duration like 90s or 20m, got %q", v))
		}
		opt.Timeout = d
	}
	if flags["verbose"] != "" {
		opt.Verbose = stderr
	}
	ids, label, err := p.Select(operands, flags["failed"] != "")
	if err != nil {
		return usageErr(stderr, "suite run", err.Error())
	}
	if len(ids) == 0 {
		fmt.Fprintf(stdout, "run %s: nothing selected\n", label)
		return 0
	}
	if !opt.DryRun {
		release, err := p.Lock()
		if err != nil {
			return fail(stderr, err.Error())
		}
		defer release()
	}
	if f, ok := stderr.(*os.File); ok && isTTY(f) && !opt.DryRun && opt.Verbose == nil {
		opt.Progress = func(done, total int) { fmt.Fprintf(f, "\r\033[K%d/%d", done, total) }
		defer fmt.Fprint(f, "\r\033[K")
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	rep, err := p.Run(ctx, ids, label, opt)
	if err != nil {
		return fail(stderr, err.Error())
	}
	if rep == nil {
		return 0
	}
	if flags["json"] != "" {
		b, err := rep.JSON(p, cwd)
		if err != nil {
			return fail(stderr, err.Error())
		}
		fmt.Fprintln(stdout, string(b))
	} else {
		rep.Print(stdout, p, cwd)
	}
	return rep.ExitCode()
}

func suiteDiscover(p *tm.Project, flags map[string]string, stdout, stderr io.Writer) int {
	var only *tm.Source
	switch {
	case flags["kind"] != "":
		if flags["kind"] == "shell" || !contains(tm.Kinds, flags["kind"]) {
			return usageErr(stderr, "suite discover", fmt.Sprintf("--kind takes go, cargo or pytest, got %q", flags["kind"]))
		}
		only = &tm.Source{Kind: flags["kind"], Dir: flags["dir"]}
	case flags["dir"] != "":
		return usageErr(stderr, "suite discover", "--dir needs --kind")
	}
	release, err := p.Lock()
	if err != nil {
		return fail(stderr, err.Error())
	}
	defer release()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	reps, err := p.Discover(ctx, only, flags["dry-run"] != "")
	if err != nil {
		return fail(stderr, err.Error(), "Name a source: testmaster suite discover --kind go --dir <path>")
	}
	code := 0
	for _, r := range reps {
		dir := r.Source.Dir
		if dir == "" {
			dir = "."
		}
		if r.Err != nil {
			fmt.Fprintf(stdout, "discover %s %s: failed\n", r.Source.Kind, dir)
			fmt.Fprintf(stderr, "%v\n", r.Err)
			code = 1
			continue
		}
		fmt.Fprintf(stdout, "discover %s %s: %d known, %d new, %d missing\n", r.Source.Kind, dir, r.Known, len(r.New), len(r.Missing))
		if flags["verbose"] != "" {
			for _, id := range r.New {
				fmt.Fprintf(stdout, "+\t%s\n", id)
			}
		}
		for _, id := range r.Missing {
			fmt.Fprintf(stdout, "MISSING\t%s\n", id)
		}
	}
	return code
}

func isTTY(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}
