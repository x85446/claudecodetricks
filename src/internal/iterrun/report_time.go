package iterrun

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// TimeSection is the plan report's step-10 section: Elapsed time partitioned by cause, without summing concurrent spans into wall time.
//
// Partition always sums to WallMS exactly: every instant of the execution
// window counts once, under the first cause that covers it. LaneTime is
// the separate per-lane view, which can exceed WallMS because lanes run
// in parallel, and is never added to it.
type TimeSection struct {
	Status     string     `json:"status"`
	StatusNote string     `json:"status_note"`
	Clock      *TimeClock `json:"clock,omitempty"`
	// WallMS is the elapsed wall-clock time, nil when no start is known.
	WallMS    *int64          `json:"wall_ms,omitempty"`
	Partition []TimeShare     `json:"partition"`
	LaneTime  []TimeLaneTotal `json:"lane_time"`
	// Intervals are the longest stretches without execution, at least
	// timeIntervalMin long, longest first; IntervalsTotal counts them all.
	Intervals      []TimeInterval  `json:"intervals"`
	IntervalsTotal int             `json:"intervals_total"`
	Sources        []string        `json:"sources"`
	Terms          []LifecycleTerm `json:"definitions"`
	Unknown        []string        `json:"unknown"`
}

// TimeClock is the execution window every figure in the section uses.
type TimeClock struct {
	Start      time.Time `json:"start"`
	StartBasis string    `json:"start_basis"`
	End        time.Time `json:"end"`
	EndBasis   string    `json:"end_basis"`
	// Open is true when the execution has not ended and End is the report
	// clock.
	Open  bool   `json:"open"`
	State string `json:"state"`
}

// TimeShare is one cause's part of the elapsed time.
type TimeShare struct {
	Cause string `json:"cause"`
	Label string `json:"label"`
	MS    int64  `json:"ms"`
	// Stretches counts the separate intervals the cause covers.
	Stretches int `json:"stretches"`
}

// TimeLaneTotal is lane-time of one kind: the sum over lanes, which
// parallel lanes can push past the elapsed time.
type TimeLaneTotal struct {
	Kind  string `json:"kind"`
	Label string `json:"label"`
	// MS is nil when nothing of this kind was measured; State says
	// whether that is a capture gap ("unknown") or a capture that saw
	// none ("none").
	MS    *int64 `json:"ms,omitempty"`
	State string `json:"state"`
	Lanes int    `json:"lanes"`
	Spans int    `json:"spans"`
	Note  string `json:"note,omitempty"`
}

// TimeInterval is one stretch without execution and what the record says
// about it.
type TimeInterval struct {
	Cause    string    `json:"cause"`
	Label    string    `json:"label"`
	Start    time.Time `json:"start"`
	End      time.Time `json:"end"`
	MS       int64     `json:"ms"`
	Evidence []string  `json:"evidence"`
	Owner    string    `json:"owner"`
	// Before and After are the observed work bracketing the interval.
	Before *TimeMark `json:"before,omitempty"`
	After  *TimeMark `json:"after,omitempty"`
}

// TimeMark is one observed piece of work at an instant.
type TimeMark struct {
	What string    `json:"what"`
	At   time.Time `json:"at"`
}

// Causes in precedence order: where causes overlap, the instant counts
// under the first.
var timeCauses = []struct{ cause, label string }{
	{"execution", "Execution"},
	{"blocked", "Blocked on operator"},
	{"paused", "Paused"},
	{"schedule", "Schedule wait"},
	{"queued", "Queued for admission"},
	{"unobserved", "Unobserved"},
}

func timeCauseLabel(cause string) string {
	for _, c := range timeCauses {
		if c.cause == cause {
			return c.label
		}
	}
	return cause
}

func timeCauseRank(cause string) int {
	for i, c := range timeCauses {
		if c.cause == cause {
			return i
		}
	}
	return len(timeCauses)
}

