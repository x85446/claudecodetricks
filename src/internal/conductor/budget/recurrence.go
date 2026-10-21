// Package budget evaluates subscription allowance and civil-time windows without side effects.
package budget

import (
	"crypto/sha256"
	"fmt"
	"math"
	"time"

	"github.com/x85446/claudecodetricks/src/internal/conductor/model"
)

func clockMinutes(s string) (int, error) {
	t, err := time.Parse("15:04", s)
	if err != nil {
		return 0, fmt.Errorf("time must use HH:MM: %q", s)
	}
	return t.Hour()*60 + t.Minute(), nil
}

func ValidateRecurrence(r model.Recurrence) error {
	if len(r.Days) == 0 {
		return fmt.Errorf("select at least one weekday")
	}
	seen := map[int]bool{}
	for _, d := range r.Days {
		if d < 1 || d > 7 || seen[d] {
			return fmt.Errorf("weekdays must be unique Monday=1 through Sunday=7")
		}
		seen[d] = true
	}
	if _, err := clockMinutes(r.Start); err != nil {
		return err
	}
	if _, err := clockMinutes(r.End); err != nil {
		return err
	}
	if r.Timezone == "" {
		return fmt.Errorf("timezone is required")
	}
	if _, err := time.LoadLocation(r.Timezone); err != nil {
		return fmt.Errorf("timezone: %w", err)
	}
	if r.EveryWeeks < 1 || r.EveryWeeks > 5200 {
		return fmt.Errorf("repeat interval must be 1 to 5200 weeks")
	}
	if r.AnchorDate != "" {
		if _, err := time.Parse("2006-01-02", r.AnchorDate); err != nil {
			return fmt.Errorf("anchor date must use YYYY-MM-DD")
		}

	}
	return nil
}

func dayNumber(t time.Time) int {
	d := int(t.Weekday())
	if d == 0 {
		return 7
	}
	return d
}
func civilDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}
func scheduledDay(r model.Recurrence, d time.Time) bool {
	found := false
	for _, n := range r.Days {
		if n == dayNumber(d) {
			found = true
		}
	}
	if !found {
		return false
	}
	anchor := time.Date(1970, 1, 5, 0, 0, 0, 0, time.UTC)
	if r.AnchorDate != "" {
		anchor, _ = time.Parse("2006-01-02", r.AnchorDate)
	}
	anchor = anchor.AddDate(0, 0, 1-dayNumber(anchor))
	date := civilDate(d)
	date = date.AddDate(0, 0, 1-dayNumber(date))
	weeks := int(date.Sub(anchor).Hours() / 168)
	return weeks%r.EveryWeeks == 0
}

// civilInstant chooses the first repeated start and last repeated end. A missing
// spring-forward time resolves to the first valid minute after the clock gap.
func civilInstant(date time.Time, minutes int, loc *time.Location, end bool) time.Time {
	guess := time.Date(date.Year(), date.Month(), date.Day(), minutes/60, minutes%60, 0, 0, loc)
	var first, last, next time.Time
	for t := guess.Add(-4 * time.Hour); !t.After(guess.Add(4 * time.Hour)); t = t.Add(time.Minute) {
		x := t.In(loc)
		if x.Year() != date.Year() || x.Month() != date.Month() || x.Day() != date.Day() {
			continue
		}
		m := x.Hour()*60 + x.Minute()
		if m == minutes {
			if first.IsZero() {
				first = t
			}
			last = t
		}
		if m > minutes && (next.IsZero() || x.Hour()*60+x.Minute() < next.In(loc).Hour()*60+next.In(loc).Minute()) {
			next = t
		}
	}
	if !first.IsZero() {
		if end {
			return last
		}
		return first
	}
	return next
}

func recurrenceOn(r model.Recurrence, date time.Time) model.Occurrence {
	loc, _ := time.LoadLocation(r.Timezone)
	start, _ := clockMinutes(r.Start)
	end, _ := clockMinutes(r.End)
	endDate := date
	if r.AllDay {
		start = 0
		end = 0
	}
	if end <= start {
		endDate = endDate.AddDate(0, 0, 1)
	}
	return model.Occurrence{Start: civilInstant(date, start, loc, false), End: civilInstant(endDate, end, loc, true)}
}

// WindowAt uses inclusive start and exclusive end; an overnight window belongs to its start day.
func WindowAt(r model.Recurrence, now time.Time) (model.Occurrence, bool, error) {
	if err := ValidateRecurrence(r); err != nil {
		return model.Occurrence{}, false, err
	}
	loc, _ := time.LoadLocation(r.Timezone)
	date := civilDate(now.In(loc))
	for back := 0; back <= 1; back++ {
		d := date.AddDate(0, 0, -back)
		if !scheduledDay(r, d) {
			continue
		}
		o := recurrenceOn(r, d)
		if !now.Before(o.Start) && now.Before(o.End) {
			return o, true, nil
		}
	}
	return model.Occurrence{}, false, nil
}

