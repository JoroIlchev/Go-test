package admin

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"

	"marquee/internal/auth"
	"marquee/internal/media"
	"marquee/internal/store"
)

type mediaDTO struct {
	*store.Media
	Warnings    []media.Warning `json:"warnings"`
	OwnerName   string          `json:"owner_name"`
	SourceID    *int64          `json:"source_id"`
	CanEdit     bool            `json:"can_edit"`
	Convertible bool            `json:"convertible"`
}

func (s *Server) toMediaDTO(u *store.User, m *store.Media, nodeOf func(int64) *store.Node) mediaDTO {
	d := mediaDTO{Media: m, Warnings: []media.Warning{}}
	json.Unmarshal([]byte(m.Warnings), &d.Warnings)
	if owner := nodeOf(m.NodeID); owner != nil {
		d.OwnerName = owner.Name
		d.CanEdit = auth.Can(u, owner, auth.PermEdit)
		d.Convertible = m.Status == "ready" && media.Convertible(m.Kind, d.Warnings) &&
			auth.Can(u, owner, auth.PermCreate) && s.worker.Tools().FFmpeg != ""
	}
	if m.SourceID.Valid {
		d.SourceID = &m.SourceID.Int64
	}
	return d
}

// canSeeOwned reports whether u may use content owned by owner: either u has
// rights on owner itself, or u works somewhere below it and inherits it.
func (s *Server) canSeeOwned(u *store.User, owner *store.Node, nodeOf func(int64) *store.Node) bool {
	if auth.Can(u, owner, auth.PermView) {
		return true
	}
	if !u.ClientID.Valid || u.ClientID.Int64 != owner.ClientID {
		return false
	}
	for _, r := range u.Roles {
		if n := nodeOf(r.NodeID); n != nil && n.IsUnder(owner) {
			return true
		}
	}
	return false
}

func (s *Server) nodeParam(w http.ResponseWriter, r *http.Request) *store.Node {
	id, err := strconv.ParseInt(r.URL.Query().Get("node_id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "node_id required")
		return nil
	}
	n, err := s.store.Node(r.Context(), id)
	if err != nil {
		s.storeErr(w, err)
		return nil
	}
	return n
}

func (s *Server) listMedia(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	n := s.nodeParam(w, r)
	if n == nil {
		return
	}
	if !auth.Can(u, n, auth.PermView) {
		forbidden(w)
		return
	}
	items, err := s.store.MediaAround(r.Context(), n)
	if err != nil {
		s.fail(w, err)
		return
	}
	nodeOf := s.nodeLookup(r)
	out := []mediaDTO{}
	for _, m := range items {
		if owner := nodeOf(m.NodeID); owner != nil && s.canSeeOwned(u, owner, nodeOf) {
			out = append(out, s.toMediaDTO(u, m, nodeOf))
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"items": out, "ffmpeg": s.worker.Tools().FFmpeg != ""})
}

type uploadResult struct {
	Name  string    `json:"name"`
	Media *mediaDTO `json:"media,omitempty"`
	Error string    `json:"error,omitempty"`
}

// uploadMedia streams multipart files straight to storage; nothing is held in memory.
func (s *Server) uploadMedia(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	n := s.nodeParam(w, r)
	if n == nil {
		return
	}
	if !auth.Can(u, n, auth.PermCreate) {
		forbidden(w)
		return
	}
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "multipart body expected")
		return
	}
	nodeOf := s.nodeLookup(r)
	results := []uploadResult{}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, "upload interrupted")
			return
		}
		filename := part.FileName()
		if filename == "" {
			part.Close()
			continue
		}
		res := uploadResult{Name: filename}
		kind, ext, ok := media.KindOf(filename)
		if !ok {
			res.Error = "unsupported file type"
			results = append(results, res)
			part.Close()
			continue
		}
		sha, size, err := s.storage.Save(part, ext, s.maxUpload)
		part.Close()
		if errors.Is(err, media.ErrTooLarge) {
			res.Error = "file too large"
			results = append(results, res)
			continue
		}
		if err != nil {
			s.fail(w, err)
			return
		}
		name := strings.TrimSuffix(filename, filename[strings.LastIndex(filename, "."):])
		m, err := s.store.CreateMedia(r.Context(), store.NewMedia{
			Node: n, Name: name, Kind: kind, SHA256: sha, Ext: ext, Size: size, UploadedBy: u.ID,
		})
		if err != nil {
			s.fail(w, err)
			return
		}
		s.worker.Enqueue(m.ID)
		s.audit(r, "media.upload", n.ID, filename)
		dto := s.toMediaDTO(u, m, nodeOf)
		res.Media = &dto
		results = append(results, res)
	}
	writeJSON(w, http.StatusOK, results)
}

// mediaFor loads the media item in the path and checks u may see it.
func (s *Server) mediaFor(w http.ResponseWriter, r *http.Request) (*store.Media, *store.Node) {
	id, ok := pathID(r)
	if !ok {
		writeErr(w, http.StatusBadRequest, "bad id")
		return nil, nil
	}
	m, err := s.store.Media(r.Context(), id)
	if err != nil {
		s.storeErr(w, err)
		return nil, nil
	}
	owner, err := s.store.Node(r.Context(), m.NodeID)
	if err != nil {
		s.fail(w, err)
		return nil, nil
	}
	if !s.canSeeOwned(userFrom(r), owner, s.nodeLookup(r)) {
		writeErr(w, http.StatusNotFound, "not found")
		return nil, nil
	}
	return m, owner
}

