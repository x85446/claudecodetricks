package iterrun

import (
	"strings"
	"testing"
)

// TESTMASTER: id=kr-usage-scopes tier=fast parallel=no
//
// A Waterbuck-shaped execution: five requests inside a 22-hour window of a
// coordinator session that also did other work and reports $784.89 for its
// whole life. Plan usage, whole-session cost, subscription allowance and
// estimated allocations each keep their own scope and never add together.
func TestKrUsageScopes(t *testing.T) {
	env := newAcctEnv(t, "waterbuck")
	rep := BuildPlanReport(env.input("waterbuck", "", true))
	u := rep.Usage

	plan := u.Plan.Figures
	if u.Plan.Scope != "exact" || plan.Requests != 5 {
		t.Fatalf("plan scope %s with %d requests, want exact with 5 (the session's 3 earlier requests are outside the window)", u.Plan.Scope, plan.Requests)
	}
	// wb-1 keeps its final streaming write (160 output, not 5 + 160).
	if val(plan.OutputTokens) != 160+140+120+180+125 || val(plan.ReasoningTokens) != 40+30+20+50+25 {
		t.Errorf("plan output %d reasoning %d", val(plan.OutputTokens), val(plan.ReasoningTokens))
	}
	wantInput := int64(5*6 + 170000 + 8000 + 178000 + 600 + 179000 + 400 + 179500 + 300 + 180000 + 200)
	if val(plan.InputTokens) != wantInput || val(plan.CacheReadTokens) != 170000+178000+179000+179500+180000 || val(plan.CacheWriteTokens) != 8000+600+400+300+200 {
		t.Errorf("plan input %d (want %d) cache read %d cache write %d", val(plan.InputTokens), wantInput, val(plan.CacheReadTokens), val(plan.CacheWriteTokens))
	}
	if plan.CostUSD != nil {
		t.Errorf("plan cost %v: no per-request price was reported, so it is unknown", *plan.CostUSD)
	}
	if len(u.SessionCost.Items) != 1 || u.SessionCost.Items[0].CostUSD != 784.89 || u.SessionCost.Items[0].Sessions != 1 {
		t.Fatalf("session cost %+v, want one $784.89 whole-session figure", u.SessionCost.Items)
	}
	if !strings.Contains(u.SessionCost.Note, "not this plan's bill") {
		t.Errorf("session cost note %q", u.SessionCost.Note)
	}
	if u.Allowance.State != "available" || len(u.Allowance.Observations) != 1 || u.Allowance.Observations[0].Provider != "anthropic" {
		t.Errorf("allowance %+v", u.Allowance)
	}
	if u.Estimated.State != "available" || u.Estimated.Figures.Requests != 1 || val(u.Estimated.Figures.InputTokens) != 360000 {
		t.Errorf("estimated %+v", u.Estimated)
	}
	for _, w := range u.Wakeups {
		if w.Scope != "estimated" {
			t.Errorf("wake-up %s scope %s", w.Label, w.Scope)
		}
	}
	for _, tl := range u.Tools {
		if tl.Scope != "estimated" {
			t.Errorf("tool %s scope %s", tl.Tool, tl.Scope)
		}
	}
	if len(u.Wakeups) == 0 || len(u.Tools) == 0 {
		t.Errorf("wake-up (%d) and tool (%d) breakdowns must survive", len(u.Wakeups), len(u.Tools))
	}

	html := sectionHTML(t, rep, "report-usage")
	rest := html[strings.Index(html, "Plan-attributed usage"):]
	planPart := rest[:strings.Index(rest, "Whole-session cost")]
	if strings.Contains(planPart, "784.89") {
		t.Error("the whole-session cost appears among the plan-attributed figures")
	}
	if !strings.Contains(planPart, `unknown</span></b><span>cost (USD)`) {
		t.Error("plan cost must render as unknown")
	}
	for _, want := range []string{"$784.89", "not this plan&#39;s bill", "Estimated allocations", "By wake-up (estimated", "By tool (estimated",
		"gen_ai.client.inference.usage.input_tokens", "OpenTelemetry GenAI semantic conventions", "1.44.0", "{inference_call}", "{tool_call}"} {
		if !strings.Contains(html, want) {
			t.Errorf("usage HTML missing %q", want)
		}
	}
	if !strings.Contains(u.Summary, "5 requests") || !strings.Contains(u.Summary, "$784.89") || !strings.Contains(u.Summary, "not just this plan") {
		t.Errorf("summary %q", u.Summary)
	}

	got := env.api("waterbuck", "", true)
	if got.Source != "record" || got.Report.Usage.Plan.Figures.Requests != 5 || got.Report.Usage.SessionCost.Items[0].CostUSD != 784.89 {
		t.Errorf("API source %s usage %+v", got.Source, got.Report.Usage.Plan.Figures)
	}
}

