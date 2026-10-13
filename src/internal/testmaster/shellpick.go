package testmaster

import "strings"

// windowsShell picks the command that runs shell-kind tests on Windows. Git
// Bash comes first, so one --cmd string runs on every OS; then PowerShell 7,
// Windows PowerShell, and cmd, which every Windows box has. The bash.exe in
// System32 and in WindowsApps launches WSL, a Linux VM with its own view of
// the filesystem, so it never qualifies. Git's own installer puts only
// <git>\cmd on PATH, so Git Bash is also found beside git.exe.
//
// It is pure string work over PATH so every OS can test it; exists reports
// whether a file is present.
func windowsShell(pathEnv, systemRoot, localAppData string, exists func(string) bool) []string {
	var dirs []string
	for _, d := range strings.Split(pathEnv, ";") {
		if d = strings.TrimRight(strings.Trim(d, `"`), `\/`); d != "" {
			dirs = append(dirs, d)
		}
	}
	wsl := map[string]bool{
		winKey(systemRoot + `\System32`):                true,
		winKey(localAppData + `\Microsoft\WindowsApps`): true,
		winKey(systemRoot + `\Sysnative`):               true,
	}
	for _, d := range dirs {
		if p := d + `\bash.exe`; !wsl[winKey(d)] && exists(p) {
			return []string{p, "-c"}
		}
	}
	for _, d := range dirs {
		if !exists(d + `\git.exe`) {
			continue
		}
		for up := d; strings.ContainsAny(up, `\/`); {
			up = up[:strings.LastIndexAny(up, `\/`)]
			if p := up + `\bin\bash.exe`; exists(p) {
				return []string{p, "-c"}
			}
		}
	}
	for _, exe := range []string{"pwsh.exe", "powershell.exe"} {
		for _, d := range append(dirs, systemRoot+`\System32\WindowsPowerShell\v1.0`) {
			if p := d + `\` + exe; exists(p) {
				return []string{p, "-NoProfile", "-NonInteractive", "-Command"}
			}
		}
	}
	return []string{"cmd", "/C"}
}

func winKey(path string) string { return strings.ToLower(strings.ReplaceAll(path, "/", `\`)) }
