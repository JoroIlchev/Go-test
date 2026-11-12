package admin_test

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"marquee/internal/campaign"
	"marquee/internal/ops"
	"marquee/internal/player"
	"marquee/internal/poll"
	"marquee/internal/store"
)

func TestPollScheduleReachesPlayerAndWarnsLatePublish(t *testing.T) {
	e := newEnv(t)
	root := e.login("root@hub")
	ivanov := root.mustNode(nil, "client", "Ivanov")
	cafes := root.mustNode(p(ivanov), "group", "Cafes")
	cafe := root.mustNode(p(cafes), "site", "Cafe")
	root.mustUser("ivan@x", p(ivanov), []store.Role{{NodeID: ivanov, Role: "admin"}})
	root.mustUser("petar@x", p(ivanov), []store.Role{{NodeID: cafe, Role: "editor", CanCreate: true, CanEdit: true, CanPublish: true}})
	ivan, petar := e.login("ivan@x"), e.login("petar@x")

	dev, _ := e.st.RegisterDevice(context.Background(), "uuid-1", "", "", "")
	root.do("POST", "/api/devices/pair", map[string]any{"code": dev.PairingCode, "site_id": cafe, "name": "Bar"}, nil)
	ph := httptest.NewServer(player.New(e.st, mustStorage(e), slog.New(slog.NewTextHandler(io.Discard, nil)),
		player.Config{PublicURL: "https://hub", RefreshSec: 900}))
	defer ph.Close()
	refresh := func() int {
		req, _ := http.NewRequest("GET", ph.URL+"/p/index.smil", nil)
		req.Header.Set("User-Agent", "GAPI/1.0 (UUID:uuid-1; NAME:x) player/1.0 (MODEL:box)")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		m := regexp.MustCompile(`Refresh" content="(\d+)"`).FindStringSubmatch(string(b))
		if m == nil {
			t.Fatalf("no refresh in:\n%s", b)
		}
		n, _ := strconv.Atoi(m[1])
		return n
	}

	if got := refresh(); got != 900 {
		t.Fatalf("hub default: refresh %d", got)
	}

	var pol struct {
		Own        *struct{ Mode string }
		FromNodeID int64 `json:"from_node_id"`
		CanEdit    bool  `json:"can_edit"`
	}
	times := map[string]any{"policy": map[string]any{"mode": "times", "times": []string{"00:00", "12:00"}, "spread_min": 0}}
	if code, _ := petar.do("PUT", "/api/nodes/"+itoa(cafe)+"/poll", times, nil); code != 403 {
		t.Fatalf("editor changes polling: %d", code)
	}
	if code, body := ivan.do("PUT", "/api/nodes/"+itoa(ivanov)+"/poll", times, &pol); code != 200 || pol.Own == nil {
		t.Fatalf("set poll: %d %s", code, body)
	}
	petar.do("GET", "/api/nodes/"+itoa(cafe)+"/poll", nil, &pol)
	if pol.Own != nil || pol.FromNodeID != ivanov || pol.CanEdit {
		t.Fatalf("inherited view: %+v", pol)
	}

	// The device now comes back at the next 00:00 or 12:00.
	got := refresh()
	now := time.Now().In(time.Local)
	next := time.Date(now.Year(), now.Month(), now.Day(), 12, 0, 0, 0, time.Local)
	if !next.After(now.Add(time.Minute)) {
		next = time.Date(now.Year(), now.Month(), now.Day()+1, 0, 0, 0, 0, time.Local)
	}
	if want := int(time.Until(next).Seconds()); got < want-5 || got > want+5 {
		t.Fatalf("refresh %d, want about %d", got, want)
	}

	// Publishing a campaign that starts before that check: the editor is warned.
	_, up := petar.upload(cafe, "menu.png", testPNG(640, 360))
	img := petar.waitReady(cafe, up[0].Media.ID, 5*time.Second)
	var c struct {
		ID          int64
		Late        int    `json:"late"`
		LatestCheck string `json:"latest_check"`
	}
	petar.do("POST", "/api/campaigns", map[string]any{"node_id": cafe, "name": "Скоро"}, &c)
	start := time.Now().Add(5 * time.Minute).In(time.Local).Format("2006-01-02T15:04")
	spec := campaign.Spec{Items: []campaign.Item{{MediaID: img.ID, DurationS: 5}}, Targets: []int64{cafe}, Start: start}
	petar.do("PUT", "/api/campaigns/"+itoa(c.ID)+"/draft", map[string]any{"name": "Скоро", "spec": spec}, nil)
	if code, body := petar.do("POST", "/api/campaigns/"+itoa(c.ID)+"/publish", nil, &c); code != 200 {
		t.Fatalf("publish: %d %s", code, body)
	}
	if c.Late != 1 || c.LatestCheck == "" {
		t.Fatalf("late warning: %+v", c)
	}
	// Once the device has picked it up, no warning.
	refresh()
	petar.do("GET", "/api/campaigns/"+itoa(c.ID), nil, &c)
	if c.Late != 0 {
		t.Fatalf("still late after the device got it: %+v", c)
	}

	// Back to the hub default.
	ivan.do("PUT", "/api/nodes/"+itoa(ivanov)+"/poll", map[string]any{"policy": nil}, nil)
	if got := refresh(); got != 900 {
		t.Fatalf("after reset: refresh %d", got)
	}
}

