package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
)

type Media struct {
	ID         int64         `json:"id"`
	NodeID     int64         `json:"node_id"`
	ClientID   int64         `json:"client_id"`
	Name       string        `json:"name"`
	Kind       string        `json:"kind"`
	SHA256     string        `json:"sha256"`
	Ext        string        `json:"ext"`
	Size       int64         `json:"size"`
	Width      int           `json:"width"`
	Height     int           `json:"height"`
	DurationMS int64         `json:"duration_ms"`
	VideoCodec string        `json:"video_codec"`
	AudioCodec string        `json:"audio_codec"`
	FPS        float64       `json:"fps"`
	Bitrate    int64         `json:"bitrate"`
	Status     string        `json:"status"`
	Warnings   string        `json:"-"`
	Error      string        `json:"error"`
	HasThumb   bool          `json:"has_thumb"`
	SourceID   sql.NullInt64 `json:"-"`
	UploadedBy sql.NullInt64 `json:"-"`
	Recipe     string        `json:"-"`
	Prep       string        `json:"-"`
	CreatedAt  string        `json:"created_at"`
}

// FileName is the blob's name in storage and in player URLs.
func (m *Media) FileName() string { return m.SHA256 + "." + m.Ext }

const mediaCols = `id, node_id, client_id, name, kind, sha256, ext, size, width, height, duration_ms,
	video_codec, audio_codec, fps, bitrate, status, warnings, error, has_thumb, source_id, uploaded_by, recipe, prep, created_at`

func scanMedia(row interface{ Scan(...any) error }) (*Media, error) {
	var m Media
	err := row.Scan(&m.ID, &m.NodeID, &m.ClientID, &m.Name, &m.Kind, &m.SHA256, &m.Ext, &m.Size, &m.Width,
		&m.Height, &m.DurationMS, &m.VideoCodec, &m.AudioCodec, &m.FPS, &m.Bitrate, &m.Status, &m.Warnings,
		&m.Error, &m.HasThumb, &m.SourceID, &m.UploadedBy, &m.Recipe, &m.Prep, &m.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &m, err
}

func (s *Store) queryMedia(ctx context.Context, q string, args ...any) ([]*Media, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Media
	for rows.Next() {
		m, err := scanMedia(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) Media(ctx context.Context, id int64) (*Media, error) {
	return scanMedia(s.db.QueryRowContext(ctx, `SELECT `+mediaCols+` FROM media WHERE id = ?`, id))
}

// MediaAround returns what is relevant at node n: items owned by n's ancestors
// (inherited), by n itself, and by everything below n.
func (s *Store) MediaAround(ctx context.Context, n *Node) ([]*Media, error) {
	anc := n.AncestorIDs()
	ph := strings.TrimSuffix(strings.Repeat("?,", len(anc)), ",")
	args := []any{}
	for _, id := range anc {
		args = append(args, id)
	}
	args = append(args, n.Path+"%")
	return s.queryMedia(ctx,
		`SELECT `+mediaCols+` FROM media
		 WHERE node_id IN (`+ph+`) OR node_id IN (SELECT id FROM nodes WHERE path LIKE ?)
		 ORDER BY created_at DESC, id DESC`, args...)
}

func (s *Store) MediaByStatus(ctx context.Context, status string) ([]*Media, error) {
	return s.queryMedia(ctx, `SELECT `+mediaCols+` FROM media WHERE status = ? ORDER BY id`, status)
}

// MediaBySHA finds a ready item with this content, to reuse its probe results.
func (s *Store) MediaBySHA(ctx context.Context, sha string) (*Media, error) {
	return scanMedia(s.db.QueryRowContext(ctx,
		`SELECT `+mediaCols+` FROM media WHERE sha256 = ? AND status = 'ready' LIMIT 1`, sha))
}

type NewMedia struct {
	Node       *Node
	Name       string
	Kind       string
	SHA256     string
	Ext        string
	Size       int64
	SourceID   int64
	Recipe     string
	Prep       string
	UploadedBy int64
}

func (s *Store) CreateMedia(ctx context.Context, nm NewMedia) (*Media, error) {
	var source, by any
	if nm.SourceID != 0 {
		source = nm.SourceID
	}
	if nm.UploadedBy != 0 {
		by = nm.UploadedBy
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO media (node_id, client_id, name, kind, sha256, ext, size, source_id, recipe, prep, uploaded_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		nm.Node.ID, nm.Node.ClientID, strings.TrimSpace(nm.Name), nm.Kind, nm.SHA256, nm.Ext, nm.Size, source, nm.Recipe, nm.Prep, by)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.Media(ctx, id)
}

// MediaProbe is what processing learns about a file.
type MediaProbe struct {
	SHA256     string
	Ext        string // "" keeps the item's extension
	Size       int64
	Width      int
	Height     int
	DurationMS int64
	VideoCodec string
	AudioCodec string
	FPS        float64
	Bitrate    int64
	Warnings   string
	HasThumb   bool
}

func (s *Store) MarkMediaReady(ctx context.Context, id int64, p MediaProbe) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE media SET status = 'ready', error = '', sha256 = ?, ext = COALESCE(NULLIF(?, ''), ext), size = ?, width = ?, height = ?,
		 duration_ms = ?, video_codec = ?, audio_codec = ?, fps = ?, bitrate = ?, warnings = ?, has_thumb = ?
		 WHERE id = ?`,
		p.SHA256, p.Ext, p.Size, p.Width, p.Height, p.DurationMS, p.VideoCodec, p.AudioCodec, p.FPS, p.Bitrate,
		p.Warnings, p.HasThumb, id)
	s.notify()
	return err
}

func (s *Store) MarkMediaFailed(ctx context.Context, id int64, msg string) error {
	_, err := s.db.ExecContext(ctx, `UPDATE media SET status = 'failed', error = ? WHERE id = ?`, msg, id)
	s.notify()
	return err
}

func (s *Store) RenameMedia(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrInvalidArg
	}
	_, err := s.db.ExecContext(ctx, `UPDATE media SET name = ? WHERE id = ?`, name, id)
	return err
}

// DeleteMedia removes the row and reports whether its blob is now unused.
func (s *Store) DeleteMedia(ctx context.Context, m *Media) (orphan bool, err error) {
	if _, err := s.db.ExecContext(ctx, `DELETE FROM media WHERE id = ?`, m.ID); err != nil {
		return false, err
	}
	if m.SHA256 == "" {
		return false, nil
	}
	var n int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media WHERE sha256 = ?`, m.SHA256).Scan(&n)
	return n == 0, err
}

// BlobInUse reports whether any item still points at the blob.
func (s *Store) BlobInUse(ctx context.Context, sha, ext string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM media WHERE sha256 = ? AND ext = ?`, sha, ext).Scan(&n)
	return n > 0, err
}

// MediaFileExists reports whether any item serves this blob, so players can
// only fetch files that are actually in the library.
func (s *Store) MediaFileExists(ctx context.Context, sha, ext string) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM media WHERE sha256 = ? AND ext = ? AND status = 'ready'`, sha, ext).Scan(&n)
	return n > 0, err
}
