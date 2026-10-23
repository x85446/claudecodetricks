package iterrun

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

// graphProject writes plan files into a fresh project and returns a
// conductor service with that project enabled under an isolated store.
func graphProject(t *testing.T, plans map[string]string, archive map[string]string) (*conductor.Service, string) {
	t.Helper()
	dir := t.TempDir()
	for sub, files := range map[string]map[string]string{"plans": plans, "archive": archive} {
		base := filepath.Join(dir, ".claude", "iterate", sub)
		if e := os.MkdirAll(base, 0o700); e != nil {
			t.Fatal(e)
		}
		for name, body := range files {
			if e := os.WriteFile(filepath.Join(base, name), []byte(body), 0o600); e != nil {
				t.Fatal(e)
			}
		}
	}
	s := NewConductorService(t.TempDir())
	now := time.Date(2026, 10, 16, 12, 0, 0, 0, time.UTC)
	s.Clock = func() time.Time { return now }
	c := model.Config{Version: 2, Enabled: true, PowerBands: []model.PowerBand{budget.Universal()}, Projects: []model.Project{{ID: "project", Name: "fixture", Path: dir, Enabled: true}}, Subscriptions: []model.Subscription{{ID: "first", Name: "First", Enabled: true, Adapter: "generic", Executable: "/usr/bin/true", Args: []string{"{prompt}", "{result}"}, PoolID: "pool", AccountID: "account", PowerBandID: "universal", Sequences: []model.Sequence{{ID: "sequence", Name: "build"}}}}, Automations: []model.Automation{{ID: "rule", Name: "Work", ProjectID: "project", Enabled: true, Mode: "keep_working", Entries: []model.Entry{{ID: "entry", SequenceID: "sequence", MaxBand: model.Green}}}}}
	if _, e := s.Apply(0, c); e != nil {
		t.Fatal(e)
	}
	u := model.Usage{SubscriptionID: "first", PoolID: "pool", AccountID: "account", Source: "isolated acceptance fixture", ObservedAt: now, Supported: true, Weekly: &model.Allowance{UsedPercent: 10, StartsAt: now.Add(-3 * 24 * time.Hour), ResetsAt: now.Add(4 * 24 * time.Hour), ResetTrusted: true, Epoch: "fixture"}, Short: &model.Allowance{UsedPercent: 0, StartsAt: now.Add(-time.Hour), ResetsAt: now.Add(4 * time.Hour), ResetTrusted: true, Epoch: "short"}}
	if e := s.PutUsage(u); e != nil {
		t.Fatal(e)
	}
	return s, dir
}

func graphPlan(name string, keys ...string) string {
	return "# Iterate Task — " + name + "\n\nname: " + name + "\nStarted: 2026-09-11T02:06:12Z (planned)\n" + strings.Join(keys, "\n") + "\nrunning: false\n\n## Goal\nfixture\n\n## Steps\n- [ ] 1. work\n\n## Validation\n- [ ] 1. verify\n"
}

func eligible(d conductor.Decision) []string {
	out := []string{}
	for _, x := range d.Candidates {
		if x.Eligible {
			out = append(out, x.Plan.Name)
		}
	}
	return out
}

