package iterrun

import (
	"path/filepath"
	"testing"
)

// animalPoolFloor is the minimum number of names any single letter's pool
// may hold. It caps how many plans ANY project on this machine can name
// before that letter starts forcing an advance-and-log skip (see
// nextPlanName) — so the floor, not the average, is what matters.
// 12 is the user's own "twelve times as big" applied per letter: the
// scarcest letters (u, x) each held exactly 1, so 12 is their 12x figure,
// and it lifts the cap those letters put on plans-per-project twelvefold.
// It is deliberately NOT higher. There are only about 10-12 real animals
// beginning with x (xerus, xenops, xantus, xoloitzcuintli, xiphias,
// xenopus, xema, xantusia, xylocopa, xiphosura); a floor of 40 would be
// unsatisfiable without fabricating species, and a test that passes on
// invented data is worse than one that fails honestly.
const animalPoolFloor = 12

// animalPoolMinTotal is the minimum total pool size across all 26 letters.
// It is NOT animalPoolFloor*26 (312) — it is set higher (1,320) on
// purpose, since 26 letters sitting exactly at the floor is the unhealthy
// case this check exists to catch: real letters (a, e, s, ...) have far
// more common one-word animal names than the scarce ones (q, u, x) and
// should carry more than the bare floor, or the pool is thin everywhere
// instead of appropriately uneven.
const animalPoolMinTotal = 1320

// TestAnimalPool is the pool's integrity check: every letter present, every
// letter at or above the floor, the total at or above the machine-wide
// minimum, no duplicate name anywhere in the pool (across ALL letters, not
// just within one), and every entry lowercase ASCII starting with its own
// key letter. Written before the pool was grown from its original 110
// entries to the 1,320+ target, so until that growth lands this test is
// EXPECTED to fail on the floor and total assertions — that is the point:
// it is the growth's progress meter, not a regression the growth work
// introduced.
func TestAnimalPool(t *testing.T) {
	seen := make(map[string]byte, animalPoolMinTotal)
	total := 0

	for _, letter := range alphabet {
		names, ok := animalsByLetter[letter]
		if !ok || len(names) == 0 {
			t.Errorf("letter %q: missing from animalsByLetter entirely", letter)
			continue
		}
		if len(names) < animalPoolFloor {
			t.Errorf("letter %q: pool has %d names, want at least %d (floor)", letter, len(names), animalPoolFloor)
		}
		total += len(names)

		for _, name := range names {
			if name == "" {
				t.Errorf("letter %q: empty name in pool", letter)
				continue
			}
			if name[0] != letter {
				t.Errorf("letter %q: entry %q does not start with its own key letter", letter, name)
			}
			for i := 0; i < len(name); i++ {
				c := name[i]
				isLower := c >= 'a' && c <= 'z'
				if !isLower {
					t.Errorf("letter %q: entry %q is not lowercase ASCII (offending byte %q)", letter, name, c)
					break
				}
			}
			if prevLetter, dup := seen[name]; dup {
				t.Errorf("entry %q appears under both letter %q and letter %q — duplicate in pool", name, prevLetter, letter)
				continue
			}
			seen[name] = letter
		}
	}

	if total < animalPoolMinTotal {
		t.Errorf("pool total = %d names, want at least %d", total, animalPoolMinTotal)
	}
}

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
	// Derive the seed from the pool itself rather than hardcoding a list.
	// A literal snapshot of w-animals silently stops meaning "every
	// w-animal" the moment the pool grows, which is exactly what happened
	// when animalsByLetter went from 110 entries to 1,442: the list still
	// named five, the letter held thirty-four, and the test failed while
	// the behaviour it guards was perfectly correct.
	preUsed := func() map[string]bool {
		used := make(map[string]bool, len(animalsByLetter['w']))
		for _, name := range animalsByLetter['w'] {
			used[name] = true
		}
		return used
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
