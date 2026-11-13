package admin

import "net/http"

// houseNode returns the operator's own ads node ("Моята реклама"), creating
// it the first time a super admin opens it.
func (s *Server) houseNode(w http.ResponseWriter, r *http.Request) {
	n, err := s.store.HouseNode(r.Context())
	if err != nil {
		s.fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]int64{"id": n.ID})
}
