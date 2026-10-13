package testmaster

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// These run on every OS: a process group on Unix and a Job Object on Windows
// must behave the same.

func gone(t *testing.T, marker string) {
	t.Helper()
	time.Sleep(3 * time.Second)
	if _, err := os.Stat(marker); err == nil {
		t.Error("a grandchild outlived the kill")
	}
}

func TestTimeoutKillsTheWholeTree(t *testing.T) {
	t.Parallel()
	marker := filepath.Join(t.TempDir(), "survived")
	r := runProc(context.Background(), ".", helperEnv(), 500*time.Millisecond, true, helperArgv("tree", marker)...)
	if !r.TimedOut || r.Exit != killedExit {
		t.Fatalf("want a timeout with exit %d, got timedOut=%v exit=%d start=%v out=%s", killedExit, r.TimedOut, r.Exit, r.StartErr, r.Stdout)
	}
	gone(t, marker)
}

func TestInterruptKillsTheWholeTree(t *testing.T) {
	t.Parallel()
	marker := filepath.Join(t.TempDir(), "survived")
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(500*time.Millisecond, cancel)
	r := runProc(ctx, ".", helperEnv(), time.Minute, true, helperArgv("tree", marker)...)
	if !r.Interrupted || r.TimedOut {
		t.Fatalf("want interrupted, got interrupted=%v timedOut=%v exit=%d", r.Interrupted, r.TimedOut, r.Exit)
	}
	gone(t, marker)
}

func TestSelfExitKeepsItsCode(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		arg  string
		exit int
	}{{"0", 0}, {"3", 3}} {
		r := runProc(context.Background(), ".", helperEnv(), time.Minute, true, helperArgv("exit", c.arg)...)
		if r.Exit != c.exit || r.TimedOut || r.Interrupted || r.StartErr != nil {
			t.Errorf("exit %s: got exit=%d timedOut=%v interrupted=%v start=%v", c.arg, r.Exit, r.TimedOut, r.Interrupted, r.StartErr)
		}
	}
}

// A child a passing test leaves running is left alone, as on Unix: the tree is
// killed only on timeout or interrupt.
func TestPassingTestsDetachedChildSurvives(t *testing.T) {
	t.Parallel()
	marker := filepath.Join(t.TempDir(), "survived")
	r := runProc(context.Background(), ".", helperEnv(), time.Minute, true, helperArgv("detach", marker)...)
	if r.Failed() {
		t.Fatalf("detach failed: exit=%d start=%v out=%s", r.Exit, r.StartErr, r.Stdout)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(marker); err == nil {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Error("the detached child of a passing test was killed")
}