func (s *Server) mediaThumb(w http.ResponseWriter, r *http.Request) {
	m, _ := s.mediaFor(w, r)
	if m == nil {
		return
	}
	if !m.HasThumb {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("ETag", `"`+m.SHA256+`"`)
	http.ServeFile(w, r, s.storage.ThumbPath(m.SHA256))
}

func (s *Server) mediaFile(w http.ResponseWriter, r *http.Request) {
	m, _ := s.mediaFor(w, r)
	if m == nil {
		return
	}
	if m.SHA256 == "" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", media.MimeOf(m.Ext))
	w.Header().Set("Cache-Control", "private, max-age=86400")
	w.Header().Set("ETag", `"`+m.SHA256+`"`)
	if r.URL.Query().Has("download") {
		w.Header().Set("Content-Disposition", `attachment; filename="`+strings.ReplaceAll(m.Name, `"`, "")+"."+m.Ext+`"`)
	}
	http.ServeFile(w, r, s.storage.Path(m.SHA256, m.Ext))
}

func (s *Server) renameMedia(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	m, owner := s.mediaFor(w, r)
	if m == nil {
		return
	}
	if !auth.Can(u, owner, auth.PermEdit) {
		forbidden(w)
		return
	}
	var in struct{ Name string }
	if !readJSON(w, r, &in) {
		return
	}
	if err := s.store.RenameMedia(r.Context(), m.ID, in.Name); err != nil {
		s.storeErr(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) deleteMedia(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	m, owner := s.mediaFor(w, r)
	if m == nil {
		return
	}
	if !auth.Can(u, owner, auth.PermEdit) {
		forbidden(w)
		return
	}
	if inUse, err := s.store.MediaInUse(r.Context(), m.ID); err != nil {
		s.fail(w, err)
		return
	} else if inUse {
		s.storeErr(w, store.ErrInUse)
		return
	}
	orphan, err := s.store.DeleteMedia(r.Context(), m)
	if err != nil {
		s.fail(w, err)
		return
	}
	if orphan {
		s.storage.Remove(m.SHA256, m.Ext)
	}
	s.audit(r, "media.delete", owner.ID, m.Name)
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) convertMedia(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	m, owner := s.mediaFor(w, r)
	if m == nil {
		return
	}
	if !auth.Can(u, owner, auth.PermCreate) {
		forbidden(w)
		return
	}
	if m.Kind != "video" || m.Status != "ready" {
		writeErr(w, http.StatusBadRequest, "only ready videos can be converted")
		return
	}
	if s.worker.Tools().FFmpeg == "" {
		writeErr(w, http.StatusBadRequest, "ffmpeg is not available on the server")
		return
	}
	c, err := s.store.CreateMedia(r.Context(), store.NewMedia{
		Node: owner, Name: m.Name + " (Android)", Kind: "video", Ext: "mp4", SourceID: m.ID, UploadedBy: u.ID,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.worker.Enqueue(c.ID)
	s.audit(r, "media.convert", owner.ID, m.Name)
	writeJSON(w, http.StatusAccepted, s.toMediaDTO(u, c, s.nodeLookup(r)))
}

type renderReq struct {
	NodeID    int64           `json:"node_id"`
	Name      string          `json:"name"`
	Slideshow media.Slideshow `json:"slideshow"`
}

// renderSlideshow queues an MP4 made from still images. The images (and the
// music) must be visible everywhere the video will be, i.e. owned at or above
// its node, the same rule campaigns follow.
func (s *Server) renderSlideshow(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	var req renderReq
	if !readJSON(w, r, &req) {
		return
	}
	node, err := s.store.Node(r.Context(), req.NodeID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	if !auth.Can(u, node, auth.PermCreate) {
		forbidden(w)
		return
	}
	if s.worker.Tools().FFmpeg == "" {
		writeErr(w, http.StatusBadRequest, "ffmpeg is not available on the server")
		return
	}
	sh := req.Slideshow
	if err := sh.Normalize(); err != nil {
		writeErr(w, http.StatusBadRequest, err.Error())
		return
	}
	if sh.HasText() {
		if _, err := media.FindFont(media.Fonts[sh.Text.Font]); err != nil {
			writeErr(w, http.StatusBadRequest, "font not available on the server")
			return
		}
	}
	nodeOf := s.nodeLookup(r)
	check := func(id int64, kind string) string {
		m, err := s.store.Media(r.Context(), id)
		if err != nil || m.Kind != kind {
			return "unknown media"
		}
		if mo := nodeOf(m.NodeID); mo == nil || !node.IsUnder(mo) {
			return "media from another branch"
		}
		if m.Status != "ready" {
			return "media not ready"
		}
		return ""
	}
	for _, it := range sh.Items {
		if msg := check(it.MediaID, "image"); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
	}
	if sh.AudioID != 0 {
		if msg := check(sh.AudioID, "audio"); msg != "" {
			writeErr(w, http.StatusBadRequest, msg)
			return
		}
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		name = "Слайдшоу"
	}
	m, err := s.store.CreateMedia(r.Context(), store.NewMedia{
		Node: node, Name: name, Kind: "video", Ext: "mp4", Recipe: sh.JSON(), UploadedBy: u.ID,
	})
	if err != nil {
		s.fail(w, err)
		return
	}
	s.worker.Enqueue(m.ID)
	s.audit(r, "media.render", node.ID, name)
	writeJSON(w, http.StatusAccepted, s.toMediaDTO(u, m, nodeOf))
}
