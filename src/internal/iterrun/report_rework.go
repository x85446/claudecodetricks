package iterrun

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// ReworkSection is the plan report's step-11 section: Failed attempts, retries, restarts, reopened requirements and recovery totals.
//
// Requirement figures cover plan steps (requirement > 0); plan-level work
// such as delivery is counted apart so it never changes a requirement
// denominator. Attempt outcomes are the recorded ones: an interrupted
// attempt stays interrupted however the plan ends.
type ReworkSection struct {
	Status     string `json:"status"`
	StatusNote string `json:"status_note"`
	// Requirements counts steps with at least one recorded attempt.
	Requirements int `json:"requirements"`
	Attempts     int `json:"attempts"`
	// Outcomes counts attempts by recorded outcome; "open" is an attempt
	// with no recorded finish.
	Outcomes            map[string]int `json:"outcomes"`
	Unsuccessful        int            `json:"unsuccessful"`
	Retries             int            `json:"retries"`
	Restarts            int            `json:"restarts"`
	Reopened            int            `json:"reopened"`
	RepeatedValidations int            `json:"repeated_validations"`
	FirstPass           ReworkRatio    `json:"first_pass"`
	Recovered           ReworkRatio    `json:"recovered"`
	// RecoveryCost is the work after each requirement's first unsuccessful
	// attempt; UnsuccessfulCost is the unsuccessful attempts themselves.
	RecoveryCost     ReworkCost          `json:"recovery_cost"`
	UnsuccessfulCost ReworkCost          `json:"unsuccessful_cost"`
	PlanLevel        ReworkPlanLevel     `json:"plan_level"`
	UnattributedUse  int                 `json:"unattributed_usage_requests"`
	Rows             []ReworkRequirement `json:"requirements_detail"`
	Terms            []LifecycleTerm     `json:"definitions"`
	Unknown          []string            `json:"unknown"`
}

// ReworkRatio is Count of Of requirements; Undetermined requirements had
// no outcome to judge and are outside the denominator.
type ReworkRatio struct {
	Count        int `json:"count"`
	Of           int `json:"of"`
	Undetermined int `json:"undetermined"`
}

// ReworkCost totals attempts' duration and tokens. A total stays nil until
// one attempt reported it; attempts that did not are counted as unknown.
type ReworkCost struct {
	Attempts        int    `json:"attempts"`
	DurationMS      *int64 `json:"duration_ms,omitempty"`
	DurationUnknown int    `json:"duration_unknown"`
	Tokens          *int64 `json:"tokens,omitempty"`
	TokensUnknown   int    `json:"tokens_unknown"`
}

// ReworkPlanLevel is plan-level work (requirement 0): delivery and other
// assignments that belong to no step.
type ReworkPlanLevel struct {
	Attempts int            `json:"attempts"`
	Outcomes map[string]int `json:"outcomes"`
}

// ReworkRequirement is one step's attempt history.
type ReworkRequirement struct {
	Number   int             `json:"number"`
	Title    string          `json:"title,omitempty"`
	Attempts []ReworkAttempt `json:"attempts"`
	// FirstPass is nil while the first attempt has no outcome.
	FirstPass *bool `json:"first_pass,omitempty"`
	Recovered bool  `json:"recovered"`
	Reopened  bool  `json:"reopened"`
}

// ReworkAttempt is one attempt as recorded.
type ReworkAttempt struct {
	Number        int        `json:"number"`
	Kind          string     `json:"kind,omitempty"`
	Outcome       string     `json:"outcome"`
	Reason        string     `json:"reason,omitempty"`
	StartedAt     *time.Time `json:"started_at,omitempty"`
	FinishedAt    *time.Time `json:"finished_at,omitempty"`
	DurationMS    *int64     `json:"duration_ms,omitempty"`
	Tokens        *int64     `json:"tokens,omitempty"`
	TokensPartial bool       `json:"tokens_partial,omitempty"`
	Validated     bool       `json:"validated"`
	// Recovery marks an attempt counted in the recovery cost.
	Recovery   bool   `json:"recovery"`
	Source     string `json:"source"`
	Provenance string `json:"provenance"`
}

