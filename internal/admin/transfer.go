package admin

import (
	"archive/zip"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"strconv"

	"marquee/internal/auth"
	"marquee/internal/campaign"
	"marquee/internal/media"
	"marquee/internal/store"
)

// A campaign moves between hubs as one zip: campaign.json (the draft and
// the media it uses) plus media/<sha256>.<ext>. So the slow work (videos
// made from pictures, conversions) can be done on a home machine and only
// the finished files go to the server.

const transferFormat = "marquee-campaign"

type transferFile struct {
	Format  string          `json:"format"`
	Version int             `json:"version"`
	Name    string          `json:"name"`
	Spec    campaign.Spec   `json:"spec"`
	Media   []transferMedia `json:"media"`
}

type transferMedia struct {
	ID     int64  `json:"id"` // in the exporting hub, to map the spec
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Ext    string `json:"ext"`
	SHA256 string `json:"sha256"`
}

func transferPath(m transferMedia) string { return "media/" + m.SHA256 + "." + m.Ext }

func (s *Server) exportCampaign(w http.ResponseWriter, r *http.Request) {
	c, _ := s.campaignFor(w, r)
	if c == nil {
		return
	}
	spec, err := campaign.ParseSpec(c.Draft)
	if err != nil {
		s.fail(w, err)
		return
	}
	spec.Targets = []int64{} // node ids mean nothing on another hub
	all, err := s.store.MediaByIDs(r.Context(), spec.MediaIDs())
	if err != nil {
		s.fail(w, err)
		return
	}
	tf := transferFile{Format: transferFormat, Version: 1, Name: c.Name, Spec: spec, Media: []transferMedia{}}
	for _, id := range spec.MediaIDs() {
		if m := all[id]; m != nil && m.Status == "ready" {
			tf.Media = append(tf.Media, transferMedia{ID: m.ID, Name: m.Name, Kind: m.Kind, Ext: m.Ext, SHA256: m.SHA256})
		}
	}

	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", mime.FormatMediaType("attachment", map[string]string{"filename": c.Name + ".zip"}))
	zw := zip.NewWriter(w)
	head, err := zw.Create("campaign.json")
	if err == nil {
		enc := json.NewEncoder(head)
		enc.SetIndent("", "  ")
		err = enc.Encode(tf)
	}
	for _, m := range tf.Media {
		if err != nil {
			break
		}
		// Media is compressed already: store it as is.
		var dst io.Writer
		if dst, err = zw.CreateHeader(&zip.FileHeader{Name: transferPath(m), Method: zip.Store}); err != nil {
			break
		}
		var f *os.File
		if f, err = os.Open(s.storage.Path(m.SHA256, m.Ext)); err != nil {
			break
		}
		_, err = io.Copy(dst, f)
		f.Close()
	}
	if err == nil {
		err = zw.Close()
	}
	if err != nil {
		// Headers are gone already; the client gets a broken zip.
		s.log.Warn("Експортът на кампания не завърши", "кампания", c.Name, "грешка", err)
		return
	}
	s.audit(r, "campaign.export", c.NodeID, c.Name)
}

// noPrep keeps imported files exactly as they came: they were prepared on
// the hub that exported them.
var noPrep = func() string {
	b, _ := json.Marshal(media.Prep{ImageScreen: "off", ImageQuality: "high", VideoAuto: "off", VideoMax: 1080, VideoQuality: "high"})
	return string(b)
}()

