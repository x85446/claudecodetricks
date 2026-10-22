package iterrun

import (
	"fmt"
	"html"
	"sort"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// Section states shared by the accounting sections (coverage, usage,
// delivery). A section always renders; its state says how much of what it
// describes was observed.
const (
	stateComplete    = "complete"
	statePartial     = "partial"
	stateUnavailable = "unavailable"
)

// otelConventions names the metric vocabulary the usage section borrows.
// The GenAI conventions are in development, so the version is explicit.
var otelConventions = UsageConventions{
	Name:    "OpenTelemetry GenAI semantic conventions",
	Version: "1.44.0",
	Status:  "development",
	URL:     "https://github.com/open-telemetry/semantic-conventions-genai/blob/main/docs/gen-ai/gen-ai-metrics.md",
}

// UsageConventions identifies the metric vocabulary and its version.
type UsageConventions struct {
	Name    string `json:"name"`
	Version string `json:"version"`
	Status  string `json:"status"`
	URL     string `json:"url"`
}

// AccountingWindow is the attribution interval: the plan execution span
// every plan-scoped figure is windowed to.
type AccountingWindow struct {
	From *time.Time `json:"from,omitempty"`
	To   *time.Time `json:"to,omitempty"`
	// Open is set when the execution has not finished and To is the time
	// of the read.
	Open bool `json:"open"`
	// Basis names the plan fields the window comes from.
	Basis string `json:"basis"`
}

// accountingWindow derives the attribution window from the plan file.
func accountingWindow(in ReportInput) *AccountingWindow {
	w := &AccountingWindow{Basis: "plan Executing: (or Started:) to Finished:"}
	if from, ok := in.Plan.EffectiveStart(); ok {
		w.From = &from
	}
	if to, ok := in.Plan.FinishedAt(); ok {
		w.To = &to
	} else if to, ok := planWindowEnd(in.Plan); ok {
		w.To = &to
		w.Basis = "plan Executing: (or Started:) to the archive time; the plan recorded no Finished:"
	} else {
		now := in.Now.UTC()
		w.To, w.Open = &now, true
		w.Basis = "plan Executing: (or Started:) to the time of this read; the plan has not finished"
	}
	if w.From == nil {
		w.Basis = "plan start not recorded; the window has no lower bound"
	}
	return w
}

// UsageFigures are token and cost totals named after the OpenTelemetry
// GenAI usage counters. A nil figure is unknown, never zero: no source
// reported it. Partial names figures some counted requests lacked, so the
// figure is a lower bound.
type UsageFigures struct {
	// Requests counts provider inference calls ({inference_call}).
	Requests int `json:"requests"`
	// InputTokens includes cache reads and cache writes
	// (gen_ai.client.inference.usage.input_tokens).
	InputTokens *int64 `json:"input_tokens,omitempty"`
	// CacheReadTokens and CacheWriteTokens are subsets of InputTokens.
	CacheReadTokens  *int64 `json:"cache_read_input_tokens,omitempty"`
	CacheWriteTokens *int64 `json:"cache_write_input_tokens,omitempty"`
	// OutputTokens includes reasoning
	// (gen_ai.client.inference.usage.output_tokens).
	OutputTokens *int64 `json:"output_tokens,omitempty"`
	// ReasoningTokens is a subset of OutputTokens.
	ReasoningTokens *int64   `json:"reasoning_output_tokens,omitempty"`
	CostUSD         *float64 `json:"cost_usd,omitempty"`
	// PriceSources names where each reported cost came from.
	PriceSources []string `json:"price_sources,omitempty"`
	Partial      []string `json:"partial,omitempty"`
}

// usage field names, in display order.
const (
	figInput      = "input_tokens"
	figCacheRead  = "cache_read_input_tokens"
	figCacheWrite = "cache_write_input_tokens"
	figOutput     = "output_tokens"
	figReasoning  = "reasoning_output_tokens"
	figCost       = "cost_usd"
)

func addFigure(dst **int64, v *int64) {
	if v == nil {
		return
	}
	if *dst == nil {
		z := int64(0)
		*dst = &z
	}
	**dst += *v
}

func (f *UsageFigures) markPartial(name string) {
	for _, p := range f.Partial {
		if p == name {
			return
		}
	}
	f.Partial = append(f.Partial, name)
}

// figureRequest is one request's usage, normalized: nil fields were not
// reported.
type figureRequest struct {
	input, cacheRead, cacheWrite, output, reasoning *int64
	cost                                            *float64
	priceSource                                     string
}

// add counts one request. A field it lacks makes that total partial.
func (f *UsageFigures) add(r figureRequest) {
	f.Requests++
	for _, x := range []struct {
		dst  **int64
		v    *int64
		name string
	}{
		{&f.InputTokens, r.input, figInput}, {&f.CacheReadTokens, r.cacheRead, figCacheRead},
		{&f.CacheWriteTokens, r.cacheWrite, figCacheWrite}, {&f.OutputTokens, r.output, figOutput},
		{&f.ReasoningTokens, r.reasoning, figReasoning},
	} {
		addFigure(x.dst, x.v)
		if x.v == nil {
			f.markPartial(x.name)
		}
	}
	if r.cost == nil {
		f.markPartial(figCost)
	} else {
		if f.CostUSD == nil {
			z := 0.0
			f.CostUSD = &z
		}
		*f.CostUSD += *r.cost
		src := r.priceSource
		if src == "" {
			src = "price source not recorded"
		}
		if !containsString(f.PriceSources, src) {
			f.PriceSources = append(f.PriceSources, src)
		}
	}
}

// merge adds another set of figures.
func (f *UsageFigures) merge(o UsageFigures) {
	if o.Requests == 0 {
		return
	}
	if f.Requests > 0 {
		// A figure one side lacked entirely is partial in the sum.
		for _, x := range []struct {
			a, b *int64
			name string
		}{
			{f.InputTokens, o.InputTokens, figInput}, {f.CacheReadTokens, o.CacheReadTokens, figCacheRead},
			{f.CacheWriteTokens, o.CacheWriteTokens, figCacheWrite}, {f.OutputTokens, o.OutputTokens, figOutput},
			{f.ReasoningTokens, o.ReasoningTokens, figReasoning},
		} {
			if (x.a == nil) != (x.b == nil) {
				f.markPartial(x.name)
			}
		}
		if (f.CostUSD == nil) != (o.CostUSD == nil) {
			f.markPartial(figCost)
		}
	}
	f.Requests += o.Requests
	addFigure(&f.InputTokens, o.InputTokens)
	addFigure(&f.CacheReadTokens, o.CacheReadTokens)
	addFigure(&f.CacheWriteTokens, o.CacheWriteTokens)
	addFigure(&f.OutputTokens, o.OutputTokens)
	addFigure(&f.ReasoningTokens, o.ReasoningTokens)
	if o.CostUSD != nil {
		if f.CostUSD == nil {
			z := 0.0
			f.CostUSD = &z
		}
		*f.CostUSD += *o.CostUSD
	}
	for _, p := range o.Partial {
		f.markPartial(p)
	}
	for _, s := range o.PriceSources {
		if !containsString(f.PriceSources, s) {
			f.PriceSources = append(f.PriceSources, s)
		}
	}
}

func containsString(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func i64(v int64) *int64 { return &v }

// tokenFigures converts transcript counters to OTel-shaped figures.
// Transcripts carry no per-request cost, so cost stays unknown. Reasoning
// is known only for the requests that reported it.
func tokenFigures(u TokenUsage, reasoningKnown int) UsageFigures {
	if u.Messages == 0 {
		return UsageFigures{}
	}
	f := UsageFigures{
		Requests:         u.Messages,
		InputTokens:      i64(u.Context()),
		CacheReadTokens:  i64(u.CacheRead),
		CacheWriteTokens: i64(u.CacheWrite),
		OutputTokens:     i64(u.Output),
		Partial:          []string{figCost},
	}
	if reasoningKnown > 0 {
		f.ReasoningTokens = i64(u.Thinking)
	}
	if reasoningKnown < u.Messages {
		f.markPartial(figReasoning)
	}
	return f
}

// recordRequest normalizes one run-record usage observation.
func recordRequest(u runrecord.Usage) figureRequest {
	return figureRequest{input: u.InputTokens, cacheRead: u.CacheReadTokens, cacheWrite: u.CacheWriteTokens,
		output: u.OutputTokens, reasoning: u.ReasoningTokens, cost: u.CostUSD, priceSource: u.PriceSource}
}

// liveRuns returns the record's runs that describe this execution.
func liveRuns(rep RecordReport) []runrecord.Run {
	var out []runrecord.Run
	for _, r := range rep.Runs {
		if !r.Superseded {
			out = append(out, r)
		}
	}
	return out
}

// laneKind names what a usage lane is: the coordinator, a team of a teamed
// plan, or a subagent of a flat plan. A flat plan's subagents are never
// called teams.
func laneKind(key string, plan PlanSummary) string {
	switch {
	case key == "":
		return "coordinator"
	case plan.Teamed || plan.HasTeams:
		return "team"
	}
	return "agent"
}

// --- formatting ----------------------------------------------------------

// groupDigits writes an integer with thousands separators.
func groupDigits(n int64) string {
	neg := n < 0
	if neg {
		n = -n
	}
	s := fmt.Sprintf("%d", n)
	var b strings.Builder
	for i, c := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	if neg {
		return "-" + b.String()
	}
	return b.String()
}

// countText renders a count with its unit, singular or plural.
func countText(n int, one, many string) string {
	if n == 1 {
		return "1 " + one
	}
	return groupDigits(int64(n)) + " " + many
}

// figureCell renders one token figure: the number, "unknown" when absent,
// and a partial marker when some requests lacked it.
func figureCell(v *int64, name string, f UsageFigures) string {
	if v == nil {
		return `<span class="ra-unknown">unknown</span>`
	}
	s := groupDigits(*v)
	if containsString(f.Partial, name) {
		s += ` <span class="ra-partial" title="some requests did not report this figure; the total is a lower bound">partial</span>`
	}
	return s
}

func costCell(f UsageFigures) string {
	if f.CostUSD == nil {
		return `<span class="ra-unknown">unknown</span>`
	}
	s := fmt.Sprintf("$%.2f", *f.CostUSD)
	if containsString(f.Partial, figCost) {
		s += ` <span class="ra-partial" title="some requests reported no cost; the total is a lower bound">partial</span>`
	}
	return s
}

func reportTime(t time.Time) string {
	if t.IsZero() {
		return "time not recorded"
	}
	return t.UTC().Format("2006-01-02 15:04:05Z")
}

func timeTag(t time.Time) string {
	if t.IsZero() {
		return `<span class="ra-unknown">time not recorded</span>`
	}
	return `<time datetime="` + t.UTC().Format(time.RFC3339) + `">` + reportTime(t) + `</time>`
}

func windowText(w *AccountingWindow) string {
	if w == nil {
		return "not recorded"
	}
	from := `<span class="ra-unknown">start not recorded</span>`
	if w.From != nil {
		from = timeTag(*w.From)
	}
	to := `<span class="ra-unknown">end not recorded</span>`
	if w.To != nil {
		to = timeTag(*w.To)
		if w.Open {
			to += " (still running; read time)"
		}
	}
	return from + " to " + to
}

// stateBadge renders a state as text plus color; the word carries the
// meaning, the color only repeats it.
func stateBadge(state string) string {
	cls := map[string]string{
		stateComplete: "ra-good", "captured": "ra-good", "passed": "ra-good", "available": "ra-good",
		statePartial: "ra-warn", "pending": "ra-warn", "stale": "ra-warn", "estimated": "ra-warn",
		"failed": "ra-bad", "missing": "ra-bad",
	}[state]
	if cls == "" {
		cls = "ra-dim"
	}
	label := strings.ReplaceAll(state, "_", " ")
	return `<span class="ra-badge ` + cls + `">` + html.EscapeString(label) + `</span>`
}

// accountingCSS is the shared stylesheet for one accounting section, scoped
// to that section's id. At narrow widths each table row becomes a stacked
// card, so no table forces horizontal scrolling; explicit ARIA roles keep
// the table semantics the display change would otherwise drop.
func accountingCSS(id string) string {
	s := `
#ID{margin:24px 0;padding:16px;border:1px solid var(--border);border-radius:8px;background:var(--surface);overflow-wrap:anywhere}
#ID h2{margin:0 0 6px;font-size:18px;display:flex;flex-wrap:wrap;gap:8px;align-items:center}
#ID h3{margin:18px 0 6px;font-size:13px;letter-spacing:.04em;text-transform:uppercase;color:var(--text-dim)}
#ID p{margin:6px 0;line-height:1.45}
#ID .ra-sum{font-size:14px}
#ID .ra-note{font-size:12.5px;color:var(--text-dim)}
#ID .ra-unknown{color:var(--text-dim);font-style:italic}
#ID .ra-partial{color:var(--warn);font-size:11px}
#ID .ra-figs{display:grid;grid-template-columns:repeat(auto-fit,minmax(140px,1fr));gap:8px;margin:10px 0;padding:0;list-style:none}
#ID .ra-figs li{padding:8px 10px;border:1px solid var(--border);border-radius:6px;background:var(--surface-2)}
#ID .ra-figs b{display:block;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-variant-numeric:tabular-nums;font-size:15px}
#ID .ra-figs span{font-size:11.5px;color:var(--text-dim)}
#ID span.ra-badge{display:inline-block;padding:1px 8px;border-radius:10px;font-size:11.5px;font-weight:600;letter-spacing:.03em;color:var(--text);border:1px solid var(--border);background:var(--surface-2)}
#ID span.ra-good{border-color:var(--good);background:var(--good-bg,var(--surface-2))}
#ID span.ra-warn{border-color:var(--warn);background:var(--warn-bg,var(--surface-2))}
#ID span.ra-bad{border-color:var(--danger);background:var(--danger-bg,var(--surface-2))}
#ID table{width:100%;border-collapse:collapse;font-size:12.5px;margin:4px 0 8px}
#ID th{text-align:left;font-size:11px;letter-spacing:.03em;text-transform:uppercase;color:var(--text-dim);font-weight:600;padding:4px 6px;border-bottom:1px solid var(--border)}
#ID td{padding:5px 6px;border-bottom:1px solid var(--border);vertical-align:top}
#ID td.ra-n{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-variant-numeric:tabular-nums;white-space:nowrap}
#ID details{margin:8px 0}
#ID summary{cursor:pointer;color:var(--accent);font-size:13px;min-height:24px;line-height:24px}
#ID summary:focus-visible,#ID a:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
#ID a{color:var(--accent)}
#ID dl{margin:6px 0;font-size:12.5px}
#ID dt{font-weight:600;margin-top:6px}
#ID dd{margin:2px 0 0 0;color:var(--text-dim)}
#ID ul.ra-list{margin:4px 0;padding-left:18px;font-size:12.5px}
#ID code{font-size:11.5px}
@media(max-width:600px){
#ID{padding:12px}
#ID thead{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0)}
#ID table,#ID tbody,#ID tr,#ID td{display:block;width:auto}
#ID tr{border:1px solid var(--border);border-radius:6px;margin:0 0 8px;padding:4px 0}
#ID td{border:none;display:flex;gap:8px;justify-content:space-between;text-align:right}
#ID td::before{content:attr(data-label);color:var(--text-dim);font-size:11px;text-transform:uppercase;letter-spacing:.03em;text-align:left;flex:0 0 42%}
#ID td.ra-n{white-space:normal}
}
`
	return "<style>" + strings.ReplaceAll(s, "#ID", "#"+id) + "</style>"
}

// table helpers keep explicit roles so the narrow-screen card layout keeps
// its table semantics.
func tableOpen(b *strings.Builder, caption string, heads ...string) {
	b.WriteString(`<table role="table"><caption class="ra-note" style="text-align:left;caption-side:top">` + html.EscapeString(caption) + `</caption><thead role="rowgroup"><tr role="row">`)
	for _, h := range heads {
		b.WriteString(`<th role="columnheader" scope="col">` + html.EscapeString(h) + `</th>`)
	}
	b.WriteString(`</tr></thead><tbody role="rowgroup">`)
}

// tableRow writes one row; cells are pre-escaped HTML, labels plain text.
func tableRow(b *strings.Builder, labels []string, cells ...string) {
	b.WriteString(`<tr role="row">`)
	for i, c := range cells {
		cls := ""
		if strings.HasPrefix(c, "\x00n") {
			c, cls = c[2:], ` class="ra-n"`
		}
		label := ""
		if i < len(labels) {
			label = labels[i]
		}
		b.WriteString(`<td role="cell"` + cls + ` data-label="` + html.EscapeString(label) + `">` + c + `</td>`)
	}
	b.WriteString(`</tr>`)
}

// num marks a table cell as numeric.
func num(s string) string { return "\x00n" + s }

func tableClose(b *strings.Builder) { b.WriteString(`</tbody></table>`) }

func sortedStrings(set map[string]bool) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		if k != "" {
			out = append(out, k)
		}
	}
	sort.Strings(out)
	return out
}

func joinOr(list []string, none string) string {
	if len(list) == 0 {
		return none
	}
	return strings.Join(list, ", ")
}
