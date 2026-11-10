package schedule

import (
	"testing"
	"time"
	_ "time/tzdata"
)

var sofia, _ = time.LoadLocation("Europe/Sofia")

func at(s string) time.Time {
	t, err := time.ParseInLocation("2006-01-02 15:04", s, sofia)
	if err != nil {
		panic(err)
	}
	return t
}

func fmtIv(ivs []Interval) []string {
	var out []string
	for _, iv := range ivs {
		out = append(out, iv.Start.In(sofia).Format("01-02 15:04")+" → "+iv.End.In(sofia).Format("01-02 15:04"))
	}
	return out
}

func check(t *testing.T, got []Interval, want ...string) {
	t.Helper()
	g := fmtIv(got)
	if len(g) != len(want) {
		t.Fatalf("got %v, want %v", g, want)
	}
	for i := range g {
		if g[i] != want[i] {
			t.Fatalf("got %v, want %v", g, want)
		}
	}
}

func TestExpandAllDayMergesAcrossDays(t *testing.T) {
	r := Rule{Start: at("2026-11-11 00:00")}
	check(t, r.Expand(at("2026-11-10 12:00"), at("2026-11-14 00:00"), sofia), "11-11 00:00 → 11-14 00:00")
}

func TestExpandDailyWindowAndWeekdays(t *testing.T) {
	// Lunch menu, weekdays 11:00-14:00. 2026-11-13 is a Friday.
	r := Rule{Days: []int{1, 2, 3, 4, 5}, TimeFrom: "11:00", TimeTo: "14:00"}
	check(t, r.Expand(at("2026-11-13 12:30"), at("2026-11-17 00:00"), sofia),
		"11-13 12:30 → 11-13 14:00", // already running when the horizon starts
		"11-16 11:00 → 11-16 14:00") // weekend skipped
}

func TestExpandPastMidnight(t *testing.T) {
	// Bar promo 22:00-02:00, Fridays only: runs into Saturday morning.
	r := Rule{Days: []int{5}, TimeFrom: "22:00", TimeTo: "02:00"}
	check(t, r.Expand(at("2026-11-14 01:00"), at("2026-11-21 06:00"), sofia),
		"11-14 01:00 → 11-14 02:00", // Friday 13th's window, seen from Saturday 01:00
		"11-20 22:00 → 11-21 02:00")
}

func TestExpandAcrossDSTKeepsLocalClock(t *testing.T) {
	// Sofia leaves summer time on 2026-10-25 at 04:00 local.
	r := Rule{TimeFrom: "08:00", TimeTo: "20:00"}
	got := r.Expand(at("2026-10-24 00:00"), at("2026-10-26 00:00"), sofia)
	check(t, got, "10-24 08:00 → 10-24 20:00", "10-25 08:00 → 10-25 20:00")
	if _, off := got[0].Start.Zone(); off != 3*3600 {
		t.Fatalf("summer offset: %d", off)
	}
	if _, off := got[1].Start.Zone(); off != 2*3600 {
		t.Fatalf("winter offset: %d", off)
	}
}

func TestTimelinePriorityAndSharing(t *testing.T) {
	from, to := at("2026-11-10 00:00"), at("2026-11-17 00:00")
	entries := []Entry[string]{
		{Rule: Rule{}, Value: "base"},
		{Rule: Rule{Start: at("2026-11-11 00:00"), End: at("2026-11-12 00:00"), Priority: 1}, Value: "black-friday"},
		{Rule: Rule{}, Value: "base2"},
	}
	segs := Timeline(entries, from, to, sofia)
	if len(segs) != 3 {
		t.Fatalf("segments: %+v", segs)
	}
	if v := segs[0].Values; len(v) != 2 || v[0] != "base" || v[1] != "base2" {
		t.Fatalf("equal priorities share: %v", v)
	}
	if v := segs[1].Values; len(v) != 1 || v[0] != "black-friday" || !segs[1].Start.Equal(at("2026-11-11 00:00")) {
		t.Fatalf("priority wins at exactly 00:00: %+v", segs[1])
	}
	if !segs[2].End.IsZero() {
		t.Fatalf("last segment should be open ended: %+v", segs[2])
	}
}

func TestTimelineGapsAreDropped(t *testing.T) {
	from, to := at("2026-11-16 00:00"), at("2026-11-18 00:00")
	entries := []Entry[string]{{Rule: Rule{TimeFrom: "11:00", TimeTo: "14:00"}, Value: "lunch"}}
	segs := Timeline(entries, from, to, sofia)
	if len(segs) != 2 || !segs[0].End.Equal(at("2026-11-16 14:00")) || !segs[1].Start.Equal(at("2026-11-17 11:00")) {
		t.Fatalf("segments: %+v", segs)
	}
}

func TestValidate(t *testing.T) {
	bad := []Rule{
		{Start: at("2026-11-12 00:00"), End: at("2026-11-11 00:00")},
		{Days: []int{0}},
		{TimeFrom: "10:00"},
		{TimeFrom: "25:00", TimeTo: "26:00"},
		{TimeFrom: "10:00", TimeTo: "10:00"},
	}
	for i, r := range bad {
		if r.Validate() == nil {
			t.Errorf("rule %d should be invalid: %+v", i, r)
		}
	}
	if err := (Rule{Days: []int{1, 7}, TimeFrom: "22:00", TimeTo: "02:00"}).Validate(); err != nil {
		t.Fatal(err)
	}
}
