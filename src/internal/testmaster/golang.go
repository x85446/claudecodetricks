package testmaster

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

// goAdapter batches every selected test of one package into a single
// `go test -json` run and reads per-test results and timings from the event
// stream, so a package compiles once instead of once per test.
type goAdapter struct{}

func normDir(d string) string {
	if d == "." {
		return ""
	}
	return d
}

func (goAdapter) identity(e *Entry) string {
	return normDir(e.Dir) + "\x00" + e.Pkg + "\x00" + e.Name
}

func (goAdapter) repro(_ *Project, e *Entry) string {
	argv := append([]string{"go", "test", "-count=1"}, e.Args...)
	argv = append(argv, "-run", "^"+e.Name+"$", e.Pkg)
	return withDir(e.Dir, shellQuote(argv))
}

type goEvent struct {
	Action  string
	Package string
	Test    string
	Elapsed float64
	Output  string
}

type goTest struct {
	action  string
	elapsed float64
	out     bytes.Buffer
}

func (a goAdapter) plan(p *Project, ids []string, floor time.Duration) []Unit {
	type group struct {
		ids     []string
		entries []*Entry
	}
	groups := map[string]*group{}
	var order []string
	for _, id := range ids {
		e := p.Reg.Tests[id]
		key := normDir(e.Dir) + "\x00" + e.Pkg + "\x00" + strings.Join(e.Args, "\x00")
		g, ok := groups[key]
		if !ok {
			g = &group{}
			groups[key] = g
			order = append(order, key)
		}
		g.ids = append(g.ids, id)
		g.entries = append(g.entries, e)
	}
	var units []Unit
	for _, key := range order {
		g := groups[key]
		first := g.entries[0]
		timeout := unitTimeout(floor, g.entries)
		units = append(units, Unit{
			Kind: "go", IDs: g.ids,
			Display: withDir(first.Dir, fmt.Sprintf("go test -json -count=1 -run <%d tests> %s", len(g.ids), first.Pkg)),
			Run:     func(ctx context.Context) []Result { return a.runGroup(ctx, p, g.ids, g.entries, timeout) },
		})
	}
	return units
}

var goTimeoutRE = regexp.MustCompile(`(?m)^panic: test timed out after`)
var goRunningRE = regexp.MustCompile(`(?m)^\s+(\w+)(?:/\S*)? \(`)

// runGroup runs the batch, and when the test binary dies partway (a panic, a
// timeout, os.Exit) runs the tests that never got a result again — so one
// crashing test never hides the results of the rest of its package.
func (a goAdapter) runGroup(ctx context.Context, p *Project, ids []string, entries []*Entry, timeout time.Duration) []Result {
	byName := map[string]string{}
	entryOf := map[string]*Entry{}
	var remaining []string
	for i, id := range ids {
		byName[entries[i].Name] = id
		entryOf[entries[i].Name] = entries[i]
		remaining = append(remaining, entries[i].Name)
	}
	first := entries[0]
	var results []Result
	for attempt := 0; len(remaining) > 0 && attempt <= len(ids); attempt++ {
		argv := append(p.Reg.Runner("go", "go"), "test", "-json", "-count=1", "-timeout", timeout.String())
		argv = append(argv, first.Args...)
		argv = append(argv, "-run", "^("+strings.Join(remaining, "|")+")$", first.Pkg)
		r := runProc(ctx, p.DirOf(first), nil, timeout+2*time.Minute, false, argv...)

		tests, pkgOut, allOut := parseGoEvents(r.Stdout, remaining)
		pkgOut = append(pkgOut, r.Stderr...)
		hung := map[string]bool{}
		if loc := goTimeoutRE.FindIndex(allOut); loc != nil {
			for _, m := range goRunningRE.FindAllSubmatch(allOut[loc[1]:], -1) {
				hung[string(m[1])] = true
			}
		}
		progress := false
		var next []string
		for _, name := range remaining {
			t := tests[name]
			res := Result{ID: byName[name], Repro: a.repro(p, entryOf[name]), Exit: r.Exit}
			switch {
			case hung[name]:
				res.Status, res.Note = Timeout, "go test -timeout "+timeout.String()+" fired while it ran"
				res.Output = joinOut(t.out.Bytes(), pkgOut)
			case t.action == "pass":
				res.Status, res.Ms, res.Timed = Pass, int64(t.elapsed*1000), true
			case t.action == "skip":
				res.Status = Skip
			case t.action == "fail":
				res.Status, res.Ms, res.Timed = Fail, int64(t.elapsed*1000), true
				res.Output = t.out.Bytes()
				if r.Exit != 0 && t.out.Len() == 0 {
					res.Output = pkgOut
				}
			default:
				next = append(next, name)
				continue
			}
			progress = true
			results = append(results, res)
		}
		remaining = next
		if len(remaining) == 0 {
			break
		}
		switch {
		case r.Interrupted:
			return fillMissing(ids, results, Interrupted, "", nil)
		case r.TimedOut:
			return fillMissing(ids, results, Timeout, "killed after "+(timeout+2*time.Minute).String(), pkgOut)
		case r.StartErr != nil:
			return fillMissing(ids, results, Fail, "cannot start go", []byte(r.StartErr.Error()+"\n"))
		case r.Exit == 0:
			return fillMissing(ids, results, Missing, "no test by this name in "+first.Pkg, nil)
		case !progress:
			// Nothing ran: the package failed to build, or TestMain/init died.
			for _, name := range remaining {
				results = append(results, Result{
					ID: byName[name], Status: Fail, Output: pkgOut, Exit: r.Exit,
					Repro: a.repro(p, entryOf[name]), Note: "package failed before any test ran",
					Group: "go:" + first.Pkg,
				})
			}
			return results
		}
	}
	return fillMissing(ids, results, Fail, "no result after retries", nil)
}

