package iterrun

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// UsageSection is the plan report's step-12 section: Plan-scoped usage by scope, model, provider, agent, tool and wake-up.
//
// Four scopes never add together: Plan holds exact provider counts
// attributed to this execution; Estimated holds allocations of a larger
// total; SessionCost is the harness's whole-session cost, which includes
// work outside this plan; Allowance holds subscription-window readings
// shared by every session on the account.
type UsageSection struct {
	State       string            `json:"state"`
	Summary     string            `json:"summary"`
	History     bool              `json:"history_loaded"`
	Window      *AccountingWindow `json:"window"`
	Conventions UsageConventions  `json:"conventions"`

	Plan        UsageScope       `json:"plan"`
	Estimated   UsageScope       `json:"estimated"`
	SessionCost UsageSessionCost `json:"session_cost"`
	Allowance   UsageAllowance   `json:"allowance"`

	Sources    []UsageSource    `json:"sources"`
	Agents     []UsageRow       `json:"agents"`
	Models     []UsageRow       `json:"models"`
	Tools      []UsageToolRow   `json:"tools"`
	Wakeups    []UsageWakeRow   `json:"wakeups"`
	Operations []UsageOperation `json:"operations"`
	// Duplicates counts requests seen in more than one place and counted
	// once.
	Duplicates  int               `json:"duplicates"`
	Definitions []UsageDefinition `json:"definitions"`
}

// UsageScope is one scope's figures.
type UsageScope struct {
	Scope   string       `json:"scope"` // exact | estimated
	State   string       `json:"state"`
	Figures UsageFigures `json:"figures"`
	Note    string       `json:"note"`
}

// UsageSessionCost is whole-session cost: what each session cost over its
// whole life, never divided across plans or lanes.
type UsageSessionCost struct {
	State string             `json:"state"` // available | unavailable
	Items []UsageSessionItem `json:"items"`
	Note  string             `json:"note"`
}

// UsageSessionItem is one whole-session cost figure and its provenance.
type UsageSessionItem struct {
	CostUSD     float64 `json:"cost_usd"`
	Sessions    int     `json:"sessions"`
	Source      string  `json:"source"`
	PriceSource string  `json:"price_source"`
}

// UsageAllowance holds subscription allowance readings.
type UsageAllowance struct {
	State        string               `json:"state"`
	Observations []UsageAllowanceItem `json:"observations"`
	Note         string               `json:"note"`
}

// UsageAllowanceItem is one allowance window's reading at the start and
// end of the attribution window.
type UsageAllowanceItem struct {
	Provider      string     `json:"provider"`
	Limit         string     `json:"limit"`
	Window        string     `json:"window"`
	WindowMinutes int        `json:"window_minutes"`
	PlanType      string     `json:"plan_type,omitempty"`
	FirstAt       time.Time  `json:"first_at"`
	FirstPercent  float64    `json:"first_used_percent"`
	LastAt        time.Time  `json:"last_at"`
	LastPercent   float64    `json:"last_used_percent"`
	ResetsAt      *time.Time `json:"resets_at,omitempty"`
	Source        string     `json:"source"`
}

// UsageSource is one usage source's exact figures.
type UsageSource struct {
	Source  string       `json:"source"`
	Label   string       `json:"label"`
	State   string       `json:"state"`
	Figures UsageFigures `json:"figures"`
	Note    string       `json:"note,omitempty"`
}

// UsageRow is one agent, team, coordinator or model row.
type UsageRow struct {
	Key      string       `json:"key"`
	Label    string       `json:"label"`
	Kind     string       `json:"kind"`
	Provider string       `json:"provider,omitempty"`
	Engine   string       `json:"engine,omitempty"`
	Models   []string     `json:"models,omitempty"`
	State    string       `json:"state"`
	Figures  UsageFigures `json:"figures"`
	// PeakContextTokens is the largest single request's input.
	PeakContextTokens *int64 `json:"peak_context_tokens,omitempty"`
	Source            string `json:"source"`
	Note              string `json:"note,omitempty"`
}

// UsageToolRow is one tool's estimated share. Calls are counted; the
// token figures are allocations.
type UsageToolRow struct {
	Tool                string  `json:"tool"`
	Calls               int     `json:"calls"`
	OutputTokens        int64   `json:"output_tokens"`
	ContextGrowthTokens int64   `json:"context_growth_tokens"`
	SharePercent        float64 `json:"share_percent"`
	Scope               string  `json:"scope"`
}

// UsageWakeRow is one wake-up kind's estimated share.
type UsageWakeRow struct {
	Label               string  `json:"label"`
	Fired               int     `json:"fired"`
	Ran                 int     `json:"ran"`
	Idle                int     `json:"idle"`
	Coalesced           int     `json:"coalesced"`
	Requests            int     `json:"requests"`
	InputTokens         int64   `json:"input_tokens"`
	OutputTokens        int64   `json:"output_tokens"`
	MedianContextTokens int64   `json:"median_context_tokens"`
	SharePercent        float64 `json:"share_percent"`
	Scope               string  `json:"scope"`
}

