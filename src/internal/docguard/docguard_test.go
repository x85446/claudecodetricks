package docguard

import "testing"

func TestScanFlagsNarration(t *testing.T) {
	for _, line := range []string{
		"The status line is now read from the project root.",
		"This was previously freeform prose.",
		"`skillinstall.sh` is now a thin shim over skillctl.",
		"We renamed the flag to --fast.",
		"The legacy tab column was dropped on 2026-04-10.",
		"zeduf, formerly izmachine, builds the VMs.",
		"As of v2 the cache lives in ~/.cache.",
		"**Update:** the parser handles TOML 1.0.",
		"The old approach placed hours at random.",
		"It no longer reads the whole file.",
		"The runner now writes a log per failure.",
		"Plans used to be named by hand.",
	} {
		if h := Scan(line); len(h) != 1 {
			t.Errorf("missed narration: %q -> %v", line, h)
		}
	}
}

func TestScanIgnoresPresentTenseAndQuotes(t *testing.T) {
	for _, line := range []string{
		"The runner reads the key block above the first heading.",
		"Commit subjects are no longer than 50 characters.",
		`Never write narration such as "previously did Y" or "is now".`,
		"Avoid `no longer` and `is now` in docs.",
		"Docs use the present tense.",
	} {
		if h := Scan(line); len(h) != 0 {
			t.Errorf("false positive: %q -> %v", line, h)
		}
	}
}

func TestScanSkipsFencedCode(t *testing.T) {
	text := "Intro.\n```\n# previously this was a shell script\n```\nOutro."
	if h := Scan(text); len(h) != 0 {
		t.Errorf("flagged fenced code: %v", h)
	}
}

func TestScanReportsLineNumbers(t *testing.T) {
	h := Scan("one\ntwo\nthree is now four\n")
	if len(h) != 1 || h[0].Line != 3 || h[0].Phrase != "is now" {
		t.Fatalf("got %v", h)
	}
}

func TestApplies(t *testing.T) {
	cases := map[string]bool{
		"/repo/docs/architecture.md":                           true,
		"/repo/README":                                         true,
		"/repo/README.md":                                      true,
		"/repo/CHANGELOG.md":                                   false,
		"/repo/docs/release-notes.md":                          false,
		"/repo/docs/adr/0007-use-sqlite.md":                    false,
		"/repo/main.go":                                        false,
		"/repo/CLAUDE.md":                                      true,
		"/repo/skills/iterate/SKILL.md":                        true,
		"/home/u/.claude/CLAUDE.md":                            true,
		"/home/u/.claude/RTK.md":                               true,
		"/home/u/.claude/skills/mailbox/SKILL.md":              true,
		"/home/u/.claude/agents/backend-expert.md":             true,
		"/repo/.claude/iterate/plans/kestrel.md":               false,
		"/repo/.claude/iterate/archive/20261005-gaur-done.md":  false,
		"/repo/.claude/product/state.md":                       false,
		"/home/u/.claude/projects/-x/memory/feedback_tests.md": false,
		"/repo/.claude/iterate/inbox/20261009-alpha-deploy.md": false,
	}
	for path, want := range cases {
		if got := Applies(path); got != want {
			t.Errorf("Applies(%q) = %v, want %v", path, got, want)
		}
	}
}
