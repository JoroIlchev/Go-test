-- Browsers a user chose to remember at login. A remembered browser is not
-- locked out when someone else keeps guessing the user's password.
CREATE TABLE trusted_devices (
    id           INTEGER PRIMARY KEY,
    token_hash   TEXT NOT NULL UNIQUE,
    user_id      INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    ip           TEXT NOT NULL DEFAULT '',
    user_agent   TEXT NOT NULL DEFAULT '',
    created_at   TEXT NOT NULL,
    last_used_at TEXT NOT NULL,
    expires_at   TEXT NOT NULL
);
CREATE INDEX idx_trusted_devices_user ON trusted_devices(user_id);
