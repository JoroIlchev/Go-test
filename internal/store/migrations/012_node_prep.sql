-- How new uploads are prepared, per node, inherited by everything below;
-- JSON of media.Prep. Nodes without one use the hub settings.
CREATE TABLE node_prep (
    node_id    INTEGER PRIMARY KEY REFERENCES nodes(id) ON DELETE CASCADE,
    prep       TEXT NOT NULL,
    updated_by INTEGER REFERENCES users(id),
    updated_at TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%SZ', 'now'))
);
