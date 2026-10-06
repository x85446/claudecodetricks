package testmaster

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// RunOptions tunes one run.
type RunOptions struct {
	Jobs     int           // concurrent invocations for parallel-safe tests
	Timeout  time.Duration // floor per invocation; measured tests get 3× their average when larger
	DryRun   bool          // print the invocations, run nothing, write nothing
	Out      io.Writer     // dry-run listing
	Progress func(done, total int)
	Verbose  io.Writer // one line per invocation when set
}

// TierChange is a test whose measured average crossed a tier boundary.
type TierChange struct {
	ID    string `json:"id"`
	From  string `json:"from"`
	To    string `json:"to"`
	AvgMs int64  `json:"avg_ms"`
}

// Report is what a run tells its caller: counts, and every result that was
// not a pass.
type Report struct {
	Selection   string         `json:"selection"`
	Ms          int64          `json:"ms"`
	Counts      map[string]int `json:"counts"`
	Problems    []Result       `json:"-"`
	TierChanges []TierChange   `json:"tier_changes,omitempty"`
	BlockedBy   string         `json:"blocked_by,omitempty"`
	Interrupted bool           `json:"interrupted,omitempty"`
}

// Select resolves selection words to registry ids. No words means the
// default set: fast + standard + every unmeasured test. Tier words, "all",
// exact ids and globs (iterrun.*) combine; --failed narrows to tests whose
// last result was a failure.
func (p *Project) Select(words []string, failed bool) ([]string, string, error) {
	tiers := map[string]bool{}
	all := false
	var pats []string
	for _, w := range words {
		switch w {
		case "fast", "standard", "slow":
			tiers[w] = true
		case "all":
			all = true
		default:
			pats = append(pats, w)
		}
	}
	defaultSet := len(words) == 0 && !failed
	if defaultSet {
		tiers["fast"], tiers["standard"] = true, true
	}
	var label []string
	switch {
	case all:
		label = append(label, "all")
	case len(tiers) > 0:
		for _, t := range []string{"fast", "standard", "slow"} {
			if tiers[t] {
				label = append(label, t)
			}
		}
	}
	chosen := map[string]bool{}
	for _, id := range p.Reg.IDs() {
		e := p.Reg.Tests[id]
		switch {
		case all, failed && len(words) == 0:
			chosen[id] = true
		case len(tiers) > 0 && (tiers[e.Tier] || !e.Measured()):
			chosen[id] = true
		}
	}
	for _, pat := range pats {
		n := 0
		for _, id := range p.Reg.IDs() {
			if ok, _ := path.Match(pat, id); ok || pat == id {
				chosen[id] = true
				n++
			}
		}
		if n == 0 {
			msg := fmt.Sprintf("no test matches %q", pat)
			if s := suggest(pat, p.Reg.IDs()); s != "" {
				msg += fmt.Sprintf(" (did you mean %s?)", s)
			}
			return nil, "", fmt.Errorf("%s", msg)
		}
		if len(pats) <= 3 {
			label = append(label, pat)
		}
	}
	if len(pats) > 3 {
		label = append(label, fmt.Sprintf("%d patterns", len(pats)))
	}
	var ids []string
	for _, id := range p.Reg.IDs() {
		if !chosen[id] {
			continue
		}
		if failed {
			if r := p.Reg.Tests[id].LastResult; r != Fail && r != Timeout {
				continue
			}
		}
		ids = append(ids, id)
	}
	if failed {
		label = append(label, "failed")
	}
	return ids, strings.Join(label, "+"), nil
}

func suggest(want string, ids []string) string {
	best, bestD := "", 3
	for _, id := range ids {
		for _, cand := range []string{id, id[strings.LastIndex(id, ".")+1:]} {
			if d := levenshtein(want, cand); d < bestD {
				best, bestD = id, d
			}
		}
	}
	return best
}

func levenshtein(a, b string) int {
	prev := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		cur := make([]int, len(b)+1)
		cur[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			cur[j] = min(prev[j]+1, cur[j-1]+1, prev[j-1]+cost)
		}
		prev = cur
	}
	return prev[len(b)]
}

// plan turns ids into invocations, grouped by kind so each adapter batches
// its own tests.
func (p *Project) plan(ids []string, floor time.Duration) ([]Unit, error) {
	byKind := map[string][]string{}
	for _, id := range ids {
		k := p.Reg.Tests[id].Kind
		byKind[k] = append(byKind[k], id)
	}
	kinds := make([]string, 0, len(byKind))
	for k := range byKind {
		kinds = append(kinds, k)
	}
	sort.Strings(kinds)
	var units []Unit
	for _, k := range kinds {
		a, err := adapterFor(k)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", byKind[k][0], err)
		}
		units = append(units, a.plan(p, byKind[k], floor)...)
	}
	return units, nil
}

