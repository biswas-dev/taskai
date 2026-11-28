-- Migration 076: opaque per-user sync objects (Brain Hub encrypted sync). See postgres/076 for the notes.
CREATE TABLE IF NOT EXISTS sync_objects (
    owner_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    key        TEXT NOT NULL,
    body       BLOB NOT NULL,
    size       INTEGER NOT NULL,
    etag       TEXT NOT NULL,
    updated_at DATETIME NOT NULL,
    PRIMARY KEY (owner_id, key)
);
