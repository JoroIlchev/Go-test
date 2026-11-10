package admin

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"marquee/internal/auth"
	"marquee/internal/store"
)

type nodeDTO struct {
	ID       int64  `json:"id"`
	ParentID *int64 `json:"parent_id"`
	ClientID int64  `json:"client_id"`
	Kind     string `json:"kind"`
	Name     string `json:"name"`
}

type deviceDTO struct {
	ID          int64   `json:"id"`
	NodeID      *int64  `json:"node_id"`
	Name        string  `json:"name"`
	Status      string  `json:"status"`
	PairingCode string  `json:"pairing_code,omitempty"`
	AuthMode    string  `json:"auth_mode"`
	AuthUser    string  `json:"auth_user"`
	HasPassword bool    `json:"has_password"`
	URL         string  `json:"url,omitempty"`
	UserAgent   string  `json:"user_agent"`
	LastIP      string  `json:"last_ip"`
	LastSeenAt  *string `json:"last_seen_at"`
	CreatedAt   string  `json:"created_at"`
	// When the device was told to check in next, and since when it is
	// considered offline (missed that check by more than the grace period).
	NextCheckAt  *string `json:"next_check_at"`
	OfflineSince *string `json:"offline_since"`
}

func toNodeDTO(n *store.Node) nodeDTO {
	d := nodeDTO{ID: n.ID, ClientID: n.ClientID, Kind: n.Kind, Name: n.Name}
	if n.ParentID.Valid {
		d.ParentID = &n.ParentID.Int64
	}
	return d
}

// toDeviceDTO exposes the secret URL and pairing code only to super admins.
func toDeviceDTO(d *store.Device, u *store.User, publicURL string) deviceDTO {
	out := deviceDTO{
		ID: d.ID, Name: d.Name, Status: d.Status, AuthMode: d.AuthMode, AuthUser: d.AuthUser,
		HasPassword: d.AuthPassHash != "", UserAgent: d.UserAgent, LastIP: d.LastIP, CreatedAt: d.CreatedAt,
	}
	if d.NodeID.Valid {
		out.NodeID = &d.NodeID.Int64
	}
	if d.LastSeenAt.Valid {
		out.LastSeenAt = &d.LastSeenAt.String
	}
	if d.NextCheckAt.Valid {
		out.NextCheckAt = &d.NextCheckAt.String
	}
	if d.OfflineSince.Valid {
		out.OfflineSince = &d.OfflineSince.String
	}
	if u.IsSuper {
		out.PairingCode = d.PairingCode
		out.URL = publicURL + "/d/" + d.Token + "/index.smil"
	}
	return out
}

// visibleRoots are the top nodes of what u may see: all clients for a super
// admin, otherwise the role nodes that are not inside another role node.
func (s *Server) visibleRoots(r *http.Request, u *store.User) ([]*store.Node, error) {
	if u.IsSuper {
		return s.store.Clients(r.Context())
	}
	var nodes []*store.Node
	for _, role := range u.Roles {
		n, err := s.store.Node(r.Context(), role.NodeID)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		nodes = append(nodes, n)
	}
	var roots []*store.Node
	for _, n := range nodes {
		nested := false
		for _, other := range nodes {
			if other.ID != n.ID && n.IsUnder(other) {
				nested = true
				break
			}
		}
		if !nested {
			roots = append(roots, n)
		}
	}
	return roots, nil
}

