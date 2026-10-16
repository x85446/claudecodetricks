//go:build windows

package conductor

import (
	"errors"
	"os"
	"os/exec"
)

func lockFile(f *os.File) error {
	return errors.New("conductor process-safe Windows locking is unavailable")
}
func unlockFile(f *os.File)                   {}
func processIdentity(pid int) string          { return "" }
func processAlive(pid int, start string) bool { return false }
func processGroup(c *exec.Cmd)                {}
func signalOwned(pid int, start string, force bool) error {
	return errors.New("owned process signalling unavailable")
}

func terminateRemainingGroup(group int) int { return 0 }
