-- Media items belong to a node and are visible to that node and everything
-- below it. The bytes live once per content hash under data/media, so the same
-- file uploaded twice (or used by many clients) is stored and downloaded once.
-- sha256 is '' while a conversion is still producing the file.
CREATE TABLE media (
    id          INTEGER PRIMARY KEY,
    node_id     INTEGER NOT NULL REFERENCES nodes(id) ON DELETE RESTRICT,
    client_id   INTEGER NOT NULL,
    name        TEXT NOT NULL,
    kind        TEXT NOT NULL CHECK (kind IN ('image', 'video', 'audio')),
    sha256      TEXT NOT NULL DEFAULT '',
    ext         TEXT NOT NULL,
    size        INTEGER NOT NULL DEFAULT 0,
    width       INTEGER NOT NULL DEFAULT 0,
    height      INTEGER NOT NULL DEFAULT 0,
    duration_ms INTEGER NOT NULL DEFAULT 0,
    video_codec TEXT NOT NULL DEFAULT '',
    audio_codec TEXT NOT NULL DEFAULT '',
    fps         REAL NOT NULL DEFAULT 0,
    bitrate     INTEGER NOT NULL DEFAULT 0,
    status      TEXT NOT NULL DEFAULT 'processing' CHECK (status IN ('processing', 'ready', 'failed')),
    -- JSON array of {"level":"warn|error","code":"...","message":"..."}
    warnings    TEXT NOT NULL DEFAULT '[]',
    error       TEXT NOT NULL DEFAULT '',
    has_thumb   INTEGER NOT NULL DEFAULT 0,
    -- set on a copy produced by "convert for Android"
    source_id   INTEGER REFERENCES media(id) ON DELETE SET NULL,
    uploaded_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at  TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX idx_media_node ON media(node_id);
CREATE INDEX idx_media_client ON media(client_id);
CREATE INDEX idx_media_sha ON media(sha256);
CREATE INDEX idx_media_status ON media(status);