func (s *Server) tree(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	roots, err := s.visibleRoots(r, u)
	if err != nil {
		s.fail(w, err)
		return
	}
	nodes, devices := []nodeDTO{}, []deviceDTO{}
	hub := s.hubURL(r.Context())
	for _, root := range roots {
		sub, err := s.store.Subtree(r.Context(), root)
		if err != nil {
			s.fail(w, err)
			return
		}
		for _, n := range sub {
			nodes = append(nodes, toNodeDTO(n))
		}
		devs, err := s.store.DevicesUnder(r.Context(), root)
		if err != nil {
			s.fail(w, err)
			return
		}
		for _, d := range devs {
			devices = append(devices, toDeviceDTO(d, u, hub))
		}
	}
	out := map[string]any{
		"nodes": nodes, "devices": devices, "server_time": time.Now().UTC().Format(time.RFC3339),
	}
	if u.IsSuper {
		out["hub_url"] = hub
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createNode(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var in struct {
		ParentID *int64 `json:"parent_id"`
		Kind     string `json:"kind"`
		Name     string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	var parent *store.Node
	if in.ParentID == nil {
		if !u.IsSuper {
			forbidden(w)
			return
		}
	} else {
		p, err := s.store.Node(r.Context(), *in.ParentID)
		if err != nil {
			s.storeErr(w, err)
			return
		}
		if !auth.Can(u, p, auth.PermAdmin) {
			forbidden(w)
			return
		}
		parent = p
	}
	n, err := s.store.CreateNode(r.Context(), parent, in.Kind, in.Name)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	s.audit(r, "node.create", n.ID, n.Kind+" "+n.Name)
	writeJSON(w, http.StatusCreated, toNodeDTO(n))
}

func (s *Server) renameNode(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in struct{ Name string }
	if !readJSON(w, r, &in) {
		return
	}
	n, err := s.store.Node(r.Context(), id)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	if !auth.Can(u, n, auth.PermAdmin) {
		forbidden(w)
		return
	}
	if err := s.store.RenameNode(r.Context(), id, in.Name); err != nil {
		s.storeErr(w, err)
		return
	}
	s.audit(r, "node.rename", id, n.Name+" -> "+in.Name)
	w.WriteHeader(http.StatusNoContent)
}

// deleteNode needs admin rights on the parent: an admin cannot delete the
// node their own role sits on, and only super admins delete clients.
func (s *Server) deleteNode(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	n, err := s.store.Node(r.Context(), id)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	allowed := u.IsSuper
	if !allowed && n.ParentID.Valid {
		if p, err := s.store.Node(r.Context(), n.ParentID.Int64); err == nil {
			allowed = auth.Can(u, p, auth.PermAdmin)
		}
	}
	if !allowed {
		forbidden(w)
		return
	}
	if err := s.store.DeleteNode(r.Context(), id); err != nil {
		s.storeErr(w, err)
		return
	}
	s.audit(r, "node.delete", id, n.Kind+" "+n.Name)
	w.WriteHeader(http.StatusNoContent)
}

// --- devices: super admin only for now ---

func (s *Server) pendingDevices(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if !u.IsSuper {
		forbidden(w)
		return
	}
	devs, err := s.store.PendingDevices(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	out := []deviceDTO{}
	hub := s.hubURL(r.Context())
	for _, d := range devs {
		out = append(out, toDeviceDTO(d, u, hub))
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) siteFor(w http.ResponseWriter, r *http.Request, id int64) *store.Node {
	site, err := s.store.Node(r.Context(), id)
	if err != nil {
		s.storeErr(w, err)
		return nil
	}
	if site.Kind != "site" {
		writeErr(w, http.StatusBadRequest, "devices can only be placed on a site")
		return nil
	}
	return site
}

func (s *Server) pairDevice(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if !u.IsSuper {
		forbidden(w)
		return
	}
	var in struct {
		Code   string `json:"code"`
		SiteID int64  `json:"site_id"`
		Name   string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	site := s.siteFor(w, r, in.SiteID)
	if site == nil {
		return
	}
	d, err := s.store.PairDevice(r.Context(), in.Code, site, in.Name)
	if errors.Is(err, store.ErrNotFound) {
		writeErr(w, http.StatusNotFound, "no pending device with this code")
		return
	}
	if err != nil {
		s.storeErr(w, err)
		return
	}
	s.audit(r, "device.pair", site.ID, d.Name+" code="+in.Code)
	writeJSON(w, http.StatusOK, toDeviceDTO(d, u, s.hubURL(r.Context())))
}

func (s *Server) createLinkDevice(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if !u.IsSuper {
		forbidden(w)
		return
	}
	var in struct {
		SiteID int64  `json:"site_id"`
		Name   string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	site := s.siteFor(w, r, in.SiteID)
	if site == nil {
		return
	}
	d, err := s.store.CreateLinkDevice(r.Context(), site, in.Name)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	s.audit(r, "device.create", site.ID, d.Name)
	writeJSON(w, http.StatusCreated, toDeviceDTO(d, u, s.hubURL(r.Context())))
}

func (s *Server) updateDevice(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if !u.IsSuper {
		forbidden(w)
		return
	}
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var in struct {
		Name         *string `json:"name"`
		SiteID       *int64  `json:"site_id"`
		Status       *string `json:"status"`
		AuthMode     *string `json:"auth_mode"`
		AuthUser     *string `json:"auth_user"`
		AuthPassword *string `json:"auth_password"`
		RotateToken  bool    `json:"rotate_token"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	upd := store.DeviceUpdate{Name: in.Name, Status: in.Status, AuthMode: in.AuthMode,
		AuthUser: in.AuthUser, RotateToken: in.RotateToken}
	if in.SiteID != nil {
		if upd.Site = s.siteFor(w, r, *in.SiteID); upd.Site == nil {
			return
		}
	}
	if in.AuthPassword != nil {
		h, err := auth.HashPassword(*in.AuthPassword)
		if err != nil {
			s.storeErr(w, err)
			return
		}
		upd.AuthPassHash = &h
	}
	d, err := s.store.UpdateDevice(r.Context(), id, upd)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	s.audit(r, "device.update", 0, "device "+strconv.FormatInt(id, 10))
	writeJSON(w, http.StatusOK, toDeviceDTO(d, u, s.hubURL(r.Context())))
}
