package iterrun

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Package iterrun, schedule: the launch-schedule grammar /iterate-rules
// writes into policy.md, evaluated in Go so the nightly tick can decide
// "not now" without paying a cold `claude -p` start just to be refused.
//
// A rule is `<allow|deny> [days] [HH:MM-HH:MM] [dates]`; every component
// is optional and an omitted one matches everything. Deny beats allow,
// and the presence of any allow rule makes the schedule default-deny, so
// an edit can only ever narrow when runs happen. Two semantics are easy
// to get wrong and are the reason this file has tests: a window whose
// start is later than its end WRAPS MIDNIGHT (22:00-06:00 is 22:00
// through 05:59 the next morning), and a day label matches the day the
// window OPENED, so `allow mon-fri 22:00-06:00` includes Saturday 02:00.

// ScheduleRule is one parsed rule line.
type ScheduleRule struct {
	Allow bool
	// Days is indexed by time.Weekday; every entry is true when the rule
	// names no days.
	Days    [7]bool
	HasTime bool
	Start   int // minutes after midnight, inclusive
	End     int // minutes after midnight, exclusive; End <= Start wraps past midnight
	// From/To are civil dates (year, month, day only) when HasDates.
	HasDates bool
	From, To time.Time
	Raw      string
}

// Schedule is one ruleset, named so a refusal can say which file decided.
type Schedule struct {
	Name  string
	Rules []ScheduleRule
}

// Empty reports a schedule with no rules — always allowed.
func (s Schedule) Empty() bool { return len(s.Rules) == 0 }

var (
	reTimeWindow = regexp.MustCompile(`^(\d{1,2}):(\d{2})-(\d{1,2}):(\d{2})$`)
	reDateSpec   = regexp.MustCompile(`^(\d{4}-\d{2}-\d{2})(?:\.\.(\d{4}-\d{2}-\d{2}))?$`)
)

var dayNames = map[string]time.Weekday{
	"sun": time.Sunday, "sunday": time.Sunday,
	"mon": time.Monday, "monday": time.Monday,
	"tue": time.Tuesday, "tues": time.Tuesday, "tuesday": time.Tuesday,
	"wed": time.Wednesday, "wednesday": time.Wednesday,
	"thu": time.Thursday, "thur": time.Thursday, "thurs": time.Thursday, "thursday": time.Thursday,
	"fri": time.Friday, "friday": time.Friday,
	"sat": time.Saturday, "saturday": time.Saturday,
}

// ParseSchedule parses rule lines (a leading "- " list marker and a
// trailing "# comment" are tolerated; blank lines are skipped). Any
// malformed line is an error naming that line — the caller fails closed,
// because a schedule that half-parses would silently widen the window
// its author meant to narrow.
func ParseSchedule(name string, lines ...string) (Schedule, error) {
	s := Schedule{Name: name}
	for _, raw := range lines {
		line := stripComment(strings.TrimPrefix(strings.TrimSpace(raw), "- "))
		if line == "" {
			continue
		}
		r, err := parseRule(line)
		if err != nil {
			return Schedule{Name: name}, fmt.Errorf("%s: malformed rule %q: %v", name, line, err)
		}
		s.Rules = append(s.Rules, r)
	}
	return s, nil
}

func parseRule(line string) (ScheduleRule, error) {
	fields := strings.Fields(line)
	r := ScheduleRule{Raw: line}
	switch strings.ToLower(fields[0]) {
	case "allow":
		r.Allow = true
	case "deny":
		r.Allow = false
	default:
		return r, fmt.Errorf("must start with allow or deny")
	}
	for i := range r.Days {
		r.Days[i] = true
	}
	seenDays, seenTime, seenDates := false, false, false
	for _, tok := range fields[1:] {
		tok = strings.ToLower(tok)
		switch {
		case reTimeWindow.MatchString(tok):
			if seenTime {
				return r, fmt.Errorf("two time windows")
			}
			start, end, err := parseWindow(tok)
			if err != nil {
				return r, err
			}
			r.HasTime, r.Start, r.End, seenTime = true, start, end, true
		case reDateSpec.MatchString(tok):
			if seenDates {
				return r, fmt.Errorf("two date ranges")
			}
			from, to, err := parseDates(tok)
			if err != nil {
				return r, err
			}
			r.HasDates, r.From, r.To, seenDates = true, from, to, true
		default:
			days, ok := parseDays(tok)
			if !ok {
				return r, fmt.Errorf("unknown component %q", tok)
			}
			if seenDays {
				return r, fmt.Errorf("two day lists")
			}
			r.Days, seenDays = days, true
		}
	}
	return r, nil
}

