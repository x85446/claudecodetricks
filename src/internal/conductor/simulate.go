package conductor

import (
	"context"
	"errors"
	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"sort"
	"time"
)

type SampleAccount struct {
	Day          *float64   `json:"day,omitempty"`
	Band         model.Band `json:"band,omitempty"`
	Availability string     `json:"availability,omitempty"`
	WeeklyUsed   *float64   `json:"weekly_used,omitempty"`
	ShortUsed    *float64   `json:"short_used,omitempty"`
}

func preserveEnablement(c *model.Config, saved model.Config) {
	for i := range c.Subscriptions {
		found := false
		for _, old := range saved.Subscriptions {
			if old.ID != c.Subscriptions[i].ID {
				continue
			}
			found = true
			s := &c.Subscriptions[i]
			s.Enabled = old.Enabled
			s.OverdriveEnabled = old.OverdriveEnabled
			for j := range s.OverdriveWindows {
				exists := false
				for _, ow := range old.OverdriveWindows {
					if ow.ID == s.OverdriveWindows[j].ID {
						s.OverdriveWindows[j].Enabled = ow.Enabled
						exists = true
					}
				}
				if !exists {
					s.OverdriveWindows[j].Enabled = false
				}
			}
		}
		if !found {
			c.Subscriptions[i].Enabled = false
			c.Subscriptions[i].OverdriveEnabled = false
		}
	}
}
func applySamples(c model.Config, r *Runtime, now time.Time, samples map[string]SampleAccount) {
	for id, sample := range samples {
		for _, sub := range c.Subscriptions {
			if sub.ID != id {
				continue
			}
			u := r.Usage[id]
			if u.Weekly == nil || sample.Band != "" {
				reset := now.Add(3 * 24 * time.Hour)
				u = model.Usage{SubscriptionID: id, PoolID: sub.PoolID, AccountID: sub.AccountID, Source: "Sample", ObservedAt: now, Supported: true, Weekly: &model.Allowance{StartsAt: reset.Add(-7 * 24 * time.Hour), ResetsAt: reset, ResetTrusted: true, Epoch: "sample"}, Short: &model.Allowance{StartsAt: now.Add(-time.Hour), ResetsAt: now.Add(4 * time.Hour), ResetTrusted: true, Epoch: "sample-short"}}
			} else {
				copy := *u.Weekly
				u.Weekly = &copy
				if u.Short != nil {
					copy := *u.Short
					u.Short = &copy
				}
			}
			loc, locErr := time.LoadLocation(sub.TypicalReset.Timezone)
			if locErr != nil {
				loc = time.UTC
			}
			if sample.Band == "" {
				alignSampleWeekly(u.Weekly, now, loc)
			}
			u.SubscriptionID = id
			u.AccountID = sub.AccountID
			u.PoolID = sub.PoolID
			u.Supported = true
			u.ObservedAt = now
			u.Source = "Sample"
			if u.Short != nil && (now.Before(u.Short.StartsAt) || !now.Before(u.Short.ResetsAt)) {
				duration := u.Short.ResetsAt.Sub(u.Short.StartsAt)
				if duration <= 0 {
					duration = 5 * time.Hour
				}
				u.Short.StartsAt = now.Add(-duration / 2)
				u.Short.ResetsAt = u.Short.StartsAt.Add(duration)
			}
			if len(u.Models) > 0 {
				models := map[string]model.Allowance{}
				for name, pool := range u.Models {
					alignSampleWeekly(&pool, now, loc)
					models[name] = pool
				}
				u.Models = models
			}
			if sample.Band != "" {
				var p model.PowerBand
				for _, b := range c.PowerBands {
					if b.ID == sub.PowerBandID {
						p = b
					}
				}
				green, red := budget.Thresholds(p, 4)
				switch sample.Band {
				case model.Green:
					u.Weekly.UsedPercent = green / 2
				case model.Yellow:
					u.Weekly.UsedPercent = (green + red) / 2
				case model.Red:
					u.Weekly.UsedPercent = (red + 100) / 2
				}
				u.Short.UsedPercent = 0
			}
			if sample.WeeklyUsed != nil {
				u.Weekly.UsedPercent = *sample.WeeklyUsed
			}
			if sample.ShortUsed != nil && u.Short != nil {
				u.Short.UsedPercent = *sample.ShortUsed
			}
			if sample.Day != nil && *sample.Day >= 0 && *sample.Day <= 7 {
				u.Weekly.StartsAt = now.Add(-time.Duration(*sample.Day * 24 * float64(time.Hour)))
				u.Weekly.ResetsAt = u.Weekly.StartsAt.Add(7 * 24 * time.Hour)
				if *sample.Day == 7 {
					u.Weekly.StartsAt = u.Weekly.StartsAt.Add(time.Nanosecond)
					u.Weekly.ResetsAt = now.Add(time.Nanosecond)
				}
			}
			if u.Short != nil && sample.Availability == "ready" && !now.Before(u.Short.ResetsAt) {
				u.Short.StartsAt = now.Add(-time.Hour)
				u.Short.ResetsAt = now.Add(4 * time.Hour)
			}
			switch sample.Availability {
			case "unavailable", "blocked", "exhausted":
				u.HardBlocked = true
				u.Reason = "Sample account unavailable"
			case "stale":
				u.ObservedAt = now.Add(-24 * time.Hour)
			case "ready":
				u.HardBlocked = false
				u.ObservedAt = now
			}
			r.Usage[id] = u
			for _, other := range c.Subscriptions {
				if other.PoolID != "" && other.PoolID == sub.PoolID && other.AccountID == sub.AccountID {
					copy := u
					copy.SubscriptionID = other.ID
					r.Usage[other.ID] = copy
				}
			}
		}
	}
}

