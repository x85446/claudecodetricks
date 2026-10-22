package iterrun

import (
	"fmt"
	"html"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/runrecord"
)

// CoverageSection is the plan report's step-6 section: Tracking completeness — which telemetry sources were captured, missing or partial for this execution.
//
// State describes usage capture: complete when every observed lane's usage
// was read, partial when some was, unavailable when none was. Every count
// names its unit, and a percentage appears only over a known denominator.
type CoverageSection struct {
	State   string            `json:"state"`
	Summary string            `json:"summary"`
	History bool              `json:"history_loaded"`
	Window  *AccountingWindow `json:"window"`
	Record  CoverageRecord    `json:"record"`
	Counts  []CoverageCount   `json:"counts"`
	Lanes   []CoverageLane    `json:"lanes"`
	Sources []CoverageSource  `json:"sources"`
	// Breakdowns says which usage breakdowns the report can show.
	Breakdowns []CoverageBreakdown `json:"breakdowns"`
}

// CoverageRecord describes the run record behind the report.
type CoverageRecord struct {
	// Source is "record" for observed runs, "legacy" for the read-only
	// reconstruction, or "unreadable".
	Source     string   `json:"source"`
	Provenance []string `json:"provenance,omitempty"`
	Runs       int      `json:"runs"`
	Error      string   `json:"error,omitempty"`
	// Unknown lists fields no recorded event established.
	Unknown []string `json:"unknown,omitempty"`
}

// CoverageCount is one capture count: sessions, agents or requests.
type CoverageCount struct {
	Kind string `json:"kind"`
	Unit string `json:"unit"`
	// Observed is the known denominator: identities some source saw. Nil
	// when no source can say how many there were.
	Observed *int `json:"observed,omitempty"`
	Captured int  `json:"captured"`
	// Missing is Observed minus Captured; nil with an unknown denominator.
	Missing *int     `json:"missing,omitempty"`
	Percent *float64 `json:"percent,omitempty"`
	State   string   `json:"state"`
	Note    string   `json:"note,omitempty"`
}

// CoverageLane is one usage lane's capture state.
type CoverageLane struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	// Kind is coordinator, team (teamed plans), or agent (a flat plan's
	// subagent).
	Kind      string   `json:"kind"`
	State     string   `json:"state"` // captured | partial | missing
	Engines   []string `json:"engines,omitempty"`
	Providers []string `json:"providers,omitempty"`
	// Observed and Found count session transcripts: identities the hook
	// log or record saw and the ones whose usage was read.
	Observed int `json:"observed"`
	Found    int `json:"found"`
	// Requests is nil when the lane's usage is unknown.
	Requests *int   `json:"requests,omitempty"`
	Source   string `json:"source"`
	Note     string `json:"note,omitempty"`
}

// CoverageSource is one telemetry source's capture state.
type CoverageSource struct {
	Telemetry  string     `json:"telemetry"`
	Label      string     `json:"label"`
	Kind       string     `json:"kind"`
	Key        string     `json:"key,omitempty"`
	State      string     `json:"state"`
	From       *time.Time `json:"from,omitempty"`
	To         *time.Time `json:"to,omitempty"`
	Provenance string     `json:"provenance,omitempty"`
	Note       string     `json:"note,omitempty"`
}

// CoverageBreakdown says whether one usage breakdown is available.
type CoverageBreakdown struct {
	Name  string `json:"name"`
	State string `json:"state"` // available | partial | estimated | unavailable | not_applicable
	Note  string `json:"note"`
}

// telemetryLabels names each telemetry source for a reader.
var telemetryLabels = map[string]string{
	"hook":             "Tool hooks (hook event log)",
	"transcript":       "Session transcripts",
	"conductor_stream": "Conductor provider stream",
	"conductor_runs":   "Conductor run directories",
	"plan_file":        "Plan file",
	"usage":            "Per-request usage",
}

