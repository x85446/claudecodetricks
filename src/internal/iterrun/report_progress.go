package iterrun

import (
	"fmt"
	"html"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// Section availability. A section never disappears: it says which of
// these it is and why.
const (
	integrityAvailable   = "available"
	integrityPartial     = "partial"
	integrityUnavailable = "unavailable"
)

// ProgressSection is the plan report's step-7 section: Verified progress — plan outcome, implemented tasks, passed validations, blocked requirements and lane lifecycle counts, each in its own unit.
type ProgressSection struct {
	Availability string   `json:"availability"`
	Notes        []string `json:"notes,omitempty"`
	// Source is "record" for observed runs, "legacy" for the read-only
	// view reconstructed from plan files, "plan-file" when the record
	// could not be read, or "none".
	Source           string                    `json:"source"`
	Outcome          ProgressOutcome           `json:"outcome"`
	Requirements     ProgressRequirementCounts `json:"requirements"`
	Lanes            ProgressLaneCounts        `json:"lanes"`
	Attempts         ProgressAttemptCounts     `json:"attempts"`
	RequirementItems []ProgressRequirement     `json:"requirement_items"`
	LaneItems        []ProgressLane            `json:"lane_items"`
}

// ProgressOutcome is the plan's own result, kept apart from requirement
// and lane counts.
type ProgressOutcome struct {
	// State is the latest lifecycle state: completed, archived (with no
	// completion observed), executing, blocked, paused, queued, planned,
	// interrupted, unblocked, admitted or unknown.
	State    string `json:"state"`
	Label    string `json:"label"`
	Terminal bool   `json:"terminal"`
	// Completed and Archived are the authoritative terminal events, when
	// the record or the plan file holds them.
	Completed  *ProgressEvent `json:"completed,omitempty"`
	Archived   *ProgressEvent `json:"archived,omitempty"`
	StatusLine string         `json:"status_line,omitempty"`
	Merge      *ProgressMerge `json:"merge,omitempty"`
	// Missing names terminal observations this execution lacks.
	Missing []string `json:"missing,omitempty"`
}

// ProgressEvent is one lifecycle observation and where it came from.
type ProgressEvent struct {
	At          *time.Time `json:"at,omitempty"`
	Provenance  string     `json:"provenance"`
	Via         string     `json:"via,omitempty"`
	Reason      string     `json:"reason,omitempty"`
	ArchiveFile string     `json:"archive_file,omitempty"`
}

// ProgressMerge is the plan's merge outcome: a delivery record, or a
// reference imported from the plan's own prose.
type ProgressMerge struct {
	Ref      string     `json:"ref,omitempty"`
	Revision string     `json:"revision,omitempty"`
	URL      string     `json:"url,omitempty"`
	Result   string     `json:"result,omitempty"`
	At       *time.Time `json:"at,omitempty"`
	Imported bool       `json:"imported"`
	Source   string     `json:"source"` // delivery record | plan log | plan status line
	Text     string     `json:"text,omitempty"`
}

// ProgressRequirementCounts counts requirements (numbered task and
// validation pairs). Every figure is in requirements.
type ProgressRequirementCounts struct {
	Total             int `json:"total"`
	Complete          int `json:"complete"`
	TasksImplemented  int `json:"tasks_implemented"`
	ValidationsPassed int `json:"validations_passed"`
	// ValidationsObserved counts passed validations backed by observed
	// (not imported) passing evidence.
	ValidationsObserved int `json:"validations_with_observed_proof"`
	Blocked             int `json:"blocked"`
	InProgress          int `json:"in_progress"`
	NotStarted          int `json:"not_started"`
}

// ProgressLaneCounts counts lanes: the coordinator and each team. Every
// figure is in lanes.
type ProgressLaneCounts struct {
	Total     int `json:"total"`
	Completed int `json:"completed"`
	Active    int `json:"active"`
	Blocked   int `json:"blocked"`
	Queued    int `json:"queued"`
	// Unknown counts lanes with no terminal observation in a plan that
	// has ended.
	Unknown int `json:"unknown"`
}

// ProgressAttemptCounts counts recorded attempts. Captured is false when
// no attempt was recorded at all; the counts are then unknown, not zero.
type ProgressAttemptCounts struct {
	Captured  bool           `json:"captured"`
	Total     int            `json:"total"`
	ByOutcome map[string]int `json:"by_outcome"`
	Note      string         `json:"note,omitempty"`
}

// ProgressRequirement is one requirement's progress and the basis for it.
type ProgressRequirement struct {
	Number            int    `json:"number"`
	Title             string `json:"title,omitempty"`
	Lane              string `json:"lane,omitempty"`
	State             string `json:"state"` // complete | in-progress | blocked | not-started
	TaskDone          bool   `json:"task_done"`
	ValidationPassed  bool   `json:"validation_passed"`
	ObservedProof     bool   `json:"observed_proof"`
	Basis             string `json:"basis"`
	TaskChecked       *bool  `json:"task_checked,omitempty"`
	ValidationChecked *bool  `json:"validation_checked,omitempty"`
	Reported          string `json:"reported,omitempty"`
	Attempts          int    `json:"attempts"`
	EvidenceAnchor    string `json:"evidence_anchor"`
}

// ProgressLane is one lane's lifecycle state and the observation it rests
// on.
type ProgressLane struct {
	Name         string `json:"name"`
	Kind         string `json:"kind"`  // coordinator | team
	State        string `json:"state"` // completed | active | blocked | queued | unknown
	Basis        string `json:"basis"`
	Requirements []int  `json:"requirements,omitempty"`
	// DependsOn is the team's own Depends-on list from the Teams table.
	DependsOn []string `json:"depends_on,omitempty"`
	Activity  bool     `json:"activity"`
}

// reportRequirement is one requirement merged from the plan file, the run
// record and the timeline lanes. Both the progress and evidence sections
// read it.
type reportRequirement struct {
	Number            int
	Task, Validation  string
	TaskChecked       *bool
	ValidationChecked *bool
	// Reported is the latest status a team report or the plan log
	// claims for the requirement; ReportedBy names where it came from.
	Reported   string
	ReportedBy string
	// LogMentions counts plan-log passages that name the requirement.
	LogMentions int
	Lane        string
	Attempts    []runrecord.AttemptView
	Evidence    []runrecord.EvidenceEntry
}

func integrityBool(b bool) *bool { return &b }

// reportRequirements merges every numbered requirement the plan file, the
// run record and the timeline name.
func reportRequirements(in ReportInput, src reportPlanSource) []reportRequirement {
	byNum := map[int]*reportRequirement{}
	get := func(n int) *reportRequirement {
		r, ok := byNum[n]
		if !ok {
			r = &reportRequirement{Number: n}
			byNum[n] = r
		}
		return r
	}
	if in.RecordErr == nil {
		for _, run := range in.Record.Runs {
			for _, q := range run.Requirements {
				if q.Number <= 0 {
					continue
				}
				r := get(q.Number)
				if q.Task != "" {
					r.Task = q.Task
				}
				if q.Validation != "" {
					r.Validation = q.Validation
				}
				if q.TaskChecked != nil {
					r.TaskChecked = q.TaskChecked
				}
				if q.ValidationChecked != nil {
					r.ValidationChecked = q.ValidationChecked
				}
				r.Attempts = append(r.Attempts, q.Attempts...)
				r.Evidence = append(r.Evidence, q.Evidence...)
			}
		}
	}
	laneOf := map[int]string{}
	for _, row := range in.Rows {
		for _, sd := range row.steps {
			r := get(sd.Num)
			if r.Task == "" {
				r.Task = sd.Step
			}
			if r.Validation == "" {
				r.Validation = sd.Validation
			}
			laneOf[sd.Num] = row.label
		}
	}
	for team, meta := range src.Teams {
		for _, n := range meta.stepNums {
			if _, ok := laneOf[n]; !ok {
				laneOf[n] = team
			}
		}
	}
	for n, text := range src.Steps {
		if r := get(n); text != "" {
			r.Task = text
		}
	}
	for n, text := range src.Validations {
		if r := get(n); text != "" {
			r.Validation = text
		}
	}
	// The plan file's checkboxes are its current state and win over a
	// recorded observation of them.
	for n, b := range src.Boxes {
		r := get(n)
		if b.StepBoxed {
			r.TaskChecked = integrityBool(b.StepChecked)
		}
		if b.ValidationBoxed {
			r.ValidationChecked = integrityBool(b.ValidationChecked)
		}
	}

	known := map[int]bool{}
	for n := range byNum {
		known[n] = true
	}
	// Team reports: the last one per step wins.
	for _, rep := range src.Reports {
		if r, ok := byNum[rep.Step]; ok {
			r.Reported, r.ReportedBy = rep.Status, "team "+rep.Team+" validation report"
		}
	}
	// Log claims fill steps no team reported on, in log order.
	teamReported := map[int]bool{}
	for _, rep := range src.Reports {
		teamReported[rep.Step] = true
	}
	for _, e := range src.Log {
		for _, seg := range splitLogEntry(e.Text, known) {
			for _, n := range seg.Steps {
				byNum[n].LogMentions++
			}
			claim := reportedClaim(seg.Text)
			if claim == "" {
				continue
			}
			for _, n := range seg.Steps {
				if !teamReported[n] {
					byNum[n].Reported, byNum[n].ReportedBy = claim, "plan log report"
				}
			}
		}
	}
	// Without a readable plan file, the timeline's own step status is the
	// only report there is.
	if !src.Read {
		for _, row := range in.Rows {
			for _, sd := range row.steps {
				if r := byNum[sd.Num]; r.Reported == "" && sd.VStatus != "" {
					r.Reported, r.ReportedBy = sd.VStatus, "timeline step status"
				}
			}
		}
	}

	var out []reportRequirement
	for n, r := range byNum {
		if r.Task == "" && r.Validation == "" && r.TaskChecked == nil && r.ValidationChecked == nil && len(r.Attempts) == 0 && len(r.Evidence) == 0 {
			continue
		}
		r.Lane = laneOf[n]
		if r.Lane == "" {
			r.Lane = "coordinator"
		}
		out = append(out, *r)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Number < out[j].Number })
	return out
}

