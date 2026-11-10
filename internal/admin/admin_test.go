package admin_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"testing/fstest"

	"marquee/internal/admin"
	"marquee/internal/auth"
	"marquee/internal/media"
	"marquee/internal/store"
)

const pw = "correct-horse-battery"

type env struct {
	t        *testing.T
	st       *store.Store
	srv      *httptest.Server
	mediaDir string
}

func newEnv(t *testing.T) *env {
	t.Helper()
	st, err := store.Open(context.Background(), t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	web := fstest.MapFS{"index.html": {Data: []byte("ui")}}
	mediaDir := filepath.Join(t.TempDir(), "media")
	storage, err := media.NewStorage(mediaDir)
	if err != nil {
		t.Fatal(err)
	}
	worker := media.NewWorker(st, storage, media.FindTools(""), log)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go worker.Run(ctx)
	srv := httptest.NewServer(admin.New(st, log, admin.Config{
		Web: web, Storage: storage, Worker: worker, MaxUpload: 10 << 20,
	}))
	t.Cleanup(srv.Close)
	hash, _ := auth.HashPassword(pw)
	if _, err := st.CreateUser(context.Background(), "root@hub", "Root", hash, true, nil, nil, nil); err != nil {
		t.Fatal(err)
	}
	return &env{t: t, st: st, srv: srv, mediaDir: mediaDir}
}

type client struct {
	e    *env
	http *http.Client
}

func (e *env) login(email string) *client {
	e.t.Helper()
	jar, _ := cookiejar.New(nil)
	c := &client{e: e, http: &http.Client{Jar: jar}}
	if code, body := c.do("POST", "/api/login", map[string]string{"email": email, "password": pw}, nil); code != 200 {
		e.t.Fatalf("login %s: %d %s", email, code, body)
	}
	return c
}

func (c *client) do(method, path string, in, out any) (int, string) {
	c.e.t.Helper()
	var body io.Reader
	if in != nil {
		b, _ := json.Marshal(in)
		body = bytes.NewReader(b)
	}
	req, _ := http.NewRequest(method, c.e.srv.URL+path, body)
	req.Header.Set("X-Marquee", "1")
	resp, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 {
		if err := json.Unmarshal(raw, out); err != nil {
			c.e.t.Fatalf("%s %s: decode %v: %s", method, path, err, raw)
		}
	}
	return resp.StatusCode, string(raw)
}

func (c *client) mustNode(parent *int64, kind, name string) int64 {
	c.e.t.Helper()
	var n struct{ ID int64 }
	if code, body := c.do("POST", "/api/nodes", map[string]any{"parent_id": parent, "kind": kind, "name": name}, &n); code != 201 {
		c.e.t.Fatalf("create node %s: %d %s", name, code, body)
	}
	return n.ID
}

func (c *client) mustUser(email string, clientID *int64, roles []store.Role) int64 {
	c.e.t.Helper()
	var u struct{ ID int64 }
	in := map[string]any{"email": email, "name": email, "password": pw, "client_id": clientID, "roles": roles}
	if code, body := c.do("POST", "/api/users", in, &u); code != 201 {
		c.e.t.Fatalf("create user %s: %d %s", email, code, body)
	}
	return u.ID
}

type treeResp struct {
	Nodes []struct {
		ID   int64
		Name string
	}
	Devices []struct {
		ID          int64
		Name        string
		PairingCode string `json:"pairing_code"`
		URL         string `json:"url"`
	}
}

func names(t treeResp) map[string]bool {
	m := map[string]bool{}
	for _, n := range t.Nodes {
		m[n.Name] = true
	}
	return m
}

func p(v int64) *int64 { return &v }

func TestTreePermissionsAndIsolation(t *testing.T) {
	e := newEnv(t)
	root := e.login("root@hub")

	ivanov := root.mustNode(nil, "client", "Ivanov")
	hotels := root.mustNode(p(ivanov), "group", "Hotels")
	hotelA := root.mustNode(p(hotels), "site", "Hotel A")
	cafes := root.mustNode(p(ivanov), "group", "Cafes")
	cafeC := root.mustNode(p(cafes), "site", "Cafe Center")
	petrov := root.mustNode(nil, "client", "Petrov")
	petrovSite := root.mustNode(p(petrov), "site", "Petrov Shop")

	root.mustUser("ivan@x", p(ivanov), []store.Role{{NodeID: ivanov, Role: "admin"}})
	mariaID := root.mustUser("maria@x", p(ivanov), []store.Role{{NodeID: hotels, Role: "admin"}})
	root.mustUser("petar@x", p(ivanov), []store.Role{{NodeID: cafeC, Role: "editor", CanEdit: true, NeedsApproval: true}})

	// A device registered by a player and paired by the super admin onto Hotel A.
	dev, err := e.st.RegisterDevice(context.Background(), "uuid-1", "", "GAPI", "1.2.3.4")
	if err != nil {
		t.Fatal(err)
	}
	if code, body := root.do("POST", "/api/devices/pair",
		map[string]any{"code": dev.PairingCode, "site_id": hotelA, "name": "Lobby"}, nil); code != 200 {
		t.Fatalf("pair: %d %s", code, body)
	}

	ivan, maria, petar := e.login("ivan@x"), e.login("maria@x"), e.login("petar@x")

	t.Run("client admin sees own client only", func(t *testing.T) {
		var tr treeResp
		ivan.do("GET", "/api/tree", nil, &tr)
		got := names(tr)
		if !got["Ivanov"] || !got["Cafe Center"] || got["Petrov"] || got["Petrov Shop"] {
			t.Fatalf("ivan tree: %v", got)
		}
	})

	t.Run("division admin sees own subtree only, without device secrets", func(t *testing.T) {
		var tr treeResp
		maria.do("GET", "/api/tree", nil, &tr)
		got := names(tr)
		if !got["Hotels"] || !got["Hotel A"] || got["Ivanov"] || got["Cafes"] {
			t.Fatalf("maria tree: %v", got)
		}
		if len(tr.Devices) != 1 || tr.Devices[0].URL != "" || tr.Devices[0].PairingCode != "" {
			t.Fatalf("maria devices leak secrets or are missing: %+v", tr.Devices)
		}
	})

	t.Run("super admin sees device secret URL", func(t *testing.T) {
		var tr treeResp
		root.do("GET", "/api/tree", nil, &tr)
		if len(tr.Devices) != 1 || tr.Devices[0].URL == "" {
			t.Fatalf("root devices: %+v", tr.Devices)
		}
	})

	t.Run("cross-client and out-of-subtree writes are refused", func(t *testing.T) {
		cases := []struct {
			name string
			c    *client
			path string
			in   any
		}{
			{"ivan node in petrov", ivan, "/api/nodes", map[string]any{"parent_id": petrov, "kind": "site", "name": "x"}},
			{"ivan user in petrov", ivan, "/api/users", map[string]any{"email": "a@x", "password": pw, "roles": []store.Role{{NodeID: petrovSite, Role: "admin"}}}},
			{"maria node in cafes", maria, "/api/nodes", map[string]any{"parent_id": cafes, "kind": "site", "name": "x"}},
			{"maria user in cafes", maria, "/api/users", map[string]any{"email": "b@x", "password": pw, "roles": []store.Role{{NodeID: cafeC, Role: "editor"}}}},
			{"maria new client", maria, "/api/nodes", map[string]any{"parent_id": nil, "kind": "client", "name": "x"}},
			{"petar creates node", petar, "/api/nodes", map[string]any{"parent_id": cafeC, "kind": "site", "name": "x"}},
			{"ivan pairs device", ivan, "/api/devices/pair", map[string]any{"code": "AAAAAA", "site_id": hotelA, "name": "x"}},
		}
		for _, tc := range cases {
			if code, body := tc.c.do("POST", tc.path, tc.in, nil); code != http.StatusForbidden {
				t.Errorf("%s: want 403, got %d %s", tc.name, code, body)
			}
		}
	})

	t.Run("division admin manages users below, not above or self", func(t *testing.T) {
		maria.mustUser("editor@x", nil, []store.Role{{NodeID: hotelA, Role: "editor", CanCreate: true}})
		var users []struct{ Email string }
		maria.do("GET", "/api/users", nil, &users)
		for _, u := range users {
			if u.Email == "ivan@x" || u.Email == "petar@x" {
				t.Fatalf("maria lists %s", u.Email)
			}
		}
		var ivanID int64
		var all []struct {
			ID    int64
			Email string
		}
		ivan.do("GET", "/api/users", nil, &all)
		for _, u := range all {
			if u.Email == "ivan@x" {
				ivanID = u.ID
			}
		}
		if code, _ := maria.do("PATCH", "/api/users/"+itoa(ivanID), map[string]any{"active": false}, nil); code != 403 {
			t.Fatalf("maria deactivates ivan: %d", code)
		}
		if code, _ := maria.do("PATCH", "/api/users/"+itoa(mariaID),
			map[string]any{"roles": []store.Role{{NodeID: ivanov, Role: "admin"}}}, nil); code != 403 {
			t.Fatalf("maria escalates herself: %d", code)
		}
		if code, _ := maria.do("PATCH", "/api/users/"+itoa(mariaID), map[string]any{"name": "Maria P"}, nil); code != 200 {
			t.Fatalf("maria renames herself: %d", code)
		}
	})
}

func TestCSRFHeaderRequired(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Post(e.srv.URL+"/api/login", "application/json",
		bytes.NewReader([]byte(`{"email":"root@hub","password":"`+pw+`"}`)))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Fatalf("want 403 without X-Marquee, got %d", resp.StatusCode)
	}
}

func TestLoginLockout(t *testing.T) {
	e := newEnv(t)
	jar, _ := cookiejar.New(nil)
	c := &client{e: e, http: &http.Client{Jar: jar}}
	for i := 0; i < 5; i++ {
		c.do("POST", "/api/login", map[string]string{"email": "root@hub", "password": "wrong-password"}, nil)
	}
	if code, _ := c.do("POST", "/api/login", map[string]string{"email": "root@hub", "password": pw}, nil); code != http.StatusTooManyRequests {
		t.Fatalf("want 429 after 5 failures, got %d", code)
	}
}

func TestUnauthenticated(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Get(e.srv.URL + "/api/tree")
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}
}

func itoa(v int64) string { b, _ := json.Marshal(v); return string(b) }
