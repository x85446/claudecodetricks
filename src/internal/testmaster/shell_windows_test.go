//go:build windows

package testmaster

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// The default shell runs exit codes the same way whichever of Git Bash,
// PowerShell or cmd it resolved to: 0 passes, 77 skips, anything else fails.
func TestWindowsDefaultShellRunsTests(t *testing.T) {
	sh := DefaultShell()
	t.Logf("default shell: %q", sh)
	if _, err := os.Stat(`C:\Program Files\Git\bin\bash.exe`); err == nil {
		if !strings.HasSuffix(strings.ToLower(sh[0]), `bash.exe`) || strings.Contains(strings.ToLower(sh[0]), `\system32\`) {
			t.Errorf("Git Bash is installed but the default shell is %q", sh)
		}
	}
	p := project(t, map[string]*Entry{
		"ok": {Kind: "shell", Cmd: "exit 0"}, "skip": {Kind: "shell", Cmd: "exit 77"}, "bad": {Kind: "shell", Cmd: "exit 3"},
	})
	rep := runAll(t, p, RunOptions{})
	if rep.Counts[Pass] != 1 || rep.Counts[Skip] != 1 || rep.Counts[Fail] != 1 {
		t.Fatalf("counts = %v", rep.Counts)
	}
	if b, _ := os.ReadFile(p.LogPath("bad")); !strings.Contains(string(b), "exit:    3") {
		t.Errorf("failure log:\n%s", b)
	}
}

// cmd.exe gets its command verbatim: Go's argv quoting would mangle it.
func TestCmdRunnerKeepsTheCommandVerbatim(t *testing.T) {
	p := project(t, map[string]*Entry{"bad": {Kind: "shell", Cmd: `echo a "quoted" arg& exit 3`}})
	p.Reg.Runners = map[string][]string{"shell": {"cmd", "/c"}}
	rep := runAll(t, p, RunOptions{})
	if rep.Counts[Fail] != 1 {
		t.Fatalf("counts = %v", rep.Counts)
	}
	b, _ := os.ReadFile(p.LogPath("bad"))
	if !strings.Contains(string(b), `a "quoted" arg`) || !strings.Contains(string(b), "exit:    3") {
		t.Errorf("cmd /c log:\n%s", b)
	}
}

func TestPowerShellRunner(t *testing.T) {
	pwsh, err := exec.LookPath("pwsh")
	if err != nil {
		t.Skip("pwsh not on PATH")
	}
	p := project(t, map[string]*Entry{
		"skip": {Kind: "shell", Cmd: "exit 77"}, "bad": {Kind: "shell", Cmd: "Write-Output 'from pwsh'; exit 3"},
	})
	p.Reg.Runners = map[string][]string{"shell": {pwsh, "-NoProfile", "-NonInteractive", "-Command"}}
	rep := runAll(t, p, RunOptions{})
	if rep.Counts[Skip] != 1 || rep.Counts[Fail] != 1 {
		t.Fatalf("counts = %v", rep.Counts)
	}
	if b, _ := os.ReadFile(p.LogPath("bad")); !strings.Contains(string(b), "from pwsh") || !strings.Contains(string(b), "exit:    3") {
		t.Errorf("pwsh log:\n%s", b)
	}
}