// latestObservedEvidence returns the newest evidence entry that is a
// runtime observation rather than imported prose.
func latestObservedEvidence(r reportRequirement) (runrecord.EvidenceEntry, bool) {
	var all []runrecord.EvidenceEntry
	all = append(all, r.Evidence...)
	for _, a := range r.Attempts {
		all = append(all, a.Evidence...)
	}
	var best runrecord.EvidenceEntry
	found := false
	for _, e := range all {
		if e.Imported {
			continue
		}
		if !found || !e.At.Before(best.At) {
			best, found = e, true
		}
	}
	return best, found
}

func latestAttempt(r reportRequirement) (runrecord.AttemptView, bool) {
	var best runrecord.AttemptView
	found := false
	for _, a := range r.Attempts {
		at := a.StartedAt
		if a.FinishedAt != nil {
			at = a.FinishedAt
		}
		var bestAt *time.Time
		if found {
			bestAt = best.StartedAt
			if best.FinishedAt != nil {
				bestAt = best.FinishedAt
			}
		}
		if !found || (at != nil && (bestAt == nil || !at.Before(*bestAt))) {
			best, found = a, true
		}
	}
	return best, found
}

func requirementProgress(r reportRequirement, laneState map[string]string) ProgressRequirement {
	p := ProgressRequirement{Number: r.Number, Title: r.Task, Lane: r.Lane, TaskChecked: r.TaskChecked,
		ValidationChecked: r.ValidationChecked, Reported: r.Reported, Attempts: len(r.Attempts),
		EvidenceAnchor: fmt.Sprintf("evidence-req-%d", r.Number)}
	attempt, hasAttempt := latestAttempt(r)
	observed, hasObserved := latestObservedEvidence(r)
	var basis []string
	switch {
	case r.ValidationChecked != nil && *r.ValidationChecked:
		p.ValidationPassed = true
		basis = append(basis, "validation box checked")
	case r.Reported == "met":
		// A report can raise a requirement but never lower a checked box.
		p.ValidationPassed = true
		basis = append(basis, "reported met ("+r.ReportedBy+")")
	case hasObserved && observed.Result == "passed":
		p.ValidationPassed = true
		basis = append(basis, "observed passing evidence")
	}
	if hasObserved && observed.Result == "passed" {
		p.ObservedProof = true
	}
	switch {
	case r.TaskChecked != nil && *r.TaskChecked:
		p.TaskDone = true
		if !p.ValidationPassed {
			basis = append(basis, "task box checked")
		}
	case p.ValidationPassed, r.Reported == "partial":
		p.TaskDone = true
	case hasAttempt && attempt.Outcome == "passed":
		p.TaskDone = true
	}
	blocked := r.Reported == "not-met" || (hasAttempt && (attempt.Outcome == "blocked" || attempt.Outcome == "failed")) || laneState[r.Lane] == "blocked"
	switch {
	case p.ValidationPassed:
		p.State = "complete"
	case blocked:
		p.State = "blocked"
		switch {
		case r.Reported == "not-met":
			basis = append(basis, "reported not met ("+r.ReportedBy+")")
		case hasAttempt && (attempt.Outcome == "blocked" || attempt.Outcome == "failed"):
			basis = append(basis, fmt.Sprintf("attempt %d %s", attempt.Number, attempt.Outcome))
		default:
			basis = append(basis, "lane "+r.Lane+" blocked")
		}
	case p.TaskDone || r.Reported == "partial" || hasAttempt || r.LogMentions > 0:
		p.State = "in-progress"
		switch {
		case r.Reported == "partial":
			basis = append(basis, "reported partial ("+r.ReportedBy+")")
		case hasAttempt && !p.TaskDone:
			basis = append(basis, fmt.Sprintf("attempt %d %s", attempt.Number, attempt.Outcome))
		case !p.TaskDone:
			basis = append(basis, fmt.Sprintf("the plan log reports work on it %d %s, with no result", r.LogMentions, unitWord(r.LogMentions, "time", "times")))
		}
	default:
		p.State = "not-started"
		basis = append(basis, "no checked box, report or attempt")
	}
	p.Basis = strings.Join(basis, "; ")
	return p
}