// UsageOperation is one operation's count, duration and errors, in the
// OpenTelemetry GenAI vocabulary. Nil duration or errors are unknown.
type UsageOperation struct {
	Operation       string         `json:"operation"` // gen_ai.operation.name
	Name            string         `json:"name"`      // tool, agent or model
	Metric          string         `json:"metric"`
	Count           int            `json:"count"`
	Unit            string         `json:"unit"`
	DurationSeconds *float64       `json:"duration_seconds,omitempty"`
	MaxSeconds      *float64       `json:"max_seconds,omitempty"`
	Errors          *int           `json:"errors,omitempty"`
	ErrorTypes      map[string]int `json:"error_types,omitempty"`
	// NoResult counts operations still open or ended without a recorded
	// result; they are neither successes nor errors.
	NoResult int    `json:"no_result,omitempty"`
	Source   string `json:"source"`
	Note     string `json:"note,omitempty"`
}

// UsageDefinition is one term the section uses.
type UsageDefinition struct {
	Term       string `json:"term"`
	Definition string `json:"definition"`
}

var usageDefinitions = []UsageDefinition{
	{"Request", "one provider inference call ({inference_call}): a Claude message id or a Codex response id. Streaming rewrites, retried writes and copies of the same id in another transcript count once."},
	{"Call", "one client-side tool call ({tool_call}), such as Bash or Edit."},
	{"Wake-up", "a prompt that starts a turn: a slash command, a cron or /loop firing, a coordinator nudge, a task notification or a human prompt."},
	{"Input tokens", "everything sent to the model for a request, including cache reads and cache writes (gen_ai.client.inference.usage.input_tokens)."},
	{"Cache read / cache write", "the parts of input served from or written to the provider's prompt cache. Both are subsets of input and never added to it."},
	{"Output tokens", "everything the model generated, including reasoning (gen_ai.client.inference.usage.output_tokens)."},
	{"Reasoning tokens", "the part of output spent on reasoning. A subset of output, never added to it."},
	{"Exact", "counts the provider reported for each request, attributed to this execution by session and attribution window."},
	{"Estimated", "an allocation of a larger total, such as tool context growth or wake-up shares. Never added to exact totals."},
	{"Whole-session cost", "the harness's own cost figure for every session this plan ran in, including work those sessions did for other plans. Not this plan's bill."},
	{"Allowance", "a subscription window's used percentage, shared by every session on the account. Never converted into this plan's usage."},
	{"Operation duration", "seconds from the start to the end of an operation (gen_ai.execute_tool.duration, gen_ai.invoke_agent.duration); concurrent operations are not summed into wall time."},
	{"Error", "an operation that ended unsuccessfully, by error.type. Unknown when the source records no result."},
	{"Unknown", "no source reported the figure. Unknown is never shown as zero; a partial figure is a lower bound."},
}

