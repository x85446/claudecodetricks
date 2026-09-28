package iterrun

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"
)

// TestPoolTables is the whole-table integrity check, and it is the reason
// the tables can be hand-edited safely: every entry must be lowercase
// ASCII filed under its own first letter, and no word may appear twice
// anywhere across all six namespaces. A word duplicated between, say,
// rivers and trees would be claimed once and then silently skipped the
// second time, so two projects owed the same letter would get different
// depths — the check is cheap and the failure mode is invisible.
func TestPoolTables(t *testing.T) {
	seen := map[string]string{}
	for _, ns := range namespaceOrder {
		table, ok := pools[ns]
		if !ok {
			t.Errorf("namespace %q is in namespaceOrder but has no table", ns)
			continue
		}
		for _, letter := range alphabet {
			for _, word := range table[letter] {
				where := fmt.Sprintf("%s/%c", ns, letter)
				if word == "" {
					t.Errorf("%s: empty entry", where)
					continue
				}
				if word[0] != letter {
					t.Errorf("%s: %q is filed under the wrong letter", where, word)
				}
				if strings.Trim(word, "abcdefghijklmnopqrstuvwxyz") != "" {
					t.Errorf("%s: %q is not lowercase ASCII", where, word)
				}
				if prev, dup := seen[word]; dup {
					t.Errorf("%q appears in both %s and %s", word, prev, where)
					continue
				}
				seen[word] = where
			}
		}
	}
	for ns := range pools {
		found := false
		for _, o := range namespaceOrder {
			if o == ns {
				found = true
			}
		}
		if !found {
			t.Errorf("namespace %q has a table but is missing from namespaceOrder, so it is never consumed", ns)
		}
	}
}

// TestEveryLetterHasWords is the property the per-project alphabet rests
// on: no letter may be empty, or a project owed that letter could not be
// served at all. stars/q is legitimately empty, which is exactly why the
// check is against the concatenated pool and not any one namespace.
func TestEveryLetterHasWords(t *testing.T) {
	for _, letter := range alphabet {
		if LetterDepth(letter) == 0 {
			t.Errorf("letter %q has no words in any namespace", letter)
		}
	}
}

// TestPoolWordConsumesNamespacesInOrder pins the ordering the user asked
// for: animals are spent before minerals, minerals before cities, and so
// on down namespaceOrder.
func TestPoolWordConsumesNamespacesInOrder(t *testing.T) {
	const letter = 'q'
	n := 0
	for _, ns := range namespaceOrder {
		for range pools[ns][letter] {
			got := PoolWord(letter, n)
			if !contains(pools[ns][letter], got) {
				t.Fatalf("PoolWord(%c, %d) = %q, want a word from %q", letter, n, got, ns)
			}
			n++
		}
	}
}

// TestPoolWordSuffixesOnWrap is the mechanism that makes a thin letter a
// non-issue: past the real words it laps, appending the occurrence
// number. Nothing is fabricated in the table to square the columns.
func TestPoolWordSuffixesOnWrap(t *testing.T) {
	for _, letter := range []byte{'q', 'x', 'a'} {
		depth := LetterDepth(letter)
		first := PoolWord(letter, 0)
		if got, want := PoolWord(letter, depth), first+"2"; got != want {
			t.Errorf("letter %c: PoolWord(depth) = %q, want %q", letter, got, want)
		}
		if got, want := PoolWord(letter, depth*2), first+"3"; got != want {
			t.Errorf("letter %c: PoolWord(2*depth) = %q, want %q", letter, got, want)
		}
	}
}

// TestPoolWordIsUniqueForever is the claim the suffix scheme has to
// support: no two indices ever produce the same word, so a project can
// keep drawing its letter indefinitely.
func TestPoolWordIsUniqueForever(t *testing.T) {
	const letter = 'x'
	seen := map[string]int{}
	for n := range LetterDepth(letter) * 5 {
		word := PoolWord(letter, n)
		if prev, dup := seen[word]; dup {
			t.Fatalf("PoolWord(%c, %d) = %q, already produced at index %d", letter, n, word, prev)
		}
		seen[word] = n
	}
}

// TestThinLetterKeepsItsLetter is the regression test for the bug this
// replaced. Twenty projects all owed 'q' at the same time used to run the
// letter dry and start silently handing out r-words; every one of them
// must now get a q.
func TestThinLetterKeepsItsLetter(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan-names.json")
	lock := filepath.Join(dir, "plan-names.json.lock")
	noSeed := func() map[string]bool { return nil }

	seen := map[string]bool{}
	for p := range 20 {
		proj := filepath.Join(dir, fmt.Sprintf("proj%d", p))
		// 16 claims lands this project's sequence on 'q'.
		var name string
		for range 17 {
			var err error
			if name, err = nextPlanName(path, lock, proj, noSeed); err != nil {
				t.Fatal(err)
			}
		}
		if name[0] != 'q' {
			t.Fatalf("project %d: 17th name = %q, want a q-word", p, name)
		}
		if seen[name] {
			t.Fatalf("project %d: %q handed out twice", p, name)
		}
		seen[name] = true
	}
}

// TestReleasePlanNameReturnsWordToPool covers the one operation an agent
// performs deliberately: checking a codename back in so it can be issued
// again.
func TestReleasePlanNameReturnsWordToPool(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "plan-names.json")
	lock := filepath.Join(dir, "plan-names.json.lock")
	noSeed := func() map[string]bool { return nil }
	proj := filepath.Join(dir, "proj")

	first, err := nextPlanName(path, lock, proj, noSeed)
	if err != nil {
		t.Fatal(err)
	}
	held, err := releasePlanName(path, lock, first)
	if err != nil {
		t.Fatal(err)
	}
	if !held {
		t.Fatalf("releasing %q reported it was not held", first)
	}
	// Releasing does not rewind the letter index, so drive the sequence
	// back round to 'a' and check the word is issuable again.
	var reissued string
	for range 26 {
		if reissued, err = nextPlanName(path, lock, proj, noSeed); err != nil {
			t.Fatal(err)
		}
	}
	if reissued != first {
		t.Errorf("after release, next a-word = %q, want the released %q back", reissued, first)
	}
	if held, err := releasePlanName(path, lock, "notaplanname"); err != nil || held {
		t.Errorf("releasing an unheld word = (%v, %v), want (false, nil)", held, err)
	}
}

func contains(words []string, want string) bool {
	for _, w := range words {
		if w == want {
			return true
		}
	}
	return false
}