const (
	// timeBridge is the longest pause between two observed calls in one
	// lane that still counts as execution: the model's turn between calls.
	timeBridge = SevereGap
	// timeIntervalMin is the shortest stretch without execution the
	// section lists.
	timeIntervalMin = 5 * time.Minute
	// timeIntervalCap bounds the listed stretches.
	timeIntervalCap = 10
)

// timeSpan is one cause interval from one source.
type timeSpan struct {
	start, end time.Time
	cause      string
	evidence   string
}

var dispatchTools = map[string]bool{"Agent": true, "Task": true, "spawn_agent": true}

var timeTerms = []LifecycleTerm{
	{"Elapsed", "Wall-clock time from the execution start to its end, or to the report time while it is still open. Counted once, however many lanes ran at the same time."},
	{"Execution", "Time covered by at least one observed tool call, build unit, Conductor attempt or recorded work interval in any lane, plus pauses under 5 minutes between two observed calls in the same lane (the model's turn). Parallel lanes count once."},
	{"Blocked on operator", "A blocker was detected and not yet cleared, or the plan's state was blocked, and no execution was observed."},
	{"Paused", "An operator paused the plan (status: paused) and no execution was observed."},
	{"Schedule wait", "The Conductor considered the plan and a schedule or launch-policy gate held it."},
	{"Queued for admission", "The plan waited to be admitted: the Conductor held it on an automation, budget or queue gate, or it sat queued or admitted."},
	{"Unobserved", "No record covers the interval. Unknown is not failure: the session may have been idle, waiting on a person, or running work the hooks do not see."},
	{"Precedence", "Where causes overlap, the time counts once, under the first that applies in this order: execution, blocked, paused, schedule wait, queued, unobserved."},
	{"Lane-time", "The sum over lanes. Lanes run in parallel, so lane-time can exceed elapsed time; it is never added to elapsed time."},
}

// timeWindow decides the execution window: the plan's Executing: value or
// the earliest observed execution evidence, through Finished:, the archive
// or completion event, or the report clock while the execution is open.
func timeWindow(in ReportInput) (TimeClock, bool) {
	var c TimeClock
	c.State = "unknown"
	for _, r := range in.Record.Runs {
		if r.State != "" && r.State != "unknown" {
			c.State = r.State
		}
	}
	if t, ok := in.Plan.ExecutingAt(); ok {
		c.Start, c.StartBasis = t.UTC(), "Executing: in the plan file"
	} else {
		var first time.Time
		basis := ""
		consider := func(t time.Time, why string) {
			if !t.IsZero() && (first.IsZero() || t.Before(first)) {
				first, basis = t, why
			}
		}
		for _, row := range in.Rows {
			for _, s := range row.spans {
				if s.tool != teamLogTool {
					consider(s.start, "first observed tool call")
				}
			}
		}
		for _, r := range in.Record.Runs {
			for _, l := range r.Lifecycle {
				if l.State == "executing" || l.State == "admitted" {
					consider(l.At, "first recorded "+l.State+" event")
				}
			}
			for _, o := range r.Ownership {
				if o.Event == "acquired" {
					consider(o.At, "first recorded ownership")
				}
			}
			for _, q := range r.Requirements {
				for _, a := range q.Attempts {
					if a.StartedAt != nil {
						consider(*a.StartedAt, "first recorded attempt")
					}
				}
			}
		}
		if first.IsZero() {
			for _, r := range in.Record.Runs {
				if r.OpenedAt != nil {
					consider(*r.OpenedAt, "run record opened")
				}
			}
		}
		if first.IsZero() {
			return c, false
		}
		c.Start, c.StartBasis = first.UTC(), basis
	}

	if t, ok := in.Plan.FinishedAt(); ok {
		c.End, c.EndBasis = t.UTC(), "Finished: in the plan file"
	}
	if c.End.IsZero() {
		for _, r := range in.Record.Runs {
			for _, l := range r.Lifecycle {
				if (l.State == "completed" || l.State == "archived") && (c.End.IsZero() || l.At.Before(c.End)) {
					c.End, c.EndBasis = l.At.UTC(), "recorded "+l.State+" event"
				}
			}
		}
	}
	if c.End.IsZero() && in.Plan.Archived {
		for _, r := range in.Record.Runs {
			if r.LastAt != nil && r.LastAt.After(c.End) {
				c.End, c.EndBasis = r.LastAt.UTC(), "last recorded event (the archive states no Finished:)"
			}
		}
	}
	if c.End.IsZero() {
		c.End, c.EndBasis, c.Open = in.Now.UTC(), "report time; the execution is still open", true
	}
	if c.End.Before(c.Start) {
		c.End = c.Start
	}
	return c, true
}

