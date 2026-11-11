package admin

import (
	"net/http"

	"marquee/internal/auth"
	"marquee/internal/store"
)

// The team page: the super admin manages the hub operator's people, super
// admins and managers. There is always at least one active super admin.

type staffAccessIn struct {
	Kind       string  `json:"kind"` // super | manager
	AllClients bool    `json:"all_clients"`
	Clients    []int64 `json:"clients"`
	CanBilling bool    `json:"can_billing"`
	CanLogs    bool    `json:"can_logs"`
}

// access checks the input; ok is false after an error was written.
func (s *Server) access(w http.ResponseWriter, r *http.Request, in staffAccessIn) (store.StaffAccess, bool) {
	a := store.StaffAccess{Super: in.Kind == "super", AllClients: in.AllClients, Clients: in.Clients,
		CanBilling: in.CanBilling, CanLogs: in.CanLogs}
	if in.Kind != "super" && in.Kind != "manager" {
		writeErr(w, http.StatusBadRequest, "kind must be super or manager")
		return a, false
	}
	for _, id := range in.Clients {
		if n, err := s.store.Node(r.Context(), id); err != nil || n.Kind != "client" {
			writeErr(w, http.StatusBadRequest, "unknown client")
			return a, false
		}
	}
	return a, true
}

func (s *Server) listStaff(w http.ResponseWriter, r *http.Request) {
	staff, err := s.store.Staff(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, staff)
}

func (s *Server) createStaff(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
		staffAccessIn
	}
	if !readJSON(w, r, &in) {
		return
	}
	a, ok := s.access(w, r, in.staffAccessIn)
	if !ok {
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	u, err := s.store.CreateStaff(r.Context(), in.Email, in.Name, hash, a, userFrom(r).ID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	s.audit(r, "staff.create", 0, u.Email+" ("+in.Kind+")")
	writeJSON(w, http.StatusCreated, u)
}

func (s *Server) updateStaff(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in struct {
		Name     *string        `json:"name"`
		Password *string        `json:"password"`
		Active   *bool          `json:"active"`
		Access   *staffAccessIn `json:"access"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	target, err := s.store.User(r.Context(), id)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	if !auth.Staff(target) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	var a store.StaffAccess
	if in.Access != nil {
		if a, ok = s.access(w, r, *in.Access); !ok {
			return
		}
	}
	// Taking away the last active super admin would lock everyone out of
	// the settings and the team page.
	losesSuper := target.IsSuper && target.Active &&
		((in.Active != nil && !*in.Active) || (in.Access != nil && !a.Super))
	if losesSuper {
		n, err := s.store.ActiveSupers(r.Context())
		if err != nil {
			s.fail(w, err)
			return
		}
		if n <= 1 {
			writeErr(w, http.StatusConflict, "last super admin")
			return
		}
	}
	upd := store.UserUpdate{Name: in.Name, Active: in.Active}
	if in.Password != nil {
		h, err := auth.HashPassword(*in.Password)
		if err != nil {
			s.storeErr(w, err)
			return
		}
		upd.PassHash = &h
	}
	if err := s.store.UpdateUser(r.Context(), id, upd); err != nil {
		s.storeErr(w, err)
		return
	}
	if in.Access != nil {
		if err := s.store.SetStaffAccess(r.Context(), id, a); err != nil {
			s.fail(w, err)
			return
		}
	}
	s.audit(r, "staff.update", 0, target.Email)
	updated, err := s.store.User(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
