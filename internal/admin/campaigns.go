package admin

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"marquee/internal/auth"
	"marquee/internal/campaign"
	"marquee/internal/store"
)

type versionDTO struct {
	*store.Version
	Spec          campaign.Spec `json:"spec"`
	CreatedByName string        `json:"created_by_name"`
	ReviewedBy    string        `json:"reviewed_by_name"`
}

type campaignSummary struct {
	ID          int64       `json:"id"`
	Name        string      `json:"name"`
	NodeID      int64       `json:"node_id"`
	OwnerName   string      `json:"owner_name"`
	State       string      `json:"state"` // draft | pending | active | scheduled | expired | stopped
	Live        *versionDTO `json:"live"`
	HasPending  bool        `json:"has_pending"`
	DraftDirty  bool        `json:"draft_dirty"`
	CanEdit     bool        `json:"can_edit"`
	CanPublish  bool        `json:"can_publish"`
	CanAdmin    bool        `json:"can_admin"`
	NeedsReview bool        `json:"needs_review"` // u's publications here wait for approval
	UpdatedAt   string      `json:"updated_at"`
}

type campaignDetail struct {
	campaignSummary
	Draft    campaign.Spec `json:"draft"`
	Versions []versionDTO  `json:"versions"`
	Ready    int           `json:"ready"`   // targeted devices that already have the live version
	Devices  int           `json:"devices"` // targeted active devices
	// Before a scheduled start: devices without the live version whose next
	// check comes after the start, and the latest such check.
	Late        int    `json:"late"`
	LatestCheck string `json:"latest_check,omitempty"`
}

func (s *Server) userName(r *http.Request, id int64, cache map[int64]string) string {
	if id == 0 {
		return ""
	}
	if n, ok := cache[id]; ok {
		return n
	}
	name := ""
	if u, err := s.store.User(r.Context(), id); err == nil {
		name = u.Name
		if name == "" {
			name = u.Email
		}
	}
	cache[id] = name
	return name
}

func (s *Server) toVersionDTO(r *http.Request, v *store.Version, names map[int64]string) versionDTO {
	spec, _ := campaign.ParseSpec(v.Spec)
	return versionDTO{Version: v, Spec: spec,
		CreatedByName: s.userName(r, v.CreatedBy.Int64, names),
		ReviewedBy:    s.userName(r, v.ReviewedBy.Int64, names)}
}

func (s *Server) summarize(r *http.Request, u *store.User, c *store.Campaign, owner *store.Node, names map[int64]string) (campaignSummary, []*store.Version, error) {
	sum := campaignSummary{
		ID: c.ID, Name: c.Name, NodeID: c.NodeID, OwnerName: owner.Name,
		CanEdit:     auth.Can(u, owner, auth.PermEdit),
		CanPublish:  auth.Can(u, owner, auth.PermPublish),
		CanAdmin:    auth.Can(u, owner, auth.PermAdmin),
		NeedsReview: auth.NeedsApproval(u, owner),
		UpdatedAt:   c.DraftUpdatedAt.String,
	}
	versions, err := s.store.Versions(r.Context(), c.ID)
	if err != nil {
		return sum, nil, err
	}
	for _, v := range versions {
		if v.Status == "pending" {
			sum.HasPending = true
		}
		if c.LiveVersionID.Valid && v.ID == c.LiveVersionID.Int64 {
			dto := s.toVersionDTO(r, v, names)
			sum.Live = &dto
		}
	}
	draft, _ := campaign.ParseSpec(c.Draft)
	switch {
	case sum.Live != nil:
		sum.DraftDirty = draft.JSON() != sum.Live.Spec.JSON()
		sum.State = s.liveState(sum.Live.Spec)
	case len(versions) == 0:
		sum.State = "draft"
		sum.DraftDirty = true
	default:
		sum.State = "stopped"
		sum.DraftDirty = draft.JSON() != mustSpec(versions[0].Spec).JSON()
	}
	if sum.HasPending && sum.Live == nil {
		sum.State = "pending"
	}
	return sum, versions, nil
}

func mustSpec(raw string) campaign.Spec {
	s, _ := campaign.ParseSpec(raw)
	return s
}