// capture is the usage-capture assessment the coverage and usage sections
// share, so the two never disagree about what was read.
type capture struct {
	history   bool
	tokens    *PlanTokens
	knownZero bool // every observed lane was read and none made a request
	lanes     []CoverageLane

	observedSessions, capturedSessions map[string]bool
	observedAgents, capturedAgents     map[string]bool
	sessionsKnown, agentsKnown         bool
	// coordinatorUnattributed is set when no coordinator session was
	// attributed to the plan at all: there was one, so the session count
	// has no known denominator.
	coordinatorUnattributed bool

	// transcriptSessions are sessions whose transcript was read; record
	// usage for those sessions is the same spend seen twice.
	transcriptSessions map[string]bool
	transcriptRequests int
	recordExact        UsageFigures
	recordByLane       map[string]*UsageFigures
	recordLaneOrder    []string
	recordOverlap      int
	recordOverlapIDs   []string

	requestState string
}

// assessCapture folds the record and the transcripts into one capture
// view. It reads nothing beyond the input.
func assessCapture(in ReportInput) capture {
	c := capture{
		history: in.HistoryLoaded, tokens: in.Tokens,
		observedSessions: map[string]bool{}, capturedSessions: map[string]bool{},
		observedAgents: map[string]bool{}, capturedAgents: map[string]bool{},
		transcriptSessions: map[string]bool{}, recordByLane: map[string]*UsageFigures{},
	}
	runs := liveRuns(in.Record)
	for _, r := range runs {
		for _, s := range r.Sessions {
			c.observedSessions[s.SessionID] = true
			c.sessionsKnown = true
		}
		for _, a := range r.Agents {
			c.observedAgents[a.AgentID] = true
			c.agentsKnown = true
		}
	}

	if pt := in.Tokens; pt != nil && in.HistoryLoaded {
		c.sessionsKnown, c.agentsKnown = true, true
		allFound := true
		for _, l := range pt.Lanes {
			lane := CoverageLane{Key: l.Key, Label: l.Label, Kind: laneKind(l.Key, in.Plan), Engines: l.Engines, Providers: l.Providers,
				Observed: l.Observed, Found: l.Found, Source: "transcript"}
			switch {
			case l.Missing && l.Observed == 0:
				lane.State = "missing"
				lane.Note = "the hook log attributes no session to this lane, so its transcripts cannot be located; usage unknown"
				allFound = false
				if l.Key == "" {
					c.coordinatorUnattributed = true
				}
			case l.Missing:
				lane.State = "missing"
				lane.Note = "no transcript found; this lane's usage is unknown"
				allFound = false
			case l.Found < l.Observed:
				lane.State = statePartial
				lane.Note = fmt.Sprintf("%d of %d session transcripts read; the rest of this lane's usage is unknown", l.Found, l.Observed)
				allFound = false
			default:
				lane.State = "captured"
			}
			if !l.Missing {
				n := l.Usage.Messages
				lane.Requests = &n
			}
			if l.Key == "" {
				for _, s := range pt.Sessions {
					c.observedSessions[s] = true
				}
				for _, s := range l.Sessions {
					c.capturedSessions[s] = true
					c.transcriptSessions[s] = true
				}
			} else {
				for _, a := range l.Agents {
					c.observedAgents[a] = true
				}
				for _, a := range l.AgentsFound {
					c.capturedAgents[a] = true
				}
				for _, s := range l.Sessions {
					c.transcriptSessions[s] = true
				}
			}
			c.lanes = append(c.lanes, lane)
		}
		c.transcriptRequests = pt.Total.Messages
		c.knownZero = allFound && pt.Total.Messages == 0 && len(pt.Lanes) > 0
	}

	// Record usage. Exact requests of a session whose transcript was read
	// are the same spend observed twice and are not added.
	for _, r := range runs {
		for _, u := range r.Usage.Requests {
			if u.SessionID != "" {
				c.observedSessions[u.SessionID] = true
				c.sessionsKnown = true
			}
			if u.AgentID != "" {
				c.observedAgents[u.AgentID] = true
			}
			if u.Scope != runrecord.ScopeExact {
				continue
			}
			if u.SessionID != "" && c.transcriptSessions[u.SessionID] {
				c.recordOverlap++
				c.recordOverlapIDs = append(c.recordOverlapIDs, u.RequestID)
				continue
			}
			c.recordExact.add(recordRequest(u.Usage))
			if u.SessionID != "" {
				c.capturedSessions[u.SessionID] = true
			}
			if u.AgentID != "" {
				c.capturedAgents[u.AgentID] = true
			}
			lane := recordLaneLabel(u)
			f := c.recordByLane[lane]
			if f == nil {
				f = &UsageFigures{}
				c.recordByLane[lane] = f
				c.recordLaneOrder = append(c.recordLaneOrder, lane)
			}
			f.add(recordRequest(u.Usage))
		}
	}
	for _, lane := range c.recordLaneOrder {
		f := c.recordByLane[lane]
		n := f.Requests
		c.lanes = append(c.lanes, CoverageLane{Key: "record:" + lane, Label: lane, Kind: "record", State: "captured",
			Observed: 1, Found: 1, Requests: &n, Source: "run record",
			Note: "provider usage the run record observed for this lane"})
	}

	// Record sessions with no usage from any source are uncaptured.
	captured := c.transcriptRequests + c.recordExact.Requests
	lanesIncomplete := false
	for _, l := range c.lanes {
		if l.State != "captured" {
			lanesIncomplete = true
		}
	}
	sessionsMissing := len(c.observedSessions) > len(c.capturedSessions)
	switch {
	case c.knownZero && !sessionsMissing:
		c.requestState = stateComplete
	case captured == 0:
		c.requestState = stateUnavailable
	case lanesIncomplete || sessionsMissing || !in.HistoryLoaded && c.transcriptRequests == 0 && len(c.observedSessions) > 0:
		c.requestState = statePartial
	default:
		c.requestState = stateComplete
	}
	return c
}