func buildTimeSection(in ReportInput) TimeSection {
	s := TimeSection{Partition: []TimeShare{}, LaneTime: []TimeLaneTotal{}, Intervals: []TimeInterval{}, Sources: []string{}, Terms: timeTerms, Unknown: []string{}}
	clock, ok := timeWindow(in)
	if !ok {
		s.Status = sectionUnavailable
		s.StatusNote = "No execution start is recorded: the plan file has no Executing: value and no run event or tool call exists."
		if in.RecordErr != nil {
			s.StatusNote += " The run record could not be read: " + in.RecordErr.Error()
		}
		s.Unknown = append(s.Unknown, "execution window")
		return s
	}
	s.Clock = &clock
	wall := clock.End.Sub(clock.Start)
	wallMS := lcMS(wall)
	s.WallMS = &wallMS

	var spans []timeSpan
	add := func(start, end time.Time, cause, evidence string) {
		if start.Before(clock.Start) {
			start = clock.Start
		}
		if end.After(clock.End) {
			end = clock.End
		}
		if end.After(start) {
			spans = append(spans, timeSpan{start, end, cause, evidence})
		}
	}
	sources := map[string]bool{}
	var execution []timeSpan // observed work only, for the before/after context

	// Lane activity from the timeline rows.
	lane := map[string]*TimeLaneTotal{}
	laneOrder := []string{"tool", "dispatch", "build", "between", "attempt", "recorded"}
	lane["tool"] = &TimeLaneTotal{Kind: "tool", Label: "Tool calls"}
	lane["dispatch"] = &TimeLaneTotal{Kind: "dispatch", Label: "Waiting on dispatched subagents"}
	lane["build"] = &TimeLaneTotal{Kind: "build", Label: "Build and test units"}
	lane["between"] = &TimeLaneTotal{Kind: "between", Label: "Between calls under 5 minutes (model turns, inferred)"}
	lane["attempt"] = &TimeLaneTotal{Kind: "attempt", Label: "Conductor attempts"}
	lane["recorded"] = &TimeLaneTotal{Kind: "recorded", Label: "Recorded work intervals"}
	addLane := func(kind string, d time.Duration, spansN int, laneSeen bool) {
		t := lane[kind]
		if t.MS == nil {
			z := int64(0)
			t.MS = &z
		}
		*t.MS += lcMS(d)
		t.Spans += spansN
		if laneSeen {
			t.Lanes++
		}
	}
	hookSpans := false
	for _, row := range in.Rows {
		name := row.label
		if name == "" {
			name = "coordinator"
		}
		byKind := map[string][]span{}
		var fine []span
		for _, sp := range row.spans {
			kind := "tool"
			switch {
			case sp.tool == teamLogTool:
				continue
			case strings.HasPrefix(sp.tool, "iterate-run:"):
				kind = "build"
			case dispatchTools[sp.tool]:
				kind = "dispatch"
			default:
				hookSpans = true
			}
			byKind[kind] = append(byKind[kind], sp)
			fine = append(fine, sp)
		}
		for kind, list := range byKind {
			sort.Slice(list, func(i, j int) bool { return list[i].start.Before(list[j].start) })
			addLane(kind, busyDuration(list), len(list), true)
		}
		if len(fine) == 0 {
			continue
		}
		sort.Slice(fine, func(i, j int) bool { return fine[i].start.Before(fine[j].start) })
		merged := mergeSpans(fine)
		var bridged time.Duration
		for i, m := range merged {
			ev := "observed activity in " + name
			add(m.start, m.end, "execution", ev)
			execution = append(execution, timeSpan{m.start, m.end, "execution", name})
			if i > 0 {
				gap := m.start.Sub(merged[i-1].end)
				if gap > 0 && gap < timeBridge {
					add(merged[i-1].end, m.start, "execution", "model turn between calls in "+name)
					bridged += gap
				}
			}
		}
		addLane("between", bridged, 0, bridged > 0)
		sources["timeline"] = true
	}

	// The run record: attempts, blockers, lifecycle, admission, intervals.
	for _, r := range in.Record.Runs {
		last := clock.End
		if r.LastAt != nil && r.LastAt.Before(last) {
			last = *r.LastAt
		}
		for _, q := range r.Requirements {
			for _, a := range q.Attempts {
				if a.StartedAt == nil {
					continue
				}
				end := last
				if a.FinishedAt != nil {
					end = *a.FinishedAt
				} else if r.Owner != nil && r.Owner.OwnerID == a.AssignmentID {
					end = clock.End
				}
				what := fmt.Sprintf("attempt %d of requirement %d", a.Number, q.Number)
				if q.Number == 0 {
					what = fmt.Sprintf("plan-level attempt %d", a.Number)
				}
				add(*a.StartedAt, end, "execution", what)
				execution = append(execution, timeSpan{*a.StartedAt, end, "execution", what})
				if a.DurationMS != nil {
					addLane("attempt", time.Duration(*a.DurationMS)*time.Millisecond, 1, true)
				} else if a.FinishedAt != nil {
					addLane("attempt", a.FinishedAt.Sub(*a.StartedAt), 1, true)
				}
				sources["attempts"] = true
			}
		}
		for _, b := range r.Blockers {
			if b.DetectedAt == nil {
				continue
			}
			end := clock.End
			if b.ClearedAt != nil {
				end = *b.ClearedAt
			}
			ev := "blocker detected"
			if b.Reason != "" {
				ev += ": " + b.Reason
			}
			add(*b.DetectedAt, end, "blocked", ev)
			sources["blockers"] = true
		}
		for i, l := range r.Lifecycle {
			end := clock.End
			if i+1 < len(r.Lifecycle) {
				end = r.Lifecycle[i+1].At
			}
			ev := "plan state " + l.State
			if l.Reason != "" {
				ev += " (" + l.Reason + ")"
			}
			switch l.State {
			case "blocked":
				add(l.At, end, "blocked", ev)
			case "paused":
				add(l.At, end, "paused", ev)
			case "queued", "admitted":
				add(l.At, end, "queued", ev)
			default:
				continue
			}
			sources["lifecycle"] = true
		}
		for i, ad := range r.Admissions {
			if ad.Decision != "gated" || ad.Gate == "ownership" {
				continue
			}
			end := clock.End
			if i+1 < len(r.Admissions) {
				end = r.Admissions[i+1].At
			}
			for _, o := range r.Ownership {
				if o.Event == "acquired" && o.At.After(ad.At) && o.At.Before(end) {
					end = o.At
				}
			}
			cause := "queued"
			if ad.Gate == "schedule" {
				cause = "schedule"
			}
			ev := "Conductor held it: " + ad.Gate
			if ad.Reason != "" {
				ev += " (" + ad.Reason + ")"
			}
			add(ad.At, end, cause, ev)
			sources["admission"] = true
		}
		for _, iv := range r.Intervals {
			ev := "recorded " + iv.Cause + " interval"
			if iv.Note != "" {
				ev += ": " + iv.Note
			}
			cause := ""
			switch iv.Cause {
			case "execution", "model", "tool", "build":
				cause = "execution"
				execution = append(execution, timeSpan{iv.Start, iv.End, "execution", ev})
				addLane("recorded", iv.End.Sub(iv.Start), 1, true)
			case "blocked-operator":
				cause = "blocked"
			case "schedule-pause":
				cause = "schedule"
			case "queue":
				cause = "queued"
			case "unobserved":
				cause = "unobserved"
			}
			if cause != "" {
				add(iv.Start, iv.End, cause, ev)
				sources["intervals"] = true
			}
		}
	}

	stretches := timeSweep(clock.Start, clock.End, spans)

	totals := map[string]*TimeShare{}
	for _, c := range timeCauses {
		totals[c.cause] = &TimeShare{Cause: c.cause, Label: c.label}
	}
	for _, st := range stretches {
		t := totals[st.cause]
		t.MS += lcMS(st.end.Sub(st.start))
		t.Stretches++
	}
	// Millisecond rounding per stretch can drift from the wall total;
	// unobserved absorbs the remainder so the partition sums exactly.
	var sum int64
	for _, c := range timeCauses {
		if c.cause != "unobserved" {
			sum += totals[c.cause].MS
		}
	}
	totals["unobserved"].MS = max(wallMS-sum, 0)
	for _, c := range timeCauses {
		s.Partition = append(s.Partition, *totals[c.cause])
	}

	for _, kind := range laneOrder {
		t := *lane[kind]
		t.State = "measured"
		switch {
		case kind == "tool" && !hookSpans && !in.HistoryLoaded:
			t.MS, t.State, t.Note = nil, "unknown", "tool history is not loaded"
		case kind == "tool" && !hookSpans:
			t.MS, t.State, t.Note = nil, "unknown", "no tool calls were captured for this plan"
		case kind == "between" && !hookSpans:
			t.MS, t.State, t.Note = nil, "unknown", "inferred only from captured tool calls"
		case t.MS == nil && (kind == "dispatch" || kind == "build") && !in.HistoryLoaded && !hookSpans:
			t.State, t.Note = "unknown", "tool history is not loaded"
		case t.MS == nil:
			t.State = "none"
		}
		if kind == "between" && t.MS != nil {
			t.Note = "inferred from adjacent tool calls; no model timing is recorded"
		}
		s.LaneTime = append(s.LaneTime, t)
	}

	// The stretches without execution, with what the record says about
	// each and who held the plan.
	for _, st := range stretches {
		d := st.end.Sub(st.start)
		if st.cause == "execution" || d < timeIntervalMin {
			continue
		}
		iv := TimeInterval{Cause: st.cause, Label: timeCauseLabel(st.cause), Start: st.start, End: st.end, MS: lcMS(d), Evidence: st.evidence}
		if iv.Evidence == nil {
			iv.Evidence = []string{}
		}
		iv.Owner = timeOwnerAt(in.Record.Runs, st.start)
		if st.cause != "unobserved" {
			s.Intervals = append(s.Intervals, iv)
			continue
		}
		// An unexplained interval gets the observed work on either side.
		iv.Evidence = append(iv.Evidence, "No record covers this interval; it is unknown, not a failure.")
		var before, after *timeSpan
		for i := range execution {
			e := &execution[i]
			if !e.end.After(st.start) && (before == nil || e.end.After(before.end)) {
				before = e
			}
			if !e.start.Before(st.end) && (after == nil || e.start.Before(after.start)) {
				after = e
			}
		}
		if before != nil {
			iv.Before = &TimeMark{What: before.evidence + " ended", At: before.end}
		}
		if after != nil {
			iv.After = &TimeMark{What: after.evidence + " started", At: after.start}
		}
		s.Intervals = append(s.Intervals, iv)
	}
	s.IntervalsTotal = len(s.Intervals)
	sort.SliceStable(s.Intervals, func(i, j int) bool { return s.Intervals[i].MS > s.Intervals[j].MS })
	if len(s.Intervals) > timeIntervalCap {
		s.Intervals = s.Intervals[:timeIntervalCap]
	}

	for _, name := range []string{"timeline", "attempts", "blockers", "lifecycle", "admission", "intervals"} {
		if sources[name] {
			s.Sources = append(s.Sources, name)
		}
	}
	var gaps []string
	if !in.HistoryLoaded {
		gaps = append(gaps, "tool history is not loaded, so tool calls are not yet counted as execution")
		s.Unknown = append(s.Unknown, "tool calls")
	}
	if in.RecordErr != nil {
		gaps = append(gaps, "the run record could not be read ("+in.RecordErr.Error()+")")
		s.Unknown = append(s.Unknown, "run record")
	} else if in.Record.Source == "legacy" {
		gaps = append(gaps, "this execution has no run record; causes come from the plan file, hook log and Conductor runs, and state changes without a stated time are placed at the file's modification time")
	}
	if !sources["admission"] && !sources["blockers"] && !sources["lifecycle"] {
		s.Unknown = append(s.Unknown, "waiting causes")
	}
	if len(gaps) > 0 {
		s.Status = sectionPartial
		s.StatusNote = lcSentence(strings.Join(gaps, "; "))
	} else {
		s.Status = sectionAvailable
		s.StatusNote = "Every instant of the execution window is counted once, under one cause."
	}
	return s
}

