//go:build !windows

package testmaster

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestShellTimeoutKillsTheWholeProcessGroup(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "survived")
	p := project(t, map[string]*Entry{
		"hang": {Kind: "shell", Cmd: "(sleep 2; touch " + marker + ") & sleep 60"},
	})
	rep := runAll(t, p, RunOptions{Timeout: 300 * time.Millisecond})
	if rep.Counts[Timeout] != 1 {
		t.Fatalf("counts = %v", rep.Counts)
	}
	time.Sleep(2500 * time.Millisecond)
	if _, err := os.Stat(marker); err == nil {
		t.Error("a grandchild outlived the timeout")
	}
	if p.Reg.Tests["hang"].Runs != 0 {
		t.Error("a timeout was recorded as a measured duration")
	}
}

func TestDefaultShellIsBashOnUnix(t *testing.T) {
	if got := DefaultShell(); len(got) != 2 || got[0] != "bash" || got[1] != "-c" {
		t.Errorf("DefaultShell() = %q, want [bash -c]", got)
	}
}