func TestHubSettingsAndBillingAreSuperOnly(t *testing.T) {
	e := newEnv(t)
	root := e.login("root@hub")
	ivanov := root.mustNode(nil, "client", "Ivanov")
	cafe := root.mustNode(p(ivanov), "site", "Cafe")
	root.mustUser("ivan@x", p(ivanov), []store.Role{{NodeID: ivanov, Role: "admin"}})
	ivan := e.login("ivan@x")

	for _, path := range []string{"/api/settings", "/api/billing"} {
		if code, _ := ivan.do("GET", path, nil, nil); code != 403 {
			t.Errorf("client admin GET %s: %d", path, code)
		}
	}
	set := map[string]any{"smtp_host": "mail.x", "smtp_port": 587, "smtp_user": "u", "smtp_pass": "secret",
		"smtp_from": "hub@x.bg", "alert_emails": []string{"ops@x.bg", " "}, "alert_admins": true,
		"alert_grace_min": 10, "backup_time": "3:00", "backup_keep": 7, "backup_dir": ""}
	var got map[string]any
	if code, body := root.do("PUT", "/api/settings", set, &got); code != 200 {
		t.Fatalf("save settings: %d %s", code, body)
	}
	if got["smtp_pass"] != nil || got["has_password"] != true || got["backup_time"] != "03:00" {
		t.Fatalf("settings echo: %v", got)
	}
	// An empty password keeps the stored one.
	set["smtp_pass"] = ""
	root.do("PUT", "/api/settings", set, &got)
	if got["has_password"] != true {
		t.Fatal("password lost on save")
	}

	dev, _ := e.st.CreateLinkDevice(context.Background(), mustNodePtr(t, e, cafe), "Bar")
	_ = dev
	var bill struct {
		Rows []struct {
			Name       string
			DeviceDays int `json:"device_days"`
			ActiveEnd  int `json:"active_end"`
		}
	}
	if code, body := root.do("GET", "/api/billing", nil, &bill); code != 200 {
		t.Fatalf("billing: %d %s", code, body)
	}
	if len(bill.Rows) != 1 || bill.Rows[0].DeviceDays != 1 || bill.Rows[0].ActiveEnd != 1 {
		t.Fatalf("billing rows: %+v", bill.Rows)
	}
	_, resp := root.do("GET", "/api/billing?format=csv", nil, nil)
	if !strings.Contains(resp, "Ivanov;1;1;1") {
		t.Fatalf("csv:\n%s", resp)
	}
}

