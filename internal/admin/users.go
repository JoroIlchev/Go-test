package admin

import (
	"net/http"
	"strconv"

	"marquee/internal/auth"
	"marquee/internal/store"
)

// nodeLookup caches nodes for one request's permission checks.
func (s *Server) nodeLookup(r *http.Request) func(int64) *store.Node {
	cache := map[int64]*store.Node{}
	return func(id int64) *store.Node {
		if n, ok := cache[id]; ok {
			return n
		}
		n, err := s.store.Node(r.Context(), id)
		if err != nil {
			n = nil
		}
		cache[id] = n
		return n
	}
}

// checkRoles verifies that every granted role is on a node of clientID where
// actor is admin. Editors get no admin-only flags implicitly; admins get all.
func checkRoles(actor *store.User, roles []store.Role, clientID int64, nodeOf func(int64) *store.Node) bool {
	for _, r := range roles {
		n := nodeOf(r.NodeID)
		if n == nil || n.ClientID != clientID || !auth.Can(actor, n, auth.PermAdmin) {
			return false
		}
	}
	return true
}

// listUsers: a super admin passes ?client_id=; a client admin gets the users
// of their own client that they are allowed to manage.
func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var clientID int64
	if u.IsSuper {
		id, err := strconv.ParseInt(r.URL.Query().Get("client_id"), 10, 64)
		if err != nil {
			writeErr(w, http.StatusBadRequest, "client_id required")
			return
		}
		clientID = id
	} else {
		clientID = u.ClientID.Int64
	}
	users, err := s.store.UsersOfClient(r.Context(), clientID)
	if err != nil {
		s.fail(w, err)
		return
	}
	nodeOf := s.nodeLookup(r)
	out := []*store.User{}
	for _, t := range users {
		if t.ID == u.ID || auth.CanManageUser(u, t, nodeOf) {
			out = append(out, t)
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var in struct {
		Email    string       `json:"email"`
		Name     string       `json:"name"`
		Password string       `json:"password"`
		ClientID *int64       `json:"client_id"`
		Roles    []store.Role `json:"roles"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	var clientID int64
	switch {
	case u.IsSuper && in.ClientID != nil:
		clientID = *in.ClientID
	case !u.IsSuper && u.ClientID.Valid:
		clientID = u.ClientID.Int64
	default:
		writeErr(w, http.StatusBadRequest, "client_id required")
		return
	}
	if client, err := s.store.Node(r.Context(), clientID); err != nil || client.Kind != "client" {
		writeErr(w, http.StatusBadRequest, "unknown client")
		return
	}
	if len(in.Roles) == 0 {
		writeErr(w, http.StatusBadRequest, "at least one role is required")
		return
	}
	if !checkRoles(u, in.Roles, clientID, s.nodeLookup(r)) {
		forbidden(w)
		return
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	created, err := s.store.CreateUser(r.Context(), in.Email, in.Name, hash, false, &clientID, &u.ID, in.Roles)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	s.audit(r, "user.create", clientID, created.Email)
	writeJSON(w, http.StatusCreated, created)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in struct {
		Name     *string      `json:"name"`
		Password *string      `json:"password"`
		Active   *bool        `json:"active"`
		Roles    []store.Role `json:"roles"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	target, err := s.store.User(r.Context(), id)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	nodeOf := s.nodeLookup(r)
	self := target.ID == u.ID
	// Everyone may change their own name and password; anything else needs
	// management rights over the target, and roles can never be self-edited
	// by a client user.
	if !self && !auth.CanManageUser(u, target, nodeOf) {
		forbidden(w)
		return
	}
	if self && !u.IsSuper && (in.Roles != nil || in.Active != nil) {
		forbidden(w)
		return
	}
	if in.Roles != nil {
		if target.IsSuper || len(in.Roles) == 0 ||
			!checkRoles(u, in.Roles, target.ClientID.Int64, nodeOf) {
			forbidden(w)
			return
		}
	}
	upd := store.UserUpdate{Name: in.Name, Active: in.Active, Roles: in.Roles}
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
	s.audit(r, "user.update", 0, target.Email)
	updated, err := s.store.User(r.Context(), id)
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, updated)
}