type BudgetPreviewRequest struct {
	SubscriptionID string           `json:"subscription_id"`
	SequenceID     string           `json:"sequence_id,omitempty"`
	Policy         *model.PowerBand `json:"policy,omitempty"`
	Now            *time.Time       `json:"now,omitempty"`
	Sample         *SampleAccount   `json:"sample,omitempty"`
	MaxBand        model.Band       `json:"max_band,omitempty"`
	HonorOverdrive bool             `json:"honor_overdrive"`
	OnlyOverdrive  bool             `json:"only_overdrive"`
}

type BudgetPreviewResult struct {
	model.BudgetDecision
	Display  *model.BudgetDecision `json:"display,omitempty"`
	Achieved bool                  `json:"overdrive_achieved"`
	Target   float64               `json:"display_target"`
}

func (s *Service) BudgetPreview(ctx context.Context, req BudgetPreviewRequest) (BudgetPreviewResult, error) {
	c, e := s.Config()
	if e != nil {
		return BudgetPreviewResult{}, e
	}
	r, e := s.Runtime()
	if e != nil {
		return BudgetPreviewResult{}, e
	}
	now := s.now()
	if req.Now != nil {
		now = *req.Now
	}
	if req.Sample != nil {
		applySamples(c, &r, now, map[string]SampleAccount{req.SubscriptionID: *req.Sample})
	}
	for _, sub := range c.Subscriptions {
		if sub.ID != req.SubscriptionID {
			continue
		}
		var p model.PowerBand
		for _, policy := range c.PowerBands {
			if policy.ID == sub.PowerBandID {
				p = policy
			}
		}
		if req.Policy != nil {
			p = *req.Policy
			if e = budget.ValidatePolicy(p); e != nil {
				return BudgetPreviewResult{}, e
			}
		}
		modelID := ""
		for _, q := range sub.Sequences {
			if q.ID == req.SequenceID {
				modelID = q.Model
			}
		}
		max := req.MaxBand
		if max == "" {
			max = model.Green
		}
		out := BudgetPreviewResult{BudgetDecision: budget.EvaluateForModel(p, sub, r.Usage[sub.ID], now, max, req.HonorOverdrive, req.OnlyOverdrive, r.Occurrences, modelID)}
		if req.Sample != nil {
			preview := sampleBudget(p, sub, r.Usage[sub.ID], now, max, req.HonorOverdrive, req.OnlyOverdrive, modelID)
			out.Display, out.Target, out.Achieved = preview.Display, preview.Target, preview.Achieved
		}
		return out, nil
	}
	return BudgetPreviewResult{}, errors.New("subscription not found")
}

type OverdriveOption struct {
	ID                string    `json:"id"`
	SubscriptionID    string    `json:"subscription_id"`
	SequenceID        string    `json:"sequence_id"`
	WindowID          string    `json:"window_id"`
	Label             string    `json:"label"`
	Start             time.Time `json:"start"`
	End               time.Time `json:"end"`
	SampleTime        time.Time `json:"sample_time"`
	Timezone          string    `json:"timezone"`
	TargetUsedPercent float64   `json:"target_used_percent"`
}

