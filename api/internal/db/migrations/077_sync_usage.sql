-- Migration 077: per-user sync usage counter. See postgres/077 for the notes.
CREATE TABLE IF NOT EXISTS sync_usage (
    owner_id INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    bytes    INTEGER NOT NULL DEFAULT 0
);
INSERT INTO sync_usage (owner_id, bytes)
SELECT owner_id, SUM(size) + COUNT(*) * 256 FROM sync_objects GROUP BY owner_id
ON CONFLICT (owner_id) DO NOTHING;