func (p *Project) estimate(u Unit) int64 {
	var sum int64
	for _, id := range u.IDs {
		if e := p.Reg.Tests[id]; e.AvgMs != nil {
			sum += *e.AvgMs
		} else {
			sum += FastMs
		}
	}
	return sum
}

// Run executes the selection in three phases. Blocking tests go first, one
// at a time; the first one that fails stops the run and everything else is
// reported not run. Then every parallel-safe invocation runs concurrently,
// longest first. Then the parallel=no tests run one at a time. Every
// independent test runs whatever the others do.
func (p *Project) Run(ctx context.Context, ids []string, label string, opt RunOptions) (*Report, error) {
	start := time.Now()
	if opt.Jobs < 1 {
		opt.Jobs = 1
	}
	var blocking, par, ser []string
	for _, id := range ids {
		switch e := p.Reg.Tests[id]; {
		case e.Blocking:
			blocking = append(blocking, id)
		case e.Parallel():
			par = append(par, id)
		default:
			ser = append(ser, id)
		}
	}
	var blockUnits [][]Unit
	for _, id := range blocking {
		u, err := p.plan([]string{id}, opt.Timeout)
		if err != nil {
			return nil, err
		}
		blockUnits = append(blockUnits, u)
	}
	parUnits, err := p.plan(par, opt.Timeout)
	if err != nil {
		return nil, err
	}
	sort.SliceStable(parUnits, func(i, j int) bool { return p.estimate(parUnits[i]) > p.estimate(parUnits[j]) })
	serUnits, err := p.plan(ser, opt.Timeout)
	if err != nil {
		return nil, err
	}

	if opt.DryRun {
		for _, us := range blockUnits {
			for _, u := range us {
				fmt.Fprintf(opt.Out, "blocking\t%s\t%d\t%s\n", u.Kind, len(u.IDs), u.Display)
			}
		}
		for _, u := range parUnits {
			fmt.Fprintf(opt.Out, "parallel\t%s\t%d\t%s\n", u.Kind, len(u.IDs), u.Display)
		}
		for _, u := range serUnits {
			fmt.Fprintf(opt.Out, "serial\t%s\t%d\t%s\n", u.Kind, len(u.IDs), u.Display)
		}
		return nil, nil
	}

	results := map[string]Result{}
	var mu sync.Mutex
	done := 0
	record := func(u Unit, rs []Result) {
		mu.Lock()
		defer mu.Unlock()
		for _, r := range fillMissing(u.IDs, rs, Fail, "runner returned no result", nil) {
			results[r.ID] = r
		}
		done += len(u.IDs)
		if opt.Progress != nil {
			opt.Progress(done, len(ids))
		}
	}
	runUnit := func(u Unit) []Result {
		if ctx.Err() != nil {
			return fillMissing(u.IDs, nil, Interrupted, "", nil)
		}
		if opt.Verbose != nil {
			fmt.Fprintf(opt.Verbose, "▸ %s\n", u.Display)
		}
		return u.Run(ctx)
	}

	rep := &Report{Selection: label, Counts: map[string]int{}}
	for i, us := range blockUnits {
		for _, u := range us {
			rs := runUnit(u)
			record(u, rs)
			for _, r := range rs {
				if r.failed() || r.Status == Missing {
					rep.BlockedBy = blocking[i]
				}
			}
		}
		if rep.BlockedBy != "" {
			break
		}
	}
	if rep.BlockedBy != "" {
		for _, id := range ids {
			if _, ok := results[id]; !ok {
				results[id] = Result{ID: id, Status: NotRun}
			}
		}
	} else {
		sem := make(chan struct{}, opt.Jobs)
		var wg sync.WaitGroup
		for _, u := range parUnits {
			wg.Add(1)
			go func(u Unit) {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				record(u, runUnit(u))
			}(u)
		}
		wg.Wait()
		for _, u := range serUnits {
			record(u, runUnit(u))
		}
	}

	rep.Ms = time.Since(start).Milliseconds()
	rep.Interrupted = ctx.Err() != nil
	if err := p.apply(results, rep); err != nil {
		return rep, err
	}
	return rep, nil
}

