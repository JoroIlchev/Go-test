// Package player serves everything a player fetches: the SMIL index
// and, later, media. Nothing here changes content; it only reads and records
// that a device was seen.
package player

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"html/template"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"

	"marquee/internal/campaign"
	"marquee/internal/media"
	"marquee/internal/poll"
	"marquee/internal/store"
)

type Handler struct {
	store   *store.Store
	storage *media.Storage
	log     *slog.Logger
	cfg     Config
	mux     *http.ServeMux
}

type Config struct {
	PublicURL  string         // prefixes media URLs in the index
	Loc        *time.Location // time zone of schedules
	RefreshSec int            // how often players re-check the index
	// DefaultPoll, when set, gives the hub-wide schedule (from the settings)
	// in place of RefreshSec.
	DefaultPoll func(context.Context) poll.Policy
	// AppFile is the Marquee Player APK offered at /app (see app.go).
	AppFile string
}

func New(st *store.Store, storage *media.Storage, log *slog.Logger, cfg Config) *Handler {
	cfg.PublicURL = strings.TrimRight(cfg.PublicURL, "/")
	if cfg.Loc == nil {
		cfg.Loc = time.Local
	}
	if cfg.RefreshSec < 60 {
		cfg.RefreshSec = 60 // SMIL players' usual minimum
	}
	h := &Handler{store: st, storage: storage, log: log, cfg: cfg, mux: http.NewServeMux()}
	// Media by content hash. The name never changes for given bytes, so players
	// cache forever and a new ad is simply a new URL.
	h.mux.HandleFunc("GET /m/{file}", h.mediaFile)
	// Shared URL for every player: the device is identified by the UUID in its User-Agent.
	h.mux.HandleFunc("GET /p/index.smil", h.sharedIndex)
	h.mux.HandleFunc("GET /p/pair.html", h.pairPage)
	// Per-device secret URL, for players configured with their own link.
	h.mux.HandleFunc("GET /d/{token}/index.smil", h.deviceIndex)
	h.mux.HandleFunc("GET /d/{token}/pair.html", h.pairPage)
	// Marquee Player (Android app): see api.go.
	h.mux.HandleFunc("POST /api/v1/sync", h.sync)
	h.mux.HandleFunc("GET /app", h.appPage)
	h.mux.HandleFunc("GET /app/"+AppName, h.appFile)
	return h
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) { h.mux.ServeHTTP(w, r) }

