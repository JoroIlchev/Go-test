// Package admin is the management API and web UI: login, the organisation
// tree, devices and users. Every handler checks rights through auth.Can.
package admin

import (
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"time"

	"marquee/internal/auth"
	"marquee/internal/logx"
	"marquee/internal/media"
	"marquee/internal/ops"
	"marquee/internal/poll"
	"marquee/internal/store"
)

const (
	cookieName = "marquee_session"
	sessionTTL = 12 * time.Hour
	// Mutating requests must carry this header. Browsers cannot add a custom
	// header cross-site without a CORS preflight, which we never allow.
	csrfHeader = "X-Marquee"
)

type Server struct {
	store        *store.Store
	log          *slog.Logger
	limiter      *auth.Limiter
	secureCookie bool
	publicURL    string
	playerPort   string
	playerScheme string
	logs         *logx.DailyFile
	storage      *media.Storage
	worker       *media.Worker
	maxUpload    int64
	loc          *time.Location
	defaultPoll  poll.Policy
	backup       *ops.Backup
	mux          *http.ServeMux
}

type Config struct {
	SecureCookie bool
	// PublicURL, when set (by a flag), fixes the hub address put into device
	// links; otherwise it comes from the settings, else this machine's IP.
	PublicURL string
	// PlayerPort and PlayerScheme describe the players' listener, for the
	// detected address.
	PlayerPort   string
	PlayerScheme string
	Logs         *logx.DailyFile
	Web          fs.FS
	Storage      *media.Storage
	Worker       *media.Worker
	MaxUpload    int64
	Loc          *time.Location // time zone campaign schedules are entered in
	// DefaultPoll applies to devices under nodes without their own policy.
	DefaultPoll poll.Policy
	Backup      *ops.Backup
}

func New(st *store.Store, log *slog.Logger, cfg Config) *Server {
	s := &Server{
		store:        st,
		log:          log,
		limiter:      auth.NewLimiter(5, 15*time.Minute),
		secureCookie: cfg.SecureCookie,
		publicURL:    strings.TrimRight(cfg.PublicURL, "/"),
		playerPort:   cfg.PlayerPort,
		playerScheme: cfg.PlayerScheme,
		logs:         cfg.Logs,
		storage:      cfg.Storage,
		worker:       cfg.Worker,
		maxUpload:    cfg.MaxUpload,
		loc:          cfg.Loc,
		defaultPoll:  cfg.DefaultPoll,
		backup:       cfg.Backup,
		mux:          http.NewServeMux(),
	}
	if s.playerScheme == "" {
		s.playerScheme, s.playerPort = "http", "8080"
	}
	if s.defaultPoll.Mode == "" {
		s.defaultPoll = poll.Interval(15 * time.Minute)
	}
	if s.loc == nil {
		s.loc = time.Local
	}
	web := cfg.Web
	s.mux.HandleFunc("POST /api/login", s.login)
	s.mux.HandleFunc("POST /api/logout", s.logout)
	s.mux.Handle("GET /api/me", s.authed(s.me))

	s.mux.Handle("GET /api/tree", s.authed(s.tree))
	s.mux.Handle("POST /api/nodes", s.authed(s.createNode))
	s.mux.Handle("PATCH /api/nodes/{id}", s.authed(s.renameNode))
	s.mux.Handle("DELETE /api/nodes/{id}", s.authed(s.deleteNode))

	s.mux.Handle("GET /api/devices/pending", s.authed(s.pendingDevices))
	s.mux.Handle("POST /api/devices/pair", s.authed(s.pairDevice))
	s.mux.Handle("POST /api/devices", s.authed(s.createLinkDevice))
	s.mux.Handle("PATCH /api/devices/{id}", s.authed(s.updateDevice))

	s.mux.Handle("GET /api/users", s.authed(s.listUsers))
	s.mux.Handle("POST /api/users", s.authed(s.createUser))
	s.mux.Handle("PATCH /api/users/{id}", s.authed(s.updateUser))

	s.mux.Handle("GET /api/media", s.authed(s.listMedia))
	s.mux.Handle("POST /api/media", s.authed(s.uploadMedia))
	s.mux.Handle("GET /api/media/{id}/thumb", s.authed(s.mediaThumb))
	s.mux.Handle("GET /api/media/{id}/file", s.authed(s.mediaFile))
	s.mux.Handle("PATCH /api/media/{id}", s.authed(s.renameMedia))
	s.mux.Handle("DELETE /api/media/{id}", s.authed(s.deleteMedia))
	s.mux.Handle("POST /api/media/{id}/convert", s.authed(s.convertMedia))
	s.mux.Handle("POST /api/media/render", s.authed(s.renderSlideshow))

	s.mux.Handle("GET /api/campaigns", s.authed(s.listCampaigns))
	s.mux.Handle("POST /api/campaigns", s.authed(s.createCampaign))
	s.mux.Handle("GET /api/campaigns/{id}", s.authed(s.getCampaign))
	s.mux.Handle("PUT /api/campaigns/{id}/draft", s.authed(s.saveDraft))
	s.mux.Handle("POST /api/campaigns/{id}/publish", s.authed(s.publishCampaign))
	s.mux.Handle("POST /api/campaigns/{id}/versions/{vid}/review", s.authed(s.reviewVersion))
	s.mux.Handle("POST /api/campaigns/{id}/live", s.authed(s.setLive))
	s.mux.Handle("DELETE /api/campaigns/{id}", s.authed(s.deleteCampaign))
	s.mux.Handle("POST /api/campaigns/sequence", s.authed(s.campaignSequence))
	s.mux.Handle("GET /api/approvals", s.authed(s.approvals))

	s.mux.Handle("GET /api/nodes/{id}/poll", s.authed(s.nodePoll))
	s.mux.Handle("PUT /api/nodes/{id}/poll", s.authed(s.setNodePoll))
	s.mux.Handle("PUT /api/settings/poll", s.superOnly(s.setDefaultPoll))
	s.mux.Handle("GET /api/settings", s.superOnly(s.getSettings))
	s.mux.Handle("PUT /api/settings", s.superOnly(s.putSettings))
	s.mux.Handle("POST /api/settings/test-mail", s.superOnly(s.testMail))
	s.mux.Handle("POST /api/backups", s.superOnly(s.backupNow))
	s.mux.Handle("GET /api/billing", s.staffWith(func(u *store.User) bool { return u.CanBilling })(s.billing))
	s.mux.Handle("GET /api/logs", s.staffWith(func(u *store.User) bool { return u.CanLogs })(s.viewLogs))

	s.mux.Handle("GET /api/staff", s.superOnly(s.listStaff))
	s.mux.Handle("POST /api/staff", s.superOnly(s.createStaff))
	s.mux.Handle("PATCH /api/staff/{id}", s.superOnly(s.updateStaff))

	s.mux.Handle("/", spaHandler(web))
	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "same-origin")
	if r.Method != http.MethodGet && r.Method != http.MethodHead && r.Header.Get(csrfHeader) == "" {
		writeErr(w, http.StatusForbidden, "missing "+csrfHeader+" header")
		return
	}
	s.mux.ServeHTTP(w, r)
}

