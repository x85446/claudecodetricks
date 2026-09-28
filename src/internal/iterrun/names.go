// Package iterrun: plan codename assignment. /iterate-planner and /iterate
// used to have the LLM "pick a random common animal not already present in
// plans/" — a per-project check only, so nothing stopped two different
// projects from independently picking the same word (confirmed live:
// "wren" got used by two unrelated projects, and see the dashboard bugs
// that same collision caused). NextPlanName replaces that with: each
// project walks the alphabet on its OWN sequence (that project's 1st new
// plan is an a-word, 2nd is a b-word, ...), while the actual word is drawn
// from the shared, machine-wide "already used" set — so two projects' first
// plans are both a-words, just never the SAME a-word.
//
// The letter is never skipped and the pool never runs dry: the words come
// from pools.go via PoolWord, which suffixes on wrap (quail ... quoll,
// quail2, quokka2, ...). Before that, a letter whose words were all spent
// machine-wide made the claim jump ahead to another letter — which broke
// the per-project sequence exactly when it mattered, at the thin letters
// (q had 14 words for the whole machine). Depth is now unbounded at every
// letter, so the only reason a project's Nth plan is not the Nth letter is
// that a name was claimed and thrown away; ReleasePlanName is the way to
// hand one back.
package iterrun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

var alphabet = func() []byte {
	letters := make([]byte, 0, 26)
	for c := byte('a'); c <= 'z'; c++ {
		letters = append(letters, c)
	}
	return letters
}()

// nameState is the persisted record: every codename handed out so far
// (shared, machine-wide — this is what "globally unique" means here), each
// project's own next-letter position in ITS alphabetical sequence (keyed
// by absolute project directory), and whether the one-time import of
// pre-existing on-disk plan names has run yet.
type nameState struct {
	Used           map[string]bool `json:"used"`
	ProjectNextIdx map[string]int  `json:"project_next_idx"`
	Seeded         bool            `json:"seeded"`
}

// NamesPath is the global registry file every NextPlanName call reads and
// writes — under StoreDir(), same as events.jsonl/labels.json, so it's one
// shared sequence machine-wide rather than per-project.
func NamesPath() string {
	return filepath.Join(StoreDir(), "plan-names.json")
}

func namesLockPath() string {
	return NamesPath() + ".lock"
}

// NextPlanName claims and returns the next codename in projectDir's OWN
// alphabetical sequence (a, b, c, ... z, a, b, ... — that project's 1st new
// plan is an a-word, 2nd a b-word, and so on), never reissuing a word
// already assigned to ANY project on the machine and never skipping the
// letter it owes. The very first call
// (from any project) seeds the "already used" set from every plan name
// already on disk across every known project, so names in use before this
// registry existed are never handed out again.
func NextPlanName(projectDir string) (string, error) {
	return nextPlanName(NamesPath(), namesLockPath(), projectDir, seedFromKnownProjects)
}

// seedFromKnownProjects collects every plan name already on disk across
// every project iterate-run knows about — best-effort, same tolerance for
// a missing/unreadable project as the rest of this package.
func seedFromKnownProjects() map[string]bool {
	used := map[string]bool{}
	projects, err := ListProjects()
	if err != nil {
		return used
	}
	for _, proj := range projects {
		plans, err := ListPlans(proj)
		if err != nil {
			continue
		}
		for _, p := range plans {
			if p.Name != "" {
				used[p.Name] = true
			}
		}
	}
	return used
}

// nextPlanName is NextPlanName's testable core — path, lockPath, and the
// seed function are injected so tests exercise the real algorithm
// (locking, seeding, per-project alphabetical cycling, persistence)
// against a throwaway directory instead of the machine's real global
// registry.
func nextPlanName(path, lockPath, projectDir string, seed func() map[string]bool) (string, error) {
	unlock, err := lockFile(lockPath)
	if err != nil {
		return "", err
	}
	defer unlock()

	st, err := loadNameState(path)
	if err != nil {
		return "", err
	}
	if !st.Seeded {
		for name := range seed() {
			st.Used[name] = true
		}
		st.Seeded = true
	}

	key := projectKey(projectDir)
	idx := st.ProjectNextIdx[key]
	letter := alphabet[idx]

	// The letter is never skipped. PoolWord suffixes past the end of the
	// real words, so this project's own a/b/c sequence always gets the
	// letter it is owed no matter what any other project has claimed --
	// which is the whole point of the per-project index.
	for n := 0; ; n++ {
		name := PoolWord(letter, n)
		if name == "" {
			return "", fmt.Errorf("iterate-run: no word pool for letter %q", letter)
		}
		if st.Used[name] {
			continue
		}
		st.Used[name] = true
		st.ProjectNextIdx[key] = (idx + 1) % len(alphabet)
		if err := saveNameState(path, st); err != nil {
			return "", err
		}
		return name, nil
	}
}

// projectKey normalizes projectDir to an absolute path so the same project
// always maps to the same entry in ProjectNextIdx regardless of which
// relative path or cwd a given call happened to use. Falls back to the raw
// string on a resolution error rather than failing the whole call.
func projectKey(projectDir string) string {
	abs, err := filepath.Abs(projectDir)
	if err != nil {
		return projectDir
	}
	return abs
}

func loadNameState(path string) (*nameState, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return &nameState{Used: map[string]bool{}, ProjectNextIdx: map[string]int{}}, nil
		}
		return nil, err
	}
	var st nameState
	if err := json.Unmarshal(data, &st); err != nil {
		return nil, err
	}
	if st.Used == nil {
		st.Used = map[string]bool{}
	}
	if st.ProjectNextIdx == nil {
		st.ProjectNextIdx = map[string]int{}
	}
	return &st, nil
}

func saveNameState(path string, st *nameState) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(st, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// lockFile takes an exclusive advisory lock on path (created if needed) so
// two concurrent `iterate-run name next` calls — from two different
// projects racing to create a plan at the same moment — can't both read
// the same state and hand out the same name. The returned func releases
// it; always call it via defer.
func lockFile(path string) (unlock func(), err error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return nil, err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		_ = f.Close()
		return nil, err
	}
	return func() {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		_ = f.Close()
	}, nil
}

// ReleasePlanName hands a codename back to the pool so it can be issued
// again -- the one operation here a person or an agent performs on
// purpose, for a plan that was abandoned before it meant anything. It
// deliberately does NOT rewind the project's letter index: the letter
// was spent, and quietly reusing it would put two plans on the same
// statusline slot. Returns whether the name was actually held.
func ReleasePlanName(word string) (bool, error) {
	return releasePlanName(NamesPath(), namesLockPath(), word)
}

func releasePlanName(path, lockPath, word string) (bool, error) {
	unlock, err := lockFile(lockPath)
	if err != nil {
		return false, err
	}
	defer unlock()

	st, err := loadNameState(path)
	if err != nil {
		return false, err
	}
	if !st.Used[word] {
		return false, nil
	}
	delete(st.Used, word)
	if err := saveNameState(path, st); err != nil {
		return false, err
	}
	return true, nil
}
