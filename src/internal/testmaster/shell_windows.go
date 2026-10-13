//go:build windows

package testmaster

import (
	"os"
	"sync"
)

var defaultShell = sync.OnceValue(func() []string {
	return windowsShell(os.Getenv("PATH"), os.Getenv("SystemRoot"), os.Getenv("LOCALAPPDATA"), func(p string) bool {
		fi, err := os.Stat(p)
		return err == nil && !fi.IsDir()
	})
})

// DefaultShell is the command a shell-kind test's --cmd is appended to when
// the project sets no shell runner: Git Bash, else PowerShell, else cmd.
func DefaultShell() []string { return append([]string(nil), defaultShell()...) }
