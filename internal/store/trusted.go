package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// TrustedDevice is a browser a user chose to remember at login.
type TrustedDevice struct {
	ID         int64  `json:"id"`
	UserID     int64  `json:"-"`
	IP         string `json:"ip"`
	UserAgent  string `json:"user_agent"`
	CreatedAt  string `json:"created_at"`
	LastUsedAt string `json:"last_used_at"`
}

// CreateTrustedDevice remembers a browser for ttl and drops the user's expired ones.
func (s *Store) CreateTrustedDevice(ctx context.Context, tokenHash string, userID int64, ip, ua string, ttl time.Duration) error {
	now := time.Now().UTC()
	if _, err := s.db.ExecContext(ctx, `DELETE FROM trusted_devices WHERE user_id = ? AND expires_at <= ?`,
		userID, now.Format(time.RFC3339)); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO trusted_devices (token_hash, user_id, ip, user_agent, created_at, last_used_at, expires_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		tokenHash, userID, ip, ua, now.Format(time.RFC3339), now.Format(time.RFC3339), now.Add(ttl).Format(time.RFC3339))
	return err
}

// TrustedDevice returns the non-expired remembered browser behind a token.
func (s *Store) TrustedDevice(ctx context.Context, tokenHash string) (*TrustedDevice, error) {
	d := &TrustedDevice{}
	err := s.db.QueryRowContext(ctx,
		`SELECT id, user_id, ip, user_agent, created_at, last_used_at FROM trusted_devices
		 WHERE token_hash = ? AND expires_at > ?`,
		tokenHash, time.Now().UTC().Format(time.RFC3339)).
		Scan(&d.ID, &d.UserID, &d.IP, &d.UserAgent, &d.CreatedAt, &d.LastUsedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return d, err
}

// TouchTrustedDevice records a login from a remembered browser and keeps it for another ttl.
func (s *Store) TouchTrustedDevice(ctx context.Context, id int64, ip, ua string, ttl time.Duration) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`UPDATE trusted_devices SET ip = ?, user_agent = ?, last_used_at = ?, expires_at = ? WHERE id = ?`,
		ip, ua, now.Format(time.RFC3339), now.Add(ttl).Format(time.RFC3339), id)
	return err
}

// TrustedDevices lists the user's remembered browsers, most recently used first.
func (s *Store) TrustedDevices(ctx context.Context, userID int64) ([]TrustedDevice, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, ip, user_agent, created_at, last_used_at FROM trusted_devices
		 WHERE user_id = ? AND expires_at > ? ORDER BY last_used_at DESC`,
		userID, time.Now().UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TrustedDevice{}
	for rows.Next() {
		var d TrustedDevice
		if err := rows.Scan(&d.ID, &d.UserID, &d.IP, &d.UserAgent, &d.CreatedAt, &d.LastUsedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

// DeleteTrustedDevice forgets one of the user's browsers.
func (s *Store) DeleteTrustedDevice(ctx context.Context, userID, id int64) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM trusted_devices WHERE id = ? AND user_id = ?`, id, userID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteTrustedDevices forgets every browser of the user.
func (s *Store) DeleteTrustedDevices(ctx context.Context, userID int64) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM trusted_devices WHERE user_id = ?`, userID)
	return err
}
