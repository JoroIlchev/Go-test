package admin_test

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"net/http"
	"os"
	"testing"
	"time"

	"marquee/internal/media"
)

func testJPEG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{uint8(x), uint8(y), 120, 255})
		}
	}
	var buf bytes.Buffer
	jpeg.Encode(&buf, img, &jpeg.Options{Quality: 95})
	return buf.Bytes()
}

// A phone-sized photo is shrunk to the screen on upload: a desktop player's
// player shows images at their own pixel size, and big ones strain Android.
func TestLargeImageShrunkOnUpload(t *testing.T) {
	if media.FindTools("").FFmpeg == "" {
		t.Skip("ffmpeg not available")
	}
	e := newEnv(t)
	root := e.login("root@hub")
	ivanov := root.mustNode(nil, "client", "Ivanov")

	_, up := root.upload(ivanov, "tall.jpg", testJPEG(1500, 2000))
	orig := up[0].Media.SHA256
	m := root.waitReady(ivanov, up[0].Media.ID, 10*time.Second)
	if m.Width != 810 || m.Height != 1080 || m.Ext != "jpg" || m.SHA256 == orig {
		t.Fatalf("not shrunk into 1920x1080: %+v", m)
	}
	if len(m.Warnings) != 1 || m.Warnings[0].Level != "info" || m.Warnings[0].Code != "resized" {
		t.Fatalf("want one info note: %+v", m.Warnings)
	}
	if _, err := os.Stat(mustStorage(e).Path(orig, "jpg")); !os.IsNotExist(err) {
		t.Errorf("original blob kept: %v", err)
	}

	// A picture that fits stays as it is.
	_, up = root.upload(ivanov, "small.png", testPNG(800, 600))
	m = root.waitReady(ivanov, up[0].Media.ID, 10*time.Second)
	if m.Width != 800 || m.SHA256 != up[0].Media.SHA256 || len(m.Warnings) != 0 {
		t.Fatalf("small image changed: %+v", m)
	}
}

// The media library may override the hub settings for one upload.
func TestUploadChoosesProcessing(t *testing.T) {
	if media.FindTools("").FFmpeg == "" {
		t.Skip("ffmpeg not available")
	}
	e := newEnv(t)
	root := e.login("root@hub")
	ivanov := root.mustNode(nil, "client", "Ivanov")
	prep := func(screen string) string {
		return `{"image_screen":"` + screen + `","image_quality":"high","video_auto":"off","video_max":720,"video_quality":"small"}`
	}

	_, up := root.uploadWith(ivanov, "keep.jpg", testJPEG(1500, 2000), prep("off"))
	m := root.waitReady(ivanov, up[0].Media.ID, 10*time.Second)
	if m.Width != 1500 || m.Height != 2000 || len(m.Warnings) != 0 {
		t.Fatalf("original not kept: %+v", m)
	}

	_, up = root.uploadWith(ivanov, "portrait.jpg", testJPEG(1500, 2000), prep("1080x1920"))
	m = root.waitReady(ivanov, up[0].Media.ID, 10*time.Second)
	if m.Width != 1080 || m.Height != 1440 {
		t.Fatalf("not fitted into 1080x1920: %+v", m)
	}

	if code, _ := root.uploadWith(ivanov, "bad.jpg", testJPEG(64, 64), prep("12x12")); code != http.StatusBadRequest {
		t.Fatalf("invalid choice accepted: %d", code)
	}
}

// Slideshows from JPEG photos must come out in limited-range yuv420p (a
// "yuvj" file froze on a player) and repeat inside the file when asked.
func TestSlideshowFromPhotosColourAndRepeat(t *testing.T) {
	if media.FindTools("").FFmpeg == "" {
		t.Skip("ffmpeg not available")
	}
	e := newEnv(t)
	root := e.login("root@hub")
	ivanov := root.mustNode(nil, "client", "Ivanov")
	var ids []int64
	for _, name := range []string{"a.jpg", "b.jpg"} {
		_, up := root.upload(ivanov, name, testJPEG(640, 480))
		ids = append(ids, root.waitReady(ivanov, up[0].Media.ID, 10*time.Second).ID)
	}
	items := []map[string]any{{"media_id": ids[0], "duration_s": 2}, {"media_id": ids[1], "duration_s": 2}}
	var v mediaItem
	if code, body := root.do("POST", "/api/media/render", map[string]any{"node_id": ivanov, "name": "Снимки",
		"slideshow": map[string]any{"items": items, "transition": "none", "repeat_to_s": 10}}, &v); code != 202 {
		t.Fatalf("render: %d %s", code, body)
	}
	v = root.waitReady(ivanov, v.ID, 60*time.Second)
	if len(v.Warnings) != 0 {
		t.Fatalf("warnings (pix_fmt?): %+v", v.Warnings)
	}
	// 4 s shown 3 times to reach at least 10 s.
	if v.DurationMS < 11500 || v.DurationMS > 12500 {
		t.Fatalf("duration %d ms, want 12 s", v.DurationMS)
	}
}