func TestHubAddressInDeviceLinks(t *testing.T) {
	e := newEnv(t)
	root := e.login("root@hub")
	ivanov := root.mustNode(nil, "client", "Ivanov")
	cafe := root.mustNode(p(ivanov), "site", "Cafe")
	var dev struct{ URL string }
	root.do("POST", "/api/devices", map[string]any{"site_id": cafe, "name": "Bar"}, &dev)
	// Without settings: this machine's IP, never localhost.
	if strings.Contains(dev.URL, "localhost") || !strings.HasPrefix(dev.URL, "http://") || !strings.Contains(dev.URL, ":8080/d/") {
		t.Fatalf("detected link: %s", dev.URL)
	}

	var set struct {
		HubURL     string   `json:"hub_url"`
		HubPrivate bool     `json:"hub_private"`
		Detected   []string `json:"detected_urls"`
	}
	if code, body := root.do("GET", "/api/settings", nil, &set); code != 200 || len(set.Detected) == 0 {
		t.Fatalf("settings: %d %s", code, body)
	}
	save := map[string]any{"public_url": "85.10.20.30", "smtp_port": 587, "alert_grace_min": 10, "backup_keep": 14}
	if code, body := root.do("PUT", "/api/settings", save, &set); code != 200 || set.HubURL != "http://85.10.20.30:8080" || set.HubPrivate {
		t.Fatalf("save: %d %s %+v", code, body, set)
	}
	var tree struct {
		Devices []struct{ URL string }
		HubURL  string `json:"hub_url"`
	}
	root.do("GET", "/api/tree", nil, &tree)
	if tree.HubURL != "http://85.10.20.30:8080" || !strings.HasPrefix(tree.Devices[0].URL, "http://85.10.20.30:8080/d/") {
		t.Fatalf("tree: %+v", tree)
	}
	save["public_url"] = "ftp://x"
	if code, _ := root.do("PUT", "/api/settings", save, nil); code != 400 {
		t.Errorf("bad address accepted: %d", code)
	}
}

func TestMediaURLsBehindTunnel(t *testing.T) {
	e := newEnv(t)
	root := e.login("root@hub")
	ivanov := root.mustNode(nil, "client", "Ivanov")
	cafe := root.mustNode(p(ivanov), "site", "Cafe")
	dev, _ := e.st.RegisterDevice(context.Background(), "uuid-t", "", "", "")
	root.do("POST", "/api/devices/pair", map[string]any{"code": dev.PairingCode, "site_id": cafe, "name": "Bar"}, nil)
	_, up := root.upload(cafe, "a.png", testPNG(320, 180))
	img := root.waitReady(cafe, up[0].Media.ID, 5*time.Second)
	var c struct{ ID int64 }
	root.do("POST", "/api/campaigns", map[string]any{"node_id": cafe, "name": "T"}, &c)
	spec := campaign.Spec{Items: []campaign.Item{{MediaID: img.ID, DurationS: 5}}, Targets: []int64{cafe}}
	root.do("PUT", "/api/campaigns/"+itoa(c.ID)+"/draft", map[string]any{"name": "T", "spec": spec}, nil)
	root.do("POST", "/api/campaigns/"+itoa(c.ID)+"/publish", nil, nil)

	ph := httptest.NewServer(player.New(e.st, mustStorage(e), slog.New(slog.NewTextHandler(io.Discard, nil)), player.Config{RefreshSec: 900}))
	defer ph.Close()
	req, _ := http.NewRequest("GET", ph.URL+"/p/index.smil", nil)
	req.Host = "abc.trycloudflare.com" // what cloudflared passes on
	req.Header.Set("X-Forwarded-Proto", "https")
	req.Header.Set("User-Agent", "GAPI/1.0 (UUID:uuid-t; NAME:x) player/1.0 (MODEL:box)")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	if want := "https://abc.trycloudflare.com/m/" + img.SHA256; !strings.Contains(string(b), want) {
		t.Fatalf("want %s in:\n%s", want, b)
	}
}