var reworkTerms = []LifecycleTerm{
	{"Attempt", "One try at a requirement: a Conductor assignment, or an attempt a CLI run recorded. Repeated events for the same attempt never add one."},
	{"Unsuccessful attempt", "A finished attempt whose outcome is failed, retry, interrupted, blocked or partial. An interrupted attempt stays interrupted even when the plan later completes."},
	{"Retry", "Any attempt after a requirement's first."},
	{"Restart", "An attempt that began after an interrupted attempt of the same requirement."},
	{"Reopened requirement", "A requirement with an attempt that began after one of its attempts had passed. The earlier success stays recorded."},
	{"Repeated validation", "Per requirement, every attempt carrying validation evidence after the first such attempt."},
	{"First-pass success", "Requirements whose first attempt passed, out of requirements whose first attempt has a recorded outcome."},
	{"Recovered", "Requirements whose first unsuccessful attempt was followed by a passed attempt, out of requirements with an unsuccessful attempt."},
	{"Recovery cost", "Duration and tokens of every attempt after a requirement's first unsuccessful attempt, through the attempt that passed, or through the latest when none passed. An attempt without a recorded duration or usage is counted as unknown, never zero."},
	{"Tokens", "Input plus output tokens recorded for the attempt."},
	{"Plan-level attempts", "Attempts on no step (requirement 0), such as delivery. Counted apart from requirement figures."},
}

func reworkUnsuccessful(outcome string) bool {
	switch outcome {
	case "failed", "retry", "interrupted", "blocked", "partial":
		return true
	}
	return false
}

func reworkAttempt(a runrecord.AttemptView) ReworkAttempt {
	ra := ReworkAttempt{Number: a.Number, Kind: a.Kind, Outcome: a.Outcome, Reason: a.Reason, StartedAt: a.StartedAt, FinishedAt: a.FinishedAt, DurationMS: a.DurationMS, Validated: len(a.Evidence) > 0, Source: a.Source}
	if len(a.Evidence) > 0 {
		ra.Provenance = a.Evidence[0].Provenance
	}
	if a.Usage.Requests > 0 && (a.Usage.InputTokens != nil || a.Usage.OutputTokens != nil) {
		var t int64
		if a.Usage.InputTokens != nil {
			t += *a.Usage.InputTokens
		}
		if a.Usage.OutputTokens != nil {
			t += *a.Usage.OutputTokens
		}
		ra.Tokens = &t
		ra.TokensPartial = containsString(a.Usage.Partial, "input_tokens") || containsString(a.Usage.Partial, "output_tokens")
	}
	return ra
}

func (c *ReworkCost) add(a ReworkAttempt) {
	c.Attempts++
	if a.DurationMS != nil {
		if c.DurationMS == nil {
			c.DurationMS = new(int64)
		}
		*c.DurationMS += *a.DurationMS
	} else {
		c.DurationUnknown++
	}
	if a.Tokens != nil {
		if c.Tokens == nil {
			c.Tokens = new(int64)
		}
		*c.Tokens += *a.Tokens
	} else {
		c.TokensUnknown++
	}
}

