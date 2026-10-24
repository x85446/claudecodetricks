package conductor

import (
	"context"
	"fmt"
	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"github.com/x85446/claudecodetricks/src/internal/conductor/provider"
	"github.com/x85446/claudecodetricks/src/internal/plangraph"
	"os/exec"
	"sort"
	"time"
)

type EvaluateRequest struct {
	ProjectID      string                   `json:"project_id,omitempty"`
	AutomationID   string                   `json:"automation_id,omitempty"`
	Samples        map[string]SampleAccount `json:"samples,omitempty"`
	Draft          *model.Config            `json:"draft,omitempty"`
	Now            *time.Time               `json:"now,omitempty"`
	Usage          map[string]model.Usage   `json:"usage,omitempty"`
	ProjectBusy    *bool                    `json:"project_busy,omitempty"`
	QueueAvailable *bool                    `json:"queue_available,omitempty"`
}
type Candidate struct {
	SampleBudget *BudgetPreviewResult `json:"sample_budget,omitempty"`
	Project      model.Project        `json:"project"`
	Plan         Plan                 `json:"plan"`
	Automation   model.Automation     `json:"automation"`
	Subscription model.Subscription   `json:"subscription"`
	Sequence     model.Sequence       `json:"sequence"`
	EntryID      string               `json:"entry_id,omitempty"`
	Eligible     bool                 `json:"eligible"`
	Selected     bool                 `json:"selected"`
	Code         string               `json:"code"`
	Reason       string               `json:"reason"`
	Budget       model.BudgetDecision `json:"budget"`
	Occurrence   *model.Occurrence    `json:"occurrence,omitempty"`
}
type Queue struct {
	RunningOwners map[string]string `json:"running_owners"`
	RunningPlans  []string          `json:"running_plans"`
	ProjectID     string            `json:"project_id"`
	Plans         []Plan            `json:"plans"`
	Depth         int               `json:"depth"`
	Ready         int               `json:"ready"`
	Waiting       int               `json:"waiting"`
	Running       int               `json:"running"`
	Error         string            `json:"error,omitempty"`
}
type Decision struct {
	Revision        int64              `json:"revision"`
	RuntimeRevision int64              `json:"runtime_revision"`
	At              time.Time          `json:"at"`
	State           string             `json:"state"`
	Queues          []Queue            `json:"queues"`
	Candidates      []Candidate        `json:"candidates"`
	Selected        *Candidate         `json:"selected,omitempty"`
	Occurrences     []model.Occurrence `json:"occurrences"`
}

