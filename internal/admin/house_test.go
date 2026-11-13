package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"marquee/internal/campaign"
	"marquee/internal/player"
	"marquee/internal/store"
)

func TestOwnAdsPlayOnClientScreensUnseen(t *testing.T) {
	e := newEnv(t)
	root := e.login("root@hub")
	ivanov := root.mustNode(nil, "client", "Ivanov")
	cafe := root.mustNode(p(ivanov), "site", "Cafe")
	root.mustUser("ivan@x", p(ivanov), []store.Role{{NodeID: ivanov, Role: "admin"}})
	ivan := e.login("ivan@x")

	var house struct{ ID int64 }
	root.mustDo("POST", "/api/house", nil, 200, &house)
	var again struct{ ID int64 }
	root.mustDo("POST", "/api/house", nil, 200, &again)
	if again.ID != house.ID {
		t.Fatalf("a second house node: %d %d", house.ID, again.ID)
	}
	hid := itoa(house.ID)

	// The client's own campaign, so the screen has something to cut into.
	_, up := ivan.upload(cafe, "menu.png", testPNG(640, 360))
	menu := ivan.waitReady(cafe, up[0].Media.ID, 5*time.Second)
	var own campaignResp
	ivan.mustDo("POST", "/api/campaigns", map[string]any{"node_id": cafe, "name": "Меню"}, 201, &own)
	spec := campaign.Spec{Items: []campaign.Item{{MediaID: menu.ID, DurationS: 8}}, Targets: []int64{cafe}, Fit: "meet"}
	ivan.mustDo("PUT", "/api/campaigns/"+itoa(own.ID)+"/draft", map[string]any{"name": "Меню", "spec": spec}, 200, nil)
	ivan.mustDo("POST", "/api/campaigns/"+itoa(own.ID)+"/publish", nil, 200, nil)

	t.Run("clients cannot set times per day", func(t *testing.T) {
		bad := spec
		bad.PerDay = 3
		if code, _ := ivan.do("PUT", "/api/campaigns/"+itoa(own.ID)+"/draft", map[string]any{"name": "Меню", "spec": bad}, nil); code != 400 {
			t.Fatalf("per_day in a client campaign: %d", code)
		}
	})

	_, up = root.upload(house.ID, "ad.png", testPNG(640, 360))
	adImg := root.waitReady(house.ID, up[0].Media.ID, 5*time.Second)
	var ad campaignResp
	root.mustDo("POST", "/api/campaigns", map[string]any{"node_id": house.ID, "name": "Твоята реклама тук"}, 201, &ad)
	adSpec := campaign.Spec{Items: []campaign.Item{{MediaID: adImg.ID, DurationS: 10}}, Targets: []int64{cafe}, Fit: "meet", PerDay: 10}
	root.mustDo("PUT", "/api/campaigns/"+itoa(ad.ID)+"/draft", map[string]any{"name": "Твоята реклама тук", "spec": adSpec}, 200, nil)
	root.mustDo("POST", "/api/campaigns/"+itoa(ad.ID)+"/publish", nil, 200, nil)

	t.Run("hidden from the client", func(t *testing.T) {
		var tr treeResp
		ivan.do("GET", "/api/tree", nil, &tr)
		if names(tr)["Моята реклама"] {
			t.Error("client sees the operator's ads node")
		}
		if code, _ := ivan.do("GET", "/api/campaigns?node_id="+hid, nil, nil); code < 400 {
			t.Errorf("client lists the operator's campaigns: %d", code)
		}
		if code, _ := ivan.do("GET", "/api/campaigns/"+itoa(ad.ID), nil, nil); code < 400 {
			t.Errorf("client opens the operator's campaign: %d", code)
		}
		if code, _ := ivan.do("GET", "/api/media?node_id="+hid, nil, nil); code < 400 {
			t.Errorf("client lists the operator's media: %d", code)
		}
		var list []struct{ Name string }
		ivan.do("GET", "/api/campaigns?node_id="+itoa(cafe), nil, &list)
		for _, c := range list {
			if c.Name == "Твоята реклама тук" {
				t.Error("the ad shows in the client's campaign list")
			}
		}
		if code, _ := root.do("POST", "/api/nodes", map[string]any{"parent_id": house.ID, "kind": "site", "name": "x"}, nil); code != 400 {
			t.Errorf("a site under the ads node: %d", code)
		}
	})

	t.Run("plays on the client's screen", func(t *testing.T) {
		dev, _ := e.st.RegisterDevice(context.Background(), "uuid-cafe", "", "", "")
		root.mustDo("POST", "/api/devices/pair", map[string]any{"code": dev.PairingCode, "site_id": cafe, "name": "Bar"}, 200, nil)
		ph := httptest.NewServer(player.New(e.st, mustStorage(e), slog.New(slog.NewTextHandler(io.Discard, nil)),
			player.Config{PublicURL: "https://hub", RefreshSec: 900}))
		defer ph.Close()
		req, _ := http.NewRequest("GET", ph.URL+"/p/index.smil", nil)
		req.Header.Set("User-Agent", "GAPI/1.0 (UUID:uuid-cafe; NAME:x) player/1.0 (MODEL:box)")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if !strings.Contains(string(b), menu.SHA256) || !strings.Contains(string(b), adImg.SHA256) {
			t.Fatalf("index without the client's content and the ad:\n%.1500s", b)
		}
	})
}

