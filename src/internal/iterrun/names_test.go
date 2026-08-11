package iterrun

import (
	"path/filepath"
	"testing"
)

func TestNextPlanNameWalksAlphabetInOrder(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan-names.json")
	lock := filepath.Join(dir, "plan-names.json.lock")
	noSeed := func() map[string]bool { return nil }

	first, err := nextPlanName(path, lock, noSeed)
	if err != nil {
		t.Fatal(err)
	}
	if first[0] != 'a' {
		t.Errorf("first name = %q, want it to start with 'a'", first)
	}

	second, err := nextPlanName(path, lock, noSeed)
	if err != nil {
		t.Fatal(err)
	}
	if second[0] != 'b' {
		t.Errorf("second name = %q, want it to start with 'b'", second)
	}
	if second == first {
		t.Errorf("second call returned the same name as the first: %q", second)
	}
}

func TestNextPlanNameNeverRepeats(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan-names.json")
	lock := filepath.Join(dir, "plan-names.json.lock")
	noSeed := func() map[string]bool { return nil }

	seen := map[string]bool{}
	for range 60 { // more than one full a-z lap
		name, err := nextPlanName(path, lock, noSeed)
		if err != nil {
			t.Fatalf("call %d: %v", len(seen)+1, err)
		}
		if seen[name] {
			t.Fatalf("name %q reused after %d calls", name, len(seen)+1)
		}
		seen[name] = true
	}
}

// TestNextPlanNameSeedsFromDisk reproduces the reported bug directly: a
// name ("wren") already used by an existing project must never be handed
// out again, even on this registry's very first call.
func TestNextPlanNameSeedsFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan-names.json")
	lock := filepath.Join(dir, "plan-names.json.lock")
	preUsed := func() map[string]bool {
		return map[string]bool{"wren": true, "wolf": true, "walrus": true, "weasel": true, "wombat": true}
	}

	// Drain every letter before 'w' so the cycle actually reaches 'w' and
	// has to prove it skips the whole (seeded-exhausted) pool.
	for range 22 { // a..v
		if _, err := nextPlanName(path, lock, preUsed); err != nil {
			t.Fatal(err)
		}
	}

	name, err := nextPlanName(path, lock, preUsed)
	if err != nil {
		t.Fatal(err)
	}
	if name[0] == 'w' {
		t.Errorf("got %q — every w-animal was pre-seeded as used, so this should have skipped to 'x'", name)
	}
}

func TestNextPlanNameConcurrentCallsNeverCollide(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan-names.json")
	lock := filepath.Join(dir, "plan-names.json.lock")
	noSeed := func() map[string]bool { return nil }

	const n = 20
	results := make(chan string, n)
	errs := make(chan error, n)
	for range n {
		go func() {
			name, err := nextPlanName(path, lock, noSeed)
			if err != nil {
				errs <- err
				return
			}
			results <- name
		}()
	}

	seen := map[string]bool{}
	for range n {
		select {
		case err := <-errs:
			t.Fatal(err)
		case name := <-results:
			if seen[name] {
				t.Fatalf("concurrent calls handed out %q twice", name)
			}
			seen[name] = true
		}
	}
}