func buildProgressSection(in ReportInput) ProgressSection {
	src := reportPlanSourceFor(in.Plan)
	s := ProgressSection{Source: "none", RequirementItems: []ProgressRequirement{}, LaneItems: []ProgressLane{},
		Attempts: ProgressAttemptCounts{ByOutcome: map[string]int{}}}
	switch {
	case in.RecordErr != nil:
		s.Source = "plan-file"
		s.Notes = append(s.Notes, "The run record could not be read ("+in.RecordErr.Error()+"); outcome and requirement state come from the plan file alone.")
	case in.Record.Source != "":
		s.Source = in.Record.Source
	}
	if s.Source == "legacy" {
		s.Notes = append(s.Notes, "No observed run record exists for this execution; the outcome and checkboxes are read from its plan file (provenance legacy:plan-file).")
	}
	s.Outcome = progressOutcome(in, src)

	lanes := progressLanes(in, src, s.Outcome)
	laneState := map[string]string{}
	for _, l := range lanes {
		laneState[l.Name] = l.State
	}
	s.LaneItems = lanes
	for _, l := range lanes {
		s.Lanes.Total++
		switch l.State {
		case "completed":
			s.Lanes.Completed++
		case "active":
			s.Lanes.Active++
		case "blocked":
			s.Lanes.Blocked++
		case "queued":
			s.Lanes.Queued++
		default:
			s.Lanes.Unknown++
		}
	}

	for _, r := range reportRequirements(in, src) {
		p := requirementProgress(r, laneState)
		s.RequirementItems = append(s.RequirementItems, p)
		c := &s.Requirements
		c.Total++
		if p.TaskDone {
			c.TasksImplemented++
		}
		if p.ValidationPassed {
			c.ValidationsPassed++
			if p.ObservedProof {
				c.ValidationsObserved++
			}
		}
		switch p.State {
		case "complete":
			c.Complete++
		case "blocked":
			c.Blocked++
		case "in-progress":
			c.InProgress++
		default:
			c.NotStarted++
		}
		for _, a := range r.Attempts {
			s.Attempts.Total++
			s.Attempts.ByOutcome[a.Outcome]++
		}
	}
	// Plan-level attempts (delivery) count as attempts too.
	if in.RecordErr == nil {
		for _, run := range in.Record.Runs {
			for _, q := range run.Requirements {
				if q.Number > 0 {
					continue
				}
				for _, a := range q.Attempts {
					s.Attempts.Total++
					s.Attempts.ByOutcome[a.Outcome]++
				}
			}
		}
	}
	s.Attempts.Captured = s.Attempts.Total > 0
	if !s.Attempts.Captured {
		s.Attempts.Note = "No attempt was recorded for this execution, so attempt counts are unknown."
		if s.Source == "legacy" {
			s.Attempts.Note = "No attempt was recorded: the plan file states checkboxes, not attempts, and no Conductor assignment ran this execution. Attempt counts are unknown."
		}
	}

	switch {
	case s.Requirements.Total == 0 && s.Lanes.Total == 0 && s.Outcome.State == "unknown":
		s.Availability = integrityUnavailable
		s.Notes = append(s.Notes, "Neither the run record nor the plan file supplied an outcome, requirements or lanes.")
	case in.RecordErr != nil || !src.Read || s.Requirements.Total == 0:
		s.Availability = integrityPartial
		if !src.Read {
			s.Notes = append(s.Notes, "The plan file could not be read; requirements come from the run record and timeline only.")
		}
		if s.Requirements.Total == 0 {
			s.Notes = append(s.Notes, "No numbered requirements were found.")
		}
	default:
		s.Availability = integrityAvailable
	}
	return s
}

