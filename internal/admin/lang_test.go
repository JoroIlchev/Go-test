package admin_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"marquee/internal/i18n"
	"marquee/internal/player"
)

// The hub language from the settings reaches players (the manifest)
// and what the hub writes itself.
func TestHubLanguage(t *testing.T) {
	defer i18n.SetHub("bg")
	e := newEnv(t)
	root := e.login("root@hub")
	ph := httptest.NewServer(player.New(e.st, mustStorage(e), slog.New(slog.NewTextHandler(io.Discard, nil)), player.Config{RefreshSec: 900}))
	defer ph.Close()

	type man struct{ Version, Lang string }
	sync := func() man {
		body, _ := json.Marshal(map[string]any{"info": map[string]any{}, "version": ""})
		req, _ := http.NewRequest("POST", ph.URL+"/api/v1/sync", bytes.NewReader(body))
		req.Header.Set("X-Device-Id", "3f6c1d0e-8a7b-4c2d-9e1f-0a1b2c3d4e5f")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		defer resp.Body.Close()
		var m man
		json.NewDecoder(resp.Body).Decode(&m)
		return m
	}
	bg := sync()
	if bg.Lang != "bg" {
		t.Fatalf("default lang %q", bg.Lang)
	}

	set := map[string]any{"smtp_port": 587, "alert_emails": []string{}, "alert_grace_min": 10, "backup_keep": 7, "lang": "en"}
	var got map[string]any
	if code, body := root.do("PUT", "/api/settings", set, &got); code != 200 || got["lang"] != "en" {
		t.Fatalf("save settings: %d %s", code, body)
	}
	en := sync()
	if en.Lang != "en" || en.Version == bg.Version {
		t.Fatalf("after switching to English: %+v (was %+v)", en, bg)
	}
	if code, body := root.do("GET", "/api/billing?format=csv", nil, nil); code != 200 || !strings.Contains(body, "Client;Device-days") {
		t.Fatalf("csv: %d %s", code, body)
	}

	set["lang"] = "xx" // anything else is Bulgarian
	root.do("PUT", "/api/settings", set, &got)
	if got["lang"] != "bg" || i18n.Hub() != "bg" {
		t.Fatalf("lang %v, hub %s", got["lang"], i18n.Hub())
	}
}