func identity(window string, revision int64, start time.Time) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(fmt.Sprintf("%s/%d/%s", window, revision, start.UTC().Format(time.RFC3339Nano)))))[:24]
}
func trustedReset(a *model.Allowance, now time.Time) bool {
	return a != nil && a.ResetTrusted && !a.ResetsAt.IsZero() && a.ResetsAt.After(now)
}
func epoch(a *model.Allowance) string {
	if a == nil || !a.ResetTrusted {
		return ""
	}
	if a.Epoch != "" {
		return a.Epoch
	}
	return a.StartsAt.UTC().Format(time.RFC3339Nano) + "/" + a.ResetsAt.UTC().Format(time.RFC3339Nano)
}
func resetHours(h float64) bool { return !math.IsNaN(h) && !math.IsInf(h, 0) && h > 0 && h <= 24*366 }

// ResolveTimeWindow resolves a named launch restriction against this subscription's trusted reset.
func ResolveTimeWindow(w model.TimeWindow, a *model.Allowance, now time.Time) (model.Occurrence, bool, error) {
	if !w.Enabled {
		return model.Occurrence{}, false, nil
	}
	var o model.Occurrence
	var active bool
	var err error
	switch w.StartMode {
	case "", "weekly":
		o, active, err = WindowAt(w.Recurrence, now)
	case "reset_relative":
		if !resetHours(w.HoursBeforeReset) {
			return o, false, fmt.Errorf("hours before reset must be greater than zero and at most 8784")
		}
		if !trustedReset(a, now) {
			return o, false, nil
		}
		o.Start = a.ResetsAt.Add(-time.Duration(w.HoursBeforeReset * float64(time.Hour)))
		o.End = a.ResetsAt
		o.Epoch = epoch(a)
		active = !now.Before(o.Start) && now.Before(o.End)
	default:
		return o, false, fmt.Errorf("unknown start mode %q", w.StartMode)
	}
	if active {
		o.WindowID = w.ID
		o.ID = identity(w.ID, w.Revision, o.Start)
	}
	return o, active, err
}

func ValidateOverdrive(w model.OverdriveWindow) error {
	if !percent(w.TargetUsedPercent) {
		return fmt.Errorf("target used must be between 0 and 100")
	}
	switch w.StartMode {
	case "", "weekly":
		if err := ValidateRecurrence(w.Recurrence); err != nil {
			return err
		}
	case "reset_relative":
		if !resetHours(w.HoursBeforeReset) {
			return fmt.Errorf("hours before reset must be greater than zero and at most 8784")
		}
	default:
		return fmt.Errorf("unknown start mode %q", w.StartMode)
	}
	switch w.EndMode {
	case "", "reset", "weekly_reset", "next_reset", "recurrence":
	case "explicit":
		if w.EndDay < 1 || w.EndDay > 7 {
			return fmt.Errorf("end weekday must be Monday=1 through Sunday=7")
		}
		if _, err := clockMinutes(w.EndTime); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown end mode %q", w.EndMode)
	}
	if w.StartMode == "reset_relative" && w.EndMode != "" && w.EndMode != "reset" && w.EndMode != "weekly_reset" && w.EndMode != "next_reset" {
		return fmt.Errorf("reset-relative overdrive ends at its trusted reset")
	}
	return nil
}

// OverdriveAt returns the resolved current occurrence, without deciding target completion.
func OverdriveAt(w model.OverdriveWindow, a *model.Allowance, now time.Time) (model.Occurrence, bool, error) {
	if err := ValidateOverdrive(w); err != nil {
		return model.Occurrence{}, false, err
	}
	if !w.Enabled {
		return model.Occurrence{}, false, nil
	}
	if w.StartMode == "reset_relative" {
		return ResolveTimeWindow(model.TimeWindow{ID: w.ID, Revision: w.Revision, Enabled: true, StartMode: w.StartMode, HoursBeforeReset: w.HoursBeforeReset}, a, now)
	}
	r := w.Recurrence
	loc, _ := time.LoadLocation(r.Timezone)
	date := civilDate(now.In(loc))
	var best model.Occurrence
	for back := 0; back < 8; back++ {
		d := date.AddDate(0, 0, -back)
		if !scheduledDay(r, d) {
			continue
		}
		o := recurrenceOn(r, d)
		switch w.EndMode {
		case "recurrence":
		case "explicit":
			delta := (w.EndDay - dayNumber(d) + 7) % 7
			endDate := d.AddDate(0, 0, delta)
			m, _ := clockMinutes(w.EndTime)
			o.End = civilInstant(endDate, m, loc, true)
			if !o.End.After(o.Start) {
				o.End = civilInstant(endDate.AddDate(0, 0, 7), m, loc, true)
			}
		default:
			if !trustedReset(a, now) {
				continue
			}
			o.End = a.ResetsAt
		}
		// Never carry a weekly occurrence across a known allowance epoch boundary.
		if a != nil && a.ResetTrusted && !a.StartsAt.IsZero() && o.Start.Before(a.StartsAt) {
			continue
		}
		if !now.Before(o.Start) && now.Before(o.End) {
			best = o
			break
		}
	}
	if best.Start.IsZero() {
		return best, false, nil
	}
	best.ID = identity(w.ID, w.Revision, best.Start)
	best.WindowID = w.ID
	best.Epoch = epoch(a)
	return best, true, nil
}
