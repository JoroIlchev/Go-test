package admin

import (
	"errors"
	"net/http"
	"time"

	"marquee/internal/auth"
	"marquee/internal/store"
)

const (
	trustedCookie = "marquee_device"
	// A remembered browser that is not used for this long is forgotten.
	trustedTTL = 90 * 24 * time.Hour
)

// trustedDevice is the browser's remembered-device record when it belongs to
// u, else nil. The cookie only names the browser; the password is still asked.
func (s *Server) trustedDevice(r *http.Request, u *store.User) (*store.TrustedDevice, error) {
	c, err := r.Cookie(trustedCookie)
	if err != nil || u == nil {
		return nil, nil
	}
	d, err := s.store.TrustedDevice(r.Context(), auth.HashToken(c.Value))
	if errors.Is(err, store.ErrNotFound) || (err == nil && d.UserID != u.ID) {
		return nil, nil
	}
	return d, err
}

// rememberDevice runs after a good password: a remembered browser is kept for
// another trustedTTL, a new one is remembered when the user asked for it.
func (s *Server) rememberDevice(w http.ResponseWriter, r *http.Request, u *store.User, dev *store.TrustedDevice, remember bool) error {
	ip := clientIP(r)
	if dev != nil {
		c, _ := r.Cookie(trustedCookie)
		s.setTrustedCookie(w, c.Value, int(trustedTTL.Seconds()))
		return s.store.TouchTrustedDevice(r.Context(), dev.ID, ip, r.UserAgent(), trustedTTL)
	}
	if !remember {
		return nil
	}
	token, hash := auth.NewSessionToken()
	if err := s.store.CreateTrustedDevice(r.Context(), hash, u.ID, ip, r.UserAgent(), trustedTTL); err != nil {
		return err
	}
	s.setTrustedCookie(w, token, int(trustedTTL.Seconds()))
	s.store.Audit(r.Context(), u.ID, "trusted.add", 0, ip)
	s.log.Info("Запомнено ново устройство за вход", "потребител", u.Email, "ip", ip)
	return nil
}

func (s *Server) setTrustedCookie(w http.ResponseWriter, value string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: trustedCookie, Value: value, Path: "/api/", MaxAge: maxAge,
		HttpOnly: true, Secure: s.secureCookie, SameSite: http.SameSiteStrictMode,
	})
}

type trustedOut struct {
	store.TrustedDevice
	Current bool `json:"current"` // the browser asking
}

func (s *Server) listTrusted(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	list, err := s.store.TrustedDevices(r.Context(), u.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	cur, err := s.trustedDevice(r, u)
	if err != nil {
		s.fail(w, err)
		return
	}
	out := make([]trustedOut, len(list))
	for i, d := range list {
		out[i] = trustedOut{d, cur != nil && cur.ID == d.ID}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) forgetTrusted(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	u := userFrom(r)
	if err := s.store.DeleteTrustedDevice(r.Context(), u.ID, id); err != nil {
		s.storeErr(w, err)
		return
	}
	s.store.Audit(r.Context(), u.ID, "trusted.remove", 0, clientIP(r))
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) forgetAllTrusted(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if err := s.store.DeleteTrustedDevices(r.Context(), u.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.setTrustedCookie(w, "", -1)
	s.store.Audit(r.Context(), u.ID, "trusted.remove_all", 0, clientIP(r))
	w.WriteHeader(http.StatusNoContent)
}