func (s *Service) Status(ctx context.Context) (Decision, error) {
	return s.Evaluate(ctx, EvaluateRequest{})
}
func (s *Service) Evaluate(ctx context.Context, req EvaluateRequest) (d Decision, e error) {
	e = s.withLock(func() error {
		c, x := s.config()
		if x != nil {
			return x
		}
		r, x := s.runtime()
		if x != nil {
			return x
		}
		if req.Draft != nil {
			saved := c
			c = *req.Draft
			preserveEnablement(&c, saved)
			normalize(&c)
			if x = ValidateConfig(c); x != nil {
				return x
			}
		}
		now := s.now()
		if req.Now != nil {
			now = *req.Now
		}
		for id, u := range req.Usage {
			r.Usage[id] = u
			for _, sub := range c.Subscriptions {
				if sub.PoolID != "" && sub.PoolID == u.PoolID && sub.AccountID == u.AccountID {
					copy := u
					copy.SubscriptionID = sub.ID
					r.Usage[sub.ID] = copy
				}
			}
		}
		applySamples(c, &r, now, req.Samples)
		d, x = s.evaluate(ctx, c, r, now, req)
		return x
	})
	return
}
func mergeOccurrences(dst []model.Occurrence, src []model.Occurrence) []model.Occurrence {
	for _, o := range src {
		found := false
		for i := range dst {
			if dst[i].ID == o.ID {
				claim, closing := dst[i].ClaimedPlan, dst[i].ClosingAction
				dst[i] = o
				if dst[i].ClaimedPlan == "" {
					dst[i].ClaimedPlan = claim
				}
				if dst[i].ClosingAction == "" {
					dst[i].ClosingAction = closing
				}
				found = true
				break
			}
		}
		if !found {
			dst = append(dst, o)
		}
	}
	return dst
}
func (s *Service) evaluate(ctx context.Context, c model.Config, r Runtime, now time.Time, req EvaluateRequest) (Decision, error) {
	d := Decision{Revision: c.Revision, RuntimeRevision: r.Revision, At: now, State: "watching", Occurrences: r.Occurrences, Queues: []Queue{}, Candidates: []Candidate{}}
	if !c.Enabled || c.Paused {
		d.State = "stood_down"
	}
	subs := map[string]model.Subscription{}
	seqs := map[string]model.Sequence{}
	pols := map[string]model.PowerBand{}
	wins := map[string]model.TimeWindow{}
	for _, sub := range c.Subscriptions {
		for _, q := range sub.Sequences {
			subs[q.ID] = sub
			seqs[q.ID] = q
		}
	}
	for _, p := range c.PowerBands {
		pols[p.ID] = p
	}
	for _, w := range c.TimeWindows {
		wins[w.ID] = w
	}
	for _, p := range c.Projects {
		if req.ProjectID != "" && req.ProjectID != p.ID {
			continue
		}
		q := Queue{ProjectID: p.ID, Plans: []Plan{}, RunningOwners: map[string]string{}}
		plans := []Plan{}
		if s.Source != nil {
			var err error
			plans, err = s.Source.List(ctx, p)
			if err != nil {
				q.Error = err.Error()
			}
		}
		if p.IntakeEnabled {
			for _, item := range r.Intake {
				if item.ProjectID == p.ID && item.State == "pending" {
					plans = append(plans, Plan{Name: intakeName(item), Staged: true, State: "queued", Kind: "intake"})
				}
			}
		}
		if req.QueueAvailable != nil && *req.QueueAvailable {
			selectable := false
			for _, pl := range plans {
				parked := r.Progress[PlanID(p.Path, pl.Name)].Parked && pl.State != "unblocked"
				if !pl.Running && !pl.Complete && !pl.Release && !pl.Blocked && !pl.Waiting && !parked && (pl.Staged || pl.State == "executing" || pl.State == "unblocked") {
					selectable = true
				}
			}
			if !selectable {
				plans = append(plans, Plan{Name: "sample-plan", Staged: true, State: "queued"})
			}
		}
		sort.SliceStable(plans, func(i, j int) bool {
			if plans[i].Position > 0 && plans[j].Position > 0 && plans[i].Position != plans[j].Position {
				return plans[i].Position < plans[j].Position
			}
			rank := func(x Plan) int {
				if x.Kind == "intake" {
					return 3
				}
				if x.State == "unblocked" {
					return 0
				}
				if x.State == "executing" {
					return 1
				}
				return 2
			}
			a, b := rank(plans[i]), rank(plans[j])
			if a != b {
				return a < b
			}
			if plans[i].Depth != plans[j].Depth {
				return plans[i].Depth < plans[j].Depth
			}
			if mi, mj := plans[i].Milestone, plans[j].Milestone; mi != mj {
				if mi == "" || mj == "" {
					return mi != ""
				}
				return plangraph.MilestoneLess(mi, mj)
			}
			if !plans[i].Started.Equal(plans[j].Started) {
				return plans[i].Started.Before(plans[j].Started)
			}
			return plans[i].Name < plans[j].Name
		})
		busy := false
		for _, l := range r.Leases {
			if l.Assignment.ProjectPath == p.Path {
				busy = true
				q.Running++
				q.RunningPlans = append(q.RunningPlans, l.Assignment.Plan)
				q.RunningOwners[l.Assignment.Plan] = "conductor"
			}
		}
		for _, pl := range plans {
			if pl.Running {
				busy = true
			}
		}
		if req.ProjectBusy != nil {
			busy = *req.ProjectBusy
		}
		gate := ""
		if s.Source != nil {
			if err := s.Source.Gate(ctx, p, now); err != nil {
				gate = err.Error()
			}
		}
		for _, pl := range plans {
			pl.ProjectPath = p.Path
			pl.ID = PlanID(p.Path, pl.Name)
			if pl.Running {
				owned := false
				for _, l := range r.Leases {
					if l.Assignment.ProjectPath == p.Path && l.Assignment.Plan == pl.Name {
						owned = true
					}
				}
				if !owned {
					q.RunningPlans = append(q.RunningPlans, pl.Name)
					owner := pl.ExecutionOwner
					if owner == "" {
						owner = "cli"
					}
					q.RunningOwners[pl.Name] = owner
					q.Running++
				}
				continue
			}
			if pl.Complete || (!pl.Staged && pl.State != "executing" && pl.State != "unblocked") || pl.Release {
				continue
			}
			active := false
			for _, l := range r.Leases {
				if l.Assignment.ProjectPath == p.Path && l.Assignment.Plan == pl.Name {
					active = true
				}
			}
			if active {
				continue
			}
			if pl.Kind != "intake" {
				visible := pl
				if pg := r.Progress[pl.ID]; pg.Parked && pl.State != "unblocked" {
					visible.Blocked = true
					visible.State = "blocked"
					visible.Reason = pg.Reason
				}
				q.Plans = append(q.Plans, visible)
				q.Depth++
			}
			if req.QueueAvailable != nil && !*req.QueueAvailable {
				continue
			}
			ready := false
			rules := append([]model.Automation{}, model.EffectiveAutomations(c)...)
			sort.SliceStable(rules, func(i, j int) bool {
				if rules[i].Priority != rules[j].Priority {
					return rules[i].Priority < rules[j].Priority
				}
				return rules[i].ID < rules[j].ID
			})
			for _, a := range rules {
				if a.ProjectID != p.ID || (req.AutomationID != "" && a.ID != req.AutomationID && a.DefinitionID != req.AutomationID) {
					continue
				}
				entries := a.Entries
				if a.Mode == "overdrive" {
					entries = nil
					for _, id := range a.Participants {
						entries = append(entries, model.Entry{SequenceID: id, MaxBand: model.Red})
					}
					sort.SliceStable(entries, func(i, j int) bool {
						return subscriptionLess(subs[entries[i].SequenceID], subs[entries[j].SequenceID], r)
					})
				}
				rowChosen := false
				for _, en := range entries {
					sub, ok := subs[en.SequenceID]
					if !ok {
						continue
					}
					sq := seqs[en.SequenceID]
					u := r.Usage[sub.ID]
					bd := budget.EvaluateForModel(pols[sub.PowerBandID], sub, u, now, en.MaxBand, a.HonorOverdrive, a.OnlyOverdrive, r.Occurrences, sq.Model)
					d.Occurrences = mergeOccurrences(d.Occurrences, bd.Occurrences)
					x := Candidate{Project: p, Plan: pl, Automation: a, Subscription: sub, Sequence: sq, EntryID: en.ID, Budget: bd, Eligible: true, Code: "eligible", Reason: "Eligible"}
					if _, sampled := req.Samples[sub.ID]; sampled {
						preview := sampleBudget(pols[sub.PowerBandID], sub, u, now, en.MaxBand, a.HonorOverdrive, a.OnlyOverdrive, sq.Model)
						x.SampleBudget = &preview
					}
					deny := func(code, reason string) {
						if x.Eligible {
							x.Eligible = false
							x.Code = code
							x.Reason = reason
						}
					}
					if !c.Enabled {
						deny("disabled", "Conductor disabled")
					}
					if c.Paused {
						deny("paused", "Conductor paused")
					}
					if !p.Enabled || p.Paused {
						deny("project_disabled", "Project disabled or paused")
					}
					if !a.Enabled {
						deny("automation_disabled", "Automation disabled")
					}
					if pl.Blocked {
						deny("plan_blocked", pl.Reason)
					}
					if pl.Waiting {
						deny("plan_waiting", pl.WaitReason)
					}
					pg := r.Progress[pl.ID]
					if pg.Parked && pl.State != "unblocked" {
						deny("operator_parked", pg.Reason)
					}
					if busy {
						deny("project_busy", "Project already owns an assignment")
					}
					if gate != "" {
						deny("legacy_gate", gate)
					}
					if !sub.Enabled {
						deny("subscription_disabled", "Subscription disabled")
					}
					if _, err := exec.LookPath(sub.Executable); err != nil {
						deny("executable_unavailable", "Subscription executable is not installed")
					}
					if _, err := provider.Compile(sub, sq, model.Assignment{}); err != nil {
						deny("transport_unavailable", err.Error())
					}
					if sub.AccountID == "" || sub.PoolID == "" {
						deny("account_unbound", "Bind an authenticated account and allowance pool")
					}
					poolCount := 0
					poolLimit := sub.Concurrency
					for _, other := range c.Subscriptions {
						if other.PoolID == sub.PoolID && other.Concurrency < poolLimit {
							poolLimit = other.Concurrency
						}
					}
					for _, l := range r.Leases {
						if l.Assignment.Subscription.PoolID == sub.PoolID {
							poolCount++
						}
					}
					if poolCount >= poolLimit {
						deny("account_busy", "Shared allowance pool concurrency reached")
					}
					if !bd.Allowed {
						deny(bd.Code, bd.Reason)
					}
					if p.WindowID != "" {
						_, open, err := budget.ResolveTimeWindow(wins[p.WindowID], u.Weekly, now)
						if err != nil || !open {
							deny("project_window", "Outside project time window")
						}
					}
					if a.WindowID != "" {
						occ, open, err := budget.ResolveTimeWindow(wins[a.WindowID], u.Weekly, now)
						occ.AutomationID = a.ID
						occ.ID = a.ID + ":" + occ.ID
						for _, old := range r.Occurrences {
							if old.ID == occ.ID {
								occ = old
							}
						}
						if err != nil || !open {
							continuation := false
							if a.OnWindowEnd == "finish_plan" {
								for _, old := range r.Occurrences {
									if old.AutomationID == a.ID && old.ClaimedPlan == pl.ID && !old.Completed && !now.Before(old.End) && old.ClosingAction == "" {
										old.ClosingAction = "finish_plan"
										d.Occurrences = mergeOccurrences(d.Occurrences, []model.Occurrence{old})
									}
									if old.AutomationID == a.ID && old.ClaimedPlan == pl.ID && old.ClosingAction == "finish_plan" && !old.Completed {
										occ = old
										continuation = true
									}
								}
							}
							if !continuation {
								deny("automation_window", "Outside automation time window")
							}
						}
						if !occ.Start.IsZero() {
							x.Occurrence = &occ
							if a.RunLimit == "one_plan" && occ.ClaimedPlan != "" && occ.ClaimedPlan != pl.ID {
								deny("occurrence_claimed", "This occurrence already claimed another plan")
							}
							if occ.Completed {
								deny("occurrence_completed", "Occurrence completed")
							}
						}
					}
					if rowChosen {
						deny("secondary", "Earlier profile is eligible")
					}
					if x.Eligible {
						rowChosen = true
						ready = true
					}
					d.Candidates = append(d.Candidates, x)
				}
			}
			if ready && pl.Kind != "intake" {
				q.Ready++
			} else if pl.Kind != "intake" {
				q.Waiting++
			}
		}
		d.Queues = append(d.Queues, q)
	}
	eligible := []int{}
	for i, x := range d.Candidates {
		if x.Eligible {
			eligible = append(eligible, i)
		}
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		a, b := d.Candidates[eligible[i]], d.Candidates[eligible[j]]
		if a.Project.ID != b.Project.ID {
			aAfter, bAfter := a.Project.ID > r.LastProject, b.Project.ID > r.LastProject
			if aAfter != bAfter {
				return aAfter
			}
			return a.Project.ID < b.Project.ID
		}
		if a.Plan.ID != b.Plan.ID {
			return false
		}
		if a.Automation.Priority != b.Automation.Priority {
			return a.Automation.Priority < b.Automation.Priority
		}
		ra, rb := trustedReset(r.Usage[a.Subscription.ID]), trustedReset(r.Usage[b.Subscription.ID])
		if !ra.Equal(rb) {
			return ra.Before(rb)
		}
		return a.Automation.ID < b.Automation.ID
	})
	if len(eligible) > 0 {
		i := eligible[0]
		d.Candidates[i].Selected = true
		d.Selected = &d.Candidates[i]
	}
	if d.Selected == nil && c.Enabled && !c.Paused {
		d.State = watchState(r, c, d)
	}
	if len(r.Leases) > 0 {
		d.State = "working"
	}
	return d, nil
}
func trustedReset(u model.Usage) time.Time {
	if u.Weekly != nil && u.Weekly.ResetTrusted {
		return u.Weekly.ResetsAt
	}
	return time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)
}
func subscriptionLess(a, b model.Subscription, r Runtime) bool {
	ra, rb := trustedReset(r.Usage[a.ID]), trustedReset(r.Usage[b.ID])
	if !ra.Equal(rb) {
		return ra.Before(rb)
	}
	aAfter, bAfter := a.ID > r.LastSubscription, b.ID > r.LastSubscription
	if aAfter != bAfter {
		return aAfter
	}
	return a.ID < b.ID
}
func (s *Service) PutUsage(u model.Usage) error {
	return s.withLock(func() error {
		c, e := s.config()
		if e != nil {
			return e
		}
		found := false
		for _, sub := range c.Subscriptions {
			if sub.ID == u.SubscriptionID {
				found = true
				if sub.AccountID != "" && (u.AccountID != sub.AccountID || u.PoolID != sub.PoolID) {
					return fmt.Errorf("usage account or pool mismatch")
				}
			}
		}
		if !found {
			return fmt.Errorf("subscription not found")
		}
		r, e := s.runtime()
		if e != nil {
			return e
		}
		if old, ok := r.Usage[u.SubscriptionID]; ok && u.ObservedAt.Before(old.ObservedAt) {
			return fmt.Errorf("usage observation is older than stored evidence")
		}
		r.Usage[u.SubscriptionID] = u
		for _, sub := range c.Subscriptions {
			if sub.PoolID != "" && sub.PoolID == u.PoolID && sub.AccountID == u.AccountID {
				copy := u
				copy.SubscriptionID = sub.ID
				r.Usage[sub.ID] = copy
			}
		}
		return s.saveRuntime(&r)
	})
}
func (s *Service) RefreshUsage(ctx context.Context, id string) (model.Usage, error) {
	c, e := s.Config()
	if e != nil {
		return model.Usage{}, e
	}
	for _, sub := range c.Subscriptions {
		if sub.ID == id {
			u, e := provider.CollectUsage(ctx, sub)
			if e != nil {
				return u, e
			}
			return u, s.PutUsage(u)
		}
	}
	return model.Usage{}, fmt.Errorf("subscription not found")
}
