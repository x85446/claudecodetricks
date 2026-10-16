package iterrun

import (
	"context"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

type conductorUsageView struct {
	SubscriptionID string    `json:"subscription_id"`
	ObservedAt     time.Time `json:"observed_at"`
	Source         string    `json:"source"`
	AgeSeconds     int64     `json:"age_seconds"`
	Available      bool      `json:"available"`
}

type conductorVisibilityView struct {
	ProjectID         string               `json:"project_id"`
	ProjectPath       string               `json:"project_path"`
	State             string               `json:"state"`
	EffectiveSequence string               `json:"effective_sequence"`
	Reason            string               `json:"reason"`
	NextOpportunity   string               `json:"next_opportunity"`
	NextOpening       *time.Time           `json:"next_opening,omitempty"`
	PreviewHorizon    time.Time            `json:"preview_horizon"`
	Usage             []conductorUsageView `json:"usage"`
}

// conductorVisibility presents evaluator decisions and recorded assignment snapshots.
// Missing conditions remain explicit; the view does not estimate launch times.
func conductorVisibility(ctx context.Context, s *conductor.Service, path string) ([]conductorVisibilityView, error) {
	c, err := s.Config()
	if err != nil {
		return nil, err
	}
	d, err := s.Status(ctx)
	if err != nil {
		return nil, err
	}
	r, err := s.Runtime()
	if err != nil {
		return nil, err
	}
	out := []conductorVisibilityView{}
	for _, p := range c.Projects {
		if path != "" && path != p.Path {
			continue
		}
		v := conductorVisibilityView{ProjectID: p.ID, ProjectPath: p.Path, State: d.State, EffectiveSequence: "None selected", Reason: "No eligible automation", Usage: []conductorUsageView{}}
		ids := map[string]bool{}
		for _, automation := range c.Automations {
			if automation.ProjectID != p.ID {
				continue
			}
			sequences := map[string]bool{}
			for _, entry := range automation.Entries {
				sequences[entry.SequenceID] = true
			}
			for _, id := range automation.Participants {
				sequences[id] = true
			}
			for _, sub := range c.Subscriptions {
				for _, sequence := range sub.Sequences {
					if sequences[sequence.ID] {
						ids[sub.ID] = true
					}
				}
			}
		}
		var chosen *conductor.Candidate
		for i := range d.Candidates {
			candidate := &d.Candidates[i]
			if candidate.Project.ID != p.ID {
				continue
			}
			if candidate.Subscription.ID != "" {
				ids[candidate.Subscription.ID] = true
			}
			if chosen == nil || candidate.Selected {
				chosen = candidate
			}
		}
		if chosen != nil {
			v.Reason = chosen.Reason
			if chosen.Selected {
				v.EffectiveSequence = model.SequenceName(chosen.Subscription, chosen.Sequence)
			}
		}
		v.NextOpportunity = "Waiting: " + v.Reason
		if chosen != nil && chosen.Selected {
			v.NextOpportunity = "Eligible at the next conductor tick; admission is checked again"
		}
		opening, err := s.NextOpportunity(ctx, p.ID)
		if err != nil {
			return nil, err
		}
		v.NextOpportunity = opening.Reason
		v.NextOpening = opening.At
		v.PreviewHorizon = opening.Horizon
		for _, lease := range r.Leases {
			if lease.Assignment.ProjectID != p.ID || (lease.Status != "running" && lease.Status != "claimed") {
				continue
			}
			a := lease.Assignment
			v.State = lease.Status
			v.EffectiveSequence = model.SequenceName(a.Subscription, a.Sequence)
			v.Reason = "Assignment in progress: " + a.Plan
			v.NextOpportunity = "After the active checkpoint; admission is evaluated again"
			v.NextOpening = nil
			ids[a.Subscription.ID] = true
		}
		for _, sub := range c.Subscriptions {
			if !ids[sub.ID] {
				continue
			}
			u := r.Usage[sub.ID]
			age := int64(0)
			if !u.ObservedAt.IsZero() && d.At.After(u.ObservedAt) {
				age = int64(d.At.Sub(u.ObservedAt).Seconds())
			}
			v.Usage = append(v.Usage, conductorUsageView{SubscriptionID: sub.ID, ObservedAt: u.ObservedAt, Source: u.Source, AgeSeconds: age, Available: u.Supported && u.Source != "" && !u.ObservedAt.IsZero()})
		}
		out = append(out, v)
	}
	return out, nil
}
