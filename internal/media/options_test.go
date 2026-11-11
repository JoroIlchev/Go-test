package media

import (
	"slices"
	"testing"
)

func TestOptionsFrom(t *testing.T) {
	o, err := OptionsFrom("1080x1920", "high", "always", 720, "small")
	if err != nil || o.ImageW != 1080 || o.ImageH != 1920 || o.ImageQ != 2 || o.VideoAuto != "always" ||
		o.VideoShort != 720 || o.VideoCRF != 25 {
		t.Fatalf("%+v %v", o, err)
	}
	if o, _ := OptionsFrom("off", "normal", "off", 1080, "normal"); !o.fits(9000, 9000) {
		t.Error("off still shrinks")
	}
	for _, bad := range [][]any{{"big", "normal", "off", 1080, "normal"}, {"1920x1080", "max", "off", 1080, "normal"},
		{"1920x1080", "normal", "sometimes", 1080, "normal"}, {"1920x1080", "normal", "off", 480, "normal"}} {
		if _, err := OptionsFrom(bad[0].(string), bad[1].(string), bad[2].(string), bad[3].(int), bad[4].(string)); err == nil {
			t.Errorf("accepted %v", bad)
		}
	}
	// 720p converts landscape into 1280x720 and portrait into 720x1280.
	args := DefaultOptions()
	args.VideoShort = 720
	if a := args.videoArgs("in", "out", true); !slices.ContainsFunc(a, func(s string) bool { return len(s) > 20 && s[:20] == "scale='min(720,iw)':" }) {
		t.Errorf("portrait box: %v", a)
	}
}

func TestSlideshowLoops(t *testing.T) {
	sh := Slideshow{Items: []Slide{{DurationS: 4}, {DurationS: 4}}}
	for repeat, want := range map[int]int{0: 0, 8: 0, 9: 1, 60: 7} {
		sh.RepeatToS = repeat
		if got := sh.Loops(); got != want {
			t.Errorf("repeat to %d: %d loops, want %d", repeat, got, want)
		}
	}
}
