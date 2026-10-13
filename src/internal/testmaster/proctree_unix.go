//go:build !windows

package testmaster

import (
	"os/exec"
	"syscall"
)

// tree holds a child's whole process tree. On Unix the child leads its own
// process group, and the kernel keeps every descendant in it.
type tree struct{}

func prepareTree(cmd *exec.Cmd) *tree {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error { return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }
	return &tree{}
}

// started runs after the child starts and before anything waits on it.
func (*tree) started() error { return nil }

func (*tree) close() {}