func buildReworkSection(in ReportInput) ReworkSection {
	s := ReworkSection{Outcomes: map[string]int{}, PlanLevel: ReworkPlanLevel{Outcomes: map[string]int{}}, Rows: []ReworkRequirement{}, Terms: reworkTerms, Unknown: []string{}}
	if in.RecordErr != nil || len(in.Record.Runs) == 0 {
		s.Status = sectionUnavailable
		s.StatusNote = "No run record exists for this execution, so attempts and rework are unknown."
		if in.RecordErr != nil {
			s.StatusNote = "The run record could not be read (" + in.RecordErr.Error() + "), so attempts and rework are unknown."
		}
		s.Unknown = append(s.Unknown, "attempts")
		return s
	}

	// One requirement may appear in several runs of the execution; their
	// attempts join in time order.
	type req struct {
		title    string
		attempts []runrecord.AttemptView
	}
	reqs := map[int]*req{}
	var numbers []int
	for _, r := range in.Record.Runs {
		for _, q := range r.Requirements {
			x, ok := reqs[q.Number]
			if !ok {
				x = &req{}
				reqs[q.Number] = x
				numbers = append(numbers, q.Number)
			}
			if q.Task != "" {
				x.title = q.Task
			}
			x.attempts = append(x.attempts, q.Attempts...)
		}
		for _, u := range r.Usage.Requests {
			if u.Attempt == 0 && u.Scope == runrecord.ScopeExact {
				s.UnattributedUse++
			}
		}
	}
	sort.Ints(numbers)
	open := 0
	for _, n := range numbers {
		x := reqs[n]
		sort.SliceStable(x.attempts, func(i, j int) bool {
			a, b := lcFirst(x.attempts[i].StartedAt, x.attempts[i].FinishedAt), lcFirst(x.attempts[j].StartedAt, x.attempts[j].FinishedAt)
			if !a.Equal(b) {
				return a.Before(b)
			}
			return x.attempts[i].Number < x.attempts[j].Number
		})
		if n == 0 {
			for _, a := range x.attempts {
				s.PlanLevel.Attempts++
				s.PlanLevel.Outcomes[a.Outcome]++
			}
			continue
		}
		if len(x.attempts) == 0 {
			continue
		}
		row := ReworkRequirement{Number: n, Title: lcTruncate(x.title, 120), Attempts: []ReworkAttempt{}}
		s.Requirements++
		firstFail, passedAt, validated := -1, -1, 0
		for i, a := range x.attempts {
			ra := reworkAttempt(a)
			s.Attempts++
			s.Outcomes[a.Outcome]++
			if a.Outcome == "open" {
				open++
			}
			if i > 0 {
				s.Retries++
				if x.attempts[i-1].Outcome == "interrupted" {
					s.Restarts++
				}
			}
			if passedAt >= 0 && !row.Reopened {
				row.Reopened = true
				s.Reopened++
			}
			if ra.Validated {
				validated++
			}
			if reworkUnsuccessful(a.Outcome) {
				s.Unsuccessful++
				s.UnsuccessfulCost.add(ra)
				if firstFail < 0 {
					firstFail = i
				}
			}
			if firstFail >= 0 && i > firstFail && !row.Recovered {
				ra.Recovery = true
				s.RecoveryCost.add(ra)
				if a.Outcome == "passed" {
					row.Recovered = true
				}
			}
			if a.Outcome == "passed" && passedAt < 0 {
				passedAt = i
			}
			row.Attempts = append(row.Attempts, ra)
		}
		if validated > 1 {
			s.RepeatedValidations += validated - 1
		}
		switch first := x.attempts[0].Outcome; {
		case first == "open":
			s.FirstPass.Undetermined++
		default:
			ok := first == "passed"
			row.FirstPass = &ok
			s.FirstPass.Of++
			if ok {
				s.FirstPass.Count++
			}
		}
		if firstFail >= 0 {
			s.Recovered.Of++
			if row.Recovered {
				s.Recovered.Count++
			}
		}
		s.Rows = append(s.Rows, row)
	}

	if s.Attempts == 0 && s.PlanLevel.Attempts == 0 {
		s.Status = sectionUnavailable
		s.StatusNote = "No attempts are recorded for this execution. The Conductor records each assignment as an attempt; a CLI /iterate run records attempts only when it appends them (iterate-run record append --type attempt.started or attempt.finished)."
		s.Unknown = append(s.Unknown, "attempts")
		return s
	}
	var gaps []string
	if in.Record.Source == "legacy" {
		gaps = append(gaps, "this execution has no run record; attempts come from Conductor run directories only, so CLI work has none")
	}
	if open > 0 {
		gaps = append(gaps, lcCount(open, "attempt")+" without a recorded outcome")
		s.Unknown = append(s.Unknown, "open attempt outcomes")
	}
	if s.RecoveryCost.DurationUnknown+s.UnsuccessfulCost.DurationUnknown > 0 {
		s.Unknown = append(s.Unknown, "some attempt durations")
	}
	if s.RecoveryCost.TokensUnknown+s.UnsuccessfulCost.TokensUnknown > 0 {
		s.Unknown = append(s.Unknown, "some attempt usage")
	}
	if s.UnattributedUse > 0 {
		gaps = append(gaps, lcCount(s.UnattributedUse, "usage request")+" not attributed to an attempt and outside every cost here")
	}
	if len(gaps) > 0 {
		s.Status = sectionPartial
		s.StatusNote = lcSentence(strings.Join(gaps, "; "))
	} else {
		s.Status = sectionAvailable
		s.StatusNote = "Every attempt has a recorded outcome."
	}
	return s
}