func (s *Service) OverdriveOptions(ctx context.Context, automationID string) ([]OverdriveOption, error) {
	c, e := s.Config()
	if e != nil {
		return nil, e
	}
	r, e := s.Runtime()
	if e != nil {
		return nil, e
	}
	var automation model.Automation
	for _, a := range append(c.Automations, model.EffectiveAutomations(c)...) {
		if a.ID == automationID {
			automation = a
		}
	}
	return s.overdriveOptions(c, r, automation)
}
func (s *Service) OverdriveOptionsFor(ctx context.Context, automation model.Automation) ([]OverdriveOption, error) {
	c, e := s.Config()
	if e != nil {
		return nil, e
	}
	r, e := s.Runtime()
	if e != nil {
		return nil, e
	}
	return s.overdriveOptions(c, r, automation)
}
func (s *Service) overdriveOptions(c model.Config, r Runtime, automation model.Automation) ([]OverdriveOption, error) {
	participants := map[string]bool{}
	for _, id := range automation.Participants {
		participants[id] = true
	}
	out := []OverdriveOption{}
	for _, sub := range c.Subscriptions {
		if !sub.Enabled || !sub.OverdriveEnabled {
			continue
		}
		u := r.Usage[sub.ID]
		for _, seq := range sub.Sequences {
			if !participants[seq.ID] {
				continue
			}
			number := 0
			for _, w := range sub.OverdriveWindows {
				if !w.Enabled {
					continue
				}
				number++
				var occ model.Occurrence
				found := false
				for h := 0; h <= 24*15; h++ {
					o, ok, err := budget.OverdriveAt(w, u.Weekly, s.now().Add(time.Duration(h)*time.Hour))
					if err == nil && ok {
						occ = o
						found = true
						break
					}
				}
				if !found {
					continue
				}
				out = append(out, OverdriveOption{ID: seq.ID + ":" + w.ID, SubscriptionID: sub.ID, SequenceID: seq.ID, WindowID: w.ID, Label: "Put in overdrive window (" + model.SequenceName(sub, seq) + ", " + fmtPID(number) + ")", Start: occ.Start, End: occ.End, SampleTime: occ.Start.Add(occ.End.Sub(occ.Start) / 2), Timezone: w.Recurrence.Timezone, TargetUsedPercent: w.TargetUsedPercent})
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].Label < out[j].Label })
	return out, nil
}

// OutsideOverdriveTime returns one common sample instant with no active participating window.
func (s *Service) OutsideOverdriveTime(ctx context.Context, automation model.Automation) (time.Time, bool, error) {
	c, e := s.Config()
	if e != nil {
		return time.Time{}, false, e
	}
	r, e := s.Runtime()
	if e != nil {
		return time.Time{}, false, e
	}
	ids := map[string]bool{}
	for _, id := range automation.Participants {
		ids[id] = true
	}
	for h := 0; h <= 24*35; h++ {
		now := s.now().Add(time.Duration(h) * time.Hour)
		inside := false
		for _, sub := range c.Subscriptions {
			if !sub.Enabled || !sub.OverdriveEnabled {
				continue
			}
			participates := false
			for _, q := range sub.Sequences {
				if ids[q.ID] {
					participates = true
				}
			}
			if !participates {
				continue
			}
			for _, w := range sub.OverdriveWindows {
				if !w.Enabled {
					continue
				}
				_, open, x := budget.OverdriveAt(w, r.Usage[sub.ID].Weekly, now)
				if x == nil && open {
					inside = true
					break
				}
			}
		}
		if !inside {
			return now, true, nil
		}
	}
	return time.Time{}, false, nil
}

// Sample intervals move by whole local weeks, preserving the reported reset weekday and clock.
func alignSampleWeekly(a *model.Allowance, now time.Time, loc *time.Location) {
	if a == nil || a.ResetsAt.IsZero() {
		return
	}
	if a.StartsAt.IsZero() {
		a.StartsAt = a.ResetsAt.In(loc).AddDate(0, 0, -7)
	}
	if !a.ResetsAt.After(a.StartsAt) {
		return
	}
	start, end := a.StartsAt.In(loc), a.ResetsAt.In(loc)
	weeks := int(now.Sub(start) / (7 * 24 * time.Hour))
	start = start.AddDate(0, 0, weeks*7)
	end = end.AddDate(0, 0, weeks*7)
	for now.Before(start) {
		start = start.AddDate(0, 0, -7)
		end = end.AddDate(0, 0, -7)
	}
	for !now.Before(end) {
		start = start.AddDate(0, 0, 7)
		end = end.AddDate(0, 0, 7)
	}
	a.StartsAt = start
	a.ResetsAt = end
	a.Epoch = "sample:" + start.UTC().Format(time.RFC3339)
}

// sampleBudget describes a hypothetical window without retiring its occurrence.
func sampleBudget(p model.PowerBand, sub model.Subscription, u model.Usage, now time.Time, max model.Band, honor, only bool, modelID string) BudgetPreviewResult {
	displaySub := sub
	displaySub.Enabled = true
	display := budget.EvaluateForModel(p, displaySub, u, now, max, honor, only, nil, modelID)
	out := BudgetPreviewResult{Display: &display}
	found := false
	if honor && sub.OverdriveEnabled {
		for _, w := range sub.OverdriveWindows {
			if _, active, err := budget.OverdriveAt(w, u.Weekly, now); err == nil && active {
				if !found || out.Target < w.TargetUsedPercent {
					out.Target = w.TargetUsedPercent
				}
				found = true
			}
		}
	}
	out.Achieved = found && u.Weekly != nil && u.Weekly.UsedPercent >= out.Target
	return out
}
