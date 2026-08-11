package iterrun

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestBuildRowsGapDetection(t *testing.T) {
	base := time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)
	mk := func(offsetSec int, hook, agentID, toolName, toolUseID string) Event {
		return Event{
			TS: base.Add(time.Duration(offsetSec) * time.Second), Hook: hook,
			AgentID: agentID, ToolName: toolName, ToolUseID: toolUseID,
		}
	}

	events := []Event{
		// coordinator: two calls 10s apart (no notable gap)
		mk(0, "pre", "", "Bash", "c1"),
		mk(2, "post", "", "Bash", "c1"),
		mk(12, "pre", "", "Bash", "c2"),
		mk(14, "post", "", "Bash", "c2"),
		// coordinator: then a 400s (severe) gap before the next call
		mk(414, "pre", "", "Bash", "c3"),
		mk(416, "post", "", "Bash", "c3"),

		// agent "team-x": one call, then a 90s (notable, not severe) gap
		mk(0, "pre", "team-x", "Bash", "a1"),
		mk(1, "post", "team-x", "Bash", "a1"),
		mk(91, "pre", "team-x", "Bash", "a2"),
		mk(93, "post", "team-x", "Bash", "a2"),
	}

	rows := BuildRows(events, map[string]string{"team-x": "badger-app"})
	if len(rows) != 2 {
		t.Fatalf("expected 2 rows (coordinator + team-x), got %d", len(rows))
	}

	var coord, teamX *Row
	for i := range rows {
		switch rows[i].key {
		case "":
			coord = &rows[i]
		case "team-x":
			teamX = &rows[i]
		}
	}
	if coord == nil || teamX == nil {
		t.Fatalf("missing expected rows: coord=%v teamX=%v", coord, teamX)
	}

	if coord.label != "coordinator" {
		t.Errorf("coordinator label = %q, want %q", coord.label, "coordinator")
	}
	if teamX.label != "badger-app" {
		t.Errorf("team-x label = %q, want %q (label resolution failed)", teamX.label, "badger-app")
	}

	if len(coord.spans) != 3 {
		t.Fatalf("coordinator spans = %d, want 3", len(coord.spans))
	}
	if len(coord.gaps) != 1 {
		t.Fatalf("coordinator gaps = %d, want 1 (only the 400s gap clears the 60s notable threshold)", len(coord.gaps))
	}
	if coord.gaps[0].dur() < SevereGap {
		t.Errorf("coordinator's one gap should be severe (>=5m), got %s", coord.gaps[0].dur())
	}

	if len(teamX.gaps) != 1 {
		t.Fatalf("team-x gaps = %d, want 1", len(teamX.gaps))
	}
	if teamX.gaps[0].dur() < NotableGap || teamX.gaps[0].dur() >= SevereGap {
		t.Errorf("team-x's gap should be notable but not severe, got %s", teamX.gaps[0].dur())
	}

	// Rendering shouldn't error, and the summary should mention the severe gap.
	var buf strings.Builder
	PrintTimelineSummary(&buf, rows)
	out := buf.String()
	if !strings.Contains(out, "SEVERE GAP") {
		t.Errorf("summary output missing SEVERE GAP marker:\n%s", out)
	}
	if !strings.Contains(out, "badger-app") {
		t.Errorf("summary output missing resolved label badger-app:\n%s", out)
	}
}

// TestBuildRowsFromHookEventsScopesCoordinatorByProject reproduces the bug
// reported live: plan codenames (finch, lynx, badger, ...) are drawn from a
// small shared pool and get reused across unrelated projects. Two different
// projects independently name a plan "lynx" hours apart; without CWD
// scoping, project B's freshly-started coordinator inherited project A's
// long-finished coordinator activity, showing a multi-hour "severe gap"
// and a bogus multi-hour total elapsed time for a run that had barely
// started.
func TestBuildRowsFromHookEventsScopesCoordinatorByProject(t *testing.T) {
	base := time.Date(2026, 8, 10, 19, 0, 0, 0, time.UTC)
	mk := func(offsetSec int, hook, agentID, team, cwd, toolUseID string) Event {
		return Event{
			TS: base.Add(time.Duration(offsetSec) * time.Second), Hook: hook,
			AgentID: agentID, Team: team, ToolName: "Bash", ToolUseID: toolUseID,
			Plan: "lynx", CWD: cwd,
		}
	}

	const projectA = "/Users/travis/workspace/x85446/financeSheets/personaldb"
	const projectB = "/Users/travis/workspace/x85446/newcorder"

	events := []Event{
		// project A's coordinator: an old, unrelated "lynx" plan, done hours
		// before project B's "lynx" plan even existed.
		mk(0, "pre", "", "", projectA, "a1"),
		mk(2, "post", "", "", projectA, "a1"),
		mk(20, "pre", "", "", projectA, "a2"),
		mk(22, "post", "", "", projectA, "a2"),

		// project B's coordinator: the plan actually being viewed, started
		// hours later — should read as freshly started, not as having a
		// multi-hour gap inherited from project A.
		mk(17000, "pre", "", "", projectB, "b1"),
		mk(17002, "post", "", "", projectB, "b1"),

		// project B's own team member: a legitimate cross-directory dispatch
		// (teams can and do work outside the plan's own project) — must NOT
		// be excluded just because its CWD isn't projectB. Its Team comes
		// pre-resolved (as it would from a real dispatch label), same as
		// hookcmd.go's resolvePlanTeam already does before writing the event.
		mk(17000, "pre", "some-agent-id", "team-x", "/some/other/checkout", "t1"),
		mk(17005, "post", "some-agent-id", "team-x", "/some/other/checkout", "t1"),
	}

	rows := BuildRowsFromHookEvents(events, nil, "lynx", projectB, time.Time{})

	var coord, teamX *Row
	for i := range rows {
		switch rows[i].key {
		case "":
			coord = &rows[i]
		case "team-x":
			teamX = &rows[i]
		}
	}
	if coord == nil {
		t.Fatalf("missing coordinator row")
	}
	if len(coord.spans) != 1 {
		t.Fatalf("coordinator spans = %d, want 1 (project A's events must be excluded), got spans: %+v", len(coord.spans), coord.spans)
	}
	if !coord.spans[0].start.Equal(base.Add(17000 * time.Second)) {
		t.Errorf("coordinator span start = %s, want project B's own event at %s (project A leaked in)",
			coord.spans[0].start, base.Add(17000*time.Second))
	}

	if teamX == nil {
		t.Fatalf("missing team-x row — a legitimate cross-directory team dispatch must not be excluded")
	}
	if len(teamX.spans) != 1 {
		t.Fatalf("team-x spans = %d, want 1", len(teamX.spans))
	}
}

