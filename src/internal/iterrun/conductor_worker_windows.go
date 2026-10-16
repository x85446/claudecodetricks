//go:build windows

package iterrun

import "os/exec"

func detachConductorWorker(cmd *exec.Cmd) {}
