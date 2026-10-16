//go:build !windows

package iterrun

import (
	"os/exec"
	"syscall"
)

func detachConductorWorker(cmd *exec.Cmd) { cmd.SysProcAttr = &syscall.SysProcAttr{Setsid: true} }
