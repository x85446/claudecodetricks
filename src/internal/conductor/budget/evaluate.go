package budget

import (
	"fmt"
	"math"
	"sort"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

func Universal() model.PowerBand {
	g := make([]float64, 8)
	for i := range g {
		g[i] = float64(i) * 100 / 7
	}
	return model.PowerBand{ID: "universal", Name: "Universal power band", GreenCurve: g, RedCurve: []float64{0, 28, 48, 58, 72, 80, 95, 100}, ShortGreen: 20, ShortRed: 40, AllowedBands: []model.Band{model.Green}}
}
func percent(n float64) bool { return !math.IsNaN(n) && !math.IsInf(n, 0) && n >= 0 && n <= 100 }
func rank(b model.Band) int {
	switch b {
	case model.Green:
		return 0
	case model.Yellow:
		return 1
	case model.Red:
		return 2
	}
	return -1
}
func ValidatePolicy(p model.PowerBand) error {
	if p.ID == "" || p.Name == "" {
		return fmt.Errorf("power band ID and name are required")
	}
	if len(p.GreenCurve) != 8 || len(p.RedCurve) != 8 {
		return fmt.Errorf("weekly curves require eight values for days 0 through 7")
	}
	for i := 0; i < 8; i++ {
		g, r := p.GreenCurve[i], p.RedCurve[i]
		if !percent(g) || !percent(r) || g > r {
			return fmt.Errorf("day %d thresholds must be ordered percentages", i)
		}
		if i > 0 && (g < p.GreenCurve[i-1] || r < p.RedCurve[i-1]) {
			return fmt.Errorf("weekly curves must be nondecreasing")
		}
	}
	if !percent(p.ShortGreen) || !percent(p.ShortRed) || p.ShortGreen > p.ShortRed {
		return fmt.Errorf("short-window thresholds must be ordered percentages")
	}
	if len(p.AllowedBands) == 0 {
		return fmt.Errorf("select at least one fallback allowed band")
	}
	seen := map[model.Band]bool{}
	for _, b := range p.AllowedBands {
		if rank(b) < 0 || seen[b] {
			return fmt.Errorf("allowed bands must be unique green, yellow or red")
		}
		seen[b] = true
	}
	return nil
}
func interpolate(curve []float64, day float64) float64 {
	day = math.Max(0, math.Min(7, day))
	i := int(day)
	if i == 7 {
		return curve[7]
	}
	return curve[i] + (curve[i+1]-curve[i])*(day-float64(i))
}

// Thresholds exposes the same interpolated pace geometry used by admission and samples.
func Thresholds(p model.PowerBand, day float64) (float64, float64) {
	if ValidatePolicy(p) != nil || math.IsNaN(day) {
		return 0, 0
	}
	return interpolate(p.GreenCurve, day), interpolate(p.RedCurve, day)
}
func weeklyBand(used, green, red float64) model.Band {
	if used >= 100 {
		return model.Red
	}
	if used <= green {
		return model.Green
	}
	if used >= red {
		return model.Red
	}
	return model.Yellow
}
func shortBand(used, green, red float64) model.Band {
	if used >= 100 || used >= red {
		return model.Red
	}
	if used >= green {
		return model.Yellow
	}
	return model.Green
}
func reject(d model.BudgetDecision, code, reason string) model.BudgetDecision {
	d.Allowed = false
	d.Code = code
	d.Reason = reason
	return d
}
func allowanceError(a *model.Allowance, now time.Time, weekly bool) string {
	if a == nil {
		return "allowance is unavailable"
	}
	if !percent(a.UsedPercent) {
		return "allowance percentage is malformed"
	}
	if a.ResetsAt.IsZero() || !a.ResetsAt.After(now) {
		return "allowance reset requires fresh evidence"
	}
	if !a.StartsAt.IsZero() && (!a.StartsAt.Before(a.ResetsAt) || a.StartsAt.After(now)) {
		return "allowance interval is malformed"
	}
	return ""
}

// Evaluate returns a pure decision and occurrence transitions. Callers persist
// Occurrences before dispatch; a simulator discards them. maxBand="" uses the
// named policy's fallback bands. An explicit row authorizes all bands up to its maximum.
func Evaluate(p model.PowerBand, s model.Subscription, u model.Usage, now time.Time, maxBand model.Band, honor, only bool, recorded []model.Occurrence) model.BudgetDecision {
	d := model.BudgetDecision{Occurrences: append([]model.Occurrence(nil), recorded...)}
	if err := ValidatePolicy(p); err != nil {
		return reject(d, "invalid_policy", err.Error())
	}
	if maxBand != "" && rank(maxBand) < 0 {
		return reject(d, "invalid_band", "unknown maximum band")
	}
	if only && !honor {
		return reject(d, "invalid_overdrive_mode", "overdrive-only requires honoring subscription overdrive")
	}
	if !s.Enabled {
		return reject(d, "subscription_disabled", "Subscription disabled")
	}
	if s.ID == "" || s.PoolID == "" || u.SubscriptionID != s.ID || u.PoolID != s.PoolID || (s.AccountID != "" && u.AccountID != s.AccountID) {
		return reject(d, "usage_scope", "Usage does not match this subscription and allowance pool")
	}
	if !u.Supported {
		return reject(d, "usage_unsupported", "Allowance telemetry is unsupported")
	}
	freshness := s.FreshnessSeconds
	if freshness <= 0 {
		freshness = 300
	}
	if u.ObservedAt.IsZero() || now.Sub(u.ObservedAt) > time.Duration(freshness)*time.Second || u.ObservedAt.After(now.Add(time.Minute)) {
		return reject(d, "usage_stale", "Wait for fresh allowance evidence")
	}
	if u.Source == "" {
		return reject(d, "usage_unknown", "Allowance provenance is unavailable")
	}
	if reason := allowanceError(u.Weekly, now, true); reason != "" {
		return reject(d, "weekly_unknown", "Weekly "+reason)
	}
	if reason := allowanceError(u.Short, now, false); reason != "" && !(u.Short == nil && u.ShortUnsupported) {
		return reject(d, "short_unknown", "Short-window "+reason)
	}
	weekly, short := u.Weekly.UsedPercent, 0.0
	d.WeeklyUsed = &weekly
	if u.Short != nil {
		short = u.Short.UsedPercent
		d.ShortUsed = &short
	}
	// Weekly pace matches the seven-day status-line clock. Explicit starts permit
	// a provider to report a different actual allowance period without inventing one.
	start := u.Weekly.StartsAt
	if start.IsZero() {
		start = u.Weekly.ResetsAt.Add(-7 * 24 * time.Hour)
	}
	day := 7 * now.Sub(start).Seconds() / u.Weekly.ResetsAt.Sub(start).Seconds()
	d.GreenEnd, d.RedStart = Thresholds(p, day)
	d.WeeklyBand = weeklyBand(weekly, d.GreenEnd, d.RedStart)
	if u.Short != nil {
		d.ShortBand = shortBand(short, p.ShortGreen, p.ShortRed)
	}
	d.Band = d.WeeklyBand
	if rank(d.ShortBand) > rank(d.Band) {
		d.Band = d.ShortBand
	}
	// Occurrence retirement remains independent of the chosen automation's opt-in.
	currentEpoch := epoch(u.Weekly)
	index := map[string]int{}
	for i := range d.Occurrences {
		o := &d.Occurrences[i]
		index[o.ID] = i
		if o.SubscriptionID != s.ID {
			continue
		}
		reset := currentEpoch != "" && o.Epoch != "" && currentEpoch != o.Epoch
		if !now.Before(o.End) || reset {
			o.Completed = true
		}
	}
	var active []model.OverdriveWindow
	if s.OverdriveEnabled {
		for _, w := range s.OverdriveWindows {
			if !w.Enabled {
				continue
			}
			o, ok, err := OverdriveAt(w, u.Weekly, now)
			if err != nil {
				return reject(d, "invalid_overdrive", err.Error())
			}
			if !ok {
				continue
			}
			o.SubscriptionID = s.ID
			if i, found := index[o.ID]; found {
				o = d.Occurrences[i]
			} else {
				index[o.ID] = len(d.Occurrences)
				d.Occurrences = append(d.Occurrences, o)
			}
			if weekly >= w.TargetUsedPercent {
				o.Completed = true
				d.Occurrences[index[o.ID]] = o
			}
			if !o.Completed && w.TargetUsedPercent > weekly {
				active = append(active, w)
			}
		}
	}
	if u.HardBlocked {
		reason := u.Reason
		if reason == "" {
			reason = "Provider blocks allowance use"
		}
		return reject(d, "provider_blocked", reason)
	}
	// All reported model pools are additional hard limits; unknown model pools
	// remain absent rather than being inferred from execution token totals.
	names := make([]string, 0, len(u.Models))
	for name := range u.Models {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		a := u.Models[name]
		if reason := allowanceError(&a, now, false); reason != "" {
			return reject(d, "model_unknown", "Model "+name+" "+reason)
		}
		if a.UsedPercent >= 100 {
			return reject(d, "model_exhausted", "Model allowance exhausted: "+name)
		}
	}
	if weekly >= 100 {
		return reject(d, "weekly_exhausted", "Weekly allowance exhausted")
	}
	if short >= 100 {
		return reject(d, "short_exhausted", "Short-window allowance exhausted")
	}
	d.HardAllowed = true
	if honor && len(active) > 0 {
		d.OverdriveActive = true
		for _, w := range active {
			d.WindowIDs = append(d.WindowIDs, w.ID)
			if w.TargetUsedPercent > d.TargetUsedPercent {
				d.TargetUsedPercent = w.TargetUsedPercent
			}
		}
		sort.Strings(d.WindowIDs)
		d.GreenEnd = math.Max(d.GreenEnd, d.TargetUsedPercent)
		d.RedStart = math.Max(d.RedStart, d.GreenEnd)
		d.Allowed = true
		d.Code = "overdrive"
		d.Reason = fmt.Sprintf("Overdrive active until %.1f%% weekly used", d.TargetUsedPercent)
		return d
	}
	if only {
		return reject(d, "outside_overdrive", "Waiting for participating subscription overdrive")
	}
	allowed := false
	if maxBand != "" {
		allowed = rank(d.Band) <= rank(maxBand)
	} else {
		for _, b := range p.AllowedBands {
			if b == d.Band {
				allowed = true
			}
		}
	}
	if !allowed {
		return reject(d, "band", "Waiting: "+string(d.Band)+" band exceeds this automation's allowance")
	}
	d.Allowed = true
	d.Code = "normal"
	d.Reason = "Normal budget allows work"
	return d
}

// EvaluateForModel applies only the selected model's reported additional pool.
// Absence of that pool does not fabricate a model allowance or bypass account limits.
func EvaluateForModel(p model.PowerBand, s model.Subscription, u model.Usage, now time.Time, maxBand model.Band, honor, only bool, recorded []model.Occurrence, modelID string) model.BudgetDecision {
	scoped := u
	scoped.Models = nil
	if a, ok := u.Models[modelID]; ok && modelID != "" {
		scoped.Models = map[string]model.Allowance{modelID: a}
	}
	return Evaluate(p, s, scoped, now, maxBand, honor, only, recorded)
}