func (s *Server) liveState(spec campaign.Spec) string {
	rule, err := spec.Rule(s.loc)
	if err != nil {
		return "active"
	}
	now := time.Now()
	switch {
	case !rule.End.IsZero() && !rule.End.After(now):
		return "expired"
	case !rule.Start.IsZero() && rule.Start.After(now):
		return "scheduled"
	}
	return "active"
}

func (s *Server) listCampaigns(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	n := s.nodeParam(w, r)
	if n == nil {
		return
	}
	if !auth.Can(u, n, auth.PermView) {
		forbidden(w)
		return
	}
	items, err := s.store.CampaignsAround(r.Context(), n)
	if err != nil {
		s.fail(w, err)
		return
	}
	nodeOf := s.nodeLookup(r)
	names := map[int64]string{}
	out := []campaignSummary{}
	for _, c := range items {
		owner := nodeOf(c.NodeID)
		if owner == nil || !s.canSeeOwned(u, owner, nodeOf) {
			continue
		}
		sum, _, err := s.summarize(r, u, c, owner, names)
		if err != nil {
			s.fail(w, err)
			return
		}
		out = append(out, sum)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createCampaign(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var in struct {
		NodeID int64  `json:"node_id"`
		Name   string `json:"name"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	owner, err := s.store.Node(r.Context(), in.NodeID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	if !auth.Can(u, owner, auth.PermCreate) {
		forbidden(w)
		return
	}
	// A new campaign plays on its own node by default.
	draft := campaign.Spec{Fit: "meet", Items: []campaign.Item{}, Targets: []int64{owner.ID}}
	c, err := s.store.CreateCampaign(r.Context(), owner, in.Name, draft.JSON(), u.ID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	s.audit(r, "campaign.create", owner.ID, c.Name)
	writeJSON(w, http.StatusCreated, map[string]int64{"id": c.ID})
}

// campaignFor loads the campaign in the path and checks u may see it.
func (s *Server) campaignFor(w http.ResponseWriter, r *http.Request) (*store.Campaign, *store.Node) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return nil, nil
	}
	c, err := s.store.Campaign(r.Context(), id)
	if err != nil {
		s.storeErr(w, err)
		return nil, nil
	}
	owner, err := s.store.Node(r.Context(), c.NodeID)
	if err != nil {
		s.fail(w, err)
		return nil, nil
	}
	if !s.canSeeOwned(userFrom(r), owner, s.nodeLookup(r)) {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil
	}
	return c, owner
}

func (s *Server) getCampaign(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	c, owner := s.campaignFor(w, r)
	if c == nil {
		return
	}
	s.writeCampaign(w, r, u, c, owner)
}

func (s *Server) writeCampaign(w http.ResponseWriter, r *http.Request, u *store.User, c *store.Campaign, owner *store.Node) {
	names := map[int64]string{}
	sum, versions, err := s.summarize(r, u, c, owner, names)
	if err != nil {
		s.fail(w, err)
		return
	}
	d := campaignDetail{campaignSummary: sum, Versions: []versionDTO{}}
	d.Draft, _ = campaign.ParseSpec(c.Draft)
	for _, v := range versions {
		d.Versions = append(d.Versions, s.toVersionDTO(r, v, names))
	}
	if sum.Live != nil {
		ids, err := s.targetDevices(r, sum.Live.Spec.Targets)
		if err != nil {
			s.fail(w, err)
			return
		}
		d.Devices = len(ids)
		if d.Ready, err = s.store.DevicesServed(r.Context(), ids, sum.Live.ID); err != nil {
			s.fail(w, err)
			return
		}
		// A screen that only checks in after the start shows the old content
		// until then: warn while there is still time to do something about it.
		if rule, err := sum.Live.Spec.Rule(s.loc); err == nil && rule.Start.After(time.Now()) {
			n, latest, err := s.store.DevicesLate(r.Context(), ids, sum.Live.ID, rule.Start)
			if err != nil {
				s.fail(w, err)
				return
			}
			d.Late, d.LatestCheck = n, latest
		}
	}
	writeJSON(w, http.StatusOK, d)
}

// targetDevices lists active devices under any of the target nodes.
func (s *Server) targetDevices(r *http.Request, targets []int64) ([]int64, error) {
	seen := map[int64]bool{}
	var ids []int64
	for _, t := range targets {
		n, err := s.store.Node(r.Context(), t)
		if errors.Is(err, store.ErrNotFound) {
			continue
		}
		if err != nil {
			return nil, err
		}
		devs, err := s.store.DevicesUnder(r.Context(), n)
		if err != nil {
			return nil, err
		}
		for _, d := range devs {
			if d.Status == "active" && !seen[d.ID] {
				seen[d.ID] = true
				ids = append(ids, d.ID)
			}
		}
	}
	return ids, nil
}

// checkSpec validates targets and media against the campaign's owner node:
// targets must be at or below it, media must be visible from it.
func (s *Server) checkSpec(r *http.Request, spec campaign.Spec, owner *store.Node, publish bool) string {
	var err error
	if publish {
		err = spec.ValidatePublish(s.loc)
	} else {
		err = spec.ValidateDraft(s.loc)
	}
	if err != nil {
		return err.Error()
	}
	nodeOf := s.nodeLookup(r)
	for _, t := range spec.Targets {
		n := nodeOf(t)
		if n == nil || !n.IsUnder(owner) {
			return "targets must be within the campaign's node"
		}
	}
	media, err := s.store.MediaByIDs(r.Context(), spec.MediaIDs())
	if err != nil {
		return "internal error"
	}
	check := func(id int64, kinds ...string) string {
		m := media[id]
		if m == nil {
			return "unknown media " + strconv.FormatInt(id, 10)
		}
		mo := nodeOf(m.NodeID)
		if mo == nil || !owner.IsUnder(mo) {
			return "media \"" + m.Name + "\" is not available here"
		}
		for _, k := range kinds {
			if m.Kind == k {
				return ""
			}
		}
		return "media \"" + m.Name + "\" has the wrong type here"
	}
	for _, it := range spec.Items {
		if msg := check(it.MediaID, "image", "video"); msg != "" {
			return msg
		}
	}
	if spec.AudioID != 0 {
		if msg := check(spec.AudioID, "audio"); msg != "" {
			return msg
		}
	}
	return ""
}

func (s *Server) saveDraft(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	c, owner := s.campaignFor(w, r)
	if c == nil {
		return
	}
	if !auth.Can(u, owner, auth.PermEdit) {
		forbidden(w)
		return
	}
	var in struct {
		Name string        `json:"name"`
		Spec campaign.Spec `json:"spec"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if msg := s.checkSpec(r, in.Spec, owner, false); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	if in.Spec.Fit == "" {
		in.Spec.Fit = "meet"
	}
	if err := s.store.SaveDraft(r.Context(), c.ID, in.Name, in.Spec.JSON(), in.Spec.MediaIDs(), u.ID); err != nil {
		s.storeErr(w, err)
		return
	}
	c, _ = s.store.Campaign(r.Context(), c.ID)
	s.writeCampaign(w, r, u, c, owner)
}

// publish freezes the saved draft into a new version. It goes live at once
// unless the user's role requires approval.
func (s *Server) publishCampaign(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	c, owner := s.campaignFor(w, r)
	if c == nil {
		return
	}
	if !auth.Can(u, owner, auth.PermPublish) {
		forbidden(w)
		return
	}
	spec, err := campaign.ParseSpec(c.Draft)
	if err != nil {
		s.fail(w, err)
		return
	}
	if msg := s.checkSpec(r, spec, owner, true); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	media, err := s.store.MediaByIDs(r.Context(), spec.MediaIDs())
	if err != nil {
		s.fail(w, err)
		return
	}
	for _, m := range media {
		if m.Status != "ready" {
			writeErr(w, http.StatusBadRequest, "media \""+m.Name+"\" is not ready yet")
			return
		}
	}
	status := "approved"
	if auth.NeedsApproval(u, owner) {
		status = "pending"
	}
	v, err := s.store.CreateVersion(r.Context(), c.ID, spec.JSON(), spec.MediaIDs(), status, u.ID)
	if err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "campaign.publish", owner.ID, c.Name+" v"+strconv.Itoa(v.Number)+" "+status)
	c, _ = s.store.Campaign(r.Context(), c.ID)
	s.writeCampaign(w, r, u, c, owner)
}

func (s *Server) reviewVersion(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	c, owner := s.campaignFor(w, r)
	if c == nil {
		return
	}
	if !auth.Can(u, owner, auth.PermAdmin) {
		forbidden(w)
		return
	}
	vid, err := strconv.ParseInt(r.PathValue("vid"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad version id")
		return
	}
	var in struct {
		Approve bool   `json:"approve"`
		Comment string `json:"comment"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	v, err := s.store.Version(r.Context(), vid)
	if err != nil || v.CampaignID != c.ID {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	if err := s.store.ReviewVersion(r.Context(), v, in.Approve, in.Comment, u.ID); err != nil {
		if errors.Is(err, store.ErrInvalidArg) {
			writeErr(w, http.StatusConflict, "version is not waiting for approval")
			return
		}
		s.fail(w, err)
		return
	}
	action := "campaign.reject"
	if in.Approve {
		action = "campaign.approve"
	}
	s.audit(r, action, owner.ID, c.Name+" v"+strconv.Itoa(v.Number))
	c, _ = s.store.Campaign(r.Context(), c.ID)
	s.writeCampaign(w, r, u, c, owner)
}

// setLive rolls back to an approved version, or stops the campaign (version_id 0).
func (s *Server) setLive(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	c, owner := s.campaignFor(w, r)
	if c == nil {
		return
	}
	if !auth.Can(u, owner, auth.PermPublish) {
		forbidden(w)
		return
	}
	var in struct {
		VersionID int64 `json:"version_id"`
	}
	if !readJSON(w, r, &in) {
		return
	}
	if in.VersionID != 0 {
		v, err := s.store.Version(r.Context(), in.VersionID)
		if err != nil || v.CampaignID != c.ID || v.Status != "approved" {
			writeErr(w, http.StatusBadRequest, "only an approved version of this campaign can go live")
			return
		}
	}
	if err := s.store.SetLive(r.Context(), c.ID, in.VersionID); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "campaign.live", owner.ID, c.Name+" version="+strconv.FormatInt(in.VersionID, 10))
	c, _ = s.store.Campaign(r.Context(), c.ID)
	s.writeCampaign(w, r, u, c, owner)
}

func (s *Server) deleteCampaign(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	c, owner := s.campaignFor(w, r)
	if c == nil {
		return
	}
	if !auth.Can(u, owner, auth.PermEdit) {
		forbidden(w)
		return
	}
	if c.LiveVersionID.Valid {
		writeErr(w, http.StatusConflict, "stop the campaign before deleting it")
		return
	}
	if err := s.store.DeleteCampaign(r.Context(), c.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "campaign.delete", owner.ID, c.Name)
	w.WriteHeader(http.StatusNoContent)
}

type approvalDTO struct {
	CampaignID    int64  `json:"campaign_id"`
	CampaignName  string `json:"campaign_name"`
	NodeID        int64  `json:"node_id"`
	OwnerName     string `json:"owner_name"`
	VersionID     int64  `json:"version_id"`
	Number        int    `json:"number"`
	CreatedByName string `json:"created_by_name"`
	CreatedAt     string `json:"created_at"`
}

// approvals lists versions waiting for u's decision.
func (s *Server) approvals(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var roots []*store.Node
	if u.IsSuper {
		roots = []*store.Node{nil}
	} else {
		for _, role := range u.Roles {
			if role.Role != "admin" {
				continue
			}
			if n, err := s.store.Node(r.Context(), role.NodeID); err == nil {
				roots = append(roots, n)
			}
		}
	}
	nodeOf := s.nodeLookup(r)
	names := map[int64]string{}
	seen := map[int64]bool{}
	out := []approvalDTO{}
	for _, root := range roots {
		versions, err := s.store.PendingVersions(r.Context(), root)
		if err != nil {
			s.fail(w, err)
			return
		}
		for _, v := range versions {
			if seen[v.ID] {
				continue
			}
			seen[v.ID] = true
			c, err := s.store.Campaign(r.Context(), v.CampaignID)
			if err != nil {
				continue
			}
			owner := nodeOf(c.NodeID)
			if owner == nil {
				continue
			}
			out = append(out, approvalDTO{
				CampaignID: c.ID, CampaignName: c.Name, NodeID: c.NodeID, OwnerName: owner.Name,
				VersionID: v.ID, Number: v.Number, CreatedByName: s.userName(r, v.CreatedBy.Int64, names),
				CreatedAt: v.CreatedAt,
			})
		}
	}
	writeJSON(w, http.StatusOK, out)
}
