package admin

import (
	"context"
	"encoding/json"
	"net/http"

	"marquee/internal/auth"
	"marquee/internal/media"
	"marquee/internal/ops"
	"marquee/internal/store"
)

// How new uploads are prepared is chosen per client or site and inherited by
// everything below, like the check schedule; the hub settings are the default.

type prepDTO struct {
	Effective media.Prep `json:"effective"`
	Own       bool       `json:"own"`          // set on this node
	FromID    int64      `json:"from_node_id"` // 0 = the hub settings
	FromName  string     `json:"from_name,omitempty"`
	CanEdit   bool       `json:"can_edit"`
}

// nodePrep returns the processing choice that applies at n (raw JSON) and
// the node it comes from; "" and 0 mean the hub settings.
func (s *Server) nodePrep(ctx context.Context, n *store.Node) (string, int64, error) {
	chain := n.AncestorIDs()
	set, err := s.store.NodePreps(ctx, chain)
	if err != nil {
		return "", 0, err
	}
	for i := len(chain) - 1; i >= 0; i-- {
		if raw, ok := set[chain[i]]; ok {
			return raw, chain[i], nil
		}
	}
	return "", 0, nil
}

func (s *Server) getNodePrep(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	n := s.pathNode(w, r)
	if n == nil {
		return
	}
	if !auth.Can(u, n, auth.PermView) {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	raw, from, err := s.nodePrep(r.Context(), n)
	if err != nil {
		s.fail(w, err)
		return
	}
	set, _ := ops.Load(r.Context(), s.store)
	out := prepDTO{Effective: set.Prep(), Own: from == n.ID, FromID: from, CanEdit: auth.Can(u, n, auth.PermAdmin)}
	if from != 0 {
		var p media.Prep
		if json.Unmarshal([]byte(raw), &p) == nil {
			out.Effective = p
		}
		if fn, err := s.store.Node(r.Context(), from); err == nil {
			out.FromName = fn.Name
		}
	}
	writeJSON(w, http.StatusOK, out)
}

// setNodePrep sets the choice for n and everything below; null inherits again.
func (s *Server) setNodePrep(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	n := s.pathNode(w, r)
	if n == nil {
		return
	}
	if !auth.Can(u, n, auth.PermAdmin) {
		forbidden(w)
		return
	}
	var in struct {
		Prep *media.Prep `json:"prep"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	raw := ""
	if in.Prep != nil {
		if _, err := in.Prep.Options(); err != nil {
			writeErr(w, http.StatusBadRequest, err.Error())
			return
		}
		b, _ := json.Marshal(in.Prep)
		raw = string(b)
	}
	if err := s.store.SetNodePrep(r.Context(), n.ID, raw, u.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "node.prep", n.ID, raw)
	s.getNodePrep(w, r)
}
