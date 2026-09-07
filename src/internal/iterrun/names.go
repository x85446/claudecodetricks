// Package iterrun: plan codename assignment. /iterate-planner and /iterate
// used to have the LLM "pick a random common animal not already present in
// plans/" — a per-project check only, so nothing stopped two different
// projects from independently picking the same word (confirmed live:
// "wren" got used by two unrelated projects, and see the dashboard bugs
// that same collision caused). NextPlanName replaces that with: each
// project walks the alphabet on its OWN sequence (that project's 1st new
// plan is an a-word, 2nd is a b-word, ...), while the actual word chosen
// for a letter is drawn from one machine-wide "already used" set — so two
// projects' first plans are both a-words, just never the SAME a-word
// (project 1 gets "aardvark", project 2's first plan skips it for the next
// available a-word, "antelope" — it does not jump ahead to a b-word).
package iterrun

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// animalsByLetter is the pool NextPlanName draws from, grouped by first
// letter so assignment can walk the alphabet in order. Every letter has at
// least one entry; letters with few common one-word animal names (q, u, x,
// y) just have a smaller pool, which only matters once that letter comes
// up for the Nth time in the a-through-z cycle.
var animalsByLetter = map[byte][]string{
	'a': {"aardvark", "antelope", "ant", "ape", "auk", "alpaca"},
	'b': {"badger", "bear", "bison", "boar", "bobcat", "bee"},
	'c': {"cat", "crow", "cobra", "coyote", "crane", "civet"},
	'd': {"dog", "deer", "dove", "dolphin", "duck", "donkey"},
	'e': {"elk", "eagle", "egret", "eel", "emu"},
	'f': {"fox", "finch", "ferret", "falcon", "frog"},
	'g': {"goat", "goose", "gecko", "gopher", "gull"},
	'h': {"hare", "heron", "hawk", "hedgehog", "husky"},
	'i': {"ibis", "iguana", "impala"},
	'j': {"jay", "jackal", "jaguar"},
	'k': {"koala", "kiwi", "kestrel", "kudu"},
	'l': {"lynx", "lark", "llama", "lemur", "loon"},
	'm': {"mole", "moose", "mink", "magpie", "marten"},
	'n': {"newt", "narwhal", "nightjar", "nutria"},
	'o': {"otter", "owl", "oryx", "ocelot", "osprey"},
	'p': {"panda", "puma", "pigeon", "pelican", "python"},
	'q': {"quail", "quokka"},
	'r': {"raven", "rabbit", "robin", "raccoon"},
	's': {"seal", "swan", "stoat", "sparrow", "skunk"},
	't': {"toad", "tiger", "tern", "tapir", "tortoise"},
	'u': {"urchin"},
	'v': {"vole", "viper", "vulture", "vixen"},
	'w': {"wren", "wolf", "walrus", "weasel", "wombat"},
	'x': {"xerus"},
	'y': {"yak", "yellowjacket"},
	'z': {"zebra", "zorilla", "zebu"},
}

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
// already assigned to ANY project on the machine. The very first call
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
	startIdx := st.ProjectNextIdx[key]

	for i := range len(alphabet) {
		idx := (startIdx + i) % len(alphabet)
		for _, name := range animalsByLetter[alphabet[idx]] {
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
	return "", fmt.Errorf("iterate-run: every animal codename in the pool is already in use — extend the pool in names.go")
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