// importCampaign creates a draft campaign in node_id from an exported zip.
// Files the node can already use are not stored twice.
func (s *Server) importCampaign(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	n := s.nodeParam(w, r)
	if n == nil {
		return
	}
	if !auth.Can(u, n, auth.PermCreate) {
		forbidden(w)
		return
	}
	path, ok := s.receiveZip(w, r)
	if !ok {
		return
	}
	defer os.Remove(path)
	zr, err := zip.OpenReader(path)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "not a campaign file")
		return
	}
	defer zr.Close()
	files := map[string]*zip.File{}
	for _, f := range zr.File {
		files[f.Name] = f
	}
	var tf transferFile
	if f := files["campaign.json"]; f != nil {
		if rc, err := f.Open(); err == nil {
			err = json.NewDecoder(io.LimitReader(rc, 1<<20)).Decode(&tf)
			rc.Close()
		}
	}
	if tf.Format != transferFormat || tf.Version != 1 {
		writeErr(w, http.StatusBadRequest, "not a campaign file")
		return
	}

	nodeOf := s.nodeLookup(r)
	around, err := s.store.MediaAround(r.Context(), n)
	if err != nil {
		s.fail(w, err)
		return
	}
	have := map[string]int64{}
	for _, m := range around {
		if mo := nodeOf(m.NodeID); mo != nil && n.IsUnder(mo) && m.Status != "failed" {
			have[m.SHA256+"."+m.Ext] = m.ID
		}
	}
	ids := map[int64]int64{}
	for _, tm := range tf.Media {
		if id, ok := have[tm.SHA256+"."+tm.Ext]; ok {
			ids[tm.ID] = id
			continue
		}
		f := files[transferPath(tm)]
		kind, ext, ok := media.KindOf("x." + tm.Ext)
		if f == nil || !ok || kind != tm.Kind {
			continue // the item is dropped from the playlist
		}
		rc, err := f.Open()
		if err != nil {
			writeErr(w, http.StatusBadRequest, "damaged campaign file")
			return
		}
		sha, size, err := s.storage.Save(rc, ext, s.maxUpload)
		rc.Close()
		if err != nil || sha != tm.SHA256 {
			writeErr(w, http.StatusBadRequest, "damaged campaign file")
			return
		}
		m, err := s.store.CreateMedia(r.Context(), store.NewMedia{
			Node: n, Name: tm.Name, Kind: kind, SHA256: sha, Ext: ext, Size: size, Prep: noPrep, UploadedBy: u.ID,
		})
		if err != nil {
			s.fail(w, err)
			return
		}
		s.worker.Enqueue(m.ID)
		have[sha+"."+ext] = m.ID
		ids[tm.ID] = m.ID
	}

	spec := tf.Spec
	items := []campaign.Item{}
	for _, it := range spec.Items {
		if id, ok := ids[it.MediaID]; ok {
			it.MediaID = id
			items = append(items, it)
		}
	}
	spec.Items, spec.AudioID = items, ids[spec.AudioID]
	spec.Targets = []int64{n.ID}
	if n.House {
		spec.Targets = []int64{}
		if spec.PerDay == 0 {
			spec.PerDay = 10
		}
	} else {
		spec.PerDay = 0
	}
	if msg := s.checkSpec(r, spec, n, false); msg != "" {
		writeErr(w, http.StatusBadRequest, msg)
		return
	}
	c, err := s.store.CreateCampaign(r.Context(), n, tf.Name, spec.JSON(), u.ID)
	if err != nil {
		s.storeErr(w, err)
		return
	}
	if err := s.store.SaveDraft(r.Context(), c.ID, c.Name, spec.JSON(), spec.MediaIDs(), u.ID); err != nil {
		s.fail(w, err)
		return
	}
	s.audit(r, "campaign.import", n.ID, c.Name+" ("+strconv.Itoa(len(tf.Media))+" файла)")
	writeJSON(w, http.StatusCreated, map[string]int64{"id": c.ID})
}

// receiveZip stores the uploaded file in a temporary file: a zip is read
// from its end, so it cannot be streamed.
func (s *Server) receiveZip(w http.ResponseWriter, r *http.Request) (string, bool) {
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "multipart body expected")
		return "", false
	}
	for {
		part, err := mr.NextPart()
		if errors.Is(err, io.EOF) {
			writeErr(w, http.StatusBadRequest, "no file")
			return "", false
		}
		if err != nil {
			writeErr(w, http.StatusBadRequest, "upload interrupted")
			return "", false
		}
		if part.FileName() == "" {
			part.Close()
			continue
		}
		f, err := s.storage.TempFile("import-*.zip")
		if err != nil {
			s.fail(w, err)
			return "", false
		}
		n, err := io.Copy(f, io.LimitReader(part, s.maxUpload+1))
		part.Close()
		f.Close()
		switch {
		case err != nil:
			os.Remove(f.Name())
			writeErr(w, http.StatusBadRequest, "upload interrupted")
			return "", false
		case n > s.maxUpload:
			os.Remove(f.Name())
			writeErr(w, http.StatusRequestEntityTooLarge, "file too large")
			return "", false
		}
		return f.Name(), true
	}
}
