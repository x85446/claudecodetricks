package chainguard

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestSplitTopLevel(t *testing.T) {
	cases := []struct {
		name    string
		command string
		want    []string
	}{
		{"bare", "gh pr merge 23 --merge", []string{"gh pr merge 23 --merge"}},
		{"and", "a && b", []string{"a", "b"}},
		{"semicolon", "a; b", []string{"a", "b"}},
		{"pipe", "a | b", []string{"a", "b"}},
		{"newline", "a\nb", []string{"a", "b"}},
		// 2>&1 is a redirection, not a command boundary. Treating it as
		// one would split every command that captures stderr.
		{"redirect is not a separator", "gh pr merge 23 2>&1", []string{"gh pr merge 23 2>&1"}},
		// A separator inside quotes belongs to the argument.
		{"quoted operator", `echo "a && b"`, []string{`echo "a && b"`}},
		{"single-quoted operator", `echo 'a | b'`, []string{`echo 'a | b'`}},
		{"escaped operator", `echo a \&\& b`, []string{`echo a \&\& b`}},
		{"the real regression", "gh pr merge 23 --merge --delete-branch 2>&1 | tail -3 && git checkout main",
			[]string{"gh pr merge 23 --merge --delete-branch 2>&1", "tail -3", "git checkout main"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SplitTopLevel(tc.command); !slices.Equal(got, tc.want) {
				t.Errorf("SplitTopLevel(%q)\n got %q\nwant %q", tc.command, got, tc.want)
			}
		})
	}
}

func TestOffender(t *testing.T) {
	granted := []string{"gh pr merge", "gh pr create", "cat", "mkdir"}
	cases := []struct {
		name     string
		command  string
		wantHit  string
		wantFlag bool
	}{
		// The case that started all this.
		{"chained merge", "gh pr merge 23 --merge | tail -3 && git checkout main", "gh pr merge", true},
		{"chained merge, second position", "git fetch && gh pr merge 23", "gh pr merge", true},
		// A bare command is always fine, however sensitive. The rule is
		// about chaining, not about the command.
		{"bare merge", "gh pr merge 23 --merge --delete-branch", "", false},
		{"bare merge with redirect", "gh pr merge 23 2>&1", "", false},
		// Benign allowlisted commands are deliberately NOT guarded:
		// the classifier approves them anyway, so blocking the chain
		// would cost round-trips and buy nothing.
		{"chained cat is fine", "cat a && cat b", "", false},
		{"chained mkdir is fine", "mkdir -p x && cd x", "", false},
		// Not allowlisted means there is no rule to lose.
		{"sensitive but ungranted", "git rebase -i && echo done", "", false},
		// Quoting must not create a false positive.
		{"merge named inside a string", `echo "gh pr merge is the command" && ls`, "", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, flagged := Offender(tc.command, granted)
			if flagged != tc.wantFlag || got != tc.wantHit {
				t.Errorf("Offender(%q) = (%q, %v), want (%q, %v)", tc.command, got, flagged, tc.wantHit, tc.wantFlag)
			}
		})
	}
}

func TestLoadAllowedBashPrefixes(t *testing.T) {
	home := t.TempDir()
	if err := os.MkdirAll(filepath.Join(home, ".claude"), 0o755); err != nil {
		t.Fatal(err)
	}
	write := func(name, body string) {
		if err := os.WriteFile(filepath.Join(home, ".claude", name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("settings.json", `{"permissions":{"allow":["Bash(gh pr merge:*)","Bash(gh pr create:*)","WebFetch(domain:github.com)"]}}`)
	write("settings.local.json", `{"permissions":{"allow":["Bash(cat:*)","mcp__voicemode__converse"]}}`)

	got := LoadAllowedBashPrefixes(home)
	for _, want := range []string{"gh pr merge", "gh pr create", "cat"} {
		if !slices.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
	// Non-Bash rules must not leak in as command prefixes.
	for _, unwanted := range []string{"WebFetch(domain:github.com)", "mcp__voicemode__converse"} {
		if slices.Contains(got, unwanted) {
			t.Errorf("non-Bash rule %q leaked into prefixes", unwanted)
		}
	}
}

// TestMissingSettingsIsNotAnError: with no settings to read there are no
// granted prefixes, so nothing is guarded and every command passes. A
// guard that fails closed here would block the user out of their own
// shell.
func TestMissingSettingsIsNotAnError(t *testing.T) {
	if got := LoadAllowedBashPrefixes(t.TempDir()); len(got) != 0 {
		t.Errorf("expected no prefixes from an empty home, got %q", got)
	}
	if _, flagged := Offender("gh pr merge 1 && ls", nil); flagged {
		t.Error("with no allow rules loaded, nothing should be guarded")
	}
}