func buildUsageSection(in ReportInput) UsageSection {
	c := assessCapture(in)
	s := UsageSection{History: in.HistoryLoaded, Window: accountingWindow(in), Conventions: otelConventions, Definitions: usageDefinitions}
	pt := in.Tokens
	if pt != nil && pt.From != nil {
		to := pt.To
		s.Window.From, s.Window.To, s.Window.Open = pt.From, &to, pt.ToOpen
	}
	runs := liveRuns(in.Record)

	// Exact figures by source.
	var plan UsageFigures
	if pt != nil && in.HistoryLoaded {
		byEngine := map[string]*UsageFigures{}
		var engines []string
		for _, m := range pt.Models {
			f := byEngine[m.Engine]
			if f == nil {
				f = &UsageFigures{}
				byEngine[m.Engine] = f
				engines = append(engines, m.Engine)
			}
			f.merge(tokenFigures(m.Usage, m.ReasoningKnown))
		}
		sort.Strings(engines)
		for _, e := range engines {
			label := map[string]string{"claude-code": "Claude Code transcripts", "codex": "Codex session rollouts"}[e]
			if label == "" {
				label = "Session transcripts (" + e + ")"
			}
			src := UsageSource{Source: "transcript:" + e, Label: label, State: "captured", Figures: *byEngine[e]}
			if e == "codex" {
				src.Note = "Codex reports no per-request cost"
			} else {
				src.Note = "transcripts carry no per-request cost; the session's own total is under whole-session cost"
			}
			s.Sources = append(s.Sources, src)
			plan.merge(*byEngine[e])
		}
		s.Duplicates = pt.Duplicates
		if len(engines) == 0 {
			src := UsageSource{Source: "transcript", Label: "Session transcripts", State: c.requestState}
			switch {
			case c.knownZero:
				src.State, src.Note = "captured", "every observed lane was read; no request in the window"
				src.Figures = zeroFigures()
			default:
				src.State, src.Note = "missing", "no transcript with requests in the attribution window"
			}
			s.Sources = append(s.Sources, src)
		}
	} else {
		s.Sources = append(s.Sources, UsageSource{Source: "transcript", Label: "Session transcripts", State: stateUnavailable,
			Note: "not loaded: transcripts are read with tool history"})
	}
	if c.recordExact.Requests > 0 || c.recordOverlap > 0 {
		src := UsageSource{Source: "run_record", Label: "Run record (provider stream)", State: "captured", Figures: c.recordExact}
		var notes []string
		for _, r := range runs {
			for _, u := range r.Usage.Requests {
				if u.Scope == runrecord.ScopeExact && u.Note != "" && !containsString(notes, u.Note) {
					notes = append(notes, u.Note)
				}
			}
		}
		if c.recordOverlap > 0 {
			notes = append(notes, fmt.Sprintf("%s also read from a session transcript and not added again", countText(c.recordOverlap, "request", "requests")))
		}
		src.Note = strings.Join(notes, "; ")
		s.Sources = append(s.Sources, src)
		plan.merge(c.recordExact)
	}

	s.Plan = UsageScope{Scope: runrecord.ScopeExact, State: c.requestState, Figures: plan,
		Note: "provider-reported counts, attributed by session and attribution window"}
	if c.knownZero && plan.Requests == 0 {
		s.Plan.Figures = zeroFigures()
	}
	if c.requestState == statePartial {
		s.Plan.Note += "; uncaptured lanes make every total a lower bound"
	}

	// Estimated, whole-session cost and allowance scopes from the record.
	var estimated UsageFigures
	var recordCost, recordAllowance []runrecord.UsageEntry
	for _, r := range runs {
		for _, u := range r.Usage.Requests {
			switch u.Scope {
			case runrecord.ScopeEstimated:
				estimated.add(recordRequest(u.Usage))
			case runrecord.ScopeSessionCost:
				recordCost = append(recordCost, u)
			case runrecord.ScopeAllowance:
				recordAllowance = append(recordAllowance, u)
			}
		}
	}
	s.Estimated = UsageScope{Scope: runrecord.ScopeEstimated, Figures: estimated, State: stateUnavailable,
		Note: "allocations of a larger total; never added to the exact figures"}
	if estimated.Requests > 0 {
		s.Estimated.State = "available"
	}

	s.SessionCost = UsageSessionCost{State: stateUnavailable,
		Note: "covers each session's whole life, including work for other plans; it is not this plan's bill and is never divided across lanes"}
	if pt != nil && pt.HaveCost {
		sessions := pt.CostSessions
		if sessions == 0 {
			sessions = len(pt.Sessions)
		}
		s.SessionCost.Items = append(s.SessionCost.Items, UsageSessionItem{CostUSD: pt.CostUSD, Sessions: sessions,
			Source: "Claude Code cost-state records", PriceSource: "Claude Code's own session total"})
	}
	for _, u := range recordCost {
		if u.CostUSD == nil {
			continue
		}
		src := u.PriceSource
		if src == "" {
			src = "price source not recorded"
		}
		s.SessionCost.Items = append(s.SessionCost.Items, UsageSessionItem{CostUSD: *u.CostUSD, Sessions: 1, Source: "run record (" + u.Source + ")", PriceSource: src})
	}
	if len(s.SessionCost.Items) > 0 {
		s.SessionCost.State = "available"
	} else if pt != nil && in.HistoryLoaded {
		s.SessionCost.Note = "no session reported a cost; cost is unknown, not zero"
	}

	s.Allowance = UsageAllowance{State: stateUnavailable,
		Note: "subscription windows are shared by every session on the account; a reading is never converted into this plan's usage"}
	if pt != nil {
		s.Allowance.Observations = allowanceItems(pt.Allowance)
	}
	for _, u := range recordAllowance {
		s.Allowance.Observations = append(s.Allowance.Observations, UsageAllowanceItem{Provider: u.Provider, Limit: u.RequestID,
			FirstAt: u.At, LastAt: u.At, Source: "run record: " + u.Note})
	}
	if len(s.Allowance.Observations) > 0 {
		s.Allowance.State = "available"
	}

	// Agents, models, tools, wake-ups.
	if pt != nil && in.HistoryLoaded {
		for _, l := range pt.Lanes {
			row := UsageRow{Key: l.Key, Label: l.Label, Kind: laneKind(l.Key, in.Plan), Models: l.Models, Source: "transcript"}
			if len(l.Providers) > 0 {
				row.Provider = strings.Join(l.Providers, ", ")
			}
			if len(l.Engines) > 0 {
				row.Engine = strings.Join(l.Engines, ", ")
			}
			switch {
			case l.Missing:
				row.State, row.Note = "missing", "no transcript found; usage unknown"
			case l.Found < l.Observed:
				row.State = statePartial
				row.Note = fmt.Sprintf("%d of %d session transcripts read; figures are lower bounds", l.Found, l.Observed)
				row.Figures = tokenFigures(l.Usage, l.ReasoningKnown)
			default:
				row.State = "captured"
				row.Figures = tokenFigures(l.Usage, l.ReasoningKnown)
				if l.Usage.Messages == 0 {
					row.Figures = zeroFigures()
				}
			}
			if !l.Missing && l.PeakContext > 0 {
				row.PeakContextTokens = i64(l.PeakContext)
			}
			s.Agents = append(s.Agents, row)
		}
		for _, m := range pt.Models {
			s.Models = append(s.Models, UsageRow{Key: m.Provider + "/" + m.Model, Label: modelLabel(m.Model), Kind: "model",
				Provider: m.Provider, Engine: m.Engine, State: "captured", Figures: tokenFigures(m.Usage, m.ReasoningKnown), Source: "transcript"})
		}
		for _, t := range pt.Tools {
			s.Tools = append(s.Tools, UsageToolRow{Tool: t.Tool, Calls: t.Calls, OutputTokens: t.Emitting,
				ContextGrowthTokens: t.ContextGrowth, SharePercent: t.Share, Scope: runrecord.ScopeEstimated})
		}
		for _, g := range pt.Groups {
			s.Wakeups = append(s.Wakeups, UsageWakeRow{Label: g.Label, Fired: g.Count, Ran: g.Ran, Idle: g.Idle, Coalesced: g.Coalesced,
				Requests: g.Usage.Messages, InputTokens: g.Usage.Context(), OutputTokens: g.Usage.Output,
				MedianContextTokens: g.Median, SharePercent: g.Share, Scope: runrecord.ScopeEstimated})
		}
	}
	for _, lane := range c.recordLaneOrder {
		f := c.recordByLane[lane]
		s.Agents = append(s.Agents, UsageRow{Key: "record:" + lane, Label: lane, Kind: "record", State: "captured", Figures: *f, Source: "run record"})
	}
	recordModels := map[string]*UsageRow{}
	var recordModelOrder []string
	for _, r := range runs {
		for _, u := range r.Usage.Requests {
			if u.Scope != runrecord.ScopeExact || (u.SessionID != "" && c.transcriptSessions[u.SessionID]) {
				continue
			}
			k := u.Provider + "/" + u.Model
			row := recordModels[k]
			if row == nil {
				row = &UsageRow{Key: k, Label: modelLabel(u.Model), Kind: "model", Provider: u.Provider, State: "captured", Source: "run record"}
				recordModels[k] = row
				recordModelOrder = append(recordModelOrder, k)
			}
			row.Figures.add(recordRequest(u.Usage))
		}
	}
	for _, k := range recordModelOrder {
		s.Models = append(s.Models, *recordModels[k])
	}

	s.Operations = usageOperations(in, plan)
	for _, list := range []*[]UsageRow{&s.Agents, &s.Models} {
		if *list == nil {
			*list = []UsageRow{}
		}
	}
	if s.Tools == nil {
		s.Tools = []UsageToolRow{}
	}
	if s.Wakeups == nil {
		s.Wakeups = []UsageWakeRow{}
	}
	if s.Operations == nil {
		s.Operations = []UsageOperation{}
	}
	if s.SessionCost.Items == nil {
		s.SessionCost.Items = []UsageSessionItem{}
	}
	if s.Allowance.Observations == nil {
		s.Allowance.Observations = []UsageAllowanceItem{}
	}
	s.State = c.requestState
	if c.requestState == stateComplete && plan.Requests == 0 && !c.knownZero {
		s.State = stateUnavailable
	}
	s.Summary = usageSummary(s, c)
	return s
}