var outcomeLabels = map[string]string{
	"executing": "Executing", "blocked": "Blocked", "paused": "Paused", "queued": "Queued",
	"planned": "Planned", "interrupted": "Interrupted", "unblocked": "Unblocked, waiting to resume",
	"admitted": "Admitted, not yet started", "resumed": "Executing",
}

func progressOutcome(in ReportInput, src reportPlanSource) ProgressOutcome {
	o := ProgressOutcome{State: "unknown", StatusLine: strings.TrimSpace(in.Plan.Status)}
	var run *runrecord.Run
	if in.RecordErr == nil {
		for i := range in.Record.Runs {
			r := &in.Record.Runs[i]
			if run == nil || (r.OpenedAt != nil && (run.OpenedAt == nil || !r.OpenedAt.Before(*run.OpenedAt))) {
				run = r
			}
		}
	}
	if run != nil {
		for _, l := range run.Lifecycle {
			ev := &ProgressEvent{At: integrityTime(l.At), Provenance: l.Provenance, Via: l.ObservedVia, Reason: l.Reason, ArchiveFile: l.ArchiveFile}
			switch l.State {
			case "completed":
				o.Completed = ev
			case "archived":
				o.Archived = ev
			}
		}
		if run.State != "" && run.State != "unknown" {
			o.State = run.State
		}
	}
	// The plan file is the fallback when no lifecycle was recorded.
	if o.Completed == nil && o.State == "unknown" && (in.Plan.Phase == "complete" || in.Plan.Phase == "done") {
		ev := &ProgressEvent{Provenance: "plan-file", Via: "phase: " + in.Plan.Phase}
		if t, ok := in.Plan.FinishedAt(); ok {
			ev.At = integrityTime(t)
		}
		o.Completed = ev
	}
	if o.Archived == nil && o.State == "unknown" && in.Plan.Archived {
		o.Archived = &ProgressEvent{Provenance: "plan-file", Via: "archive file", ArchiveFile: in.Plan.ArchiveFile}
	}
	if o.State == "unknown" && !in.Plan.Archived && in.Plan.Name != "" {
		if st := in.Plan.StateLabel(); st != "" && st != "complete" && st != "done" {
			o.State = st
		}
	}
	switch {
	case o.Completed != nil:
		o.State, o.Terminal = "completed", true
		o.Label = "Completed"
		if o.Archived != nil {
			o.Label = "Completed and archived"
		}
	case o.Archived != nil:
		o.State, o.Terminal = "archived", true
		o.Label = "Archived without a completion record"
		o.Missing = append(o.Missing, "No completion event was recorded for this execution; it was archived without one.")
	default:
		o.Label = outcomeLabels[o.State]
		if o.Label == "" {
			o.Label = "Unknown: no lifecycle observation"
			o.State = "unknown"
		}
	}
	if o.Terminal && in.Plan.Archived && o.Archived == nil {
		o.Missing = append(o.Missing, "The plan file is archived, but no archive event was recorded.")
	}
	o.Merge = progressMerge(run, in.Plan, src)
	return o
}

func integrityTime(t time.Time) *time.Time { return &t }

var reURL = regexp.MustCompile(`https?://[^\s)\]>"']+`)

// progressMerge finds the plan's merge: a recorded merge delivery, else
// the latest merge the plan's own prose reports, marked imported.
func progressMerge(run *runrecord.Run, plan PlanSummary, src reportPlanSource) *ProgressMerge {
	if run != nil {
		for i := len(run.Deliveries) - 1; i >= 0; i-- {
			d := run.Deliveries[i]
			if d.Stage == "merge" {
				return &ProgressMerge{Ref: d.Ref, Revision: d.Revision, URL: d.URL, Result: d.Result, At: integrityTime(d.At),
					Imported: strings.HasPrefix(d.Provenance, "legacy:"), Source: "delivery record", Text: d.Evidence}
			}
		}
	}
	fromText := func(text, source string, at *time.Time) *ProgressMerge {
		m := &ProgressMerge{Imported: true, Source: source, Text: text, At: at, Result: "unknown"}
		for _, f := range evidenceFacts(text) {
			switch {
			case f.Kind == "merge" && m.Ref == "":
				m.Ref = f.Text
			case f.Kind == "revision" && m.Revision == "":
				m.Revision = f.Text
			}
		}
		// The commit a merge produced, when the prose names it, beats any
		// other revision on the line.
		if mm := reFactMergedAs.FindStringSubmatch(text); mm != nil && isRevisionLike(mm[1]) {
			m.Revision = strings.ToLower(mm[1])
		}
		if u := reURL.FindString(text); u != "" {
			m.URL = strings.TrimRight(u, ".,;")
		}
		if m.Ref == "" {
			return nil
		}
		return m
	}
	for i := len(src.Log) - 1; i >= 0; i-- {
		if m := fromText(src.Log[i].Text, "plan log", src.Log[i].At); m != nil {
			return m
		}
	}
	return fromText(plan.Status, "plan status line", nil)
}

