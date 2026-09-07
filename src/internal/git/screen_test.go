package git

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TESTMASTER: id=git-screen-path tier=fast parallel=yes
func TestScreenPath(t *testing.T) {
	dir := t.TempDir()

	write := func(rel string, data []byte) {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0644); err != nil {
			t.Fatal(err)
		}
	}

	write("main.go", []byte("package main\n"))
	write("README.md", []byte("# hi\n"))
	write("server", []byte("\x7fELF\x02\x01\x01\x00some compiled thing"))
	write("tool", []byte("\xcf\xfa\xed\xfemach-o binary"))
	write("lib.o", []byte("whatever"))
	write("node_modules/pkg/index.js", []byte("module.exports = 1\n"))
	write("big.txt", make([]byte, MaxAutoCommitBytes+1))
	write("script.sh", []byte("#!/bin/sh\necho hi\n"))

	cases := []struct {
		path     string
		rejected bool
		contains string
	}{
		{"main.go", false, ""},
		{"README.md", false, ""},
		{"script.sh", false, ""},
		{"server", true, "executable"},
		{"tool", true, "executable"},
		{"lib.o", true, "binary file type"},
		{"node_modules/pkg/index.js", true, "artifact dir"},
		{"big.txt", true, "oversized"},
		{"deleted-file.go", false, ""}, // missing: git handles deletions
	}

	for _, c := range cases {
		got := ScreenPath(dir, c.path)
		if c.rejected && got == "" {
			t.Errorf("ScreenPath(%q) = accepted, want rejected", c.path)
		}
		if !c.rejected && got != "" {
			t.Errorf("ScreenPath(%q) = rejected (%s), want accepted", c.path, got)
		}
		if c.contains != "" && !strings.Contains(got, c.contains) {
			t.Errorf("ScreenPath(%q) = %q, want reason containing %q", c.path, got, c.contains)
		}
	}
}

// TESTMASTER: id=git-screen-no-blob tier=fast parallel=no
// The whole point of screening before `git add`: a rejected binary must never
// reach .git/objects, because git hashes a file the moment it is added.
func TestStageFilesKeepsBinaryOutOfObjectStore(t *testing.T) {
	dir := t.TempDir()
	run := func(args ...string) {
		cmd := exec.Command("git", args...)
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %s: %v (%s)", strings.Join(args, " "), err, out)
		}
	}
	run("init", "-q", ".")
	run("config", "user.email", "test@example.com")
	run("config", "user.name", "test")

	if err := os.WriteFile(filepath.Join(dir, "main.go"), []byte("package main\n"), 0644); err != nil {
		t.Fatal(err)
	}
	blob := append([]byte("\x7fELF\x02\x01\x01\x00"), make([]byte, 200000)...)
	if err := os.WriteFile(filepath.Join(dir, "server"), blob, 0644); err != nil {
		t.Fatal(err)
	}

	rejected, err := StageFiles(dir, []string{"main.go", "server"})
	if err != nil {
		t.Fatalf("StageFiles: %v", err)
	}
	if len(rejected) != 1 || rejected[0].Path != "server" {
		t.Fatalf("rejected = %v, want exactly [server]", rejected)
	}

	// No object in the store may be anywhere near the binary's size.
	cmd := exec.Command("git", "cat-file", "--batch-all-objects", "--batch-check=%(objectsize)")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	for _, line := range strings.Fields(string(out)) {
		if len(line) >= 5 { // any object ≥10000 bytes means the blob got in
			t.Fatalf("oversized object %s bytes in store — binary reached .git/objects", line)
		}
	}

	// And the binary must now be ignored so it stops being re-offered.
	if !IsInGitIgnore(dir, "server") {
		t.Error("rejected binary was not added to .gitignore")
	}
}