func mustNodePtr(t *testing.T, e *env, id int64) *store.Node {
	n, err := e.st.Node(context.Background(), id)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

// The hub-wide check schedule comes from the settings; a client's own wins.
func TestDefaultPollFromSettings(t *testing.T) {
	e := newEnv(t)
	root := e.login("root@hub")
	ivanov := root.mustNode(nil, "client", "Ivanov")
	cafe := root.mustNode(p(ivanov), "site", "Cafe")
	dev, _ := e.st.RegisterDevice(context.Background(), "uuid-d", "", "", "")
	root.do("POST", "/api/devices/pair", map[string]any{"code": dev.PairingCode, "site_id": cafe, "name": "Bar"}, nil)
	ph := httptest.NewServer(player.New(e.st, mustStorage(e), slog.New(slog.NewTextHandler(io.Discard, nil)),
		player.Config{RefreshSec: 900, DefaultPoll: func(ctx context.Context) poll.Policy {
			p, _ := ops.DefaultPoll(ctx, e.st, poll.Interval(15*time.Minute))
			return p
		}}))
	defer ph.Close()
	refresh := func() string {
		req, _ := http.NewRequest("GET", ph.URL+"/p/index.smil", nil)
		req.Header.Set("User-Agent", "GAPI/1.0 (UUID:uuid-d; NAME:x) player/1.0 (MODEL:box)")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		return regexp.MustCompile(`Refresh" content="(\d+)"`).FindStringSubmatch(string(b))[1]
	}

	var set struct {
		Text string `json:"default_poll_text"`
		Own  bool   `json:"default_poll_own"`
	}
	five := map[string]any{"policy": map[string]any{"mode": "interval", "minutes": 5}}
	if code, body := root.do("PUT", "/api/settings/poll", five, &set); code != 200 || !set.Own {
		t.Fatalf("set default: %d %s", code, body)
	}
	if got := refresh(); got != "300" {
		t.Fatalf("refresh %s, want 300", got)
	}
	root.do("PUT", "/api/nodes/"+itoa(ivanov)+"/poll", map[string]any{"policy": map[string]any{"mode": "interval", "minutes": 30}}, nil)
	if got := refresh(); got != "1800" {
		t.Fatalf("client's own: refresh %s, want 1800", got)
	}
	root.do("PUT", "/api/nodes/"+itoa(ivanov)+"/poll", map[string]any{"policy": nil}, nil)
	root.do("PUT", "/api/settings/poll", map[string]any{"policy": nil}, &set)
	if got := refresh(); set.Own || got != "900" {
		t.Fatalf("back to -refresh: %s %+v", got, set)
	}
}

// Some players open pair.html in a WebView with the browser's own User-Agent, so
// the shared index must name the device in the page link for the code to show.
func TestPairPageWithoutPlayerAgent(t *testing.T) {
	e := newEnv(t)
	ph := httptest.NewServer(player.New(e.st, mustStorage(e), slog.New(slog.NewTextHandler(io.Discard, nil)), player.Config{RefreshSec: 900}))
	defer ph.Close()
	get := func(path, agent string) string {
		req, _ := http.NewRequest("GET", ph.URL+path, nil)
		req.Header.Set("User-Agent", agent)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		b, _ := io.ReadAll(resp.Body)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET %s: %d %s", path, resp.StatusCode, b)
		}
		return string(b)
	}
	smil := get("/p/index.smil", "GAPI/1.0 (UUID:uuid-web; NAME:x) player/1.0 (MODEL:box)")
	if !strings.Contains(smil, `src="pair.html?u=uuid-web"`) {
		t.Fatalf("pair page link without the device:\n%s", smil)
	}
	dev, err := e.st.DeviceByUUID(context.Background(), "uuid-web")
	if err != nil {
		t.Fatal(err)
	}
	if page := get("/p/pair.html?u=uuid-web", "Mozilla/5.0 (Linux; Android 7.1.1) WebView"); !strings.Contains(page, dev.PairingCode) {
		t.Fatalf("no code %s on the page", dev.PairingCode)
	}
}
