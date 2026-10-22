package conductor

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

// AutomationPreviewRequest carries one draft automation and the sample inputs its
// own rules read. Projects, plans, leases, executables, account bindings and the
// Conductor switch are not inputs: the preview describes a hypothetical eligible
// project, and production admission still enforces every live gate.
type AutomationPreviewRequest struct {
	Automation model.Automation  `json:"automation"`
	Window     *model.TimeWindow `json:"window,omitempty"`
	// Band is the sample effective band for every row's subscription.
	Band model.Band `json:"band,omitempty"`
	// Now is the sample instant; schedules and overdrive windows read it.
	Now *time.Time `json:"now,omitempty"`
	// OverdriveActive samples subscription overdrive for rules that honor it.
	OverdriveActive bool `json:"overdrive_active,omitempty"`
	// Samples set weekly usage per subscription in overdrive mode.
	Samples map[string]SampleAccount `json:"samples,omitempty"`
}

// PreviewRow is one sequence row's verdict for the sample.
type PreviewRow struct {
	Index        int                  `json:"index"`
	EntryID      string               `json:"entry_id,omitempty"`
	SequenceID   string               `json:"sequence_id"`
	Subscription string               `json:"subscription_id,omitempty"`
	Label        string               `json:"label"`
	MaxBand      model.Band           `json:"max_band"`
	Eligible     bool                 `json:"eligible"`
	Selected     bool                 `json:"selected"`
	Code         string               `json:"code"`
	Reason       string               `json:"reason"`
	SampleBudget *BudgetPreviewResult `json:"sample_budget,omitempty"`
}

// BandOutcome names the row that launches for one sample band, or why none does.
type BandOutcome struct {
	Band     model.Band `json:"band"`
	Selected int        `json:"selected"`
	Label    string     `json:"label,omitempty"`
	Reason   string     `json:"reason"`
}

// AutomationPreview is the single outcome of a draft automation for one sample.
type AutomationPreview struct {
	Band     model.Band    `json:"band,omitempty"`
	Maximum  model.Band    `json:"maximum,omitempty"`
	Rows     []PreviewRow  `json:"rows"`
	Selected *PreviewRow   `json:"selected,omitempty"`
	Outcome  string        `json:"outcome"`
	Reason   string        `json:"reason"`
	Bands    []BandOutcome `json:"bands,omitempty"`
	Now      time.Time     `json:"now"`
}

var previewBands = []model.Band{model.Green, model.Yellow, model.Red}

func bandRank(b model.Band) int {
	for i, x := range previewBands {
		if x == b {
			return i
		}
	}
	return -1
}

// BandTitle is the display name of one band.
func BandTitle(b model.Band) string {
	if bandRank(b) < 0 {
		return string(b)
	}
	return strings.ToUpper(string(b[:1])) + string(b[1:])
}

// BandRange names everything a row's maximum band permits, as the row selector does.
func BandRange(max model.Band) string {
	switch max {
	case model.Green:
		return "Green only"
	case model.Yellow:
		return "Green + Yellow"
	case model.Red:
		return "Green + Yellow + Red"
	}
	return "Unknown band"
}

// PreviewAutomation evaluates a draft automation's rows for a hypothetical eligible project.
func (s *Service) PreviewAutomation(ctx context.Context, req AutomationPreviewRequest) (AutomationPreview, error) {
	c, e := s.Config()
	if e != nil {
		return AutomationPreview{}, e
	}
	now := s.now()
	if req.Now != nil {
		now = *req.Now
	}
	a := req.Automation
	if a.Mode == "overdrive" {
		r, e := s.Runtime()
		if e != nil {
			return AutomationPreview{}, e
		}
		return previewOverdrive(c, r, a, now, req.Samples), nil
	}
	band := req.Band
	if band == "" {
		band = model.Green
	}
	if bandRank(band) < 0 {
		return AutomationPreview{}, fmt.Errorf("sample band must be green, yellow or red")
	}
	window, e := previewWindow(c, a, req.Window)
	if e != nil {
		return AutomationPreview{}, e
	}
	out := AutomationPreview{Band: band, Now: now}
	for _, en := range a.Entries {
		if bandRank(en.MaxBand) > bandRank(out.Maximum) {
			out.Maximum = en.MaxBand
		}
	}
	for _, b := range previewBands {
		rows := previewBandRows(c, a, window, b, now, req.OverdriveActive)
		o := BandOutcome{Band: b, Selected: -1}
		outcome, reason, selected := previewOutcome(rows, b)
		if selected != nil {
			o.Selected, o.Label = selected.Index, selected.Label
		}
		o.Reason = reason
		out.Bands = append(out.Bands, o)
		if b == band {
			out.Rows, out.Outcome, out.Reason = rows, outcome, reason
			if selected != nil {
				sel := *selected
				out.Selected = &sel
			}
		}
	}
	return out, nil
}

