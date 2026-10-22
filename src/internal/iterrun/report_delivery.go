package iterrun

import (
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// DeliverySection is the plan report's step-13 section: Delivery outcomes: commit, PR, review/CI, merge, install/deploy and runtime verification.
//
// Every stage stands alone: a green CI result says nothing about an
// installation, and an installation says nothing about the running
// target. Each stage keeps what counts as proof (intended acceptance), the
// recorded observations (observed proof) and its open gate apart. Reading
// the section publishes, deploys and changes nothing.
type DeliverySection struct {
	State   string          `json:"state"`
	Summary string          `json:"summary"`
	Stages  []DeliveryStage `json:"stages"`
	// Outstanding lists stages whose latest observation is pending,
	// failed, stale or without a result.
	Outstanding []DeliveryGate `json:"outstanding"`
	// Imported are plan-file notes that mention delivery. They are prose,
	// not observations, and never set a stage's state.
	Imported []DeliveryNote `json:"imported,omitempty"`
	Error    string         `json:"error,omitempty"`
}

// DeliveryStage is one delivery stage.
type DeliveryStage struct {
	Stage string `json:"stage"`
	Label string `json:"label"`
	// Proof is the stage's intended acceptance: what an observation must
	// show for the stage to count.
	Proof string `json:"proof"`
	// State is passed, failed, pending, unknown, stale or not_recorded.
	State  string               `json:"state"`
	Latest *DeliveryObservation `json:"latest,omitempty"`
	// Observed holds every recorded result, oldest first.
	Observed []DeliveryObservation `json:"observed"`
	// Intended holds pending declarations: a gate someone recorded as
	// expected, with no result yet.
	Intended []DeliveryObservation `json:"intended,omitempty"`
	Stale    string                `json:"stale,omitempty"`
	Note     string                `json:"note,omitempty"`
}

// DeliveryObservation is one recorded delivery observation.
type DeliveryObservation struct {
	At           time.Time `json:"at"`
	Result       string    `json:"result"`
	Revision     string    `json:"revision,omitempty"`
	Ref          string    `json:"ref,omitempty"`
	Target       string    `json:"target,omitempty"`
	ArtifactPath string    `json:"artifact_path,omitempty"`
	URL          string    `json:"url,omitempty"`
	Evidence     string    `json:"evidence,omitempty"`
	Source       string    `json:"source"`
	Provenance   string    `json:"provenance"`
	RunID        string    `json:"run_id"`
	EventID      string    `json:"event_id"`
}

// DeliveryGate is one stage that has not passed.
type DeliveryGate struct {
	Stage  string `json:"stage"`
	Label  string `json:"label"`
	State  string `json:"state"`
	Reason string `json:"reason"`
}

// DeliveryNote is an imported plan-file note that mentions delivery.
type DeliveryNote struct {
	Text       string    `json:"text"`
	At         time.Time `json:"at"`
	Provenance string    `json:"provenance"`
}

var deliveryStageInfo = []struct{ stage, label, proof string }{
	{"source_commit", "Implemented", "a commit holding the change, named by revision"},
	{"pr", "Pull request", "a pull or merge request opened for the change"},
	{"review", "Reviewed", "a review decision on that pull request"},
	{"ci", "Tested (CI)", "a CI or test-suite result for a named revision"},
	{"merge", "Merged", "a merge commit on the target branch"},
	{"install", "Installed", "an installed artifact on a named target, by revision"},
	{"deploy", "Deployed", "a deployment to a named target, by revision"},
	{"runtime_verification", "Runtime verified", "an observation from the running target at the installed or deployed revision"},
}

var reDeliveryNote = regexp.MustCompile(`(?i)\b(merged?|pull request|PR ?#?\d+|MR ?!?\d+|install(ed)?|deploy(ed|ment)?|releas(e|ed)|publish(ed)?|tag(ged)? v?\d)`)

func buildDeliverySection(in ReportInput) DeliverySection {
	s := DeliverySection{}
	if in.RecordErr != nil {
		s.Error = in.RecordErr.Error()
	}
	byStage := map[string][]DeliveryObservation{}
	recorded := false
	for _, r := range liveRuns(in.Record) {
		for _, d := range r.Deliveries {
			result := d.Result
			if result == "" {
				result = "unknown"
			}
			byStage[d.Stage] = append(byStage[d.Stage], DeliveryObservation{At: d.At, Result: result, Revision: d.Revision, Ref: d.Ref,
				Target: d.Target, ArtifactPath: d.ArtifactPath, URL: d.URL, Evidence: d.Evidence, Source: d.Source,
				Provenance: d.Provenance, RunID: r.RunID, EventID: d.EventID})
			recorded = true
		}
		for _, q := range r.Requirements {
			if q.Number != 0 {
				continue
			}
			for _, e := range q.Evidence {
				if e.Imported && reDeliveryNote.MatchString(e.Summary) {
					s.Imported = append(s.Imported, DeliveryNote{Text: e.Summary, At: e.At, Provenance: e.Provenance})
				}
			}
		}
	}

	// proof holds each stage's latest observation with a result; a pending
	// declaration is never what a later stage is checked against.
	proof := map[string]*DeliveryObservation{}
	for _, info := range deliveryStageInfo {
		st := DeliveryStage{Stage: info.stage, Label: info.label, Proof: info.proof, State: "not_recorded", Observed: []DeliveryObservation{}}
		for _, o := range sortObservations(byStage[info.stage]) {
			if o.Result == "pending" {
				st.Intended = append(st.Intended, o)
			} else {
				st.Observed = append(st.Observed, o)
			}
		}
		if n := len(st.Observed); n > 0 {
			p := st.Observed[n-1]
			proof[info.stage] = &p
		}
		all := sortObservations(byStage[info.stage])
		if n := len(all); n > 0 {
			l := all[n-1]
			st.Latest = &l
			st.State = l.Result
			if l.Revision == "" && info.stage != "pr" && info.stage != "review" {
				st.Note = "revision not recorded, so this observation cannot be linked to the other stages"
			}
		}
		s.Stages = append(s.Stages, st)
	}

	// Staleness: an observation is stale when the thing it proves has
	// since changed.
	for i := range s.Stages {
		st := &s.Stages[i]
		if st.Latest == nil || st.Latest.Result == "pending" {
			continue
		}
		switch st.Stage {
		case "ci":
			st.Stale = staleAgainst(*st.Latest, proof["source_commit"], "commit", false)
		case "install", "deploy":
			up, name := proof["merge"], "merge"
			if up == nil {
				up, name = proof["source_commit"], "commit"
			}
			st.Stale = staleAgainst(*st.Latest, up, name, false)
		case "runtime_verification":
			up, name := newest(proof["install"], proof["deploy"])
			st.Stale = staleAgainst(*st.Latest, up, name, true)
		}
		if st.Stale != "" {
			st.State = "stale"
		}
	}

	for _, st := range s.Stages {
		reason := ""
		switch st.State {
		case "pending":
			reason = "recorded as pending; no result yet"
		case "failed":
			reason = "the latest observation failed"
		case "unknown":
			reason = "the latest observation has no result"
		case "stale":
			reason = st.Stale
		default:
			continue
		}
		s.Outstanding = append(s.Outstanding, DeliveryGate{Stage: st.Stage, Label: st.Label, State: st.State, Reason: reason})
	}

	rv := s.Stages[len(s.Stages)-1]
	switch {
	case !recorded:
		s.State = stateUnavailable
	case rv.State == "passed" && len(s.Outstanding) == 0:
		s.State = stateComplete
	default:
		s.State = statePartial
	}
	if s.Outstanding == nil {
		s.Outstanding = []DeliveryGate{}
	}
	s.Summary = deliverySummary(s)
	return s
}

func sortObservations(in []DeliveryObservation) []DeliveryObservation {
	out := append([]DeliveryObservation(nil), in...)
	for i := 1; i < len(out); i++ {
		for j := i; j > 0 && out[j].At.Before(out[j-1].At); j-- {
			out[j], out[j-1] = out[j-1], out[j]
		}
	}
	return out
}

func newest(a, b *DeliveryObservation) (*DeliveryObservation, string) {
	switch {
	case a == nil && b == nil:
		return nil, ""
	case b == nil || (a != nil && !a.At.Before(b.At)):
		return a, "installation"
	}
	return b, "deployment"
}

// staleAgainst reports why o no longer proves its stage: a later upstream
// observation at another revision, or — for runtime proof — a revision
// that differs from what was installed.
func staleAgainst(o DeliveryObservation, up *DeliveryObservation, upName string, sameRevision bool) string {
	if up == nil {
		return ""
	}
	later := up.At.After(o.At)
	switch {
	case o.Revision != "" && up.Revision != "" && o.Revision != up.Revision && (later || sameRevision):
		return fmt.Sprintf("proof is for revision %s; the latest %s is revision %s", shortRev(o.Revision), upName, shortRev(up.Revision))
	case later && (o.Revision == "" || up.Revision == ""):
		return fmt.Sprintf("a later %s was recorded after this proof and the revisions cannot be compared", upName)
	}
	return ""
}

func shortRev(r string) string {
	if len(r) > 12 {
		return r[:12]
	}
	return r
}

func deliverySummary(s DeliverySection) string {
	if s.State == stateUnavailable {
		msg := "No delivery stage has been recorded: implementation, tests, merge, installation and runtime verification are unknown, not failed."
		if len(s.Imported) > 0 {
			msg += " The plan file mentions delivery in prose, shown below as unverified notes."
		}
		return msg
	}
	var done, open []string
	for _, st := range s.Stages {
		switch st.State {
		case "passed":
			done = append(done, strings.ToLower(st.Label))
		case "not_recorded":
		default:
			open = append(open, strings.ToLower(st.Label)+" "+strings.ReplaceAll(st.State, "_", " "))
		}
	}
	msg := "Passed: " + joinOr(done, "none") + "."
	if len(open) > 0 {
		msg += " Open: " + strings.Join(open, "; ") + "."
	}
	if rv := s.Stages[len(s.Stages)-1]; rv.State == "not_recorded" {
		msg += " Runtime verification is not recorded, so the change is not proven running."
	}
	return msg
}

// safeURL returns an http(s) URL for a link, or "" for anything else.
func safeURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return ""
	}
	return u.String()
}