func lcTruncate(s string, n int) string {
	s = strings.TrimSpace(s)
	if r := []rune(s); len(r) > n {
		return string(r[:n-1]) + "…"
	}
	return s
}

func reworkCostText(c ReworkCost) string {
	if c.Attempts == 0 {
		return "none"
	}
	parts := []string{lcCount(c.Attempts, "attempt")}
	if c.DurationMS != nil {
		parts = append(parts, lcDur(*c.DurationMS))
	}
	if c.DurationUnknown > 0 {
		parts = append(parts, fmt.Sprintf("duration unknown for %d", c.DurationUnknown))
	}
	if c.Tokens != nil {
		parts = append(parts, lcTokens(*c.Tokens))
	}
	if c.TokensUnknown > 0 {
		parts = append(parts, fmt.Sprintf("usage unknown for %d", c.TokensUnknown))
	}
	return strings.Join(parts, ", ")
}

func lcTokens(n int64) string {
	s := fmt.Sprint(n)
	var out []byte
	for i := range s {
		if i > 0 && (len(s)-i)%3 == 0 {
			out = append(out, ',')
		}
		out = append(out, s[i])
	}
	if n == 1 {
		return "1 token"
	}
	return string(out) + " tokens"
}

func reworkRatioText(r ReworkRatio, unit string) string {
	text := fmt.Sprintf("%d of %s", r.Count, lcCount(r.Of, unit))
	if r.Of > 0 {
		text += " (" + lcPercent(int64(r.Count), int64(r.Of)) + ")"
	}
	if r.Undetermined > 0 {
		text += fmt.Sprintf("; %d undetermined", r.Undetermined)
	}
	return text
}

const reworkCSS = `#report-rework .rw-stats{display:grid;grid-template-columns:repeat(auto-fit,minmax(220px,1fr));gap:1px;background:var(--border);border:1px solid var(--border);border-radius:8px;overflow:hidden;padding:0;margin:0 0 12px;list-style:none}
#report-rework .rw-stats li{background:var(--surface);padding:8px 12px;margin:0}
#report-rework .rw-k{display:block;font-size:11px;letter-spacing:.04em;text-transform:uppercase;color:var(--text-dim);font-weight:600}
#report-rework .rw-v{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-variant-numeric:tabular-nums}
#report-rework ol.rw-reqs{padding:0}
#report-rework .rw-req{list-style:none;padding:8px 10px;background:var(--surface);border:1px solid var(--border);border-radius:8px;margin:0 0 8px}
#report-rework .rw-req ol{margin:4px 0 0;padding-left:18px;font-size:12.5px}
#report-rework .rw-out{font-weight:600}
#report-rework .rw-passed{color:var(--good)}
#report-rework .rw-unsuccessful{color:var(--warn)}
#report-rework .rw-tag{display:inline-block;font-size:11px;border:1px solid var(--border);border-radius:4px;padding:0 5px;margin-left:6px;color:var(--text-dim)}
`

