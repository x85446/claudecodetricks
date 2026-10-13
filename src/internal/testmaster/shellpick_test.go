package testmaster

import (
	"os"
	"reflect"
	"strings"
	"testing"
)

func TestWindowsShellResolution(t *testing.T) {
	const sys, local = `C:\Windows`, `C:\Users\u\AppData\Local`
	cases := []struct {
		name  string
		path  string
		files []string
		want  []string
	}{
		{"git bash on PATH", `C:\Windows\System32;C:\Program Files\Git\bin`,
			[]string{`C:\Windows\System32\bash.exe`, `C:\Program Files\Git\bin\bash.exe`},
			[]string{`C:\Program Files\Git\bin\bash.exe`, "-c"}},
		{"only the WSL launchers", `C:\Windows\System32;C:\Users\u\AppData\Local\Microsoft\WindowsApps;C:\Program Files\PowerShell\7`,
			[]string{`C:\Windows\System32\bash.exe`, `C:\Users\u\AppData\Local\Microsoft\WindowsApps\bash.exe`, `C:\Program Files\PowerShell\7\pwsh.exe`},
			[]string{`C:\Program Files\PowerShell\7\pwsh.exe`, "-NoProfile", "-NonInteractive", "-Command"}},
		{"git bash beside git.exe", `C:\Windows\System32;C:\Program Files\Git\cmd`,
			[]string{`C:\Program Files\Git\cmd\git.exe`, `C:\Program Files\Git\bin\bash.exe`},
			[]string{`C:\Program Files\Git\bin\bash.exe`, "-c"}},
		{"windows powershell", `C:\Windows\System32`,
			[]string{`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`},
			[]string{`C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe`, "-NoProfile", "-NonInteractive", "-Command"}},
		{"nothing but cmd", `C:\Windows\System32`, nil, []string{"cmd", "/C"}},
	}
	for _, c := range cases {
		files := map[string]bool{}
		for _, f := range c.files {
			files[strings.ToLower(f)] = true
		}
		got := windowsShell(c.path, sys, local, func(p string) bool { return files[strings.ToLower(p)] })
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %q, want %q", c.name, got, c.want)
		}
	}
}

func TestShellRunnerOverrideBeatsTheDefault(t *testing.T) {
	t.Setenv("TESTMASTER_HELPER", "1")
	p := project(t, map[string]*Entry{"t": {Kind: "shell", Cmd: "3"}})
	p.Reg.Runners = map[string][]string{"shell": helperArgv("exit")}
	rep := runAll(t, p, RunOptions{})
	if rep.Counts[Fail] != 1 || p.Reg.Tests["t"].LastResult != Fail {
		t.Fatalf("override runner not used: %v", rep.Counts)
	}
	if b, err := os.ReadFile(p.LogPath("t")); err != nil || !strings.Contains(string(b), "exit:    3") {
		t.Errorf("failure log does not carry exit 3: %v\n%s", err, b)
	}
}