// recordLaneLabel names the lane a record usage observation belongs to.
func recordLaneLabel(u runrecord.UsageEntry) string {
	switch {
	case u.Lane != "":
		return u.Lane
	case u.Requirement > 0 && u.Attempt > 0:
		return fmt.Sprintf("step %d attempt %d", u.Requirement, u.Attempt)
	case u.Requirement > 0:
		return fmt.Sprintf("step %d", u.Requirement)
	}
	return "plan"
}

func buildCoverageSection(in ReportInput) CoverageSection {
	c := assessCapture(in)
	s := CoverageSection{History: in.HistoryLoaded, Window: accountingWindow(in), Lanes: c.lanes, State: c.requestState}
	if in.Tokens != nil && in.Tokens.From != nil {
		to := in.Tokens.To
		s.Window.From, s.Window.To, s.Window.Open = in.Tokens.From, &to, in.Tokens.ToOpen
	}

	// The record behind the report.
	runs := liveRuns(in.Record)
	s.Record = CoverageRecord{Source: in.Record.Source, Runs: len(runs)}
	if in.RecordErr != nil {
		s.Record.Source, s.Record.Error = "unreadable", in.RecordErr.Error()
	}
	prov, unknown := map[string]bool{}, map[string]bool{}
	for _, r := range runs {
		prov[r.Provenance] = true
		for _, u := range r.Unknown {
			unknown[u] = true
		}
	}
	s.Record.Provenance, s.Record.Unknown = sortedStrings(prov), sortedStrings(unknown)

	// Counts.
	sessions := CoverageCount{Kind: "sessions", Unit: "sessions", Captured: len(c.capturedSessions)}
	agents := CoverageCount{Kind: "agents", Unit: "agents", Captured: len(c.capturedAgents)}
	for _, x := range []struct {
		cnt      *CoverageCount
		observed map[string]bool
		known    bool
	}{{&sessions, c.observedSessions, c.sessionsKnown && !c.coordinatorUnattributed}, {&agents, c.observedAgents, c.agentsKnown}} {
		cnt := x.cnt
		if cnt.Kind == "sessions" && c.coordinatorUnattributed {
			cnt.State = statePartial
			if cnt.Captured == 0 {
				cnt.State = stateUnavailable
			}
			cnt.Note = "no coordinator session was attributed to this plan, so the number of sessions is unknown"
			continue
		}
		if !x.known {
			cnt.State = stateUnavailable
			cnt.Note = "no source has observed them yet"
			if !in.HistoryLoaded {
				cnt.Note = "observed through tool history, which has not loaded"
			}
			continue
		}
		n := len(x.observed)
		cnt.Observed = &n
		missing := n - cnt.Captured
		if missing < 0 {
			missing = 0
		}
		cnt.Missing = &missing
		if n > 0 {
			p := 100 * float64(cnt.Captured) / float64(n)
			cnt.Percent = &p
		}
		switch {
		case n == 0:
			cnt.State, cnt.Note = stateComplete, "none observed"
		case missing == 0:
			cnt.State = stateComplete
		case cnt.Captured == 0:
			cnt.State = stateUnavailable
		default:
			cnt.State = statePartial
		}
		if missing > 0 && !in.HistoryLoaded {
			cnt.Note = "transcripts are checked when tool history loads"
		}
	}
	if !(in.Plan.Teamed || in.Plan.HasTeams) && agents.Observed != nil && *agents.Observed == 0 {
		agents.Note = "flat plan with no subagents observed"
	}
	requests := CoverageCount{Kind: "requests", Unit: "requests", Captured: c.transcriptRequests + c.recordExact.Requests, State: c.requestState}
	if c.requestState == stateComplete {
		// Every observed lane was read, so the captured count is the
		// whole count.
		n := requests.Captured
		zero := 0
		requests.Observed, requests.Missing = &n, &zero
		if n > 0 {
			p := 100.0
			requests.Percent = &p
		}
		if c.knownZero {
			requests.Note = "every observed lane was read and made no request: a known zero"
		}
	} else {
		requests.Note = "the total is unknown while any lane or session is uncaptured, so no percentage is shown"
	}
	s.Counts = []CoverageCount{sessions, agents, requests}

	// Sources.
	if in.HistoryLoaded {
		src := CoverageSource{Telemetry: "transcript", Label: telemetryLabels["transcript"], Kind: "request", Provenance: "observed"}
		if s.Window != nil {
			src.From, src.To = s.Window.From, s.Window.To
		}
		observed, found := 0, 0
		engines := map[string]bool{}
		if in.Tokens != nil {
			for _, l := range in.Tokens.Lanes {
				observed += l.Observed
				found += l.Found
				for _, e := range l.Engines {
					engines[e] = true
				}
			}
		}
		switch {
		case observed == 0:
			src.State, src.Note = "missing", "the hook log attributes no session to this plan, so no transcript can be located"
		case found == 0:
			src.State, src.Note = "missing", fmt.Sprintf("0 of %d session transcripts found", observed)
		case found < observed:
			src.State, src.Note = statePartial, fmt.Sprintf("%d of %d session transcripts found", found, observed)
		default:
			src.State, src.Note = "captured", fmt.Sprintf("%d of %d session transcripts found", found, observed)
		}
		if len(engines) > 0 {
			src.Note += "; engines: " + strings.Join(sortedStrings(engines), ", ")
		}
		s.Sources = append(s.Sources, src)
	} else {
		s.Sources = append(s.Sources, CoverageSource{Telemetry: "transcript", Label: telemetryLabels["transcript"], Kind: "request",
			State: stateUnavailable, Note: "not loaded: transcripts are read with tool history"})
	}
	seen := map[string]int{}
	for _, r := range runs {
		for _, e := range r.Coverage {
			src := CoverageSource{Telemetry: e.Telemetry, Label: telemetryLabels[e.Telemetry], Kind: e.Kind, Key: e.Key,
				State: e.State, Provenance: e.Provenance, Note: e.Note}
			if src.Label == "" {
				src.Label = e.Telemetry
			}
			if !e.From.IsZero() {
				f := e.From
				src.From = &f
			}
			if !e.To.IsZero() {
				t := e.To
				src.To = &t
			}
			if e.Telemetry == "usage" && c.transcriptRequests > 0 {
				src.Note += "; per-request usage comes from session transcripts instead"
			}
			k := e.Telemetry + "\x00" + e.Kind + "\x00" + e.Key
			if i, ok := seen[k]; ok {
				s.Sources[i] = src
				continue
			}
			seen[k] = len(s.Sources)
			s.Sources = append(s.Sources, src)
		}
	}
	if c.recordExact.Requests > 0 || c.recordOverlap > 0 {
		note := fmt.Sprintf("%s observed by the run record", countText(c.recordExact.Requests+c.recordOverlap, "request", "requests"))
		if c.recordOverlap > 0 {
			note += fmt.Sprintf("; %d also read from a session transcript and counted once", c.recordOverlap)
		}
		s.Sources = append(s.Sources, CoverageSource{Telemetry: "conductor_stream", Label: telemetryLabels["conductor_stream"], Kind: "request",
			State: "captured", Provenance: "observed", Note: note})
	}
	if in.RecordErr != nil {
		s.Sources = append(s.Sources, CoverageSource{Telemetry: "record", Label: "Run record", Kind: "plan", State: stateUnavailable,
			Note: "the run record could not be read: " + in.RecordErr.Error()})
	}

	s.Breakdowns = coverageBreakdowns(in, c)
	s.Summary = coverageSummary(s, c)
	return s
}

