package campaign

import (
	"strings"
	"testing"
	"time"

	"marquee/internal/store"
)

func TestSpotsSpreadOverOpeningHours(t *testing.T) {
	media := map[int64]*store.Media{
		1: {ID: 1, Kind: "image", SHA256: "shop", Ext: "jpg", Status: "ready"},
		2: {ID: 2, Kind: "video", SHA256: "ad", Ext: "mp4", Status: "ready", DurationMS: 14200},
	}
	// The shop's screen runs 10:00-20:00; the ad is agreed 10 times a day.
	shop := mustRule(t, Spec{Items: []Item{{MediaID: 1, DurationS: 8}}, TimeFrom: "10:00", TimeTo: "20:00"})
	shop.VersionID = 1
	ad := mustRule(t, Spec{Items: []Item{{MediaID: 2}}, PerDay: 10})
	ad.VersionID = 2

	b := Builder{Loc: sofia}
	from := time.Date(2026, 11, 16, 0, 0, 0, 0, sofia)
	segs, versions := b.Plan(from, []Playable{shop, ad}, media)
	if len(versions) != 2 {
		t.Fatalf("versions %v", versions)
	}

	var day []Segment
	for _, s := range segs {
		if s.Start.Before(from.Add(24 * time.Hour)) {
			day = append(day, s)
		}
	}
	var spots []time.Time
	for i, s := range day {
		if s.Items[0].Media.ID != 2 {
			continue
		}
		spots = append(spots, s.Start)
		// One showing, rounded up to whole seconds, then the shop again.
		if s.End.Sub(s.Start) != 15*time.Second {
			t.Errorf("spot length %v", s.End.Sub(s.Start))
		}
		if i+1 >= len(day) || day[i+1].Items[0].Media.ID != 1 || !day[i+1].Start.Equal(s.End) {
			t.Errorf("shop does not resume right after the spot at %v", s.Start)
		}
	}
	// Every 60 minutes, half an hour after each opening hour starts.
	if len(spots) != 10 {
		t.Fatalf("got %d spots on the day: %v", len(spots), spots)
	}
	for i, at := range spots {
		want := time.Date(2026, 11, 16, 10, 30, 0, 0, sofia).Add(time.Duration(i) * time.Hour)
		if !at.Equal(want) {
			t.Errorf("spot %d at %v, want %v", i, at.In(sofia), want)
		}
	}
	// No ad while the screen has nothing to show.
	for _, s := range segs {
		if h := s.Start.In(sofia).Hour(); s.Items[0].Media.ID == 2 && (h < 10 || h >= 20) {
			t.Errorf("spot outside opening hours at %v", s.Start.In(sofia))
		}
	}

	// The SMIL index switches on the player's clock: a par per spot.
	body, _ := Builder{Loc: sofia, RefreshSec: 900, MediaURL: func(m *store.Media) string { return "/m/" + m.FileName() }}.
		Build(from, []Playable{shop, ad}, media)
	if !strings.Contains(string(body), `<par begin="wallclock(2026-11-16T10:30:00+02:00)" end="wallclock(2026-11-16T10:30:15+02:00)">`) {
		t.Errorf("no spot par in\n%.2000s", body)
	}
}

func TestSpotsWithoutContentOrDue(t *testing.T) {
	media := map[int64]*store.Media{
		2: {ID: 2, Kind: "image", SHA256: "ad", Ext: "jpg", Status: "ready"},
	}
	ad := mustRule(t, Spec{Items: []Item{{MediaID: 2, DurationS: 10}}, PerDay: 5})
	ad.VersionID = 2
	// A screen with nothing of its own shows no ads either.
	if segs, versions := (Builder{Loc: sofia}).Plan(time.Now(), []Playable{ad}, media); len(segs) != 0 || len(versions) != 0 {
		t.Fatalf("spots without content: %v %v", segs, versions)
	}
}
