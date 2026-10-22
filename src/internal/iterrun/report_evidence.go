package iterrun

import (
	"fmt"
	"html"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// EvidenceSection is the plan report's step-8 section: Observed validation evidence per requirement and attempt, with freshness against covered code.
type EvidenceSection struct {
	Availability string                `json:"availability"`
	Notes        []string              `json:"notes,omitempty"`
	Source       string                `json:"source"`
	Counts       EvidenceCounts        `json:"counts"`
	Requirements []RequirementEvidence `json:"requirements"`
	// Plan holds evidence bound to no requirement: plan-level record
	// entries, deliveries and plan-log lines that name no step.
	Plan []EvidenceItem `json:"plan"`
}

// EvidenceCounts sorts requirements by the strongest evidence each has,
// and counts evidence items separately. Requirement figures are in
// requirements; item figures are in evidence items.
type EvidenceCounts struct {
	Requirements        int `json:"requirements"`
	ObservedPassed      int `json:"observed_passed"`
	ObservedStale       int `json:"observed_stale"`
	ObservedFailed      int `json:"observed_failed"`
	ObservedUnknown     int `json:"observed_unknown"`
	ImportedOnly        int `json:"imported_only"`
	CheckedWithoutProof int `json:"checked_without_proof"`
	NoEvidence          int `json:"no_evidence"`
	Items               int `json:"items"`
	ObservedItems       int `json:"observed_items"`
	ImportedItems       int `json:"imported_items"`
	StaleItems          int `json:"stale_items"`
}

// RequirementEvidence is one requirement's acceptance text and the
// evidence observed or imported for it, attempt by attempt.
type RequirementEvidence struct {
	Number int    `json:"number"`
	Anchor string `json:"anchor"`
	Task   string `json:"task,omitempty"`
	// Acceptance is what the validation must show: the plan's intended
	// check, never evidence that it passed.
	Acceptance        string `json:"acceptance,omitempty"`
	Lane              string `json:"lane,omitempty"`
	TaskChecked       *bool  `json:"task_checked,omitempty"`
	ValidationChecked *bool  `json:"validation_checked,omitempty"`
	// Proof is observed-passed, observed-stale, observed-failed,
	// observed-unknown, imported-only, checked-without-proof or none.
	Proof      string            `json:"proof"`
	ProofLabel string            `json:"proof_label"`
	Attempts   []AttemptEvidence `json:"attempts"`
	// Items holds evidence tied to the requirement but to no attempt.
	Items []EvidenceItem `json:"items"`
}

// AttemptEvidence is one attempt and the evidence it produced. Earlier
// attempts stay listed after a later one.
type AttemptEvidence struct {
	Number     int            `json:"number"`
	Key        string         `json:"key"`
	Outcome    string         `json:"outcome"`
	Kind       string         `json:"kind,omitempty"`
	Team       string         `json:"team,omitempty"`
	Engine     string         `json:"engine,omitempty"`
	Model      string         `json:"model,omitempty"`
	Reason     string         `json:"reason,omitempty"`
	StartedAt  *time.Time     `json:"started_at,omitempty"`
	FinishedAt *time.Time     `json:"finished_at,omitempty"`
	Latest     bool           `json:"latest"`
	Items      []EvidenceItem `json:"items"`
}

// EvidenceItem is one piece of evidence: a recorded observation or a line
// of imported prose. Result is what was observed; Reported is what prose
// claims, and is never promoted to a result.
type EvidenceItem struct {
	ID           string         `json:"id"`
	Requirement  int            `json:"requirement,omitempty"`
	Attempt      int            `json:"attempt,omitempty"`
	Result       string         `json:"result"` // passed | failed | unknown
	Reported     string         `json:"reported,omitempty"`
	Imported     bool           `json:"imported"`
	Origin       string         `json:"origin"`
	Provenance   string         `json:"provenance,omitempty"`
	Summary      string         `json:"summary,omitempty"`
	Command      string         `json:"command,omitempty"`
	ExitStatus   *int           `json:"exit_status,omitempty"`
	At           *time.Time     `json:"at,omitempty"`
	DateOnly     bool           `json:"date_only,omitempty"`
	ArtifactPath string         `json:"artifact_path,omitempty"`
	ArtifactURL  string         `json:"artifact_url,omitempty"`
	Revision     string         `json:"revision,omitempty"`
	Environment  string         `json:"environment,omitempty"`
	CoveredFiles []string       `json:"covered_files,omitempty"`
	Facts        []EvidenceFact `json:"facts,omitempty"`
	// Freshness is current, stale or unknown. Stale proof stays listed;
	// ChangedFiles names the covered files that changed since Revision.
	Freshness     string   `json:"freshness"`
	FreshnessNote string   `json:"freshness_note,omitempty"`
	ChangedFiles  []string `json:"changed_files,omitempty"`
	// Superseded marks a report a later one for the same requirement
	// replaced. It stays for history.
	Superseded bool `json:"superseded,omitempty"`
}

// evidenceFreshness resolves freshness against the project working tree,
// asking the change source once per revision.
type evidenceFreshness struct {
	project string
	cache   map[string]freshnessResult
}

type freshnessResult struct {
	changed []string
	err     error
}

func (f *evidenceFreshness) apply(it *EvidenceItem) {
	switch {
	case it.Revision == "" && it.Imported:
		it.Freshness, it.FreshnessNote = "unknown", "imported prose records no source revision or covered files"
		return
	case it.Revision == "":
		it.Freshness, it.FreshnessNote = "unknown", "no source revision was recorded"
		return
	case len(it.CoveredFiles) == 0:
		it.Freshness, it.FreshnessNote = "unknown", "recorded at "+shortRevision(it.Revision)+"; the code it covers was not recorded"
		return
	case f.project == "":
		it.Freshness, it.FreshnessNote = "unknown", "the project directory is unknown"
		return
	}
	res, ok := f.cache[it.Revision]
	if !ok {
		res.changed, res.err = evidenceChangedFiles(f.project, it.Revision)
		f.cache[it.Revision] = res
	}
	if res.err != nil {
		it.Freshness, it.FreshnessNote = "unknown", res.err.Error()
		return
	}
	changed := map[string]bool{}
	for _, c := range res.changed {
		changed[filepath.ToSlash(filepath.Clean(c))] = true
	}
	for _, c := range it.CoveredFiles {
		p := c
		if filepath.IsAbs(p) {
			if rel, err := filepath.Rel(f.project, p); err == nil {
				p = rel
			}
		}
		if p = filepath.ToSlash(filepath.Clean(p)); changed[p] {
			it.ChangedFiles = append(it.ChangedFiles, c)
		}
	}
	if len(it.ChangedFiles) > 0 {
		it.Freshness = "stale"
		it.FreshnessNote = fmt.Sprintf("%d of %d covered %s changed since %s", len(it.ChangedFiles), len(it.CoveredFiles), unitWord(len(it.CoveredFiles), "file", "files"), shortRevision(it.Revision))
		return
	}
	it.Freshness = "current"
	it.FreshnessNote = fmt.Sprintf("no covered file changed since %s", shortRevision(it.Revision))
}

func recordEvidenceItem(e runrecord.EvidenceEntry, requirement int) EvidenceItem {
	it := EvidenceItem{Requirement: requirement, Attempt: e.Attempt, Result: e.Result, Imported: e.Imported,
		Provenance: e.Provenance, Summary: e.Summary, Command: e.Command, ExitStatus: e.ExitStatus,
		ArtifactPath: e.ArtifactPath, ArtifactURL: e.ArtifactURL, Revision: e.Revision,
		Environment: e.Environment, CoveredFiles: e.CoveredFiles}
	if !e.At.IsZero() {
		it.At = integrityTime(e.At)
	}
	if it.Result == "" {
		it.Result = "unknown"
	}
	switch {
	case e.Provenance == "legacy:"+runrecord.LegacyPlanFile:
		it.Origin = "plan evidence line"
	case e.Provenance == "legacy:"+runrecord.LegacyConductorRuns:
		it.Origin = "Conductor result (reconstructed)"
	case e.Source == runrecord.SourceConductor:
		it.Origin = "Conductor result"
	case e.Source == runrecord.SourceCLI:
		it.Origin = "run record (iterate-run record)"
	default:
		it.Origin = "run record (" + e.Source + ")"
	}
	if it.Imported {
		it.Facts = evidenceFacts(it.Summary)
	}
	return it
}

func buildEvidenceSection(in ReportInput) EvidenceSection {
	src := reportPlanSourceFor(in.Plan)
	s := EvidenceSection{Source: "none", Requirements: []RequirementEvidence{}, Plan: []EvidenceItem{}}
	switch {
	case in.RecordErr != nil:
		s.Source = "plan-file"
		s.Notes = append(s.Notes, "The run record could not be read ("+in.RecordErr.Error()+"); only reports imported from the plan file and team logs are shown.")
	case in.Record.Source != "":
		s.Source = in.Record.Source
	}
	if s.Source == "legacy" {
		s.Notes = append(s.Notes, "No observed run record exists for this execution. Evidence here is prose imported from the plan file and team logs; its results were not observed.")
	}
	project := in.Plan.ProjectDir
	if project == "" {
		project = in.Record.ProjectPath
	}
	fresh := &evidenceFreshness{project: project, cache: map[string]freshnessResult{}}

	reqs := reportRequirements(in, src)
	known := map[int]bool{}
	for _, r := range reqs {
		known[r.Number] = true
	}
	recorded := map[string]bool{} // record summaries, so prose lines are not shown twice

	// Log segments, bound to the requirements they name. Text naming no
	// requirement is plan-level.
	bySteps := map[int][]EvidenceItem{}
	var planLog []EvidenceItem
	segments := 0
	for _, e := range src.Log {
		origin := "plan log"
		if e.Context != "" {
			origin += " · " + e.Context
		}
		for _, seg := range splitLogEntry(e.Text, known) {
			facts := evidenceFacts(seg.Text)
			signal := hasEvidenceSignal(seg.Text) || (len(seg.Steps) > 0 && reLooseSignal.MatchString(seg.Text))
			if !signal && len(facts) == 0 {
				continue
			}
			segments++
			it := EvidenceItem{Result: "unknown", Imported: true, Origin: origin, Provenance: fmt.Sprintf("plan file line %d", e.Line),
				Summary: seg.Text, Facts: facts, At: e.At, DateOnly: e.DateOnly}
			if len(seg.Steps) == 0 {
				planLog = append(planLog, it)
				continue
			}
			it.Reported = reportedClaim(seg.Text)
			for _, n := range seg.Steps {
				x := it
				x.Requirement = n
				bySteps[n] = append(bySteps[n], x)
			}
		}
	}

	for _, r := range reqs {
		re := RequirementEvidence{Number: r.Number, Anchor: fmt.Sprintf("evidence-req-%d", r.Number), Task: r.Task,
			Acceptance: r.Validation, Lane: r.Lane, TaskChecked: r.TaskChecked, ValidationChecked: r.ValidationChecked,
			Attempts: []AttemptEvidence{}, Items: []EvidenceItem{}}
		for _, e := range r.Evidence {
			recorded[strings.TrimSpace(e.Summary)] = true
			re.Items = append(re.Items, recordEvidenceItem(e, r.Number))
		}
		latest, hasLatest := latestAttempt(r)
		attempts := append([]runrecord.AttemptView(nil), r.Attempts...)
		sort.SliceStable(attempts, func(i, j int) bool { return attempts[i].Number < attempts[j].Number })
		for _, a := range attempts {
			ae := AttemptEvidence{Number: a.Number, Key: a.Key, Outcome: a.Outcome, Kind: a.Kind, Team: a.Team, Engine: a.Engine,
				Model: a.Model, Reason: a.Reason, StartedAt: a.StartedAt, FinishedAt: a.FinishedAt,
				Latest: hasLatest && a.Key == latest.Key, Items: []EvidenceItem{}}
			for _, e := range a.Evidence {
				recorded[strings.TrimSpace(e.Summary)] = true
				it := recordEvidenceItem(e, r.Number)
				it.Attempt = a.Number
				ae.Items = append(ae.Items, it)
			}
			re.Attempts = append(re.Attempts, ae)
		}
		// Team validation reports, every one; a later report supersedes
		// an earlier one without erasing it.
		var reports []teamValidationReport
		for _, rep := range src.Reports {
			if rep.Step == r.Number {
				reports = append(reports, rep)
			}
		}
		for i, rep := range reports {
			re.Items = append(re.Items, EvidenceItem{Requirement: r.Number, Result: "unknown", Reported: rep.Status, Imported: true,
				Origin: "team " + rep.Team + " validation report", Provenance: "team log " + rep.Team + ".log.md",
				Summary: rep.Note, Facts: evidenceFacts(rep.Note), Superseded: i < len(reports)-1})
		}
		re.Items = append(re.Items, bySteps[r.Number]...)
		s.Requirements = append(s.Requirements, re)
	}

	// Plan-level: record entries for requirement 0, deliveries, then log
	// lines that name no requirement.
	if in.RecordErr == nil {
		for _, run := range in.Record.Runs {
			for _, q := range run.Requirements {
				if q.Number > 0 {
					continue
				}
				for _, e := range q.Evidence {
					recorded[strings.TrimSpace(e.Summary)] = true
					s.Plan = append(s.Plan, recordEvidenceItem(e, 0))
				}
				for _, a := range q.Attempts {
					for _, e := range a.Evidence {
						recorded[strings.TrimSpace(e.Summary)] = true
						it := recordEvidenceItem(e, 0)
						it.Attempt = a.Number
						s.Plan = append(s.Plan, it)
					}
				}
			}
			for _, d := range run.Deliveries {
				result := d.Result
				if result == "" {
					result = "unknown"
				}
				summary := strings.TrimSpace(strings.Join(nonEmpty(d.Ref, d.Target, d.Evidence), " · "))
				s.Plan = append(s.Plan, EvidenceItem{Result: result, Imported: strings.HasPrefix(d.Provenance, "legacy:"),
					Origin: "delivery: " + d.Stage, Provenance: d.Provenance, Summary: summary, At: integrityTime(d.At),
					ArtifactPath: d.ArtifactPath, ArtifactURL: d.URL, Revision: d.Revision})
			}
		}
	}
	for _, it := range planLog {
		text := strings.TrimSpace(reRecordEvidencePrefix.ReplaceAllString(it.Summary, ""))
		if recorded[text] {
			continue
		}
		s.Plan = append(s.Plan, it)
	}

	// Freshness, ids and counts.
	seq := 0
	finish := func(it *EvidenceItem) {
		seq++
		it.ID = fmt.Sprintf("evidence-item-%d", seq)
		fresh.apply(it)
		s.Counts.Items++
		if it.Imported {
			s.Counts.ImportedItems++
		} else {
			s.Counts.ObservedItems++
		}
		if it.Freshness == "stale" {
			s.Counts.StaleItems++
		}
	}
	for i := range s.Requirements {
		re := &s.Requirements[i]
		for j := range re.Attempts {
			for k := range re.Attempts[j].Items {
				finish(&re.Attempts[j].Items[k])
			}
		}
		for j := range re.Items {
			finish(&re.Items[j])
		}
		re.Proof, re.ProofLabel = requirementProof(*re)
		s.Counts.Requirements++
		switch re.Proof {
		case "observed-passed":
			s.Counts.ObservedPassed++
		case "observed-stale":
			s.Counts.ObservedStale++
		case "observed-failed":
			s.Counts.ObservedFailed++
		case "observed-unknown":
			s.Counts.ObservedUnknown++
		case "imported-only":
			s.Counts.ImportedOnly++
		case "checked-without-proof":
			s.Counts.CheckedWithoutProof++
		default:
			s.Counts.NoEvidence++
		}
	}
	for i := range s.Plan {
		finish(&s.Plan[i])
	}

	switch {
	case in.RecordErr != nil && !src.Read && len(reqs) == 0:
		s.Availability = integrityUnavailable
		s.Notes = append(s.Notes, "Neither the run record nor the plan file could be read.")
	case in.RecordErr != nil || !src.Read || s.Source == "legacy" || s.Source == "none":
		s.Availability = integrityPartial
		if !src.Read {
			s.Notes = append(s.Notes, "The plan file and team logs could not be read; only recorded evidence is shown.")
		}
	default:
		s.Availability = integrityAvailable
	}
	return s
}

var reRecordEvidencePrefix = regexp.MustCompile(`^(?:Validation|Delivery) evidence:\s*`)

func nonEmpty(parts ...string) []string {
	var out []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			out = append(out, p)
		}
	}
	return out
}

