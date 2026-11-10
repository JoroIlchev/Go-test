package admin_test

import (
	"testing"
	"time"

	"marquee/internal/media"
	"marquee/internal/store"
)

func TestSlideshowRender(t *testing.T) {
	if media.FindTools("").FFmpeg == "" {
		t.Skip("ffmpeg not available")
	}
	e := newEnv(t)
	root := e.login("root@hub")
	ivanov := root.mustNode(nil, "client", "Ivanov")
	cafes := root.mustNode(p(ivanov), "group", "Cafes")
	hotels := root.mustNode(p(ivanov), "group", "Hotels")
	root.mustUser("petar@x", p(ivanov), []store.Role{{NodeID: cafes, Role: "editor", CanCreate: true}})
	root.mustUser("viewer@x", p(ivanov), []store.Role{{NodeID: cafes, Role: "editor"}})
	petar, viewer := e.login("petar@x"), e.login("viewer@x")

	_, up := root.upload(ivanov, "logo.png", testPNG(439, 500))
	logo := root.waitReady(ivanov, up[0].Media.ID, 5*time.Second)
	_, up = petar.upload(cafes, "menu.png", testPNG(1024, 1024))
	menu := petar.waitReady(cafes, up[0].Media.ID, 5*time.Second)
	_, up = root.upload(hotels, "pool.png", testPNG(800, 600))
	pool := root.waitReady(hotels, up[0].Media.ID, 5*time.Second)

	req := func(node int64, ids ...int64) map[string]any {
		var items []map[string]any
		for _, id := range ids {
			items = append(items, map[string]any{"media_id": id, "duration_s": 2, "text": "Обедно меню: 9,99 лв.", "text_pos": "bottom"})
		}
		return map[string]any{"node_id": node, "name": "Меню", "slideshow": map[string]any{
			"items": items, "transition": "fade", "transition_s": 0.5, "motion": true}}
	}

	if code, _ := viewer.do("POST", "/api/media/render", req(cafes, logo.ID, menu.ID), nil); code != 403 {
		t.Errorf("editor without create rights renders: %d", code)
	}
	if code, _ := petar.do("POST", "/api/media/render", req(cafes, logo.ID, pool.ID), nil); code != 400 {
		t.Errorf("image from a sibling branch accepted: %d", code)
	}
	if code, _ := root.do("POST", "/api/media/render", req(ivanov, menu.ID), nil); code != 400 {
		t.Errorf("video at the client may not use an image from below: %d", code)
	}

	var v mediaItem
	if code, body := petar.do("POST", "/api/media/render", req(cafes, logo.ID, menu.ID), &v); code != 202 {
		t.Fatalf("render: %d %s", code, body)
	}
	v = petar.waitReady(cafes, v.ID, 60*time.Second)
	if v.Kind != "video" || v.Ext != "mp4" || v.VideoCodec != "h264" || v.Width != 1920 || v.Height != 1080 || !v.HasThumb {
		t.Fatalf("rendered: %+v", v)
	}
	if len(v.Warnings) != 0 {
		t.Errorf("rendered video has warnings: %+v", v.Warnings)
	}
}
