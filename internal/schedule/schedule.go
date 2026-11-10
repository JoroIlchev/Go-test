// Package schedule turns campaign schedules (date range, weekdays, daily time
// window, priority) into a flat timeline of segments: for every stretch of
// time, which campaigns play. The SMIL builder turns that timeline into
// wallclock-timed blocks so a player switches content on its own clock.
package schedule

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Rule is the scheduling part of a campaign version.
type Rule struct {
	Start    time.Time // zero = already running
	End      time.Time // zero = open ended
	Days     []int     // ISO weekdays 1 (Mon) .. 7 (Sun); empty = every day
	TimeFrom string    // "HH:MM"; empty = whole day
	TimeTo   string    // "HH:MM"; earlier than TimeFrom = runs past midnight
	Priority int       // higher wins; equal priorities share the screen
}

type Interval struct {
	Start, End time.Time
}

// ParseClock parses "HH:MM" into minutes after midnight.
func ParseClock(s string) (int, error) {
	h, m, ok := strings.Cut(s, ":")
	hh, err1 := strconv.Atoi(h)
	mm, err2 := strconv.Atoi(m)
	if !ok || err1 != nil || err2 != nil || hh < 0 || hh > 24 || mm < 0 || mm > 59 || (hh == 24 && mm != 0) {
		return 0, fmt.Errorf("invalid time %q, want HH:MM", s)
	}
	return hh*60 + mm, nil
}

func (r Rule) Validate() error {
	if !r.Start.IsZero() && !r.End.IsZero() && !r.End.After(r.Start) {
		return fmt.Errorf("end must be after start")
	}
	for _, d := range r.Days {
		if d < 1 || d > 7 {
			return fmt.Errorf("invalid weekday %d", d)
		}
	}
	if (r.TimeFrom == "") != (r.TimeTo == "") {
		return fmt.Errorf("set both time_from and time_to, or neither")
	}
	if r.TimeFrom != "" {
		from, err := ParseClock(r.TimeFrom)
		if err != nil {
			return err
		}
		to, err := ParseClock(r.TimeTo)
		if err != nil {
			return err
		}
		if from == to {
			return fmt.Errorf("time_from and time_to must differ")
		}
	}
	return nil
}

// Expand returns the rule's active intervals within [from, to), merged where
// they touch. Day boundaries and clock times are taken in loc, so a window of
// 08:00-20:00 stays 08:00-20:00 local across DST changes.
func (r Rule) Expand(from, to time.Time, loc *time.Location) []Interval {
	lo, hi := from, to
	if !r.Start.IsZero() && r.Start.After(lo) {
		lo = r.Start
	}
	if !r.End.IsZero() && r.End.Before(hi) {
		hi = r.End
	}
	if !hi.After(lo) {
		return nil
	}
	days := map[int]bool{}
	for _, d := range r.Days {
		days[d] = true
	}
	fromMin, toMin := 0, 24*60
	if r.TimeFrom != "" {
		fromMin, _ = ParseClock(r.TimeFrom)
		toMin, _ = ParseClock(r.TimeTo)
		if toMin <= fromMin {
			toMin += 24 * 60 // past midnight into the next day
		}
	}

	var out []Interval
	// Start a day early: yesterday's window may run past midnight into lo.
	l := lo.In(loc)
	day := time.Date(l.Year(), l.Month(), l.Day()-1, 0, 0, 0, 0, loc)
	for !day.After(hi) {
		wd := int(day.Weekday())
		if wd == 0 {
			wd = 7
		}
		if len(days) == 0 || days[wd] {
			s := atMinute(day, fromMin, loc)
			e := atMinute(day, toMin, loc)
			if s.Before(lo) {
				s = lo
			}
			if e.After(hi) {
				e = hi
			}
			if e.After(s) {
				if n := len(out); n > 0 && !s.After(out[n-1].End) {
					if e.After(out[n-1].End) {
						out[n-1].End = e
					}
				} else {
					out = append(out, Interval{s, e})
				}
			}
		}
		day = time.Date(day.Year(), day.Month(), day.Day()+1, 0, 0, 0, 0, loc)
	}
	return out
}

// atMinute is midnight of day plus minutes as local wall-clock time
// (time.Date normalises minute overflow into the next day).
func atMinute(day time.Time, minutes int, loc *time.Location) time.Time {
	return time.Date(day.Year(), day.Month(), day.Day(), 0, minutes, 0, 0, loc)
}

// Entry is one scheduled campaign version going into a timeline.
type Entry[T any] struct {
	Rule  Rule
	Value T
}

// Segment is a stretch of time with the entries that play during it, in the
// order they were given. End is zero for the open-ended last segment.
type Segment[T any] struct {
	Start, End time.Time
	Values     []T
}

// Timeline flattens entries into non-overlapping segments within [from, to).
// Only the highest-priority entries active at a moment play; equal priorities
// share the segment. Stretches where nothing plays are left out.
func Timeline[T any](entries []Entry[T], from, to time.Time, loc *time.Location) []Segment[T] {
	type span struct {
		iv  Interval
		idx int
	}
	var spans []span
	cuts := map[int64]time.Time{from.UnixNano(): from, to.UnixNano(): to}
	for i, e := range entries {
		for _, iv := range e.Rule.Expand(from, to, loc) {
			spans = append(spans, span{iv, i})
			cuts[iv.Start.UnixNano()] = iv.Start
			cuts[iv.End.UnixNano()] = iv.End
		}
	}
	points := make([]time.Time, 0, len(cuts))
	for _, t := range cuts {
		points = append(points, t)
	}
	sort.Slice(points, func(i, j int) bool { return points[i].Before(points[j]) })

	var out []Segment[T]
	var lastIdx []int
	for i := 0; i+1 < len(points); i++ {
		s, e := points[i], points[i+1]
		best := -1 << 31
		var idx []int
		for _, sp := range spans {
			if sp.iv.Start.After(s) || !sp.iv.End.After(s) {
				continue
			}
			p := entries[sp.idx].Rule.Priority
			if p > best {
				best, idx = p, nil
			}
			if p == best {
				idx = append(idx, sp.idx)
			}
		}
		sort.Ints(idx)
		if len(idx) == 0 {
			lastIdx = nil
			continue
		}
		if n := len(out); n > 0 && out[n-1].End.Equal(s) && equalInts(idx, lastIdx) {
			out[n-1].End = e
			continue
		}
		vals := make([]T, len(idx))
		for k, j := range idx {
			vals[k] = entries[j].Value
		}
		out = append(out, Segment[T]{Start: s, End: e, Values: vals})
		lastIdx = idx
	}
	// A segment reaching the horizon keeps playing until the next refresh.
	if n := len(out); n > 0 && out[n-1].End.Equal(to) {
		out[n-1].End = time.Time{}
	}
	return out
}

func equalInts(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