// coverageBreakdowns says which breakdowns the usage section can show.
func coverageBreakdowns(in ReportInput, c capture) []CoverageBreakdown {
	pt := in.Tokens
	teamed := in.Plan.Teamed || in.Plan.HasTeams
	notLoaded := CoverageBreakdown{State: stateUnavailable, Note: "needs session transcripts, which load with tool history"}
	var out []CoverageBreakdown

	// Teams and agents.
	team := CoverageBreakdown{Name: "team"}
	agent := CoverageBreakdown{Name: "agent"}
	teamLanes, teamFound, agentLanes, agentFound := 0, 0, 0, 0
	for _, l := range c.lanes {
		switch l.Kind {
		case "team":
			teamLanes++
			if l.State != "missing" {
				teamFound++
			}
		case "agent":
			agentLanes++
			if l.State != "missing" {
				agentFound++
			}
		}
	}
	switch {
	case !teamed:
		team.State, team.Note = "not_applicable", "flat plan: no teams; subagents are listed as agents"
	case pt == nil || !in.HistoryLoaded:
		team = CoverageBreakdown{Name: "team", State: notLoaded.State, Note: notLoaded.Note}
	case teamLanes == 0:
		team.State, team.Note = stateUnavailable, "no team activity attributed to this plan"
	case teamFound < teamLanes:
		team.State, team.Note = statePartial, fmt.Sprintf("%d of %d team lanes have usage", teamFound, teamLanes)
	default:
		team.State, team.Note = "available", countText(teamLanes, "team lane", "team lanes")
	}
	switch {
	case teamed:
		agent.State, agent.Note = "not_applicable", "teamed plan: subagents are grouped by team"
	case pt == nil || !in.HistoryLoaded:
		agent = CoverageBreakdown{Name: "agent", State: notLoaded.State, Note: notLoaded.Note}
	case agentLanes == 0:
		agent.State, agent.Note = "available", "no subagents observed; all usage is the coordinator's"
	case agentFound < agentLanes:
		agent.State, agent.Note = statePartial, fmt.Sprintf("%d of %d subagent lanes have usage", agentFound, agentLanes)
	default:
		agent.State, agent.Note = "available", countText(agentLanes, "subagent lane", "subagent lanes")
	}
	out = append(out, team, agent)

	for _, x := range []struct {
		name, what, none string
		n                int
	}{
		{"tool", "tool attribution is an estimate: context growth after each call, split evenly between calls one request made",
			"no counted request made a tool call", lenTools(pt)},
		{"wake-up", "wake-up allocation is an estimate: everything a lane spent until the next wake-up",
			"no prompt or wake-up was recorded in the attribution window", lenGroups(pt)},
	} {
		b := CoverageBreakdown{Name: x.name}
		switch {
		case pt == nil || !in.HistoryLoaded:
			b.State, b.Note = notLoaded.State, notLoaded.Note
		case x.n == 0:
			b.State, b.Note = stateUnavailable, x.none
		default:
			b.State, b.Note = "estimated", x.what
		}
		out = append(out, b)
	}

	models, providers := map[string]bool{}, map[string]bool{}
	if pt != nil {
		for _, m := range pt.Models {
			models[m.Model] = true
			providers[m.Provider] = true
		}
	}
	for _, r := range liveRuns(in.Record) {
		for _, u := range r.Usage.Requests {
			models[u.Model] = true
			providers[u.Provider] = true
		}
	}
	for _, x := range []struct {
		name string
		set  map[string]bool
	}{{"model", models}, {"provider", providers}} {
		b := CoverageBreakdown{Name: x.name}
		if names := sortedStrings(x.set); len(names) > 0 {
			b.State, b.Note = "available", strings.Join(names, ", ")
			if c.requestState == statePartial {
				b.State = statePartial
				b.Note += "; uncaptured lanes may have used others"
			}
		} else {
			b.State, b.Note = stateUnavailable, "no counted request named a "+x.name
		}
		out = append(out, b)
	}
	return out
}