func previewWindow(c model.Config, a model.Automation, draft *model.TimeWindow) (*model.TimeWindow, error) {
	if a.Mode != "scheduled" && a.WindowID == "" {
		return nil, nil
	}
	if draft != nil && draft.ID == a.WindowID {
		w := *draft
		if e := budget.ValidateRecurrence(w.Recurrence); w.StartMode != "reset_relative" && e != nil {
			return nil, fmt.Errorf("time window: %w", e)
		}
		return &w, nil
	}
	for _, w := range c.TimeWindows {
		if w.ID == a.WindowID {
			return &w, nil
		}
	}
	return &model.TimeWindow{}, nil
}

// previewWeekly is the sample allowance interval: four days elapsed, three to its reset.
func previewWeekly(now time.Time) *model.Allowance {
	return &model.Allowance{StartsAt: now.Add(-4 * 24 * time.Hour), ResetsAt: now.Add(3 * 24 * time.Hour), ResetTrusted: true, Epoch: "sample"}
}

func previewBandRows(c model.Config, a model.Automation, w *model.TimeWindow, band model.Band, now time.Time, overdrive bool) []PreviewRow {
	rows := []PreviewRow{}
	chosen := -1
	for i, en := range a.Entries {
		row := PreviewRow{Index: i, EntryID: en.ID, SequenceID: en.SequenceID, MaxBand: en.MaxBand, Label: "Unavailable sequence", Eligible: true}
		deny := func(code, reason string) {
			if row.Eligible {
				row.Eligible, row.Code, row.Reason = false, code, reason
			}
		}
		found := false
		for _, sub := range c.Subscriptions {
			for _, q := range sub.Sequences {
				if q.ID == en.SequenceID {
					row.Label, row.Subscription, found = model.SequenceName(sub, q), sub.ID, true
				}
			}
		}
		if !found {
			deny("sequence_unavailable", "Launch sequence unavailable; choose a saved sequence")
		}
		if bandRank(en.MaxBand) < 0 {
			deny("invalid_band", "Choose a maximum band for this row")
		}
		if w != nil {
			if w.ID == "" {
				deny("automation_window", "Time window unavailable; choose a saved time window")
			} else if _, open, err := budget.ResolveTimeWindow(*w, previewWeekly(now), now); err != nil {
				deny("automation_window", "Time window: "+err.Error())
			} else if !open {
				deny("automation_window", "Outside the automation time window at the sample time")
			}
		}
		if a.OnlyOverdrive && !a.HonorOverdrive {
			deny("invalid_overdrive_mode", "Only during subscription overdrive requires Honor subscription overdrive")
		}
		exceeds := bandRank(en.MaxBand) >= 0 && bandRank(band) > bandRank(en.MaxBand)
		bypass := a.HonorOverdrive && overdrive && exceeds
		if a.OnlyOverdrive && !overdrive {
			deny("outside_overdrive", "Subscription overdrive is not active at the sample")
		}
		if exceeds && !bypass {
			deny("band", BandRange(en.MaxBand)+" does not permit "+BandTitle(band))
		}
		if row.Eligible {
			if bypass {
				row.Code, row.Reason = "overdrive", "Subscription overdrive permits "+BandTitle(band)+" beyond the row's band"
			} else {
				row.Code, row.Reason = "permitted", BandRange(en.MaxBand)+" permits "+BandTitle(band)
			}
			if chosen < 0 {
				chosen = i
				row.Selected = true
			} else {
				row.Code, row.Reason = "secondary", row.Reason+"; row "+fmt.Sprint(chosen+1)+" launches first"
			}
		}
		rows = append(rows, row)
	}
	return rows
}

func previewOutcome(rows []PreviewRow, band model.Band) (string, string, *PreviewRow) {
	for i := range rows {
		if rows[i].Selected {
			return "launch", "Row " + fmt.Sprint(rows[i].Index+1) + " · " + rows[i].Label + " launches: " + rows[i].Reason, &rows[i]
		}
	}
	if len(rows) == 0 {
		return "wait", "Waits: add at least one sequence and maximum band", nil
	}
	codes := map[string]bool{}
	max := model.Band("")
	for _, r := range rows {
		codes[r.Code] = true
		if bandRank(r.MaxBand) > bandRank(max) {
			max = r.MaxBand
		}
	}
	switch {
	case len(codes) == 1 && codes["band"]:
		return "wait", "Waits: no row permits " + BandTitle(band) + "; the highest row permits " + BandRange(max), nil
	case len(codes) == 1:
		return "wait", "Waits: " + rows[0].Reason, nil
	case band == "":
		return "wait", "Waits: no participating sequence is in an active overdrive window below its target", nil
	}
	return "wait", "Waits: no row permits " + BandTitle(band) + " at the sample time", nil
}

