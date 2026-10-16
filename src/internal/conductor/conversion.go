package conductor

import (
	"context"
	"fmt"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

type OverdriveConversion struct {
	Revision       int64                 `json:"revision"`
	SubscriptionID string                `json:"subscription_id"`
	Window         model.OverdriveWindow `json:"window"`
	Start          time.Time             `json:"start"`
	End            time.Time             `json:"end"`
	Timezone       string                `json:"timezone"`
	Source         string                `json:"source"`
	Explanation    string                `json:"explanation"`
}

// PreviewOverdriveConversion derives an account-owned hour offset without saving.
// The returned config revision guards the caller's ordinary Apply transaction.
func (s *Service) PreviewOverdriveConversion(ctx context.Context, subscriptionID, windowID string) (out OverdriveConversion, err error) {
	err = s.withLock(func() error {
		if e := ctx.Err(); e != nil {
			return e
		}
		c, e := s.config()
		if e != nil {
			return e
		}
		r, e := s.runtime()
		if e != nil {
			return e
		}
		field := fmt.Sprintf("subscriptions[%s].overdrive_windows[%s]", subscriptionID, windowID)
		fail := func(message string) error { return fmt.Errorf("%s: %s", field, message) }
		for _, sub := range c.Subscriptions {
			if sub.ID != subscriptionID {
				continue
			}
			for _, w := range sub.OverdriveWindows {
				if w.ID != windowID {
					continue
				}
				u, now := r.Usage[sub.ID], s.now()
				freshness := sub.FreshnessSeconds
				if freshness <= 0 {
					freshness = 300
				}
				if sub.AccountID == "" || sub.PoolID == "" || u.AccountID != sub.AccountID || u.PoolID != sub.PoolID || u.SubscriptionID != sub.ID {
					return fail("conversion requires allowance evidence bound to this subscription")
				}
				if !u.Supported || u.Source == "" || u.ObservedAt.IsZero() || now.Sub(u.ObservedAt) > time.Duration(freshness)*time.Second || u.ObservedAt.After(now.Add(time.Minute)) {
					return fail("conversion requires fresh allowance evidence")
				}
				a := u.Weekly
				if a == nil || !a.ResetTrusted || !a.ResetsAt.After(now) || a.StartsAt.IsZero() || a.StartsAt.After(now) || !a.StartsAt.Before(a.ResetsAt) {
					return fail("conversion requires a trusted current weekly reset and allowance interval")
				}
				probe := w
				probe.Enabled = true
				if w.StartMode != "reset_relative" {
					if len(w.Recurrence.Days) != 1 || w.Recurrence.EveryWeeks != 1 {
						return fail("multiple days or multiweek recurrence cannot preserve timing as one reset-relative window")
					}
					switch w.EndMode {
					case "", "reset", "weekly_reset", "next_reset":
					default:
						return fail("ordinary weekly windows with an independent end remain weekly; conversion requires an end at the trusted reset")
					}
				}
				o, active, e := budget.OverdriveAt(probe, a, a.ResetsAt.Add(-time.Nanosecond))
				if e != nil {
					return fail(e.Error())
				}
				if !active || o.Start.Before(a.StartsAt) || !o.End.Equal(a.ResetsAt) {
					return fail("no equivalent occurrence exists in the current allowance interval")
				}
				w.StartMode = "reset_relative"
				w.HoursBeforeReset = o.End.Sub(o.Start).Hours()
				if e := budget.ValidateOverdrive(w); e != nil {
					return fail(e.Error())
				}
				out = OverdriveConversion{Revision: c.Revision, SubscriptionID: sub.ID, Window: w, Start: o.Start, End: o.End, Timezone: w.Recurrence.Timezone, Source: u.Source, Explanation: "Preserves this allowance interval's start and reset exactly. Future starts follow this hour offset from the trusted reset; daylight-saving or provider reset changes can change the local start time."}
				return nil
			}
			return fail("window not found")
		}
		return fail("subscription not found")
	})
	return
}
