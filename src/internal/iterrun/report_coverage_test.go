package iterrun

import (
	"errors"
	"strings"
	"testing"
)

// TESTMASTER: id=kr-coverage-missing tier=fast parallel=no
//
// Absent, known-zero and partial usage must stay three different answers:
// a Gemsbok-shaped archive with no transcript says unavailable (never a
// zero and never a missing section), a run whose lanes were all read but
// made no request is a known zero, and an Albatross-shaped run with every
// team captured but the coordinator missing is partial, keeps all five team
// lanes, and shows no request percentage over an unknown denominator.
func TestKrCoverageMissing(t *testing.T) {
	t.Run("absent usage is unavailable, not zero", func(t *testing.T) {
		env := newAcctEnv(t, "gemsbok")
		archive := "20261020T094038Z-gemsbok.md"
		rep := BuildPlanReport(env.input("", archive, true))
		cov, use := rep.Coverage, rep.Usage
		if cov.State != stateUnavailable || use.State != stateUnavailable {
			t.Fatalf("coverage %s usage %s, want unavailable/unavailable", cov.State, use.State)
		}
		coord := lane(t, cov.Lanes, "coordinator")
		if coord.State != "missing" || coord.Requests != nil {
			t.Errorf("coordinator lane %+v: want missing with unknown requests", coord)
		}
		f := use.Plan.Figures
		if f.InputTokens != nil || f.OutputTokens != nil || f.CostUSD != nil {
			t.Errorf("absent usage reported figures %+v; unknown must stay nil", f)
		}
		if req := count(t, cov, "requests"); req.Percent != nil || req.Observed != nil {
			t.Errorf("requests count %+v: no denominator is known", req)
		}
		if s := count(t, cov, "sessions"); s.Observed == nil || *s.Observed != 1 || s.Captured != 0 || s.Percent == nil || *s.Percent != 0 {
			t.Errorf("sessions count %+v: one observed session, none captured", s)
		}
		for _, id := range []string{"report-coverage", "report-usage"} {
			html := sectionHTML(t, rep, id)
			if !strings.Contains(html, "unavailable") || !strings.Contains(html, "unknown") {
				t.Errorf("%s must say unavailable and unknown:\n%s", id, html)
			}
		}
		if strings.Contains(sectionHTML(t, rep, "report-usage"), `<b>0</b><span>input tokens`) {
			t.Error("unknown input rendered as zero")
		}

		// Same answer through the API; the page never loses the section.
		got := env.api("", archive, true)
		if got.Report.Usage.State != stateUnavailable || got.Report.Coverage.State != stateUnavailable {
			t.Errorf("API states %s/%s", got.Report.Coverage.State, got.Report.Usage.State)
		}
		before := BuildPlanReport(env.input("", archive, false))
		if before.Coverage.State != stateUnavailable || !strings.Contains(before.Coverage.Summary, "tool history") {
			t.Errorf("before history loads: %s %q", before.Coverage.State, before.Coverage.Summary)
		}
		sectionHTML(t, before, "report-usage")
	})

	t.Run("known zero stays zero", func(t *testing.T) {
		env := newAcctEnv(t, "zero")
		rep := BuildPlanReport(env.input("heron", "", true))
		if rep.Coverage.State != stateComplete {
			t.Fatalf("coverage %s, want complete: %s", rep.Coverage.State, rep.Coverage.Summary)
		}
		req := count(t, rep.Coverage, "requests")
		if req.Captured != 0 || req.Observed == nil || *req.Observed != 0 {
			t.Errorf("requests %+v: want a known 0 of 0", req)
		}
		f := rep.Usage.Plan.Figures
		if val(f.InputTokens) != 0 || val(f.OutputTokens) != 0 || val(f.ReasoningTokens) != 0 {
			t.Errorf("known zero figures %+v must be 0, not unknown", f)
		}
		if rep.Usage.State != stateComplete {
			t.Errorf("usage %s, want complete", rep.Usage.State)
		}
		if !strings.Contains(sectionHTML(t, rep, "report-usage"), `<b>0</b><span>input tokens`) {
			t.Error("known zero input not rendered as 0")
		}
	})

	t.Run("incomplete capture stays partial and keeps every lane", func(t *testing.T) {
		env := newAcctEnv(t, "albatross")
		archive := "2026-09-11T02-27-19Z-albatross-done.md"
		rep := BuildPlanReport(env.input("", archive, true))
		cov := rep.Coverage
		if cov.State != statePartial {
			t.Fatalf("coverage %s, want partial: %s", cov.State, cov.Summary)
		}
		if coord := lane(t, cov.Lanes, "coordinator"); coord.State != "missing" {
			t.Errorf("coordinator %+v, want missing", coord)
		}
		wantReqs := map[string]int{"index": 3, "build": 4, "boot": 5, "metal": 6, "stage0": 7}
		total := 0
		for team, n := range wantReqs {
			l := lane(t, cov.Lanes, team)
			if l.Kind != "team" || l.State != "captured" || l.Requests == nil || *l.Requests != n {
				t.Errorf("team %s lane %+v, want captured team with %d requests", team, l, n)
			}
			total += n
		}
		req := count(t, cov, "requests")
		if req.Captured != total || req.Percent != nil || req.Observed != nil {
			t.Errorf("requests %+v: %d captured over an unknown total, no percentage", req, total)
		}
		if a := count(t, cov, "agents"); a.Observed == nil || *a.Observed != 5 || a.Captured != 5 || a.Percent == nil || *a.Percent != 100 {
			t.Errorf("agents %+v: 5 of 5 over a known denominator", a)
		}
		use := rep.Usage
		if use.State != statePartial || use.Plan.Figures.Requests != total {
			t.Errorf("usage %s with %d requests, want partial with %d", use.State, use.Plan.Figures.Requests, total)
		}
		if !strings.Contains(use.Plan.Note, "lower bound") {
			t.Errorf("partial plan figures must say they are lower bounds: %q", use.Plan.Note)
		}
		teams := 0
		for _, r := range use.Agents {
			if r.Kind == "team" && r.State == "captured" {
				teams++
			}
		}
		if teams != 5 {
			t.Errorf("usage keeps %d team rows, want the five Albatross lanes", teams)
		}
		html := sectionHTML(t, rep, "report-coverage")
		for _, want := range []string{"coordinator", "missing", "stage0", "partial", "of unknown"} {
			if !strings.Contains(html, want) {
				t.Errorf("coverage HTML missing %q", want)
			}
		}
	})

	t.Run("an unattributed coordinator leaves the session count unknown", func(t *testing.T) {
		// The real Albatross: the hook log attributes no coordinator
		// session at all, so how many sessions ran is unknown.
		pt := &PlanTokens{Plan: "albatross", Total: TokenUsage{Messages: 3},
			Lanes: []LaneTokens{{Key: "", Label: "coordinator", Missing: true},
				{Key: "index", Label: "index", Observed: 1, Found: 1, Agents: []string{"a1"}, AgentsFound: []string{"a1"}, Sessions: []string{"s1"}, Usage: TokenUsage{Messages: 3}}}}
		rep := BuildPlanReport(ReportInput{Plan: PlanSummary{Name: "albatross", Teamed: true}, Tokens: pt, HistoryLoaded: true})
		s := count(t, rep.Coverage, "sessions")
		if s.Observed != nil || s.Percent != nil || s.State == stateComplete {
			t.Errorf("sessions %+v: the denominator is unknown", s)
		}
		if l := lane(t, rep.Coverage.Lanes, "coordinator"); l.State != "missing" || !strings.Contains(l.Note, "attributes no session") {
			t.Errorf("coordinator %+v", l)
		}
		if rep.Coverage.State != statePartial {
			t.Errorf("coverage %s, want partial", rep.Coverage.State)
		}
	})

	t.Run("an unreadable record stays explicit", func(t *testing.T) {
		in := ReportInput{Plan: PlanSummary{Name: "ghost"}, RecordErr: errors.New("store unreadable")}
		rep := BuildPlanReport(in)
		if rep.Coverage.State != stateUnavailable || rep.Coverage.Record.Source != "unreadable" {
			t.Errorf("coverage %+v", rep.Coverage.Record)
		}
		for _, id := range []string{"report-coverage", "report-usage", "report-delivery"} {
			if !strings.Contains(sectionHTML(t, rep, id), "unavailable") {
				t.Errorf("%s must render an unavailable state", id)
			}
		}
	})
}

