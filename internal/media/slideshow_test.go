package media

import (
	"context"
	"image"
	"image/color"
	"image/png"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSlideshowNormalize(t *testing.T) {
	ok := Slideshow{Items: []Slide{{MediaID: 1, DurationS: 5}, {MediaID: 2, DurationS: 5}}, Transition: "fade", TransitionS: 1}
	if err := ok.Normalize(); err != nil || ok.Background != "blur" {
		t.Fatalf("valid recipe: %v %+v", err, ok)
	}
	one := Slideshow{Items: []Slide{{MediaID: 1, DurationS: 5}}, Transition: "fade", TransitionS: 1}
	if err := one.Normalize(); err != nil || one.Transition != "none" || one.TransitionS != 0 {
		t.Fatalf("a single slide has no transition: %v %+v", err, one)
	}
	for name, bad := range map[string]Slideshow{
		"empty":                {},
		"unknown transition":   {Items: []Slide{{MediaID: 1, DurationS: 5}, {MediaID: 2, DurationS: 5}}, Transition: "explode"},
		"slide shorter than T": {Items: []Slide{{MediaID: 1, DurationS: 5}, {MediaID: 2, DurationS: 1}}, Transition: "fade", TransitionS: 1},
		"zero duration":        {Items: []Slide{{MediaID: 1, DurationS: 0}}},
		"too long":             {Items: []Slide{{MediaID: 1, DurationS: 120}, {MediaID: 2, DurationS: 120}, {MediaID: 3, DurationS: 120}, {MediaID: 4, DurationS: 120}, {MediaID: 5, DurationS: 120}, {MediaID: 6, DurationS: 120}, {MediaID: 7, DurationS: 120}, {MediaID: 8, DurationS: 120}, {MediaID: 9, DurationS: 120}, {MediaID: 10, DurationS: 120}, {MediaID: 11, DurationS: 120}, {MediaID: 12, DurationS: 120}, {MediaID: 13, DurationS: 120}, {MediaID: 14, DurationS: 120}, {MediaID: 15, DurationS: 120}, {MediaID: 16, DurationS: 120}}},
		"background":           {Items: []Slide{{MediaID: 1, DurationS: 5}}, Background: "rainbow"},
	} {
		if err := bad.Normalize(); err == nil {
			t.Errorf("%s: accepted", name)
		}
	}
}

// TestSlideshowRender runs the real ffmpeg over the combinations the editor
// offers and checks the result is what Android 7 players expect.
func TestSlideshowRender(t *testing.T) {
	tools := FindTools(filepath.Join("..", "..", "tools"))
	if tools.FFmpeg == "" || tools.FFprobe == "" {
		t.Skip("ffmpeg not available")
	}
	dir := t.TempDir()
	// A portrait-ish photo and a square one, like phone pictures.
	imgs := []string{writePNG(t, dir, "a.png", 439, 500), writePNG(t, dir, "b.png", 1024, 1024), writePNG(t, dir, "c.png", 1600, 900)}
	audio := filepath.Join(dir, "music.m4a")
	if out, err := command(context.Background(), tools.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "sine=frequency=440:duration=3",
		"-c:a", "aac", audio).CombinedOutput(); err != nil {
		t.Fatalf("make audio: %v %s", err, out)
	}

	cases := map[string]Slideshow{
		"fade+motion+blur+music": {Items: []Slide{{MediaID: 1, DurationS: 3}, {MediaID: 2, DurationS: 2}, {MediaID: 3, DurationS: 2}}, Transition: "fade", TransitionS: 1, Motion: true, AudioID: 9},
		"cut+black":              {Items: []Slide{{MediaID: 1, DurationS: 2}, {MediaID: 2, DurationS: 2}}, Background: "black"},
		"slide+crop+portrait":    {Items: []Slide{{MediaID: 1, DurationS: 2}, {MediaID: 2, DurationS: 2}}, Transition: "slideleft", TransitionS: 0.5, Background: "crop", Portrait: true},
		"single":                 {Items: []Slide{{MediaID: 1, DurationS: 2}}},
		"captions": {Items: []Slide{
			{MediaID: 1, DurationS: 2, Text: `Черен петък: -50% на 'всичко' \ "нови" {цени} [12:00]`, TextPos: "bottom"},
			{MediaID: 2, DurationS: 2, Text: "Горе\nдва реда", TextPos: "top"},
			{MediaID: 3, DurationS: 2, Text: strings.Repeat("дълъг текст ", 20)}},
			Transition: "fade", TransitionS: 0.5, Motion: true, Text: TextStyle{Font: "impact", Size: "l", Color: "#ffcc00", Box: "none"}},
		"caption+light box+portrait": {Items: []Slide{{MediaID: 1, DurationS: 2, Text: "Кафе 2,50 лв.", TextPos: "center"}},
			Portrait: true, Text: TextStyle{Box: "light", Color: "#202020"}},
	}
	for name, sh := range cases {
		t.Run(name, func(t *testing.T) {
			if err := sh.Normalize(); err != nil {
				t.Fatal(err)
			}
			a := ""
			if sh.AudioID != 0 {
				a = audio
			}
			job := filepath.Join(dir, strings.NewReplacer("+", "_", " ", "_").Replace(name))
			if err := os.Mkdir(job, 0o755); err != nil {
				t.Fatal(err)
			}
			out := filepath.Join(job, "out.mp4")
			j := sh.Job(imgs[:len(sh.Items)], a, out)
			for f, content := range j.Files {
				if err := os.WriteFile(filepath.Join(job, f), []byte(content), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			if sh.HasText() {
				font, err := FindFont(Fonts[sh.Text.Font])
				if err != nil {
					t.Skip(err)
				}
				if err := copyFile(font, filepath.Join(job, SlideshowFont)); err != nil {
					t.Fatal(err)
				}
			}
			cmd := command(context.Background(), tools.FFmpeg, j.Args...)
			cmd.Dir = job
			if outp, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("ffmpeg: %v\n%s\ngraph:\n%s", err, outp, j.Files["graph.txt"])
			}
			p, err := tools.Probe(context.Background(), out)
			if err != nil {
				t.Fatal(err)
			}
			w, h := sh.Size()
			if p.Width != w || p.Height != h || p.VideoCodec != "h264" || math.Abs(p.FPS-30) > 0.01 {
				t.Errorf("stream: %dx%d %s %gfps", p.Width, p.Height, p.VideoCodec, p.FPS)
			}
			if got, want := float64(p.DurationMS)/1000, sh.TotalS(); math.Abs(got-want) > 0.15 {
				t.Errorf("duration %.2fs, want %.2fs", got, want)
			}
			if (a != "") != (p.AudioCodec == "aac") {
				t.Errorf("audio codec %q", p.AudioCodec)
			}
			if ws := CheckVideo("mp4", p); len(ws) > 0 {
				t.Errorf("compatibility warnings: %+v", ws)
			}
		})
	}
}

func TestWrap(t *testing.T) {
	if got := wrap("едно две три четири", 9); got != "едно две\nтри\nчетири" {
		t.Errorf("wrap: %q", got)
	}
	if got := wrap("ред1\n\nред2", 40); got != "ред1\n\nред2" {
		t.Errorf("keeps blank lines: %q", got)
	}
	if got := wrap("суперкалифраджилистик", 5); got != "суперкалифраджилистик" {
		t.Errorf("long word: %q", got)
	}
}

func TestCaptionValidation(t *testing.T) {
	for name, st := range map[string]TextStyle{
		"font": {Font: "wingdings"}, "size": {Size: "huge"}, "color": {Color: "red"}, "box": {Box: "glass"},
	} {
		sh := Slideshow{Items: []Slide{{MediaID: 1, DurationS: 2, Text: "x"}}, Text: st}
		if err := sh.Normalize(); err == nil {
			t.Errorf("bad %s accepted", name)
		}
	}
	sh := Slideshow{Items: []Slide{{MediaID: 1, DurationS: 2, Text: strings.Repeat("я", 301)}}}
	if err := sh.Normalize(); err == nil {
		t.Error("overlong text accepted")
	}
	sh = Slideshow{Items: []Slide{{MediaID: 1, DurationS: 2, Text: "  x  "}}}
	if err := sh.Normalize(); err != nil || sh.Items[0].TextPos != "bottom" || sh.Text.Font != "arial" {
		t.Errorf("defaults: %v %+v", err, sh)
	}
}

func writePNG(t *testing.T, dir, name string, w, h int) string {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.Set(x, y, color.RGBA{uint8(x * 255 / w), uint8(y * 255 / h), 120, 255})
		}
	}
	path := filepath.Join(dir, name)
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	return path
}
