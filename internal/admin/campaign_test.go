package admin_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"marquee/internal/campaign"
	"marquee/internal/player"
	"marquee/internal/store"
)

type campaignResp struct {
	ID         int64
	Name       string
	State      string
	HasPending bool `json:"has_pending"`
	Ready      int
	Devices    int
	Live       *struct {
		ID     int64
		Number int
	}
	Versions []struct {
		ID     int64
		Number int
		Status string
	}
}

func TestCampaignApprovalFlowReachesPlayer(t *testing.T) {
	e := newEnv(t)
	root := e.login("root@hub")
	ivanov := root.mustNode(nil, "client", "Ivanov")
	hotels := root.mustNode(p(ivanov), "group", "Hotels")
	cafes := root.mustNode(p(ivanov), "group", "Cafes")
	cafeC := root.mustNode(p(cafes), "site", "Cafe Center")
	root.mustUser("ivan@x", p(ivanov), []store.Role{{NodeID: ivanov, Role: "admin"}})
	root.mustUser("maria@x", p(ivanov), []store.Role{{NodeID: hotels, Role: "admin"}})
	root.mustUser("petar@x", p(ivanov), []store.Role{{NodeID: cafeC, Role: "editor",
		CanCreate: true, CanEdit: true, CanPublish: true, NeedsApproval: true}})
	ivan, maria, petar := e.login("ivan@x"), e.login("maria@x"), e.login("petar@x")

	// A paired screen in Cafe Center.
	dev, _ := e.st.RegisterDevice(context.Background(), "uuid-cafe", "", "", "")
	root.do("POST", "/api/devices/pair", map[string]any{"code": dev.PairingCode, "site_id": cafeC, "name": "Bar"}, nil)
	ph := httptest.NewServer(player.New(e.st, mustStorage(e), slog.New(slog.NewTextHandler(io.Discard, nil)),
		player.Config{PublicURL: "https://hub", RefreshSec: 900}))
	defer ph.Close()
	index := func() string {
		req, _ := http.NewRequest("GET", ph.URL+"/p/index.smil", nil)
		req.Header.Set("User-Agent", "GAPI/1.0 (UUID:uuid-cafe; NAME:x) player/1.0 (MODEL:box)")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return string(b)
	}

	_, up := petar.upload(cafeC, "menu.png", testPNG(1280, 720))
	img := petar.waitReady(cafeC, up[0].Media.ID, 5*time.Second)
	_, up = ivan.upload(hotels, "pool.png", testPNG(800, 600))
	pool := ivan.waitReady(hotels, up[0].Media.ID, 5*time.Second)

	var c campaignResp
	if code, body := petar.do("POST", "/api/campaigns", map[string]any{"node_id": cafeC, "name": "Обедно меню"}, &c); code != 201 {
		t.Fatalf("create: %d %s", code, body)
	}
	cid := itoa(c.ID)
	spec := campaign.Spec{Items: []campaign.Item{{MediaID: img.ID, DurationS: 7}}, Targets: []int64{cafeC}, Fit: "meet"}

	t.Run("draft validation", func(t *testing.T) {
		bad := spec
		bad.Targets = []int64{hotels}
		if code, _ := petar.do("PUT", "/api/campaigns/"+cid+"/draft", map[string]any{"name": "x", "spec": bad}, nil); code != 400 {
			t.Errorf("target outside owner accepted: %d", code)
		}
		bad = spec
		bad.Items = []campaign.Item{{MediaID: pool.ID, DurationS: 5}}
		if code, _ := petar.do("PUT", "/api/campaigns/"+cid+"/draft", map[string]any{"name": "x", "spec": bad}, nil); code != 400 {
			t.Errorf("media from hotels accepted in a cafe campaign: %d", code)
		}
		bad = spec
		bad.Start, bad.End = "2026-11-12T00:00", "2026-11-11T00:00"
		if code, _ := petar.do("PUT", "/api/campaigns/"+cid+"/draft", map[string]any{"name": "x", "spec": bad}, nil); code != 400 {
			t.Errorf("end before start accepted: %d", code)
		}
	})

	if code, body := petar.do("PUT", "/api/campaigns/"+cid+"/draft", map[string]any{"name": "Обедно меню", "spec": spec}, nil); code != 200 {
		t.Fatalf("save draft: %d %s", code, body)
	}

	t.Run("editor publication waits for approval", func(t *testing.T) {
		petar.do("POST", "/api/campaigns/"+cid+"/publish", nil, &c)
		if !c.HasPending || c.Live != nil || c.State != "pending" {
			t.Fatalf("after publish: %+v", c)
		}
		if strings.Contains(index(), img.SHA256) {
			t.Fatal("pending version reached the player")
		}
	})

	t.Run("only an admin above may approve", func(t *testing.T) {
		vid := itoa(c.Versions[0].ID)
		if code, _ := maria.do("POST", "/api/campaigns/"+cid+"/versions/"+vid+"/review", map[string]any{"approve": true}, nil); code != 404 {
			t.Fatalf("hotels admin reviews a cafe campaign: %d", code)
		}
		if code, _ := petar.do("POST", "/api/campaigns/"+cid+"/versions/"+vid+"/review", map[string]any{"approve": true}, nil); code != 403 {
			t.Fatalf("editor approves himself: %d", code)
		}
		var appr []struct {
			VersionID int64 `json:"version_id"`
		}
		ivan.do("GET", "/api/approvals", nil, &appr)
		if len(appr) != 1 {
			t.Fatalf("ivan approvals: %+v", appr)
		}
		if code, body := ivan.do("POST", "/api/campaigns/"+cid+"/versions/"+vid+"/review", map[string]any{"approve": true}, &c); code != 200 {
			t.Fatalf("approve: %d %s", code, body)
		}
		if c.Live == nil || c.State != "active" {
			t.Fatalf("after approve: %+v", c)
		}
	})

	t.Run("player gets the campaign and readiness counts it", func(t *testing.T) {
		smil := index()
		// Media come from the address the device itself used to reach the hub.
		if !strings.Contains(smil, ph.URL+"/m/"+img.SHA256+".png") || !strings.Contains(smil, `dur="7s"`) {
			t.Fatalf("index:\n%s", smil)
		}
		ivan.do("GET", "/api/campaigns/"+cid, nil, &c)
		if c.Devices != 1 || c.Ready != 1 {
			t.Fatalf("readiness: %d/%d", c.Ready, c.Devices)
		}
	})

	t.Run("media in use cannot be deleted", func(t *testing.T) {
		if code, _ := petar.do("DELETE", "/api/media/"+itoa(img.ID), nil, nil); code != 409 {
			t.Fatalf("delete used media: %d", code)
		}
	})

	t.Run("stop, rollback, delete", func(t *testing.T) {
		v1 := c.Live.ID
		if code, _ := petar.do("POST", "/api/campaigns/"+cid+"/live", map[string]any{"version_id": 0}, &c); code != 200 || c.State != "stopped" {
			t.Fatalf("stop: %d %+v", code, c)
		}
		if strings.Contains(index(), img.SHA256) {
			t.Fatal("stopped campaign still sent")
		}
		if code, _ := petar.do("POST", "/api/campaigns/"+cid+"/live", map[string]any{"version_id": v1}, &c); code != 200 || c.Live == nil {
			t.Fatalf("rollback: %d %+v", code, c)
		}
		if code, _ := petar.do("DELETE", "/api/campaigns/"+cid, nil, nil); code != 409 {
			t.Fatalf("delete live campaign: %d", code)
		}
	})

	t.Run("admin publishes directly", func(t *testing.T) {
		var c2 campaignResp
		ivan.do("POST", "/api/campaigns", map[string]any{"node_id": cafes, "name": "Черен петък"}, &c2)
		s2 := campaign.Spec{Items: []campaign.Item{{MediaID: img.ID, DurationS: 5}}, Targets: []int64{cafes},
			Start: "2026-11-27T00:00", End: "2026-11-28T00:00", Priority: 1}
		if code, body := ivan.do("PUT", "/api/campaigns/"+itoa(c2.ID)+"/draft", map[string]any{"name": "Черен петък", "spec": s2}, nil); code != 400 {
			// img belongs to Cafe Center, below the campaign's node: not visible to all of Cafes.
			t.Fatalf("media from below accepted: %d %s", code, body)
		}
	})
}
