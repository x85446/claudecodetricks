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

// TestRenderTimelineHTMLCountsStatuslessActiveRowsAsRunning reproduces the
// bug reported live: the coordinator row (and any orphan dispatch before
// its terminal line arrives) never gets a status field set at all, but the
// old stats loop only had three buckets — done/blocked, in-progress, and
// no-spans-at-all — so a status-less row WITH activity fell through all
// three, uncounted anywhere. A plan with real ongoing coordinator work
// could read as fully accounted for (e.g. "N done, 0 running") in the top
// summary while its own coordinator section showed hours of live activity.
func TestRenderTimelineHTMLCountsStatuslessActiveRowsAsRunning(t *testing.T) {
	base := time.Date(2026, 8, 11, 12, 0, 0, 0, time.UTC)
	rows := []Row{
		{key: "", label: "coordinator", spans: []span{{start: base, end: base.Add(time.Minute)}}}, // status intentionally "" — the coordinator never gets one
		{key: "repo-ci", label: "repo-ci", status: "done", spans: []span{{start: base, end: base.Add(time.Minute)}}},
	}
	out := RenderTimelineHTML(rows, PlanSummary{Name: "badger"}, "")

	if !strings.Contains(out, `<div class="n">1</div><div class="l">done</div>`) {
		t.Errorf("expected exactly 1 done in stats; output:\n%s", out)
	}
	if !strings.Contains(out, `<div class="n">1</div><div class="l">running</div>`) {
		t.Errorf("expected the status-less-but-active coordinator to count as 1 running, not fall through uncounted; output:\n%s", out)
	}
	if strings.Contains(out, `<div class="n">0</div><div class="l">running</div>`) {
		t.Errorf("running count is 0 — the coordinator's own activity isn't being counted")
	}
}

// TestRunboxPrefersExecutingOverStarted reproduces the bug reported live: a
// plan drafted via /iterate-planner at 13:59, not actually run until 16:32,
// showed "running for 2h36m" the instant /iterate was invoked — that read
// as elapsed drafting time, not elapsed execution time. Once Executing: is
// present, the runbox must anchor to it instead of Started:.
func TestRunboxPrefersExecutingOverStarted(t *testing.T) {
	drafted := time.Date(2026, 8, 11, 13, 59, 24, 0, time.Local)
	executing := time.Date(2026, 8, 11, 16, 32, 55, 0, time.Local)
	rows := []Row{
		{key: "coordinator", label: "coordinator", spans: []span{{start: executing, end: executing.Add(3 * time.Minute)}}},
	}
	plan := PlanSummary{
		Name:      "aardvark",
		Started:   drafted.Format("2006-01-02 15:04:05"),
		Executing: executing.Format("2006-01-02 15:04:05"),
	}
	out := RenderTimelineHTML(rows, plan, "")

	if !strings.Contains(out, "since "+executing.Format("2006-01-02 15:04:05")) {
		t.Errorf("runbox should anchor to Executing: (%s), not Started: (%s); output:\n%s",
			executing.Format("2006-01-02 15:04:05"), drafted.Format("2006-01-02 15:04:05"), out)
	}
	if strings.Contains(out, "2h36m") {
		t.Errorf("runbox shows elapsed drafting time (2h36m) instead of elapsed execution time; output:\n%s", out)
	}
}

