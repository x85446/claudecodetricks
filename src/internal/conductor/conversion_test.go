package conductor

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

// TESTMASTER: id=indigobird-account-preview-hours tier=fast parallel=yes
func TestIndigobirdOverdriveConversion(t *testing.T) {
	for _, tc := range []struct {
		name, zone string
		month      time.Month
		day        int
		hours      float64
	}{
		{"ordinary", "UTC", time.October, 18, 27.5},
		{"fall-back", "America/Chicago", time.November, 1, 28.5},
		{"spring-forward", "America/Chicago", time.March, 8, 26.5},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, _, c := fixture(t, "")
			s = NewService(filepath.Join(t.TempDir(), "store"), s.Source)
			c.Revision = 0
			loc, err := time.LoadLocation(tc.zone)
			if err != nil {
				t.Fatal(err)
			}
			// Both accounts share the same civil start definition, with independent reset instants.
			reset := time.Date(2026, tc.month, tc.day, 3, 30, 0, 0, loc)
			if tc.name == "spring-forward" {
				reset = time.Date(2026, tc.month, tc.day, 3, 30, 0, 0, loc)
			}
			now := reset.AddDate(0, 0, -2)
			s.Clock = func() time.Time { return now }
			twoSubscriptions(&c)
			for i := range c.Subscriptions {
				c.Subscriptions[i].OverdriveEnabled = true
				c.Subscriptions[i].OverdriveWindows = []model.OverdriveWindow{{ID: fmt.Sprintf("midnight-%d", i), Name: "Before reset", Enabled: true, StartMode: "weekly", EndMode: "next_reset", TargetUsedPercent: 60, Recurrence: model.Recurrence{Days: []int{6}, Start: "00:00", End: "00:00", Timezone: tc.zone, EveryWeeks: 1}}}
			}
			c.Automations[0].Mode = "overdrive"
			c.Automations[0].HonorOverdrive = true
			c.Automations[0].OnlyOverdrive = true
			c.Automations[0].Participants = []string{"sequence", "second-sequence"}
			c.Automations[0].Entries = nil
			c, err = s.Apply(c.Revision, c)
			if err != nil {
				t.Fatal(err)
			}
			for i, sub := range c.Subscriptions {
				end := reset.Add(time.Duration(i) * time.Hour)
				err = s.PutUsage(model.Usage{SubscriptionID: sub.ID, AccountID: sub.AccountID, PoolID: sub.PoolID, Supported: true, Source: "provider reset fixture", ObservedAt: now, Weekly: &model.Allowance{StartsAt: end.AddDate(0, 0, -7), ResetsAt: end, ResetTrusted: true, Epoch: sub.ID}, ShortUnsupported: true})
				if err != nil {
					t.Fatal(err)
				}
			}
			cfgBefore, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
			runtimeBefore, _ := os.ReadFile(filepath.Join(s.Dir, "runtime.json"))
			converted := make([]OverdriveConversion, 2)
			for i, sub := range c.Subscriptions {
				p, e := s.PreviewOverdriveConversion(context.Background(), sub.ID, sub.OverdriveWindows[0].ID)
				if e != nil {
					t.Fatal(e)
				}
				converted[i] = p
				if p.Revision != c.Revision || p.Window.HoursBeforeReset != tc.hours+float64(i) || p.Window.ID != sub.OverdriveWindows[0].ID || p.Window.TargetUsedPercent != 60 || p.Window.Name != "Before reset" || !p.Window.Enabled || p.Source != "provider reset fixture" {
					t.Fatalf("incorrect conversion %+v", p)
				}
				r, _ := s.Runtime()
				a := r.Usage[sub.ID].Weekly
				original, ok, e := budget.OverdriveAt(sub.OverdriveWindows[0], a, p.Start.Add(time.Second))
				if e != nil || !ok || !original.Start.Equal(p.Start) || !original.End.Equal(p.End) {
					t.Fatalf("original timing %+v %+v %v", original, p, e)
				}
			}
			cfgAfter, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
			runtimeAfter, _ := os.ReadFile(filepath.Join(s.Dir, "runtime.json"))
			if string(cfgBefore) != string(cfgAfter) || string(runtimeBefore) != string(runtimeAfter) {
				t.Fatal("preview mutated persistent state")
			}
			for i := range c.Subscriptions {
				c.Subscriptions[i].OverdriveWindows[0] = converted[i].Window
			}
			c, err = s.Apply(converted[0].Revision, c)
			if err != nil {
				t.Fatal(err)
			}
			restarted := NewService(s.Dir, s.Source)
			restarted.Clock = s.Clock
			reloaded, err := restarted.Config()
			if err != nil {
				t.Fatal(err)
			}
			for i, sub := range reloaded.Subscriptions {
				p, e := restarted.PreviewOverdriveConversion(context.Background(), sub.ID, sub.OverdriveWindows[0].ID)
				if e != nil {
					t.Fatal(e)
				}
				if !p.Start.Equal(converted[i].Start) || !p.End.Equal(converted[i].End) || p.Window.HoursBeforeReset != converted[i].Window.HoursBeforeReset {
					t.Fatalf("save/reload moved occurrence %+v", p)
				}
			}
			if strings.Join(reloaded.Automations[0].Participants, ",") != "sequence,second-sequence" {
				t.Fatal("conversion rewired participating sequences")
			}
			if _, err = s.Apply(converted[0].Revision, c); err != ErrConflict {
				t.Fatalf("stale conversion save %v", err)
			}
			// Fresh trusted evidence is mandatory; typical reset metadata is not substituted.
			r, _ := s.Runtime()
			u := r.Usage[c.Subscriptions[0].ID]
			u.Weekly.ResetTrusted = false
			if err = s.PutUsage(u); err != nil {
				t.Fatal(err)
			}
			if _, err = s.PreviewOverdriveConversion(context.Background(), u.SubscriptionID, c.Subscriptions[0].OverdriveWindows[0].ID); err == nil || !strings.Contains(err.Error(), "trusted") {
				t.Fatalf("untrusted reset accepted %v", err)
			}
			u.Weekly.ResetTrusted = true
			if err = s.PutUsage(u); err != nil {
				t.Fatal(err)
			}
			now = now.Add(301 * time.Second)
			if _, err = s.PreviewOverdriveConversion(context.Background(), u.SubscriptionID, c.Subscriptions[0].OverdriveWindows[0].ID); err == nil || !strings.Contains(err.Error(), "fresh") {
				t.Fatalf("stale reset accepted %v", err)
			}
		})
	}
}

