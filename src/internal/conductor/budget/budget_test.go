package budget_test

import (
	"math"
	"testing"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

func instant(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}
func fixture() (model.PowerBand, model.Subscription, model.Usage, time.Time) {
	now := instant("2026-10-15T16:00:00Z")
	s := model.Subscription{ID: "codex", Name: "Codex", Enabled: true, PoolID: "real-pool", PowerBandID: "universal", FreshnessSeconds: 300, OverdriveEnabled: true}
	u := model.Usage{SubscriptionID: s.ID, PoolID: s.PoolID, Source: "provider-test-fixture", ObservedAt: now, Supported: true, Weekly: &model.Allowance{UsedPercent: 38, StartsAt: now.Add(-3 * 24 * time.Hour), ResetsAt: now.Add(4 * 24 * time.Hour), ResetTrusted: true, Epoch: "week1"}, Short: &model.Allowance{UsedPercent: 12, StartsAt: now.Add(-time.Hour), ResetsAt: now.Add(4 * time.Hour), ResetTrusted: true, Epoch: "short1"}}
	return budget.Universal(), s, u, now
}
func window(id string, target float64) model.OverdriveWindow {
	return model.OverdriveWindow{ID: id, Name: id, Enabled: true, Revision: 1, StartMode: "weekly", EndMode: "reset", TargetUsedPercent: target, Recurrence: model.Recurrence{Days: []int{4}, Start: "00:00", End: "00:00", Timezone: "UTC", EveryWeeks: 1}}
}

// TESTMASTER: id=indigobird-bands tier=fast parallel=yes
func TestIndigobirdBudgetBands(t *testing.T) {
	p, s, u, now := fixture()
	for _, c := range []struct {
		used, short float64
		max         model.Band
		band        model.Band
		allowed     bool
	}{{38, 12, model.Green, model.Green, true}, {50, 12, model.Green, model.Yellow, false}, {50, 12, model.Yellow, model.Yellow, true}, {60, 12, model.Yellow, model.Red, false}, {60, 12, model.Red, model.Red, true}, {38, 20, model.Green, model.Yellow, false}, {38, 40, model.Yellow, model.Red, false}, {38, 40, model.Red, model.Red, true}, {100, 0, model.Red, model.Red, false}, {0, 100, model.Red, model.Red, false}} {
		u.Weekly.UsedPercent = c.used
		u.Short.UsedPercent = c.short
		d := budget.Evaluate(p, s, u, now, c.max, false, false, nil)
		if d.Band != c.band || d.Allowed != c.allowed {
			t.Errorf("used=%v short=%v max=%s got %+v", c.used, c.short, c.max, d)
		}
	}
	u.Weekly.UsedPercent = 50
	u.Short.UsedPercent = 0
	if budget.Evaluate(p, s, u, now, "", false, false, nil).Allowed {
		t.Fatal("fallback Green permitted Yellow")
	}
	p.AllowedBands = []model.Band{model.Yellow}
	if !budget.Evaluate(p, s, u, now, "", false, false, nil).Allowed {
		t.Fatal("editable fallback ignored")
	}
	g, r := budget.Thresholds(p, 3.5)
	if math.Abs(g-50) > .001 || r != 65 {
		t.Fatalf("interpolation %v/%v", g, r)
	}
	p.RedCurve[3] = 10
	if budget.ValidatePolicy(p) == nil {
		t.Fatal("invalid curve accepted")
	}
}

// TESTMASTER: id=indigobird-remaining tier=fast parallel=yes
func TestIndigobirdUsageGates(t *testing.T) {
	for _, kind := range []string{"scope", "pool", "stale", "future", "unsupported", "missing", "malformed", "reset", "source", "short"} {
		t.Run(kind, func(t *testing.T) {
			p, s, u, now := fixture()
			s.OverdriveWindows = []model.OverdriveWindow{window("w", 100)}
			switch kind {
			case "scope":
				u.SubscriptionID = "other"
			case "pool":
				u.PoolID = "other"
			case "stale":
				u.ObservedAt = now.Add(-6 * time.Minute)
			case "future":
				u.ObservedAt = now.Add(time.Hour)
			case "unsupported":
				u.Supported = false
			case "missing":
				u.Weekly = nil
			case "malformed":
				u.Weekly.UsedPercent = math.NaN()
			case "reset":
				u.Weekly.ResetsAt = now
			case "source":
				u.Source = ""
			case "short":
				u.Short = nil
			}
			d := budget.Evaluate(p, s, u, now, model.Red, true, true, nil)
			if d.Allowed || d.HardAllowed {
				t.Fatalf("%s bypassed by overdrive: %+v", kind, d)
			}
		})
	}
	p, s, u, now := fixture()
	d := budget.Evaluate(p, s, u, now, model.Green, false, false, nil)
	if *d.WeeklyUsed != 38 || *d.ShortUsed != 12 || 100-*d.WeeklyUsed != 62 {
		t.Fatal("used/remaining inverted")
	}
	u.Models = map[string]model.Allowance{"other": {UsedPercent: 100, ResetsAt: now.Add(time.Hour)}}
	if !budget.EvaluateForModel(p, s, u, now, model.Green, false, false, nil, "selected").Allowed {
		t.Fatal("unrelated model blocked")
	}
	if budget.EvaluateForModel(p, s, u, now, model.Green, false, false, nil, "other").Allowed {
		t.Fatal("selected model hard limit bypassed")
	}
}

// TESTMASTER: id=indigobird-overdrive-targets tier=fast parallel=yes
func TestIndigobirdOverdriveTargets(t *testing.T) {
	p, s, u, now := fixture()
	u.Weekly.UsedPercent = 56
	s.OverdriveWindows = []model.OverdriveWindow{window("lower", 55), window("target", 60), window("higher", 80), window("disabled", 100)}
	s.OverdriveWindows[3].Enabled = false
	d := budget.Evaluate(p, s, u, now, model.Green, true, true, nil)
	if !d.Allowed || d.TargetUsedPercent != 80 || d.GreenEnd != 80 || d.RedStart < 80 {
		t.Fatalf("overlap %+v", d)
	}
	s.OverdriveWindows = s.OverdriveWindows[:2]
	d = budget.Evaluate(p, s, u, now, model.Green, true, true, d.Occurrences)
	if !d.Allowed || d.TargetUsedPercent != 60 || d.GreenEnd != 60 {
		t.Fatalf("target60 %+v", d)
	}
	u.Weekly.UsedPercent = 60
	d = budget.Evaluate(p, s, u, now, model.Red, true, true, d.Occurrences)
	if d.Allowed || d.OverdriveActive {
		t.Fatal("target completion admitted overdrive-only")
	}
	u.Weekly.UsedPercent = 50
	d = budget.Evaluate(p, s, u, now, model.Red, true, true, d.Occurrences)
	if d.Allowed {
		t.Fatal("telemetry correction reopened completed occurrence")
	}
	if !budget.Evaluate(p, s, u, now, model.Red, true, false, d.Occurrences).Allowed {
		t.Fatal("regular rule did not restore normal policy")
	}
	s.OverdriveWindows = []model.OverdriveWindow{window("zero", 0)}
	u.Weekly.UsedPercent = 0
	if budget.Evaluate(p, s, u, now, model.Green, true, true, nil).Allowed {
		t.Fatal("zero target permits override")
	}
	s.OverdriveWindows = []model.OverdriveWindow{window("full", 100)}
	u.Weekly.UsedPercent = 99
	d = budget.Evaluate(p, s, u, now, model.Green, true, true, nil)
	if !d.Allowed || d.TargetUsedPercent != 100 {
		t.Fatal("100 target rejected before exhaustion")
	}
	u.Short.UsedPercent = 100
	if budget.Evaluate(p, s, u, now, model.Red, true, true, nil).Allowed {
		t.Fatal("short hard limit bypassed")
	}
}

// TESTMASTER: id=indigobird-project-rules-reset tier=fast parallel=yes
func TestIndigobirdOverdriveLifecycle(t *testing.T) {
	for _, action := range []string{"opt-out", "engine-off", "overdrive-off", "window-off", "delete", "reset", "end"} {
		t.Run(action, func(t *testing.T) {
			p, s, u, now := fixture()
			s.OverdriveWindows = []model.OverdriveWindow{window("w", 60)}
			u.Weekly.UsedPercent = 56
			first := budget.Evaluate(p, s, u, now, model.Green, true, true, nil)
			if !first.Allowed {
				t.Fatal(first)
			}
			honor, only := true, true
			switch action {
			case "opt-out":
				honor, only = false, false
			case "engine-off":
				s.Enabled = false
			case "overdrive-off":
				s.OverdriveEnabled = false
			case "window-off":
				s.OverdriveWindows[0].Enabled = false
			case "delete":
				s.OverdriveWindows = nil
			case "reset":
				u.Weekly.Epoch = "manual-reset"
				u.Weekly.UsedPercent = 0
			case "end":
				now = u.Weekly.ResetsAt
				u.ObservedAt = now
			}
			d := budget.Evaluate(p, s, u, now, model.Green, honor, only, first.Occurrences)
			if d.Allowed {
				t.Fatalf("%s permits work %+v", action, d)
			}
		})
	}
	p, s, u, now := fixture()
	s.OverdriveWindows = []model.OverdriveWindow{window("w", 60)}
	u.Weekly.ResetTrusted = false
	if budget.Evaluate(p, s, u, now, model.Green, true, true, nil).Allowed {
		t.Fatal("unknown reset opens reset-ended window")
	}
	s.OverdriveWindows[0].EndMode = "explicit"
	s.OverdriveWindows[0].EndDay = 5
	s.OverdriveWindows[0].EndTime = "00:00"
	if !budget.Evaluate(p, s, u, now, model.Green, true, true, nil).Allowed {
		t.Fatal("explicit bounded window rejected")
	}
}

// TESTMASTER: id=indigobird-band-simulator tier=fast parallel=yes
func TestIndigobirdPowerbarGeometry(t *testing.T) {
	p, s, u, now := fixture()
	u.Weekly.UsedPercent = 56
	s.OverdriveWindows = []model.OverdriveWindow{window("w", 60)}
	d := budget.Evaluate(p, s, u, now, model.Red, true, true, nil)
	if d.TargetUsedPercent != 60 || d.GreenEnd != 60 || d.RedStart != 60 {
		t.Fatalf("exact target geometry %+v", d)
	}
	p.GreenCurve = []float64{0, 30, 60, 83, 85, 90, 95, 100}
	p.RedCurve = []float64{0, 40, 70, 92, 94, 96, 98, 100}
	d = budget.Evaluate(p, s, u, now, model.Red, true, true, nil)
	if d.TargetUsedPercent != 60 || d.GreenEnd != 83 || d.RedStart != 92 {
		t.Fatalf("normal green beyond bright target lost %+v", d)
	}
	u.Weekly.UsedPercent = 60
	d = budget.Evaluate(p, s, u, now, model.Red, true, false, d.Occurrences)
	if d.OverdriveActive || d.GreenEnd != 83 {
		t.Fatal("normal geometry not restored")
	}
}

// TESTMASTER: id=indigobird-remaining-unsupported tier=fast parallel=yes
func TestIndigobirdUnsupportedShortPool(t *testing.T) {
	p, s, u, now := fixture()
	u.Short = nil
	u.ShortUnsupported = true
	d := budget.Evaluate(p, s, u, now, model.Green, false, false, nil)
	if !d.Allowed || d.ShortUsed != nil || d.ShortBand != "" {
		t.Fatal("verified nonexistent pool fabricated or blocked", d)
	}
	s.AccountID = "account-a"
	u.AccountID = "account-b"
	d = budget.Evaluate(p, s, u, now, model.Green, false, false, nil)
	if d.Allowed || d.Code != "usage_scope" {
		t.Fatal("account mismatch permitted", d)
	}
	u.AccountID = "account-a"
	if !budget.Evaluate(p, s, u, now, model.Green, false, false, nil).Allowed {
		t.Fatal("matching account blocked")
	}
}

// TESTMASTER: id=indigobird-overdrive-custom-disabled tier=fast parallel=yes
func TestIndigobirdOverdriveToggleResume(t *testing.T) {
	p, s, u, now := fixture()
	s.OverdriveWindows = []model.OverdriveWindow{window("w", 60)}
	u.Weekly.UsedPercent = 56
	d := budget.Evaluate(p, s, u, now, model.Green, true, true, nil)
	if !d.Allowed {
		t.Fatal(d)
	}
	s.OverdriveWindows[0].Enabled = false
	d = budget.Evaluate(p, s, u, now, model.Green, true, true, d.Occurrences)
	if d.Allowed || d.Occurrences[0].Completed {
		t.Fatal("disable completed an unmet occurrence", d)
	}
	s.OverdriveWindows[0].Enabled = true
	d = budget.Evaluate(p, s, u, now, model.Green, true, true, d.Occurrences)
	if !d.Allowed {
		t.Fatal("re-enable did not resume unmet occurrence", d)
	}
	s.OverdriveEnabled = false
	d = budget.Evaluate(p, s, u, now, model.Green, true, true, d.Occurrences)
	if d.Allowed {
		t.Fatal("disabled subscription overdrive permits work")
	}
	s.OverdriveEnabled = true
	d = budget.Evaluate(p, s, u, now, model.Green, true, true, d.Occurrences)
	if !d.Allowed {
		t.Fatal("account overdrive re-enable did not resume", d)
	}
}

// TESTMASTER: id=indigobird-overdrive-exhaustion tier=fast parallel=yes
func TestIndigobirdProviderHardBlock(t *testing.T) {
	p, s, u, now := fixture()
	u.HardBlocked = true
	u.Reason = "Provider spend control reached"
	s.OverdriveWindows = []model.OverdriveWindow{window("w", 60)}
	for _, row := range []struct {
		max         model.Band
		honor, only bool
	}{{model.Green, false, false}, {model.Red, false, false}, {model.Red, true, true}} {
		d := budget.Evaluate(p, s, u, now, row.max, row.honor, row.only, nil)
		if d.Allowed || d.HardAllowed || d.Code != "provider_blocked" || d.Reason != u.Reason || d.WeeklyUsed == nil || *d.WeeklyUsed != 38 {
			t.Fatalf("provider block bypassed or usage fabricated: %+v", d)
		}
	}
	u.Weekly.UsedPercent = 60
	d := budget.Evaluate(p, s, u, now, model.Red, true, true, nil)
	if d.Code != "provider_blocked" || len(d.Occurrences) != 1 || !d.Occurrences[0].Completed {
		t.Fatal("provider block lost target completion", d)
	}
	u.HardBlocked = false
	u.Weekly.UsedPercent = 38
	d = budget.Evaluate(p, s, u, now, model.Red, true, true, d.Occurrences)
	if d.Allowed {
		t.Fatal("provider recovery reopened completed occurrence", d)
	}
}