// lcSentence capitalizes a clause and ends it with a period.
func lcSentence(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:] + "."
}

// timeStretch is a maximal run of the window under one cause.
type timeStretch struct {
	start, end time.Time
	cause      string
	evidence   []string
}

// timeSweep partitions [start, end) so every instant counts once, under
// the highest-precedence cause covering it, or unobserved. It walks the
// span boundaries once, keeping the spans active at each point.
func timeSweep(start, end time.Time, spans []timeSpan) []timeStretch {
	type edge struct {
		at    time.Time
		open  bool
		index int
	}
	edges := make([]edge, 0, 2*len(spans))
	for i, sp := range spans {
		edges = append(edges, edge{sp.start, true, i}, edge{sp.end, false, i})
	}
	sort.Slice(edges, func(i, j int) bool {
		if !edges[i].at.Equal(edges[j].at) {
			return edges[i].at.Before(edges[j].at)
		}
		return !edges[i].open && edges[j].open
	})
	active := make([][]int, len(timeCauses))
	var out []timeStretch
	emit := func(a, z time.Time) {
		if !z.After(a) {
			return
		}
		cause, evidence := "unobserved", ""
		for rank, list := range active {
			if len(list) > 0 {
				cause, evidence = timeCauses[rank].cause, spans[list[0]].evidence
				break
			}
		}
		if n := len(out); n > 0 && out[n-1].cause == cause && out[n-1].end.Equal(a) {
			st := &out[n-1]
			st.end = z
			if evidence != "" && len(st.evidence) < 4 && !containsString(st.evidence, evidence) {
				st.evidence = append(st.evidence, evidence)
			}
			return
		}
		st := timeStretch{start: a, end: z, cause: cause}
		if evidence != "" {
			st.evidence = []string{evidence}
		}
		out = append(out, st)
	}
	cursor := start
	for _, e := range edges {
		if e.at.After(cursor) {
			emit(cursor, e.at)
			cursor = e.at
		}
		rank := timeCauseRank(spans[e.index].cause)
		if e.open {
			active[rank] = append(active[rank], e.index)
			continue
		}
		list := active[rank]
		for k, idx := range list {
			if idx == e.index {
				active[rank] = append(list[:k:k], list[k+1:]...)
				break
			}
		}
	}
	emit(cursor, end)
	return out
}

