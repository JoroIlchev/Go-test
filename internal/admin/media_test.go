package admin_test

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"io"
	"log/slog"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"marquee/internal/media"
	"marquee/internal/player"
	"marquee/internal/store"
)

type mediaItem struct {
	ID          int64
	Name        string
	Kind        string
	Status      string
	SHA256      string `json:"sha256"`
	Ext         string
	Width       int
	Height      int
	VideoCodec  string `json:"video_codec"`
	DurationMS  int64  `json:"duration_ms"`
	HasThumb    bool   `json:"has_thumb"`
	OwnerName   string `json:"owner_name"`
	CanEdit     bool   `json:"can_edit"`
	Convertible bool
	Warnings    []media.Warning
}

func (c *client) upload(nodeID int64, filename string, data []byte) (int, []struct {
	Name  string
	Media *mediaItem
	Error string
}) {
	c.e.t.Helper()
	return c.uploadWith(nodeID, filename, data, "")
}

// uploadWith sends the file with a processing choice (JSON of media.Prep).
func (c *client) uploadWith(nodeID int64, filename string, data []byte, prep string) (int, []struct {
	Name  string
	Media *mediaItem
	Error string
}) {
	c.e.t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	if prep != "" {
		mw.WriteField("prep", prep)
	}
	fw, _ := mw.CreateFormFile("file", filename)
	fw.Write(data)
	mw.Close()
	req, _ := http.NewRequest("POST", c.e.srv.URL+"/api/media?node_id="+itoa(nodeID), &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("X-Marquee", "1")
	resp, err := c.http.Do(req)
	if err != nil {
		c.e.t.Fatal(err)
	}
	defer resp.Body.Close()
	var out []struct {
		Name  string
		Media *mediaItem
		Error string
	}
	if resp.StatusCode == 200 {
		json.NewDecoder(resp.Body).Decode(&out)
	}
	return resp.StatusCode, out
}

func (c *client) mediaList(nodeID int64) []mediaItem {
	c.e.t.Helper()
	var out struct{ Items []mediaItem }
	if code, body := c.do("GET", "/api/media?node_id="+itoa(nodeID), nil, &out); code != 200 {
		c.e.t.Fatalf("list media: %d %s", code, body)
	}
	return out.Items
}

// waitReady polls until the worker has processed item id.
func (c *client) waitReady(nodeID, id int64, timeout time.Duration) mediaItem {
	c.e.t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		for _, m := range c.mediaList(nodeID) {
			if m.ID == id && m.Status != "processing" {
				return m
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	c.e.t.Fatalf("media %d not processed within %s", id, timeout)
	return mediaItem{}
}

func testPNG(w, h int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for x := 0; x < w; x++ {
		img.Set(x, x%h, color.RGBA{200, 60, 20, 255})
	}
	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}

func TestMediaLibrary(t *testing.T) {
	e := newEnv(t)
	root := e.login("root@hub")
	ivanov := root.mustNode(nil, "client", "Ivanov")
	hotels := root.mustNode(p(ivanov), "group", "Hotels")
	cafes := root.mustNode(p(ivanov), "group", "Cafes")
	cafeC := root.mustNode(p(cafes), "site", "Cafe Center")
	petrov := root.mustNode(nil, "client", "Petrov")

	root.mustUser("ivan@x", p(ivanov), []store.Role{{NodeID: ivanov, Role: "admin"}})
	root.mustUser("maria@x", p(ivanov), []store.Role{{NodeID: hotels, Role: "admin"}})
	root.mustUser("petar@x", p(ivanov), []store.Role{{NodeID: cafeC, Role: "editor", CanCreate: true}})
	root.mustUser("viewer@x", p(ivanov), []store.Role{{NodeID: cafeC, Role: "editor"}})
	root.mustUser("pp@x", p(petrov), []store.Role{{NodeID: petrov, Role: "admin"}})
	ivan, maria, petar, viewer, pp := e.login("ivan@x"), e.login("maria@x"), e.login("petar@x"), e.login("viewer@x"), e.login("pp@x")

	// Ivan uploads the company logo at the top; Petar uploads a cafe photo.
	code, res := ivan.upload(ivanov, "logo.png", testPNG(640, 360))
	if code != 200 || len(res) != 1 || res[0].Media == nil {
		t.Fatalf("ivan upload: %d %+v", code, res)
	}
	logo := ivan.waitReady(ivanov, res[0].Media.ID, 5*time.Second)
	if logo.Status != "ready" || logo.Width != 640 || logo.Height != 360 || !logo.HasThumb {
		t.Fatalf("logo not processed: %+v", logo)
	}
	_, res = petar.upload(cafeC, "menu.png", testPNG(1280, 720))
	menu := petar.waitReady(cafeC, res[0].Media.ID, 5*time.Second)

	has := func(items []mediaItem, id int64) bool {
		for _, m := range items {
			if m.ID == id {
				return true
			}
		}
		return false
	}

	t.Run("inherited down, not sideways or across clients", func(t *testing.T) {
		at := petar.mediaList(cafeC)
		if !has(at, logo.ID) || !has(at, menu.ID) {
			t.Fatalf("petar should see logo (inherited) and menu: %+v", at)
		}
		for _, m := range at {
			if m.ID == logo.ID && m.CanEdit {
				t.Fatal("petar must not be able to edit the inherited logo")
			}
		}
		if got := maria.mediaList(hotels); !has(got, logo.ID) || has(got, menu.ID) {
			t.Fatalf("maria sees logo but not cafe menu: %+v", got)
		}
		if code, _ := pp.do("GET", "/api/media?node_id="+itoa(ivanov), nil, nil); code != 403 {
			t.Fatalf("other client lists ivanov media: %d", code)
		}
		if code, _ := pp.do("GET", "/api/media/"+itoa(logo.ID)+"/file", nil, nil); code != 404 {
			t.Fatalf("other client fetches ivanov file: %d", code)
		}
	})

	t.Run("rights", func(t *testing.T) {
		if code, _ := viewer.upload(cafeC, "x.png", testPNG(10, 10)); code != 403 {
			t.Fatalf("editor without create uploads: %d", code)
		}
		if code, _ := petar.do("DELETE", "/api/media/"+itoa(logo.ID), nil, nil); code != 403 {
			t.Fatalf("petar deletes inherited logo: %d", code)
		}
		if code, _ := petar.do("GET", "/api/media/"+itoa(logo.ID)+"/thumb", nil, nil); code != 200 {
			t.Fatalf("petar thumb of inherited logo: %d", code)
		}
		_, r := petar.upload(cafeC, "virus.exe", []byte("MZ"))
		if len(r) != 1 || r[0].Error == "" {
			t.Fatalf("exe accepted: %+v", r)
		}
	})

	t.Run("player serves library files only, by hash", func(t *testing.T) {
		ph := httptest.NewServer(player.New(e.st, mustStorage(e), slog.New(slog.NewTextHandler(io.Discard, nil)), player.Config{}))
		defer ph.Close()
		resp, err := http.Get(ph.URL + "/m/" + logo.SHA256 + ".png")
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != 200 || resp.Header.Get("Cache-Control") == "" {
			t.Fatalf("player media: %d %v", resp.StatusCode, resp.Header)
		}
		resp, _ = http.Get(ph.URL + "/m/" + logo.SHA256 + ".mp4")
		resp.Body.Close()
		if resp.StatusCode != 404 {
			t.Fatalf("wrong ext served: %d", resp.StatusCode)
		}
	})

	t.Run("duplicate upload shares the blob; delete keeps it until last user", func(t *testing.T) {
		_, r := ivan.upload(cafes, "logo-copy.png", testPNG(640, 360))
		dup := ivan.waitReady(cafes, r[0].Media.ID, 5*time.Second)
		if dup.SHA256 != logo.SHA256 {
			t.Fatal("same bytes should give the same hash")
		}
		blob := mustStorage(e).Path(logo.SHA256, "png")
		if code, _ := ivan.do("DELETE", "/api/media/"+itoa(dup.ID), nil, nil); code != 204 {
			t.Fatalf("delete dup: %d", code)
		}
		if _, err := os.Stat(blob); err != nil {
			t.Fatal("blob removed while the logo still uses it")
		}
		ivan.do("DELETE", "/api/media/"+itoa(logo.ID), nil, nil)
		if _, err := os.Stat(blob); !os.IsNotExist(err) {
			t.Fatal("orphan blob not removed")
		}
	})
}

// TestVideoCheckAndConvert needs ffmpeg with libx265 and libx264; it is skipped otherwise.
func TestVideoCheckAndConvert(t *testing.T) {
	tools := media.FindTools("")
	if tools.FFmpeg == "" || tools.FFprobe == "" {
		t.Skip("ffmpeg not available")
	}
	src := filepath.Join(t.TempDir(), "uhd.mp4")
	out, err := exec.Command(tools.FFmpeg, "-v", "error", "-f", "lavfi", "-i", "testsrc=size=3840x2160:rate=60",
		"-t", "1", "-c:v", "libx265", "-pix_fmt", "yuv420p", "-tag:v", "hvc1", src).CombinedOutput()
	if err != nil {
		t.Skipf("cannot make HEVC sample: %v %s", err, out)
	}
	data, _ := os.ReadFile(src)

	e := newEnv(t)
	root := e.login("root@hub")
	client := root.mustNode(nil, "client", "C")
	_, res := root.upload(client, "uhd.mp4", data)
	v := root.waitReady(client, res[0].Media.ID, 30*time.Second)
	codes := map[string]bool{}
	for _, w := range v.Warnings {
		codes[w.Code] = true
	}
	if v.VideoCodec != "hevc" || !codes["codec"] || !codes["resolution"] || !codes["fps"] || !v.Convertible || !v.HasThumb {
		t.Fatalf("4K HEVC not flagged: %+v", v)
	}

	var conv struct{ ID int64 }
	if code, body := root.do("POST", "/api/media/"+itoa(v.ID)+"/convert", nil, &conv); code != 202 {
		t.Fatalf("convert: %d %s", code, body)
	}
	c := root.waitReady(client, conv.ID, 120*time.Second)
	if c.Status != "ready" || c.VideoCodec != "h264" || c.Width != 1920 || c.Height != 1080 || len(c.Warnings) != 0 {
		t.Fatalf("converted video not clean: %+v", c)
	}
}

func mustStorage(e *env) *media.Storage {
	s, err := media.NewStorage(e.mediaDir)
	if err != nil {
		e.t.Fatal(err)
	}
	return s
}
