package budget_test

import (
	"encoding/json"
	"github.com/x85446/claudecodetricks/src/internal/conductor/budget"
	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
	"math"
	"strings"
	"testing"
	"time"
)

// TESTMASTER: id=javelina-policy-preview tier=? parallel=yes
func TestJavelinaPolicyPreview(t *testing.T) {
	p := budget.Universal()
	budget.NormalizeShort(&p)
	for _, row := range []struct{ day, hour, used float64 }{{0, 0, 0}, {0.001, 0.001, 0}, {7, 5, 100}, {3.4, 2.55, 51}} {
		d, e := budget.PreviewPolicy(budget.PolicySample{Policy: p, Day: row.day, Hour: row.hour, WeeklyUsed: row.used, ShortUsed: row.used})
		if e != nil {
			t.Fatal(e)
		}
		if row.used == 0 && (d.Weekly.Band != model.Green || d.Short.Band != model.Green) {
			t.Fatal("zero sample", d)
		}
		if row.used == 100 && (d.Weekly.Band != model.Red || d.Short.Band != model.Red) {
			t.Fatal("exhaustion", d)
		}
		b, _ := json.Marshal(d)
		for _, unwanted := range []string{"allowed", "subscription", "overdrive", "reason"} {
			if strings.Contains(string(b), unwanted) {
				t.Fatal("sample leaks admission", string(b))
			}
		}
	}
	for _, v := range []float64{-1, 101, math.NaN(), math.Inf(1)} {
		if _, e := budget.PreviewPolicy(budget.PolicySample{Policy: p, WeeklyUsed: v}); e == nil {
			t.Fatal("invalid used accepted", v)
		}
	}
	p.ShortGreenCurve = []float64{0, 10, 20, 30, 40, 50}
	p.ShortRedCurve = []float64{0, 20, 40, 60, 80, 100}
	for _, h := range []float64{0, 1, 2, 2.55, 3, 4, 5} {
		g, r := budget.ShortThresholds(p, h)
		if math.Abs(g-h*10) > 0.00001 || math.Abs(r-h*20) > 0.00001 {
			t.Fatal(h, g, r)
		}
	}
	if _, e := budget.PreviewPolicy(budget.PolicySample{Policy: p, Hour: 6}); e == nil {
		t.Fatal("hour beyond five accepted")
	}
	p.ShortRedCurve[2] = 1
	if budget.ValidatePolicy(p) == nil {
		t.Fatal("inverted hourly threshold accepted")
	}
	p.ShortRedCurve = []float64{0, 20}
	if budget.ValidatePolicy(p) == nil {
		t.Fatal("invalid length accepted")
	}
}

// TESTMASTER: id=javelina-hourly-admission tier=? parallel=yes
func TestJavelinaHourlyAdmission(t *testing.T) {
	p, s, u, now := fixture()
	p.ShortGreenCurve = []float64{0, 10, 20, 30, 40, 50}
	p.ShortRedCurve = []float64{0, 20, 40, 60, 80, 100}
	u.Short.StartsAt = now.Add(-2 * time.Hour)
	u.Short.ResetsAt = now.Add(3 * time.Hour)
	u.Short.UsedPercent = 30
	d := budget.Evaluate(p, s, u, now, model.Red, false, false, nil)
	if d.ShortBand != model.Yellow {
		t.Fatal(d)
	}
	u.Short.StartsAt = time.Time{}
	d = budget.Evaluate(p, s, u, now, model.Red, false, false, nil)
	if d.Code != "short_unknown" || d.Allowed {
		t.Fatal(d)
	}
	p = budget.Universal()
	before := budget.Evaluate(p, s, u, now, model.Red, false, false, nil)
	budget.NormalizeShort(&p)
	after := budget.Evaluate(p, s, u, now, model.Red, false, false, nil)
	if before.ShortBand != after.ShortBand || before.Allowed != after.Allowed {
		t.Fatal("fixed policy changed", before, after)
	}
}