func lenTools(pt *PlanTokens) int {
	if pt == nil {
		return 0
	}
	return len(pt.Tools)
}

func lenGroups(pt *PlanTokens) int {
	if pt == nil {
		return 0
	}
	return len(pt.Groups)
}

// coverageSummary is the section's one-sentence answer.
func coverageSummary(s CoverageSection, c capture) string {
	var missing []string
	for _, l := range c.lanes {
		if l.State == "missing" {
			missing = append(missing, l.Label)
		}
	}
	req := s.Counts[2]
	switch s.State {
	case stateComplete:
		if c.knownZero {
			return "Every observed lane was read; this execution made 0 model requests."
		}
		return fmt.Sprintf("Usage was captured for every observed lane: %s.", countText(req.Captured, "request", "requests"))
	case statePartial:
		msg := fmt.Sprintf("Usage is partial: %s captured", countText(req.Captured, "request", "requests"))
		if len(missing) > 0 {
			msg += "; no usage was found for " + strings.Join(missing, ", ")
		}
		if !s.History && req.Captured > 0 {
			msg += "; session transcripts have not loaded"
		}
		return msg + ". Totals are lower bounds."
	}
	switch {
	case !s.History:
		return "Usage is unavailable until tool history loads; the run record holds no per-request usage."
	case len(c.lanes) == 0:
		return "Usage is unavailable: no session, agent or request was attributed to this execution."
	}
	return "Usage is unavailable: no transcript or recorded request was found for " + joinOr(missing, "any lane") + ". This is unknown, not zero."
}

