package store

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"errors"
	"strings"
	"time"
)

type Device struct {
	ID           int64          `json:"id"`
	NodeID       sql.NullInt64  `json:"-"`
	UUID         sql.NullString `json:"-"`
	Name         string         `json:"name"`
	Token        string         `json:"token"`
	PairingCode  string         `json:"pairing_code"`
	Status       string         `json:"status"`
	AuthMode     string         `json:"auth_mode"`
	AuthUser     string         `json:"auth_user"`
	AuthPassHash string         `json:"-"`
	UserAgent    string         `json:"user_agent"`
	LastIP       string         `json:"last_ip"`
	LastSeenAt   sql.NullString `json:"-"`
	CreatedAt    string         `json:"created_at"`
	NextCheckAt  sql.NullString `json:"-"` // when the device was told to come back
	OfflineSince sql.NullString `json:"-"` // set when it missed that and an alert went out
}

var ErrNotFound = errors.New("not found")

const deviceCols = `id, node_id, uuid, name, token, pairing_code, status, auth_mode,
	auth_user, auth_pass_hash, user_agent, last_ip, last_seen_at, created_at, next_check_at, offline_since`

func scanDevice(row interface{ Scan(...any) error }) (*Device, error) {
	var d Device
	err := row.Scan(&d.ID, &d.NodeID, &d.UUID, &d.Name, &d.Token, &d.PairingCode, &d.Status,
		&d.AuthMode, &d.AuthUser, &d.AuthPassHash, &d.UserAgent, &d.LastIP, &d.LastSeenAt, &d.CreatedAt,
		&d.NextCheckAt, &d.OfflineSince)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &d, err
}

func (s *Store) Device(ctx context.Context, id int64) (*Device, error) {
	return scanDevice(s.db.QueryRowContext(ctx, `SELECT `+deviceCols+` FROM devices WHERE id = ?`, id))
}

func (s *Store) DeviceByUUID(ctx context.Context, uuid string) (*Device, error) {
	return scanDevice(s.db.QueryRowContext(ctx,
		`SELECT `+deviceCols+` FROM devices WHERE uuid = ?`, uuid))
}

func (s *Store) DeviceByToken(ctx context.Context, token string) (*Device, error) {
	return scanDevice(s.db.QueryRowContext(ctx,
		`SELECT `+deviceCols+` FROM devices WHERE token = ?`, token))
}

func (s *Store) PendingDevices(ctx context.Context) ([]*Device, error) {
	return s.queryDevices(ctx,
		`SELECT `+deviceCols+` FROM devices WHERE status = 'pending' ORDER BY created_at DESC`)
}

// DevicesUnder returns the devices placed anywhere in root's subtree.
func (s *Store) DevicesUnder(ctx context.Context, root *Node) ([]*Device, error) {
	return s.queryDevices(ctx,
		`SELECT `+deviceCols+` FROM devices
		 WHERE node_id IN (SELECT id FROM nodes WHERE path LIKE ?) ORDER BY name`, root.Path+"%")
}

