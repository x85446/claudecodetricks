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