// progressLanes classifies each lane. A team lane closes on its own
// terminal observation (the Teams table or its log's TEAM DONE/BLOCKED
// line); the coordinator lane closes on the plan's completion event. In a
// plan that has ended, a lane with neither is unknown — never queued or
// running.
func progressLanes(in ReportInput, src reportPlanSource, o ProgressOutcome) []ProgressLane {
	type lane struct {
		name, status string
		coordinator  bool
		activity     bool
		steps        []int
		dependsOn    []string
	}
	var lanes []lane
	seen := map[string]int{}
	labelOf := map[string]string{}
	for _, row := range in.Rows {
		labelOf[row.key] = row.label
	}
	for _, row := range in.Rows {
		l := lane{name: row.label, status: row.status, coordinator: row.key == "", activity: len(row.spans) > 0}
		for _, k := range row.dependsOn {
			if label, ok := labelOf[k]; ok {
				k = label
			}
			l.dependsOn = append(l.dependsOn, k)
		}
		for _, sd := range row.steps {
			l.steps = append(l.steps, sd.Num)
		}
		if i, ok := seen[l.name]; ok {
			lanes[i].activity = lanes[i].activity || l.activity
			continue
		}
		seen[l.name] = len(lanes)
		lanes = append(lanes, l)
	}
	if len(in.Rows) == 0 {
		owned := map[int]bool{}
		var names []string
		for name := range src.Teams {
			names = append(names, name)
		}
		sort.Strings(names)
		for _, name := range names {
			meta := src.Teams[name]
			lanes = append(lanes, lane{name: name, status: meta.status, steps: meta.stepNums, dependsOn: meta.dependsOn})
			for _, n := range meta.stepNums {
				owned[n] = true
			}
		}
		var rest []int
		for n := range src.Steps {
			if !owned[n] {
				rest = append(rest, n)
			}
		}
		if len(rest) > 0 {
			sort.Ints(rest)
			lanes = append(lanes, lane{name: "coordinator", coordinator: true, steps: rest})
		}
	}

	var out []ProgressLane
	for _, l := range lanes {
		p := ProgressLane{Name: l.name, Kind: "team", Requirements: l.steps, DependsOn: l.dependsOn, Activity: l.activity}
		status := strings.ToLower(strings.TrimSpace(l.status))
		end := src.TeamEnds[l.name]
		if l.coordinator {
			p.Kind = "coordinator"
			switch {
			case o.Completed != nil:
				p.State, p.Basis = "completed", "closed by the plan's completion event"
			case o.Terminal:
				p.State, p.Basis = "unknown", "plan archived with no completion event; no terminal observation for this lane"
			case o.State == "executing" || o.State == "resumed" || o.State == "unblocked" || o.State == "admitted":
				p.State, p.Basis = "active", "plan is "+o.State
			case o.State == "blocked":
				p.State, p.Basis = "blocked", "plan is blocked"
			case o.State == "queued" || o.State == "planned" || o.State == "paused" || o.State == "interrupted":
				p.State, p.Basis = "queued", "plan is "+o.State+"; waits to run"
			default:
				p.State, p.Basis = "unknown", "no lifecycle observation for the plan"
			}
			out = append(out, p)
			continue
		}
		switch {
		case strings.HasPrefix(status, "done"):
			p.State, p.Basis = "completed", "Teams table status: done"
		case strings.HasPrefix(status, "blocked"):
			p.State, p.Basis = "blocked", "Teams table status: "+l.status
		case end == "done":
			p.State, p.Basis = "completed", "TEAM DONE line in the team log"
		case end == "blocked":
			p.State, p.Basis = "blocked", "TEAM BLOCKED line in the team log"
		case o.Terminal && l.activity:
			p.State, p.Basis = "unknown", "activity recorded, but no terminal observation before the plan ended"
		case o.Terminal:
			p.State, p.Basis = "unknown", "no activity and no terminal observation before the plan ended"
		case status == "in-progress" || (status == "" && l.activity):
			p.State, p.Basis = "active", "recorded activity, no terminal observation yet"
			if status == "in-progress" {
				p.Basis = "Teams table status: in-progress"
			}
		case status == "" || status == "pending" || status == "queued":
			p.State, p.Basis = "queued", "no activity recorded yet"
		default:
			p.State, p.Basis = "queued", "Teams table status: "+l.status
		}
		out = append(out, p)
	}
	return out
}

func unitWord(n int, one, many string) string {
	if n == 1 {
		return one
	}
	return many
}

