package admin_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"marquee/internal/campaign"
	"marquee/internal/player"
)

type syncResp struct {
	Version     string `json:"version"`
	Status      string `json:"status"`
	PairingCode string `json:"pairing_code"`
	Name        string `json:"name"`
	Site        string `json:"site"`
	Segments    []struct {
		Start string `json:"start"`
		End   string `json:"end"`
		Items []struct {
			File       string `json:"file"`
			DurationMS int64  `json:"duration_ms"`
			Fit        string `json:"fit"`
		} `json:"items"`
	} `json:"segments"`
	Files []struct {
		Name string `json:"name"`
		URL  string `json:"url"`
		Kind string `json:"kind"`
	} `json:"files"`
}

// Marquee Player: registers with a code, is paired in the admin, wakes up
// on the change instead of polling, gets its playlist and reports its device.
func TestPlayerSync(t *testing.T) {
	e := newEnv(t)
	root := e.login("root@hub")
	ph := httptest.NewServer(player.New(e.st, mustStorage(e), slog.New(slog.NewTextHandler(io.Discard, nil)), player.Config{RefreshSec: 900}))
	defer ph.Close()

	const id = "3f6c1d0e-8a7b-4c2d-9e1f-0a1b2c3d4e5f"
	info := map[string]any{"manufacturer": "Rockchip", "model": "RK3288", "android": "7.1.1", "sdk": 25, "app": "0.1.0",
		"screen_w": 1920, "screen_h": 1080, "video_max_w": 1920, "video_max_h": 1080, "free_bytes": 4 << 30}
	sync := func(version string, wait bool) (int, syncResp) {
		body, _ := json.Marshal(map[string]any{"info": info, "version": version, "wait": wait})
		req, _ := http.NewRequest("POST", ph.URL+"/api/v1/sync", bytes.NewReader(body))
		req.Header.Set("X-Device-Id", id)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var out syncResp
		json.NewDecoder(resp.Body).Decode(&out)
		return resp.StatusCode, out
	}

	code, first := sync("", false)
	if code != 200 || first.Status != "pending" || len(first.PairingCode) == 0 || first.Version == "" {
		t.Fatalf("first sync: %d %+v", code, first)
	}

	// The player waits; pairing in the admin must wake it well before waitMax.
	ivanov := root.mustNode(nil, "client", "Ivanov")
	cafe := root.mustNode(p(ivanov), "site", "Cafe")
	woke := make(chan syncResp, 1)
	go func() { _, r := sync(first.Version, true); woke <- r }()
	time.Sleep(200 * time.Millisecond)
	root.mustDo("POST", "/api/devices/pair", map[string]any{"code": first.PairingCode, "site_id": cafe, "name": "Bar"}, http.StatusOK, nil)
	var paired syncResp
	select {
	case paired = <-woke:
	case <-time.After(10 * time.Second):
		t.Fatal("the waiting player was not woken by the pairing")
	}
	if paired.Status != "active" || paired.Name != "Bar" || paired.Site != "Cafe" || len(paired.Segments) != 0 {
		t.Fatalf("after pairing: %+v", paired)
	}

	// A published campaign reaches the waiting player too.
	_, up := root.upload(cafe, "a.png", testPNG(320, 180))
	img := root.waitReady(cafe, up[0].Media.ID, 5*time.Second)
	go func() { _, r := sync(paired.Version, true); woke <- r }()
	var c struct{ ID int64 }
	root.do("POST", "/api/campaigns", map[string]any{"node_id": cafe, "name": "T"}, &c)
	spec := campaign.Spec{Items: []campaign.Item{{MediaID: img.ID, DurationS: 7}}, Targets: []int64{cafe}, Fit: "slice"}
	root.do("PUT", "/api/campaigns/"+itoa(c.ID)+"/draft", map[string]any{"name": "T", "spec": spec}, nil)
	root.mustDo("POST", "/api/campaigns/"+itoa(c.ID)+"/publish", nil, http.StatusOK, nil)
	var live syncResp
	for deadline := time.Now().Add(10 * time.Second); len(live.Segments) == 0; {
		select {
		case live = <-woke:
		case <-time.After(time.Until(deadline)):
			t.Fatal("the campaign did not reach the waiting player")
		}
		if len(live.Segments) == 0 { // woken by the draft save; wait on
			go func(v string) { _, r := sync(v, true); woke <- r }(live.Version)
		}
	}
	it := live.Segments[0].Items
	if len(it) != 1 || it[0].DurationMS != 7000 || it[0].Fit != "slice" || it[0].File != img.SHA256+".png" {
		t.Fatalf("items: %+v", it)
	}
	if len(live.Files) != 1 || !strings.HasSuffix(live.Files[0].URL, "/m/"+img.SHA256+".png") {
		t.Fatalf("files: %+v", live.Files)
	}

	// Nothing changed: the same version, so a waiting player keeps waiting.
	if _, again := sync("", false); again.Version != live.Version {
		t.Fatalf("version changed without a change: %s -> %s", live.Version, again.Version)
	}

	// The admin sees what the player reported.
	var tree struct {
		Devices []struct {
			Name string `json:"name"`
			Info *struct {
				Model   string `json:"model"`
				ScreenW int    `json:"screen_w"`
			} `json:"info"`
		} `json:"devices"`
	}
	root.do("GET", "/api/tree", nil, &tree)
	if len(tree.Devices) != 1 || tree.Devices[0].Info == nil || tree.Devices[0].Info.Model != "RK3288" || tree.Devices[0].Info.ScreenW != 1920 {
		t.Fatalf("device info in the tree: %+v", tree.Devices)
	}

	req, _ := http.NewRequest("POST", ph.URL+"/api/v1/sync", strings.NewReader(`{}`))
	req.Header.Set("X-Device-Id", "short")
	if resp, err := http.DefaultClient.Do(req); err != nil || resp.StatusCode != http.StatusBadRequest {
		t.Fatalf("a bad device id was accepted: %v %v", resp, err)
	}
}

// The player installs from the hub: /app explains the steps and serves the APK.
func TestAppDownload(t *testing.T) {
	e := newEnv(t)
	apk := filepath.Join(t.TempDir(), player.AppName)
	ph := httptest.NewServer(player.New(e.st, mustStorage(e), slog.New(slog.NewTextHandler(io.Discard, nil)),
		player.Config{RefreshSec: 900, AppFile: apk}))
	defer ph.Close()
	get := func(path string) (*http.Response, string) {
		resp, err := http.Get(ph.URL + path)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return resp, string(b)
	}
	if _, page := get("/app"); !strings.Contains(page, "още не е сложен") {
		t.Fatal("no notice while the APK is missing")
	}
	if resp, _ := get("/app/" + player.AppName); resp.StatusCode != http.StatusNotFound {
		t.Fatalf("missing APK: %d", resp.StatusCode)
	}
	os.WriteFile(apk, []byte("PK fake apk"), 0o644)
	if _, page := get("/app"); !strings.Contains(page, `href="/app/marquee-player.apk"`) || !strings.Contains(page, ph.URL) {
		t.Fatalf("page without the download or the hub address:\n%s", page)
	}
	resp, body := get("/app/" + player.AppName)
	if resp.Header.Get("Content-Type") != "application/vnd.android.package-archive" || body != "PK fake apk" {
		t.Fatalf("download: %s %q", resp.Header.Get("Content-Type"), body)
	}
}
