//go:build !windows

package testmaster

// DefaultShell is the command a shell-kind test's --cmd is appended to when
// the project sets no shell runner.
func DefaultShell() []string { return []string{"bash", "-c"} }