func writeDeliveryObservation(b *strings.Builder, o DeliveryObservation) {
	b.WriteString(`<dl class="rd-obs">`)
	item := func(term, value string) {
		if value != "" {
			b.WriteString(`<dt>` + term + `</dt><dd>` + value + `</dd>`)
		}
	}
	item("Result", stateBadge(o.Result))
	item("Recorded at", timeTag(o.At))
	if o.Revision != "" {
		item("Revision", `<code title="`+html.EscapeString(o.Revision)+`">`+html.EscapeString(shortRev(o.Revision))+`</code>`)
	} else {
		item("Revision", `<span class="ra-unknown">not recorded</span>`)
	}
	item("Reference", html.EscapeString(o.Ref))
	item("Target", html.EscapeString(o.Target))
	item("Artifact", html.EscapeString(o.ArtifactPath))
	if o.URL != "" {
		if u := safeURL(o.URL); u != "" {
			item("Link", `<a href="`+html.EscapeString(u)+`" rel="noopener noreferrer">`+html.EscapeString(u)+`</a>`)
		} else {
			item("Link", html.EscapeString(o.URL))
		}
	}
	item("Evidence", html.EscapeString(o.Evidence))
	item("Source", html.EscapeString(o.Source+" · "+o.Provenance))
	b.WriteString(`</dl>`)
}

