package main

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"
)

func cli(t *testing.T, args ...string) (string, string, int) {
	t.Helper()
	var out, errb strings.Builder
	code := run(args, &out, &errb)
	return out.String(), errb.String(), code
}

// The house grammar: every flag parses in every position.
func TestFlagsParseInEveryPosition(t *testing.T) {
	forms := [][]string{
		{"--json", "-C", "/x", "suite", "run", "fast", "-j", "2"},
		{"suite", "--json", "run", "-j=2", "fast", "-C", "/x"},
		{"suite", "run", "fast", "--project=/x", "--jobs", "2", "--json"},
	}
	var first []string
	var firstFlags map[string]string
	for i, f := range forms {
		pos, flags, err := parse(f)
		if err != nil {
			t.Fatal(err)
		}
		if i == 0 {
			first, firstFlags = pos, flags
			continue
		}
		if !reflect.DeepEqual(pos, first) || !reflect.DeepEqual(flags, firstFlags) {
			t.Errorf("form %d parsed to %v %v, want %v %v", i, pos, flags, first, firstFlags)
		}
	}
	if pos, _, _ := parse([]string{"test", "add", "--", "--odd-id"}); pos[2] != "--odd-id" {
		t.Errorf("-- did not end flag parsing: %v", pos)
	}
}

func TestCLIEndToEnd(t *testing.T) {
	dir := t.TempDir()

	if _, errOut, code := cli(t, "suite", "run", "-C", dir); code != 3 || !strings.Contains(errOut, "testmaster suite discover") {
		t.Errorf("unregistered project: code %d, stderr %q", code, errOut)
	}
	for _, args := range [][]string{
		{"test", "add", "ok", "--kind", "shell", "--cmd", "true"},
		{"test", "add", "bad", "--kind", "shell", "--cmd", "echo nope; exit 1"},
		{"test", "add", "gate", "--kind", "shell", "--cmd", "test -d .", "--blocking"},
	} {
		if _, errOut, code := cli(t, append(args, "-C", dir)...); code != 0 {
			t.Fatalf("%v: code %d: %s", args, code, errOut)
		}
	}
	if _, errOut, code := cli(t, "test", "add", "ok", "--kind", "shell", "--cmd", "true", "-C", dir); code != 1 || !strings.Contains(errOut, "already registered") {
		t.Errorf("duplicate add: %d %q", code, errOut)
	}
	if _, _, code := cli(t, "test", "add", "x", "--kind", "go", "-C", dir); code != 1 {
		t.Errorf("go test without --pkg/--name should be refused, got %d", code)
	}

	out, _, code := cli(t, "suite", "run", "-C", dir)
	lines := strings.Split(strings.TrimSpace(out), "\n")
	if code != 1 || len(lines) != 2 || !strings.HasPrefix(lines[0], "run fast+standard: 2 passed, 1 failed") ||
		!strings.HasPrefix(lines[1], "FAIL\tbad\t") {
		t.Errorf("run: code %d\n%s", code, out)
	}

	out, _, _ = cli(t, "test", "list", "--failing", "-C", dir)
	if strings.TrimSpace(out) != "bad" {
		t.Errorf("failing list = %q", out)
	}

	out, _, code = cli(t, "suite", "run", "--failed", "--json", "-C", dir)
	var rep struct {
		Selection string         `json:"selection"`
		Counts    map[string]int `json:"counts"`
		Exit      int            `json:"exit"`
		Problems  []struct {
			ID, Status, Log, Rerun string
		} `json:"problems"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("--json output is not JSON: %v\n%s", err, out)
	}
	if code != 1 || rep.Exit != 1 || rep.Selection != "failed" || len(rep.Problems) != 1 || rep.Problems[0].Rerun != "echo nope; exit 1" {
		t.Errorf("json report = %+v (code %d)", rep, code)
	}

	if _, _, code := cli(t, "test", "set", "bad", "--serial", "-C", dir); code != 0 {
		t.Error("test set --serial failed")
	}
	out, _, _ = cli(t, "test", "show", "bad", "-C", dir)
	if !strings.Contains(out, `"parallel_safe": false`) {
		t.Errorf("show after set:\n%s", out)
	}
	if _, _, code := cli(t, "test", "remove", "bad", "-C", dir); code != 0 {
		t.Error("remove failed")
	}
	out, _, code = cli(t, "suite", "run", "-C", dir)
	if code != 0 || strings.Count(out, "\n") != 1 {
		t.Errorf("all green should be one line and exit 0: %d\n%s", code, out)
	}

	out, _, _ = cli(t, "suite", "status", "-C", dir)
	if !strings.Contains(out, "blocking\tgate") || !strings.Contains(out, "results\tpass 2") {
		t.Errorf("status:\n%s", out)
	}

	if _, errOut, code := cli(t, "suite", "run", "nosuch", "-C", dir); code != 2 || !strings.Contains(errOut, `no test matches "nosuch"`) {
		t.Errorf("unknown id: %d %q", code, errOut)
	}
	if _, errOut, code := cli(t, "test", "list", "--jobs", "2", "-C", dir); code != 2 || !strings.Contains(errOut, "--jobs does not apply to test list") {
		t.Errorf("misplaced local flag: %d %q", code, errOut)
	}
	if _, _, code := cli(t, "runner", "set", "shell", "-C", dir, "--", "sh", "-c"); code != 0 {
		t.Error("runner set failed")
	}
	if out, _, _ := cli(t, "runner", "list", "-C", dir); out != "shell\tsh -c\n" {
		t.Errorf("runner list = %q", out)
	}
	if out, _, code := cli(t, "suite", "run", "-C", dir); code != 0 {
		t.Errorf("run through the sh runner failed: %s", out)
	}
	if out, _, code := cli(t); code != 0 || !strings.Contains(out, "Usage:") {
		t.Errorf("bare invocation should print help and exit 0: %d", code)
	}
}
