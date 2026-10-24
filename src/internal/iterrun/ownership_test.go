package iterrun

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

func ownerFixture(t *testing.T) string {
	t.Helper()
	p := t.TempDir()
	writeRecordPlan(t, p, "alpha", "phase: planned\ndepends: none\nrunning: false")
	return p
}

// TESTMASTER: id=nene-owner-1 tier=? parallel=no
func TestOwnerCLIIdleStart(t *testing.T) {
	p := ownerFixture(t)
	o, e := ClaimPlan(p, "alpha", "cli", os.Getpid(), time.Now())
	if e != nil || !o.Live || o.Owner != "cli" {
		t.Fatalf("CLI claim: %+v %v", o, e)
	}
}

// TESTMASTER: id=nene-owner-2 tier=? parallel=no
func TestOwnerConductorIdleStart(t *testing.T) {
	p := ownerFixture(t)
	a := model.Assignment{ID: "assignment", ProjectPath: p, Plan: "alpha"}
	if e := (ConductorPlanSource{}).Claim(context.Background(), a); e != nil {
		t.Fatal(e)
	}
	o, e := ProjectExecutionOwner(p, time.Now())
	if e != nil || o.Owner != "conductor" || !o.Live {
		t.Fatalf("conductor owner: %+v %v", o, e)
	}
	plans, e := (ConductorPlanSource{}).List(context.Background(), model.Project{ID: "fixture", Path: p})
	if e != nil || len(plans) != 1 || !plans[0].Running || plans[0].ExecutionOwner != "conductor" {
		t.Fatalf("status source lost plan owner: %+v %v", plans, e)
	}
}

// TESTMASTER: id=nene-owner-4 tier=? parallel=no
func TestOwnerCLIExcludesConductor(t *testing.T) {
	p := ownerFixture(t)
	if _, e := ClaimPlan(p, "alpha", "cli", os.Getpid(), time.Now()); e != nil {
		t.Fatal(e)
	}
	if e := (ConductorPlanSource{}).Claim(context.Background(), model.Assignment{ID: "other", ProjectPath: p, Plan: "alpha"}); e == nil || !strings.Contains(e.Error(), "cli") {
		t.Fatalf("expected CLI busy: %v", e)
	}
}

// TESTMASTER: id=nene-owner-3 tier=? parallel=no
func TestOwnerConductorExcludesCLI(t *testing.T) {
	p := ownerFixture(t)
	if _, e := ClaimPlan(p, "alpha", "conductor", os.Getpid(), time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e := ClaimPlan(p, "alpha", "cli", os.Getpid(), time.Now()); e == nil || !strings.Contains(e.Error(), "conductor") {
		t.Fatalf("expected conductor busy: %v", e)
	}
}

// TESTMASTER: id=nene-owner-5 tier=? parallel=no
func TestOwnerCompletedHandoffBothWays(t *testing.T) {
	for _, first := range []string{"cli", "conductor"} {
		p := ownerFixture(t)
		second := "cli"
		if first == "cli" {
			second = "conductor"
		}
		if _, e := ClaimPlan(p, "alpha", first, os.Getpid(), time.Now()); e != nil {
			t.Fatal(e)
		}
		if e := UpdatePlanOwnership(p, "alpha", "release", time.Now()); e != nil {
			t.Fatal(e)
		}
		if _, e := ClaimPlan(p, "alpha", second, os.Getpid(), time.Now()); e != nil {
			t.Fatalf("%s to %s: %v", first, second, e)
		}
	}
}

// TESTMASTER: id=nene-owner-6 tier=? parallel=no
func TestOwnerDeadLeaseReconcilesEvidence(t *testing.T) {
	p := ownerFixture(t)
	path := writeRecordPlan(t, p, "alpha", "phase: executing\ndepends: none\nowner: conductor\nowner-pid: 999999999\nowner-start: dead\nrunning: 2020-01-01T00:00:00Z\ncheckpoint: uncertain-delivery")
	if e := UpdatePlanOwnership(p, "alpha", "reconcile", time.Now()); e != nil {
		t.Fatal(e)
	}
	b, _ := os.ReadFile(path)
	if !strings.Contains(string(b), "checkpoint: uncertain-delivery") || ownerField(string(b), "running") != "false" {
		t.Fatalf("reconciled plan: %s", b)
	}
	if _, e := ClaimPlan(p, "alpha", "cli", os.Getpid(), time.Now()); e != nil {
		t.Fatal(e)
	}
}

// TESTMASTER: id=nene-owner-7 tier=? parallel=no
func TestOwnerLiveProcessKeepsStaleHeartbeat(t *testing.T) {
	p := ownerFixture(t)
	child := exec.Command("sleep", "30")
	if e := child.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = child.Process.Kill(); _ = child.Wait() }()
	path := writeRecordPlan(t, p, "alpha", "phase: executing\ndepends: none\nowner: conductor\nowner-pid: 999999999\nowner-start: dead\nrunning: 2020-01-01T00:00:00Z")
	if e := SetPlanOwnerChild(p, "alpha", child.Process.Pid, ownerProcessIdentity(child.Process.Pid)); e != nil {
		t.Fatal(e)
	}
	before, _ := os.ReadFile(path)
	if e := UpdatePlanOwnership(p, "alpha", "reconcile", time.Now()); e != nil {
		t.Fatal(e)
	}
	if _, e := ClaimPlan(p, "alpha", "cli", os.Getpid(), time.Now()); e == nil {
		t.Fatal("claimed while orphan child alive")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("live orphan marker modified")
	}
}

// TESTMASTER: id=nene-owner-8 tier=? parallel=no
func TestOwnerSimultaneousClaims(t *testing.T) {
	p := ownerFixture(t)
	var wg sync.WaitGroup
	results := make(chan error, 2)
	for _, kind := range []string{"cli", "conductor"} {
		wg.Add(1)
		go func(k string) {
			defer wg.Done()
			_, e := ClaimPlan(p, "alpha", k, os.Getpid(), time.Now())
			results <- e
		}(kind)
	}
	wg.Wait()
	close(results)
	wins := 0
	for e := range results {
		if e == nil {
			wins++
		}
	}
	if wins != 1 {
		t.Fatalf("successful claims=%d, want 1", wins)
	}
}

// TESTMASTER: id=nene-dependency-admission tier=? parallel=no
func TestOwnerDependencyDenialPreservesPlan(t *testing.T) {
	p := ownerFixture(t)
	path := writeRecordPlan(t, p, "beta", "phase: planned\ndepends: alpha\nrunning: false")
	before, _ := os.ReadFile(path)
	if _, e := ClaimPlan(p, "beta", "cli", os.Getpid(), time.Now()); e == nil || !strings.Contains(e.Error(), "alpha") {
		t.Fatalf("missing prerequisite: %v", e)
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("denied claim changed plan")
	}
	archive := filepath.Join(p, ".claude", "iterate", "archive")
	if e := os.MkdirAll(archive, 0755); e != nil {
		t.Fatal(e)
	}
	if e := os.Rename(filepath.Join(p, ".claude", "iterate", "plans", "alpha.md"), filepath.Join(archive, "20261024T000000Z-alpha-done.md")); e != nil {
		t.Fatal(e)
	}
	if _, e := ClaimPlan(p, "beta", "cli", os.Getpid(), time.Now()); e != nil {
		t.Fatal(e)
	}
}