// TESTMASTER: id=indigobird-account-preview-hours tier=fast parallel=yes
func TestIndigobirdOverdriveConversionRefusesLossySchedule(t *testing.T) {
	s, _, c := fixture(t, "")
	w := model.OverdriveWindow{ID: "weekly", Name: "Ordinary weekly", Enabled: false, StartMode: "weekly", EndMode: "recurrence", TargetUsedPercent: 100, Recurrence: model.Recurrence{Days: []int{6}, Start: "00:00", End: "12:00", Timezone: "UTC", EveryWeeks: 1}}
	for _, scenario := range []string{"independent-end", "multiple-days", "multiweek"} {
		t.Run(scenario, func(t *testing.T) {
			draft := w
			if scenario != "independent-end" {
				draft.EndMode = "next_reset"
			}
			if scenario == "multiple-days" {
				draft.Recurrence.Days = []int{5, 6}
			}
			if scenario == "multiweek" {
				draft.Recurrence.EveryWeeks = 2
				draft.Recurrence.AnchorDate = "2026-10-12"
			}
			c.Subscriptions[0].OverdriveWindows = []model.OverdriveWindow{draft}
			var err error
			c, err = s.Apply(c.Revision, c)
			if err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
			if _, err = s.PreviewOverdriveConversion(context.Background(), "account", "weekly"); err == nil {
				t.Fatal("lossy conversion accepted")
			}
			after, _ := os.ReadFile(filepath.Join(s.Dir, "config.json"))
			if string(before) != string(after) {
				t.Fatal("refusal mutated config")
			}
		})
	}
}
