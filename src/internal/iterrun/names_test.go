package iterrun

import (
	"path/filepath"
	"testing"
)

func TestNextPlanNameWalksAlphabetInOrderPerProject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan-names.json")
	lock := filepath.Join(dir, "plan-names.json.lock")
	noSeed := func() map[string]bool { return nil }
	proj := filepath.Join(dir, "proj1")

	first, err := nextPlanName(path, lock, proj, noSeed)
	if err != nil {
		t.Fatal(err)
	}
	if first[0] != 'a' {
		t.Errorf("first name = %q, want it to start with 'a'", first)
	}

	second, err := nextPlanName(path, lock, proj, noSeed)
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
	proj := filepath.Join(dir, "proj1")

	seen := map[string]bool{}
	for range 60 { // more than one full a-z lap
		name, err := nextPlanName(path, lock, proj, noSeed)
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
	proj := filepath.Join(dir, "proj1")
	preUsed := func() map[string]bool {
		return map[string]bool{"wren": true, "wolf": true, "walrus": true, "weasel": true, "wombat": true}
	}

	// Drain every letter before 'w' so this project's own sequence
	// actually reaches 'w' and has to prove it skips the whole
	// (seeded-exhausted) pool.
	for range 22 { // a..v
		if _, err := nextPlanName(path, lock, proj, preUsed); err != nil {
			t.Fatal(err)
		}
	}

	name, err := nextPlanName(path, lock, proj, preUsed)
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
	proj := filepath.Join(dir, "proj1")

	const n = 20
	results := make(chan string, n)
	errs := make(chan error, n)
	for range n {
		go func() {
			name, err := nextPlanName(path, lock, proj, noSeed)
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

// TestNextPlanNamePerProjectSequenceGloballyUniqueWords is the exact
// scenario reported live: each project should run its OWN alphabetical
// sequence (project 1's first plan is an a-word, project 2's first plan is
// ALSO an a-word, since it's project 2's own first plan too) — but the two
// must never land on the same word. Project 2 skips project 1's a-word for
// the next available a-word rather than jumping ahead to a b-word.
func TestNextPlanNamePerProjectSequenceGloballyUniqueWords(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan-names.json")
	lock := filepath.Join(dir, "plan-names.json.lock")
	noSeed := func() map[string]bool { return nil }
	proj1 := filepath.Join(dir, "proj1")
	proj2 := filepath.Join(dir, "proj2")

	p1First, err := nextPlanName(path, lock, proj1, noSeed)
	if err != nil {
		t.Fatal(err)
	}
	if p1First[0] != 'a' {
		t.Fatalf("project 1's first name = %q, want an a-word", p1First)
	}

	p2First, err := nextPlanName(path, lock, proj2, noSeed)
	if err != nil {
		t.Fatal(err)
	}
	if p2First[0] != 'a' {
		t.Errorf("project 2's first name = %q, want it to ALSO be an a-word (its own first plan) — not skip ahead to 'b' just because project 1 already used one", p2First)
	}
	if p2First == p1First {
		t.Errorf("project 2 got the same name as project 1: %q", p2First)
	}

	// Project 1's second plan continues ITS OWN sequence at 'b',
	// regardless of what project 2 has done in the meantime.
	p1Second, err := nextPlanName(path, lock, proj1, noSeed)
	if err != nil {
		t.Fatal(err)
	}
	if p1Second[0] != 'b' {
		t.Errorf("project 1's second name = %q, want a b-word (its own 2nd plan)", p1Second)
	}
}
