-- Organisation tree. A root node (parent_id NULL, kind 'client') is a customer;
-- below it any depth of 'group' nodes (divisions, regions) and 'site' nodes
-- (physical locations that own devices).
-- path is the materialised ancestor chain, e.g. '/1/5/9/', so a subtree is
-- "path LIKE '/1/5/%'".
CREATE TABLE nodes (
    id         INTEGER PRIMARY KEY,
    parent_id  INTEGER REFERENCES nodes(id) ON DELETE RESTRICT,
    client_id  INTEGER NOT NULL,
    kind       TEXT NOT NULL CHECK (kind IN ('client', 'group', 'site')),
    name       TEXT NOT NULL,
    path       TEXT NOT NULL DEFAULT '',
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX idx_nodes_parent ON nodes(parent_id);
CREATE INDEX idx_nodes_path ON nodes(path);
CREATE INDEX idx_nodes_client ON nodes(client_id);

-- status: pending (self-registered, waiting for pairing) | active | blocked
-- auth_mode: link (secret path only) | basic (secret path + user/password)
CREATE TABLE devices (
    id             INTEGER PRIMARY KEY,
    node_id        INTEGER REFERENCES nodes(id) ON DELETE RESTRICT,
    uuid           TEXT UNIQUE,
    name           TEXT NOT NULL DEFAULT '',
    token          TEXT NOT NULL UNIQUE,
    pairing_code   TEXT NOT NULL UNIQUE,
    status         TEXT NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'active', 'blocked')),
    auth_mode      TEXT NOT NULL DEFAULT 'link' CHECK (auth_mode IN ('link', 'basic')),
    auth_user      TEXT NOT NULL DEFAULT '',
    auth_pass_hash TEXT NOT NULL DEFAULT '',
    user_agent     TEXT NOT NULL DEFAULT '',
    last_ip        TEXT NOT NULL DEFAULT '',
    last_seen_at   TEXT,
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX idx_devices_node ON devices(node_id);
CREATE INDEX idx_devices_status ON devices(status);

-- client_id is NULL only for super admins (the hub operator).
CREATE TABLE users (
    id         INTEGER PRIMARY KEY,
    email      TEXT NOT NULL UNIQUE COLLATE NOCASE,
    name       TEXT NOT NULL DEFAULT '',
    pass_hash  TEXT NOT NULL,
    is_super   INTEGER NOT NULL DEFAULT 0,
    client_id  INTEGER REFERENCES nodes(id) ON DELETE CASCADE,
    active     INTEGER NOT NULL DEFAULT 1,
    created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- A role grants rights on a node and everything below it.
-- admin: everything incl. managing users and approving; editor: the listed flags.
-- needs_approval: this user's publications wait for an admin at or above node_id.
CREATE TABLE user_roles (
    user_id        INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    node_id        INTEGER NOT NULL REFERENCES nodes(id) ON DELETE CASCADE,
    role           TEXT NOT NULL CHECK (role IN ('admin', 'editor')),
    can_create     INTEGER NOT NULL DEFAULT 0,
    can_edit       INTEGER NOT NULL DEFAULT 0,
    can_publish    INTEGER NOT NULL DEFAULT 0,
    needs_approval INTEGER NOT NULL DEFAULT 0,
    PRIMARY KEY (user_id, node_id)
);
CREATE INDEX idx_user_roles_node ON user_roles(node_id);

-- token_hash is sha256 of the cookie value; the raw token never touches disk.
CREATE TABLE sessions (
    token_hash   TEXT PRIMARY KEY,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ip           TEXT NOT NULL DEFAULT '',
    user_agent   TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    expires_at   TEXT NOT NULL
);
CREATE INDEX idx_sessions_user ON sessions(user_id);

CREATE TABLE audit_log (
    id      INTEGER PRIMARY KEY,
    at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    user_id INTEGER REFERENCES users(id) ON DELETE SET NULL,
    action  TEXT NOT NULL,
    node_id INTEGER,
    details TEXT NOT NULL DEFAULT ''
);
CREATE INDEX idx_audit_at ON audit_log(at);

-- Every status/placement change of a device; billing counts device-days from this.
CREATE TABLE device_history (
    id        INTEGER PRIMARY KEY,
    device_id INTEGER NOT NULL REFERENCES devices(id) ON DELETE CASCADE,
    node_id   INTEGER,
    client_id INTEGER,
    status    TEXT NOT NULL,
    at        TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX idx_device_history_client ON device_history(client_id, at);