func TestParsePlanStarted(t *testing.T) {
	cases := []struct {
		in      string
		wantOK  bool
		wantYMD string // "2006-01-02" of the parsed instant, in local time
	}{
		{"2026-08-10 05:39:52", true, "2026-08-10"},
		{"2026-08-10", true, "2026-08-10"},
		{"2026-08-10 (planned)", true, "2026-08-10"},
		{"", false, ""},
		{"whenever we get to it", false, ""},
	}
	for _, c := range cases {
		got, ok := parsePlanStarted(c.in)
		if ok != c.wantOK {
			t.Errorf("parsePlanStarted(%q) ok = %v, want %v", c.in, ok, c.wantOK)
			continue
		}
		if ok && got.Format("2006-01-02") != c.wantYMD {
			t.Errorf("parsePlanStarted(%q) = %s, want date %s", c.in, got, c.wantYMD)
		}
	}
}

// TestBuildRowsFromFilesystemExcludesStaleRegistryEntries reproduces the
// second bug reported live, alongside the coordinator one: plan codenames
// get reused within the SAME project over time too, not just across
// projects. An old, completed "badger" plan (team "dup-detect", started
// 2026-08-05) left its iterate-run registry entries on disk; five days
// later a brand new, unrelated "badger" plan (team "repo-ci") reused the
// name. Without this fix, the dashboard showed a "144h49m so far" total
// and five ghost team rows that don't even appear in the current plan's
// own Teams table, entirely inherited from the old run.
func TestBuildRowsFromFilesystemExcludesStaleRegistryEntries(t *testing.T) {
	home := t.TempDir()
	plansDir := filepath.Join(home, ".claude", "iterate", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	planMD := "# Plan\n\nname: badger\nStarted: 2026-08-10 (planned)\nphase: executing\n\n" +
		"## Teams\n\n| Team | Steps | Focus | Depends on | Agent | Status |\n" +
		"|---|---|---|---|---|---|\n| repo-ci | 1 | ci | — | agent | in-progress |\n"
	if err := os.WriteFile(filepath.Join(plansDir, "badger.md"), []byte(planMD), 0o644); err != nil {
		t.Fatal(err)
	}

	regDir := RegistryDir(home)
	staleFinished := time.Date(2026, 8, 5, 10, 52, 2, 0, time.UTC)
	stale := &Entry{Plan: "badger", Team: "dup-detect", Unit: "step0-baseline",
		Started: time.Date(2026, 8, 5, 10, 51, 58, 0, time.UTC), Finished: &staleFinished}
	freshFinished := time.Date(2026, 8, 11, 0, 13, 0, 0, time.UTC)
	fresh := &Entry{Plan: "badger", Team: "repo-ci", Unit: "current-work",
		Started: time.Date(2026, 8, 11, 0, 8, 0, 0, time.UTC), Finished: &freshFinished}
	if err := stale.Write(regDir); err != nil {
		t.Fatal(err)
	}
	if err := fresh.Write(regDir); err != nil {
		t.Fatal(err)
	}

	rows, err := BuildRowsFromFilesystem("badger", home, nil)
	if err != nil {
		t.Fatal(err)
	}

	var dupDetect, repoCI *Row
	for i := range rows {
		switch rows[i].key {
		case "dup-detect":
			dupDetect = &rows[i]
		case "repo-ci":
			repoCI = &rows[i]
		}
	}
	if dupDetect != nil {
		t.Errorf("dup-detect row present with %d span(s) — a registry entry from an earlier, unrelated \"badger\" run (started 2026-08-05, before this plan's declared 2026-08-10 start) leaked into this run's picture", len(dupDetect.spans))
	}
	if repoCI == nil || len(repoCI.spans) != 1 {
		t.Fatalf("repo-ci row missing or wrong span count, got %+v", repoCI)
	}
}
