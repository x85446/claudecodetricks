//go:build !windows

package testmaster

import (
	"os"
	"syscall"
)

// lockFile takes an exclusive, non-blocking lock on f. The kernel releases it
// when the file closes or the process exits, however it exits.
func lockFile(f *os.File) error {
	return syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
}