func zeroFigures() UsageFigures {
	return UsageFigures{InputTokens: i64(0), CacheReadTokens: i64(0), CacheWriteTokens: i64(0), OutputTokens: i64(0), ReasoningTokens: i64(0)}
}

func modelLabel(m string) string {
	if m == "" {
		return "model not recorded"
	}
	return m
}

// allowanceItems pairs each window's first and last reading.
func allowanceItems(obs []AllowanceObservation) []UsageAllowanceItem {
	var out []UsageAllowanceItem
	idx := map[string]int{}
	for _, a := range obs {
		k := a.Provider + "\x00" + a.Limit + "\x00" + a.Window + "\x00" + timeKey(a.ResetsAt)
		i, ok := idx[k]
		if !ok {
			idx[k] = len(out)
			out = append(out, UsageAllowanceItem{Provider: a.Provider, Limit: a.Limit, Window: a.Window, WindowMinutes: a.WindowMinutes,
				PlanType: a.PlanType, FirstAt: a.At, FirstPercent: a.UsedPercent, LastAt: a.At, LastPercent: a.UsedPercent,
				ResetsAt: a.ResetsAt, Source: a.Provider + " rate-limit readings in the session rollout"})
			continue
		}
		it := &out[i]
		if a.At.Before(it.FirstAt) {
			it.FirstAt, it.FirstPercent = a.At, a.UsedPercent
		}
		if !a.At.Before(it.LastAt) {
			it.LastAt, it.LastPercent = a.At, a.UsedPercent
		}
	}
	return out
}

