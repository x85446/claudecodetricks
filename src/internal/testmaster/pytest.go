package testmaster

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// pytestAdapter batches every selected node id under one directory into a
// single pytest run and reads per-test results and timings from its JUnit
// XML report.
type pytestAdapter struct{}

func (pytestAdapter) identity(e *Entry) string { return normDir(e.Dir) + "\x00" + e.NodeID }

// pytestRunner prefers the project's own virtualenv, then pytest on PATH.
func pytestRunner(p *Project, dir string) []string {
	if r := p.Reg.Runner("pytest"); len(r) > 0 {
		return r
	}
	for _, venv := range []string{".venv", "venv"} {
		py := filepath.Join(dir, venv, "bin", "python")
		if _, err := os.Stat(py); err == nil {
			return []string{py, "-m", "pytest"}
		}
	}
	if _, err := exec.LookPath("pytest"); err == nil {
		return []string{"pytest"}
	}
	return []string{"python3", "-m", "pytest"}
}

var pytestQuiet = []string{"-p", "no:cacheprovider"}

func (pytestAdapter) repro(p *Project, e *Entry) string {
	argv := append(pytestRunner(p, p.DirOf(e)), e.Args...)
	if rel, err := filepath.Rel(p.DirOf(e), argv[0]); err == nil && filepath.IsAbs(argv[0]) && !strings.HasPrefix(rel, "..") {
		argv[0] = "./" + rel
	}
	return withDir(e.Dir, shellQuote(append(argv, e.NodeID)))
}

type junitMsg struct {
	Message string `xml:"message,attr"`
	Text    string `xml:",chardata"`
}

type junitCase struct {
	Name      string     `xml:"name,attr"`
	Classname string     `xml:"classname,attr"`
	Time      float64    `xml:"time,attr"`
	Failure   []junitMsg `xml:"failure"`
	Error     []junitMsg `xml:"error"`
	Skipped   []junitMsg `xml:"skipped"`
	SystemOut string     `xml:"system-out"`
	SystemErr string     `xml:"system-err"`
}

type junitSuite struct {
	Cases  []junitCase  `xml:"testcase"`
	Suites []junitSuite `xml:"testsuite"`
}

func (s junitSuite) all() []junitCase {
	out := append([]junitCase{}, s.Cases...)
	for _, c := range s.Suites {
		out = append(out, c.all()...)
	}
	return out
}

// junitKey turns a node id into the classname and name pytest's JUnit
// report gives it: tests/test_x.py::TestA::test_b[1] → tests.test_x.TestA,
// test_b[1]. A bare file id has no name and matches every case in it.
func junitKey(nodeid string) (classname, name string) {
	parts := strings.Split(nodeid, "::")
	mod := strings.ReplaceAll(strings.TrimSuffix(parts[0], ".py"), "/", ".")
	if len(parts) == 1 {
		return mod, ""
	}
	return strings.Join(append([]string{mod}, parts[1:len(parts)-1]...), "."), parts[len(parts)-1]
}

func junitMatch(nodeid string, c junitCase) bool {
	cls, name := junitKey(nodeid)
	if name == "" {
		return c.Classname == cls || strings.HasPrefix(c.Classname, cls+".")
	}
	return c.Classname == cls && (c.Name == name || strings.HasPrefix(c.Name, name+"["))
}

func (a pytestAdapter) plan(p *Project, ids []string, floor time.Duration) []Unit {
	groups := map[string][]string{}
	var order []string
	for _, id := range ids {
		e := p.Reg.Tests[id]
		key := normDir(e.Dir) + "\x00" + strings.Join(e.Args, "\x00")
		if _, ok := groups[key]; !ok {
			order = append(order, key)
		}
		groups[key] = append(groups[key], id)
	}
	var units []Unit
	for _, key := range order {
		gids := groups[key]
		first := p.Reg.Tests[gids[0]]
		var entries []*Entry
		for _, id := range gids {
			entries = append(entries, p.Reg.Tests[id])
		}
		timeout := unitTimeout(floor, entries)
		units = append(units, Unit{
			Kind: "pytest", IDs: gids,
			Display: withDir(first.Dir, fmt.Sprintf("pytest --junitxml <%d tests>", len(gids))),
			Run:     func(ctx context.Context) []Result { return a.runGroup(ctx, p, gids, timeout) },
		})
	}
	return units
}