// requirementProof names the strongest evidence a requirement has: the
// latest observed result first, then imported reports, then a checked box
// with nothing behind it.
func requirementProof(re RequirementEvidence) (string, string) {
	var latest *EvidenceItem
	imported := false
	visit := func(it *EvidenceItem) {
		if it.Imported {
			imported = true
			return
		}
		if latest == nil || (it.At != nil && (latest.At == nil || !it.At.Before(*latest.At))) {
			latest = it
		}
	}
	for i := range re.Attempts {
		for j := range re.Attempts[i].Items {
			visit(&re.Attempts[i].Items[j])
		}
	}
	for i := range re.Items {
		visit(&re.Items[i])
	}
	checked := re.ValidationChecked != nil && *re.ValidationChecked
	switch {
	case latest != nil && latest.Result == "passed" && latest.Freshness == "stale":
		return "observed-stale", "Passed at " + shortRevision(latest.Revision) + "; covered code changed since"
	case latest != nil && latest.Result == "passed":
		return "observed-passed", "Observed passing proof"
	case latest != nil && latest.Result == "failed":
		return "observed-failed", "Latest observed proof failed"
	case latest != nil:
		return "observed-unknown", "Observed evidence, result unknown"
	case imported && checked:
		return "imported-only", "Checked; imported reports only, results not observed"
	case imported:
		return "imported-only", "Imported reports only, results not observed"
	case checked:
		return "checked-without-proof", "Checked, no proof recorded"
	}
	return "none", "No evidence recorded"
}