func writeDeliverySection(b *strings.Builder, s DeliverySection) {
	b.WriteString(`<section class="report-section" id="report-delivery" aria-labelledby="report-delivery-h">`)
	css := accountingCSS("report-delivery")
	css = strings.TrimSuffix(css, "</style>") + `
#report-delivery ol.rd-stages{list-style:none;margin:8px 0;padding:0;display:grid;gap:8px}
#report-delivery ol.rd-stages>li{border:1px solid var(--border);border-radius:6px;padding:8px 10px;background:var(--surface-2)}
#report-delivery .rd-head{display:flex;flex-wrap:wrap;gap:8px;align-items:center;font-weight:600}
#report-delivery dl.rd-obs{display:grid;grid-template-columns:max-content 1fr;gap:2px 10px;margin:6px 0}
#report-delivery dl.rd-obs dt{margin:0;font-weight:600;color:var(--text-dim)}
#report-delivery dl.rd-obs dd{margin:0;color:var(--text)}
@media(max-width:600px){#report-delivery dl.rd-obs{grid-template-columns:1fr}#report-delivery dl.rd-obs dd{margin-bottom:4px}}
</style>`
	b.WriteString(css)
	b.WriteString(`<h2 id="report-delivery-h">Delivery ` + stateBadge(s.State) + `</h2>`)
	b.WriteString(`<p class="ra-sum">` + html.EscapeString(s.Summary) + `</p>`)
	b.WriteString(`<p class="ra-note">Each stage needs its own evidence: a passed test does not imply a merge, and a merge does not imply an installation or a running change. Reading this section publishes, deploys and changes nothing.</p>`)
	if s.Error != "" {
		b.WriteString(`<p class="ra-note">The run record could not be read: ` + html.EscapeString(s.Error) + `</p>`)
	}

	if len(s.Outstanding) > 0 {
		b.WriteString(`<h3>Outstanding gates</h3><ul class="ra-list">`)
		for _, g := range s.Outstanding {
			fmt.Fprintf(b, `<li><b>%s</b> %s — %s</li>`, html.EscapeString(g.Label), stateBadge(g.State), html.EscapeString(g.Reason))
		}
		b.WriteString(`</ul>`)
	}

	// With nothing recorded at any stage the summary already says every
	// stage is unknown; the list of what each stage needs folds into one
	// disclosure instead of repeating that eight times.
	recorded := false
	for _, st := range s.Stages {
		recorded = recorded || st.Latest != nil
	}
	if recorded {
		b.WriteString(`<h3>Stages</h3><ol class="rd-stages">`)
	} else {
		fmt.Fprintf(b, `<details class="rd-all"><summary>Stages and the proof each needs (%d)</summary><ol class="rd-stages">`, len(s.Stages))
	}
	for _, st := range s.Stages {
		fmt.Fprintf(b, `<li><div class="rd-head">%s %s</div>`, html.EscapeString(st.Label), stateBadge(st.State))
		b.WriteString(`<p class="ra-note">Proof required: ` + html.EscapeString(st.Proof) + `.</p>`)
		if st.Latest == nil {
			b.WriteString(`<p class="ra-note"><span class="ra-unknown">No evidence recorded.</span> Unknown, not failed.</p></li>`)
			continue
		}
		if st.Stale != "" {
			b.WriteString(`<p class="ra-note">Stale: ` + html.EscapeString(st.Stale) + `.</p>`)
		}
		if st.Note != "" {
			b.WriteString(`<p class="ra-note">` + html.EscapeString(st.Note) + `.</p>`)
		}
		writeDeliveryObservation(b, *st.Latest)
		if older := len(st.Observed) + len(st.Intended) - 1; older > 0 {
			fmt.Fprintf(b, `<details><summary>All observations for %s (%d)</summary>`, html.EscapeString(strings.ToLower(st.Label)), older+1)
			for _, o := range sortObservations(append(append([]DeliveryObservation{}, st.Observed...), st.Intended...)) {
				writeDeliveryObservation(b, o)
			}
			b.WriteString(`</details>`)
		}
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ol>`)
	if !recorded {
		b.WriteString(`</details>`)
	}

	if len(s.Imported) > 0 {
		b.WriteString(`<h3>Plan-file notes that mention delivery</h3><p class="ra-note">Imported prose, not recorded observations: these never set a stage's state.</p><ul class="ra-list">`)
		for _, n := range s.Imported {
			fmt.Fprintf(b, `<li>%s <span class="ra-note">(%s)</span></li>`, html.EscapeString(n.Text), html.EscapeString(n.Provenance))
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`</section>`)
}
