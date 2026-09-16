package iterrun

import (
	"fmt"
	"html"
	"strings"
)

// tokenPanelCSS is appended to the timeline page's own stylesheet. Kept
// beside the markup it styles rather than in timelineHead's block, which
// is already long enough to make a change there risky.
const tokenPanelCSS = `
.tk-top{display:flex;flex-wrap:wrap;gap:18px;align-items:baseline;margin:0 0 14px;padding:12px 15px;background:var(--surface);border:1px solid var(--border);border-left:3px solid var(--accent);border-radius:8px}
.tk-fig{display:flex;flex-direction:column;gap:2px}
.tk-fig b{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-variant-numeric:tabular-nums;font-size:16px;font-weight:700;color:var(--accent);line-height:1.1}
.tk-fig span{font-size:10.5px;letter-spacing:.05em;text-transform:uppercase;color:var(--text-dim)}
.tk-note{font-size:11.5px;color:var(--text-dim);margin:-6px 0 16px}
.tk-h{font-size:11px;letter-spacing:.05em;text-transform:uppercase;color:var(--text-dim);font-weight:600;margin:16px 0 6px}
.tk-scroll{overflow-x:auto;-webkit-overflow-scrolling:touch;margin:0 0 6px}
table.tk{width:100%;border-collapse:collapse;font-size:12.5px;margin:0}
table.tk th{text-align:right;font-weight:600;font-size:10.5px;letter-spacing:.04em;text-transform:uppercase;color:var(--text-faint);padding:0 8px 5px;border-bottom:1px solid var(--border);white-space:nowrap}
table.tk th:first-child,table.tk td:first-child{text-align:left}
table.tk td{text-align:right;padding:5px 8px;border-bottom:1px solid var(--border);font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-variant-numeric:tabular-nums;white-space:nowrap}
table.tk td:first-child{font-family:-apple-system,BlinkMacSystemFont,"Segoe UI",sans-serif}
table.tk tr:last-child td{border-bottom:none}
table.tk tr.tk-coord td:first-child{font-weight:700;color:var(--accent)}
.tk-bar{position:relative;display:block;width:100%;min-width:64px;height:9px;background:var(--surface-2);border:1px solid var(--border);border-radius:3px;overflow:hidden}
.tk-bar i{position:absolute;left:0;top:0;bottom:0;background:var(--busy);display:block}
.tk-bar.tk-warn i{background:var(--warn)}
td.tk-barcell{width:110px;padding-right:4px}
td.tk-pct,th.tk-pct{width:48px;padding-left:0}
.tk-dim{color:var(--text-faint)}
.tk-miss{color:var(--warn)}
@media(max-width:620px){table.tk{font-size:11.5px}table.tk th,table.tk td{padding:4px 5px}td.tk-barcell{width:56px}}
`

// humanTokens renders a token count the way a reader scans it. Exact below
// 10,000 — a 900-token difference matters when you are looking at one
// wake-up — then k and M, which is the granularity the rest of the numbers
// on this page are trustworthy at anyway.
func humanTokens(n int64) string {
	switch {
	case n < 0:
		return "0"
	case n < 10_000:
		return fmt.Sprintf("%d", n)
	case n < 1_000_000:
		return fmt.Sprintf("%.0fk", float64(n)/1000)
	case n < 100_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	default:
		return fmt.Sprintf("%.0fM", float64(n)/1e6)
	}
}

func shareBar(pct float64, warn bool) string {
	if pct < 0 {
		pct = 0
	}
	if pct > 100 {
		pct = 100
	}
	cls := "tk-bar"
	if warn {
		cls += " tk-warn"
	}
	return fmt.Sprintf(`<span class="%s"><i style="width:%.1f%%"></i></span>`, cls, pct)
}

