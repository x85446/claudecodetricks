//go:build !windows

package iterrun

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// ownerLockFile blocks until it holds an exclusive advisory lock on f. The kernel
// releases it when the file closes or the process exits.
func ownerLockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) }

func ownerUnlockFile(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }

func ownerProcessIdentity(pid int) string {
	if pid <= 0 {
		return ""
	}
	b, e := exec.Command("ps", "-p", strconv.Itoa(pid), "-o", "lstart=").Output()
	if e != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}

// callerDescendsFrom reports whether this process has pid among its
// ancestors: the Conductor's own calls descend from the Conductor.
func callerDescendsFrom(pid int) bool {
	if pid <= 0 {
		return false
	}
	cur := os.Getppid()
	for i := 0; i < 12 && cur > 1; i++ {
		if cur == pid {
			return true
		}
		b, e := exec.Command("ps", "-p", strconv.Itoa(cur), "-o", "ppid=").Output()
		if e != nil {
			return false
		}
		if cur, e = strconv.Atoi(strings.TrimSpace(string(b))); e != nil {
			return false
		}
	}
	return cur == pid
}
