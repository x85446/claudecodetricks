package testmaster

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// cargoAdapter builds each directory's test binaries once (`cargo test
// --no-run`), then runs every test as its own process straight from the
// binary. Each test gets a real measured time and its own output, and tests
// run in parallel without contending for cargo's build lock.
type cargoAdapter struct{}

func (cargoAdapter) identity(e *Entry) string {
	return normDir(e.Dir) + "\x00" + e.Pkg + "\x00" + e.Target + "\x00" + e.Name
}

// targetFlag turns a "kind:name" target key into cargo's selector flag.
func targetFlag(target string) []string {
	kind, name, _ := strings.Cut(target, ":")
	if kind == "lib" {
		return []string{"--lib"}
	}
	return []string{"--" + kind, name}
}

func (cargoAdapter) repro(_ *Project, e *Entry) string {
	argv := []string{"cargo", "test"}
	if e.Pkg != "" {
		argv = append(argv, "-p", e.Pkg)
	}
	argv = append(argv, targetFlag(e.Target)...)
	argv = append(argv, e.Args...)
	argv = append(argv, "--", "--exact", e.Name)
	return withDir(e.Dir, shellQuote(argv))
}

type cargoMsg struct {
	Reason       string `json:"reason"`
	ManifestPath string `json:"manifest_path"`
	Target       struct {
		Kind    []string `json:"kind"`
		Name    string   `json:"name"`
		SrcPath string   `json:"src_path"`
	} `json:"target"`
	Profile struct {
		Test bool `json:"test"`
	} `json:"profile"`
	Executable string `json:"executable"`
}

type cargoExe struct {
	exe, manifestDir, src, pkg, target string
}

// exeKey identifies one test binary: a target name repeats across the
// packages of a workspace, so the package is part of the key.
func exeKey(pkg, target string) string { return pkg + "\x00" + target }

// lookup finds e's binary. An entry with no package matches a target only
// when exactly one package has it.
func lookup(exes map[string]cargoExe, pkg, target string) (cargoExe, bool) {
	if x, ok := exes[exeKey(pkg, target)]; ok || pkg != "" {
		return x, ok
	}
	var hit cargoExe
	n := 0
	for _, x := range exes {
		if x.target == target {
			hit, n = x, n+1
		}
	}
	return hit, n == 1
}

// cargoPackages maps each workspace package's manifest directory to its name.
func cargoPackages(ctx context.Context, p *Project, dir string) (map[string]string, error) {
	r := runProc(ctx, dir, nil, 5*time.Minute, false, append(p.Reg.Runner("cargo", "cargo"), "metadata", "--no-deps", "--format-version", "1")...)
	if r.Failed() {
		return nil, fmt.Errorf("cargo metadata failed in %s:\n%s", p.Rel(dir), r.Stderr)
	}
	var meta struct {
		Packages []struct {
			Name         string `json:"name"`
			ManifestPath string `json:"manifest_path"`
		} `json:"packages"`
	}
	if err := json.Unmarshal(r.Stdout, &meta); err != nil {
		return nil, fmt.Errorf("cargo metadata: %w", err)
	}
	pkgOf := map[string]string{}
	for _, pk := range meta.Packages {
		pkgOf[filepath.Dir(pk.ManifestPath)] = pk.Name
	}
	return pkgOf, nil
}

func targetKey(kinds []string, name string) string {
	for _, k := range kinds {
		switch k {
		case "lib", "rlib", "dylib", "cdylib", "staticlib", "proc-macro":
			return "lib:" + name
		}
	}
	if len(kinds) == 0 {
		return "lib:" + name
	}
	return kinds[0] + ":" + name
}

// build compiles every test binary in dir and maps package + target key →
// binary. pkgOf names the package of each manifest directory.
func (cargoAdapter) build(ctx context.Context, p *Project, dir string, pkgs []string, args []string, pkgOf map[string]string) (map[string]cargoExe, proc) {
	argv := append(p.Reg.Runner("cargo", "cargo"), "test", "--no-run", "--message-format=json-render-diagnostics")
	if len(pkgs) == 0 {
		argv = append(argv, "--workspace")
	}
	for _, pkg := range pkgs {
		argv = append(argv, "-p", pkg)
	}
	argv = append(argv, args...)
	r := runProc(ctx, dir, nil, 60*time.Minute, false, argv...)
	exes := map[string]cargoExe{}
	sc := bufio.NewScanner(bytes.NewReader(r.Stdout))
	sc.Buffer(make([]byte, 1024*1024), 64*1024*1024)
	for sc.Scan() {
		var m cargoMsg
		if json.Unmarshal(sc.Bytes(), &m) != nil || m.Reason != "compiler-artifact" || !m.Profile.Test || m.Executable == "" {
			continue
		}
		md, tk := filepath.Dir(m.ManifestPath), targetKey(m.Target.Kind, m.Target.Name)
		exes[exeKey(pkgOf[md], tk)] = cargoExe{
			exe: m.Executable, manifestDir: md, src: m.Target.SrcPath, pkg: pkgOf[md], target: tk,
		}
	}
	return exes, r
}

