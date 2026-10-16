package budget

import (
	"fmt"
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