// TestRunboxFallsBackToEarliestActivityWhenExecutingMissing covers plans
// written before the Executing: field existed — the runbox should prefer
// real recorded activity (minT) over a stale Started: drafting timestamp
// rather than reverting to the original bug.
func TestRunboxFallsBackToEarliestActivityWhenExecutingMissing(t *testing.T) {
	drafted := time.Date(2026, 8, 11, 13, 59, 24, 0, time.Local)
	activity := time.Date(2026, 8, 11, 16, 32, 55, 0, time.Local)
	rows := []Row{
		{key: "coordinator", label: "coordinator", spans: []span{{start: activity, end: activity.Add(3 * time.Minute)}}},
	}
	plan := PlanSummary{Name: "aardvark", Started: drafted.Format("2006-01-02 15:04:05") + " (planned)"}
	out := RenderTimelineHTML(rows, plan, "")

	if !strings.Contains(out, "since "+activity.Format("2006-01-02 15:04:05")) {
		t.Errorf("runbox should fall back to earliest recorded activity (%s) when Executing: is absent, not Started: (%s); output:\n%s",
			activity.Format("2006-01-02 15:04:05"), drafted.Format("2006-01-02 15:04:05"), out)
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

// TestBurndownChartShadesByValidationState confirms the burndown grid
// classifies each requirement correctly: met -> green (bd-done), reported
// but not fully resolved (partial/not-met) or currently the team's inferred
// working step -> yellow (bd-active), no signal at all -> white (bd-queued)
// — and that clicking through embeds the owning team's dependency chain.
func TestBurndownChartShadesByValidationState(t *testing.T) {
	base := time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)
	rows := []Row{
		{key: "engine", label: "engine", status: "done", spans: []span{{start: base, end: base.Add(time.Minute)}},
			steps: []StepDetail{{Num: 1, Step: "build the engine", Validation: "engine builds clean", VStatus: "met"}}},
		{key: "app-macos", label: "app-macos", status: "in-progress", dependsOn: []string{"engine"}, spans: []span{{start: base, end: base.Add(time.Minute)}},
			steps: []StepDetail{
				{Num: 2, Step: "wire the UI", Validation: "UI renders"},        // no VStatus, but it's the working step -> active
				{Num: 3, Step: "ship installer", Validation: "installer runs"}, // untouched -> queued
			}},
	}
	out := RenderTimelineHTML(rows, PlanSummary{Name: "badger"}, "")

	if !strings.Contains(out, `<h2>Requirements</h2>`) {
		t.Fatalf("expected a Requirements burndown section; output:\n%s", out)
	}
	if !strings.Contains(out, `class="bd-cell bd-done" data-num="1"`) {
		t.Errorf("requirement 1 (VStatus=met) should render bd-done; output:\n%s", out)
	}
	if !strings.Contains(out, `class="bd-cell bd-active" data-num="2"`) {
		t.Errorf("requirement 2 (inferred working step) should render bd-active; output:\n%s", out)
	}
	if !strings.Contains(out, `class="bd-cell bd-queued" data-num="3"`) {
		t.Errorf("requirement 3 (no signal at all) should render bd-queued; output:\n%s", out)
	}
	if !strings.Contains(out, `"dependsOn":["engine"]`) {
		t.Errorf("requirement 2/3's embedded data should carry app-macos's own full Depends-on list; output:\n%s", out)
	}
}

// TestBurndownChartShowsFullDependsOnListNotJustPrimary reproduces a real
// bug found live: a team can list several dependencies (a real plan had one
// team depending on all seven others), but an earlier version of this
// chart walked only the single "primary" dependency orderTeamRows uses for
// display indentation — that's a deliberate simplification scoped to
// indentation only, and silently dropped every dependency but the first
// when reused here. The click-through detail must show the team's own
// complete Depends-on list.
func TestBurndownChartShowsFullDependsOnListNotJustPrimary(t *testing.T) {
	base := time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)
	rows := []Row{
		{key: "audio-filters", label: "audio-filters", status: "done", spans: []span{{start: base, end: base.Add(time.Minute)}}},
		{key: "settings-surfaces", label: "settings-surfaces", status: "done", dependsOn: []string{"audio-filters"}, spans: []span{{start: base, end: base.Add(time.Minute)}}},
		{key: "tech-debt", label: "tech-debt", status: "in-progress", dependsOn: []string{"audio-filters", "settings-surfaces"}, spans: []span{{start: base, end: base.Add(time.Minute)}},
			steps: []StepDetail{{Num: 17, Step: "clean up", Validation: "clean"}}},
	}
	out := RenderTimelineHTML(rows, PlanSummary{Name: "badger"}, "")

	if !strings.Contains(out, `"dependsOn":["audio-filters","settings-surfaces"]`) {
		t.Errorf("requirement 17's dependsOn should list BOTH of tech-debt's real dependencies, not just the first; output:\n%s", out)
	}
}

// TestBurndownChartOmittedWhenNoStepDetail confirms a plan with no
// per-team Steps/Validation detail (a flat/unteamed plan, or any plan
// whose rows just don't have step data) renders no Requirements section at
// all rather than an empty, confusing one.
func TestBurndownChartOmittedWhenNoStepDetail(t *testing.T) {
	base := time.Date(2026, 8, 11, 16, 0, 0, 0, time.UTC)
	rows := []Row{
		{key: "", label: "coordinator", spans: []span{{start: base, end: base.Add(time.Minute)}}},
	}
	out := RenderTimelineHTML(rows, PlanSummary{Name: "finch"}, "")
	if strings.Contains(out, `<h2>Requirements</h2>`) {
		t.Errorf("expected no Requirements section for a plan with no step detail; output:\n%s", out)
	}
}