func TestCampaignExportImport(t *testing.T) {
	e := newEnv(t)
	root := e.login("root@hub")
	home := root.mustNode(nil, "client", "Home")
	_, up := root.upload(home, "a.png", testPNG(320, 240))
	img := root.waitReady(home, up[0].Media.ID, 5*time.Second)
	var c campaignResp
	root.mustDo("POST", "/api/campaigns", map[string]any{"node_id": home, "name": "Лято"}, 201, &c)
	spec := campaign.Spec{Items: []campaign.Item{{MediaID: img.ID, DurationS: 6, Every: 0}}, Targets: []int64{home},
		Fit: "slice", Order: "shuffle", TimeFrom: "09:00", TimeTo: "18:00"}
	root.mustDo("PUT", "/api/campaigns/"+itoa(c.ID)+"/draft", map[string]any{"name": "Лято", "spec": spec}, 200, nil)

	req, _ := http.NewRequest("GET", e.srv.URL+"/api/campaigns/"+itoa(c.ID)+"/export", nil)
	req.Header.Set("X-Marquee", "1")
	resp, err := root.http.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	zipped, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if resp.StatusCode != 200 || resp.Header.Get("Content-Type") != "application/zip" {
		t.Fatalf("export: %d %s", resp.StatusCode, resp.Header.Get("Content-Type"))
	}

	// Into another client of the same hub: the file is reused, not stored twice.
	other := root.mustNode(nil, "client", "Other")
	importZip := func(node int64, data []byte) (int, string, int64) {
		var buf bytes.Buffer
		mw := multipart.NewWriter(&buf)
		fw, _ := mw.CreateFormFile("file", "Лято.zip")
		fw.Write(data)
		mw.Close()
		req, _ := http.NewRequest("POST", e.srv.URL+"/api/campaigns/import?node_id="+itoa(node), &buf)
		req.Header.Set("X-Marquee", "1")
		req.Header.Set("Content-Type", mw.FormDataContentType())
		resp, err := root.http.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		var out struct{ ID int64 }
		if resp.StatusCode == 201 {
			_ = json.Unmarshal(raw, &out)
		}
		return resp.StatusCode, string(raw), out.ID
	}
	code, body, id := importZip(other, zipped)
	if code != 201 {
		t.Fatalf("import: %d %s", code, body)
	}
	var got struct {
		Name     string
		NodeID   int64 `json:"node_id"`
		Draft    campaign.Spec
		Versions []struct{}
	}
	root.mustDo("GET", "/api/campaigns/"+itoa(id), nil, 200, &got)
	if got.Name != "Лято" || got.NodeID != other || len(got.Versions) != 0 {
		t.Fatalf("imported campaign: %+v", got)
	}
	d := got.Draft
	if len(d.Items) != 1 || d.Items[0].DurationS != 6 || d.Fit != "slice" || d.Order != "shuffle" ||
		d.TimeFrom != "09:00" || len(d.Targets) != 1 || d.Targets[0] != other {
		t.Fatalf("imported draft: %+v", d)
	}
	lib := root.mediaList(other)
	if len(lib) != 1 || lib[0].ID != d.Items[0].MediaID || lib[0].SHA256 != img.SHA256 {
		t.Fatalf("imported media: %+v", lib)
	}
	root.waitReady(other, lib[0].ID, 5*time.Second)

	// Again into the same node: the library already has the file.
	if code, body, _ := importZip(other, zipped); code != 201 {
		t.Fatalf("second import: %d %s", code, body)
	}
	if n := len(root.mediaList(other)); n != 1 {
		t.Errorf("file stored twice: %d items", n)
	}

	if code, _, _ := importZip(other, []byte("not a zip")); code != 400 {
		t.Errorf("garbage accepted: %d", code)
	}
}