// timeOwnerAt says who held the plan at an instant, from recorded
// ownership only.
func timeOwnerAt(runs []runrecord.Run, at time.Time) string {
	var current *runrecord.OwnershipEntry
	seen := false
	for _, r := range runs {
		for i := range r.Ownership {
			o := &r.Ownership[i]
			if o.At.After(at) {
				break
			}
			seen = true
			switch o.Event {
			case "acquired":
				current = o
			case "released":
				if current != nil && current.OwnerID == o.OwnerID {
					current = nil
				}
			}
		}
	}
	switch {
	case !seen:
		return "ownership not recorded by then"
	case current == nil:
		return "no owner held the plan"
	}
	return ownerName(current.OwnerKind, current.OwnerID) + " held the plan"
}

// ownerName renders an owner for reading.
func ownerName(kind, id string) string {
	switch kind {
	case "cli":
		return "CLI session " + id
	case "conductor":
		return "Conductor " + id
	}
	return kind + " " + id
}

var timeLaneUnit = map[string]string{"tool": "call", "dispatch": "dispatch", "build": "unit", "attempt": "attempt", "recorded": "interval"}

var timeCauseColor = map[string]string{
	"execution": "var(--busy)", "blocked": "var(--warn)", "paused": "var(--accent)",
	"schedule": "var(--queued)", "queued": "var(--text-faint)",
	"unobserved": "repeating-linear-gradient(135deg,var(--surface-2),var(--surface-2) 4px,var(--border) 4px,var(--border) 8px)",
}

