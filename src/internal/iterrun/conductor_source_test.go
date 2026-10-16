package iterrun

import (
	"context"
	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeConductorPlan(t *testing.T, project, text string) string {
	t.Helper()
	path := filepath.Join(project, ".claude", "iterate", "plans", "alpha.md")
	if e := conductor.AtomicWrite(path, []byte(text)); e != nil {
		t.Fatal(e)
	}
	return path
}

const sourcePlan = "name: alpha\nphase: planned\nstatus: queued\nrunning: false\n\n## Steps\n- [ ] 1. dependent work\n- [ ] 2. prerequisite work\n\n## Validation\n- [ ] 1. verify dependent\n- [ ] 2. verify prerequisite\n\n## Teams\n| Team | Steps | Depends | Agent | Status |\n| --- | --- | --- | --- | --- |\n| dependent | 1 | prerequisite | backend | pending |\n| prerequisite | 2 | — | backend | pending |\n"

// TESTMASTER: id=indigobird-execution-teams tier=fast parallel=yes
func TestConductorSourceDependencyCheckpoint(t *testing.T) {
	ctx := context.Background()
	p := model.Project{ID: "p", Path: t.TempDir()}
	path := writeConductorPlan(t, p.Path, sourcePlan)
	src := ConductorPlanSource{}
	pair, e := src.Pair(ctx, p, "alpha")
	if e != nil || pair.Step != 2 || pair.Team != "prerequisite" {
		t.Fatal(pair, e)
	}
	a := model.Assignment{ID: "assignment", ProjectPath: p.Path, Plan: "alpha", Step: 2, SourceDigest: pair.Digest}
	r := model.Result{Evidence: []string{"real fixture verified"}}
	if e = src.Checkpoint(ctx, a, r); e != nil {
		t.Fatal(e)
	}
	if e = src.Checkpoint(ctx, a, r); e != nil {
		t.Fatal("idempotent checkpoint", e)
	}
	pair, e = src.Pair(ctx, p, "alpha")
	if e != nil || pair.Step != 1 {
		t.Fatal(pair, e)
	}
	b, _ := os.ReadFile(path)
	if strings.Count(string(b), "conductor checkpoint assignment") != 1 {
		t.Fatal("duplicate checkpoint")
	}
	a.Step = 1
	a.SourceDigest = pair.Digest
	edited := strings.Replace(string(b), "dependent work", "changed work", 1)
	_ = conductor.AtomicWrite(path, []byte(edited))
	if e = src.Checkpoint(ctx, a, r); e == nil {
		t.Fatal("changed source accepted")
	}
}

// TESTMASTER: id=indigobird-execution-ownership tier=fast parallel=yes
func TestConductorSourceOwnTrail(t *testing.T) {
	ctx := context.Background()
	p := model.Project{ID: "p", Path: t.TempDir()}
	writeConductorPlan(t, p.Path, sourcePlan)
	src := ConductorPlanSource{}
	a := model.Assignment{ID: "owned", ProjectPath: p.Path, Plan: "alpha"}
	if e := src.Claim(ctx, a); e != nil {
		t.Fatal(e)
	}
	TouchHeartbeat(p.Path)
	t.Cleanup(func() { _ = os.Remove(HeartbeatPath(p.Path)) })
	if e := src.Release(ctx, a); e != nil {
		t.Fatal(e)
	}
	if e := src.Gate(ctx, p, time.Now()); e != nil {
		t.Fatal("own hook trail blocked", e)
	}
	info, _ := os.Stat(HeartbeatPath(p.Path))
	at := info.ModTime().Add(time.Second)
	if e := os.Chtimes(HeartbeatPath(p.Path), at, at); e != nil {
		t.Fatal(e)
	}
	if e := src.Gate(ctx, p, at); e == nil {
		t.Fatal("new manual heartbeat ignored")
	}
}

// TESTMASTER: id=indigobird-plan-navigation-identity tier=fast parallel=yes
func TestConductorSourceIdentityAndArchive(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	a := model.Project{ID: "a", Path: filepath.Join(root, "one", "same")}
	b := model.Project{ID: "b", Path: filepath.Join(root, "two", "same")}
	writeConductorPlan(t, a.Path, sourcePlan)
	writeConductorPlan(t, b.Path, sourcePlan)
	src := ConductorPlanSource{}
	pa, _ := src.List(ctx, a)
	pb, _ := src.List(ctx, b)
	if len(pa) != 1 || len(pb) != 1 || pa[0].ID == pb[0].ID {
		t.Fatal(pa, pb)
	}
	if _, e := src.Pair(ctx, a, "../escape"); e == nil {
		t.Fatal("unsafe plan")
	}
	for i := 0; i < 2; i++ {
		pair, e := src.Pair(ctx, a, "alpha")
		if e != nil {
			t.Fatal(e)
		}
		assignment := model.Assignment{ID: conductor.NewID("a"), ProjectPath: a.Path, Plan: "alpha", Step: pair.Step, SourceDigest: pair.Digest}
		if e = src.Checkpoint(ctx, assignment, model.Result{Evidence: []string{"proof"}}); e != nil {
			t.Fatal(e)
		}
	}
	pair, e := src.Pair(ctx, a, "alpha")
	if e != nil || pair.Step != 0 {
		t.Fatal(pair, e)
	}
	assignment := model.Assignment{ID: "delivery", ProjectPath: a.Path, Plan: "alpha", Kind: "delivery", SourceDigest: pair.Digest}
	if e = src.Outcome(ctx, assignment, model.Result{Outcome: "completed", Evidence: []string{"fixture branch delivered"}}); e != nil {
		t.Fatal(e)
	}
	if e = src.Outcome(ctx, assignment, model.Result{Outcome: "completed", Evidence: []string{"fixture branch delivered"}}); e != nil {
		t.Fatal("delivery replay not idempotent", e)
	}
	arch, e := ListArchivedPlans(a.Path)
	if e != nil || len(arch) != 1 {
		t.Fatal(arch, e)
	}
	other, _ := src.List(ctx, b)
	if len(other) != 1 || other[0].Complete {
		t.Fatal("same-name cross project mutation")
	}
}

// TESTMASTER: id=indigobird-execution-intake-provenance tier=fast parallel=yes
func TestConductorSourceIntakeProvenance(t *testing.T) {
	p := t.TempDir()
	text := strings.Replace(sourcePlan, "status: queued", "status: planned", 1) + "\nIssue source: github:fixture/repo:1\n"
	path := writeConductorPlan(t, p, text)
	src := ConductorPlanSource{}
	a := model.Assignment{ID: "intake", ProjectPath: p, Plan: "alpha", Kind: "intake", SourceDigest: "github:fixture/repo:1"}
	r := model.Result{Outcome: "completed", Evidence: []string{"paired issue plan inspected"}}
	if e := src.Outcome(context.Background(), a, r); e != nil {
		t.Fatal(e)
	}
	text = strings.Replace(text, "status: planned", "status: queued", 1)
	_ = conductor.AtomicWrite(path, []byte(text))
	if e := src.Outcome(context.Background(), a, r); e == nil {
		t.Fatal("intake escalated unapproved issue into staged work")
	}
}

// TESTMASTER: id=indigobird-project-rules-checkpoint tier=fast parallel=yes
func TestConductorSourceReleaseNextClaim(t *testing.T) {
	ctx := context.Background()
	p := model.Project{Path: t.TempDir()}
	path := writeConductorPlan(t, p.Path, sourcePlan)
	src := ConductorPlanSource{}
	for _, id := range []string{"first", "second", "third"} {
		a := model.Assignment{ID: id, ProjectPath: p.Path, Plan: "alpha"}
		if e := src.Claim(ctx, a); e != nil {
			t.Fatal(id, e)
		}
		if e := src.Release(ctx, a); e != nil {
			t.Fatal(id, e)
		}
	}
	a := model.Assignment{ID: "owner", ProjectPath: p.Path, Plan: "alpha"}
	if e := src.Claim(ctx, a); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(path)
	a.ID = "intruder"
	if e := src.Claim(ctx, a); e == nil {
		t.Fatal("foreign owner accepted")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("failed claim changed source")
	}
	a.ID = "owner"
	_ = src.Release(ctx, a)
	pair, e := src.Pair(ctx, p, "alpha")
	if e != nil {
		t.Fatal(e)
	}
	a.Step = pair.Step
	a.SourceDigest = pair.Digest
	if e = src.Checkpoint(ctx, a, model.Result{Evidence: []string{"verified after repeated claims"}}); e != nil {
		t.Fatal(e)
	}
}

// TESTMASTER: id=indigobird-projects-queue tier=fast parallel=yes
func TestConductorSourceTypedPlanExclusion(t *testing.T) {
	project := t.TempDir()
	for _, mode := range []string{"rc", "release", "normal"} {
		body := strings.Replace(sourcePlan, "name: alpha", "name: "+mode, 1)
		if mode != "normal" {
			body = "mode: " + mode + "\n" + body
		}
		if e := conductor.AtomicWrite(filepath.Join(project, ".claude", "iterate", "plans", mode+".md"), []byte(body)); e != nil {
			t.Fatal(e)
		}
	}
	src := ConductorPlanSource{}
	plans, e := src.List(context.Background(), model.Project{ID: "project", Path: project})
	if e != nil || len(plans) != 3 {
		t.Fatal(plans, e)
	}
	for _, plan := range plans {
		if plan.Release != (plan.Name != "normal") {
			t.Fatal("typed-only plan classification", plan)
		}
	}
	s := NewConductorService(t.TempDir())
	c, e := s.Config()
	if e != nil {
		t.Fatal(e)
	}
	c.Projects = []model.Project{{ID: "project", Name: "Typed fixture", Path: project}}
	if _, e = s.Apply(c.Revision, c); e != nil {
		t.Fatal(e)
	}
	d, e := s.Evaluate(context.Background(), conductor.EvaluateRequest{})
	if e != nil || len(d.Queues) != 1 || d.Queues[0].Depth != 1 || d.Queues[0].Plans[0].Name != "normal" {
		t.Fatal("typed plan entered automated queue", d, e)
	}
}