// TESTMASTER: id=kr-coverage-providers tier=fast parallel=no
//
// A Codex coordinator (current per-response records and an older
// token_count-only rollout) and Claude Code teams and subagents report the
// same fields; what a provider does not report is explicit, a flat plan's
// subagent is an agent and never a team, and Conductor stream usage keeps
// its own fields without double counting a session a transcript holds.
func TestKrCoverageProviders(t *testing.T) {
	env := newAcctEnv(t, "providers")

	codex := BuildPlanReport(env.input("kestrel", "", true))
	cov, use := codex.Coverage, codex.Usage
	coord := lane(t, cov.Lanes, "coordinator")
	if coord.State != "captured" || coord.Found != 2 || strings.Join(coord.Engines, ",") != "codex" || strings.Join(coord.Providers, ",") != "openai" {
		t.Fatalf("Codex coordinator lane %+v", coord)
	}
	if coord.Requests == nil || *coord.Requests != 4 {
		t.Errorf("Codex requests %v, want 4 (2 per-response records + 2 token_count growths)", coord.Requests)
	}
	var codexSrc *UsageSource
	for i := range use.Sources {
		if use.Sources[i].Source == "transcript:codex" {
			codexSrc = &use.Sources[i]
		}
	}
	if codexSrc == nil {
		t.Fatalf("no Codex usage source in %+v", use.Sources)
	}
	f := codexSrc.Figures
	// resp_k1 25000/0/180/40, resp_k2 36000/25000/130/10, old rollout
	// 10000/2000/90/20 and 12000/9000/60/10; the repeated token_count adds
	// nothing.
	if val(f.InputTokens) != 83000 || val(f.CacheReadTokens) != 36000 || val(f.CacheWriteTokens) != 0 ||
		val(f.OutputTokens) != 460 || val(f.ReasoningTokens) != 80 || f.CostUSD != nil {
		t.Errorf("Codex figures %+v", f)
	}
	if !strings.Contains(codexSrc.Note, "no per-request cost") {
		t.Errorf("Codex cost gap must be explicit: %q", codexSrc.Note)
	}
	if len(use.Allowance.Observations) != 1 || use.Allowance.Observations[0].FirstPercent != 34 || use.Allowance.Observations[0].LastPercent != 35 {
		t.Errorf("Codex allowance %+v", use.Allowance.Observations)
	}
	// Conductor stream: the session a rollout holds is counted once; the
	// other keeps input/output and leaves cache and reasoning unknown.
	var rec *UsageSource
	for i := range use.Sources {
		if use.Sources[i].Source == "run_record" {
			rec = &use.Sources[i]
		}
	}
	if rec == nil || rec.Figures.Requests != 1 || val(rec.Figures.InputTokens) != 40000 || rec.Figures.CacheReadTokens != nil || rec.Figures.ReasoningTokens != nil {
		t.Fatalf("run-record source %+v", rec)
	}
	if !strings.Contains(rec.Note, "not added again") {
		t.Errorf("overlap must be stated: %q", rec.Note)
	}
	if use.Plan.Figures.Requests != 5 || !containsString(use.Plan.Figures.Partial, figCacheRead) || !containsString(use.Plan.Figures.Partial, figReasoning) {
		t.Errorf("merged plan figures %+v: 5 requests, cache and reasoning partial", use.Plan.Figures)
	}
	for _, br := range cov.Breakdowns {
		if br.Name == "team" && br.State != "not_applicable" {
			t.Errorf("flat Codex plan team breakdown %+v", br)
		}
	}
	for _, l := range cov.Lanes {
		if l.Kind == "team" {
			t.Errorf("flat plan grew a team lane: %+v", l)
		}
	}
	html := sectionHTML(t, codex, "report-usage")
	for _, want := range []string{"Codex session rollouts", "gpt-6.1-sol", "openai", "unknown", "34% used", "exec_command"} {
		if !strings.Contains(html, want) {
			t.Errorf("Codex usage HTML missing %q", want)
		}
	}

	// A teamed Claude Code plan: equivalent fields, team lanes.
	teamed := BuildPlanReport(env.input("osprey", "", true))
	for _, name := range []string{"alpha", "beta"} {
		l := lane(t, teamed.Coverage.Lanes, name)
		if l.Kind != "team" || l.State != "captured" || strings.Join(l.Engines, ",") != "claude-code" || strings.Join(l.Providers, ",") != "anthropic" {
			t.Errorf("teamed lane %s %+v", name, l)
		}
	}
	tf := teamed.Usage.Plan.Figures
	for name, v := range map[string]*int64{figInput: tf.InputTokens, figCacheRead: tf.CacheReadTokens, figCacheWrite: tf.CacheWriteTokens, figOutput: tf.OutputTokens, figReasoning: tf.ReasoningTokens} {
		if v == nil {
			t.Errorf("Claude figure %s missing; the Codex path reports it", name)
		}
	}
	if tf.CostUSD != nil {
		t.Errorf("Claude transcripts carry no per-request cost, got %v", *tf.CostUSD)
	}

	// A flat Claude Code plan's subagent is an agent, never a team.
	flat := BuildPlanReport(env.input("plover", "", true))
	agents := 0
	for _, l := range flat.Coverage.Lanes {
		switch l.Kind {
		case "team":
			t.Errorf("flat plan fabricated a team: %+v", l)
		case "agent":
			agents++
		}
	}
	if agents != 1 {
		t.Errorf("flat plan agent lanes %d, want 1", agents)
	}
	if html := sectionHTML(t, flat, "report-coverage"); !strings.Contains(html, "not applicable") || !strings.Contains(html, "flat plan") {
		t.Error("flat plan must say the team breakdown does not apply")
	}

	// The API carries the same model.
	got := env.api("kestrel", "", true)
	if got.Report.Usage.Plan.Figures.Requests != 5 || got.Report.Coverage.State != codex.Coverage.State {
		t.Errorf("API kestrel %+v", got.Report.Usage.Plan.Figures)
	}
}
