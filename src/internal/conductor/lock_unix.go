//go:build !windows

package conductor

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

func lockFile(f *os.File) error { return syscall.Flock(int(f.Fd()), syscall.LOCK_EX) }
func unlockFile(f *os.File)     { _ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN) }
func processIdentity(pid int) string {
	if pid <= 0 {
		return ""
	}
	b, e := exec.Command("ps", "-p", fmtPID(pid), "-o", "lstart=").Output()
	if e != nil {
		return ""
	}
	return strings.TrimSpace(string(b))
}
func processAlive(pid int, start string) bool { return start != "" && processIdentity(pid) == start }
func processGroup(c *exec.Cmd)                { c.SysProcAttr = &syscall.SysProcAttr{Setpgid: true} }
func signalOwned(pid int, start string, force bool) error {
	if !processAlive(pid, start) {
		return nil
	}
	sig := syscall.SIGTERM
	if force {
		sig = syscall.SIGKILL
	}
	return syscall.Kill(-pid, sig)
}

// A live process group reserves its group ID after its leader exits.
func terminateRemainingGroup(group int) int {
	b, e := exec.Command("ps", "-ax", "-o", "pid=,pgid=").Output()
	if e != nil {
		return 0
	}
	count := 0
	for _, line := range strings.Split(string(b), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 2 {
			continue
		}
		pid, _ := strconv.Atoi(fields[0])
		pgid, _ := strconv.Atoi(fields[1])
		if pgid != group || pid == os.Getpid() {
			continue
		}
		identity := processIdentity(pid)
		if identity == "" {
			continue
		}
		if processAlive(pid, identity) {
			_ = syscall.Kill(pid, syscall.SIGKILL)
			count++
		}
	}
	return count
}