func writeProgressSection(b *strings.Builder, s ProgressSection) {
	esc := html.EscapeString
	b.WriteString(progressCSS)
	b.WriteString(`<section class="report-section" id="report-progress" aria-labelledby="report-progress-title">`)
	b.WriteString(`<h2 id="report-progress-title">Verified progress</h2>`)
	writeIntegrityState(b, s.Availability, s.Notes, map[string]string{
		integrityAvailable:   "Available",
		integrityPartial:     "Partial",
		integrityUnavailable: "Unavailable",
	})
	if s.Availability == integrityUnavailable {
		b.WriteString(`</section>`)
		return
	}

	// Outcome first: the plan's own result, separate from any count.
	o := s.Outcome
	fmt.Fprintf(b, `<div class="pg-outcome pg-o-%s"><p class="pg-kicker">Plan outcome</p><p class="pg-outcome-value">%s</p>`, esc(o.State), esc(o.Label))
	if o.StatusLine != "" {
		fmt.Fprintf(b, `<p class="pg-line"><span class="pg-key">Plan status line</span> %s</p>`, esc(o.StatusLine))
	}
	if ev := o.Completed; ev != nil {
		fmt.Fprintf(b, `<p class="pg-line"><span class="pg-key">Completion</span> %s</p>`, esc(describeProgressEvent(*ev)))
	}
	if ev := o.Archived; ev != nil {
		text := describeProgressEvent(*ev)
		if ev.ArchiveFile != "" {
			text += " · " + ev.ArchiveFile
		}
		fmt.Fprintf(b, `<p class="pg-line"><span class="pg-key">Archive</span> %s</p>`, esc(text))
	}
	if m := o.Merge; m != nil {
		var parts []string
		if m.Ref != "" {
			parts = append(parts, m.Ref)
		}
		if m.Revision != "" {
			parts = append(parts, "revision "+shortRevision(m.Revision))
		}
		if m.Result != "" && m.Result != "unknown" {
			parts = append(parts, m.Result)
		}
		from := "from the " + m.Source
		if m.Imported {
			from += ", imported prose; result not observed"
		}
		if m.At != nil {
			from += ", " + m.At.UTC().Format("2006-01-02 15:04 UTC")
		}
		fmt.Fprintf(b, `<p class="pg-line"><span class="pg-key">Merge</span> %s <span class="pg-dim">(%s)</span>`, esc(strings.Join(parts, " · ")), esc(from))
		if m.URL != "" {
			fmt.Fprintf(b, ` <a href="%s" rel="noreferrer">open</a>`, esc(m.URL))
		}
		b.WriteString(`</p>`)
	}
	for _, miss := range o.Missing {
		fmt.Fprintf(b, `<p class="pg-line pg-missing"><span class="pg-key">Missing</span> %s</p>`, esc(miss))
	}
	b.WriteString(`</div>`)

	// Requirements, in requirements.
	c := s.Requirements
	fmt.Fprintf(b, `<div class="pg-group"><h3 id="report-progress-req">Requirements <span class="pg-unit">%d %s</span></h3>`, c.Total, unitWord(c.Total, "requirement", "requirements"))
	if c.Total == 0 {
		b.WriteString(`<p class="pg-dim">No numbered requirements were found for this plan.</p>`)
	} else {
		b.WriteString(`<dl class="pg-tiles" aria-labelledby="report-progress-req">`)
		tile := func(class, label, value, sub string) {
			fmt.Fprintf(b, `<div class="pg-tile %s"><dt>%s</dt><dd class="pg-n">%s</dd>`, class, esc(label), esc(value))
			if sub != "" {
				fmt.Fprintf(b, `<dd class="pg-sub">%s</dd>`, esc(sub))
			}
			b.WriteString(`</div>`)
		}
		of := func(n int) string { return fmt.Sprintf("%d of %d", n, c.Total) }
		tile("pg-t-good", "Complete", of(c.Complete), "requirements whose validation passed")
		tile("", "Tasks implemented", of(c.TasksImplemented), "requirements")
		tile("", "Validations passed", of(c.ValidationsPassed), fmt.Sprintf("%d with observed proof", c.ValidationsObserved))
		tile("pg-t-bad", "Blocked or not met", of(c.Blocked), "requirements")
		tile("pg-t-warn", "In progress", of(c.InProgress), "requirements")
		tile("", "Not started", of(c.NotStarted), "requirements")
		b.WriteString(`</dl>`)
		writeRequirementGrid(b, s.RequirementItems)
		fmt.Fprintf(b, `<details class="pg-more"><summary>Requirement by requirement (%d)</summary><ul class="pg-list">`, c.Total)
		for _, r := range s.RequirementItems {
			fmt.Fprintf(b, `<li><span class="pg-badge pg-b-%s">%s</span> <a href="#%s">Requirement %d</a> <span class="pg-dim">· lane %s</span><span class="pg-basis">%s</span></li>`,
				esc(r.State), esc(requirementStateLabel(r.State)), esc(r.EvidenceAnchor), r.Number, esc(r.Lane), esc(requirementFacts(r)))
		}
		b.WriteString(`</ul></details>`)
	}
	b.WriteString(`</div>`)

	// Lanes, in lanes.
	l := s.Lanes
	fmt.Fprintf(b, `<div class="pg-group"><h3 id="report-progress-lanes">Lanes <span class="pg-unit">%d %s</span></h3>`, l.Total, unitWord(l.Total, "lane", "lanes"))
	if l.Total == 0 {
		b.WriteString(`<p class="pg-dim">No lane was observed: the plan has no coordinator steps, teams or recorded activity.</p>`)
	} else {
		fmt.Fprintf(b, `<dl class="pg-tiles pg-tiles-lanes" aria-labelledby="report-progress-lanes">`)
		for _, t := range []struct {
			class, label string
			n            int
		}{{"pg-t-good", "Completed", l.Completed}, {"pg-t-warn", "Active", l.Active}, {"pg-t-bad", "Blocked", l.Blocked}, {"", "Queued", l.Queued}, {"pg-t-unknown", "Unknown end", l.Unknown}} {
			fmt.Fprintf(b, `<div class="pg-tile %s"><dt>%s</dt><dd class="pg-n">%d</dd><dd class="pg-sub">%s</dd></div>`, t.class, t.label, t.n, unitWord(t.n, "lane", "lanes"))
		}
		b.WriteString(`</dl>`)
		fmt.Fprintf(b, `<details class="pg-more"><summary>Lane by lane (%d)</summary><ul class="pg-list">`, l.Total)
		for _, ln := range s.LaneItems {
			reqs := "no requirements"
			if len(ln.Requirements) > 0 {
				var nums []string
				for _, n := range ln.Requirements {
					nums = append(nums, fmt.Sprint(n))
				}
				reqs = unitWord(len(ln.Requirements), "requirement ", "requirements ") + strings.Join(nums, ", ")
			}
			if len(ln.DependsOn) > 0 {
				reqs += " · depends on " + strings.Join(ln.DependsOn, ", ")
			}
			fmt.Fprintf(b, `<li><span class="pg-badge pg-l-%s">%s</span> <strong>%s</strong> <span class="pg-dim">· %s · %s</span><span class="pg-basis">%s</span></li>`,
				esc(ln.State), esc(laneStateLabel(ln.State)), esc(ln.Name), esc(ln.Kind), esc(reqs), esc(ln.Basis))
		}
		b.WriteString(`</ul></details>`)
	}
	b.WriteString(`</div>`)

	// Attempts, in attempts.
	a := s.Attempts
	b.WriteString(`<div class="pg-group"><h3>Attempts`)
	if a.Captured {
		fmt.Fprintf(b, ` <span class="pg-unit">%d %s</span></h3><p class="pg-line">`, a.Total, unitWord(a.Total, "attempt", "attempts"))
		var keys []string
		for k := range a.ByOutcome {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		var parts []string
		for _, k := range keys {
			parts = append(parts, fmt.Sprintf("%d %s", a.ByOutcome[k], attemptOutcomeLabel(k)))
		}
		b.WriteString(esc(strings.Join(parts, " · ")) + `</p>`)
	} else {
		fmt.Fprintf(b, ` <span class="pg-unit">unknown</span></h3><p class="pg-dim">%s</p>`, esc(a.Note))
	}
	b.WriteString(`</div>`)

	b.WriteString(`<details class="pg-defs"><summary>How progress is counted</summary><dl>`)
	for _, d := range [][2]string{
		{"Plan outcome", "The execution's latest lifecycle state. Completed and archived come from recorded lifecycle events, or from the plan file's Finished: field and archive name when no run was observed."},
		{"Requirement", "One numbered task and validation pair. Requirement counts never include lanes or attempts."},
		{"Complete", "The validation box is checked, a team or the plan log reports it met, or observed evidence passed. An earlier partial report never undoes a checked box."},
		{"Task implemented", "The task box is checked, its validation passed, a report says partial or met, or an attempt passed."},
		{"In progress", "Not complete and not blocked, with a checked task box, an attempt, a partial report, or plan-log lines naming the requirement."},
		{"Observed proof", "Evidence recorded at run time with a result, as opposed to imported prose or a checked box. See Validation evidence."},
		{"Blocked or not met", "Not complete, and a report says not met, the latest attempt was blocked or failed, or the owning lane is blocked."},
		{"Lane", "The coordinator or one team. A team lane ends on its Teams table status or its log's TEAM DONE or TEAM BLOCKED line; the coordinator lane ends with the plan's completion event."},
		{"Unknown end", "The plan ended, but the lane has no terminal observation. It is not counted as queued or running."},
		{"Attempt", "One recorded try at a requirement. Without any recorded attempt the count is unknown, not zero."},
	} {
		fmt.Fprintf(b, `<dt>%s</dt><dd>%s</dd>`, esc(d[0]), esc(d[1]))
	}
	b.WriteString(`</dl></details></section>`)
}

// writeRequirementGrid writes one cell per requirement, coloured and shaped
// by its progress state. Each cell is a link to that requirement's evidence,
// so the grid is reachable and operable from the keyboard.
func writeRequirementGrid(b *strings.Builder, items []ProgressRequirement) {
	if len(items) == 0 {
		return
	}
	b.WriteString(`<ul class="bd-grid" aria-label="Requirements by state; each opens its evidence">`)
	for _, r := range items {
		label := fmt.Sprintf("Requirement %d, %s", r.Number, requirementStateLabel(r.State))
		if r.Lane != "" {
			label += ", lane " + r.Lane
		}
		fmt.Fprintf(b, `<li><a class="bd-cell bd-%s" href="#%s" title="%s" aria-label="%s">%d</a></li>`,
			html.EscapeString(r.State), html.EscapeString(r.EvidenceAnchor), html.EscapeString(label), html.EscapeString(label), r.Number)
	}
	b.WriteString(`</ul><p class="bd-key" aria-hidden="true"><span class="bd-k bd-complete">&#10003;</span> complete <span class="bd-k bd-in-progress">&#9679;</span> in progress <span class="bd-k bd-blocked">&#10007;</span> blocked or not met <span class="bd-k bd-not-started"></span> not started</p>`)
}

func describeProgressEvent(ev ProgressEvent) string {
	var parts []string
	if ev.At != nil {
		parts = append(parts, ev.At.UTC().Format("2006-01-02 15:04 UTC"))
	} else {
		parts = append(parts, "time not recorded")
	}
	switch {
	case ev.Provenance == "observed":
		parts = append(parts, "observed")
	case strings.HasPrefix(ev.Provenance, "legacy:"):
		parts = append(parts, "reconstructed from the plan file ("+ev.Provenance+")")
	case ev.Provenance != "":
		parts = append(parts, "from the "+strings.ReplaceAll(ev.Provenance, "-", " "))
	}
	if ev.Reason != "" {
		parts = append(parts, ev.Reason)
	} else if ev.Via != "" {
		parts = append(parts, "via "+ev.Via)
	}
	return strings.Join(parts, " · ")
}

func requirementStateLabel(s string) string {
	switch s {
	case "complete":
		return "Complete"
	case "blocked":
		return "Blocked"
	case "in-progress":
		return "In progress"
	}
	return "Not started"
}

func laneStateLabel(s string) string {
	switch s {
	case "completed":
		return "Completed"
	case "active":
		return "Active"
	case "blocked":
		return "Blocked"
	case "queued":
		return "Queued"
	}
	return "Unknown end"
}

func attemptOutcomeLabel(o string) string {
	if o == "open" {
		return "open (no finish recorded)"
	}
	return o
}

func requirementFacts(r ProgressRequirement) string {
	box := func(name string, v *bool) string {
		switch {
		case v == nil:
			return name + " box: none"
		case *v:
			return name + " box: checked"
		}
		return name + " box: unchecked"
	}
	parts := []string{box("task", r.TaskChecked), box("validation", r.ValidationChecked)}
	if r.Basis != "" {
		parts = append(parts, "basis: "+r.Basis)
	}
	if r.Attempts > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", r.Attempts, unitWord(r.Attempts, "attempt", "attempts")))
	}
	return strings.Join(parts, " · ")
}

