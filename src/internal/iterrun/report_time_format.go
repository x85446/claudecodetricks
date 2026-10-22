package iterrun

import (
	"fmt"
	"html"
	"strings"
	"time"
)

// Formatting shared by the ownership, time and rework sections.

// LifecycleTerm is one explicit definition a section shows beside its
// figures.
type LifecycleTerm struct {
	Term    string `json:"term"`
	Meaning string `json:"meaning"`
}

// Section availability states. A section never disappears: it says which
// of these applies and why.
const (
	sectionAvailable   = "available"
	sectionPartial     = "partial"
	sectionUnavailable = "unavailable"
)

// lcDur renders a duration in milliseconds for reading: seconds under a
// minute, minutes and seconds under an hour, hours and minutes under a
// day, then days and hours.
func lcDur(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	d := time.Duration(ms) * time.Millisecond
	switch {
	case d < time.Minute:
		return fmt.Sprintf("%ds", int64(d.Round(time.Second)/time.Second))
	case d < time.Hour:
		d = d.Round(time.Second)
		return fmt.Sprintf("%dm %ds", int64(d/time.Minute), int64(d%time.Minute/time.Second))
	case d < 24*time.Hour:
		d = d.Round(time.Minute)
		return fmt.Sprintf("%dh %dm", int64(d/time.Hour), int64(d%time.Hour/time.Minute))
	}
	d = d.Round(time.Minute)
	return fmt.Sprintf("%dd %dh %dm", int64(d/(24*time.Hour)), int64(d%(24*time.Hour)/time.Hour), int64(d%time.Hour/time.Minute))
}

func lcMS(d time.Duration) int64 { return d.Milliseconds() }

// lcTime renders an instant as a <time> element in the viewer's zone, with
// the exact instant in its datetime attribute.
func lcTime(t time.Time) string {
	return `<time datetime="` + t.UTC().Format(time.RFC3339) + `">` + html.EscapeString(t.Local().Format("2006-01-02 15:04 MST")) + `</time>`
}

// lcCount renders a count with its unit, pluralized.
func lcCount(n int, unit string) string {
	if n == 1 {
		return "1 " + unit
	}
	if strings.HasSuffix(unit, "y") && !strings.ContainsAny(unit[len(unit)-2:len(unit)-1], "aeiou") {
		return fmt.Sprintf("%d %sies", n, unit[:len(unit)-1])
	}
	return fmt.Sprintf("%d %ss", n, unit)
}

// lcPercent renders part of whole as a percentage with one decimal place
// under ten percent.
func lcPercent(part, whole int64) string {
	if whole <= 0 {
		return "—"
	}
	p := 100 * float64(part) / float64(whole)
	if p > 0 && p < 10 {
		return fmt.Sprintf("%.1f%%", p)
	}
	return fmt.Sprintf("%.0f%%", p)
}

func lcEsc(s string) string { return html.EscapeString(s) }

// writeLcStatus writes a section's availability line.
func writeLcStatus(b *strings.Builder, prefix, status, note string) {
	label := map[string]string{sectionAvailable: "Available", sectionPartial: "Partial", sectionUnavailable: "Unavailable"}[status]
	if label == "" {
		label = "Unavailable"
	}
	fmt.Fprintf(b, `<p class="%s-status %s-%s" data-status="%s"><strong>%s.</strong> %s</p>`, prefix, prefix, status, status, label, lcEsc(note))
}

// writeLcTerms writes a section's definitions as a disclosure.
func writeLcTerms(b *strings.Builder, prefix string, terms []LifecycleTerm) {
	if len(terms) == 0 {
		return
	}
	fmt.Fprintf(b, `<details class="%s-terms"><summary>Definitions</summary><dl>`, prefix)
	for _, t := range terms {
		fmt.Fprintf(b, `<dt>%s</dt><dd>%s</dd>`, lcEsc(t.Term), lcEsc(t.Meaning))
	}
	b.WriteString(`</dl></details>`)
}

// lcSectionCSS is the styling the three sections share, scoped to each
// section id by its prefix.
func lcSectionCSS(id, prefix string) string {
	r := strings.NewReplacer("#S", "#"+id, "P-", prefix+"-")
	return r.Replace(`#S{margin:28px 0 0;overflow-wrap:anywhere}
#S h3{font-size:12px;letter-spacing:.04em;text-transform:uppercase;color:var(--text-dim);font-weight:600;margin:18px 0 8px}
#S .P-status{font-size:13px;margin:0 0 10px;padding:8px 12px;border-radius:6px;border:1px solid var(--border);background:var(--surface)}
#S .P-partial{border-left:3px solid var(--warn)}
#S .P-unavailable{border-left:3px solid var(--text-faint)}
#S .P-available{border-left:3px solid var(--good)}
#S table{width:100%;border-collapse:collapse;font-size:13px;background:var(--surface);border:1px solid var(--border);border-radius:8px}
#S th,#S td{text-align:left;padding:6px 10px;border-top:1px solid var(--border);vertical-align:top}
#S thead th{border-top:0;font-size:11px;letter-spacing:.04em;text-transform:uppercase;color:var(--text-dim);font-weight:600}
#S td.num,#S th.num{text-align:right;font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-variant-numeric:tabular-nums;white-space:nowrap}
#S .P-note{display:block;font-size:12px;color:var(--text-dim)}
#S ol,#S ul{margin:0;padding-left:20px;font-size:13px}
#S li{margin:0 0 8px}
#S .P-dim{color:var(--text-dim)}
#S details.P-terms{margin:14px 0 0;font-size:13px}
#S details summary{cursor:pointer;color:var(--accent);min-height:24px}
#S details summary:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
#S dl{margin:8px 0 0}
#S dt{font-weight:600;margin-top:8px}
#S dd{margin:2px 0 0;color:var(--text-dim)}
#S .vh{position:absolute;width:1px;height:1px;overflow:hidden;clip:rect(0 0 0 0);white-space:nowrap}
`)
}
