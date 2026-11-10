-- A campaign belongs to a node and may target that node or anything below it.
-- draft is the editable working copy (JSON spec); publishing freezes it into
-- an immutable campaign_versions row. live_version_id is what players get.
CREATE TABLE campaigns (
    id               INTEGER PRIMARY KEY,
    node_id          INTEGER NOT NULL REFERENCES nodes(id) ON DELETE RESTRICT,
    client_id        INTEGER NOT NULL,
    name             TEXT NOT NULL,
    draft            TEXT NOT NULL DEFAULT '{}',
    draft_updated_at TEXT,
    draft_updated_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    live_version_id  INTEGER REFERENCES campaign_versions(id) ON DELETE SET NULL,
    created_by       INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at       TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
CREATE INDEX idx_campaigns_node ON campaigns(node_id);
CREATE INDEX idx_campaigns_live ON campaigns(live_version_id);

-- status: pending (waiting for an admin) | approved | rejected
CREATE TABLE campaign_versions (
    id             INTEGER PRIMARY KEY,
    campaign_id    INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    number         INTEGER NOT NULL,
    spec           TEXT NOT NULL,
    status         TEXT NOT NULL CHECK (status IN ('pending', 'approved', 'rejected')),
    created_by     INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at     TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now')),
    reviewed_by    INTEGER REFERENCES users(id) ON DELETE SET NULL,
    reviewed_at    TEXT,
    review_comment TEXT NOT NULL DEFAULT '',
    UNIQUE (campaign_id, number)
);

-- Which media a draft (version_id NULL) or version uses, so media in use
-- cannot be deleted from under a campaign.
CREATE TABLE campaign_media (
    campaign_id INTEGER NOT NULL REFERENCES campaigns(id) ON DELETE CASCADE,
    version_id  INTEGER REFERENCES campaign_versions(id) ON DELETE CASCADE,
    media_id    INTEGER NOT NULL
);
CREATE INDEX idx_campaign_media_media ON campaign_media(media_id);
CREATE INDEX idx_campaign_media_campaign ON campaign_media(campaign_id);

-- What each device last received, for "ready for campaign X" status.
ALTER TABLE devices ADD COLUMN served_versions TEXT NOT NULL DEFAULT '';
