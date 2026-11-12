package player

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"

	"marquee/internal/campaign"
	"marquee/internal/store"
)

// Marquee Player's API. The player calls POST /api/v1/sync in a loop:
// it reports itself and gets its manifest (what to play when, and the files
// to download). With "wait" the hub holds the call until the manifest
// differs from the version the player has, or waitMax passes, so a published
// campaign reaches the screen in seconds and every call doubles as a
// heartbeat. The device is identified by the random id the app generated on
// install, sent in X-Device-Id; it is the device's secret.

const waitMax = 50 * time.Second

var deviceIDPattern = regexp.MustCompile(`^[A-Za-z0-9-]{16,64}$`)

type syncRequest struct {
	Info    store.DeviceInfo `json:"info"`
	Version string           `json:"version"` // manifest version the player has
	Wait    bool             `json:"wait"`
}

type manifest struct {
	Version     string         `json:"version"`
	Status      string         `json:"status"` // pending | active
	PairingCode string         `json:"pairing_code,omitempty"`
	Name        string         `json:"name,omitempty"`
	Site        string         `json:"site,omitempty"`
	Timezone    string         `json:"timezone"`
	Segments    []manifestSeg  `json:"segments"`
	Files       []manifestFile `json:"files"`
}

// manifestSeg plays its items in a loop from Start until End ("" = no end).
type manifestSeg struct {
	Start string         `json:"start"`
	End   string         `json:"end"`
	Items []manifestItem `json:"items"`
	Audio string         `json:"audio,omitempty"`
}

type manifestItem struct {
	File       string `json:"file"`
	DurationMS int64  `json:"duration_ms"` // 0: a video plays to its end
	Fit        string `json:"fit"`         // meet | fill | slice
}

type manifestFile struct {
	Name       string `json:"name"`
	URL        string `json:"url"`
	SHA256     string `json:"sha256"`
	Size       int64  `json:"size"`
	Kind       string `json:"kind"`
	Width      int    `json:"width,omitempty"`
	Height     int    `json:"height,omitempty"`
	DurationMS int64  `json:"duration_ms,omitempty"`
}

func (h *Handler) sync(w http.ResponseWriter, r *http.Request) {
	id := strings.ToLower(r.Header.Get("X-Device-Id"))
	if !deviceIDPattern.MatchString(id) {
		writeAPIError(w, http.StatusBadRequest, "missing or invalid X-Device-Id")
		return
	}
	var in syncRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 64<<10)).Decode(&in); err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid body")
		return
	}
	ctx := r.Context()
	ua := playerAgent(in.Info)
	d, err := h.store.DeviceByUUID(ctx, id)
	if errors.Is(err, store.ErrNotFound) {
		name := strings.TrimSpace(in.Info.Manufacturer + " " + in.Info.Model)
		if d, err = h.store.RegisterDevice(ctx, id, name, ua, clientIP(r)); err == nil {
			h.log.Info("Ново устройство чака свързване", "код", d.PairingCode, "име", name, "ip", clientIP(r))
		}
	}
	if err != nil {
		h.fail(w, err)
		return
	}
	if err := h.store.TouchDevice(ctx, d.ID, ua, clientIP(r)); err != nil {
		h.log.Warn("Посещението на устройството не се записа", "устройство", d.ID, "грешка", err)
	}
	if err := h.store.SetDeviceInfo(ctx, d.ID, in.Info); err != nil {
		h.log.Warn("Данните на устройството не се записаха", "устройство", d.ID, "грешка", err)
	}

	timeout := time.NewTimer(waitMax)
	defer timeout.Stop()
	for {
		// Take the channel before reading, so a change in between still wakes us.
		changed := h.store.Changed()
		if d, err = h.store.Device(ctx, d.ID); err != nil {
			h.fail(w, err)
			return
		}
		if d.Status == "blocked" {
			writeAPIError(w, http.StatusForbidden, "blocked")
			return
		}
		m, err := h.manifest(ctx, r, d)
		if err != nil {
			h.fail(w, err)
			return
		}
		if !in.Wait || m.Version != in.Version {
			// The player is back within waitMax, or right after playing this.
			h.setNextCheck(r, d, time.Now(), int(waitMax/time.Second)+10)
			writeJSON(w, m)
			return
		}
		select {
		case <-changed:
		case <-timeout.C:
			h.setNextCheck(r, d, time.Now(), int(waitMax/time.Second)+10)
			writeJSON(w, m)
			return
		case <-ctx.Done():
			return
		}
	}
}

// manifest builds what the device plays and records which versions it got.
func (h *Handler) manifest(ctx context.Context, r *http.Request, d *store.Device) (*manifest, error) {
	m := &manifest{Status: d.Status, Name: d.Name, Timezone: h.cfg.Loc.String(),
		Segments: []manifestSeg{}, Files: []manifestFile{}}
	if d.Status != "active" || !d.NodeID.Valid {
		m.Status = "pending"
		m.PairingCode = d.PairingCode
		m.Version = versionOf(m)
		return m, nil
	}
	site, err := h.store.Node(ctx, d.NodeID.Int64)
	if err != nil {
		return nil, err
	}
	m.Site = site.Name
	playables, media, labelOf, err := h.live(ctx, site)
	if err != nil {
		return nil, err
	}
	base := requestBase(r, h.cfg.PublicURL)
	b := campaign.Builder{Loc: h.cfg.Loc, Seed: uint64(d.ID)}
	segments, versions := b.Plan(time.Now(), playables, media)
	files := map[string]bool{}
	addFile := func(f *store.Media) string {
		name := f.FileName()
		if !files[name] {
			files[name] = true
			m.Files = append(m.Files, manifestFile{Name: name, URL: base + "/m/" + name, SHA256: f.SHA256,
				Size: f.Size, Kind: f.Kind, Width: f.Width, Height: f.Height, DurationMS: f.DurationMS})
		}
		return name
	}
	for _, s := range segments {
		ms := manifestSeg{Start: s.Start.Format(time.RFC3339), Items: []manifestItem{}}
		if !s.End.IsZero() {
			ms.End = s.End.Format(time.RFC3339)
		}
		for _, it := range s.Items {
			ms.Items = append(ms.Items, manifestItem{File: addFile(it.Media),
				DurationMS: int64(it.DurationS * 1000), Fit: it.Fit})
		}
		if s.Audio != nil {
			ms.Audio = addFile(s.Audio)
		}
		m.Segments = append(m.Segments, ms)
	}
	m.Version = versionOf(m)

	changed, err := h.store.SetServedVersions(ctx, d.ID, versions)
	if err != nil {
		h.log.Warn("Не се записа какво е получило устройството", "устройство", d.ID, "грешка", err)
	}
	if changed {
		var labels []string
		for _, id := range versions {
			labels = append(labels, labelOf[id])
		}
		what := strings.Join(labels, ", ")
		if what == "" {
			what = "няма активни кампании"
		}
		h.log.Info("Устройството получи ново съдържание", "устройство", deviceLabel(d, site), "кампании", what)
	}
	return m, nil
}

// versionOf hashes the manifest itself: the same content, the same version.
func versionOf(m *manifest) string {
	m.Version = ""
	b, _ := json.Marshal(m)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:12])
}

func playerAgent(i store.DeviceInfo) string {
	return strings.TrimSpace("marquee-player/" + i.App + " (" + strings.TrimSpace(i.Manufacturer+" "+i.Model) +
		"; Android " + i.Android + ")")
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	json.NewEncoder(w).Encode(v)
}

func writeAPIError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