// previewOverdrive follows subscription overdrive for participants, treating each
// subscription as enabled, bound and ready. Saved overdrive windows and their
// switches are the rules; runtime usage only aligns the sample reset.
func previewOverdrive(c model.Config, r Runtime, a model.Automation, now time.Time, samples map[string]SampleAccount) AutomationPreview {
	out := AutomationPreview{Maximum: model.Red, Now: now, Rows: []PreviewRow{}}
	for i := range c.Subscriptions {
		sub := &c.Subscriptions[i]
		sub.Enabled = true
		if sub.PoolID == "" {
			sub.PoolID = "sample-pool-" + sub.ID
		}
		if sub.AccountID == "" {
			sub.AccountID = "sample-account-" + sub.ID
		}
	}
	usage := map[string]model.Usage{}
	for id, u := range r.Usage {
		usage[id] = u
	}
	filled := map[string]SampleAccount{}
	for id, x := range samples {
		filled[id] = x
	}
	zero := 0.0
	for _, id := range a.Participants {
		for _, sub := range c.Subscriptions {
			for _, q := range sub.Sequences {
				if _, ok := filled[sub.ID]; q.ID == id && !ok {
					filled[sub.ID] = SampleAccount{WeeklyUsed: &zero}
				}
			}
		}
	}
	for id, x := range filled {
		x.Availability, x.Band = "ready", ""
		filled[id] = x
	}
	sampled := Runtime{Usage: usage}
	applySamples(c, &sampled, now, filled)
	pols := map[string]model.PowerBand{}
	for _, p := range c.PowerBands {
		pols[p.ID] = p
	}
	type participant struct {
		sub model.Subscription
		seq model.Sequence
		u   model.Usage
	}
	list := []participant{}
	for _, id := range a.Participants {
		for _, sub := range c.Subscriptions {
			for _, q := range sub.Sequences {
				if q.ID != id {
					continue
				}
				u := sampled.Usage[sub.ID]
				if u.Weekly == nil {
					reset := now.Add(3 * 24 * time.Hour)
					u = model.Usage{Weekly: &model.Allowance{StartsAt: reset.Add(-7 * 24 * time.Hour), ResetsAt: reset, ResetTrusted: true, Epoch: "sample"}}
				}
				u.SubscriptionID, u.PoolID, u.AccountID = sub.ID, sub.PoolID, sub.AccountID
				u.Supported, u.ObservedAt, u.Source = true, now, "Sample"
				u.HardBlocked, u.Reason, u.Models = false, "", nil
				if u.Short == nil && !u.ShortUnsupported {
					u.Short = &model.Allowance{StartsAt: now.Add(-time.Hour), ResetsAt: now.Add(4 * time.Hour), ResetTrusted: true, Epoch: "sample-short"}
				}
				if u.Short != nil {
					short := *u.Short
					short.UsedPercent = 0
					if sample, ok := filled[sub.ID]; ok && sample.ShortUsed != nil {
						short.UsedPercent = *sample.ShortUsed
					}
					u.Short = &short
				}
				list = append(list, participant{sub, q, u})
			}
		}
	}
	sort.SliceStable(list, func(i, j int) bool {
		ri, rj := trustedReset(list[i].u), trustedReset(list[j].u)
		if !ri.Equal(rj) {
			return ri.Before(rj)
		}
		return list[i].sub.ID < list[j].sub.ID
	})
	chosen := -1
	for i, p := range list {
		pol := pols[p.sub.PowerBandID]
		bd := budget.EvaluateForModel(pol, p.sub, p.u, now, model.Red, true, true, nil, p.seq.Model)
		preview := sampleBudget(pol, p.sub, p.u, now, model.Red, true, true, p.seq.Model)
		row := PreviewRow{Index: i, SequenceID: p.seq.ID, Subscription: p.sub.ID, Label: model.SequenceName(p.sub, p.seq), MaxBand: model.Red, Eligible: bd.Allowed, Code: bd.Code, Reason: bd.Reason, SampleBudget: &preview}
		if row.Eligible {
			if chosen < 0 {
				chosen = i
				row.Selected = true
			} else {
				row.Code, row.Reason = "secondary", row.Reason+"; "+list[chosen].sub.Name+" resets sooner and launches first"
			}
		}
		out.Rows = append(out.Rows, row)
	}
	outcome, reason, selected := previewOutcome(out.Rows, "")
	if len(out.Rows) == 0 {
		reason = "Select at least one participating sequence"
	}
	out.Outcome, out.Reason = outcome, reason
	if selected != nil {
		sel := *selected
		out.Selected = &sel
	}
	return out
}
