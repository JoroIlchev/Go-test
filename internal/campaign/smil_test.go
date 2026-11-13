package campaign

import (
	"strings"
	"testing"
	"time"
	_ "time/tzdata"

	"marquee/internal/store"
)

var sofia, _ = time.LoadLocation("Europe/Sofia")

func mustRule(t *testing.T, s Spec) Playable {
	t.Helper()
	r, err := s.Rule(sofia)
	if err != nil {
		t.Fatal(err)
	}
	return Playable{Spec: s, Rule: r}
}

func TestBuildBlackFridaySwitch(t *testing.T) {
	media := map[int64]*store.Media{
		1: {ID: 1, Kind: "image", SHA256: "aaa", Ext: "jpg", Status: "ready"},
		2: {ID: 2, Kind: "video", SHA256: "bbb", Ext: "mp4", Status: "ready"},
		3: {ID: 3, Kind: "audio", SHA256: "ccc", Ext: "mp3", Status: "ready"},
		4: {ID: 4, Kind: "image", SHA256: "ddd", Ext: "jpg", Status: "processing"},
	}
	base := mustRule(t, Spec{Items: []Item{{MediaID: 1, DurationS: 8}, {MediaID: 4, DurationS: 5}}, AudioID: 3})
	base.VersionID = 10
	bf := mustRule(t, Spec{Items: []Item{{MediaID: 2}}, Start: "2026-11-11T00:00", End: "2026-11-12T00:00", Priority: 1, Fit: "fill"})
	bf.VersionID = 20

	b := Builder{Loc: sofia, RefreshSec: 900, MediaURL: func(m *store.Media) string { return "https://hub/m/" + m.FileName() }}
	now := time.Date(2026, 11, 10, 15, 0, 0, 0, sofia)
	body, versions := b.Build(now, []Playable{base, bf}, media)
	s := string(body)

	for _, want := range []string{
		`<meta http-equiv="Refresh" content="900"/>`,
		// base plays now, until Black Friday starts at local midnight
		`<par begin="0" end="wallclock(2026-11-11T00:00:00+02:00)">`,
		`<img src="https://hub/m/aaa.jpg" region="screen" dur="8s" fit="meet"/>`,
		`<audio src="https://hub/m/ccc.mp3" repeatCount="indefinite" soundLevel="100%"/>`,
		// Black Friday takes over on the player's own clock
		`<par begin="wallclock(2026-11-11T00:00:00+02:00)" end="wallclock(2026-11-12T00:00:00+02:00)">`,
		`<video src="https://hub/m/bbb.mp4" region="screen" fit="fill" soundLevel="100%"/>`,
		// and base resumes afterwards, open ended
		`<par begin="wallclock(2026-11-12T00:00:00+02:00)">`,
	} {
		if !strings.Contains(s, want) {
			t.Errorf("missing %s\n%s", want, s)
		}
	}
	// With music the videos stay silent; without it they play their own sound.
	music := mustRule(t, Spec{Items: []Item{{MediaID: 2}}, AudioID: 3})
	music.VersionID = 30
	if body, _ := b.Build(now, []Playable{music}, media); !strings.Contains(string(body),
		`<video src="https://hub/m/bbb.mp4" region="screen" fit="meet" soundLevel="0%"/>`) {
		t.Errorf("video under music not muted:\n%s", body)
	}
	if strings.Contains(s, "ddd.jpg") {
		t.Error("media still processing must not be sent")
	}
	if len(versions) != 2 {
		t.Errorf("versions: %v", versions)
	}

	// Same input later the same day gives the same body (stable ETag), so
	// polling players get 304s instead of reloading.
	again, _ := b.Build(now.Add(2*time.Hour), []Playable{base, bf}, media)
	if string(again) != s {
		t.Error("body changed within the day without a content change")
	}
}

func TestBuildNothingScheduled(t *testing.T) {
	b := Builder{Loc: sofia, RefreshSec: 60, MediaURL: func(m *store.Media) string { return "" }}
	body, versions := b.Build(time.Now(), nil, nil)
	if !strings.Contains(string(body), "<excl>") || len(versions) != 0 {
		t.Fatalf("%s", body)
	}
}