// usageOperations reports operation counts, durations and errors from the
// timeline spans (tool calls) and the record's attempts (agent runs).
func usageOperations(in ReportInput, plan UsageFigures) []UsageOperation {
	var out []UsageOperation
	if plan.Requests > 0 {
		out = append(out, UsageOperation{Operation: "chat", Name: "all models", Metric: "gen_ai.client.inference.usage.*",
			Count: plan.Requests, Unit: "{inference_call}", Source: "exact usage",
			Note: "request durations and errors are not recorded by transcripts or the provider stream"})
	}
	type agg struct {
		count    int
		sum, max float64
	}
	tools := map[string]*agg{}
	for _, r := range in.Rows {
		for _, sp := range r.spans {
			if sp.tool == "" {
				continue
			}
			a := tools[sp.tool]
			if a == nil {
				a = &agg{}
				tools[sp.tool] = a
			}
			d := sp.end.Sub(sp.start).Seconds()
			if d < 0 {
				d = 0
			}
			a.count++
			a.sum += d
			if d > a.max {
				a.max = d
			}
		}
	}
	var names []string
	for n := range tools {
		names = append(names, n)
	}
	sort.Slice(names, func(i, j int) bool {
		if tools[names[i]].sum != tools[names[j]].sum {
			return tools[names[i]].sum > tools[names[j]].sum
		}
		return names[i] < names[j]
	})
	for _, n := range names {
		a := tools[n]
		sum, max := a.sum, a.max
		out = append(out, UsageOperation{Operation: "execute_tool", Name: n, Metric: "gen_ai.execute_tool.duration",
			Count: a.count, Unit: "{tool_call}", DurationSeconds: &sum, MaxSeconds: &max, Source: "tool hooks",
			Note: "tool hook spans record no error result"})
	}

	agents := map[string]*UsageOperation{}
	var order []string
	for _, r := range liveRuns(in.Record) {
		for _, q := range r.Requirements {
			for _, at := range q.Attempts {
				name := at.Team
				if name == "" {
					name = "coordinator"
					if at.AssignmentID != "" {
						name = "conductor assignment"
					}
				}
				op := agents[name]
				if op == nil {
					zero := 0
					op = &UsageOperation{Operation: "invoke_agent", Name: name, Metric: "gen_ai.invoke_agent.duration",
						Unit: "{attempt}", Errors: &zero, ErrorTypes: map[string]int{}, Source: "run record attempts"}
					agents[name] = op
					order = append(order, name)
				}
				op.Count++
				if at.DurationMS != nil {
					d := float64(*at.DurationMS) / 1000
					if op.DurationSeconds == nil {
						sum, mx := 0.0, 0.0
						op.DurationSeconds, op.MaxSeconds = &sum, &mx
					}
					*op.DurationSeconds += d
					if d > *op.MaxSeconds {
						*op.MaxSeconds = d
					}
				} else {
					op.Note = "some attempts have no recorded duration"
				}
				switch at.Outcome {
				case "passed":
				case "", "open":
					op.NoResult++
				default:
					*op.Errors++
					op.ErrorTypes[at.Outcome]++
				}
			}
		}
	}
	for _, n := range order {
		op := agents[n]
		if len(op.ErrorTypes) == 0 {
			op.ErrorTypes = nil
		}
		out = append(out, *op)
	}
	return out
}

// usageSummary is the section's one-sentence answer.
func usageSummary(s UsageSection, c capture) string {
	f := s.Plan.Figures
	switch s.State {
	case stateUnavailable:
		if !s.History {
			return "Plan usage is unavailable until tool history loads; the run record holds no per-request usage for this execution."
		}
		return "Plan usage is unavailable: no session transcript or recorded request was found. Usage is unknown, not zero."
	}
	msg := fmt.Sprintf("%s attributed to this plan", countText(f.Requests, "request", "requests"))
	if f.InputTokens != nil && f.OutputTokens != nil {
		msg += fmt.Sprintf(": %s input tokens and %s output tokens", groupDigits(*f.InputTokens), groupDigits(*f.OutputTokens))
	}
	if s.State == statePartial {
		msg += "; capture is partial, so these are lower bounds"
	}
	msg += "."
	for _, it := range s.SessionCost.Items {
		msg += fmt.Sprintf(" Whole-session cost of $%.2f covers %s, not just this plan.", it.CostUSD, countText(it.Sessions, "session", "sessions"))
	}
	return msg
}