func parseWindow(tok string) (start, end int, err error) {
	m := reTimeWindow.FindStringSubmatch(tok)
	h1, _ := strconv.Atoi(m[1])
	m1, _ := strconv.Atoi(m[2])
	h2, _ := strconv.Atoi(m[3])
	m2, _ := strconv.Atoi(m[4])
	if h1 > 23 || h2 > 23 || m1 > 59 || m2 > 59 {
		return 0, 0, fmt.Errorf("time out of range in %q", tok)
	}
	return h1*60 + m1, h2*60 + m2, nil
}

func parseDates(tok string) (from, to time.Time, err error) {
	m := reDateSpec.FindStringSubmatch(tok)
	from, err = time.Parse("2006-01-02", m[1])
	if err != nil {
		return from, to, fmt.Errorf("bad date %q", m[1])
	}
	to = from
	if m[2] != "" {
		if to, err = time.Parse("2006-01-02", m[2]); err != nil {
			return from, to, fmt.Errorf("bad date %q", m[2])
		}
	}
	if to.Before(from) {
		return from, to, fmt.Errorf("date range %q ends before it starts", tok)
	}
	return from, to, nil
}

func parseDays(tok string) ([7]bool, bool) {
	var days [7]bool
	switch tok {
	case "daily", "everyday", "every-day":
		for i := range days {
			days[i] = true
		}
		return days, true
	case "weekdays":
		for d := time.Monday; d <= time.Friday; d++ {
			days[d] = true
		}
		return days, true
	case "weekends":
		days[time.Saturday], days[time.Sunday] = true, true
		return days, true
	}
	any := false
	for _, part := range strings.Split(tok, ",") {
		if part == "" {
			return days, false
		}
		if a, b, isRange := strings.Cut(part, "-"); isRange {
			da, okA := dayNames[a]
			db, okB := dayNames[b]
			if !okA || !okB {
				return days, false
			}
			for d := da; ; d = (d + 1) % 7 {
				days[d] = true
				if d == db {
					break
				}
			}
			any = true
			continue
		}
		d, ok := dayNames[part]
		if !ok {
			return days, false
		}
		days[d] = true
		any = true
	}
	return days, any
}

func civilDate(t time.Time) time.Time {
	return time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
}

// matchesAt reports whether the rule covers instant t. The window that
// contains t must have OPENED on a day the rule names and on a date it
// covers; a window with Start >= End spans midnight, so an early-morning t
// belongs to the previous calendar day's window.
func (r ScheduleRule) matchesAt(t time.Time) bool {
	minute := t.Hour()*60 + t.Minute()
	opened := t
	if r.HasTime {
		if r.Start < r.End {
			if minute < r.Start || minute >= r.End {
				return false
			}
		} else {
			switch {
			case minute >= r.Start:
				// opened today
			case minute < r.End:
				opened = t.AddDate(0, 0, -1)
			default:
				return false
			}
		}
	}
	if !r.Days[opened.Weekday()] {
		return false
	}
	if r.HasDates {
		d := civilDate(opened)
		if d.Before(r.From) || d.After(r.To) {
			return false
		}
	}
	return true
}

// decide applies the evaluation order: any matching deny refuses; else any
// matching allow permits; else allow rules exist → refuse; no rules → permit.
func (s Schedule) decide(now time.Time) (ok bool, rule string) {
	for _, r := range s.Rules {
		if !r.Allow && r.matchesAt(now) {
			return false, r.Raw
		}
	}
	hasAllow := false
	for _, r := range s.Rules {
		if !r.Allow {
			continue
		}
		hasAllow = true
		if r.matchesAt(now) {
			return true, r.Raw
		}
	}
	if hasAllow {
		return false, "no allow rule matches"
	}
	return true, ""
}