func joinOut(test, pkg []byte) []byte {
	if len(pkg) == 0 {
		return test
	}
	out := append([]byte{}, test...)
	out = append(out, "\n--- package output ---\n"...)
	return append(out, pkg...)
}

// parseGoEvents folds a test2json stream into per-test results for the
// wanted top-level tests (subtest output belongs to its parent), the
// package-level output, and the whole output as plain text.
func parseGoEvents(stream []byte, wanted []string) (map[string]*goTest, []byte, []byte) {
	tests := map[string]*goTest{}
	for _, n := range wanted {
		tests[n] = &goTest{}
	}
	var pkgOut, allOut bytes.Buffer
	sc := bufio.NewScanner(bytes.NewReader(stream))
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		line := sc.Bytes()
		var ev goEvent
		if len(line) == 0 || line[0] != '{' || json.Unmarshal(line, &ev) != nil {
			pkgOut.Write(line)
			pkgOut.WriteByte('\n')
			allOut.Write(line)
			allOut.WriteByte('\n')
			continue
		}
		allOut.WriteString(ev.Output)
		top, _, _ := strings.Cut(ev.Test, "/")
		t, ok := tests[top]
		if ev.Test == "" || !ok {
			if ev.Test == "" {
				pkgOut.WriteString(ev.Output)
			}
			continue
		}
		t.out.WriteString(ev.Output)
		if ev.Test == top && (ev.Action == "pass" || ev.Action == "fail" || ev.Action == "skip") {
			t.action, t.elapsed = ev.Action, ev.Elapsed
		}
	}
	return tests, pkgOut.Bytes(), allOut.Bytes()
}

var goTestNameRE = regexp.MustCompile(`^(Test|Example|Fuzz)\w*$`)

func (a goAdapter) discover(ctx context.Context, p *Project, src Source) ([]Found, error) {
	dir := p.DirOf(&Entry{Dir: src.Dir})
	goBin := p.Reg.Runner("go", "go")
	r := runProc(ctx, dir, nil, 10*time.Minute, false, append(goBin, "list", "-f", "{{.ImportPath}}\t{{.Dir}}", "./...")...)
	if r.Failed() {
		return nil, fmt.Errorf("go list failed in %s:\n%s", p.Rel(dir), r.Stderr)
	}
	pkgDir := map[string]string{}
	for _, line := range strings.Split(strings.TrimSpace(string(r.Stdout)), "\n") {
		if imp, d, ok := strings.Cut(line, "\t"); ok {
			pkgDir[imp] = d
		}
	}
	r = runProc(ctx, dir, nil, 30*time.Minute, false, append(goBin, "test", "-list", "^(Test|Example|Fuzz)", "-json", "./...")...)
	names := map[string][]string{}
	var order []string
	sc := bufio.NewScanner(bytes.NewReader(r.Stdout))
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		var ev goEvent
		if json.Unmarshal(sc.Bytes(), &ev) != nil || ev.Action != "output" || ev.Test != "" {
			continue
		}
		name := strings.TrimSpace(ev.Output)
		if goTestNameRE.MatchString(name) {
			if _, ok := names[ev.Package]; !ok {
				order = append(order, ev.Package)
			}
			names[ev.Package] = append(names[ev.Package], name)
		}
	}
	if r.Failed() && len(order) == 0 {
		return nil, fmt.Errorf("go test -list failed in %s:\n%s%s", p.Rel(dir), r.Stdout, r.Stderr)
	}
	var found []Found
	for _, imp := range order {
		abs := pkgDir[imp]
		rel, err := filepath.Rel(dir, abs)
		if abs == "" || err != nil {
			continue
		}
		pkg := "./" + filepath.ToSlash(rel)
		if rel == "." {
			pkg = "."
		}
		files := goTestFiles(abs)
		for _, name := range names[imp] {
			e := Entry{Kind: "go", Dir: normDir(src.Dir), Pkg: pkg, Name: name}
			if f, ok := files[name]; ok {
				e.File = p.Rel(f)
			}
			e.Cmd = a.repro(p, &e)
			alt := strings.ReplaceAll(strings.TrimPrefix(pkg, "./"), "/", ".") + "." + name
			found = append(found, Found{ID: filepath.Base(abs) + "." + name, AltID: alt, Entry: e})
		}
	}
	return found, nil
}

// goTestFiles maps each test function in a package directory to its file.
func goTestFiles(dir string) map[string]string {
	out := map[string]string{}
	matches, _ := filepath.Glob(filepath.Join(dir, "*_test.go"))
	fset := token.NewFileSet()
	for _, path := range matches {
		src, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		f, err := parser.ParseFile(fset, path, src, parser.SkipObjectResolution)
		if err != nil {
			continue
		}
		for _, d := range f.Decls {
			if fn, ok := d.(*ast.FuncDecl); ok && fn.Recv == nil {
				out[fn.Name.Name] = path
			}
		}
	}
	return out
}
