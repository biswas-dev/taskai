-- Migration 077: per-user sync usage counter, so the quota check on PUT is O(1) and does not sum every object.
-- bytes = SUM(size) + 256 per object (syncRowOverhead in internal/api/sync_objects.go). Writes update it in the same
-- transaction as the object, under the per-owner advisory lock.
CREATE TABLE IF NOT EXISTS sync_usage (
    owner_id BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    bytes    BIGINT NOT NULL DEFAULT 0
);
INSERT INTO sync_usage (owner_id, bytes)
SELECT owner_id, SUM(size) + COUNT(*) * 256 FROM sync_objects GROUP BY owner_id
ON CONFLICT (owner_id) DO NOTHING;
