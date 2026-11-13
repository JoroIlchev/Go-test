package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
	"time"
)

var ErrInUse = errors.New("in use")

type Campaign struct {
	ID             int64          `json:"id"`
	NodeID         int64          `json:"node_id"`
	ClientID       int64          `json:"client_id"`
	Name           string         `json:"name"`
	Draft          string         `json:"-"`
	DraftUpdatedAt sql.NullString `json:"-"`
	DraftUpdatedBy sql.NullInt64  `json:"-"`
	LiveVersionID  sql.NullInt64  `json:"-"`
	CreatedBy      sql.NullInt64  `json:"-"`
	CreatedAt      string         `json:"created_at"`
}

type Version struct {
	ID            int64          `json:"id"`
	CampaignID    int64          `json:"campaign_id"`
	Number        int            `json:"number"`
	Spec          string         `json:"-"`
	Status        string         `json:"status"`
	CreatedBy     sql.NullInt64  `json:"-"`
	CreatedAt     string         `json:"created_at"`
	ReviewedBy    sql.NullInt64  `json:"-"`
	ReviewedAt    sql.NullString `json:"-"`
	ReviewComment string         `json:"review_comment"`
}

const campaignCols = `id, node_id, client_id, name, draft, draft_updated_at, draft_updated_by,
	live_version_id, created_by, created_at`

func scanCampaign(row interface{ Scan(...any) error }) (*Campaign, error) {
	var c Campaign
	err := row.Scan(&c.ID, &c.NodeID, &c.ClientID, &c.Name, &c.Draft, &c.DraftUpdatedAt, &c.DraftUpdatedBy,
		&c.LiveVersionID, &c.CreatedBy, &c.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &c, err
}

const versionCols = `id, campaign_id, number, spec, status, created_by, created_at, reviewed_by, reviewed_at, review_comment`

func scanVersion(row interface{ Scan(...any) error }) (*Version, error) {
	var v Version
	err := row.Scan(&v.ID, &v.CampaignID, &v.Number, &v.Spec, &v.Status, &v.CreatedBy, &v.CreatedAt,
		&v.ReviewedBy, &v.ReviewedAt, &v.ReviewComment)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &v, err
}

func (s *Store) Campaign(ctx context.Context, id int64) (*Campaign, error) {
	return scanCampaign(s.db.QueryRowContext(ctx, `SELECT `+campaignCols+` FROM campaigns WHERE id = ?`, id))
}

// CampaignsAround returns campaigns owned by n's ancestors, n, or anything below n.
func (s *Store) CampaignsAround(ctx context.Context, n *Node) ([]*Campaign, error) {
	anc := n.AncestorIDs()
	args := []any{}
	for _, id := range anc {
		args = append(args, id)
	}
	args = append(args, n.Path+"%")
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+campaignCols+` FROM campaigns
		 WHERE node_id IN (`+placeholders(len(anc))+`) OR node_id IN (SELECT id FROM nodes WHERE path LIKE ?)
		 ORDER BY name`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Campaign
	for rows.Next() {
		c, err := scanCampaign(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// LiveCampaigns returns every campaign of a client that has a live version, with that version.
func (s *Store) LiveCampaigns(ctx context.Context, clientID int64) ([]*Campaign, []*Version, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT c.id, v.`+strings.ReplaceAll(versionCols, ", ", ", v.")+`
		 FROM campaigns c JOIN campaign_versions v ON v.id = c.live_version_id
		 WHERE (c.client_id = ? OR c.node_id IN (SELECT id FROM nodes WHERE house = 1))
		   AND v.status = 'approved' ORDER BY c.id`, clientID)
	if err != nil {
		return nil, nil, err
	}
	var ids []int64
	var versions []*Version
	for rows.Next() {
		var cid int64
		var v Version
		if err := rows.Scan(&cid, &v.ID, &v.CampaignID, &v.Number, &v.Spec, &v.Status, &v.CreatedBy,
			&v.CreatedAt, &v.ReviewedBy, &v.ReviewedAt, &v.ReviewComment); err != nil {
			rows.Close()
			return nil, nil, err
		}
		ids = append(ids, cid)
		versions = append(versions, &v)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, nil, err
	}
	campaigns := make([]*Campaign, len(ids))
	for i, id := range ids {
		c, err := s.Campaign(ctx, id)
		if err != nil {
			return nil, nil, err
		}
		campaigns[i] = c
	}
	return campaigns, versions, nil
}

func (s *Store) CreateCampaign(ctx context.Context, owner *Node, name, draft string, by int64) (*Campaign, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrInvalidArg
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO campaigns (node_id, client_id, name, draft, draft_updated_at, draft_updated_by, created_by)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`, owner.ID, owner.ClientID, name, draft, now(), by, by)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.Campaign(ctx, id)
}

// SaveDraft stores the working copy and the media it references.
func (s *Store) SaveDraft(ctx context.Context, id int64, name, draft string, mediaIDs []int64, by int64) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrInvalidArg
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx,
		`UPDATE campaigns SET name = ?, draft = ?, draft_updated_at = ?, draft_updated_by = ? WHERE id = ?`,
		name, draft, now(), by, id); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`DELETE FROM campaign_media WHERE campaign_id = ? AND version_id IS NULL`, id); err != nil {
		return err
	}
	if err := insertCampaignMedia(ctx, tx, id, nil, mediaIDs); err != nil {
		return err
	}
	return tx.Commit()
}

