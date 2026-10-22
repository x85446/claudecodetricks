package iterrun

import (
	"fmt"
	"html"
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

// TestBuildRowsFromHookEventsPairsAcrossMissingPostPlanTag reproduces the
// bug found live: PreToolUse reliably gets cwd from Claude Code, so
// resolvePlanTeam reliably tags the pre event with Plan/Team — but a
// PostToolUse hook payload often doesn't carry cwd at all, so its Plan
// comes back empty and never gets written. The OLD code filtered pre AND
// post independently by Plan before ever pairing them, so a post missing
// that tag got dropped before pairing — its perfectly real, already-
// completed pre looked permanently open forever. The fix pairs by
// tool_use_id first and classifies the span using only the pre's tag.
func TestBuildRowsFromHookEventsPairsAcrossMissingPostPlanTag(t *testing.T) {
	base := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	const proj = "/Users/travis/workspace/x85446/financeSheets/personaldb"

	events := []Event{
		// A coordinator call whose post never got a Plan/CWD tag at all —
		// this is the exact real-world shape found live.
		{TS: base, Hook: "pre", ToolName: "Bash", ToolUseID: "toolu_1", Plan: "badger", CWD: proj},
		{TS: base.Add(5 * time.Second), Hook: "post", ToolName: "Bash", ToolUseID: "toolu_1"},

		// A second call whose post DOES carry the tag — must still work
		// the same way as before (no regression on the common case).
		{TS: base.Add(10 * time.Second), Hook: "pre", ToolName: "Bash", ToolUseID: "toolu_2", Plan: "badger", CWD: proj},
		{TS: base.Add(15 * time.Second), Hook: "post", ToolName: "Bash", ToolUseID: "toolu_2", Plan: "badger", CWD: proj},
	}

	rows := BuildRowsFromHookEvents(events, nil, "badger", proj, time.Time{})
	if len(rows) != 1 {
		t.Fatalf("rows = %d, want 1 (coordinator)", len(rows))
	}
	if len(rows[0].spans) != 2 {
		t.Fatalf("coordinator spans = %d, want 2 — the post missing its Plan tag must still pair with its pre", len(rows[0].spans))
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

// TestParsePlanStartedRespectsUTCSuffix reproduces a real bug found live
// while verifying the "running for" fix on a genuinely-executing plan: the
// old regex captured only the digit groups and silently discarded a
// trailing "Z" — exactly what /iterate's and /iterate-planner's own
// `date -u +%Y-%m-%dT%H:%M:%SZ` produces — then fed those bare digits to
// time.ParseInLocation(..., time.Local), reinterpreting a UTC clock
// reading as already-local wall-clock time. That doesn't just mislabel the
// displayed timestamp, it silently shifts the parsed INSTANT by the
// machine's UTC offset. A "Z"-suffixed value must parse to the exact same
// instant as the equivalent explicit UTC time, not to that value's digits
// read as local.
func TestParsePlanStartedRespectsUTCSuffix(t *testing.T) {
	got, ok := parsePlanStarted("2026-08-11T13:59:24Z (planned)")
	if !ok {
		t.Fatal("parsePlanStarted returned ok=false for a valid Z-suffixed timestamp")
	}
	want := time.Date(2026, 8, 11, 13, 59, 24, 0, time.UTC)
	if !got.Equal(want) {
		t.Errorf("parsePlanStarted(%q) = %s, want the exact instant %s (13:59:24 UTC, not 13:59:24 local)",
			"2026-08-11T13:59:24Z (planned)", got, want)
	}
}

// TestBuildRowsFromArchivePreservesGoalAndSteps reproduces the bug found
// live: once /iterate archives a finished plan (moves it out of plans/ to
// archive/<timestamp>-<name>-done.md per its own SKILL.md), its own
// dashboard page kept resolving — but with nothing left at the old plans/
// path, teams/steps/validations all came back empty: no Goal, no
// Requirements burndown, no team status, while registry/hook data for
// that plan name (permanent and global, unaffected by archiving) still
// got pulled in unbounded — a large ungrouped dump instead of the plan's
// real structure. BuildRowsFromArchive must read the SAME Teams/Steps/
// Validation detail a live plan gets, just from the archived file and its
// sibling "<file>.teams/" dir instead of plans/<name>.teams/.
func TestBuildRowsFromArchivePreservesGoalAndSteps(t *testing.T) {
	home := t.TempDir()
	archiveDir := filepath.Join(home, ".claude", "iterate", "archive")
	if err := os.MkdirAll(archiveDir, 0o755); err != nil {
		t.Fatal(err)
	}
	archiveFile := "20260812T012354Z-aardvark-done.md"
	planMD := "# Plan\n\nname: aardvark\nStarted: 2026-08-11T13:59:24Z (planned)\nphase: complete\nteamed: true\n\n" +
		"## Goal\nShip the thing.\n\n" +
		"## Steps\n1. build it\n\n" +
		"## Validation\n1. it builds\n\n" +
		"## Teams\n\n| Team | Steps | Focus | Depends on | Agent | Status |\n" +
		"|---|---|---|---|---|---|\n| audio-filters | 1 | build | — | agent | done |\n"
	if err := os.WriteFile(filepath.Join(archiveDir, archiveFile), []byte(planMD), 0o644); err != nil {
		t.Fatal(err)
	}

	teamsDir := filepath.Join(archiveDir, "20260812T012354Z-aardvark-done.teams")
	if err := os.MkdirAll(teamsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	logContent := "some progress\nTEAM DONE: shipped\n##ITERATE-VALIDATION## {\"step\":1,\"status\":\"met\",\"note\":\"green\"}\n"
	if err := os.WriteFile(filepath.Join(teamsDir, "audio-filters.log.md"), []byte(logContent), 0o644); err != nil {
		t.Fatal(err)
	}

	rows, err := BuildRowsFromArchive(archiveFile, "aardvark", home, nil)
	if err != nil {
		t.Fatal(err)
	}

	var team *Row
	for i := range rows {
		if rows[i].key == "audio-filters" {
			team = &rows[i]
		}
	}
	if team == nil {
		t.Fatalf("audio-filters row missing entirely; rows: %+v", rows)
	}
	if len(team.steps) != 1 || team.steps[0].Step != "build it" {
		t.Errorf("audio-filters.steps = %+v, want the plan's own Step 1 detail read from the archived file", team.steps)
	}
	if team.status != "done" {
		t.Errorf("audio-filters.status = %q, want %q from the archived Teams table", team.status, "done")
	}
}

// TestBuildRowsFromFilesystemAttributesUnassignedStepsToCoordinator
// reproduces a real gap found live: a plan drafted via /iterate-planner
// but never teamified (teamed: false, no ## Teams table) had real Steps/
// Validation detail and real per-step progress in its own Status/Log
// ("step 1 DONE: ...", "steps 2-3 DONE: ...", "step 5 PARTIAL: ..." — the
// exact free-text shape a live flat-plan coordinator actually writes,
// since there's no per-team log file for it to write a structured
// ##ITERATE-VALIDATION## marker into), yet showed no Requirements
// burndown chart at all — every step number was invisible because
// nothing ever attached them to any row; that only ever happened inside
// the Teams-table loop, and teams is empty on a flat plan. Confirms the
// coordinator row now carries every unassigned step, correctly shaded by
// its own inline status reports, with an untouched step correctly
// inferred as the one currently being worked.
func TestBuildRowsFromFilesystemAttributesUnassignedStepsToCoordinator(t *testing.T) {
	home := t.TempDir()
	plansDir := filepath.Join(home, ".claude", "iterate", "plans")
	if err := os.MkdirAll(plansDir, 0o755); err != nil {
		t.Fatal(err)
	}
	planMD := "# Plan\n\nname: bison\nStarted: 2026-08-12T02:54:00Z\nExecuting: 2026-08-12T02:54:00Z\nphase: executing\nteamed: false\n\n" +
		"## Steps\n1. back up state\n2. create bridge\n3. confirm host survived\n\n" +
		"## Validation\n1. backup file exists\n2. bridge stanza present\n3. host reachable\n\n" +
		"## Status / Log\n" +
		"- 2026-08-12T02:55Z step 1 DONE: backed up.\n" +
		"- 2026-08-12T03:00Z steps 2-3 DONE: bridge up, host survived.\n"
	if err := os.WriteFile(filepath.Join(plansDir, "bison.md"), []byte(planMD), 0o644); err != nil {
		t.Fatal(err)
	}

	// Coordinator activity so it reads as currently running — otherwise
	// there'd be no row at all (BuildRowsFromFilesystem only creates rows
	// from team-log files / registry entries / the Teams table; a flat
	// plan's coordinator needs SOME recorded activity to exist as a row).
	regDir := RegistryDir(home)
	entry := &Entry{Plan: "bison", Team: "", Unit: "step1",
		Started: time.Date(2026, 8, 12, 2, 55, 0, 0, time.UTC)}
	if err := entry.Write(regDir); err != nil {
		t.Fatal(err)
	}

	rows, err := BuildRowsFromFilesystem("bison", home, nil)
	if err != nil {
		t.Fatal(err)
	}

	var coord *Row
	for i := range rows {
		if rows[i].key == "" {
			coord = &rows[i]
		}
	}
	if coord == nil {
		t.Fatalf("no coordinator row at all; rows: %+v", rows)
	}
	if len(coord.steps) != 3 {
		t.Fatalf("coordinator.steps = %d, want 3 (all of them unassigned on a flat plan); got: %+v", len(coord.steps), coord.steps)
	}
	byNum := map[int]StepDetail{}
	for _, sd := range coord.steps {
		byNum[sd.Num] = sd
	}
	if byNum[1].VStatus != "met" {
		t.Errorf("step 1 VStatus = %q, want %q (from \"step 1 DONE\")", byNum[1].VStatus, "met")
	}
	if byNum[2].VStatus != "met" || byNum[3].VStatus != "met" {
		t.Errorf("steps 2-3 VStatus = %q/%q, want both %q (from \"steps 2-3 DONE\")", byNum[2].VStatus, byNum[3].VStatus, "met")
	}
}

// TestParseCoordinatorStepStatusRangesAndCase confirms the free-text
// parser handles both a single-step and a range report, is
// case-insensitive (a real log mixed "DONE" and "partial" in the same
// file), and folds "blocked" into the same not-met bucket
// ##ITERATE-VALIDATION## already uses.
func TestParseCoordinatorStepStatusRangesAndCase(t *testing.T) {
	log := "- step 1 DONE: backed up.\n" +
		"- steps 2-3 DONE: bridge up.\n" +
		"- step 5 PARTIAL: windows only.\n" +
		"- step 6 partial: reachable.\n" +
		"- step 9 blocked: no access.\n"
	marks := parseCoordinatorStepStatus(log)
	want := map[int]string{1: "met", 2: "met", 3: "met", 5: "partial", 6: "partial", 9: "not-met"}
	for n, wantStatus := range want {
		if got := marks[n].status; got != wantStatus {
			t.Errorf("step %d status = %q, want %q", n, got, wantStatus)
		}
	}
}

// TestParseCoordinatorStepStatusCapturesNote reproduces a real gap found
// live: the descriptive text after the status word ("Windows VM on
// bridge...") was being read out of the coordinator's own log and then
// silently discarded — a flat plan's unassigned steps never got the "why"
// a teamed plan's own ##ITERATE-VALIDATION## notes already provide.
func TestParseCoordinatorStepStatusCapturesNote(t *testing.T) {
	log := "- 2026-08-12T03:00Z step 5 PARTIAL: Windows VM on bridge (192.168.144.184). macos-vm deferred.\n"
	marks := parseCoordinatorStepStatus(log)
	want := "Windows VM on bridge (192.168.144.184). macos-vm deferred."
	if got := marks[5].note; got != want {
		t.Errorf("step 5 note = %q, want %q", got, want)
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

// TestOrderTeamRowsNestsByDependency reproduces the reported bug: with
// plain start-time sorting, "windows-port" (which depends on "winvm") was
// rendering sandwiched between "gui-macos2" and "gui-windows" — two teams
// it has no relationship to — purely because of when its own activity
// happened to start. This asserts the real fix: a dependent team appears
// immediately after its primary dependency, and unrelated/orphan rows
// (no Depends-on match — e.g. a dispatch name not in the Teams table)
// stay at the root level instead of interleaving with someone else's chain.
func TestOrderTeamRowsNestsByDependency(t *testing.T) {
	base := time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC)
	mkRow := func(key string, startOffsetMin int, dependsOn ...string) Row {
		return Row{
			key: key, label: key, dependsOn: dependsOn,
			spans: []span{{start: base.Add(time.Duration(startOffsetMin) * time.Minute), end: base.Add(time.Duration(startOffsetMin+1) * time.Minute)}},
		}
	}

	rows := []Row{
		mkRow("gui-macos2", 50),               // orphan — no Teams-table entry, no dependsOn
		mkRow("windows-port", 10, "winvm"),    // depends on winvm
		mkRow("winvm", 0),                     // root
		mkRow("gui-windows", 60),              // orphan
		mkRow("gui", 40, "core-abi", "winvm"), // depends on two — primary is the FIRST, core-abi
		mkRow("core-abi", 5),                  // root, starts after winvm
	}

	ordered := orderTeamRows(rows)
	pos := map[string]int{}
	for i, r := range ordered {
		pos[r.key] = i
	}

	if pos["windows-port"] != pos["winvm"]+1 {
		t.Errorf("windows-port at %d, want immediately after winvm (at %d)", pos["windows-port"], pos["winvm"])
	}
	if pos["gui"] != pos["core-abi"]+1 {
		t.Errorf("gui at %d, want immediately after its primary dependency core-abi (at %d), not winvm", pos["gui"], pos["core-abi"])
	}
	// winvm started before core-abi, so as siblings at the root, winvm's
	// whole chain (winvm, windows-port) should come first.
	if pos["winvm"] > pos["core-abi"] {
		t.Errorf("winvm (started first) should sort before core-abi at the root level: winvm=%d core-abi=%d", pos["winvm"], pos["core-abi"])
	}
	if len(ordered) != len(rows) {
		t.Fatalf("orderTeamRows dropped rows: got %d, want %d", len(ordered), len(rows))
	}
}

// TestBuildRowsFromFilesystemMatchesOrphanDispatchesToParent reproduces
// the exact scenario reported live: a Teams-table row ("gui", owning
// steps 25-26 in this test) fans out into two concurrently-dispatched
// sub-agents with their own log files ("app-macos", "gui-windows") that
// don't literally match any Teams-table row name. Without a name match
// they used to show as ungrouped root rows defaulting to "running" status
// forever. This asserts both fixes: they nest under "gui" (matched via
// which step numbers their own ##ITERATE-VALIDATION## markers report
// against), and a real "done" status is read from their own TEAM DONE
// terminal line instead of defaulting to running just because they have
// recorded activity.
func TestBuildRowsFromFilesystemMatchesOrphanDispatchesToParent(t *testing.T) {
	home := t.TempDir()
	plansDir := filepath.Join(home, ".claude", "iterate", "plans")
	teamsDir := filepath.Join(plansDir, "finch.teams")
	if err := os.MkdirAll(teamsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	planMD := "# Plan\n\nname: finch\nStarted: 2026-08-01\nphase: executing\n\n" +
		"## Teams\n\n| Team | Steps | Focus | Depends on | Agent | Status |\n" +
		"|---|---|---|---|---|---|\n| gui | 25,26 | both platforms | — | agent | done |\n"
	if err := os.WriteFile(filepath.Join(plansDir, "finch.md"), []byte(planMD), 0o644); err != nil {
		t.Fatal(err)
	}

	write := func(name, content string) {
		if err := os.WriteFile(filepath.Join(teamsDir, name+".log.md"), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("app-macos", "starting work\n"+
		`##ITERATE-VALIDATION## {"step":25,"status":"met","note":"macOS half done"}`+"\n"+
		"TEAM DONE: macOS half complete\n")
	write("gui-windows", "starting work\n"+
		`##ITERATE-VALIDATION## {"step":26,"status":"met","note":"Windows half done"}`+"\n"+
		"still going, no terminal line yet\n")

	rows, err := BuildRowsFromFilesystem("finch", home, nil)
	if err != nil {
		t.Fatal(err)
	}

	var appMacos, guiWindows *Row
	for i := range rows {
		switch rows[i].key {
		case "app-macos":
			appMacos = &rows[i]
		case "gui-windows":
			guiWindows = &rows[i]
		}
	}
	if appMacos == nil || guiWindows == nil {
		t.Fatalf("missing rows: app-macos=%v gui-windows=%v", appMacos, guiWindows)
	}

	if len(appMacos.dependsOn) != 1 || appMacos.dependsOn[0] != "gui" {
		t.Errorf("app-macos.dependsOn = %v, want [gui] (matched via step 25 ownership)", appMacos.dependsOn)
	}
	if len(guiWindows.dependsOn) != 1 || guiWindows.dependsOn[0] != "gui" {
		t.Errorf("gui-windows.dependsOn = %v, want [gui] (matched via step 26 ownership)", guiWindows.dependsOn)
	}
	// depth has to be set explicitly for these — teamDepths(teams) never
	// sees them, since they're not literal Teams-table rows.
	wantDepth := 1 // gui is a root (depth 0) here, so its orphan children are depth 1
	if appMacos.depth != wantDepth {
		t.Errorf("app-macos.depth = %d, want %d (one level under gui)", appMacos.depth, wantDepth)
	}
	if guiWindows.depth != wantDepth {
		t.Errorf("gui-windows.depth = %d, want %d (one level under gui)", guiWindows.depth, wantDepth)
	}

	if appMacos.status != "done" {
		t.Errorf("app-macos.status = %q, want %q (has a TEAM DONE terminal line)", appMacos.status, "done")
	}
	if guiWindows.status == "done" {
		t.Errorf("gui-windows.status = %q, want anything but done — it has no terminal line", guiWindows.status)
	}
}

// TestStatuslessActiveLaneCountsAsActive reproduces the bug reported live:
// a lane with recorded activity but no status field (an orphan dispatch
// before its terminal line arrives) fell through every bucket of the old
// done/running/queued cards, so a plan with work in flight read as fully
// accounted for. The progress section's lane counts carry it as active.
func TestStatuslessActiveLaneCountsAsActive(t *testing.T) {
	base := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	rows := []Row{
		{key: "", label: "coordinator", spans: []span{{start: base, end: base.Add(time.Minute)}}},
		{key: "repo-ci", label: "repo-ci", status: "done", spans: []span{{start: base, end: base.Add(time.Minute)}}},
		{key: "dispatch", label: "dispatch", spans: []span{{start: base, end: base.Add(time.Minute)}}}, // status intentionally ""
	}
	in := ReportInput{Plan: PlanSummary{Name: "badger"}, Rows: rows, Now: base.Add(time.Hour)}
	lanes := BuildPlanReport(in).Progress.Lanes
	if lanes.Completed != 1 || lanes.Active < 1 || lanes.Total != 3 {
		t.Errorf("lanes = %+v, want 1 completed and the status-less dispatch counted active among 3", lanes)
	}
	out := RenderPlanPage(in, PlanPageOptions{})
	if !strings.Contains(out, `<dt>Active</dt><dd class="pg-n">`) || strings.Contains(out, `class="stats"`) {
		t.Errorf("the page must show lane counts in the progress section and no unlabelled stat cards; output:\n%s", out)
	}
}

// planElapsed returns the page's one elapsed line.
func planElapsed(t *testing.T, page string) string {
	t.Helper()
	i := strings.Index(page, `<p class="plan-elapsed" id="plan-elapsed">`)
	if i < 0 {
		t.Fatalf("page has no elapsed line:\n%s", page)
	}
	return page[i : i+strings.Index(page[i:], "</p>")]
}

// TestElapsedPrefersExecutingOverStarted reproduces the bug reported live: a
// plan drafted at 13:59 and run from 16:32 showed "running for 2h36m" the
// instant /iterate started, elapsed drafting time rather than execution
// time. The page's one elapsed figure starts at Executing:.
func TestElapsedPrefersExecutingOverStarted(t *testing.T) {
	drafted := time.Date(2026, 8, 11, 13, 59, 24, 0, time.UTC)
	executing := time.Date(2026, 8, 11, 16, 32, 55, 0, time.UTC)
	rows := []Row{
		{key: "", label: "coordinator", spans: []span{{start: executing, end: executing.Add(3 * time.Minute)}}},
	}
	plan := PlanSummary{Name: "aardvark", Started: drafted.Format(time.RFC3339), Executing: executing.Format(time.RFC3339)}
	line := planElapsed(t, RenderPlanPage(ReportInput{Plan: plan, Rows: rows, Now: executing.Add(10 * time.Minute)}, PlanPageOptions{}))
	if !strings.Contains(line, `datetime="`+executing.Format(time.RFC3339)+`"`) || !strings.Contains(line, "<strong>10m 0s</strong>") {
		t.Errorf("elapsed should run from Executing: (%s) for 10m 0s, not from Started:; got %s", executing, line)
	}
	if strings.Contains(line, drafted.Format(time.RFC3339)) {
		t.Errorf("elapsed starts at drafting time; got %s", line)
	}
}

// TestElapsedFallsBackToEarliestActivityWhenExecutingMissing covers plans
// written before the Executing: field existed: the first observed tool call
// beats a stale Started: drafting timestamp.
func TestElapsedFallsBackToEarliestActivityWhenExecutingMissing(t *testing.T) {
	drafted := time.Date(2026, 8, 11, 13, 59, 24, 0, time.UTC)
	activity := time.Date(2026, 8, 11, 16, 32, 55, 0, time.UTC)
	rows := []Row{
		{key: "", label: "coordinator", spans: []span{{start: activity, end: activity.Add(3 * time.Minute), tool: "Bash"}}},
	}
	plan := PlanSummary{Name: "aardvark", Started: drafted.Format("2006-01-02 15:04:05") + " (planned)"}
	line := planElapsed(t, RenderPlanPage(ReportInput{Plan: plan, Rows: rows, Now: activity.Add(time.Hour)}, PlanPageOptions{}))
	if !strings.Contains(line, `datetime="`+activity.Format(time.RFC3339)+`"`) {
		t.Errorf("elapsed should start at the earliest recorded tool call (%s) when Executing: is absent; got %s", activity, line)
	}
}

// TestElapsedHandlesZeroConfirmedActivity reproduces the bug shown live: a
// flat plan with a correct Executing: and no hook or registry data showed
// "0000-12-31 18:09:24" and "Running for 0s". The figure runs from
// Executing: to the report time and the page never prints a zero date.
func TestElapsedHandlesZeroConfirmedActivity(t *testing.T) {
	now := time.Date(2026, 9, 15, 12, 0, 0, 0, time.UTC)
	rows := []Row{
		{key: "", label: "coordinator", steps: []StepDetail{{Num: 1, Step: "x", Validation: "y"}}}, // no spans at all
	}
	plan := PlanSummary{Name: "bison", Executing: now.Add(-90 * time.Minute).Format(time.RFC3339)}
	out := RenderPlanPage(ReportInput{Plan: plan, Rows: rows, Now: now}, PlanPageOptions{})
	if line := planElapsed(t, out); !strings.Contains(line, "<strong>1h 30m</strong>") || !strings.Contains(line, "still open at") {
		t.Errorf("elapsed should read 1h 30m, still open; got %s", line)
	}
	if strings.Contains(out, "0000-") {
		t.Errorf("zero-value timestamp leaked into the page; output:\n%s", out)
	}
	if strings.Contains(out, `class="axis">`) {
		t.Errorf("axis line should be omitted with zero confirmed activity; output:\n%s", out)
	}
}

// TestElapsedPrefersFinishedOverActivityForArchivedPlans confirms an
// archived plan's elapsed figure ends at its declared Finished:, not at the
// latest confirmed activity, which can end long before the real finish.
func TestElapsedPrefersFinishedOverActivityForArchivedPlans(t *testing.T) {
	executing := time.Date(2026, 8, 12, 2, 54, 23, 0, time.UTC)
	staleActivity := time.Date(2026, 8, 12, 2, 55, 0, 0, time.UTC)
	finished := time.Date(2026, 8, 12, 5, 0, 0, 0, time.UTC)
	rows := []Row{
		{key: "", label: "coordinator", spans: []span{{start: staleActivity, end: staleActivity.Add(time.Second)}}},
	}
	plan := PlanSummary{Name: "bison", Archived: true, Executing: executing.Format(time.RFC3339), Finished: finished.Format(time.RFC3339)}
	line := planElapsed(t, RenderPlanPage(ReportInput{Plan: plan, Rows: rows, Now: finished.Add(48 * time.Hour)}, PlanPageOptions{}))
	if !strings.Contains(line, "<strong>2h 6m</strong>") || !strings.Contains(line, `datetime="`+finished.Format(time.RFC3339)+`"`) {
		t.Errorf("elapsed should be Executing: to Finished: (2h 6m), ending at %s; got %s", finished, line)
	}
	if strings.Contains(line, "still open") {
		t.Errorf("an archived plan's clock must not read open; got %s", line)
	}
}

// TestWriteGanttRowMarksEveryInProgressRowsLastSpanOpen reproduces the bug
// reported live: only the row whose last span happened to end at the
// plan-wide maxT got the "still running" stripe — every other genuinely
// in-progress row's last bar rendered as plain "confirmed activity" and
// looked finished. The open styling belongs to the last span of EACH
// in-progress row, independent of the others.
func TestWriteGanttRowMarksEveryInProgressRowsLastSpanOpen(t *testing.T) {
	base := time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)
	rows := []Row{
		{key: "prd", label: "prd", status: "in-progress", spans: []span{{start: base, end: base.Add(3 * time.Minute)}}},
		{key: "audio-filters", label: "audio-filters", status: "in-progress", spans: []span{{start: base, end: base.Add(12 * time.Minute)}}}, // ends latest — this used to be the only row ever marked open
	}
	out := RenderTimelineHTML(rows, PlanSummary{Name: "badger"}, "")

	if got := strings.Count(out, `class="bar bar-open"`); got != 2 {
		t.Errorf(`expected 2 rows with class="bar bar-open" (both in-progress rows' last span), got %d; output:\n%s`, got, out)
	}
}

// gridCells returns the requirement grid's cells as number -> class.
func gridCells(t *testing.T, page string) map[string]string {
	t.Helper()
	i := strings.Index(page, `<ul class="bd-grid"`)
	if i < 0 {
		return nil
	}
	grid := page[i : i+strings.Index(page[i:], "</ul>")]
	cells := map[string]string{}
	for _, part := range strings.Split(grid, `<a class="bd-cell `)[1:] {
		cls := part[:strings.Index(part, `"`)]
		num := part[strings.Index(part, `">`)+2 : strings.Index(part, "</a>")]
		cells[num] = cls
		if !strings.Contains(part, `href="#evidence-req-`+num+`"`) {
			t.Errorf("cell %s does not link to its evidence: %s", num, part)
		}
	}
	return cells
}

// TestRequirementGridFollowsTheProgressModel confirms every requirement cell
// is a link to its evidence, coloured by the progress section's state for
// that requirement, so the grid, the lanes and the summary never disagree.
func TestRequirementGridFollowsTheProgressModel(t *testing.T) {
	base := time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)
	rows := []Row{
		{key: "engine", label: "engine", status: "done", spans: []span{{start: base, end: base.Add(time.Minute)}},
			steps: []StepDetail{{Num: 1, Step: "build the engine", Validation: "engine builds clean", VStatus: "met"}}},
		{key: "store", label: "store", status: "done", spans: []span{{start: base, end: base.Add(time.Minute)}},
			steps: []StepDetail{{Num: 2, Step: "hard blocker", Validation: "x", VStatus: "not-met", VNote: "no access"}}},
		{key: "app-macos", label: "app-macos", status: "in-progress", dependsOn: []string{"engine"}, spans: []span{{start: base, end: base.Add(time.Minute)}},
			steps: []StepDetail{
				{Num: 3, Step: "wire the UI", Validation: "UI renders", VStatus: "partial"},
				{Num: 4, Step: "ship installer", Validation: "installer runs"},
			}},
	}
	in := ReportInput{Plan: PlanSummary{Name: "badger"}, Rows: rows, Now: base.Add(time.Hour)}
	rep := BuildPlanReport(in)
	out := RenderPlanPage(in, PlanPageOptions{})
	cells := gridCells(t, out)
	if len(cells) != 4 {
		t.Fatalf("grid cells = %v, want 4; output:\n%s", cells, out)
	}
	want := map[string]string{"1": "complete", "2": "blocked", "3": "in-progress", "4": "not-started"}
	for _, r := range rep.Progress.RequirementItems {
		n := fmt.Sprint(r.Number)
		if cells[n] != "bd-"+r.State {
			t.Errorf("cell %s class = %q, progress state = %q", n, cells[n], r.State)
		}
		if r.State != want[n] {
			t.Errorf("requirement %s state = %q, want %q", n, r.State, want[n])
		}
		if !strings.Contains(out, `id="`+r.EvidenceAnchor+`"`) {
			t.Errorf("requirement %s's evidence anchor %q is missing from the page", n, r.EvidenceAnchor)
		}
	}
	if strings.Contains(out, "onclick") || strings.Contains(out, "bdShow") {
		t.Error("the grid must be plain links, not click handlers")
	}
	if !strings.Contains(out, `aria-label="Requirement 2, Blocked, lane store"`) {
		t.Errorf("each cell names its requirement, state and lane; output:\n%s", out)
	}
}

// TestLaneListShowsFullDependsOnListNotJustPrimary reproduces a real bug: a
// team can depend on several others, but the old requirement detail walked
// only the single primary dependency used for indentation. The progress
// section's lane list carries the team's complete Depends-on list.
func TestLaneListShowsFullDependsOnListNotJustPrimary(t *testing.T) {
	base := time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)
	rows := []Row{
		{key: "audio-filters", label: "audio-filters", status: "done", spans: []span{{start: base, end: base.Add(time.Minute)}}},
		{key: "settings-surfaces", label: "settings-surfaces", status: "done", dependsOn: []string{"audio-filters"}, spans: []span{{start: base, end: base.Add(time.Minute)}}},
		{key: "tech-debt", label: "tech-debt", status: "in-progress", dependsOn: []string{"audio-filters", "settings-surfaces"}, spans: []span{{start: base, end: base.Add(time.Minute)}},
			steps: []StepDetail{{Num: 17, Step: "clean up", Validation: "clean"}}},
	}
	out := RenderTimelineHTML(rows, PlanSummary{Name: "badger"}, "")
	if !strings.Contains(out, "depends on audio-filters, settings-surfaces") {
		t.Errorf("tech-debt's lane should list BOTH of its dependencies; output:\n%s", out)
	}
}

// TestRequirementGridOmittedWhenNoStepDetail confirms a plan with no
// numbered requirements renders no grid rather than an empty one.
func TestRequirementGridOmittedWhenNoStepDetail(t *testing.T) {
	base := time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)
	rows := []Row{
		{key: "", label: "coordinator", spans: []span{{start: base, end: base.Add(time.Minute)}}},
	}
	out := RenderTimelineHTML(rows, PlanSummary{Name: "finch"}, "")
	if strings.Contains(out, `class="bd-grid"`) {
		t.Errorf("expected no requirement grid for a plan with no step detail; output:\n%s", out)
	}
	if !strings.Contains(out, "No numbered requirements were found") {
		t.Errorf("the progress section should say why there is no grid; output:\n%s", out)
	}
}

// TestTeamValidationNotesReachEvidenceNotAModal: a team's validation note is
// often several sentences, so the page shows it in full, selectable, under
// the requirement's evidence, and every gantt step links there by keyboard.
// Nothing on the page opens on a click handler.
func TestTeamValidationNotesReachEvidenceNotAModal(t *testing.T) {
	long := "partial: verify/test/conformance/round-trip/acceptance/verify-leak all exit 0; retention proved from the CLI (16 imports leave exactly 10 manifests) and in Rust at the default N=10 with a prior snapshot opened read-only before pre-edit balances."
	proj := writePlan(t, "ocelot", "# Iterate Task\n\nname: ocelot\nphase: executing\nExecuting: 2026-09-15T08:00:00Z\nteamed: true\n\n## Steps\n- [x] 1. Build the store.\n\n## Validation\n- [ ] 1. The store round-trips.\n\n## Teams\n| Team | Steps | Focus | Depends on | Agent | Status |\n|---|---|---|---|---|---|\n| store | 1 | Store | — | general-purpose | in-progress |\n")
	teams := filepath.Join(proj, ".claude", "iterate", "plans", "ocelot.teams")
	if err := os.MkdirAll(teams, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(teams, "store.log.md"), []byte("# store\n##ITERATE-VALIDATION## {\"step\":1,\"status\":\"partial\",\"note\":\""+long+"\"}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	rows, err := BuildRowsFromFilesystem("ocelot", proj, nil)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := GetPlanSummary(proj, "ocelot")
	if err != nil {
		t.Fatal(err)
	}
	out := RenderPlanPage(ReportInput{Plan: plan, Rows: rows, Now: time.Date(2026, 9, 15, 9, 0, 0, 0, time.UTC)}, PlanPageOptions{})
	ev := out[strings.Index(out, `id="evidence-req-1"`):]
	if !strings.Contains(ev[:strings.Index(ev, "</li></ol>")], html.EscapeString(long)) {
		t.Errorf("requirement 1's evidence should carry the team's full note")
	}
	if !strings.Contains(out, `<a class="step-ev" href="#evidence-req-1">Evidence for requirement 1</a>`) {
		t.Errorf("the gantt step should link to its evidence; output:\n%s", out)
	}
	for _, gone := range []string{"onclick", "note-modal", "showNote"} {
		if strings.Contains(out, gone) {
			t.Errorf("page still carries %q", gone)
		}
	}
}

// TestPlanDetailShowsOnlyItsOwnHarnessLaunchCommand confirms the plan
// detail page never prints a command the reader's own harness doesn't
// have: a claude-code plan shows only "/iterate <name>", a codex plan
// shows only "$iterate <name>", and an unknown plan shows both, each
// labelled with its harness rather than guessing.
func TestPlanDetailShowsOnlyItsOwnHarnessLaunchCommand(t *testing.T) {
	cases := []struct {
		harness  string
		wantHas  []string
		wantMiss []string
	}{
		{"claude-code", []string{"/iterate cheetah"}, []string{"$iterate cheetah"}},
		{"codex", []string{"$iterate cheetah"}, []string{"/iterate cheetah"}},
		{"unknown", []string{"/iterate cheetah", "$iterate cheetah", "claude-code", "codex"}, nil},
	}
	for _, c := range cases {
		plan := PlanSummary{Name: "cheetah", Harness: c.harness}
		out := RenderTimelineHTML(nil, plan, "")
		for _, want := range c.wantHas {
			if !strings.Contains(out, want) {
				t.Errorf("harness=%s: expected output to contain %q; output:\n%s", c.harness, want, out)
			}
		}
		for _, miss := range c.wantMiss {
			if strings.Contains(out, miss) {
				t.Errorf("harness=%s: expected output to NOT contain %q (wrong harness's command); output:\n%s", c.harness, miss, out)
			}
		}
	}
}

// TestPlanDetailShowsVersionFromEitherMarker confirms the plan detail
// page displays PlanSummary.Version regardless of which of the two
// marker shapes (Claude Code frontmatter vs. Codex body line) actually
// produced it — by the time it reaches PlanSummary.Version the parser
// has already reconciled that; the page just needs to not render it
// blank.
func TestPlanDetailShowsVersionFromEitherMarker(t *testing.T) {
	plan := PlanSummary{Name: "cheetah", Harness: "codex", Version: "5.1.0"}
	out := RenderTimelineHTML(nil, plan, "")
	if !strings.Contains(out, "iterate family 5.1.0") {
		t.Errorf("expected the plan detail page to show the resolved version; output:\n%s", out)
	}
}

func TestAxisTicksLandOnWallClockBoundaries(t *testing.T) {
	loc := time.FixedZone("TST", -7*3600)
	// The span from the live dashboard: 03:35:44 → 11:51:31, 8h16m.
	minT := time.Date(2026, 9, 15, 3, 35, 44, 0, loc)
	maxT := time.Date(2026, 9, 15, 11, 51, 31, 0, loc)
	ticks, interval := axisTicks(minT, maxT)
	if interval != time.Hour {
		t.Fatalf("interval = %v, want 1h for an 8h16m span", interval)
	}
	if len(ticks) != 8 {
		t.Fatalf("len(ticks) = %d, want 8 (04:00 through 11:00): %v", len(ticks), ticks)
	}
	for _, tk := range ticks {
		if tk.Minute() != 0 || tk.Second() != 0 {
			t.Errorf("tick %v is not on an hour boundary", tk)
		}
		if tk.Before(minT) || tk.After(maxT) {
			t.Errorf("tick %v outside the span", tk)
		}
	}
	if got := ticks[0].Hour(); got != 4 {
		t.Errorf("first tick hour = %d, want 4", got)
	}
	if got := tickLabel(ticks[0], interval); got != "04" {
		t.Errorf("tickLabel = %q, want \"04\"", got)
	}
}

func TestAxisTicksUseMinutesForShortSpansAndDatesForLongOnes(t *testing.T) {
	loc := time.FixedZone("TST", 0)
	short := time.Date(2026, 9, 15, 10, 2, 0, 0, loc)
	ticks, interval := axisTicks(short, short.Add(24*time.Minute))
	if interval != 5*time.Minute {
		t.Fatalf("interval = %v, want 5m for a 24m span", interval)
	}
	if got := tickLabel(ticks[0], interval); got != "10:05" {
		t.Errorf("short-span label = %q, want \"10:05\"", got)
	}

	// 5 days lands on 12h marks (10 of them) — dates at midnight, "12" at noon.
	midStart := time.Date(2026, 9, 10, 8, 0, 0, 0, loc)
	mticks, minterval := axisTicks(midStart, midStart.Add(120*time.Hour))
	if minterval != 12*time.Hour {
		t.Fatalf("interval = %v, want 12h for a 5-day span", minterval)
	}
	if got := tickLabel(mticks[0], minterval); got != "12" {
		t.Errorf("12h-scale label = %q, want \"12\" (noon)", got)
	}

	// Past what 12h marks can cover, the axis goes daily and labels dates.
	longStart := time.Date(2026, 9, 10, 8, 0, 0, 0, loc)
	lticks, linterval := axisTicks(longStart, longStart.Add(15*24*time.Hour))
	if linterval != 24*time.Hour {
		t.Fatalf("interval = %v, want 24h for a 15-day span", linterval)
	}
	if got := tickLabel(lticks[0], linterval); got != "Sep 11" {
		t.Errorf("long-span label = %q, want \"Sep 11\"", got)
	}
}

func TestAxisTicksSurviveASpringForwardTransition(t *testing.T) {
	loc, err := time.LoadLocation("America/Denver")
	if err != nil {
		t.Skipf("tzdata unavailable: %v", err)
	}
	// 2026-03-08: 02:00 MST jumps to 03:00 MDT. A run spanning it must still
	// label real local hours — the bug a naive t.Add(interval) loop produces
	// is every post-transition mark reading an hour early.
	minT := time.Date(2026, 3, 8, 0, 30, 0, 0, loc)
	maxT := time.Date(2026, 3, 8, 7, 30, 0, 0, loc)
	ticks, interval := axisTicks(minT, maxT)
	if interval != time.Hour {
		t.Fatalf("interval = %v, want 1h", interval)
	}
	for _, tk := range ticks {
		if tk.Minute() != 0 {
			t.Errorf("tick %v not on the hour", tk)
		}
	}
	var hours []int
	for _, tk := range ticks {
		hours = append(hours, tk.Hour())
	}
	// 01:00 exists, 02:00 does not, then 03:00-07:00.
	want := []int{1, 3, 4, 5, 6, 7}
	if len(hours) != len(want) {
		t.Fatalf("hours = %v, want %v", hours, want)
	}
	for i := range want {
		if hours[i] != want[i] {
			t.Fatalf("hours = %v, want %v", hours, want)
		}
	}
}

func TestRenderTimelineHTMLDrawsClockTicksAlignedToTheTrack(t *testing.T) {
	start := time.Now().Add(-4 * time.Hour)
	rows := []Row{{
		key:    "app",
		label:  "app",
		status: "done",
		spans:  []span{{start: start, end: start.Add(3 * time.Hour)}},
	}}
	out := RenderTimelineHTML(rows, PlanSummary{Name: "ticktest"}, "")
	if !strings.Contains(out, `class="axis-ticks"`) {
		t.Fatal("no tick axis rendered")
	}
	if !strings.Contains(out, `class="tick-lab"`) {
		t.Fatal("tick marks carry no labels")
	}
	// The axis must repeat the row geometry, or every mark is offset by the
	// width of the label column.
	if !strings.Contains(out, `class="ax-pad"`) || !strings.Contains(out, `class="ax-pad-r"`) {
		t.Error("tick axis is missing the label/duration column spacers")
	}
}

func TestBuildRowsFromHookEventsRecoversTheTeamFromAgentIDForOldEvents(t *testing.T) {
	// Every event written before resolvePlanTeam was fixed carries agent_id
	// but no team. The team must be recovered from the id, or a finished
	// plan's whole history reads as coordinator work and its teams look idle.
	base := time.Date(2026, 9, 15, 17, 0, 0, 0, time.UTC)
	ev := func(hook, tid, agent string, off time.Duration) Event {
		return Event{
			Hook: hook, ToolUseID: tid, AgentID: agent, Team: "", // never recorded
			Plan: "galago", CWD: "/p", TS: base.Add(off), ToolName: "Edit",
		}
	}
	events := []Event{
		ev("pre", "c1", "", 0), ev("post", "c1", "", time.Second),
		ev("pre", "r1", "agalago-reporting-f68431d4419554c9", time.Minute),
		ev("post", "r1", "agalago-reporting-f68431d4419554c9", time.Minute+time.Second),
		// a re-dispatch of the same team folds into the one lane
		ev("pre", "t1", "agalago-trees-2-ad83a94607dcf3c4", 2*time.Minute),
		ev("post", "t1", "agalago-trees-2-ad83a94607dcf3c4", 2*time.Minute+time.Second),
		ev("pre", "t2", "agalago-trees-4a81c2f3eaa2171b", 3*time.Minute),
		ev("post", "t2", "agalago-trees-4a81c2f3eaa2171b", 3*time.Minute+time.Second),
	}
	byKey := map[string]Row{}
	for _, r := range BuildRowsFromHookEvents(events, nil, "galago", "/p", time.Time{}) {
		byKey[r.key] = r
	}
	if got := len(byKey[""].spans); got != 1 {
		t.Errorf("coordinator has %d spans, want 1 — old team events are still leaking in", got)
	}
	if got := len(byKey["reporting"].spans); got != 1 {
		t.Errorf("reporting has %d spans, want 1", got)
	}
	if got := len(byKey["trees"].spans); got != 2 {
		t.Errorf("trees has %d spans, want 2 (both dispatches in one lane)", got)
	}
}

func TestBusyAndGapsUseTheUnionOfOverlappingSpans(t *testing.T) {
	base := time.Date(2026, 9, 15, 11, 0, 0, 0, time.UTC)
	at := func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) }
	// The real shape of a team row: one coarse log-file span covering the
	// whole 90 minutes, plus three tool calls inside it.
	spans := []span{
		{start: at(0), end: at(90), tool: "team-log"},
		{start: at(5), end: at(6), tool: "Bash"},
		{start: at(40), end: at(41), tool: "Edit"},
		{start: at(88), end: at(89), tool: "Bash"},
	}
	if got, want := busyDuration(spans), 90*time.Minute; got != want {
		t.Errorf("busy = %v, want %v — overlapping spans are being summed, not unioned", got, want)
	}
	if got := computeGaps(spans); len(got) != 0 {
		t.Errorf("gaps = %v, want none: a span the row already holds covers every minute", got)
	}

	// With no covering span, the holes are real and must still be reported.
	// The 19-minute hole is notable; the 30-second one is normal think-time.
	sparse := []span{
		{start: at(0), end: at(1)},
		{start: at(20), end: at(21)},
		{start: at(21).Add(30 * time.Second), end: at(22)},
	}
	if got, want := busyDuration(sparse), 2*time.Minute+30*time.Second; got != want {
		t.Errorf("busy = %v, want %v", got, want)
	}
	gaps := computeGaps(sparse)
	if len(gaps) != 1 {
		t.Fatalf("gaps = %v, want exactly the 19-minute hole", gaps)
	}
	if got, want := gaps[0].dur(), 19*time.Minute; got != want {
		t.Errorf("gap = %v, want %v", got, want)
	}
}

func TestCoarseLogSpanIsAFallbackNotASupplement(t *testing.T) {
	base := time.Date(2026, 9, 15, 4, 0, 0, 0, time.UTC)
	at := func(m int) time.Time { return base.Add(time.Duration(m) * time.Minute) }

	// A team with real per-call data: the log-file bounding box must not
	// count as work, or the six hours its model was exhausted read as busy.
	instrumented := []span{
		{start: at(0), end: at(420), tool: teamLogTool},
		{start: at(1), end: at(2), tool: "Bash"},
		{start: at(410), end: at(411), tool: "Edit"},
	}
	got := dropCoarseSpans(instrumented)
	if len(got) != 2 {
		t.Fatalf("kept %d spans, want the 2 real calls", len(got))
	}
	if d := busyDuration(got); d != 2*time.Minute {
		t.Errorf("busy = %v, want 2m — the bounding box is still counted as work", d)
	}
	gaps := computeGaps(got)
	if len(gaps) != 1 || gaps[0].dur() < SevereGap {
		t.Errorf("gaps = %v, want the real multi-hour outage reported", gaps)
	}

	// A team with nothing but its log file keeps that span — it is all the
	// evidence there is, and dropping it would erase the row entirely.
	bare := []span{{start: at(0), end: at(30), tool: teamLogTool}}
	if got := dropCoarseSpans(bare); len(got) != 1 {
		t.Errorf("an uninstrumented team lost its only span: %v", got)
	}
}
