package iterrun

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	enrolledConductor = "---\nenabled: true\npaused: false\ncron:\ntick-source: launchd\ntick: working\n---\n\n## Sweep log\n"
	plainConductor    = "---\nenabled: true\npaused: false\ncron: abc123\n---\n"
	queuedPlan        = "name: quokka\nStarted: 2026-10-05T00:00:00Z (planned)\nphase: planned\nrunning: false\nstatus: queued\n\n## Goal\nq\n"
	plannedPlan       = "name: yak\nStarted: 2026-10-05T00:00:00Z (planned)\nphase: planned\nrunning: false\n\n## Goal\ny\n"
)

// nightlyHome points every store path (heartbeats, locks, projects.json,
// nightly.json) at a scratch HOME so no test touches the real one.
func nightlyHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	return home
}

func mkNightlyProject(t *testing.T, home, name, conductor, plan string) string {
	t.Helper()
	dir := filepath.Join(home, "ws", name)
	it := filepath.Join(dir, ".claude", "iterate")
	if err := os.MkdirAll(filepath.Join(it, "plans"), 0o755); err != nil {
		t.Fatal(err)
	}
	if conductor != "" {
		if err := os.WriteFile(filepath.Join(it, "conductor.md"), []byte(conductor), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if plan != "" {
		if err := os.WriteFile(filepath.Join(it, "plans", "p.md"), []byte(plan), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	RegisterProject(dir)
	return dir
}

// Monday 2026-10-05 23:00 local — inside a 22:00-06:00 window.
var mondayNight = time.Date(2026, 10, 5, 23, 0, 0, 0, time.Local)

// TESTMASTER: id=iterrun.TestDecideProjectSkipOrder tier=? parallel=yes
func TestDecideProjectSkipOrder(t *testing.T) {
	home := nightlyHome(t)
	live := 15 * time.Minute

	// tick-1: enrolled, queued, nothing else in the way → launch.
	p1 := mkNightlyProject(t, home, "p1", enrolledConductor, queuedPlan)
	if d := decideProject(p1, mondayNight, live); !d.Launch {
		t.Errorf("tick-1: %s; want launch", d)
	}

	// tick-2: registered but not enrolled.
	p2 := mkNightlyProject(t, home, "p2", plainConductor, queuedPlan)
	if d := decideProject(p2, mondayNight, live); d.Launch || d.Reason != "not enrolled" {
		t.Errorf("tick-2: %s; want skip: not enrolled", d)
	}
	if d := decideProject(mkNightlyProject(t, home, "p2b", "", queuedPlan), mondayNight, live); d.Reason != "not enrolled" {
		t.Errorf("tick-2 (no conductor.md): %s", d)
	}

	// tick-10: enrolled but the conductor is disabled, or paused.
	p3 := mkNightlyProject(t, home, "p3", strings.Replace(enrolledConductor, "enabled: true", "enabled: false", 1), queuedPlan)
	if d := decideProject(p3, mondayNight, live); d.Reason != "conductor disabled" {
		t.Errorf("tick-10 disabled: %s", d)
	}
	p4 := mkNightlyProject(t, home, "p4", strings.Replace(enrolledConductor, "paused: false", "paused: true", 1), queuedPlan)
	if d := decideProject(p4, mondayNight, live); d.Reason != "conductor paused" {
		t.Errorf("tick-10 paused: %s", d)
	}

	// Nothing startable: a bare planned plan is never launched unattended (tick-7's tick half).
	p5 := mkNightlyProject(t, home, "p5", enrolledConductor, plannedPlan)
	if d := decideProject(p5, mondayNight, live); d.Reason != "nothing queued" {
		t.Errorf("planned-only: %s; want skip: nothing queued", d)
	}
	// ...but an executing plan whose run died is (tick-8: the next tick resumes it).
	p6 := mkNightlyProject(t, home, "p6", enrolledConductor, strings.Replace(plannedPlan, "phase: planned", "phase: executing\nloop-mechanism: external", 1))
	if d := decideProject(p6, mondayNight, live); !d.Launch {
		t.Errorf("stale executing: %s; want launch", d)
	}

	// tick-5: a fresh heartbeat means a human session is driving.
	TouchHeartbeat(p1)
	if d := decideProject(p1, time.Now(), live); d.Reason != "live" {
		t.Errorf("tick-5: %s; want skip: live", d)
	}
	if d := decideProject(p1, time.Now().Add(live+time.Minute), live); !d.Launch {
		t.Errorf("heartbeat older than the window: %s; want launch", d)
	}
	os.Remove(HeartbeatPath(p1))

	// tick-6: the previous child still holds the lock.
	unlock, err := tryNightlyLock(p1)
	if err != nil {
		t.Fatal(err)
	}
	if d := decideProject(p1, mondayNight, live); d.Reason != "running" {
		t.Errorf("tick-6: %s; want skip: running", d)
	}
	unlock()
	if d := decideProject(p1, mondayNight, live); !d.Launch {
		t.Errorf("after release: %s; want launch (the lock dies with its holder)", d)
	}
}

// TESTMASTER: id=iterrun.TestDecideProjectSchedules tier=? parallel=yes
func TestDecideProjectSchedules(t *testing.T) {
	home := nightlyHome(t)
	live := 15 * time.Minute
	dir := mkNightlyProject(t, home, "sched", enrolledConductor, queuedPlan)
	policy := filepath.Join(dir, ".claude", "iterate", "policy.md")
	if err := os.WriteFile(policy, []byte("---\nlaunch-window: 22:00-06:00\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// tick-4: outside the launch window, with the next window named.
	noon := time.Date(2026, 10, 5, 12, 0, 0, 0, time.Local)
	d := decideProject(dir, noon, live)
	if d.Launch || d.Reason != "outside launch-schedule (no allow rule matches)" {
		t.Errorf("tick-4: %s", d)
	}
	if want := time.Date(2026, 10, 5, 22, 0, 0, 0, time.Local); !d.Next.Equal(want) {
		t.Errorf("tick-4 next = %v, want %v", d.Next, want)
	}
	if d := decideProject(dir, mondayNight, live); !d.Launch {
		t.Errorf("inside window: %s; want launch", d)
	}

	// A narrower conductor-schedule narrows further (and is named).
	cond := strings.Replace(enrolledConductor, "tick: working\n", "tick: working\nconductor-schedule:\n  - allow daily 23:30-05:00\n", 1)
	if err := os.WriteFile(filepath.Join(dir, ".claude", "iterate", "conductor.md"), []byte(cond), 0o644); err != nil {
		t.Fatal(err)
	}
	d = decideProject(dir, mondayNight, live)
	if d.Launch || !strings.HasPrefix(d.Reason, "outside conductor-schedule") {
		t.Errorf("narrow conductor schedule at 23:00: %s", d)
	}
	if d := decideProject(dir, mondayNight.Add(45*time.Minute), live); !d.Launch {
		t.Errorf("23:45 inside both: %s; want launch", d)
	}

	// A malformed policy fails closed and names the line.
	if err := os.WriteFile(policy, []byte("---\nlaunch-schedule:\n  - allow mon-fri 22:00-06:00\n  - permit weekends\n---\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	d = decideProject(dir, mondayNight, live)
	if d.Launch || !strings.Contains(d.Reason, "permit weekends") {
		t.Errorf("malformed policy: %s; want refused, naming the line", d)
	}
}

// TESTMASTER: id=iterrun.TestTickDisabledAndDryRun tier=? parallel=yes
func TestTickDisabledAndDryRun(t *testing.T) {
	home := nightlyHome(t)
	launch := mkNightlyProject(t, home, "a-launch", enrolledConductor, queuedPlan)
	idle := mkNightlyProject(t, home, "b-idle", enrolledConductor, plannedPlan)
	mkNightlyProject(t, home, "c-unenrolled", plainConductor, queuedPlan)

	// tick-3: the global switch off → one line, nothing decided.
	if err := SaveNightlyConfig(NightlyConfig{Enabled: false}); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	if err := Tick(&buf, true); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "disabled\n" {
		t.Errorf("tick-3 dry-run = %q, want \"disabled\\n\"", buf.String())
	}

	// Switch on: enrolled projects are decided, unenrolled ones are not mentioned.
	if err := SaveNightlyConfig(NightlyConfig{Enabled: true}); err != nil {
		t.Fatal(err)
	}
	buf.Reset()
	if err := Tick(&buf, true); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.Contains(out, "would launch "+launch+"\n") {
		t.Errorf("dry-run missing the launch line:\n%s", out)
	}
	if !strings.Contains(out, filepath.Base(idle)+": skip: nothing queued") {
		t.Errorf("dry-run missing the idle skip:\n%s", out)
	}
	if strings.Contains(out, "c-unenrolled") {
		t.Errorf("dry-run mentions an unenrolled project:\n%s", out)
	}
	if _, err := os.Stat(filepath.Join(NightlyLogDir(), "status.txt")); !os.IsNotExist(err) {
		t.Error("a dry run wrote status.txt; it must only print")
	}
}

// TESTMASTER: id=iterrun.TestEnrollWithdrawRoundTrip tier=? parallel=yes
func TestEnrollWithdrawRoundTrip(t *testing.T) {
	home := nightlyHome(t)
	dir := filepath.Join(home, "ws", "fresh")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Enroll(dir); err != nil {
		t.Fatal(err)
	}
	cs, ok := ReadConductorState(dir)
	if !ok || !cs.Enabled || cs.TickSource != "launchd" {
		t.Errorf("after Enroll on a project with no conductor.md: ok=%v %+v; want enabled, tick-source launchd", ok, cs)
	}
	known, _ := ListProjects()
	if len(known) != 1 {
		t.Errorf("Enroll did not register the project: %v", known)
	}

	// Existing conductor.md: other keys survive, the key is inserted once.
	path := filepath.Join(dir, ".claude", "iterate", "conductor.md")
	if err := os.WriteFile(path, []byte(plainConductor+"\n# Conductor\n\n## Sweep log\n- [2026-10-01T00:00Z] swept\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := Enroll(dir); err != nil {
			t.Fatal(err)
		}
	}
	data, _ := os.ReadFile(path)
	if c := strings.Count(string(data), "tick-source: launchd"); c != 1 {
		t.Errorf("tick-source lines = %d, want exactly 1:\n%s", c, data)
	}
	cs, _ = ReadConductorState(dir)
	if cs.Cron != "abc123" || cs.TickSource != "launchd" || !strings.Contains(string(data), "## Sweep log") {
		t.Errorf("Enroll disturbed other content: %+v\n%s", cs, data)
	}

	if err := Withdraw(dir); err != nil {
		t.Fatal(err)
	}
	cs, _ = ReadConductorState(dir)
	data, _ = os.ReadFile(path)
	if cs.TickSource != "" || strings.Contains(string(data), "tick-source") || cs.Cron != "abc123" {
		t.Errorf("after Withdraw: %+v\n%s", cs, data)
	}
	// Withdrawing a project with no conductor.md is a no-op, not an error.
	if err := Withdraw(filepath.Join(home, "ws", "none")); err != nil {
		t.Errorf("Withdraw on a bare dir: %v", err)
	}
}

// TESTMASTER: id=iterrun.TestSetFrontmatterKey tier=? parallel=yes
func TestSetFrontmatterKey(t *testing.T) {
	got := setFrontmatterKey("---\na: 1\nb: 2\n---\nbody\n", "tick-source", "launchd")
	if got != "---\na: 1\nb: 2\ntick-source: launchd\n---\nbody\n" {
		t.Errorf("insert = %q", got)
	}
	got = setFrontmatterKey("---\na: 1\ntick-source: cron\nb: 2\n---\n", "tick-source", "launchd")
	if got != "---\na: 1\ntick-source: launchd\nb: 2\n---\n" {
		t.Errorf("replace = %q", got)
	}
	got = setFrontmatterKey("---\na: 1\ntick-source: launchd\n---\n", "tick-source", "")
	if got != "---\na: 1\n---\n" {
		t.Errorf("remove = %q", got)
	}
	got = setFrontmatterKey("# no frontmatter\n", "tick-source", "launchd")
	if got != "---\ntick-source: launchd\n---\n# no frontmatter\n" {
		t.Errorf("add block = %q", got)
	}
}

// TESTMASTER: id=iterrun.TestPlistXMLShape tier=? parallel=yes
func TestPlistXMLShape(t *testing.T) {
	x := PlistXML("/usr/local/bin/iterate-run", 10*time.Minute, "/a:/b")
	for _, want := range []string{
		"<key>Label</key><string>com.x85446.iterate-nightly</string>",
		"<string>/usr/local/bin/iterate-run</string><string>nightly</string><string>tick</string>",
		"<key>StartInterval</key><integer>600</integer>",
		"<key>ProcessType</key><string>Standard</string>",
		"<key>PATH</key><string>/a:/b</string>",
	} {
		if !strings.Contains(x, want) {
			t.Errorf("plist missing %q", want)
		}
	}
	if strings.Contains(x, "LowPriorityIO") || strings.Contains(x, "Background") {
		t.Error("plist throttles the tick; Background/LowPriorityIO made claude's Keychain read time out (confirmed live)")
	}
}
