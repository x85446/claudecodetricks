package testmaster

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// SourceReport is what discovery found at one source.
type SourceReport struct {
	Source  Source
	Known   int
	New     []string
	Missing []string
	Err     error
}

// ErrNoSources means no go, cargo or pytest source is recorded and none is
// at the root or one level below: a project whose tests are all shell.
var ErrNoSources = errors.New("no go.mod, Cargo.toml or pytest config at the root or one level below")

// DetectSources finds the toolchains at the project root, and in its
// immediate subdirectories for toolchains the root does not have.
func DetectSources(root string) []Source {
	var out []Source
	have := map[string]bool{}
	check := func(dir, rel string, skip map[string]bool) {
		for _, kind := range []string{"go", "cargo", "pytest"} {
			if !skip[kind] && hasMarker(dir, kind) {
				out = append(out, Source{Kind: kind, Dir: rel})
				have[kind] = true
			}
		}
	}
	check(root, ".", nil)
	rootHas := map[string]bool{}
	for k := range have {
		rootHas[k] = true
	}
	ents, _ := os.ReadDir(root)
	for _, ent := range ents {
		name := ent.Name()
		if !ent.IsDir() || strings.HasPrefix(name, ".") || name == "node_modules" || name == "vendor" || name == "target" || name == "testdata" {
			continue
		}
		// A root Cargo workspace or pytest rootdir already covers its
		// children; a nested go.mod is a separate module, so go is checked.
		check(filepath.Join(root, name), name, map[string]bool{"cargo": rootHas["cargo"], "pytest": rootHas["pytest"]})
	}
	return out
}

func hasMarker(dir, kind string) bool {
	exists := func(name string) bool {
		_, err := os.Stat(filepath.Join(dir, name))
		return err == nil
	}
	contains := func(name, needle string) bool {
		b, err := os.ReadFile(filepath.Join(dir, name))
		return err == nil && strings.Contains(string(b), needle)
	}
	switch kind {
	case "go":
		return exists("go.mod")
	case "cargo":
		return exists("Cargo.toml")
	case "pytest":
		return exists("pytest.ini") || exists("conftest.py") || contains("pyproject.toml", "[tool.pytest") ||
			contains("setup.cfg", "[tool:pytest]") || contains("tox.ini", "[pytest]")
	}
	return false
}

func (p *Project) key(e *Entry) string {
	a, err := adapterFor(e.Kind)
	if err != nil {
		return ""
	}
	return e.Kind + "\x00" + a.identity(e)
}

// Discover asks each source's toolchain what tests exist, registers the new
// ones unmeasured, and reports registered tests the toolchain no longer
// sees. It never removes an entry: that is a prune decision.
func (p *Project) Discover(ctx context.Context, only *Source, dry bool) ([]SourceReport, error) {
	var sources []Source
	switch {
	case only != nil:
		only.Dir = normDir(only.Dir)
		sources = []Source{*only}
		known := false
		for _, s := range p.Reg.Sources {
			known = known || (s.Kind == only.Kind && normDir(s.Dir) == only.Dir)
		}
		if !known {
			p.Reg.Sources = append(p.Reg.Sources, Source{Kind: only.Kind, Dir: dirOrDot(only.Dir)})
		}
	case len(p.Reg.Sources) > 0:
		sources = p.Reg.Sources
	default:
		sources = DetectSources(p.Root)
		if len(sources) == 0 {
			return nil, ErrNoSources
		}
		p.Reg.Sources = sources
	}

	index := map[string]string{}
	for id, e := range p.Reg.Tests {
		index[p.key(e)] = id
	}
	var reports []SourceReport
	for _, src := range sources {
		rep := SourceReport{Source: src}
		a, err := adapterFor(src.Kind)
		if err != nil {
			rep.Err = err
			reports = append(reports, rep)
			continue
		}
		found, err := a.discover(ctx, p, src)
		if err != nil {
			rep.Err = err
			reports = append(reports, rep)
			continue
		}
		seen := map[string]bool{}
		for _, f := range found {
			e := f.Entry
			k := p.key(&e)
			seen[k] = true
			if _, ok := index[k]; ok {
				rep.Known++
				continue
			}
			id := p.freeID(f)
			e.Tier = "?"
			p.Reg.Tests[id] = &e
			index[k] = id
			rep.New = append(rep.New, id)
		}
		for id, e := range p.Reg.Tests {
			if e.Kind == src.Kind && normDir(e.Dir) == normDir(src.Dir) && !seen[p.key(e)] {
				rep.Missing = append(rep.Missing, id)
			}
		}
		sort.Strings(rep.New)
		sort.Strings(rep.Missing)
		reports = append(reports, rep)
	}
	if !dry {
		if err := p.Save(); err != nil {
			return reports, err
		}
	}
	return reports, nil
}

func dirOrDot(d string) string {
	if d == "" {
		return "."
	}
	return d
}

func (p *Project) freeID(f Found) string {
	for _, cand := range []string{f.ID, f.AltID} {
		if _, taken := p.Reg.Tests[cand]; cand != "" && !taken {
			return cand
		}
	}
	for n := 2; ; n++ {
		if cand := fmt.Sprintf("%s#%d", f.ID, n); p.Reg.Tests[cand] == nil {
			return cand
		}
	}
}
