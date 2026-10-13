package testmaster

import (
	"bufio"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// holdLock starts a child process that takes dir's project lock and waits
// until it reports holding it.
func holdLock(t *testing.T, dir string) *exec.Cmd {
	t.Helper()
	argv := helperArgv("lock", dir)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Env = append(os.Environ(), helperEnv()...)
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	line := make(chan string, 1)
	go func() {
		s := bufio.NewScanner(out)
		s.Scan()
		line <- s.Text()
	}()
	select {
	case l := <-line:
		if l != "locked" {
			t.Fatalf("helper could not take the lock: %q", l)
		}
	case <-time.After(30 * time.Second):
		t.Fatal("helper never reported the lock")
	}
	return cmd
}

func TestLockIsExclusiveAcrossProcessesAndDiesWithItsHolder(t *testing.T) {
	p := project(t, nil)
	holder := holdLock(t, p.Root)

	if release, err := p.Lock(); err == nil {
		release()
		t.Fatal("a second process took a lock another process holds")
	} else if !strings.Contains(err.Error(), filepath.Join(".claude", "testmaster", "failures", ".lock")) {
		t.Errorf("refusal does not name the lock file: %v", err)
	}

	// A hard kill (SIGKILL, TerminateProcess) runs no cleanup; the kernel
	// still releases the lock.
	if err := holder.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	_ = holder.Wait()
	release, err := p.Lock()
	if err != nil {
		t.Fatalf("lock still held after its holder was killed: %v", err)
	}
	release()
}

func TestLockReleaseAndStaleFile(t *testing.T) {
	p := project(t, nil)
	release, err := p.Lock()
	if err != nil {
		t.Fatal(err)
	}
	release()
	if _, err := os.Stat(filepath.Join(p.FailureDir(), ".lock")); err != nil {
		t.Fatalf("lock file missing after release: %v", err)
	}
	// The file left behind is not a lock.
	release, err = p.Lock()
	if err != nil {
		t.Fatalf("a leftover lock file blocked a new run: %v", err)
	}
	release()
}

func TestLockIsPerProject(t *testing.T) {
	a, b := project(t, nil), project(t, nil)
	ra, err := a.Lock()
	if err != nil {
		t.Fatal(err)
	}
	defer ra()
	rb, err := b.Lock()
	if err != nil {
		t.Fatalf("one project's lock blocked another project: %v", err)
	}
	rb()
}