func (a pytestAdapter) runGroup(ctx context.Context, p *Project, ids []string, timeout time.Duration) []Result {
	first := p.Reg.Tests[ids[0]]
	dir := p.DirOf(first)
	var results []Result
	remaining := append([]string{}, ids...)
	for attempt := 0; len(remaining) > 0 && attempt < 3; attempt++ {
		xmlFile, err := os.CreateTemp("", "testmaster-junit-*.xml")
		if err != nil {
			return fillMissing(ids, results, Fail, "cannot create junit file", []byte(err.Error()+"\n"))
		}
		xmlFile.Close()
		argv := append(pytestRunner(p, dir), pytestQuiet...)
		argv = append(argv, "-q", "-o", "junit_family=xunit2", "-o", "junit_logging=all", "--junitxml="+xmlFile.Name())
		argv = append(argv, first.Args...)
		for _, id := range remaining {
			argv = append(argv, p.Reg.Tests[id].NodeID)
		}
		r := runProc(ctx, dir, nil, timeout, true, argv...)
		report, _ := os.ReadFile(xmlFile.Name())
		os.Remove(xmlFile.Name())

		if r.Interrupted {
			return fillMissing(ids, results, Interrupted, "", nil)
		}
		// Exit 4 with "not found" names node ids pytest has no such test for;
		// mark them missing and run the rest, instead of failing the batch.
		if r.Exit == 4 {
			var keep []string
			for _, id := range remaining {
				if pytestNotFound(r.Stdout, p.Reg.Tests[id].NodeID) {
					results = append(results, Result{ID: id, Status: Missing, Note: "pytest has no such node id"})
				} else {
					keep = append(keep, id)
				}
			}
			if len(keep) < len(remaining) {
				remaining = keep
				continue
			}
		}
		var suite junitSuite
		cases := []junitCase{}
		if len(report) > 0 && xml.Unmarshal(report, &suite) == nil {
			cases = suite.all()
		}
		for _, id := range remaining {
			e := p.Reg.Tests[id]
			res := Result{ID: id, Repro: a.repro(p, e), Exit: r.Exit}
			var matched []junitCase
			for _, c := range cases {
				if junitMatch(e.NodeID, c) {
					matched = append(matched, c)
				}
			}
			switch {
			case len(matched) == 0 && r.TimedOut:
				res.Status, res.Output, res.Note = Timeout, r.Stdout, "killed after "+timeout.String()
			case len(matched) == 0 && (r.Exit == 0 || r.Exit == 5):
				res.Status, res.Note = Missing, "pytest collected nothing for this node id"
			case len(matched) == 0:
				res.Status, res.Output, res.Note, res.Group = Fail, r.Stdout, "pytest stopped before this test ran", "pytest:"+dir
			default:
				res.Status, res.Ms, res.Timed, res.Output = junitOutcome(matched)
			}
			results = append(results, res)
		}
		return results
	}
	return fillMissing(ids, results, Fail, "no result after retries", nil)
}

func pytestNotFound(out []byte, nodeid string) bool {
	for _, line := range strings.Split(string(out), "\n") {
		for _, prefix := range []string{"ERROR: not found: ", "ERROR: file or directory not found: "} {
			if rest, ok := strings.CutPrefix(strings.TrimSpace(line), prefix); ok {
				if rest == nodeid || strings.HasSuffix(rest, "/"+nodeid) || strings.HasPrefix(nodeid, rest+"::") {
					return true
				}
			}
		}
	}
	return false
}

// junitOutcome folds every case a node id matched (one, or each
// parametrization) into one result: any failure fails it, all-skipped
// skips it.
func junitOutcome(cases []junitCase) (string, int64, bool, []byte) {
	var secs float64
	var out bytes.Buffer
	failed, skipped := false, 0
	for _, c := range cases {
		secs += c.Time
		for _, m := range append(append([]junitMsg{}, c.Failure...), c.Error...) {
			failed = true
			fmt.Fprintf(&out, "=== %s::%s ===\n%s\n%s\n", c.Classname, c.Name, m.Message, strings.TrimSpace(m.Text))
			if s := strings.TrimSpace(c.SystemOut); s != "" {
				fmt.Fprintf(&out, "--- captured stdout ---\n%s\n", s)
			}
			if s := strings.TrimSpace(c.SystemErr); s != "" {
				fmt.Fprintf(&out, "--- captured stderr ---\n%s\n", s)
			}
		}
		if len(c.Skipped) > 0 {
			skipped++
		}
	}
	ms := int64(secs * 1000)
	switch {
	case failed:
		return Fail, ms, true, out.Bytes()
	case skipped == len(cases):
		return Skip, 0, false, nil
	default:
		return Pass, ms, true, nil
	}
}

func (a pytestAdapter) discover(ctx context.Context, p *Project, src Source) ([]Found, error) {
	dir := p.DirOf(&Entry{Dir: src.Dir})
	argv := append(pytestRunner(p, dir), pytestQuiet...)
	argv = append(argv, "--collect-only", "-q")
	r := runProc(ctx, dir, nil, 10*time.Minute, true, argv...)
	if r.Failed() && r.Exit != 5 {
		return nil, fmt.Errorf("pytest --collect-only failed in %s (exit %d):\n%s", p.Rel(dir), r.Exit, r.Stdout)
	}
	var found []Found
	for _, line := range strings.Split(string(r.Stdout), "\n") {
		line = strings.TrimSpace(line)
		if !strings.Contains(line, "::") || strings.ContainsAny(line, " \t") {
			continue
		}
		file, _, _ := strings.Cut(line, "::")
		e := Entry{Kind: "pytest", Dir: normDir(src.Dir), NodeID: line, File: p.Rel(filepath.Join(dir, file))}
		e.Cmd = a.repro(p, &e)
		id := line
		if d := normDir(src.Dir); d != "" {
			id = d + "/" + line
		}
		found = append(found, Found{ID: id, Entry: e})
	}
	return found, nil
}
