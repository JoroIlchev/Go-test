package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// NodePolls returns the polling policies (raw JSON) set on the given nodes.
func (s *Store) NodePolls(ctx context.Context, ids []int64) (map[int64]string, error) {
	out := map[int64]string{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	rows, err := s.db.QueryContext(ctx,
		`SELECT node_id, policy FROM node_poll WHERE node_id IN (`+placeholders(len(ids))+`)`, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var p string
		if err := rows.Scan(&id, &p); err != nil {
			return nil, err
		}
		out[id] = p
	}
	return out, rows.Err()
}

// SetNodePoll sets the node's policy, or removes it (inherit) when policy is "".
func (s *Store) SetNodePoll(ctx context.Context, nodeID int64, policy string, by int64) error {
	if policy == "" {
		_, err := s.db.ExecContext(ctx, `DELETE FROM node_poll WHERE node_id = ?`, nodeID)
		return err
	}
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO node_poll (node_id, policy, updated_by, updated_at) VALUES (?, ?, ?, ?)
		 ON CONFLICT(node_id) DO UPDATE SET policy = excluded.policy, updated_by = excluded.updated_by,
		 updated_at = excluded.updated_at`, nodeID, policy, by, now())
	return err
}

func (s *Store) Setting(ctx context.Context, key string) (string, error) {
	var v string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO settings (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value)
	return err
}

// HistoryEvent is one change of a device's status or placement.
type HistoryEvent struct {
	DeviceID int64
	ClientID sql.NullInt64
	Status   string
	At       time.Time
}

// DeviceHistory returns every event up to `to`, oldest first: a device's
// state at the start of a period depends on events before it.
func (s *Store) DeviceHistory(ctx context.Context, to time.Time) ([]HistoryEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT device_id, client_id, status, at FROM device_history WHERE at < ? ORDER BY at, id`,
		to.UTC().Format(time.RFC3339))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []HistoryEvent
	for rows.Next() {
		var e HistoryEvent
		var at string
		if err := rows.Scan(&e.DeviceID, &e.ClientID, &e.Status, &at); err != nil {
			return nil, err
		}
		if e.At, err = time.Parse(time.RFC3339, at); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

// BackupTo writes a consistent copy of the database to path (which must not
// exist), without stopping the hub.
func (s *Store) BackupTo(ctx context.Context, path string) error {
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, path)
	return err
}