func (s *Store) queryDevices(ctx context.Context, q string, args ...any) ([]*Device, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Device
	for rows.Next() {
		d, err := scanDevice(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// RegisterDevice creates a pending device for a player that called in with an
// unknown UUID. A concurrent registration of the same UUID returns the existing row.
func (s *Store) RegisterDevice(ctx context.Context, uuid, name, userAgent, ip string) (*Device, error) {
	for attempt := 0; attempt < 5; attempt++ {
		_, err := s.db.ExecContext(ctx,
			`INSERT INTO devices (uuid, name, token, pairing_code, user_agent, last_ip, last_seen_at)
			 VALUES (?, ?, ?, ?, ?, ?, ?)
			 ON CONFLICT(uuid) DO NOTHING`,
			uuid, name, newToken(), newPairingCode(), userAgent, ip, now())
		if err == nil {
			return s.DeviceByUUID(ctx, uuid)
		}
		// Token or pairing code collision: retry with fresh random values.
		if !strings.Contains(err.Error(), "UNIQUE") {
			return nil, err
		}
	}
	return nil, errors.New("could not allocate unique device token")
}

// CreateLinkDevice creates an active device without a UUID, for a player that
// is configured with its own secret URL instead of self-registering.
func (s *Store) CreateLinkDevice(ctx context.Context, site *Node, name string) (*Device, error) {
	if site.Kind != "site" {
		return nil, ErrBadParent
	}
	res, err := s.db.ExecContext(ctx,
		`INSERT INTO devices (node_id, name, token, pairing_code, status) VALUES (?, ?, ?, ?, 'active')`,
		site.ID, strings.TrimSpace(name), newToken(), newPairingCode())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	s.recordHistory(ctx, id, site, "active")
	return s.Device(ctx, id)
}

// PairDevice activates the pending device showing code and places it on site.
func (s *Store) PairDevice(ctx context.Context, code string, site *Node, name string) (*Device, error) {
	if site.Kind != "site" {
		return nil, ErrBadParent
	}
	code = strings.ToUpper(strings.TrimSpace(code))
	res, err := s.db.ExecContext(ctx,
		`UPDATE devices SET node_id = ?, name = ?, status = 'active'
		 WHERE pairing_code = ? AND status = 'pending'`,
		site.ID, strings.TrimSpace(name), code)
	if err != nil {
		return nil, err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return nil, ErrNotFound
	}
	d, err := scanDevice(s.db.QueryRowContext(ctx,
		`SELECT `+deviceCols+` FROM devices WHERE pairing_code = ?`, code))
	if err != nil {
		return nil, err
	}
	s.recordHistory(ctx, d.ID, site, "active")
	return d, nil
}

type DeviceUpdate struct {
	Name         *string
	Site         *Node
	Status       *string
	AuthMode     *string
	AuthUser     *string
	AuthPassHash *string
	RotateToken  bool
}

func (s *Store) UpdateDevice(ctx context.Context, id int64, upd DeviceUpdate) (*Device, error) {
	if upd.Site != nil && upd.Site.Kind != "site" {
		return nil, ErrBadParent
	}
	sets, args := []string{}, []any{}
	add := func(col string, v any) { sets = append(sets, col+" = ?"); args = append(args, v) }
	if upd.Name != nil {
		add("name", strings.TrimSpace(*upd.Name))
	}
	if upd.Site != nil {
		add("node_id", upd.Site.ID)
	}
	if upd.Status != nil {
		add("status", *upd.Status)
	}
	if upd.AuthMode != nil {
		add("auth_mode", *upd.AuthMode)
	}
	if upd.AuthUser != nil {
		add("auth_user", strings.TrimSpace(*upd.AuthUser))
	}
	if upd.AuthPassHash != nil {
		add("auth_pass_hash", *upd.AuthPassHash)
	}
	if upd.RotateToken {
		add("token", newToken())
	}
	if len(sets) > 0 {
		args = append(args, id)
		if _, err := s.db.ExecContext(ctx,
			`UPDATE devices SET `+strings.Join(sets, ", ")+` WHERE id = ?`, args...); err != nil {
			if strings.Contains(err.Error(), "CHECK") {
				return nil, ErrInvalidArg
			}
			return nil, err
		}
	}
	d, err := s.Device(ctx, id)
	if err != nil {
		return nil, err
	}
	if upd.Site != nil || upd.Status != nil {
		var site *Node
		if d.NodeID.Valid {
			site, _ = s.Node(ctx, d.NodeID.Int64)
		}
		s.recordHistory(ctx, d.ID, site, d.Status)
	}
	return d, nil
}

func (s *Store) recordHistory(ctx context.Context, deviceID int64, site *Node, status string) {
	var nodeID, clientID any
	if site != nil {
		nodeID, clientID = site.ID, site.ClientID
	}
	s.db.ExecContext(ctx,
		`INSERT INTO device_history (device_id, node_id, client_id, status) VALUES (?, ?, ?, ?)`,
		deviceID, nodeID, clientID, status)
}

func (s *Store) TouchDevice(ctx context.Context, id int64, userAgent, ip string) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE devices SET user_agent = ?, last_ip = ?, last_seen_at = ? WHERE id = ?`,
		userAgent, ip, now(), id)
	return err
}

// SetNextCheck records when the device was told to come back.
func (s *Store) SetNextCheck(ctx context.Context, id int64, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE devices SET next_check_at = ? WHERE id = ?`,
		at.UTC().Format(time.RFC3339), id)
	return err
}

// OverdueDevices are active devices, not marked offline yet, whose expected
// check fell between after and before without them showing up.
func (s *Store) OverdueDevices(ctx context.Context, after, before time.Time) ([]*Device, error) {
	return s.queryDevices(ctx,
		`SELECT `+deviceCols+` FROM devices
		 WHERE status = 'active' AND offline_since IS NULL AND next_check_at >= ? AND next_check_at < ?
		 ORDER BY id`, after.UTC().Format(time.RFC3339), before.UTC().Format(time.RFC3339))
}

// AdminEmails returns the e-mails of active users holding an admin role on any of nodeIDs.
func (s *Store) AdminEmails(ctx context.Context, nodeIDs []int64) ([]string, error) {
	if len(nodeIDs) == 0 {
		return nil, nil
	}
	args := make([]any, len(nodeIDs))
	for i, id := range nodeIDs {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT u.email FROM users u JOIN user_roles r ON r.user_id = u.id
		 WHERE u.active = 1 AND r.role = 'admin' AND r.node_id IN (`+placeholders(len(nodeIDs))+`) ORDER BY u.email`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var e string
		if err := rows.Scan(&e); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// ReturnedDevices are devices marked offline that were seen since, or that
// are no longer active (blocked or unpaired), so the mark should go.
func (s *Store) ReturnedDevices(ctx context.Context) ([]*Device, error) {
	return s.queryDevices(ctx,
		`SELECT `+deviceCols+` FROM devices
		 WHERE offline_since IS NOT NULL AND (last_seen_at > offline_since OR status != 'active') ORDER BY id`)
}

// SetOffline marks the device offline since at, or clears the mark for a zero at.
func (s *Store) SetOffline(ctx context.Context, id int64, at time.Time) error {
	var v any
	if !at.IsZero() {
		v = at.UTC().Format(time.RFC3339)
	}
	_, err := s.db.ExecContext(ctx, `UPDATE devices SET offline_since = ? WHERE id = ?`, v, id)
	return err
}

func (s *Store) AllDevices(ctx context.Context) ([]*Device, error) {
	return s.queryDevices(ctx, `SELECT `+deviceCols+` FROM devices ORDER BY id`)
}

func now() string { return time.Now().UTC().Format(time.RFC3339) }

// newToken returns 128 random bits, URL-safe; used as the device's secret path.
func newToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// Pairing codes are read off a screen by a human, so skip look-alikes (0/O, 1/I/L).
const pairingAlphabet = "ABCDEFGHJKMNPQRSTUVWXYZ23456789"

func newPairingCode() string {
	b := make([]byte, 6)
	rand.Read(b)
	for i := range b {
		b[i] = pairingAlphabet[int(b[i])%len(pairingAlphabet)]
	}
	return string(b)
}