func (h *Handler) sharedIndex(w http.ResponseWriter, r *http.Request) {
	a := parseAgent(r.UserAgent(), r.Header.Get("X-Signage-Agent"))
	if a.UUID == "" {
		http.Error(w, "unknown player", http.StatusBadRequest)
		return
	}
	d, err := h.store.DeviceByUUID(r.Context(), a.UUID)
	if errors.Is(err, store.ErrNotFound) {
		d, err = h.store.RegisterDevice(r.Context(), a.UUID, a.Name, a.Raw, clientIP(r))
		if err == nil {
			h.log.Info("Ново устройство чака свързване", "код", d.PairingCode, "име", a.Name, "uuid", a.UUID, "ip", clientIP(r))
		}
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	h.serveIndex(w, r, d, a)
}

func (h *Handler) deviceIndex(w http.ResponseWriter, r *http.Request) {
	d, err := h.store.DeviceByToken(r.Context(), r.PathValue("token"))
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	if d.AuthMode == "basic" && !checkBasicAuth(r, d) {
		w.Header().Set("WWW-Authenticate", `Basic realm="marquee"`)
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	h.serveIndex(w, r, d, parseAgent(r.UserAgent(), r.Header.Get("X-Signage-Agent")))
}

func (h *Handler) serveIndex(w http.ResponseWriter, r *http.Request, d *store.Device, a agent) {
	if err := h.store.TouchDevice(r.Context(), d.ID, a.Raw, clientIP(r)); err != nil {
		h.log.Warn("Посещението на устройството не се записа", "устройство", d.ID, "грешка", err)
	}
	if d.Status == "blocked" {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	now := time.Now()
	// Pending devices come back every minute so pairing shows up quickly.
	refresh := poll.MinRefreshSec
	if d.Status == "active" && d.NodeID.Valid {
		site, err := h.store.Node(r.Context(), d.NodeID.Int64)
		if err != nil {
			h.fail(w, err)
			return
		}
		policy, err := h.policyFor(r.Context(), site)
		if err != nil {
			h.fail(w, err)
			return
		}
		refresh = policy.RefreshSec(now, h.cfg.Loc, d.ID)
		body, versions, labels, err := h.campaignIndex(r.Context(), site, uint64(d.ID), now, refresh, requestBase(r, h.cfg.PublicURL))
		if err != nil {
			h.fail(w, err)
			return
		}
		changed, err := h.store.SetServedVersions(r.Context(), d.ID, versions)
		if err != nil {
			h.log.Warn("Не се записа какво е получило устройството", "устройство", d.ID, "грешка", err)
		}
		// Polls themselves are not worth a line; a change of content is.
		if changed {
			what := strings.Join(labels, ", ")
			if what == "" {
				what = "няма активни кампании"
			}
			h.log.Info("Устройството получи ново съдържание", "устройство", deviceLabel(d, site), "кампании", what,
				"следваща проверка", now.Add(time.Duration(refresh)*time.Second).In(h.cfg.Loc).Format("02.01 15:04"))
		}
		if len(versions) > 0 {
			h.setNextCheck(r, d, now, refresh)
			serveVersioned(w, r, "application/smil+xml", body)
			return
		}
	}
	// Pending devices show their pairing code; active ones without any
	// scheduled campaign show "connected, waiting for content".
	// Some players open the page in a WebView that sends its own User-Agent, not
	// the player's, so on the shared URL the page names the device itself.
	page := "pair.html"
	if r.PathValue("token") == "" && a.UUID != "" {
		page += "?u=" + url.QueryEscape(a.UUID)
	}
	h.setNextCheck(r, d, now, refresh)
	serveVersioned(w, r, "application/smil+xml", statusSMIL(refresh, page))
}

func (h *Handler) setNextCheck(r *http.Request, d *store.Device, now time.Time, refresh int) {
	if err := h.store.SetNextCheck(r.Context(), d.ID, now.Add(time.Duration(refresh)*time.Second)); err != nil {
		h.log.Warn("Не се записа следващата проверка", "устройство", d.ID, "грешка", err)
	}
}

// policyFor resolves the polling policy of a site from its nearest configured ancestor.
func (h *Handler) policyFor(ctx context.Context, site *store.Node) (poll.Policy, error) {
	chain := site.AncestorIDs()
	set, err := h.store.NodePolls(ctx, chain)
	if err != nil {
		return poll.Policy{}, err
	}
	def := poll.Interval(time.Duration(h.cfg.RefreshSec) * time.Second)
	if h.cfg.DefaultPoll != nil {
		def = h.cfg.DefaultPoll(ctx)
	}
	p, _ := poll.Resolve(chain, set, def)
	return p, nil
}

func (h *Handler) pairPage(w http.ResponseWriter, r *http.Request) {
	var (
		d   *store.Device
		err error
	)
	if tok := r.PathValue("token"); tok != "" {
		d, err = h.store.DeviceByToken(r.Context(), tok)
	} else {
		uuid := r.URL.Query().Get("u")
		if uuid == "" {
			uuid = parseAgent(r.UserAgent(), r.Header.Get("X-Signage-Agent")).UUID
		}
		d, err = h.store.DeviceByUUID(r.Context(), uuid)
	}
	if errors.Is(err, store.ErrNotFound) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	var buf bytes.Buffer
	if err := pairTmpl.Execute(&buf, d); err != nil {
		h.fail(w, err)
		return
	}
	serveVersioned(w, r, "text/html; charset=utf-8", buf.Bytes())
}

// campaignIndex builds the SMIL for every live campaign targeting the device's
// site or one of its ancestors.
// labels name the included versions ("Обедно меню v2") for the log.
func (h *Handler) campaignIndex(ctx context.Context, site *store.Node, seed uint64, now time.Time, refresh int, base string) (body []byte, included []int64, labels []string, err error) {
	playables, media, labelOf, err := h.live(ctx, site)
	if err != nil || len(playables) == 0 {
		return nil, nil, nil, err
	}
	b := campaign.Builder{
		Loc:        h.cfg.Loc,
		RefreshSec: refresh,
		Seed:       seed,
		MediaURL:   func(m *store.Media) string { return base + "/m/" + m.FileName() },
	}
	body, included = b.Build(now, playables, media)
	for _, id := range included {
		labels = append(labels, labelOf[id])
	}
	return body, included, labels, nil
}

// live resolves the live campaign versions targeting the site or one of its
// ancestors, the media they use and a label per version for the log.
func (h *Handler) live(ctx context.Context, site *store.Node) ([]campaign.Playable, map[int64]*store.Media, map[int64]string, error) {
	onPath := map[int64]bool{}
	for _, id := range site.AncestorIDs() {
		onPath[id] = true
	}
	campaigns, versions, err := h.store.LiveCampaigns(ctx, site.ClientID)
	if err != nil {
		return nil, nil, nil, err
	}
	names := map[int64]string{}
	for _, c := range campaigns {
		names[c.ID] = c.Name
	}
	labelOf := map[int64]string{}
	var playables []campaign.Playable
	var mediaIDs []int64
	for _, v := range versions {
		spec, err := campaign.ParseSpec(v.Spec)
		if err != nil {
			h.log.Warn("Повредена кампания, пропуска се", "версия", v.ID, "грешка", err)
			continue
		}
		targeted := false
		for _, t := range spec.Targets {
			if onPath[t] {
				targeted = true
				break
			}
		}
		if !targeted {
			continue
		}
		rule, err := spec.Rule(h.cfg.Loc)
		if err != nil {
			h.log.Warn("Невалиден график на кампания, пропуска се", "версия", v.ID, "грешка", err)
			continue
		}
		playables = append(playables, campaign.Playable{VersionID: v.ID, Spec: spec, Rule: rule})
		mediaIDs = append(mediaIDs, spec.MediaIDs()...)
		labelOf[v.ID] = fmt.Sprintf("%s v%d", names[v.CampaignID], v.Number)
	}
	if len(playables) == 0 {
		return nil, nil, nil, nil
	}
	media, err := h.store.MediaByIDs(ctx, mediaIDs)
	if err != nil {
		return nil, nil, nil, err
	}
	return playables, media, labelOf, nil
}

// requestBase is the address the device used to reach us. Media URLs built
// on it work wherever the device sits (LAN IP, public IP, domain), without
// configuration; fallback serves requests without a Host header.
func requestBase(r *http.Request, fallback string) string {
	if r.Host == "" {
		return fallback
	}
	scheme := "http"
	// A tunnel or reverse proxy (cloudflared, nginx) ends TLS itself and says so
	// here. Trusting it is harmless: a liar only gets https links it cannot use.
	if r.TLS != nil || strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https") {
		scheme = "https"
	}
	return scheme + "://" + r.Host
}

func deviceLabel(d *store.Device, site *store.Node) string {
	name := d.Name
	if name == "" {
		name = fmt.Sprintf("#%d", d.ID)
	}
	if site != nil {
		return site.Name + " › " + name
	}
	return name
}

var mediaName = regexp.MustCompile(`^([0-9a-f]{64})\.([a-z0-9]{2,4})$`)

func (h *Handler) mediaFile(w http.ResponseWriter, r *http.Request) {
	m := mediaName.FindStringSubmatch(r.PathValue("file"))
	if m == nil {
		http.NotFound(w, r)
		return
	}
	sha, ext := m[1], m[2]
	ok, err := h.store.MediaFileExists(r.Context(), sha, ext)
	if err != nil {
		h.fail(w, err)
		return
	}
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", media.MimeOf(ext))
	w.Header().Set("ETag", `"`+sha+`"`)
	w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
	http.ServeFile(w, r, h.storage.Path(sha, ext))
}

func (h *Handler) fail(w http.ResponseWriter, err error) {
	// The device hung up mid-request; nothing went wrong on our side.
	if errors.Is(err, context.Canceled) {
		return
	}
	h.log.Error("Грешка при заявка от устройство", "грешка", err)
	http.Error(w, "internal error", http.StatusInternalServerError)
}

// serveVersioned sets a content-hash ETag so a player's conditional HEAD/GET
// gets a 304 when nothing changed; without it the player re-downloads every poll.
func serveVersioned(w http.ResponseWriter, r *http.Request, contentType string, body []byte) {
	sum := sha256.Sum256(body)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("ETag", `"`+hex.EncodeToString(sum[:16])+`"`)
	w.Header().Set("Cache-Control", "no-cache")
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}

func checkBasicAuth(r *http.Request, d *store.Device) bool {
	user, pass, ok := r.BasicAuth()
	if !ok || user != d.AuthUser || d.AuthPassHash == "" {
		return false
	}
	return bcrypt.CompareHashAndPassword([]byte(d.AuthPassHash), []byte(pass)) == nil
}

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func statusSMIL(refresh int, page string) []byte {
	return []byte(fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8"?>
<smil>
  <head>
    <meta http-equiv="Refresh" content="%d"/>
  </head>
  <body>
    <seq repeatCount="indefinite">
      <ref src="%s" type="text/html" dur="60s"/>
    </seq>
  </body>
</smil>
`, refresh, page))
}

var pairTmpl = template.Must(template.New("pair").Parse(`<!doctype html>
<html><head><meta charset="utf-8"><title>Marquee</title>
<style>
html,body{margin:0;height:100%;background:#120c1f;color:#eee;font-family:sans-serif}
body{display:flex;flex-direction:column;align-items:center;justify-content:center;gap:2vh;
background:radial-gradient(circle at 50% 45%,#2e1a52 0,#120c1f 65%)}
.logo{position:fixed;top:5vh;left:5vh;display:flex;align-items:center;gap:1.6vh;font-size:4.2vh;font-weight:700;letter-spacing:-.02em}
.logo svg{width:6vh;height:6vh}
.code{font-size:18vh;font-weight:700;letter-spacing:2vh;color:#c4b5fd}
.hint{font-size:4vh;opacity:.7}
</style></head><body>
<div class="logo"><svg viewBox="0 0 64 64"><rect width="64" height="64" rx="16" fill="#7c3aed"/><circle cx="32" cy="32" r="22" fill="none" stroke="#fff" stroke-width="3" opacity=".28"/><circle cx="32" cy="32" r="14" fill="none" stroke="#fff" stroke-width="3.5" opacity=".6"/><circle cx="32" cy="32" r="7" fill="#fff"/></svg>marquee</div>
{{if eq .Status "active"}}
<div class="hint">Устройството е свързано</div>
<div class="code" style="font-size:8vh;letter-spacing:0">{{.Name}}</div>
<div class="hint">Очаква съдържание</div>
{{else}}
<div class="hint">Код за свързване</div>
<div class="code">{{.PairingCode}}</div>
<div class="hint">Въведете кода в marquee</div>
{{end}}
</body></html>`))
