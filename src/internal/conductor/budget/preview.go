package budget

import (
	"fmt"
	"math"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

// PreviewStarts returns civil recurrence boundaries within a bounded preview.
// It shares the scheduler's gap, fold, overnight and anchor-date resolution.
func PreviewStarts(r model.Recurrence, from, until time.Time) ([]time.Time, error) {
	if err := ValidateRecurrence(r); err != nil {
		return nil, err
	}
	if !until.After(from) || until.Sub(from) > 31*24*time.Hour {
		return nil, fmt.Errorf("preview horizon must be positive and at most 31 days")
	}
	loc, _ := time.LoadLocation(r.Timezone)
	first, last := civilDate(from.In(loc)).AddDate(0, 0, -7), civilDate(until.In(loc))
	var out []time.Time
	for day := first; !day.After(last); day = day.AddDate(0, 0, 1) {
		if scheduledDay(r, day) {
			out = append(out, recurrenceOn(r, day).Start)
		}
	}
	return out, nil
}

// PolicySample contains only the draft policy and explicit sample inputs.
type PolicySample struct {
	Policy     model.PowerBand `json:"policy"`
	Day        float64         `json:"day"`
	Hour       float64         `json:"hour"`
	WeeklyUsed float64         `json:"weekly_used"`
	ShortUsed  float64         `json:"short_used"`
}
type SampleBand struct {
	GreenEnd float64    `json:"green_end"`
	RedStart float64    `json:"red_start"`
	Used     float64    `json:"used"`
	Band     model.Band `json:"band"`
}
type PolicyGeometry struct {
	Weekly SampleBand `json:"weekly"`
	Short  SampleBand `json:"short"`
}

func ShortThresholds(p model.PowerBand, hour float64) (float64, float64) {
	if len(p.ShortGreenCurve) == 6 && len(p.ShortRedCurve) == 6 {
		return interpolate(p.ShortGreenCurve, hour), interpolate(p.ShortRedCurve, hour)
	}
	return p.ShortGreen, p.ShortRed
}
func ConstantShort(p model.PowerBand) bool {
	for _, curve := range [][]float64{p.ShortGreenCurve, p.ShortRedCurve} {
		for _, v := range curve {
			if v != curve[0] {
				return false
			}
		}
	}
	return true
}

// NormalizeShort expands legacy fixed thresholds into constant hourly curves.
func NormalizeShort(p *model.PowerBand) {
	if len(p.ShortGreenCurve) == 0 && len(p.ShortRedCurve) == 0 {
		for i := 0; i < 6; i++ {
			p.ShortGreenCurve = append(p.ShortGreenCurve, p.ShortGreen)
			p.ShortRedCurve = append(p.ShortRedCurve, p.ShortRed)
		}
	}
}
func PreviewPolicy(s PolicySample) (PolicyGeometry, error) {
	var out PolicyGeometry
	if err := ValidatePolicy(s.Policy); err != nil {
		return out, err
	}
	if math.IsNaN(s.Day) || math.IsNaN(s.Hour) || s.Day < 0 || s.Day > 7 || s.Hour < 0 || s.Hour > 5 || !percent(s.WeeklyUsed) || !percent(s.ShortUsed) {
		return out, fmt.Errorf("sample time and percentages are outside their ranges")
	}
	g, r := Thresholds(s.Policy, s.Day)
	out.Weekly = SampleBand{g, r, s.WeeklyUsed, weeklyBand(s.WeeklyUsed, g, r)}
	g, r = ShortThresholds(s.Policy, s.Hour)
	out.Short = SampleBand{g, r, s.ShortUsed, shortBand(s.ShortUsed, g, r)}
	if s.ShortUsed == 0 {
		out.Short.Band = model.Green
	}
	return out, nil
}
