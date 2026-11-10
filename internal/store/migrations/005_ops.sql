-- Polling policy per node, inherited by everything below; JSON of poll.Policy.
CREATE TABLE node_poll (
    node_id    INTEGER PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    policy     TEXT NOT NULL,
    updated_by INTEGER REFERENCES users(id),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);

-- When the device said it would come back (from the Refresh it was given),
-- and whether we already raised an "offline" alert for it.
ALTER TABLE devices ADD COLUMN next_check_at TEXT;
ALTER TABLE devices ADD COLUMN offline_since TEXT;

-- Hub-wide settings (SMTP, alerts, backups) as key/value.
CREATE TABLE settings (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);