func insertCampaignMedia(ctx context.Context, tx *sql.Tx, campaignID int64, versionID any, mediaIDs []int64) error {
	for _, m := range mediaIDs {
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO campaign_media (campaign_id, version_id, media_id) VALUES (?, ?, ?)`,
			campaignID, versionID, m); err != nil {
			return err
		}
	}
	return nil
}

// CreateVersion freezes spec as the next version. An approved version goes
// live at once; a pending one waits for ReviewVersion.
func (s *Store) CreateVersion(ctx context.Context, campaignID int64, spec string, mediaIDs []int64,
	status string, by int64) (*Version, error) {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	var next int
	if err := tx.QueryRowContext(ctx,
		`SELECT COALESCE(MAX(number), 0) + 1 FROM campaign_versions WHERE campaign_id = ?`, campaignID).Scan(&next); err != nil {
		return nil, err
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO campaign_versions (campaign_id, number, spec, status, created_by) VALUES (?, ?, ?, ?, ?)`,
		campaignID, next, spec, status, by)
	if err != nil {
		return nil, err
	}
	vid, _ := res.LastInsertId()
	if err := insertCampaignMedia(ctx, tx, campaignID, vid, mediaIDs); err != nil {
		return nil, err
	}
	if status == "approved" {
		if _, err := tx.ExecContext(ctx, `UPDATE campaigns SET live_version_id = ? WHERE id = ?`, vid, campaignID); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Version(ctx, vid)
}

func (s *Store) Version(ctx context.Context, id int64) (*Version, error) {
	return scanVersion(s.db.QueryRowContext(ctx, `SELECT `+versionCols+` FROM campaign_versions WHERE id = ?`, id))
}

func (s *Store) Versions(ctx context.Context, campaignID int64) ([]*Version, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+versionCols+` FROM campaign_versions WHERE campaign_id = ? ORDER BY number DESC`, campaignID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Version
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// PendingVersions returns versions waiting for approval in campaigns owned at or below root.
func (s *Store) PendingVersions(ctx context.Context, root *Node) ([]*Version, error) {
	q := `SELECT v.` + strings.ReplaceAll(versionCols, ", ", ", v.") + `
		FROM campaign_versions v JOIN campaigns c ON c.id = v.campaign_id
		WHERE v.status = 'pending'`
	args := []any{}
	if root != nil {
		q += ` AND c.node_id IN (SELECT id FROM nodes WHERE path LIKE ?)`
		args = append(args, root.Path+"%")
	}
	rows, err := s.db.QueryContext(ctx, q+` ORDER BY v.created_at`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Version
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// ReviewVersion approves (and puts live) or rejects a pending version.
func (s *Store) ReviewVersion(ctx context.Context, v *Version, approve bool, comment string, by int64) error {
	status := "rejected"
	if approve {
		status = "approved"
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`UPDATE campaign_versions SET status = ?, reviewed_by = ?, reviewed_at = ?, review_comment = ?
		 WHERE id = ? AND status = 'pending'`, status, by, now(), strings.TrimSpace(comment), v.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrInvalidArg
	}
	if approve {
		if _, err := tx.ExecContext(ctx, `UPDATE campaigns SET live_version_id = ? WHERE id = ?`, v.ID, v.CampaignID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// SetLive puts an approved version live, or stops the campaign with versionID 0.
func (s *Store) SetLive(ctx context.Context, campaignID, versionID int64) error {
	var live any
	if versionID != 0 {
		live = versionID
	}
	_, err := s.db.ExecContext(ctx, `UPDATE campaigns SET live_version_id = ? WHERE id = ?`, live, campaignID)
	return err
}

func (s *Store) DeleteCampaign(ctx context.Context, id int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM campaigns WHERE id = ?`, id)
	return err
}

func (s *Store) MediaInUse(ctx context.Context, mediaID int64) (bool, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM campaign_media WHERE media_id = ?`, mediaID).Scan(&n)
	return n > 0, err
}

// MediaByIDs loads media rows by id, keyed by id.
func (s *Store) MediaByIDs(ctx context.Context, ids []int64) (map[int64]*Media, error) {
	out := map[int64]*Media{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	items, err := s.queryMedia(ctx, `SELECT `+mediaCols+` FROM media WHERE id IN (`+placeholders(len(ids))+`)`, args...)
	if err != nil {
		return nil, err
	}
	for _, m := range items {
		out[m.ID] = m
	}
	return out, nil
}

// SetServedVersions records which campaign versions a device's last index contained.
// It reports whether that differs from what the device had before.
func (s *Store) SetServedVersions(ctx context.Context, deviceID int64, versionIDs []int64) (changed bool, err error) {
	parts := make([]string, len(versionIDs))
	for i, id := range versionIDs {
		parts[i] = strconv.FormatInt(id, 10)
	}
	v := "," + strings.Join(parts, ",") + ","
	res, err := s.db.ExecContext(ctx, `UPDATE devices SET served_versions = ? WHERE id = ? AND served_versions != ?`,
		v, deviceID, v)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n > 0, err
}

// DevicesServed counts, among devices, those whose last index contained versionID.
func (s *Store) DevicesServed(ctx context.Context, deviceIDs []int64, versionID int64) (int, error) {
	if len(deviceIDs) == 0 {
		return 0, nil
	}
	args := []any{"%," + strconv.FormatInt(versionID, 10) + ",%"}
	for _, id := range deviceIDs {
		args = append(args, id)
	}
	var n int
	err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM devices WHERE served_versions LIKE ? AND id IN (`+placeholders(len(deviceIDs))+`)`,
		args...).Scan(&n)
	return n, err
}

// DevicesLate counts, among devices, those that do not have versionID yet and
// will next check in only after start; latest is the last of those checks.
func (s *Store) DevicesLate(ctx context.Context, deviceIDs []int64, versionID int64, start time.Time) (n int, latest string, err error) {
	if len(deviceIDs) == 0 {
		return 0, "", nil
	}
	args := []any{"%," + strconv.FormatInt(versionID, 10) + ",%", start.UTC().Format(time.RFC3339)}
	for _, id := range deviceIDs {
		args = append(args, id)
	}
	var max sql.NullString
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*), MAX(next_check_at) FROM devices
		 WHERE served_versions NOT LIKE ? AND next_check_at > ? AND id IN (`+placeholders(len(deviceIDs))+`)`,
		args...).Scan(&n, &max)
	return n, max.String, err
}

func placeholders(n int) string {
	if n == 0 {
		return "NULL"
	}
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}