const timeCSS = `#report-time .rt-clock{font-size:14px;margin:0 0 10px}
#report-time .rt-clock strong{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-size:18px;color:var(--text)}
#report-time .rt-bar{display:flex;height:14px;border-radius:4px;overflow:hidden;border:1px solid var(--border);margin:0 0 10px;background:var(--surface-2)}
#report-time .rt-bar span{display:block;height:100%}
#report-time .rt-sw{display:inline-block;width:10px;height:10px;border-radius:2px;margin-right:6px;vertical-align:-1px;border:1px solid var(--border)}
#report-time tfoot td,#report-time tfoot th{font-weight:600}
#report-time .rt-iv{padding:8px 10px;background:var(--surface);border:1px solid var(--border);border-radius:8px;list-style:none;margin:0 0 8px}
#report-time ol.rt-ivs{padding:0}
#report-time .rt-iv-head{display:flex;flex-wrap:wrap;gap:4px 10px;align-items:baseline}
#report-time .rt-iv-dur{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace;font-weight:700}
#report-time .rt-iv ul{margin:4px 0 0;padding-left:18px;color:var(--text-dim);font-size:12.5px}
#report-time .rt-iv li{margin:0 0 2px}
`

func writeTimeSection(b *strings.Builder, s TimeSection) {
	b.WriteString(`<section class="report-section" id="report-time" aria-labelledby="report-time-h"><style>`)
	b.WriteString(lcSectionCSS("report-time", "rt") + timeCSS)
	b.WriteString(`</style><h2 id="report-time-h">Time</h2>`)
	status := s.Status
	if status == "" {
		status = sectionUnavailable
	}
	writeLcStatus(b, "rt", status, s.StatusNote)
	if s.Clock == nil || s.WallMS == nil {
		writeLcTerms(b, "rt", s.Terms)
		b.WriteString(`</section>`)
		return
	}
	c := s.Clock
	wall := *s.WallMS
	fmt.Fprintf(b, `<p class="rt-clock">Elapsed <strong>%s</strong> <span class="rt-dim">from %s (%s) to %s (%s)</span></p>`,
		lcDur(wall), lcTime(c.Start), lcEsc(c.StartBasis), lcTime(c.End), lcEsc(c.EndBasis))

	var aria []string
	for _, p := range s.Partition {
		if p.MS > 0 {
			aria = append(aria, fmt.Sprintf("%s %s (%s)", p.Label, lcDur(p.MS), lcPercent(p.MS, wall)))
		}
	}
	fmt.Fprintf(b, `<div class="rt-bar" role="img" aria-label="Elapsed time by cause: %s">`, lcEsc(strings.Join(aria, ", ")))
	for _, p := range s.Partition {
		if p.MS > 0 && wall > 0 {
			fmt.Fprintf(b, `<span style="width:%.3f%%;background:%s"></span>`, 100*float64(p.MS)/float64(wall), timeCauseColor[p.Cause])
		}
	}
	b.WriteString(`</div>`)
	b.WriteString(`<table class="rt-partition"><caption class="vh">Elapsed time by cause; the rows sum to the elapsed time</caption><thead><tr><th scope="col">Cause</th><th scope="col" class="num">Time</th><th scope="col" class="num">Share</th></tr></thead><tbody>`)
	for _, p := range s.Partition {
		stretches := ""
		if p.Stretches > 0 && p.Cause != "unobserved" {
			stretches = `<span class="rt-note">` + lcCount(p.Stretches, "interval") + `</span>`
		}
		value, share := lcDur(p.MS), lcPercent(p.MS, wall)
		if p.MS == 0 && p.Cause != "unobserved" {
			value, share = "none recorded", "—"
		}
		fmt.Fprintf(b, `<tr data-cause="%s"><th scope="row"><span class="rt-sw" style="background:%s" aria-hidden="true"></span>%s%s</th><td class="num">%s</td><td class="num">%s</td></tr>`,
			p.Cause, timeCauseColor[p.Cause], lcEsc(p.Label), stretches, value, share)
	}
	fmt.Fprintf(b, `</tbody><tfoot><tr><th scope="row">Elapsed</th><td class="num">%s</td><td class="num">100%%</td></tr></tfoot></table>`, lcDur(wall))

	b.WriteString(`<h3>Lane-time, not part of elapsed</h3><table class="rt-lanes"><caption class="vh">Lane-time by kind; lanes overlap, so these figures are not added to elapsed time</caption><thead><tr><th scope="col">Kind</th><th scope="col" class="num">Lane-time</th></tr></thead><tbody>`)
	for _, t := range s.LaneTime {
		value := map[string]string{"unknown": "unknown", "none": "none observed"}[t.State]
		if t.MS != nil {
			value = lcDur(*t.MS)
		}
		var notes []string
		if t.Lanes > 0 && t.Kind != "attempt" && t.Kind != "recorded" {
			notes = append(notes, lcCount(t.Lanes, "lane"))
		}
		if t.Spans > 0 {
			notes = append(notes, lcCount(t.Spans, timeLaneUnit[t.Kind]))
		}
		if t.Note != "" {
			notes = append(notes, t.Note)
		}
		note := ""
		if len(notes) > 0 {
			note = `<span class="rt-note">` + lcEsc(strings.Join(notes, "; ")) + `</span>`
		}
		fmt.Fprintf(b, `<tr data-kind="%s"><th scope="row">%s%s</th><td class="num">%s</td></tr>`, t.Kind, lcEsc(t.Label), note, value)
	}
	b.WriteString(`</tbody></table>`)

	fmt.Fprintf(b, `<h3>Intervals without execution, %d minutes or longer</h3>`, int(timeIntervalMin/time.Minute))
	if len(s.Intervals) == 0 {
		b.WriteString(`<p class="rt-dim">None: every stretch of 5 minutes or longer had observed execution.</p>`)
	} else {
		if s.IntervalsTotal > len(s.Intervals) {
			fmt.Fprintf(b, `<p class="rt-dim">The %d longest of %s.</p>`, len(s.Intervals), lcCount(s.IntervalsTotal, "interval"))
		}
		b.WriteString(`<ol class="rt-ivs">`)
		for _, iv := range s.Intervals {
			fmt.Fprintf(b, `<li class="rt-iv" data-cause="%s"><div class="rt-iv-head"><span class="rt-sw" style="background:%s" aria-hidden="true"></span><strong>%s</strong><span class="rt-iv-dur">%s</span><span class="rt-dim">%s to %s</span></div><ul>`,
				iv.Cause, timeCauseColor[iv.Cause], lcEsc(iv.Label), lcDur(iv.MS), lcTime(iv.Start), lcTime(iv.End))
			for _, e := range iv.Evidence {
				fmt.Fprintf(b, `<li>%s</li>`, lcEsc(e))
			}
			fmt.Fprintf(b, `<li>Ownership: %s.</li>`, lcEsc(iv.Owner))
			if iv.Before != nil {
				fmt.Fprintf(b, `<li>Before: %s %s.</li>`, lcEsc(iv.Before.What), lcTime(iv.Before.At))
			}
			if iv.After != nil {
				fmt.Fprintf(b, `<li>After: %s %s.</li>`, lcEsc(iv.After.What), lcTime(iv.After.At))
			}
			b.WriteString(`</ul></li>`)
		}
		b.WriteString(`</ol>`)
	}
	writeLcTerms(b, "rt", s.Terms)
	b.WriteString(`</section>`)
}