// writeReportState writes a section's availability line.
func writeIntegrityState(b *strings.Builder, state string, notes []string, labels map[string]string) {
	fmt.Fprintf(b, `<p class="rs-state rs-%s"><span class="rs-state-label">%s</span>`, html.EscapeString(state), html.EscapeString(labels[state]))
	if len(notes) > 0 {
		b.WriteString(` ` + html.EscapeString(strings.Join(notes, " ")))
	}
	b.WriteString(`</p>`)
}

const progressCSS = `<style>
#report-progress{--ri-good:#047857}
@media (prefers-color-scheme:dark){#report-progress{--ri-good:var(--good)}}
#report-progress{margin:26px 0;padding:16px;background:var(--surface);border:1px solid var(--border);border-radius:10px;overflow-wrap:anywhere}
#report-progress h2{margin:0 0 10px}
#report-progress h3{font-size:13px;margin:0 0 8px;color:var(--text)}
#report-progress .rs-state{font-size:12.5px;color:var(--text-dim);margin:0 0 12px}
#report-progress .rs-state-label{display:inline-block;font-weight:700;font-size:11px;letter-spacing:.04em;text-transform:uppercase;padding:1px 6px;border-radius:4px;border:1px solid var(--border);margin-right:4px}
#report-progress .rs-available .rs-state-label{color:var(--ri-good)}
#report-progress .rs-partial .rs-state-label{color:var(--warn)}
#report-progress .rs-unavailable .rs-state-label{color:var(--danger)}
#report-progress .pg-outcome{padding:12px 14px;border:1px solid var(--border);border-left:4px solid var(--text-faint);border-radius:8px;background:var(--surface-2);margin-bottom:16px}
#report-progress .pg-o-completed{border-left-color:var(--ri-good)}
#report-progress .pg-o-blocked,#report-progress .pg-o-archived{border-left-color:var(--danger)}
#report-progress .pg-o-executing,#report-progress .pg-o-paused{border-left-color:var(--warn)}
#report-progress .pg-kicker{margin:0;font-size:11px;letter-spacing:.06em;text-transform:uppercase;color:var(--text-dim);font-weight:600}
#report-progress .pg-outcome-value{margin:2px 0 6px;font-size:18px;font-weight:700}
#report-progress .pg-line{margin:4px 0;font-size:13px}
#report-progress .pg-key{font-weight:600;margin-right:6px}
#report-progress .pg-dim,#report-progress .pg-basis{color:var(--text-dim)}
#report-progress .pg-missing{color:var(--danger)}
#report-progress .pg-group{margin:16px 0 0}
#report-progress .pg-unit{font-weight:400;color:var(--text-dim);margin-left:4px}
#report-progress .pg-tiles{display:grid;grid-template-columns:repeat(auto-fit,minmax(130px,1fr));gap:1px;margin:0;background:var(--border);border:1px solid var(--border);border-radius:8px;overflow:hidden}
#report-progress .pg-tile{background:var(--surface);padding:10px 12px;margin:0}
#report-progress .pg-tile dt{font-size:11px;letter-spacing:.04em;text-transform:uppercase;color:var(--text-dim)}
#report-progress .pg-tile dd{margin:0}
#report-progress .pg-n{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-variant-numeric:tabular-nums;font-size:18px;font-weight:700}
#report-progress .pg-sub{font-size:11.5px;color:var(--text-dim)}
#report-progress .pg-t-good .pg-n{color:var(--ri-good)}
#report-progress .pg-t-warn .pg-n{color:var(--warn)}
#report-progress .pg-t-bad .pg-n{color:var(--danger)}
#report-progress .pg-t-unknown .pg-n{color:var(--text-dim)}
#report-progress details{margin-top:8px}
#report-progress summary{cursor:pointer;color:var(--accent);font-size:13px;padding:6px 0;min-height:24px}
#report-progress summary:focus-visible,#report-progress a:focus-visible{outline:2px solid var(--accent);outline-offset:2px}
#report-progress a{color:var(--accent)}
#report-progress .pg-list{list-style:none;margin:4px 0 0;padding:0}
#report-progress .pg-list li{padding:7px 0;border-top:1px solid var(--border);font-size:13px}
#report-progress .pg-basis{display:block;font-size:12px;margin-top:2px}
#report-progress .pg-badge{display:inline-block;font-size:11px;font-weight:700;padding:1px 6px;border-radius:4px;border:1px solid currentColor}
#report-progress .pg-b-complete,#report-progress .pg-l-completed{color:var(--ri-good)}
#report-progress .pg-b-in-progress,#report-progress .pg-l-active{color:var(--warn)}
#report-progress .pg-b-blocked,#report-progress .pg-l-blocked{color:var(--danger)}
#report-progress .pg-b-not-started,#report-progress .pg-l-queued,#report-progress .pg-l-unknown{color:var(--text-dim)}
#report-progress .bd-grid{display:flex;flex-wrap:wrap;gap:6px;margin:12px 0 4px;padding:0;list-style:none}
#report-progress .bd-cell{position:relative;width:34px;height:34px;display:flex;align-items:center;justify-content:center;border-radius:6px;border:1px solid var(--border);font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:12px;font-weight:700;color:var(--text);background:var(--surface);text-decoration:none}
#report-progress .bd-cell:hover{border-color:var(--accent)}
#report-progress .bd-cell::after,#report-progress .bd-k::after{position:absolute;top:-6px;right:-5px;font-size:10px;line-height:1;padding:1px;border-radius:50%;background:var(--surface)}
#report-progress .bd-k{position:relative;display:inline-block;width:14px;height:14px;border-radius:3px;border:1px solid var(--border);margin:0 2px 0 8px;vertical-align:-2px;font-size:0}
#report-progress .bd-complete{background:var(--good-bg);border-color:var(--ri-good)}
#report-progress .bd-complete::after{content:'\2713';color:var(--ri-good)}
#report-progress .bd-in-progress{background:var(--warn-bg);border-color:var(--warn)}
#report-progress .bd-in-progress::after{content:'\25CF';color:var(--warn)}
#report-progress .bd-blocked{background:var(--danger-b,var(--surface-2));border-color:var(--danger)}
#report-progress .bd-blocked::after{content:'\2717';color:var(--danger)}
#report-progress .bd-not-started{border-style:dashed}
#report-progress .bd-cell.bd-selected{box-shadow:0 0 0 2px var(--accent)}
#report-progress .bd-key{margin:4px 0 0;font-size:12px;color:var(--text-dim)}
#report-progress .pg-defs dl{margin:6px 0 0;font-size:12.5px}
#report-progress .pg-defs dt{font-weight:600;margin-top:6px}
#report-progress .pg-defs dd{margin:0;color:var(--text-dim)}
</style>`