func writeEvidenceSection(b *strings.Builder, s EvidenceSection) {
	esc := html.EscapeString
	b.WriteString(evidenceCSS)
	b.WriteString(`<section class="report-section" id="report-evidence" aria-labelledby="report-evidence-title">`)
	b.WriteString(`<h2 id="report-evidence-title">Validation evidence</h2>`)
	writeIntegrityState(b, s.Availability, s.Notes, map[string]string{
		integrityAvailable:   "Available",
		integrityPartial:     "Partial",
		integrityUnavailable: "Unavailable",
	})
	if s.Availability == integrityUnavailable {
		b.WriteString(`</section>`)
		return
	}
	c := s.Counts
	if c.Requirements == 0 {
		b.WriteString(`<p class="ev-dim">No numbered requirements were found, so there is no requirement evidence to show.</p>`)
	} else {
		fmt.Fprintf(b, `<p class="ev-summary">Of %d %s: <strong>%d</strong> with observed passing proof · <strong>%d</strong> stale · <strong>%d</strong> failed · <strong>%d</strong> with imported reports only · <strong>%d</strong> checked without proof · <strong>%d</strong> with no evidence.</p>`,
			c.Requirements, unitWord(c.Requirements, "requirement", "requirements"), c.ObservedPassed, c.ObservedStale, c.ObservedFailed, c.ImportedOnly, c.CheckedWithoutProof, c.NoEvidence)
		fmt.Fprintf(b, `<p class="ev-dim">%d evidence %s: %d observed, %d imported, %d stale.</p>`, c.Items, unitWord(c.Items, "item", "items"), c.ObservedItems, c.ImportedItems, c.StaleItems)
		b.WriteString(`<ol class="ev-reqs">`)
		for _, re := range s.Requirements {
			writeRequirementEvidence(b, re)
		}
		b.WriteString(`</ol>`)
	}
	fmt.Fprintf(b, `<details class="ev-plan" id="evidence-plan"><summary>Plan-level evidence (%d %s)</summary>`, len(s.Plan), unitWord(len(s.Plan), "item", "items"))
	if len(s.Plan) == 0 {
		b.WriteString(`<p class="ev-dim">No plan-level evidence: no delivery was recorded and no plan-log line reports a result without naming a requirement.</p>`)
	} else {
		b.WriteString(`<ul class="ev-items">`)
		for _, it := range s.Plan {
			writeEvidenceItem(b, it)
		}
		b.WriteString(`</ul>`)
	}
	b.WriteString(`</details>`)
	b.WriteString(`<details class="ev-defs"><summary>What counts as evidence</summary><dl>`)
	for _, d := range [][2]string{
		{"Acceptance", "The plan's validation text: what must be shown. It is the intended check, not proof that it passed."},
		{"Observed", "Recorded at run time, with a result (passed, failed or unknown) and, when captured, the command, exit status, artifact, source revision and environment."},
		{"Imported", "Prose copied from the plan file, the plan log or a team log. What it reports (met, partial, not met) is shown as a claim; its result was not observed."},
		{"Checked, no proof recorded", "The validation box is ticked, but no observation or report backs it. A checked box is not evidence."},
		{"Fresh, stale, unknown", "Observed proof records the revision it ran at and the files it covers. When a covered file changes after that revision, the proof is stale; it stays listed. Without a revision or covered files, freshness is unknown."},
		{"Attempts", "Each recorded try keeps its own evidence. A later attempt never erases an earlier one."},
	} {
		fmt.Fprintf(b, `<dt>%s</dt><dd>%s</dd>`, esc(d[0]), esc(d[1]))
	}
	b.WriteString(`</dl></details>`)
	b.WriteString(evidenceAnchorScript)
	b.WriteString(`</section>`)
}