// Allowed evaluates one schedule at now. rule is the line that decided
// (empty when no rule applied); next is the first later minute the
// schedule allows, or the zero time when a year's scan finds none.
func (s Schedule) Allowed(now time.Time) (ok bool, rule string, next time.Time) {
	return AllowedAll(now, s)
}

// AllowedAll requires every schedule to allow now — the conductor's own
// schedule is intersected with the project's launch schedule this way, so
// it can narrow the hours and never widen them. rule names the schedule
// and the line that refused; next is the first later minute at which every
// schedule allows (zero when none within a year).
func AllowedAll(now time.Time, schedules ...Schedule) (ok bool, rule string, next time.Time) {
	for _, s := range schedules {
		if ok, r := s.decide(now); !ok {
			rule = s.Name + ": " + r
			return false, rule, nextAllowedAll(now, schedules)
		}
	}
	return true, "", time.Time{}
}

func nextAllowedAll(now time.Time, schedules []Schedule) time.Time {
	t := now.Truncate(time.Minute).Add(time.Minute)
	for end := t.AddDate(1, 0, 0); t.Before(end); t = t.Add(time.Minute) {
		all := true
		for _, s := range schedules {
			if ok, _ := s.decide(t); !ok {
				all = false
				break
			}
		}
		if all {
			return t
		}
	}
	return time.Time{}
}

// frontmatter is the minimal reading of a `---`-fenced key block the
// iterate files use: `key: scalar` lines and `key:` followed by indented
// `- item` lines. Trailing `# comments` are dropped from both.
type frontmatter struct {
	scalars map[string]string
	lists   map[string][]string
}

func stripComment(s string) string {
	if i := strings.Index(s, "#"); i >= 0 {
		s = s[:i]
	}
	return strings.TrimSpace(s)
}

func readFrontmatter(path string) frontmatter {
	fm := frontmatter{scalars: map[string]string{}, lists: map[string][]string{}}
	data, err := os.ReadFile(path)
	if err != nil {
		return fm
	}
	in, cur := false, ""
	for _, raw := range strings.Split(string(data), "\n") {
		line := strings.TrimRight(raw, " \t\r")
		if strings.TrimSpace(line) == "---" {
			if !in {
				in = true
				continue
			}
			break
		}
		if !in {
			continue
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			item := strings.TrimSpace(line)
			if cur != "" && strings.HasPrefix(item, "- ") {
				if v := stripComment(strings.TrimPrefix(item, "- ")); v != "" {
					fm.lists[cur] = append(fm.lists[cur], v)
				}
			}
			continue
		}
		cur = ""
		k, v, ok := strings.Cut(line, ":")
		if !ok || strings.TrimSpace(k) == "" {
			continue
		}
		k = strings.TrimSpace(k)
		fm.scalars[k] = stripComment(v)
		cur = k
	}
	return fm
}

// ReadLaunchSchedule reads `launch-schedule:` — or the `launch-window:
// HH:MM-HH:MM` shorthand, which means `allow daily <window>` — from
// <projectDir>/.claude/iterate/policy.md. No file or no key is an empty
// schedule (always allowed). A malformed line is an error; callers treat
// that as refused and name the line.
func ReadLaunchSchedule(projectDir string) (Schedule, error) {
	fm := readFrontmatter(filepath.Join(projectDir, ".claude", "iterate", "policy.md"))
	lines := fm.lists["launch-schedule"]
	if len(lines) == 0 {
		if w := fm.scalars["launch-window"]; w != "" {
			lines = []string{"allow daily " + w}
		}
	}
	return ParseSchedule("launch-schedule", lines...)
}

// ReadConductorSchedule reads `conductor-schedule:` from
// <projectDir>/.claude/iterate/conductor.md — same grammar, intersected
// with the launch schedule by AllowedAll so it only ever narrows.
func ReadConductorSchedule(projectDir string) (Schedule, error) {
	fm := readFrontmatter(filepath.Join(projectDir, ".claude", "iterate", "conductor.md"))
	return ParseSchedule("conductor-schedule", fm.lists["conductor-schedule"]...)
}
