package campaign

import (
	"math"
	"sort"
	"time"

	"marquee/internal/schedule"
	"marquee/internal/store"
)

// Spots are the hub operator's own ads: a campaign with PerDay plays that
// many times a day, cutting into whatever the screen shows at the time. The
// times are spread evenly over the hours the screen has content that day
// (within the ad's own schedule), so a shop open 8 hours and one open 16
// both get the agreed number. The same content gives the same times, which
// keeps the index stable between polls.
func (b Builder) insertSpots(segs []Segment, spots []*Playable, from, to time.Time, media map[int64]*store.Media) []Segment {
	if len(spots) == 0 || len(segs) == 0 {
		return segs
	}
	end := func(s Segment) time.Time {
		if s.End.IsZero() {
			return to
		}
		return s.End
	}
	var breaks []Segment
	for _, p := range spots {
		items, audio := b.playlist(p, from, media)
		length := spotLength(items)
		if length <= 0 {
			continue
		}
		allowed := p.Rule.Expand(from, to, b.Loc)
		for day := from; day.Before(to); {
			next := nextDay(day, b.Loc)
			// The stretches of this day with content where the ad may play.
			var covered []schedule.Interval
			var total time.Duration
			for _, s := range segs {
				if len(s.Items) == 0 {
					continue
				}
				for _, a := range allowed {
					lo := latest(s.Start, a.Start, day)
					hi := earliest(end(s), a.End, next)
					if hi.After(lo) {
						covered = append(covered, schedule.Interval{Start: lo, End: hi})
						total += hi.Sub(lo)
					}
				}
			}
			for i := 0; total > 0 && i < p.Spec.PerDay; i++ {
				at := pointAt(covered, time.Duration(float64(total)*(float64(i)+0.5)/float64(p.Spec.PerDay)))
				at = at.Truncate(time.Second)
				breaks = append(breaks, Segment{Start: at, End: at.Add(length), Items: items, Audio: audio,
					versions: []int64{p.VersionID}})
			}
			day = next
		}
	}
	sort.SliceStable(breaks, func(i, j int) bool { return breaks[i].Start.Before(breaks[j].Start) })
	// Two ads due at once play one after the other.
	for i := 1; i < len(breaks); i++ {
		if d := breaks[i-1].End.Sub(breaks[i].Start); d > 0 {
			breaks[i].Start = breaks[i].Start.Add(d)
			breaks[i].End = breaks[i].End.Add(d)
		}
	}
	for _, br := range breaks {
		segs = carve(segs, br, to)
	}
	return segs
}

// spotLength is how long one showing of the ad takes, in whole seconds so
// the wallclock times stay exact.
func spotLength(items []PlanItem) time.Duration {
	sec := 0.0
	for _, it := range items {
		switch {
		case it.DurationS > 0:
			sec += it.DurationS
		case it.Media.DurationMS > 0:
			sec += float64(it.Media.DurationMS) / 1000
		default:
			sec += 10
		}
	}
	return time.Duration(math.Ceil(sec)) * time.Second
}

// carve puts br into the sorted segments, cutting the ones it overlaps.
// Whatever was playing starts again from its first item after the ad.
func carve(segs []Segment, br Segment, to time.Time) []Segment {
	out := make([]Segment, 0, len(segs)+2)
	placed := false
	place := func() {
		if !placed {
			out = append(out, br)
			placed = true
		}
	}
	for _, s := range segs {
		e := s.End
		if e.IsZero() {
			e = to
		}
		if !s.Start.Before(br.End) {
			place()
			out = append(out, s)
			continue
		}
		if !e.After(br.Start) {
			out = append(out, s)
			continue
		}
		if s.Start.Before(br.Start) {
			head := s
			head.End = br.Start
			out = append(out, head)
		}
		place()
		if e.After(br.End) {
			tail := s
			tail.Start = br.End
			out = append(out, tail)
		}
	}
	place()
	return out
}

// pointAt is the moment offset d into the covered stretches.
func pointAt(covered []schedule.Interval, d time.Duration) time.Time {
	for _, iv := range covered {
		if l := iv.End.Sub(iv.Start); d < l {
			return iv.Start.Add(d)
		} else {
			d -= l
		}
	}
	return covered[len(covered)-1].End
}

func nextDay(day time.Time, loc *time.Location) time.Time {
	l := day.In(loc)
	return time.Date(l.Year(), l.Month(), l.Day()+1, 0, 0, 0, 0, loc)
}

func latest(ts ...time.Time) time.Time {
	out := ts[0]
	for _, t := range ts[1:] {
		if t.After(out) {
			out = t
		}
	}
	return out
}

func earliest(ts ...time.Time) time.Time {
	out := ts[0]
	for _, t := range ts[1:] {
		if t.Before(out) {
			out = t
		}
	}
	return out
}