func writeRequirementEvidence(b *strings.Builder, re RequirementEvidence) {
	esc := html.EscapeString
	title := re.Task
	if len(title) > 140 {
		title = title[:140] + "…"
	}
	fmt.Fprintf(b, `<li class="ev-req" id="%s"><details><summary><span class="ev-num">Requirement %d</span> <span class="ev-proof ev-p-%s">%s</span> <span class="ev-title">%s</span></summary><div class="ev-body">`,
		esc(re.Anchor), re.Number, esc(re.Proof), esc(re.ProofLabel), esc(title))
	box := func(v *bool) string {
		switch {
		case v == nil:
			return "none"
		case *v:
			return "checked"
		}
		return "unchecked"
	}
	fmt.Fprintf(b, `<p class="ev-boxes">Lane %s · task box %s · validation box %s</p>`, esc(re.Lane), box(re.TaskChecked), box(re.ValidationChecked))
	if re.Acceptance != "" {
		fmt.Fprintf(b, `<details class="ev-accept"><summary>Acceptance: what the validation must show</summary><p>%s</p></details>`, esc(re.Acceptance))
	}
	if len(re.Attempts) > 0 {
		fmt.Fprintf(b, `<h3 class="ev-h">Attempts <span class="ev-dim">%d %s</span></h3>`, len(re.Attempts), unitWord(len(re.Attempts), "attempt", "attempts"))
		for _, a := range re.Attempts {
			label := fmt.Sprintf("Attempt %d · %s", a.Number, attemptOutcomeLabel(a.Outcome))
			if a.Latest && len(re.Attempts) > 1 {
				label += " · latest"
			} else if len(re.Attempts) > 1 {
				label += " · earlier attempt"
			}
			var meta []string
			if a.Team != "" {
				meta = append(meta, "team "+a.Team)
			}
			if a.Engine != "" || a.Model != "" {
				meta = append(meta, strings.TrimSpace(a.Engine+" "+a.Model))
			}
			if a.StartedAt != nil {
				meta = append(meta, "started "+a.StartedAt.UTC().Format("2006-01-02 15:04 UTC"))
			}
			if a.FinishedAt != nil {
				meta = append(meta, "finished "+a.FinishedAt.UTC().Format("2006-01-02 15:04 UTC"))
			}
			if a.Reason != "" {
				meta = append(meta, a.Reason)
			}
			fmt.Fprintf(b, `<div class="ev-attempt"><p class="ev-attempt-head"><strong>%s</strong> <span class="ev-dim">%s</span></p>`, esc(label), esc(strings.Join(meta, " · ")))
			if len(a.Items) == 0 {
				b.WriteString(`<p class="ev-dim">No evidence was recorded for this attempt.</p>`)
			} else {
				b.WriteString(`<ul class="ev-items">`)
				for _, it := range a.Items {
					writeEvidenceItem(b, it)
				}
				b.WriteString(`</ul>`)
			}
			b.WriteString(`</div>`)
		}
	}
	var observed, imported []EvidenceItem
	for _, it := range re.Items {
		if it.Imported {
			imported = append(imported, it)
		} else {
			observed = append(observed, it)
		}
	}
	if len(observed) > 0 {
		fmt.Fprintf(b, `<h3 class="ev-h">Observed, not tied to an attempt <span class="ev-dim">%d %s</span></h3><ul class="ev-items">`, len(observed), unitWord(len(observed), "item", "items"))
		for _, it := range observed {
			writeEvidenceItem(b, it)
		}
		b.WriteString(`</ul>`)
	}
	if len(imported) > 0 {
		fmt.Fprintf(b, `<h3 class="ev-h">Imported reports <span class="ev-dim">%d %s; results not observed</span></h3><ul class="ev-items">`, len(imported), unitWord(len(imported), "item", "items"))
		for _, it := range imported {
			writeEvidenceItem(b, it)
		}
		b.WriteString(`</ul>`)
	}
	if len(re.Attempts) == 0 && len(re.Items) == 0 {
		if re.ValidationChecked != nil && *re.ValidationChecked {
			b.WriteString(`<p class="ev-empty">The validation box is checked, but no command result, artifact or report backs it.</p>`)
		} else {
			b.WriteString(`<p class="ev-empty">No evidence was recorded for this requirement.</p>`)
		}
	}
	b.WriteString(`</div></details></li>`)
}