// spaHandler serves the built UI and falls back to index.html for client-side routes.
func spaHandler(web fs.FS) http.Handler {
	files := http.FileServerFS(web)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		name := strings.TrimPrefix(r.URL.Path, "/")
		if name == "" {
			name = "index.html"
		}
		if _, err := fs.Stat(web, name); err != nil {
			r = r.Clone(r.Context())
			r.URL.Path = "/"
			name = "index.html"
		}
		// Built assets carry a content hash in their name; index.html does not and
		// must be revalidated, or browsers keep showing the previous UI after an update.
		if strings.HasPrefix(name, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		files.ServeHTTP(w, r)
	})
}

// --- session plumbing ---

type ctxKey struct{}

func userFrom(r *http.Request) *store.User { return r.Context().Value(ctxKey{}).(*store.User) }

func (s *Server) authed(h http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		c, err := r.Cookie(cookieName)
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "not logged in")
			return
		}
		u, err := s.store.SessionUser(r.Context(), auth.HashToken(c.Value))
		if errors.Is(err, store.ErrNotFound) {
			writeErr(w, http.StatusUnauthorized, "not logged in")
			return
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		h(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	var in struct{ Email, Password string }
	if !readJSON(w, r, &in) {
		return
	}
	ip := clientIP(r)
	if s.limiter.Locked("email:"+in.Email, "ip:"+ip) {
		writeErr(w, http.StatusTooManyRequests, "too many failed attempts, try again later")
		return
	}
	u, err := s.store.UserByEmail(r.Context(), in.Email)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.fail(w, err)
		return
	}
	if u == nil {
		auth.BurnPasswordCheck(in.Password)
	}
	if u == nil || !u.Active || !auth.CheckPassword(u.PassHash, in.Password) {
		s.limiter.Fail("email:"+in.Email, "ip:"+ip)
		s.log.Warn("Неуспешен вход", "имейл", in.Email, "ip", ip)
		writeErr(w, http.StatusUnauthorized, "wrong e-mail or password")
		return
	}
	s.limiter.Reset("email:" + in.Email)
	token, hash := auth.NewSessionToken()
	if err := s.store.CreateSession(r.Context(), hash, u.ID, ip, r.UserAgent(), sessionTTL); err != nil {
		s.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", MaxAge: int(sessionTTL.Seconds()),
		HttpOnly: true, Secure: s.secureCookie, SameSite: http.SameSiteStrictMode,
	})
	s.store.Audit(r.Context(), u.ID, "login", 0, ip)
	s.log.Info("Вход в админ панела", "потребител", u.Email, "ip", ip)
	writeJSON(w, http.StatusOK, u)
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.store.DeleteSession(r.Context(), auth.HashToken(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: cookieName, Value: "", Path: "/", MaxAge: -1,
		HttpOnly: true, Secure: s.secureCookie, SameSite: http.SameSiteStrictMode})
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) me(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, userFrom(r))
}

// --- helpers ---

func pathID(r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	return id, err == nil
}

func readJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, status int, msg string) {
	writeJSON(w, status, map[string]string{"error": msg})
}

// storeErr maps store errors to HTTP answers; anything unknown is a 500.
func (s *Server) storeErr(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not found")
	case errors.Is(err, store.ErrNotEmpty):
		writeErr(w, http.StatusConflict, "not empty: remove children and devices first")
	case errors.Is(err, store.ErrInUse):
		writeErr(w, http.StatusConflict, "used in a campaign")
	case errors.Is(err, store.ErrDuplicate):
		writeErr(w, http.StatusConflict, "already exists")
	case errors.Is(err, store.ErrBadParent), errors.Is(err, store.ErrInvalidArg), errors.Is(err, auth.ErrWeakPassword):
		writeErr(w, http.StatusBadRequest, err.Error())
	default:
		s.fail(w, err)
	}
}

func (s *Server) fail(w http.ResponseWriter, err error) {
	// The browser left the page or reloaded mid-request; not an error.
	if errors.Is(err, context.Canceled) {
		return
	}
	s.log.Error("Грешка в админ заявка", "грешка", err)
	writeErr(w, http.StatusInternalServerError, "internal error")
}

func forbidden(w http.ResponseWriter) { writeErr(w, http.StatusForbidden, "forbidden") }

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}
