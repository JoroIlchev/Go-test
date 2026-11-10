package poll

import (
	"testing"
	"time"
)

func TestNext(t *testing.T) {
	sofia, _ := time.LoadLocation("Europe/Sofia")
	at := func(s string) time.Time {
		v, err := time.ParseInLocation("2006-01-02 15:04:05", s, sofia)
		if err != nil {
			t.Fatal(err)
		}
		return v
	}
	twice := Policy{Mode: "times", Times: []string{"12:00", "0:00"}}
	if err := twice.Normalize(); err != nil || twice.Times[0] != "00:00" {
		t.Fatalf("normalize: %v %v", err, twice.Times)
	}

	cases := []struct {
		name string
		p    Policy
		now  string
		want string
	}{
		{"before noon", twice, "2026-11-10 09:30:00", "2026-11-10 12:00:00"},
		{"after noon -> midnight", twice, "2026-11-10 12:00:30", "2026-11-11 00:00:00"},
		{"within the minimum refresh", twice, "2026-11-10 11:59:30", "2026-11-11 00:00:00"},
		{"interval", Policy{Mode: "interval", Minutes: 15}, "2026-11-10 09:30:00", "2026-11-10 09:45:00"},
		// 2026-10-25 04:00 EEST falls back to 03:00 EET: wall-clock times hold.
		{"across DST", Policy{Mode: "times", Times: []string{"06:00"}}, "2026-10-24 07:00:00", "2026-10-25 06:00:00"},
	}
	for _, c := range cases {
		if got := c.p.Next(at(c.now), sofia, 1); !got.Equal(at(c.want)) {
			t.Errorf("%s: got %s, want %s", c.name, got.In(sofia), c.want)
		}
	}
}

func TestSpreadIsStableAndBounded(t *testing.T) {
	sofia, _ := time.LoadLocation("Europe/Sofia")
	p := Policy{Mode: "times", Times: []string{"00:00"}, SpreadMin: 10}
	now := time.Date(2026, 11, 10, 15, 0, 0, 0, sofia)
	midnight := time.Date(2026, 11, 11, 0, 0, 0, 0, sofia)
	seen := map[time.Time]bool{}
	for id := int64(1); id <= 200; id++ {
		n := p.Next(now, sofia, id)
		if n.Before(midnight) || !n.Before(midnight.Add(10*time.Minute)) {
			t.Fatalf("device %d at %s, outside the window", id, n)
		}
		if !n.Equal(p.Next(now.Add(time.Hour), sofia, id)) {
			t.Fatalf("device %d moves around", id)
		}
		seen[n] = true
	}
	if len(seen) < 100 {
		t.Errorf("200 devices share only %d moments", len(seen))
	}
}

func TestNormalizeRejects(t *testing.T) {
	for name, p := range map[string]Policy{
		"mode":     {Mode: "sometimes"},
		"zero":     {Mode: "interval"},
		"no times": {Mode: "times"},
		"bad time": {Mode: "times", Times: []string{"25:00"}},
		"spread":   {Mode: "times", Times: []string{"12:00"}, SpreadMin: 500},
	} {
		if err := p.Normalize(); err == nil {
			t.Errorf("%s accepted", name)
		}
	}
}

func TestRefreshSecMinimum(t *testing.T) {
	p := Policy{Mode: "interval", Minutes: 1}
	if got := p.RefreshSec(time.Now(), time.UTC, 1); got != 60 {
		t.Errorf("got %d", got)
	}
}
