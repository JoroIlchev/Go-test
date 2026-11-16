package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Everything Marquee Player and the install page need must reach the player
// handler over HTTPS, with or without the public admin.
func TestPublicRoutesPlayer(t *testing.T) {
	player := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(299) })
	admin := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(298) })
	for _, rest := range []http.Handler{nil, admin} {
		h := public(player, rest)
		for _, p := range []string{"/m/x.mp4", "/p/index.smil", "/d/t/index.smil", "/api/v1/sync", "/app", "/app/marquee-player.apk"} {
			w := httptest.NewRecorder()
			h.ServeHTTP(w, httptest.NewRequest("GET", p, nil))
			if w.Code != 299 {
				t.Errorf("%s (admin=%v): got %d, want the player handler", p, rest != nil, w.Code)
			}
		}
	}
	w := httptest.NewRecorder()
	public(player, admin).ServeHTTP(w, httptest.NewRequest("GET", "/api/campaigns", nil))
	if w.Code != 298 {
		t.Errorf("/api/campaigns: got %d, want the admin handler", w.Code)
	}
}
