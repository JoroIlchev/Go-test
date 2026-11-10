package store

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"time"
)

type User struct {
	ID        int64         `json:"id"`
	Email     string        `json:"email"`
	Name      string        `json:"name"`
	PassHash  string        `json:"-"`
	IsSuper   bool          `json:"is_super"`
	ClientID  sql.NullInt64 `json:"-"`
	ClientRef *int64        `json:"client_id"`
	Active    bool          `json:"active"`
	CreatedAt string        `json:"created_at"`
	Roles     []Role        `json:"roles"`
}

type Role struct {
	NodeID        int64  `json:"node_id"`
	Role          string `json:"role"`
	CanCreate     bool   `json:"can_create"`
	CanEdit       bool   `json:"can_edit"`
	CanPublish    bool   `json:"can_publish"`
	NeedsApproval bool   `json:"needs_approval"`
}

const userCols = `id, email, name, pass_hash, is_super, client_id, active, created_at`

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	var u User
	err := row.Scan(&u.ID, &u.Email, &u.Name, &u.PassHash, &u.IsSuper, &u.ClientID, &u.Active, &u.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if u.ClientID.Valid {
		u.ClientRef = &u.ClientID.Int64
	}
	return &u, err
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx,
		`SELECT `+userCols+` FROM users WHERE email = ?`, strings.TrimSpace(email)))
	if err != nil {
		return nil, err
	}
	return u, s.loadRoles(ctx, u)
}

func (s *Store) User(ctx context.Context, id int64) (*User, error) {
	u, err := scanUser(s.db.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	return u, s.loadRoles(ctx, u)
}

// UsersOfClient returns the client's users (never super admins).
func (s *Store) UsersOfClient(ctx context.Context, clientID int64) ([]*User, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+userCols+` FROM users WHERE client_id = ? ORDER BY name, email`, clientID)
	if err != nil {
		return nil, err
	}
	var out []*User
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, u := range out {
		if err := s.loadRoles(ctx, u); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) loadRoles(ctx context.Context, u *User) error {
	rows, err := s.db.QueryContext(ctx,
		`SELECT node_id, role, can_create, can_edit, can_publish, needs_approval
		 FROM user_roles WHERE user_id = ? ORDER BY node_id`, u.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	u.Roles = []Role{}
	for rows.Next() {
		var r Role
		if err := rows.Scan(&r.NodeID, &r.Role, &r.CanCreate, &r.CanEdit, &r.CanPublish, &r.NeedsApproval); err != nil {
			return err
		}
		u.Roles = append(u.Roles, r)
	}
	return rows.Err()
}

// CreateUser inserts a user with its roles in one transaction.
// clientID is nil for a super admin.
func (s *Store) CreateUser(ctx context.Context, email, name, passHash string, isSuper bool,
	clientID *int64, createdBy *int64, roles []Role) (*User, error) {
	email = strings.TrimSpace(email)
	if email == "" || !strings.Contains(email, "@") {
		return nil, ErrInvalidArg
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	res, err := tx.ExecContext(ctx,
		`INSERT INTO users (email, name, pass_hash, is_super, client_id, created_by) VALUES (?, ?, ?, ?, ?, ?)`,
		email, strings.TrimSpace(name), passHash, isSuper, clientID, createdBy)
	if err != nil {
		if strings.Contains(err.Error(), "UNIQUE") {
			return nil, ErrDuplicate
		}
		return nil, err
	}
	id, _ := res.LastInsertId()
	if err := replaceRoles(ctx, tx, id, roles); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.User(ctx, id)
}

type UserUpdate struct {
	Name     *string
	PassHash *string
	Active   *bool
	Roles    []Role // nil = leave roles unchanged
}

func (s *Store) UpdateUser(ctx context.Context, id int64, upd UserUpdate) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if upd.Name != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET name = ? WHERE id = ?`, strings.TrimSpace(*upd.Name), id); err != nil {
			return err
		}
	}
	if upd.PassHash != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET pass_hash = ? WHERE id = ?`, *upd.PassHash, id); err != nil {
			return err
		}
	}
	if upd.Active != nil {
		if _, err := tx.ExecContext(ctx, `UPDATE users SET active = ? WHERE id = ?`, *upd.Active, id); err != nil {
			return err
		}
	}
	if upd.Roles != nil {
		if err := replaceRoles(ctx, tx, id, upd.Roles); err != nil {
			return err
		}
	}
	// A password change or deactivation ends every open session of the user.
	if upd.PassHash != nil || (upd.Active != nil && !*upd.Active) {
		if _, err := tx.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, id); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func replaceRoles(ctx context.Context, tx *sql.Tx, userID int64, roles []Role) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM user_roles WHERE user_id = ?`, userID); err != nil {
		return err
	}
	for _, r := range roles {
		if r.Role != "admin" && r.Role != "editor" {
			return ErrInvalidArg
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO user_roles (user_id, node_id, role, can_create, can_edit, can_publish, needs_approval)
			 VALUES (?, ?, ?, ?, ?, ?, ?)`,
			userID, r.NodeID, r.Role, r.CanCreate, r.CanEdit, r.CanPublish, r.NeedsApproval); err != nil {
			if strings.Contains(err.Error(), "PRIMARY KEY") || strings.Contains(err.Error(), "UNIQUE") {
				return ErrDuplicate
			}
			return err
		}
	}
	return nil
}

// Sessions

func (s *Store) CreateSession(ctx context.Context, tokenHash string, userID int64, ip, ua string, ttl time.Duration) error {
	now := time.Now().UTC()
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO sessions (token_hash, user_id, ip, user_agent, created_at, expires_at) VALUES (?, ?, ?, ?, ?, ?)`,
		tokenHash, userID, ip, ua, now.Format(time.RFC3339), now.Add(ttl).Format(time.RFC3339))
	return err
}

// SessionUser returns the active user behind a non-expired session.
func (s *Store) SessionUser(ctx context.Context, tokenHash string) (*User, error) {
	var userID int64
	err := s.db.QueryRowContext(ctx,
		`SELECT s.user_id FROM sessions s JOIN users u ON u.id = s.user_id
		 WHERE s.token_hash = ? AND s.expires_at > ? AND u.active = 1`,
		tokenHash, time.Now().UTC().Format(time.RFC3339)).Scan(&userID)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	return s.User(ctx, userID)
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *Store) Audit(ctx context.Context, userID int64, action string, nodeID int64, details string) {
	var node any
	if nodeID != 0 {
		node = nodeID
	}
	s.db.ExecContext(ctx,
		`INSERT INTO audit_log (user_id, action, node_id, details) VALUES (?, ?, ?, ?)`,
		userID, action, node, details)
}
