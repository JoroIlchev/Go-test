package store

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"strings"
)

type Node struct {
	ID        int64         `json:"id"`
	ParentID  sql.NullInt64 `json:"-"`
	ClientID  int64         `json:"client_id"`
	Kind      string        `json:"kind"`
	Name      string        `json:"name"`
	Path      string        `json:"-"`
	CreatedAt string        `json:"created_at"`
}

var (
	ErrNotEmpty   = errors.New("node is not empty")
	ErrBadParent  = errors.New("invalid parent")
	ErrDuplicate  = errors.New("already exists")
	ErrInvalidArg = errors.New("invalid argument")
)

// AncestorIDs returns the node's ancestor chain from the root down to the node itself.
func (n *Node) AncestorIDs() []int64 {
	var ids []int64
	for _, part := range strings.Split(strings.Trim(n.Path, "/"), "/") {
		if id, err := strconv.ParseInt(part, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// IsUnder reports whether n is anc or one of its descendants.
func (n *Node) IsUnder(anc *Node) bool { return strings.HasPrefix(n.Path, anc.Path) }

const nodeCols = `id, parent_id, client_id, kind, name, path, created_at`

func scanNode(row interface{ Scan(...any) error }) (*Node, error) {
	var n Node
	err := row.Scan(&n.ID, &n.ParentID, &n.ClientID, &n.Kind, &n.Name, &n.Path, &n.CreatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return &n, err
}

func (s *Store) Node(ctx context.Context, id int64) (*Node, error) {
	return scanNode(s.db.QueryRowContext(ctx, `SELECT `+nodeCols+` FROM nodes WHERE id = ?`, id))
}

// Subtree returns root and all its descendants, parents before children.
func (s *Store) Subtree(ctx context.Context, root *Node) ([]*Node, error) {
	return s.queryNodes(ctx,
		`SELECT `+nodeCols+` FROM nodes WHERE path LIKE ? ORDER BY length(path), name`, root.Path+"%")
}

func (s *Store) Clients(ctx context.Context) ([]*Node, error) {
	return s.queryNodes(ctx,
		`SELECT `+nodeCols+` FROM nodes WHERE parent_id IS NULL ORDER BY name`)
}

func (s *Store) queryNodes(ctx context.Context, q string, args ...any) ([]*Node, error) {
	rows, err := s.db.QueryContext(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []*Node
	for rows.Next() {
		n, err := scanNode(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// CreateNode adds a client (parent nil, kind "client") or a child node.
func (s *Store) CreateNode(ctx context.Context, parent *Node, kind, name string) (*Node, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return nil, ErrInvalidArg
	}
	switch {
	case parent == nil && kind != "client":
		return nil, ErrBadParent
	case parent != nil && kind != "group" && kind != "site":
		return nil, ErrBadParent
	case parent != nil && parent.Kind == "site":
		// Sites are leaves: devices hang off them, not further nodes.
		return nil, ErrBadParent
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	var parentID any
	var clientID int64
	if parent != nil {
		parentID, clientID = parent.ID, parent.ClientID
	}
	res, err := tx.ExecContext(ctx,
		`INSERT INTO nodes (parent_id, client_id, kind, name) VALUES (?, ?, ?, ?)`,
		parentID, clientID, kind, name)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	path := "/" + strconv.FormatInt(id, 10) + "/"
	if parent != nil {
		path = parent.Path + strconv.FormatInt(id, 10) + "/"
	} else {
		clientID = id
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE nodes SET path = ?, client_id = ? WHERE id = ?`, path, clientID, id); err != nil {
		return nil, err
	}
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return s.Node(ctx, id)
}

func (s *Store) RenameNode(ctx context.Context, id int64, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return ErrInvalidArg
	}
	_, err := s.db.ExecContext(ctx, `UPDATE nodes SET name = ? WHERE id = ?`, name, id)
	return err
}

// DeleteNode removes an empty node. Anything with children or devices is
// refused so a mis-click cannot wipe a customer's setup.
func (s *Store) DeleteNode(ctx context.Context, id int64) error {
	var children, devices int
	if err := s.db.QueryRowContext(ctx,
		`SELECT (SELECT COUNT(*) FROM nodes WHERE parent_id = ?),
		        (SELECT COUNT(*) FROM devices WHERE node_id = ?)`, id, id).Scan(&children, &devices); err != nil {
		return err
	}
	if children > 0 || devices > 0 {
		return ErrNotEmpty
	}
	_, err := s.db.ExecContext(ctx, `DELETE FROM nodes WHERE id = ?`, id)
	return err
}
