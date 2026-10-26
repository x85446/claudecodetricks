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
