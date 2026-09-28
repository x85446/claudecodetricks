package iterrun

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"sort"
)

// ProjectsPath is the index of every project directory iterate-run has
// ever seen with an actual plans/ directory — how the dashboard and purge
// commands know what exists without a filesystem-wide scan.
func ProjectsPath() string {
	return filepath.Join(StoreDir(), "projects.json")
}

// RegisterProject records dir as a known iterate project. Best-effort and
// silent on any error — this is a convenience index, not a source of
// truth (the source of truth is always the filesystem itself, which is
// why ListProjects re-checks every entry rather than trusting the file).
// Only directories with an actual plans/ folder qualify — a directory
// that merely has iterate-run registry entries (any team's working
// directory ends up with one) is not a plan's home and would just show up
// as an empty, confusing entry on the dashboard.
func RegisterProject(dir string) {
	if dir == "" {
		return
	}
	if _, err := os.Stat(filepath.Join(dir, ".claude", "iterate", "plans")); err != nil {
		return
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return
	}
	known, _ := ListProjects()
	if slices.Contains(known, abs) {
		return
	}
	known = append(known, abs)
	sort.Strings(known)
	data, err := json.MarshalIndent(known, "", "  ")
	if err != nil {
		return
	}
	path := ProjectsPath()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	tmp := path + ".tmp"
	if os.WriteFile(tmp, data, 0o644) == nil {
		_ = os.Rename(tmp, path)
	}
}

// ListProjects returns every registered project that still has a plans/
// directory — self-healing: a project that got deleted or moved just
// quietly drops off the list instead of needing a separate cleanup step.
func ListProjects() ([]string, error) {
	data, err := os.ReadFile(ProjectsPath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var known []string
	if json.Unmarshal(data, &known) != nil {
		return nil, nil
	}
	var live []string
	for _, p := range known {
		if _, err := os.Stat(filepath.Join(p, ".claude", "iterate", "plans")); err == nil {
			live = append(live, p)
		}
	}
	return live, nil
}

// ProjectRoot resolves the directory that owns dir's iterate state.
//
// Plans live at <project>/.claude/iterate/plans, but every caller here is
// handed a session's cwd, and a session's cwd wanders: one `cd src` in an
// unrelated turn, or a tool call under .claude/iterate/archive, and the
// literal cwd no longer has a .claude/iterate at all. Everything keyed on
// that raw path then silently detaches from the project — the heartbeat
// lands under a bogus key so the status line never sees liveness, and
// CurrentPlanName finds no pointer so the coordinator's events lose their
// plan. Walking up to the owning directory fixes both at the source.
//
// The walk stops at the repository root (.git, a file in a worktree and a
// directory otherwise): plans sit beside .git, never above it, so a stray
// ~/.claude/iterate/plans can never be mistaken for an unrelated project's.
// A dir with no plans anywhere above it resolves to itself, which is what
// every caller already did before.
func ProjectRoot(dir string) string {
	if dir == "" {
		return dir
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return dir
	}
	for d := abs; ; {
		if st, err := os.Stat(filepath.Join(d, ".claude", "iterate", "plans")); err == nil && st.IsDir() {
			return d
		}
		if _, err := os.Stat(filepath.Join(d, ".git")); err == nil {
			return abs
		}
		parent := filepath.Dir(d)
		if parent == d {
			return abs
		}
		d = parent
	}
}