func writeReworkSection(b *strings.Builder, s ReworkSection) {
	b.WriteString(`<section class="report-section" id="report-rework" aria-labelledby="report-rework-h"><style>`)
	b.WriteString(lcSectionCSS("report-rework", "rw") + reworkCSS)
	b.WriteString(`</style><h2 id="report-rework-h">Attempts and rework</h2>`)
	status := s.Status
	if status == "" {
		status = sectionUnavailable
	}
	writeLcStatus(b, "rw", status, s.StatusNote)
	if status == sectionUnavailable {
		writeLcTerms(b, "rw", s.Terms)
		b.WriteString(`</section>`)
		return
	}
	stat := func(k, v string) {
		fmt.Fprintf(b, `<li><span class="rw-k">%s</span><span class="rw-v">%s</span></li>`, lcEsc(k), lcEsc(v))
	}
	b.WriteString(`<ul class="rw-stats">`)
	stat("Attempts", fmt.Sprintf("%s on %s", lcCount(s.Attempts, "attempt"), lcCount(s.Requirements, "requirement")))
	var outcomes []string
	for _, o := range []string{"passed", "failed", "retry", "interrupted", "blocked", "partial", "open"} {
		if n := s.Outcomes[o]; n > 0 {
			outcomes = append(outcomes, fmt.Sprintf("%d %s", n, o))
		}
	}
	stat("Outcomes", lcOr(strings.Join(outcomes, ", "), "none"))
	stat("Unsuccessful attempts", lcCount(s.Unsuccessful, "attempt"))
	stat("Retries and restarts", lcCount(s.Retries, "retry")+", "+lcCount(s.Restarts, "restart"))
	stat("Reopened", lcCount(s.Reopened, "requirement"))
	stat("Repeated validations", lcCount(s.RepeatedValidations, "validation"))
	stat("First-pass success", reworkRatioText(s.FirstPass, "requirement"))
	stat("Recovered", reworkRatioText(s.Recovered, "requirement"))
	stat("Recovery cost", reworkCostText(s.RecoveryCost))
	stat("Unsuccessful-attempt cost", reworkCostText(s.UnsuccessfulCost))
	if s.PlanLevel.Attempts > 0 {
		var po []string
		for _, o := range []string{"passed", "failed", "retry", "interrupted", "blocked", "partial", "open"} {
			if n := s.PlanLevel.Outcomes[o]; n > 0 {
				po = append(po, fmt.Sprintf("%d %s", n, o))
			}
		}
		stat("Plan-level attempts", lcCount(s.PlanLevel.Attempts, "attempt")+": "+strings.Join(po, ", "))
	}
	b.WriteString(`</ul>`)

	b.WriteString(`<h3>By requirement</h3>`)
	if len(s.Rows) == 0 {
		b.WriteString(`<p class="rw-dim">No step has a recorded attempt.</p>`)
	} else {
		b.WriteString(`<ol class="rw-reqs">`)
		for _, r := range s.Rows {
			fmt.Fprintf(b, `<li class="rw-req" data-requirement="%d"><strong>Requirement %d</strong>`, r.Number, r.Number)
			if r.Title != "" {
				fmt.Fprintf(b, ` <span class="rw-dim">%s</span>`, lcEsc(r.Title))
			}
			if r.FirstPass != nil && *r.FirstPass {
				b.WriteString(`<span class="rw-tag">first pass</span>`)
			}
			if r.Recovered {
				b.WriteString(`<span class="rw-tag">recovered</span>`)
			}
			if r.Reopened {
				b.WriteString(`<span class="rw-tag">reopened</span>`)
			}
			b.WriteString(`<ol>`)
			for _, a := range r.Attempts {
				cls := "rw-out"
				switch {
				case a.Outcome == "passed":
					cls += " rw-passed"
				case reworkUnsuccessful(a.Outcome):
					cls += " rw-unsuccessful"
				}
				parts := []string{}
				if a.Kind != "" && a.Kind != "pair" {
					parts = append(parts, a.Kind)
				}
				if a.DurationMS != nil {
					parts = append(parts, lcDur(*a.DurationMS))
				} else {
					parts = append(parts, "duration unknown")
				}
				if a.Tokens != nil {
					t := lcTokens(*a.Tokens)
					if a.TokensPartial {
						t += " (partial)"
					}
					parts = append(parts, t)
				} else {
					parts = append(parts, "usage unknown")
				}
				if a.Recovery {
					parts = append(parts, "recovery work")
				}
				fmt.Fprintf(b, `<li value="%d" data-outcome="%s">Attempt %d: <span class="%s">%s</span> <span class="rw-dim">%s</span>`, a.Number, lcEsc(a.Outcome), a.Number, cls, lcEsc(a.Outcome), lcEsc(strings.Join(parts, "; ")))
				if a.Reason != "" {
					fmt.Fprintf(b, `<span class="rw-note">%s</span>`, lcEsc(lcTruncate(a.Reason, 200)))
				}
				b.WriteString(`</li>`)
			}
			b.WriteString(`</ol></li>`)
		}
		b.WriteString(`</ol>`)
	}
	writeLcTerms(b, "rw", s.Terms)
	b.WriteString(`</section>`)
}
