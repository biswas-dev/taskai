-- Migration 076: opaque per-user sync objects for Brain Hub encrypted sync (PUT/GET/DELETE /api/sync/objects/{key}).
-- The server stores ciphertext bytes only and never parses them. etag is the SHA-256 hex of body.
-- key uses COLLATE "C" so ORDER BY key and the after= cursor follow byte order, the same as SQLite.
CREATE TABLE IF NOT EXISTS sync_objects (
    owner_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key        TEXT COLLATE "C" NOT NULL,
    body       BYTEA NOT NULL,
    size       BIGINT NOT NULL,
    etag       TEXT NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (owner_id, key)
);