// TESTMASTER: id=dag-adm-11-disk tier=fast parallel=yes
func TestConductorHoldsTheIzcrosChainAsItSitsOnDisk(t *testing.T) {
	names := []string{"caiman", "dace", "earthworm", "fairywren", "gadwall", "haddock", "ibex", "jabiru"}
	plans := map[string]string{"caiman.md": graphPlan("caiman", "phase: executing", "status: blocked-on-operator: owned assignment interrupted", "depends: baboon")}
	for i := 1; i < len(names); i++ {
		plans[names[i]+".md"] = graphPlan(names[i], "phase: executing", "status: unblocked", "depends: "+names[i-1])
	}
	plans["independent.md"] = graphPlan("independent", "phase: planned", "status: queued: 2026-10-16T00:00:00Z by /ip stage")
	s, _ := graphProject(t, plans, map[string]string{"2026-10-09T05-24-17Z-baboon-done.md": graphPlan("baboon", "phase: executing")})
	d, e := s.Evaluate(context.Background(), conductor.EvaluateRequest{})
	if e != nil {
		t.Fatal(e)
	}
	if got := strings.Join(eligible(d), ","); got != "independent" {
		t.Fatalf("eligible = %q, want only the independent plan", got)
	}
	held := 0
	for _, p := range d.Queues[0].Plans {
		if p.Waiting {
			if p.HeldBy != "caiman" || !strings.HasPrefix(p.WaitReason, "waiting on ") {
				t.Fatalf("%s = %+v", p.Name, p)
			}
			held++
		}
	}
	if held != 7 {
		t.Fatalf("held = %d, want 7", held)
	}
	h := ConductorAPI(s)
	w := conductorRequest(t, h, "GET", "status", "", "localhost:8420", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"wait_reason":"waiting on dace (unblocked)"`) {
		t.Fatalf("queue API does not report the wait: %d %s", w.Code, w.Body.String())
	}
}

// TESTMASTER: id=dag-adm-5-disk tier=fast parallel=yes
func TestConductorAdmitsADependentOnceItsPredecessorArchivesDone(t *testing.T) {
	s, dir := graphProject(t, map[string]string{
		"alpha.md": graphPlan("alpha", "phase: executing", "status: blocked-on-operator: token"),
		"bravo.md": graphPlan("bravo", "phase: planned", "status: queued: 2026-10-16T00:00:00Z by /ip stage", "depends: alpha"),
	}, nil)
	bravo := filepath.Join(dir, ".claude", "iterate", "plans", "bravo.md")
	before, _ := os.ReadFile(bravo)
	d, _ := s.Evaluate(context.Background(), conductor.EvaluateRequest{})
	if len(eligible(d)) != 0 {
		t.Fatalf("eligible before the predecessor landed: %v", eligible(d))
	}
	plans := filepath.Join(dir, ".claude", "iterate", "plans")
	if e := os.Rename(filepath.Join(plans, "alpha.md"), filepath.Join(dir, ".claude", "iterate", "archive", "20261016T110000Z-alpha-done.md")); e != nil {
		t.Fatal(e)
	}
	d, _ = s.Evaluate(context.Background(), conductor.EvaluateRequest{})
	if got := strings.Join(eligible(d), ","); got != "bravo" {
		t.Fatalf("eligible = %q, want bravo", got)
	}
	after, _ := os.ReadFile(bravo)
	if !bytes.Equal(before, after) {
		t.Fatal("bravo's file changed; waiting must be computed, never stored")
	}
}

// TESTMASTER: id=dag-err-adapter tier=fast parallel=yes
func TestConductorBlocksAPlanWhoseDependencyIsUnknown(t *testing.T) {
	s, _ := graphProject(t, map[string]string{
		"bravo.md": graphPlan("bravo", "phase: planned", "status: queued: 2026-10-16T00:00:00Z by /ip stage", "depends: ghost"),
	}, nil)
	d, _ := s.Evaluate(context.Background(), conductor.EvaluateRequest{})
	var got []byte
	for _, x := range d.Candidates {
		got, _ = json.Marshal(x)
		if x.Code != "plan_blocked" || !strings.Contains(x.Reason, "unknown dependency ghost") {
			t.Fatalf("candidate = %s", got)
		}
	}
	if len(d.Candidates) == 0 {
		t.Fatal("no candidate explained the unknown dependency")
	}
}

// TESTMASTER: id=dag-adm-8 tier=fast parallel=no
func TestDagAdm8ANightlyTickLaunchesNothingForAWaitingQueue(t *testing.T) {
	home := nightlyHome(t)
	dir := mkNightlyProject(t, home, "held", enrolledConductor, "")
	plans := filepath.Join(dir, ".claude", "iterate", "plans")
	for name, body := range map[string]string{
		"alpha.md":   graphPlan("alpha", "phase: executing", "status: blocked-on-operator: token"),
		"bravo.md":   graphPlan("bravo", "phase: planned", "status: queued: 2026-10-05T00:00:00Z by /ip stage", "depends: alpha"),
		"charlie.md": graphPlan("charlie", "phase: planned", "status: queued: 2026-10-05T00:00:00Z by /ip stage", "depends: bravo"),
	} {
		if e := os.WriteFile(filepath.Join(plans, name), []byte(body), 0o644); e != nil {
			t.Fatal(e)
		}
	}
	d := decideProject(dir, mondayNight, 15*time.Minute)
	if d.Launch || d.Reason != "nothing startable (2 waiting)" {
		t.Fatalf("decision = %s, want skip: nothing startable (2 waiting)", d)
	}

	s, _ := graphProject(t, map[string]string{
		"alpha.md": graphPlan("alpha", "phase: executing", "status: blocked-on-operator: token"),
		"bravo.md": graphPlan("bravo", "phase: planned", "status: queued: 2026-10-05T00:00:00Z by /ip stage", "depends: alpha"),
	}, nil)
	started := 0
	spy := func(string, string) (int, error) { started++; return 1, nil }
	if e := wakeConductor(context.Background(), s, spy); e != nil || started != 1 {
		t.Fatalf("first wake must record the watch: %v %d", e, started)
	}
	if out, e := s.Tick(context.Background()); e != nil || out.Assignment != nil {
		t.Fatalf("tick admitted held work: %+v %v", out.Assignment, e)
	}
	for i := 0; i < 3; i++ {
		if e := wakeConductor(context.Background(), s, spy); e != nil {
			t.Fatal(e)
		}
	}
	if started != 1 {
		t.Fatalf("launcher spy recorded %d calls after the watch stood down, want 1", started)
	}
}