func writeUsageSection(b *strings.Builder, s UsageSection) {
	b.WriteString(`<section class="report-section" id="report-usage" aria-labelledby="report-usage-h">`)
	b.WriteString(accountingCSS("report-usage"))
	b.WriteString(`<h2 id="report-usage-h">Usage ` + stateBadge(s.State) + `</h2>`)
	b.WriteString(`<p class="ra-sum">` + html.EscapeString(s.Summary) + `</p>`)
	fmt.Fprintf(b, `<p class="ra-note">Attribution window: %s. Terms follow the <a href="%s" rel="noopener noreferrer">%s</a> %s (%s).</p>`,
		windowText(s.Window), html.EscapeString(s.Conventions.URL), html.EscapeString(s.Conventions.Name),
		html.EscapeString(s.Conventions.Version), html.EscapeString(s.Conventions.Status))

	// Exact, plan-attributed.
	b.WriteString(`<h3>Plan-attributed usage (exact) ` + stateBadge(s.Plan.State) + `</h3>`)
	writeFigureTiles(b, s.Plan.Figures, s.Plan.State == stateUnavailable)
	b.WriteString(`<p class="ra-note">` + html.EscapeString(s.Plan.Note))
	if s.Duplicates > 0 {
		fmt.Fprintf(b, `. %s appeared in more than one transcript and counted once`, html.EscapeString(countText(s.Duplicates, "request", "requests")))
	}
	if len(s.Plan.Figures.PriceSources) > 0 {
		b.WriteString(`. Cost source: ` + html.EscapeString(strings.Join(s.Plan.Figures.PriceSources, "; ")))
	}
	b.WriteString(`.</p>`)

	// Whole-session cost.
	b.WriteString(`<h3>Whole-session cost (not this plan's bill)</h3>`)
	if len(s.SessionCost.Items) == 0 {
		b.WriteString(`<p><span class="ra-unknown">unknown</span> — ` + html.EscapeString(s.SessionCost.Note) + `</p>`)
	} else {
		b.WriteString(`<ul class="ra-list">`)
		for _, it := range s.SessionCost.Items {
			fmt.Fprintf(b, `<li><b>$%.2f</b> for %s · %s · price source: %s</li>`, it.CostUSD,
				html.EscapeString(countText(it.Sessions, "whole session", "whole sessions")), html.EscapeString(it.Source), html.EscapeString(it.PriceSource))
		}
		b.WriteString(`</ul><p class="ra-note">` + html.EscapeString(s.SessionCost.Note) + `.</p>`)
	}

	// Allowance.
	b.WriteString(`<h3>Subscription allowance</h3>`)
	if len(s.Allowance.Observations) == 0 {
		b.WriteString(`<p><span class="ra-unknown">no reading</span> — no source reported an allowance in the attribution window.</p>`)
	} else {
		b.WriteString(`<ul class="ra-list">`)
		for _, a := range s.Allowance.Observations {
			win := a.Window
			if a.WindowMinutes > 0 {
				win += " window of " + windowMinutesText(a.WindowMinutes)
			}
			plan := ""
			if a.PlanType != "" {
				plan = ", plan " + a.PlanType
			}
			resets := ""
			if a.ResetsAt != nil {
				resets = ", resets " + timeTag(*a.ResetsAt)
			}
			fmt.Fprintf(b, `<li>%s %s%s%s: %.0f%% used at %s, %.0f%% at %s</li>`, html.EscapeString(a.Provider), html.EscapeString(win), html.EscapeString(plan),
				resets, a.FirstPercent, timeTag(a.FirstAt), a.LastPercent, timeTag(a.LastAt))
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`<p class="ra-note">` + html.EscapeString(s.Allowance.Note) + `.</p>`)

	// Estimated.
	if s.Estimated.Figures.Requests > 0 {
		b.WriteString(`<h3>Estimated allocations</h3>`)
		writeFigureTiles(b, s.Estimated.Figures, false)
		b.WriteString(`<p class="ra-note">` + html.EscapeString(s.Estimated.Note) + `.</p>`)
	}

	// By source.
	figLabels := []string{"Name", "State", "Requests", "Input tokens", "Cache read tokens", "Cache write tokens", "Output tokens", "Reasoning tokens", "Cost (USD)"}
	b.WriteString(`<h3>By source</h3>`)
	tableOpen(b, "Exact usage per source; sources overlap only where noted", figLabels...)
	for _, src := range s.Sources {
		cells := figureCells(src.Figures, src.State)
		name := html.EscapeString(src.Label)
		if src.Note != "" {
			name += `<br><span class="ra-note">` + html.EscapeString(src.Note) + `</span>`
		}
		tableRow(b, figLabels, append([]string{name, stateBadge(src.State)}, cells...)...)
	}
	tableClose(b)

	// By lane.
	if len(s.Agents) > 0 {
		b.WriteString(`<h3>By coordinator, team and agent</h3>`)
		labels := append([]string{"Lane"}, figLabels[1:]...)
		labels = append(labels, "Peak request input tokens")
		tableOpen(b, "Exact usage per lane", labels...)
		for _, r := range s.Agents {
			name := html.EscapeString(laneLabel(r.Label)) + ` <span class="ra-note">` + html.EscapeString(r.Kind)
			if r.Engine != "" {
				name += " · " + html.EscapeString(r.Engine)
			}
			name += `</span>`
			if r.Note != "" {
				name += `<br><span class="ra-note">` + html.EscapeString(r.Note) + `</span>`
			}
			peak := `<span class="ra-unknown">unknown</span>`
			if r.PeakContextTokens != nil {
				peak = groupDigits(*r.PeakContextTokens)
			}
			cells := append([]string{name, stateBadge(r.State)}, figureCells(r.Figures, r.State)...)
			tableRow(b, labels, append(cells, num(peak))...)
		}
		tableClose(b)
	}

	// By model.
	if len(s.Models) > 0 {
		b.WriteString(`<h3>By model and provider</h3>`)
		labels := append([]string{"Model"}, figLabels[1:]...)
		tableOpen(b, "Exact usage per model", labels...)
		for _, r := range s.Models {
			name := html.EscapeString(r.Label) + ` <span class="ra-note">` + html.EscapeString(joinOr([]string{r.Provider}, "provider not recorded"))
			if r.Provider == "" {
				name = html.EscapeString(r.Label) + ` <span class="ra-note">provider not recorded`
			}
			name += ` · ` + html.EscapeString(r.Source) + `</span>`
			tableRow(b, labels, append([]string{name, stateBadge(r.State)}, figureCells(r.Figures, r.State)...)...)
		}
		tableClose(b)
	}

	// Estimated breakdowns.
	if len(s.Tools) > 0 {
		fmt.Fprintf(b, `<details><summary>By tool (estimated, %s)</summary>`, html.EscapeString(countText(len(s.Tools), "tool", "tools")))
		labels := []string{"Tool", "Calls", "Share of attributed usage", "Context growth tokens", "Output tokens writing the call"}
		tableOpen(b, "Estimated allocation per tool", labels...)
		for _, t := range s.Tools {
			tableRow(b, labels, html.EscapeString(laneLabel(t.Tool)), num(groupDigits(int64(t.Calls))+" calls"), num(fmt.Sprintf("%.1f%%", t.SharePercent)),
				num(groupDigits(t.ContextGrowthTokens)), num(groupDigits(t.OutputTokens)))
		}
		tableClose(b)
		b.WriteString(`<p class="ra-note">Estimates: context growth is how much larger the next request became after the call, split evenly when one request made several calls; output is the request's output divided among its calls. Calls are counted exactly.</p></details>`)
	}
	if len(s.Wakeups) > 0 {
		fmt.Fprintf(b, `<details><summary>By wake-up (estimated, %s)</summary>`, html.EscapeString(countText(len(s.Wakeups), "kind", "kinds")))
		labels := []string{"Wake-up", "Fired (wake-ups)", "Ran", "Idle", "Coalesced", "Requests", "Input tokens", "Median request input tokens", "Share of plan usage"}
		tableOpen(b, "Estimated allocation per wake-up kind", labels...)
		for _, w := range s.Wakeups {
			tableRow(b, labels, html.EscapeString(w.Label), num(groupDigits(int64(w.Fired))), num(groupDigits(int64(w.Ran))), num(groupDigits(int64(w.Idle))),
				num(groupDigits(int64(w.Coalesced))), num(groupDigits(int64(w.Requests))), num(groupDigits(w.InputTokens)),
				num(groupDigits(w.MedianContextTokens)), num(fmt.Sprintf("%.1f%%", w.SharePercent)))
		}
		tableClose(b)
		b.WriteString(`<p class="ra-note">Estimates: a wake-up is charged everything its lane spent until the next wake-up. Coalesced firings arrived during a running turn and made no request; idle ones re-sent the context and made no tool call.</p></details>`)
	}

	// Operations.
	if len(s.Operations) > 0 {
		fmt.Fprintf(b, `<details><summary>Operations: count, duration and errors (%s)</summary>`, html.EscapeString(countText(len(s.Operations), "row", "rows")))
		labels := []string{"Operation", "Name", "Count", "Duration (sum)", "Longest", "Errors", "Source"}
		tableOpen(b, "Operation counts, durations and errors", labels...)
		for _, op := range s.Operations {
			dur, mx, errs := `<span class="ra-unknown">not recorded</span>`, `<span class="ra-unknown">not recorded</span>`, `<span class="ra-unknown">not recorded</span>`
			if op.DurationSeconds != nil {
				dur = durationText(*op.DurationSeconds)
			}
			if op.MaxSeconds != nil {
				mx = durationText(*op.MaxSeconds)
			}
			if op.Errors != nil {
				errs = groupDigits(int64(*op.Errors))
				if len(op.ErrorTypes) > 0 {
					var parts []string
					for k, v := range op.ErrorTypes {
						parts = append(parts, fmt.Sprintf("%s %d", k, v))
					}
					sort.Strings(parts)
					errs += ` <span class="ra-note">(` + html.EscapeString(strings.Join(parts, ", ")) + `)</span>`
				}
				if op.NoResult > 0 {
					errs += ` <span class="ra-note">` + html.EscapeString(countText(op.NoResult, "without a result", "without a result")) + `</span>`
				}
			}
			src := html.EscapeString(op.Source)
			if op.Note != "" {
				src += `<br><span class="ra-note">` + html.EscapeString(op.Note) + `</span>`
			}
			tableRow(b, labels, `<code>`+html.EscapeString(op.Operation)+`</code>`, html.EscapeString(laneLabel(op.Name)),
				num(groupDigits(int64(op.Count))+" "+html.EscapeString(op.Unit)), num(dur), num(mx), errs, src)
		}
		tableClose(b)
		b.WriteString(`<p class="ra-note">Durations are summed per row only; concurrent operations overlap in wall time and are never added into elapsed time.</p></details>`)
	}

	b.WriteString(`<details><summary>Definitions</summary><dl>`)
	for _, d := range s.Definitions {
		b.WriteString(`<dt>` + html.EscapeString(d.Term) + `</dt><dd>` + html.EscapeString(d.Definition) + `</dd>`)
	}
	b.WriteString(`</dl></details></section>`)
}

// writeFigureTiles renders the headline figures of one scope.
func writeFigureTiles(b *strings.Builder, f UsageFigures, unavailable bool) {
	b.WriteString(`<ul class="ra-figs" aria-label="Usage figures">`)
	req := groupDigits(int64(f.Requests))
	if unavailable {
		req = `<span class="ra-unknown">unknown</span>`
	}
	fmt.Fprintf(b, `<li><b>%s</b><span>requests</span></li>`, req)
	for _, x := range []struct {
		v          *int64
		name, unit string
	}{
		{f.InputTokens, figInput, "input tokens (incl. cache)"},
		{f.CacheReadTokens, figCacheRead, "of which cache read"},
		{f.CacheWriteTokens, figCacheWrite, "of which cache write"},
		{f.OutputTokens, figOutput, "output tokens (incl. reasoning)"},
		{f.ReasoningTokens, figReasoning, "of which reasoning"},
	} {
		fmt.Fprintf(b, `<li><b>%s</b><span>%s</span></li>`, figureCell(x.v, x.name, f), x.unit)
	}
	fmt.Fprintf(b, `<li><b>%s</b><span>cost (USD)</span></li>`, costCell(f))
	b.WriteString(`</ul>`)
}

// figureCells renders a figures row's numeric cells.
func figureCells(f UsageFigures, state string) []string {
	req := groupDigits(int64(f.Requests))
	if state == stateUnavailable || state == "missing" {
		req = `<span class="ra-unknown">unknown</span>`
	}
	return []string{num(req), num(figureCell(f.InputTokens, figInput, f)), num(figureCell(f.CacheReadTokens, figCacheRead, f)),
		num(figureCell(f.CacheWriteTokens, figCacheWrite, f)), num(figureCell(f.OutputTokens, figOutput, f)),
		num(figureCell(f.ReasoningTokens, figReasoning, f)), num(costCell(f))}
}

func durationText(sec float64) string {
	d := time.Duration(sec * float64(time.Second)).Round(time.Second)
	if d < time.Second && sec > 0 {
		return fmt.Sprintf("%.2fs", sec)
	}
	return d.String()
}

func windowMinutesText(m int) string {
	switch {
	case m%(24*60) == 0:
		return countText(m/(24*60), "day", "days")
	case m%60 == 0:
		return countText(m/60, "hour", "hours")
	}
	return countText(m, "minute", "minutes")
}