func writeCoverageSection(b *strings.Builder, s CoverageSection) {
	b.WriteString(`<section class="report-section" id="report-coverage" aria-labelledby="report-coverage-h">`)
	b.WriteString(accountingCSS("report-coverage"))
	b.WriteString(`<h2 id="report-coverage-h">Tracking coverage ` + stateBadge(s.State) + `</h2>`)
	b.WriteString(`<p class="ra-sum">` + html.EscapeString(s.Summary) + `</p>`)
	rec := s.Record.Source
	switch rec {
	case "record":
		rec = "observed run record"
	case "legacy":
		rec = "read-only reconstruction from plan, hook and Conductor files (no observed record)"
	case "":
		rec = "none"
	}
	if s.Record.Error != "" {
		rec = "unreadable (" + s.Record.Error + ")"
	}
	history := "not loaded"
	if s.History {
		history = "loaded"
	}
	b.WriteString(`<p class="ra-note">Capture interval: ` + windowText(s.Window) + `. Run record: ` + html.EscapeString(rec))
	if len(s.Record.Provenance) > 0 {
		b.WriteString(` · provenance ` + html.EscapeString(strings.Join(s.Record.Provenance, ", ")))
	}
	b.WriteString(`. Tool history: ` + history + `.</p>`)

	b.WriteString(`<ul class="ra-figs" aria-label="Capture counts">`)
	for _, c := range s.Counts {
		var val string
		switch {
		case c.Observed != nil:
			val = fmt.Sprintf("%s of %s", groupDigits(int64(c.Captured)), groupDigits(int64(*c.Observed)))
			if c.Percent != nil {
				val += fmt.Sprintf(" (%.0f%%)", *c.Percent)
			}
		case c.Captured > 0:
			val = groupDigits(int64(c.Captured)) + " of unknown"
		default:
			val = "unknown"
		}
		fmt.Fprintf(b, `<li><b>%s</b><span>%s captured · %s</span>`, html.EscapeString(val), html.EscapeString(c.Unit), stateBadge(c.State))
		if c.Note != "" {
			b.WriteString(`<span style="display:block">` + html.EscapeString(c.Note) + `</span>`)
		}
		b.WriteString(`</li>`)
	}
	b.WriteString(`</ul>`)

	b.WriteString(`<h3>Lanes</h3>`)
	if len(s.Lanes) == 0 {
		msg := "No lane has been attributed to this execution."
		if !s.History {
			msg = "Lanes are read from tool history, which has not loaded."
		}
		b.WriteString(`<p class="ra-note">` + msg + `</p>`)
	} else {
		labels := []string{"Lane", "Kind", "Engine / provider", "Capture", "Transcripts read", "Requests"}
		tableOpen(b, "Usage capture per lane", labels...)
		for _, l := range s.Lanes {
			eng := joinOr(append(append([]string{}, l.Engines...), l.Providers...), "unknown")
			reqs := `<span class="ra-unknown">unknown</span>`
			if l.Requests != nil {
				reqs = groupDigits(int64(*l.Requests)) + " requests"
			}
			read := fmt.Sprintf("%d of %d", l.Found, l.Observed)
			if l.Source != "transcript" {
				read = html.EscapeString(l.Source)
			}
			state := stateBadge(l.State)
			if l.Note != "" {
				state += `<br><span class="ra-note">` + html.EscapeString(l.Note) + `</span>`
			}
			tableRow(b, labels, html.EscapeString(laneLabel(l.Label)), html.EscapeString(l.Kind), html.EscapeString(eng), state, num(read), num(reqs))
		}
		tableClose(b)
	}

	b.WriteString(`<h3>Telemetry sources</h3>`)
	labels := []string{"Source", "Covers", "State", "Interval", "Provenance", "Note"}
	tableOpen(b, "What each telemetry source captured", labels...)
	for _, src := range s.Sources {
		interval := `<span class="ra-unknown">not stated</span>`
		if src.From != nil || src.To != nil {
			var from, to time.Time
			if src.From != nil {
				from = *src.From
			}
			if src.To != nil {
				to = *src.To
			}
			interval = timeTag(from) + " to " + timeTag(to)
		}
		prov := src.Provenance
		if prov == "" {
			prov = "—"
		}
		tableRow(b, labels, html.EscapeString(src.Label), html.EscapeString(src.Kind), stateBadge(src.State), interval,
			html.EscapeString(prov), html.EscapeString(src.Note))
	}
	tableClose(b)

	b.WriteString(`<h3>Breakdowns</h3><ul class="ra-list">`)
	for _, br := range s.Breakdowns {
		fmt.Fprintf(b, `<li><b>%s</b> %s — %s</li>`, html.EscapeString(br.Name), stateBadge(br.State), html.EscapeString(br.Note))
	}
	b.WriteString(`</ul>`)

	if len(s.Record.Unknown) > 0 {
		b.WriteString(`<details><summary>Fields no recorded event established (` + fmt.Sprint(len(s.Record.Unknown)) + `)</summary><p class="ra-note">These are unknown, not zero or empty: `)
		b.WriteString(html.EscapeString(strings.Join(s.Record.Unknown, ", ")) + `.</p></details>`)
	}
	b.WriteString(`<details><summary>Definitions</summary><dl>`)
	for _, d := range [][2]string{
		{"Captured", "a source recorded this identity's usage for the capture interval."},
		{"Partial", "some of an identity's usage was recorded; the rest is unknown, so totals are lower bounds."},
		{"Missing", "a source saw the identity work on this plan, but no usage for it was found."},
		{"Unavailable", "no source can report this; it is unknown, not zero."},
		{"Known zero", "every observed lane was read and none made a request."},
		{"Percentages", "shown only over a known denominator: the identities some source observed."},
		{"Lane kinds", "coordinator; team for a teamed plan; agent for a flat plan's subagent; record for usage only the run record observed."},
	} {
		b.WriteString(`<dt>` + d[0] + `</dt><dd>` + html.EscapeString(d[1]) + `</dd>`)
	}
	b.WriteString(`</dl></details></section>`)
}