// TESTMASTER: id=kr-usage-dedup tier=fast parallel=no
//
// Streaming rewrites, a subagent request echoed into its parent's
// transcript, history copied into a resumed session, cumulative Conductor
// stream totals, a retried record write and attempt roll-ups must each
// count a request once; reasoning and cache stay subsets; a request with
// no price leaves the cost partial and a cost without its price source says
// so.
func TestKrUsageDedup(t *testing.T) {
	env := newAcctEnv(t, "dedup")
	rep := BuildPlanReport(env.input("dunlin", "", true))
	u := rep.Usage

	coordinator, app := -1, -1
	for i, r := range u.Agents {
		switch r.Label {
		case "coordinator":
			coordinator = i
		case "app":
			app = i
		}
	}
	if coordinator < 0 || app < 0 {
		t.Fatalf("lanes %+v", u.Agents)
	}
	c, a := u.Agents[coordinator].Figures, u.Agents[app].Figures
	// Coordinator: dun-c1 (final write), dun-c2 once across both sessions,
	// dun-c3. The echoed dun-s1 belongs to the app team.
	if c.Requests != 3 || val(c.OutputTokens) != 120+60+90 || val(c.ReasoningTokens) != 40+15+20 {
		t.Errorf("coordinator %+v", c)
	}
	if val(c.InputTokens) != (10+1000+200)+(8+2000+50)+(9+2500+40) {
		t.Errorf("coordinator input %d", val(c.InputTokens))
	}
	if a.Requests != 2 || val(a.OutputTokens) != 70+55 {
		t.Errorf("app %+v", a)
	}
	if u.Duplicates != 2 {
		t.Errorf("duplicates %d, want 2 (the echoed subagent request and the resumed session's copy)", u.Duplicates)
	}

	// Record: asg-d1's three cumulative totals and its retried final write
	// are one request; asg-d2 has no price; asg-d0's session is a transcript
	// already counted.
	var rec *UsageSource
	for i := range u.Sources {
		if u.Sources[i].Source == "run_record" {
			rec = &u.Sources[i]
		}
	}
	if rec == nil {
		t.Fatalf("sources %+v", u.Sources)
	}
	if rec.Figures.Requests != 2 || val(rec.Figures.InputTokens) != 300+400 || val(rec.Figures.OutputTokens) != 30+40 {
		t.Errorf("record figures %+v", rec.Figures)
	}
	if rec.Figures.CostUSD == nil || *rec.Figures.CostUSD != 0.5 || !containsString(rec.Figures.Partial, figCost) {
		t.Errorf("record cost %+v: $0.50 from one priced request, partial", rec.Figures)
	}
	if !containsString(rec.Figures.PriceSources, "fixture price table 2026-10") {
		t.Errorf("price provenance %v", rec.Figures.PriceSources)
	}

	plan := u.Plan.Figures
	if plan.Requests != 7 {
		t.Errorf("plan requests %d, want 5 transcript + 2 record", plan.Requests)
	}
	if val(plan.OutputTokens) != 270+125+70 {
		t.Errorf("plan output %d", val(plan.OutputTokens))
	}
	if val(plan.ReasoningTokens) != 75+15 || !containsString(plan.Partial, figReasoning) {
		t.Errorf("reasoning %d partial %v: transcripts' reasoning only, partial because the stream reports none", val(plan.ReasoningTokens), plan.Partial)
	}
	if val(plan.InputTokens) < val(plan.CacheReadTokens)+val(plan.CacheWriteTokens) {
		t.Error("cache figures must be subsets of input")
	}

	// Attempts stay attempts: two, one failed, never an extra request.
	var agentOp *UsageOperation
	for i := range u.Operations {
		if u.Operations[i].Operation == "invoke_agent" && u.Operations[i].Name == "conductor assignment" {
			agentOp = &u.Operations[i]
		}
	}
	if agentOp == nil || agentOp.Errors == nil || *agentOp.Errors != 1 || agentOp.ErrorTypes["failed"] != 1 || agentOp.DurationSeconds == nil || *agentOp.DurationSeconds != 540 {
		t.Errorf("invoke_agent operation %+v", agentOp)
	}

	html := sectionHTML(t, rep, "report-usage")
	for _, want := range []string{"counted once", "fixture price table 2026-10", "partial", "$0.50"} {
		if !strings.Contains(html, want) {
			t.Errorf("usage HTML missing %q", want)
		}
	}

	// A price with no recorded source says so.
	var f UsageFigures
	in, out, cost := int64(1), int64(2), 0.25
	f.add(figureRequest{input: &in, output: &out, cost: &cost})
	if !containsString(f.PriceSources, "price source not recorded") {
		t.Errorf("unsourced price %v", f.PriceSources)
	}
}