func writeEvidenceItem(b *strings.Builder, it EvidenceItem) {
	esc := html.EscapeString
	fmt.Fprintf(b, `<li class="ev-item" id="%s"><p class="ev-head">`, esc(it.ID))
	switch it.Result {
	case "passed":
		b.WriteString(`<span class="ev-result ev-r-passed">Passed</span>`)
	case "failed":
		b.WriteString(`<span class="ev-result ev-r-failed">Failed</span>`)
	default:
		b.WriteString(`<span class="ev-result ev-r-unknown">Result not observed</span>`)
	}
	kind := "Observed"
	if it.Imported {
		kind = "Imported"
	}
	fmt.Fprintf(b, ` <span class="ev-kind">%s</span> <span class="ev-origin">%s</span>`, kind, esc(it.Origin))
	switch {
	case it.At != nil && it.DateOnly:
		fmt.Fprintf(b, ` <time class="ev-dim" datetime="%s">%s, time of day not recorded</time>`, it.At.UTC().Format("2006-01-02"), it.At.UTC().Format("2006-01-02"))
	case it.At != nil:
		fmt.Fprintf(b, ` <time class="ev-dim" datetime="%s">%s</time>`, it.At.UTC().Format(time.RFC3339), it.At.UTC().Format("2006-01-02 15:04 UTC"))
	default:
		b.WriteString(` <span class="ev-dim">time not recorded</span>`)
	}
	if it.Attempt > 0 {
		fmt.Fprintf(b, ` <span class="ev-dim">· attempt %d</span>`, it.Attempt)
	}
	if it.Superseded {
		b.WriteString(` <span class="ev-superseded">superseded by a later report</span>`)
	}
	b.WriteString(`</p>`)
	if it.Reported != "" {
		fmt.Fprintf(b, `<p class="ev-claim">Reports: %s</p>`, esc(reportedLabel(it.Reported)))
	}
	if it.Summary != "" {
		fmt.Fprintf(b, `<p class="ev-text">%s</p>`, esc(it.Summary))
	}
	var rows [][2]string
	add := func(k, v string) {
		if v != "" {
			rows = append(rows, [2]string{k, v})
		}
	}
	add("Command", it.Command)
	if it.ExitStatus != nil {
		add("Exit status", fmt.Sprint(*it.ExitStatus))
	}
	add("Source revision", it.Revision)
	add("Environment", it.Environment)
	add("Artifact", it.ArtifactPath)
	if len(it.CoveredFiles) > 0 {
		add("Covered files", strings.Join(it.CoveredFiles, ", "))
	}
	if len(rows) > 0 || it.ArtifactURL != "" || len(it.Facts) > 0 || !it.Imported {
		b.WriteString(`<dl class="ev-meta">`)
		for _, r := range rows {
			if r[0] == "Command" || r[0] == "Source revision" || r[0] == "Artifact" {
				fmt.Fprintf(b, `<div><dt>%s</dt><dd><code>%s</code></dd></div>`, esc(r[0]), esc(r[1]))
				continue
			}
			fmt.Fprintf(b, `<div><dt>%s</dt><dd>%s</dd></div>`, esc(r[0]), esc(r[1]))
		}
		if it.ArtifactURL != "" {
			fmt.Fprintf(b, `<div><dt>Artifact link</dt><dd><a href="%s" rel="noreferrer">%s</a></dd></div>`, esc(it.ArtifactURL), esc(it.ArtifactURL))
		}
		for _, f := range it.Facts {
			fmt.Fprintf(b, `<div><dt>%s</dt><dd>%s</dd></div>`, esc(factLabel(f.Kind)), esc(f.Text))
		}
		if !it.Imported || it.Revision != "" {
			fmt.Fprintf(b, `<div><dt>Freshness</dt><dd><span class="ev-fresh ev-f-%s">%s</span> %s`, esc(it.Freshness), esc(freshnessLabel(it.Freshness)), esc(it.FreshnessNote))
			if len(it.ChangedFiles) > 0 {
				fmt.Fprintf(b, `: %s. The proof stays as recorded.`, esc(strings.Join(it.ChangedFiles, ", ")))
			}
			b.WriteString(`</dd></div>`)
		}
		b.WriteString(`</dl>`)
	}
	b.WriteString(`</li>`)
}

