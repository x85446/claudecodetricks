//go:build !windows

package runrecord

import (
	"os"
	"syscall"
)

// lockFile blocks until it holds an exclusive advisory lock on f. The kernel
// releases it when the file closes or the process exits.
func lockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) }

func unlockFile(f *os.File) { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
