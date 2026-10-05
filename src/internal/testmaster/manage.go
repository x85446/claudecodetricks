package testmaster

import (
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
)

// Add registers one test by hand, unmeasured. Shell tests come in this way;
// the other kinds usually arrive through Discover.
func (p *Project) Add(id string, e Entry) error {
	if id == "" {
		return fmt.Errorf("a test needs an id")
	}
	if _, taken := p.Reg.Tests[id]; taken {
		return fmt.Errorf("test %q is already registered", id)
	}
	a, err := adapterFor(e.Kind)
	if err != nil {
		return err
	}
	need := map[string][]struct{ flag, val string }{
		"go":     {{"--pkg", e.Pkg}, {"--name", e.Name}},
		"pytest": {{"--nodeid", e.NodeID}},
		"cargo":  {{"--target", e.Target}, {"--name", e.Name}},
		"shell":  {{"--cmd", e.Cmd}},
	}
	for _, n := range need[e.Kind] {
		if n.val == "" {
			return fmt.Errorf("a %s test needs %s", e.Kind, n.flag)
		}
	}
	e.Dir = normDir(e.Dir)
	if e.Kind != "shell" {
		e.Cmd = a.repro(p, &e)
	}
	k := p.key(&e)
	for other, o := range p.Reg.Tests {
		if p.key(o) == k {
			return fmt.Errorf("that test is already registered as %q", other)
		}
	}
	e.Tier, e.Runs, e.AvgMs, e.LastMs = "?", 0, nil, nil
	p.Reg.Tests[id] = &e
	return p.Save()
}

// Set changes the declared properties of registered tests.
func (p *Project) Set(ids []string, blocking, parallel *bool) error {
	for _, id := range ids {
		if p.Reg.Tests[id] == nil {
			return fmt.Errorf("no test %q", id)
		}
	}
	for _, id := range ids {
		e := p.Reg.Tests[id]
		if blocking != nil {
			e.Blocking = *blocking
		}
		if parallel != nil {
			v := *parallel
			e.ParallelSafe = &v
		}
	}
	return p.Save()
}

// Remove deletes registry entries and their failure logs. History stays:
// it is the record of what ran.
func (p *Project) Remove(ids []string) error {
	for _, id := range ids {
		if p.Reg.Tests[id] == nil {
			return fmt.Errorf("no test %q", id)
		}
	}
	for _, id := range ids {
		delete(p.Reg.Tests, id)
		os.Remove(p.LogPath(id))
	}
	return p.Save()
}

// ListFilter narrows `test list`.
type ListFilter struct {
	Tier       string
	Kind       string
	Failing    bool
	Unmeasured bool
	Blocking   bool
}

// List returns matching ids, sorted.
func (p *Project) List(f ListFilter) []string {
	var out []string
	for _, id := range p.Reg.IDs() {
		e := p.Reg.Tests[id]
		switch {
		case f.Tier != "" && e.Tier != f.Tier,
			f.Kind != "" && e.Kind != f.Kind,
			f.Failing && e.LastResult != Fail && e.LastResult != Timeout,
			f.Unmeasured && e.Measured(),
			f.Blocking && !e.Blocking:
			continue
		}
		out = append(out, id)
	}
	return out
}

// Status writes the one-screen summary of the registry.
func (p *Project) Status(w io.Writer, cwd string) {
	tiers := map[string]int{}
	results := map[string]int{}
	kinds := map[string]int{}
	var failing, blocking []string
	serial := 0
	for _, id := range p.Reg.IDs() {
		e := p.Reg.Tests[id]
		if e.Measured() {
			tiers[e.Tier]++
		} else {
			tiers["unmeasured"]++
		}
		r := e.LastResult
		if r == "" {
			r = "never run"
		}
		results[r]++
		kinds[e.Kind]++
		if e.LastResult == Fail || e.LastResult == Timeout {
			failing = append(failing, id)
		}
		if e.Blocking {
			blocking = append(blocking, id)
		}
		if !e.Parallel() {
			serial++
		}
	}
	fmt.Fprintf(w, "tests\t%d\t%s\n", len(p.Reg.Tests), counts(kinds, nil))
	fmt.Fprintf(w, "tiers\t%s\n", counts(tiers, []string{"fast", "standard", "slow", "unmeasured"}))
	fmt.Fprintf(w, "results\t%s\n", counts(results, []string{Pass, Fail, Timeout, Skip, Missing, "never run"}))
	if lr := p.Reg.LastRun; lr != nil {
		fmt.Fprintf(w, "last run\t%s\t%s\t%s\t%d passed, %d failed\n", lr.TS, lr.Selection, fmtMs(lr.Ms), lr.Passed, lr.Failed)
	}
	if len(blocking) > 0 {
		fmt.Fprintf(w, "blocking\t%s\n", strings.Join(blocking, " "))
	}
	if serial > 0 {
		fmt.Fprintf(w, "serial\t%d\n", serial)
	}
	for _, id := range failing {
		fmt.Fprintf(w, "FAIL\t%s\t%s\n", id, displayPath(p.LogPath(id), cwd))
	}
	type slow struct {
		id string
		ms int64
	}
	var all []slow
	for id, e := range p.Reg.Tests {
		if e.AvgMs != nil {
			all = append(all, slow{id, *e.AvgMs})
		}
	}
	sort.Slice(all, func(i, j int) bool { return all[i].ms > all[j].ms || all[i].ms == all[j].ms && all[i].id < all[j].id })
	for i := 0; i < len(all) && i < 5; i++ {
		fmt.Fprintf(w, "slowest\t%s\t%s\n", fmtMs(all[i].ms), all[i].id)
	}
}

func counts(m map[string]int, order []string) string {
	if order == nil {
		for k := range m {
			order = append(order, k)
		}
		sort.Strings(order)
	}
	var parts []string
	for _, k := range order {
		if m[k] > 0 {
			parts = append(parts, fmt.Sprintf("%s %d", k, m[k]))
		}
	}
	return strings.Join(parts, ", ")
}
