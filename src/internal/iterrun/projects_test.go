package iterrun

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

// mkProject builds a repo root with .git and, optionally, a plans directory.
func mkProject(t *testing.T, withPlans bool) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if withPlans {
		if err := os.MkdirAll(filepath.Join(root, ".claude", "iterate", "plans"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func TestProjectRootFindsOwnerFromSubdirectory(t *testing.T) {
	// The bug this exists for: a session that cd'd into a subdirectory kept
	// working on the same project, but every path keyed on its raw cwd
	// (heartbeat, .claude/iterate/current) pointed somewhere with no iterate
	// state at all — so the status line's plan segment vanished mid-session.
	root := mkProject(t, true)
	deep := filepath.Join(root, "src", "internal", "thing")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, filepath.Join(root, "src"), deep} {
		if got := ProjectRoot(dir); got != root {
			t.Errorf("ProjectRoot(%q) = %q, want %q", dir, got, root)
		}
	}
}

func TestProjectRootFindsOwnerFromInsideIterateItself(t *testing.T) {
	// Observed live: heartbeats keyed on .../<proj>/.claude/iterate/archive,
	// i.e. the walk has to work from inside the iterate tree too.
	root := mkProject(t, true)
	inside := filepath.Join(root, ".claude", "iterate", "archive")
	if err := os.MkdirAll(inside, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(inside); got != root {
		t.Errorf("ProjectRoot(%q) = %q, want %q", inside, got, root)
	}
}

func TestProjectRootStopsAtTheRepositoryRoot(t *testing.T) {
	// Plans sit beside .git, never above it. Without this bound the walk
	// would climb out of the repo and a stray ~/.claude/iterate/plans would
	// be adopted by every unrelated project on the machine.
	outer := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outer, ".claude", "iterate", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(outer, "repo")
	if err := os.MkdirAll(filepath.Join(inner, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	deep := filepath.Join(inner, "cmd")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(deep); got != deep {
		t.Errorf("ProjectRoot(%q) = %q, want it unchanged (walk must stop at the repo root)", deep, got)
	}
}

func TestProjectRootWorktreeGitFileBoundsTheWalk(t *testing.T) {
	// In a git worktree .git is a file, not a directory; the bound has to
	// hold there too or worktrees would climb out of the repo.
	outer := t.TempDir()
	if err := os.MkdirAll(filepath.Join(outer, ".claude", "iterate", "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	inner := filepath.Join(outer, "wt")
	if err := os.MkdirAll(inner, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(inner, ".git"), []byte("gitdir: /elsewhere\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(inner); got != inner {
		t.Errorf("ProjectRoot(%q) = %q, want it unchanged", inner, got)
	}
}

func TestProjectRootUnchangedWhenNothingOwnsIt(t *testing.T) {
	// A project with no plans yet must resolve to itself — that is what every
	// caller did before, and it is what keeps a fresh repo behaving normally.
	root := mkProject(t, false)
	if got := ProjectRoot(root); got != root {
		t.Errorf("ProjectRoot(%q) = %q, want %q", root, got, root)
	}
	if got := ProjectRoot(""); got != "" {
		t.Errorf("ProjectRoot(\"\") = %q, want \"\"", got)
	}
}

func TestProjectRootIgnoresAPlansFile(t *testing.T) {
	// A regular file named plans is not a plans directory.
	root := mkProject(t, false)
	if err := os.MkdirAll(filepath.Join(root, ".claude", "iterate"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".claude", "iterate", "plans"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := ProjectRoot(root); got != root {
		t.Errorf("ProjectRoot(%q) = %q, want %q", root, got, root)
	}
}

func TestHeartbeatKeyedOnProjectRootNotCWD(t *testing.T) {
	// End to end for the reported symptom: the status line stats the heartbeat
	// at the project root, so a hook firing from a subdirectory has to land on
	// that same key or liveness (⚡ / bold green) silently never shows.
	t.Setenv("HOME", t.TempDir())
	root := mkProject(t, true)
	deep := filepath.Join(root, "src")
	if err := os.MkdirAll(deep, 0o755); err != nil {
		t.Fatal(err)
	}

	TouchHeartbeat(ProjectRoot(deep))

	if _, ok := HeartbeatAge(root, time.Now()); !ok {
		t.Fatal("a hook fired from a subdirectory left no heartbeat at the project root")
	}
	if _, ok := HeartbeatAge(deep, time.Now()); ok {
		t.Error("the subdirectory got its own heartbeat key; that is the bug")
	}
}