// writeTokenPanel renders the whole token section: who spent it, what it
// went on, and what each wake-up cost. Writes nothing at all when no
// transcript could be found for any lane — an empty table implying zero
// spend would be a lie, and the plan may simply predate this data or have
// run on a machine whose transcripts are elsewhere.
func writeTokenPanel(b *strings.Builder, pt *PlanTokens) {
	if pt == nil || pt.Total.Messages == 0 {
		return
	}

	b.WriteString(`<h2>Token spend</h2>`)

	b.WriteString(`<div class="tk-top">`)
	fmt.Fprintf(b, `<div class="tk-fig"><b>%s</b><span>context sent</span></div>`, humanTokens(pt.Total.Context()))
	fmt.Fprintf(b, `<div class="tk-fig"><b>%s</b><span>generated</span></div>`, humanTokens(pt.Total.Output))
	fmt.Fprintf(b, `<div class="tk-fig"><b>%s</b><span>of that, thinking</span></div>`, humanTokens(pt.Total.Thinking))
	fmt.Fprintf(b, `<div class="tk-fig"><b>%d</b><span>requests</span></div>`, pt.Total.Messages)
	if pt.HaveCost {
		fmt.Fprintf(b, `<div class="tk-fig"><b>$%.2f</b><span>session cost</span></div>`, pt.CostUSD)
	}
	b.WriteString(`</div>`)

	// Context is what dominates the bill and it is the least intuitive
	// number on the page, so say what it means once, here.
	note := `<b>Context sent</b> is everything re-sent to the model to make each request — it grows every turn and is what a long run actually pays for. <b>Generated</b> is what the model wrote back.`
	if pt.HaveCost {
		note += ` Session cost is Claude Code's own figure for the session` +
			plural(len(pt.Sessions)) + ` this plan ran in`
		if pt.CostPartial {
			note += `, which covers everything else those sessions did too — it is not this plan's bill`
		}
		note += `.`
	}
	fmt.Fprintf(b, `<p class="tk-note">%s</p>`, note)

	// --- who spent it -----------------------------------------------
	b.WriteString(`<div class="tk-h">Who spent it</div>`)
	b.WriteString(`<div class="tk-scroll"><table class="tk"><tr><th>lane</th><th>share</th><th class="tk-pct"></th><th>context</th><th>generated</th><th>thinking</th><th>peak ctx</th><th>reqs</th></tr>`)
	for _, l := range pt.Lanes {
		cls := ""
		if l.Key == "" {
			cls = ` class="tk-coord"`
		}
		label := l.Label
		if l.Missing {
			fmt.Fprintf(b, `<tr%s><td>%s</td><td colspan="7" class="tk-miss" style="text-align:left">no transcript found for this lane — its spend is not counted above</td></tr>`,
				cls, html.EscapeString(label))
			continue
		}
		fmt.Fprintf(b, `<tr%s><td>%s</td><td class="tk-barcell">%s</td><td class="tk-pct">%.1f%%</td><td>%s</td><td>%s</td><td class="tk-dim">%s</td><td>%s</td><td class="tk-dim">%d</td></tr>`,
			cls, html.EscapeString(label), shareBar(l.Share, false), l.Share,
			humanTokens(l.Usage.Context()), humanTokens(l.Usage.Output),
			humanTokens(l.Usage.Thinking), humanTokens(l.PeakContext), l.Usage.Messages)
	}
	b.WriteString(`</table></div>`)

	// --- what it went on --------------------------------------------
	if len(pt.Tools) > 0 {
		b.WriteString(`<div class="tk-h">What it went on, by tool</div>`)
		b.WriteString(`<div class="tk-scroll"><table class="tk"><tr><th>tool</th><th>share</th><th class="tk-pct"></th><th>calls</th><th>result &rarr; context</th><th>writing the call</th><th>per call</th></tr>`)
		shown := 0
		for _, t := range pt.Tools {
			if shown >= 12 {
				break
			}
			shown++
			per := int64(0)
			if t.Calls > 0 {
				per = (t.ContextGrowth + t.Emitting) / int64(t.Calls)
			}
			fmt.Fprintf(b, `<tr><td>%s</td><td class="tk-barcell">%s</td><td class="tk-pct">%.1f%%</td><td class="tk-dim">%d</td><td>%s</td><td>%s</td><td class="tk-dim">%s</td></tr>`,
				html.EscapeString(t.Tool), shareBar(t.Share, false), t.Share, t.Calls,
				humanTokens(t.ContextGrowth), humanTokens(t.Emitting), humanTokens(per))
		}
		b.WriteString(`</table></div>`)
		b.WriteString(`<p class="tk-note"><b>result &rarr; context</b> is how much bigger the next request got because the call happened — the real price of a read, paid again on every later turn. <b>writing the call</b> is what the model spent composing it. Where one request made several calls the growth is split evenly between them, so a single row is an attribution, not a receipt.</p>`)
	}

	// --- wake-ups ---------------------------------------------------
	if len(pt.Groups) > 0 {
		b.WriteString(`<div class="tk-h">What each wake-up cost</div>`)
		b.WriteString(`<div class="tk-scroll"><table class="tk"><tr><th>wake-up</th><th>share</th><th class="tk-pct"></th><th>fired</th><th>ran</th><th>idle</th><th>coalesced</th><th>median ctx</th><th>context</th></tr>`)
		for _, g := range pt.Groups {
			idle := fmt.Sprintf(`%d`, g.Idle)
			if g.Idle > 0 {
				idle = fmt.Sprintf(`<span class="tk-miss">%d</span>`, g.Idle)
			}
			fmt.Fprintf(b, `<tr><td>%s</td><td class="tk-barcell">%s</td><td class="tk-pct">%.1f%%</td><td class="tk-dim">%d</td><td>%d</td><td>%s</td><td class="tk-dim">%d</td><td>%s</td><td>%s</td></tr>`,
				html.EscapeString(g.Label), shareBar(g.Share, g.Idle > 0), g.Share,
				g.Count, g.Ran, idle, g.Coalesced,
				humanTokens(g.Median), humanTokens(g.Usage.Context()))
		}
		b.WriteString(`</table></div>`)
		b.WriteString(`<p class="tk-note">A <b>coalesced</b> firing arrived while the previous turn was still running, never got its own request, and cost nothing. An <b>idle</b> one did run — it re-sent the whole context and then made no tool call, which is the expensive kind. <span class="tk-dim">(nudge)</span> is the coordinator asking a team for something; everything downstream of that ask is counted against it.</p>`)
	}
}