func reportedLabel(s string) string {
	switch s {
	case "met":
		return "met"
	case "partial":
		return "partial"
	case "not-met":
		return "not met"
	}
	return s
}

func factLabel(kind string) string {
	switch kind {
	case "tests":
		return "Test count"
	case "exit":
		return "Exit status"
	case "revision":
		return "Revision or hash"
	case "merge":
		return "Merge"
	}
	return kind
}

func freshnessLabel(f string) string {
	switch f {
	case "current":
		return "Current"
	case "stale":
		return "Stale"
	}
	return "Unknown"
}

// evidenceAnchorScript opens the disclosure a #evidence-req-N link
// targets, so links from the progress section land on visible content.
const evidenceAnchorScript = `<script>(function(){function o(){var h=location.hash.slice(1);if(!h)return;var t=document.getElementById(h);if(!t||!t.closest('#report-evidence'))return;for(var p=t;p;p=p.parentElement){if(p.tagName==='DETAILS')p.open=true}var d=t.querySelector(':scope>details');if(d){d.open=true;var s=d.querySelector('summary');if(s)s.focus()}}window.addEventListener('hashchange',o);o()})()</script>`

const evidenceCSS = `<style>
#report-evidence{--ri-good:#047857}
@media (prefers-color-scheme:dark){#report-evidence{--ri-good:var(--good)}}
#report-evidence{margin:26px 0;padding:16px;background:var(--surface);border:1px solid var(--border);border-radius:10px;overflow-wrap:anywhere}
#report-evidence h2{margin:0 0 10px}
#report-evidence h3{font-size:12px;margin:12px 0 6px;color:var(--text);letter-spacing:.02em}
#report-evidence .rs-state{font-size:12.5px;color:var(--text-dim);margin:0 0 12px}
#report-evidence .rs-state-label{display:inline-block;font-weight:700;font-size:11px;letter-spacing:.04em;text-transform:uppercase;padding:1px 6px;border-radius:4px;border:1px solid var(--border);margin-right:4px}
#report-evidence .rs-available .rs-state-label{color:var(--ri-good)}
#report-evidence .rs-partial .rs-state-label{color:var(--warn)}
#report-evidence .rs-unavailable .rs-state-label{color:var(--danger)}
#report-evidence .ev-summary{font-size:13px;margin:0 0 4px}
#report-evidence .ev-dim,#report-evidence .ev-boxes{color:var(--text-dim);font-size:12.5px}
#report-evidence .ev-reqs{list-style:none;margin:10px 0 0;padding:0}
#report-evidence .ev-req{border-top:1px solid var(--border)}
#report-evidence summary{cursor:pointer;padding:8px 0;min-height:24px;font-size:13px}
#report-evidence summary:focus-visible,#report-evidence a:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
#report-evidence a{color:var(--accent)}
#report-evidence .ev-num{font-weight:700}
#report-evidence .ev-title{display:block;color:var(--text-dim);font-size:12.5px;margin:2px 0 0 18px}
#report-evidence .ev-proof,#report-evidence .ev-result,#report-evidence .ev-fresh{display:inline-block;font-size:11px;font-weight:700;padding:1px 6px;border-radius:4px;border:1px solid currentColor}
#report-evidence .ev-p-observed-passed,#report-evidence .ev-r-passed,#report-evidence .ev-f-current{color:var(--ri-good)}
#report-evidence .ev-p-observed-stale,#report-evidence .ev-p-imported-only,#report-evidence .ev-p-checked-without-proof,#report-evidence .ev-f-stale{color:var(--warn)}
#report-evidence .ev-p-observed-failed,#report-evidence .ev-r-failed{color:var(--danger)}
#report-evidence .ev-p-none,#report-evidence .ev-p-observed-unknown,#report-evidence .ev-r-unknown,#report-evidence .ev-f-unknown{color:var(--text-dim)}
#report-evidence .ev-body{padding:0 0 12px 18px}
#report-evidence .ev-accept{margin:6px 0;padding:6px 10px;background:var(--surface-2);border:1px solid var(--border);border-radius:6px}
#report-evidence .ev-accept p{margin:4px 0;font-size:13px}
#report-evidence .ev-attempt{margin:6px 0 10px;padding-left:10px;border-left:2px solid var(--border)}
#report-evidence .ev-attempt-head{margin:4px 0;font-size:13px}
#report-evidence .ev-items{list-style:none;margin:0;padding:0}
#report-evidence .ev-item{padding:8px 0;border-top:1px dashed var(--border);font-size:13px}
#report-evidence .ev-head{margin:0 0 4px;display:flex;flex-wrap:wrap;gap:6px;align-items:baseline}
#report-evidence .ev-kind{font-size:11px;font-weight:600;text-transform:uppercase;letter-spacing:.04em;color:var(--text-dim)}
#report-evidence .ev-origin{font-size:12.5px}
#report-evidence .ev-superseded{font-size:11.5px;font-style:italic;color:var(--text-dim)}
#report-evidence .ev-claim{margin:2px 0;font-size:12.5px;font-weight:600}
#report-evidence .ev-text{margin:2px 0;white-space:pre-wrap}
#report-evidence .ev-meta{margin:6px 0 0;display:grid;gap:2px}
#report-evidence .ev-meta div{display:flex;flex-wrap:wrap;gap:6px}
#report-evidence .ev-meta dt{font-weight:600;font-size:12px;min-width:110px}
#report-evidence .ev-meta dd{margin:0;font-size:12.5px;min-width:0}
#report-evidence code{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:12px;overflow-wrap:anywhere}
#report-evidence .ev-empty{color:var(--text-dim);font-style:italic;font-size:13px}
#report-evidence .ev-plan,#report-evidence .ev-defs{margin-top:12px;border-top:1px solid var(--border)}
#report-evidence .ev-defs dl{margin:6px 0 0;font-size:12.5px}
#report-evidence .ev-defs dt{font-weight:600;margin-top:6px}
#report-evidence .ev-defs dd{margin:0;color:var(--text-dim)}
</style>`
