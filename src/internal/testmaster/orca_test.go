package testmaster

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// Two workspace packages each have a `combine` integration test target. Both
// sets run, nothing is MISSING, and discovery lists the same set every time.
func TestCargoWorkspaceSameTargetName(t *testing.T) {
	need(t, "cargo")
	t.Parallel()
	dir := t.TempDir()
	write(t, dir, "Cargo.toml", "[workspace]\nmembers = [\"core\", \"tui\"]\nresolver = \"2\"\n")
	for _, pkg := range []string{"core", "tui"} {
		write(t, dir, pkg+"/Cargo.toml", "[package]\nname = \"ws-"+pkg+"\"\nversion = \"0.1.0\"\nedition = \"2021\"\n")
		write(t, dir, pkg+"/src/lib.rs", "pub fn one() -> i32 { 1 }\n")
		write(t, dir, pkg+"/tests/combine.rs", "#[test]\nfn "+pkg+"_combines() { assert_eq!(1, 1); }\n#[test]\nfn shared_name() {}\n")
	}
	p, _ := Open(dir)
	discovered := func() []string {
		reps, err := p.Discover(context.Background(), nil, false)
		if err != nil || len(reps) != 1 || reps[0].Err != nil {
			t.Fatalf("discover: %v %+v", err, reps)
		}
		if len(reps[0].Missing) != 0 {
			t.Errorf("discover reports MISSING %v", reps[0].Missing)
		}
		ids := make([]string, 0, len(p.Reg.Tests))
		for id := range p.Reg.Tests {
			ids = append(ids, id)
		}
		sort.Strings(ids)
		return ids
	}
	first := discovered()
	if second := discovered(); strings.Join(first, ",") != strings.Join(second, ",") {
		t.Errorf("discover is unstable: %v then %v", first, second)
	}
	combine := 0
	for _, e := range p.Reg.Tests {
		if e.Target == "test:combine" {
			combine++
		}
	}
	if combine != 4 {
		t.Fatalf("want 4 combine tests across both packages, got %d: %v", combine, first)
	}
	rep := runAll(t, p, RunOptions{})
	if rep.Counts[Missing] != 0 || rep.Counts[Pass] != len(p.Reg.Tests) {
		t.Errorf("counts %v over %d tests: %v", rep.Counts, len(p.Reg.Tests), statuses(rep))
	}
}

// A catalog with flat top-level cases[] and a `tests` list is stamped and
// checked for links like the nested shape.
func TestFlatCatalogCases(t *testing.T) {
	need(t, "git")
	p := project(t, map[string]*Entry{"ok": {Kind: "shell", Cmd: "true"}})
	for _, args := range [][]string{{"init", "-q"}, {"-c", "user.email=t@t", "-c", "user.name=t", "commit", "-q", "--allow-empty", "-m", "x"}} {
		if out, err := exec.Command("git", append([]string{"-C", p.Root}, args...)...).CombinedOutput(); err != nil {
			t.Fatalf("git %v: %s", args, out)
		}
	}
	write(t, p.Root, StateDir+"/catalog.json", `{"schema":1,"cases":[{"id":"flat","tests":["ok"]},{"id":"dangling","test":"nope"}]}`)
	runAll(t, p, RunOptions{})
	b, _ := os.ReadFile(filepath.Join(p.Root, StateDir, "catalog.json"))
	var cat struct{ Cases []map[string]any }
	if err := json.Unmarshal(b, &cat); err != nil {
		t.Fatal(err)
	}
	if cat.Cases[0]["last_validated_commit"] == nil {
		t.Errorf("flat case not stamped:\n%s", b)
	}
	un, err := p.Unlinked()
	if err != nil || len(un) != 1 || un[0].Case != "dangling" || un[0].Test != "nope" {
		t.Errorf("Unlinked = %v, %v; want dangling → nope", un, err)
	}
}

// A legacy bare-array registry.json opens with every entry, and the next save
// writes the object shape.
func TestBareArrayRegistryMigrates(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, StateDir+"/registry.json", `[{"id":"a","kind":"shell","cmd":"true"},{"id":"b","kind":"shell","cmd":"false"}]`)
	p, err := Open(dir)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if len(p.Reg.Tests) != 2 || p.Reg.Tests["a"] == nil || p.Reg.Tests["b"].Cmd != "false" {
		t.Fatalf("tests = %v", p.Reg.Tests)
	}
	if err := p.Save(); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, StateDir, "registry.json"))
	if !strings.HasPrefix(strings.TrimSpace(string(b)), "{") || strings.Contains(string(b), `"id"`) {
		t.Errorf("saved registry is not the object shape:\n%s", b)
	}
	write(t, dir, StateDir+"/registry.json", `[{"kind":"shell","cmd":"true"}]`)
	if _, err := Open(dir); err == nil || !strings.Contains(err.Error(), "no \"id\"") {
		t.Errorf("an entry without id must name the fix, got %v", err)
	}
}
