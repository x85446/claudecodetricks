package conductor

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

type ScheduleOpportunity struct {
	At           *time.Time `json:"at,omitempty"`
	Horizon      time.Time  `json:"horizon"`
	Reason       string     `json:"reason"`
	AutomationID string     `json:"automation_id,omitempty"`
	SequenceID   string     `json:"sequence_id,omitempty"`
}

type scheduleInterval struct{ start, end time.Time }

// NextOpportunity previews schedule intersections; it never predicts quota or queue readiness.
func (s *Service) NextOpportunity(ctx context.Context, projectID string) (out ScheduleOpportunity, err error) {
	var c model.Config
	var r Runtime
	err = s.withLock(func() error {
		var e error
		c, e = s.config()
		if e != nil {
			return e
		}
		r, e = s.runtime()
		return e
	})
	if err != nil {
		return
	}
	now := s.now()
	out.Horizon = now.Add(14 * 24 * time.Hour)
	var p model.Project
	for _, x := range c.Projects {
		if x.ID == projectID {
			p = x
			break
		}
	}
	if p.ID == "" {
		return out, fmt.Errorf("project not found")
	}
	if !c.Enabled || c.Paused || !p.Enabled || p.Paused {
		out.Reason = "Conductor or project is disabled or paused; no automatic opportunity"
		return
	}
	windows := map[string]model.TimeWindow{}
	for _, w := range c.TimeWindows {
		windows[w.ID] = w
	}
	var unknown bool
	rules := append([]model.Automation(nil), c.Automations...)
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].Priority != rules[j].Priority {
			return rules[i].Priority < rules[j].Priority
		}
		return rules[i].ID < rules[j].ID
	})
	for _, a := range rules {
		if e := ctx.Err(); e != nil {
			return out, e
		}
		if !a.Enabled || a.ProjectID != p.ID {
			continue
		}
		ids := []string{}
		for _, en := range a.Entries {
			ids = append(ids, en.SequenceID)
		}
		if a.Mode == "overdrive" {
			ids = a.Participants
		}
		for _, id := range ids {
			var sub model.Subscription
			for _, candidate := range c.Subscriptions {
				for _, seq := range candidate.Sequences {
					if seq.ID == id {
						sub = candidate
					}
				}
			}
			if !sub.Enabled {
				continue
			}
			u := r.Usage[sub.ID]
			allowance := u.Weekly
			fresh := sub.FreshnessSeconds
			if fresh <= 0 {
				fresh = 300
			}
			if !u.Supported || u.Source == "" || u.SubscriptionID != sub.ID || u.AccountID != sub.AccountID || sub.AccountID == "" || u.PoolID != sub.PoolID || u.ObservedAt.IsZero() || now.Sub(u.ObservedAt) > time.Duration(fresh)*time.Second || u.ObservedAt.After(now.Add(time.Minute)) {
				allowance = nil
			}
			if allowance != nil && (!allowance.ResetTrusted || !allowance.ResetsAt.After(now)) {
				allowance = nil
			}
			intervals := []scheduleInterval{{now, out.Horizon}}
			for _, wid := range []string{p.WindowID, a.WindowID} {
				if wid == "" {
					continue
				}
				w, exists := windows[wid]
				if !exists || !w.Enabled {
					intervals = nil
					break
				}
				if w.StartMode == "reset_relative" && allowance == nil {
					unknown = true
					intervals = nil
					break
				}
				next, e := timeIntervals(w, allowance, now, out.Horizon)
				if e != nil {
					return out, e
				}
				intervals = intersectIntervals(intervals, next)
			}
			if a.OnlyOverdrive || a.Mode == "overdrive" {
				if !sub.OverdriveEnabled {
					continue
				}
				var odds []scheduleInterval
				for _, w := range sub.OverdriveWindows {
					if !w.Enabled || w.TargetUsedPercent <= 0 {
						continue
					}
					needsReset := w.StartMode == "reset_relative" || w.EndMode == "" || w.EndMode == "reset" || w.EndMode == "weekly_reset" || w.EndMode == "next_reset"
					if needsReset && allowance == nil {
						unknown = true
						continue
					}
					times, e := windowProbeTimes(w.Recurrence, w.StartMode, w.HoursBeforeReset, allowance, now, out.Horizon)
					if e != nil {
						return out, e
					}
					for _, at := range times {
						o, open, e := budget.OverdriveAt(w, allowance, at)
						if e != nil {
							return out, e
						}
						if !open {
							continue
						}
						retired := false
						for _, recorded := range r.Occurrences {
							if recorded.ID == o.ID && recorded.Completed {
								retired = true
							}
						}
						if retired {
							continue
						}
						if allowance != nil && allowance.UsedPercent >= w.TargetUsedPercent {
							unknown = true
							continue
						}
						odds = append(odds, scheduleInterval{o.Start, o.End})
					}
				}
				intervals = intersectIntervals(intervals, odds)
			}
			for _, i := range intervals {
				if !i.start.Before(out.Horizon) {
					continue
				}
				if out.At == nil || i.start.Before(*out.At) {
					at := i.start
					out.At = &at
					out.AutomationID = a.ID
					out.SequenceID = id
				}
			}
		}
	}
	if out.At != nil {
		out.Reason = "Schedule opens at " + out.At.Format(time.RFC3339) + "; queue, budget, availability and ownership require admission at that time"
	} else {
		out.Reason = "No enabled schedule intersection within the next 14 days"
		if unknown {
			out.Reason += "; fresh trusted reset or allowance evidence is required for unresolved account windows"
		}
	}
	return
}

func windowProbeTimes(r model.Recurrence, mode string, hours float64, a *model.Allowance, from, until time.Time) ([]time.Time, error) {
	times := []time.Time{from}
	if mode == "reset_relative" {
		if a != nil {
			times = append(times, a.ResetsAt.Add(-time.Duration(hours*float64(time.Hour))))
		}
		return times, nil
	}
	starts, e := budget.PreviewStarts(r, from, until)
	if e != nil {
		return nil, e
	}
	for _, t := range starts {
		if !t.Before(from) && t.Before(until) {
			times = append(times, t)
		}
	}
	return times, nil
}
func timeIntervals(w model.TimeWindow, a *model.Allowance, from, until time.Time) ([]scheduleInterval, error) {
	times, e := windowProbeTimes(w.Recurrence, w.StartMode, w.HoursBeforeReset, a, from, until)
	if e != nil {
		return nil, e
	}
	var out []scheduleInterval
	for _, at := range times {
		o, ok, e := budget.ResolveTimeWindow(w, a, at)
		if e != nil {
			return nil, e
		}
		if ok {
			out = append(out, scheduleInterval{o.Start, o.End})
		}
	}
	return out, nil
}
func intersectIntervals(a, b []scheduleInterval) []scheduleInterval {
	var out []scheduleInterval
	for _, x := range a {
		for _, y := range b {
			start, end := x.start, x.end
			if y.start.After(start) {
				start = y.start
			}
			if y.end.Before(end) {
				end = y.end
			}
			if start.Before(end) {
				out = append(out, scheduleInterval{start, end})
			}
		}
	}
	return out
}