// apply writes a run's outcome: registry timing and results, history lines,
// and the failure logs (written for failures, removed for everything that
// is no longer failing).
func (p *Project) apply(results map[string]Result, rep *Report) error {
	ts := now()
	commit := ""
	if out, err := exec.Command("git", "-C", p.Root, "rev-parse", "--short=12", "HEAD").Output(); err == nil {
		commit = strings.TrimSpace(string(out))
	}
	if err := p.ensureFailureDir(); err != nil {
		return err
	}
	hist, err := os.OpenFile(filepath.Join(p.Root, StateDir, "history.jsonl"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer hist.Close()
	enc := json.NewEncoder(hist)
	enc.SetEscapeHTML(false)
	var green []string

	ids := make([]string, 0, len(results))
	for id := range results {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		r := results[id]
		e := p.Reg.Tests[id]
		rep.Counts[r.Status]++
		if r.Status == NotRun || r.Status == Interrupted {
			continue
		}
		if r.Status != Pass {
			rep.Problems = append(rep.Problems, r)
		}
		if r.Timed && (r.Status == Pass || r.Status == Fail) {
			wasMeasured, oldTier := e.Measured(), e.Tier
			var avg int64
			if e.AvgMs != nil {
				avg = *e.AvgMs
			}
			e.Runs++
			avg = (avg*int64(e.Runs-1) + r.Ms) / int64(e.Runs)
			e.AvgMs, e.LastMs = &avg, &r.Ms
			e.Tier = TierFor(avg)
			if wasMeasured && oldTier != e.Tier {
				rep.TierChanges = append(rep.TierChanges, TierChange{ID: id, From: oldTier, To: e.Tier, AvgMs: avg})
			}
		}
		e.LastResult, e.Updated = r.Status, ts
		if r.Status == Pass {
			green = append(green, id)
		}
		line := map[string]any{"ts": ts, "id": id, "result": r.Status, "tier": e.Tier, "runner": "testmaster"}
		if r.Timed {
			line["ms"] = r.Ms
		}
		if err := enc.Encode(line); err != nil {
			return err
		}
		logPath := p.LogPath(id)
		if r.failed() {
			if err := writeFailureLog(logPath, id, r, ts, commit); err != nil {
				return err
			}
		} else {
			os.Remove(logPath)
		}
	}
	last := &LastRun{
		TS: ts, Selection: rep.Selection, Ms: rep.Ms,
		Passed: rep.Counts[Pass], Failed: rep.Counts[Fail] + rep.Counts[Timeout],
		Skipped: rep.Counts[Skip], Missing: rep.Counts[Missing], NotRun: rep.Counts[NotRun],
	}
	if err := enc.Encode(map[string]any{
		"ts": ts, "batch": rep.Selection, "ms": rep.Ms, "passed": last.Passed, "failed": last.Failed,
		"skipped": last.Skipped, "missing": last.Missing, "not_run": last.NotRun, "runner": "testmaster",
	}); err != nil {
		return err
	}
	if !rep.Interrupted {
		p.Reg.LastRun = last
	}
	if err := p.Save(); err != nil {
		return err
	}
	return p.stampCatalog(green, commit, ts)
}

// stampCatalog records, on every catalog case a green test backs, the commit
// it was proven against. /testmaster-catalog recomputes validity from that
// stamp; the runner never decides validity itself.
func (p *Project) stampCatalog(green []string, commit, ts string) error {
	if len(green) == 0 || commit == "" {
		return nil
	}
	cat, err := p.readCatalog()
	if cat == nil {
		return err
	}
	ok := map[string]bool{}
	for _, id := range green {
		ok[id] = true
	}
	short := commit
	if len(short) > 8 {
		short = short[:8]
	}
	stamped := 0
	for _, cs := range catalogCases(cat) {
		id, _ := cs["id"].(string)
		if ok[id] || allIn(caseTests(cs), ok) {
			cs["last_validated_commit"], cs["last_validated"] = short, ts
			stamped++
		}
	}
	if stamped == 0 {
		return nil
	}
	out, err := marshal(cat, "  ")
	if err != nil {
		return err
	}
	return p.writeState("catalog.json", out)
}

// allIn reports whether ids is non-empty and every one is in set: a case
// backed by several tests is proven only when all of them passed.
func allIn(ids []string, set map[string]bool) bool {
	for _, id := range ids {
		if !set[id] {
			return false
		}
	}
	return len(ids) > 0
}

const logCap = 1 << 20 // keep the first and last MiB of a huge failure

func writeFailureLog(path, id string, r Result, ts, commit string) error {
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	fmt.Fprintf(f, "test:    %s\nresult:  %s\n", id, r.Status)
	if r.Note != "" {
		fmt.Fprintf(f, "cause:   %s\n", r.Note)
	}
	if r.Exit != 0 {
		fmt.Fprintf(f, "exit:    %d\n", r.Exit)
	}
	if r.Timed {
		fmt.Fprintf(f, "time:    %s\n", fmtMs(r.Ms))
	}
	fmt.Fprintf(f, "when:    %s\n", ts)
	if commit != "" {
		fmt.Fprintf(f, "commit:  %s\n", commit)
	}
	if r.Repro != "" {
		fmt.Fprintf(f, "rerun:   %s\n", r.Repro)
	}
	fmt.Fprintf(f, "--- output ---\n")
	out := r.Output
	if len(out) > 2*logCap {
		fmt.Fprintf(f, "%s\n… %d bytes cut …\n%s", out[:logCap], len(out)-2*logCap, out[len(out)-logCap:])
	} else {
		f.Write(out)
	}
	return nil
}

func fmtMs(ms int64) string {
	d := time.Duration(ms) * time.Millisecond
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	return d.Round(time.Second).String()
}

// ExitCode is 0 only when every selected test passed or skipped.
func (r *Report) ExitCode() int {
	switch {
	case r.Interrupted:
		return 130
	case r.Counts[Fail]+r.Counts[Timeout]+r.Counts[Missing]+r.Counts[NotRun] > 0:
		return 1
	}
	return 0
}

// Print writes the human/AI report: one summary line, then one line per
// problem. Passes are only ever a count.
func (r *Report) Print(w io.Writer, p *Project, cwd string) {
	parts := []string{
		fmt.Sprintf("%d passed", r.Counts[Pass]),
		fmt.Sprintf("%d failed", r.Counts[Fail]+r.Counts[Timeout]),
	}
	for _, k := range []struct{ status, word string }{{Skip, "skipped"}, {Missing, "missing"}, {NotRun, "not run"}, {Interrupted, "interrupted"}} {
		if n := r.Counts[k.status]; n > 0 {
			parts = append(parts, fmt.Sprintf("%d %s", n, k.word))
		}
	}
	sel := r.Selection
	if sel == "" {
		sel = "selection"
	}
	fmt.Fprintf(w, "run %s: %s, %s\n", sel, strings.Join(parts, ", "), fmtMs(r.Ms))

	shown := map[string]bool{}
	for _, res := range r.Problems {
		if res.Status == Skip {
			continue
		}
		where := displayPath(p.LogPath(res.ID), cwd)
		tag := strings.ToUpper(res.Status)
		if res.Group != "" {
			if shown[res.Group] {
				continue
			}
			shown[res.Group] = true
			n := 0
			for _, o := range r.Problems {
				if o.Group == res.Group {
					n++
				}
			}
			_, scope, _ := strings.Cut(res.Group, ":")
			fmt.Fprintf(w, "%s\t%d tests in %s\t%s\t%s\n", tag, n, p.Rel(scope), where, res.Note)
			continue
		}
		switch res.Status {
		case Missing:
			fmt.Fprintf(w, "MISSING\t%s\t%s\n", res.ID, res.Note)
		default:
			if res.Note != "" {
				fmt.Fprintf(w, "%s\t%s\t%s\t%s\n", tag, res.ID, where, res.Note)
			} else {
				fmt.Fprintf(w, "%s\t%s\t%s\n", tag, res.ID, where)
			}
		}
	}
	if r.BlockedBy != "" {
		fmt.Fprintf(w, "BLOCKED\t%d tests not run: blocking test %s did not pass\n", r.Counts[NotRun], r.BlockedBy)
	}
	for _, t := range r.TierChanges {
		note := ""
		if t.To == "slow" {
			note = "\tleaves the default selection"
		}
		fmt.Fprintf(w, "TIER\t%s\t%s→%s\t%s avg%s\n", t.ID, t.From, t.To, fmtMs(t.AvgMs), note)
	}
}

// JSON renders the report for --json callers.
func (r *Report) JSON(p *Project, cwd string) ([]byte, error) {
	type problem struct {
		ID     string `json:"id"`
		Status string `json:"status"`
		Log    string `json:"log,omitempty"`
		Note   string `json:"note,omitempty"`
		Rerun  string `json:"rerun,omitempty"`
	}
	var probs []problem
	for _, res := range r.Problems {
		pr := problem{ID: res.ID, Status: res.Status, Note: res.Note, Rerun: res.Repro}
		if res.failed() {
			pr.Log = displayPath(p.LogPath(res.ID), cwd)
		}
		probs = append(probs, pr)
	}
	return json.Marshal(struct {
		*Report
		Problems []problem `json:"problems"`
		Exit     int       `json:"exit"`
	}{r, probs, r.ExitCode()})
}

func displayPath(abs, cwd string) string {
	if rel, err := filepath.Rel(cwd, abs); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return abs
}