func (a cargoAdapter) plan(p *Project, ids []string, floor time.Duration) []Unit {
	type group struct {
		once  sync.Once
		exes  map[string]cargoExe
		build proc
		pkgs  []string
		all   bool
	}
	groups := map[string]*group{}
	for _, id := range ids {
		e := p.Reg.Tests[id]
		key := normDir(e.Dir) + "\x00" + strings.Join(e.Args, "\x00")
		g, ok := groups[key]
		if !ok {
			g = &group{}
			groups[key] = g
		}
		if e.Pkg == "" {
			g.all = true
		} else if !contains(g.pkgs, e.Pkg) {
			g.pkgs = append(g.pkgs, e.Pkg)
		}
	}
	var units []Unit
	for _, id := range ids {
		id, e := id, p.Reg.Tests[id]
		g := groups[normDir(e.Dir)+"\x00"+strings.Join(e.Args, "\x00")]
		timeout := unitTimeout(floor, []*Entry{e})
		units = append(units, Unit{
			Kind: "cargo", IDs: []string{id}, Display: a.repro(p, e),
			Run: func(ctx context.Context) []Result {
				g.once.Do(func() {
					pkgs := g.pkgs
					if g.all {
						pkgs = nil
					}
					pkgOf, err := cargoPackages(ctx, p, p.DirOf(e))
					if err != nil {
						g.build = proc{Exit: 1, Stderr: []byte(err.Error())}
						return
					}
					g.exes, g.build = a.build(ctx, p, p.DirOf(e), pkgs, e.Args, pkgOf)
				})
				res := Result{ID: id, Repro: a.repro(p, e)}
				if g.build.Interrupted {
					res.Status = Interrupted
					return []Result{res}
				}
				if g.build.Failed() {
					res.Status, res.Exit, res.Note, res.Group = Fail, g.build.Exit, "cargo build failed", "cargo:"+p.DirOf(e)
					res.Output = append(append([]byte{}, g.build.Stderr...), g.build.Stdout...)
					return []Result{res}
				}
				x, ok := lookup(g.exes, e.Pkg, e.Target)
				if !ok {
					res.Status, res.Note = Missing, "no test target "+e.Target
					return []Result{res}
				}
				r := runProc(ctx, x.manifestDir, []string{"CARGO_MANIFEST_DIR=" + x.manifestDir}, timeout, true, x.exe, "--exact", e.Name)
				res.Ms, res.Exit = r.Elapsed.Milliseconds(), r.Exit
				out := string(r.Stdout)
				switch {
				case r.Interrupted:
					res.Status = Interrupted
				case r.TimedOut:
					res.Status, res.Output, res.Note = Timeout, r.Stdout, "killed after "+timeout.String()
				case r.Exit == 0 && strings.Contains(out, "running 0 tests"):
					res.Status, res.Note = Missing, "no test by this name in "+e.Target
				case r.Exit == 0 && strings.Contains(out, "test "+e.Name+" ... ignored"):
					res.Status = Skip
				case r.Exit == 0:
					res.Status, res.Timed = Pass, true
				default:
					res.Status, res.Timed, res.Output = Fail, true, r.Stdout
				}
				return []Result{res}
			},
		})
	}
	return units
}

func contains(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

func (a cargoAdapter) discover(ctx context.Context, p *Project, src Source) ([]Found, error) {
	dir := p.DirOf(&Entry{Dir: src.Dir})
	pkgOf, err := cargoPackages(ctx, p, dir)
	if err != nil {
		return nil, err
	}
	exes, b := a.build(ctx, p, dir, nil, nil, pkgOf)
	if b.Failed() {
		return nil, fmt.Errorf("cargo test --no-run failed in %s:\n%s", p.Rel(dir), b.Stderr)
	}
	keys := make([]string, 0, len(exes))
	for k := range exes {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var found []Found
	for _, key := range keys {
		x := exes[key]
		l := runProc(ctx, x.manifestDir, []string{"CARGO_MANIFEST_DIR=" + x.manifestDir}, 5*time.Minute, false, x.exe, "--list", "--format", "terse")
		if l.Failed() {
			return nil, fmt.Errorf("%s --list failed:\n%s", filepath.Base(x.exe), l.Stderr)
		}
		_, tname, _ := strings.Cut(x.target, ":")
		for _, line := range strings.Split(string(l.Stdout), "\n") {
			name, ok := strings.CutSuffix(strings.TrimSpace(line), ": test")
			if !ok {
				continue
			}
			e := Entry{Kind: "cargo", Dir: normDir(src.Dir), Pkg: x.pkg, Target: x.target, Name: name, File: p.relFile(x.src)}
			e.Cmd = a.repro(p, &e)
			found = append(found, Found{ID: tname + "::" + name, AltID: x.pkg + "::" + x.target + "::" + name, Entry: e})
		}
	}
	return found, nil
}
