package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// hookRun feeds one hook payload to the real binary and returns its stdout.
func hookRun(t *testing.T, env []string, payload string, args ...string) string {
	t.Helper()
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^$", "--", "hook"}, args...)...)
	cmd.Env = append(append(os.Environ(), "ITERATE_RUN_MAIN_HELPER=1"), env...)
	cmd.Stdin = strings.NewReader(payload)
	out, err := cmd.Output()
	if err != nil {
		t.Fatal(err)
	}
	return string(out)
}

// TESTMASTER: id=coord-cli-session-claim tier=? parallel=no
// The installed entry points, end to end: a session claims with no --pid,
// a stranger cannot heartbeat or release, the owner report names what the
// evidence is, the stop hook continues and then records the turn end, and
// a second session reconciles and takes the plan.
func TestPlanOwnershipCLIBindsAClaimToItsSession(t *testing.T) {
	p := graphFixture(t, map[string]string{"alpha": "phase: executing\ndepends: none\nrunning: false"})
	store := t.TempDir()
	host := exec.Command("sleep", "60")
	if e := host.Start(); e != nil {
		t.Fatal(e)
	}
	defer func() { _ = host.Process.Kill(); _ = host.Wait() }()
	base := []string{"ITERATE_RUN_SESSIONS_DIR=" + store, "HOME=" + t.TempDir()}
	as := func(session string) []string {
		if session == "" {
			return base
		}
		return append([]string{"CODEX_THREAD_ID=" + session}, base...)
	}
	run := func(session string, args ...string) (string, string, int) {
		return iterateRun(t, as(session), append([]string{"plan"}, append(args, "-C", p)...)...)
	}

	if _, errs, code := run("", "claim", "alpha"); code != 2 || !strings.Contains(errs, "no session identity") {
		t.Fatalf("claim with no identity: %d %q", code, errs)
	}
	out, errs, code := run("thread-a", "claim", "alpha", "--pid", strconv.Itoa(host.Process.Pid), "--json")
	var o struct {
		Session         string
		State           string
		CoordinatorLive bool `json:"coordinator_live"`
	}
	if code != 0 || json.Unmarshal([]byte(out), &o) != nil || o.Session != "thread-a" || !o.CoordinatorLive {
		t.Fatalf("claim: %d %q %q", code, out, errs)
	}
	if _, errs, code = run("thread-b", "claim", "alpha"); code != 1 || !strings.Contains(errs, "project busy: cli owns alpha") {
		t.Fatalf("contender: %d %q", code, errs)
	}
	if _, _, code = run("thread-a", "claim", "alpha"); code != 0 {
		t.Fatalf("the owner's second claim: %d", code)
	}
	for _, verb := range []string{"heartbeat", "release"} {
		if _, errs, code = run("thread-b", verb, "alpha"); code != 1 || !strings.Contains(errs, "claimed by session thread-a") {
			t.Fatalf("stranger %s: %d %q", verb, code, errs)
		}
	}
	if _, errs, code = run("thread-b", "wait", "alpha", "--for", "x"); code != 1 {
		t.Fatalf("stranger wait: %d %q", code, errs)
	}
	if out, _, code = run("thread-a", "wait", "alpha", "--for", "pipeline 7", "--minutes", "5"); code != 0 || !strings.Contains(out, "pipeline 7") {
		t.Fatalf("wait: %d %q", code, out)
	}

	// The turn ends with the host process still alive.
	stop := `{"session_id":"thread-a","cwd":"` + p + `"}`
	if got := hookRun(t, base, stop, "stop", "--continue"); got != "" {
		t.Fatalf("a declared wait was continued: %q", got)
	}
	if out, _, _ = run("", "owner"); !strings.Contains(out, "alpha\tcli\tdriven\twaiting") || !strings.Contains(out, "pipeline 7") {
		t.Fatalf("owner during a wait: %q", out)
	}
	hookRun(t, base, `{"session_id":"thread-a","cwd":"`+p+`","tool_name":"Bash","tool_use_id":"t"}`, "pre")
	if got := hookRun(t, base, stop, "stop", "--continue"); !strings.Contains(got, `"decision":"block"`) {
		t.Fatalf("runnable work was not continued: %q", got)
	}
	if got := hookRun(t, base, stop, "stop"); got != "" {
		t.Fatalf("record-only stop wrote %q", got)
	}
	out, _, code = run("", "owner", "alpha")
	if code != 0 || !strings.Contains(out, "not driven\tturn-ended") || !strings.Contains(out, "next: iterate-run plan reconcile alpha") {
		t.Fatalf("owner after the turn ended: %q", out)
	}
	// A status read changes nothing.
	if again, _, _ := run("thread-b", "owner", "alpha"); !strings.Contains(again, "not driven\tturn-ended") {
		t.Fatalf("reading the claim revived it: %q", again)
	}

	if _, _, code = run("thread-b", "reconcile", "alpha"); code != 0 {
		t.Fatal("reconcile failed")
	}
	if _, errs, code = run("thread-b", "claim", "alpha"); code != 0 {
		t.Fatalf("successor claim: %d %q", code, errs)
	}
	// The first thread's late release cannot clear the successor.
	if _, errs, code = run("thread-a", "release", "alpha"); code != 1 || !strings.Contains(errs, "claimed by session thread-b") {
		t.Fatalf("late release: %d %q", code, errs)
	}
	out, errs, code = run("thread-b", "release", "alpha")
	if code != 0 || !strings.Contains(out, "released alpha not-terminal (phase: executing)") || !strings.Contains(errs, "not terminal") {
		t.Fatalf("release receipt: %d %q %q", code, out, errs)
	}
	if out, _, _ = run("", "owner"); out != "no claim\n" {
		t.Fatalf("owner after release: %q", out)
	}
}
