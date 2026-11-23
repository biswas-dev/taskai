-- Migration 073: Agent memories, owned by a user (not a project). See postgres/073 for the full notes;
-- SQLite has no tsvector or pgvector, so search here is keyword only.

CREATE TABLE IF NOT EXISTS agent_memories (
    id                   INTEGER PRIMARY KEY AUTOINCREMENT,
    owner_id             INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind                 TEXT NOT NULL CHECK (kind IN ('fact', 'decision', 'gotcha', 'runbook', 'handoff')),
    title                TEXT NOT NULL,
    body                 TEXT NOT NULL,
    repo                 TEXT NOT NULL DEFAULT '',
    issue                INTEGER NOT NULL DEFAULT 0,
    pr                   INTEGER NOT NULL DEFAULT 0,
    folder               TEXT NOT NULL DEFAULT '',
    files                TEXT NOT NULL DEFAULT '',
    agent                TEXT NOT NULL DEFAULT '',
    provider             TEXT NOT NULL DEFAULT '',
    host                 TEXT NOT NULL DEFAULT '',
    session              TEXT NOT NULL DEFAULT '',
    confidence           TEXT NOT NULL DEFAULT 'medium' CHECK (confidence IN ('low', 'medium', 'high')),
    supersedes           INTEGER,
    supersedes_origin_id TEXT NOT NULL DEFAULT '',
    superseded_by        INTEGER,
    status               TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'superseded', 'compacted')),
    expires_at           DATETIME,
    content_hash         TEXT NOT NULL,
    origin_host          TEXT NOT NULL DEFAULT '',
    origin_id            TEXT NOT NULL DEFAULT '',
    size_bytes           INTEGER NOT NULL DEFAULT 0,
    created_at           DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at           DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_memories_origin ON agent_memories(owner_id, origin_host, origin_id) WHERE origin_id <> '';
CREATE INDEX IF NOT EXISTS idx_agent_memories_owner_status ON agent_memories(owner_id, status, repo);
CREATE INDEX IF NOT EXISTS idx_agent_memories_owner_hash ON agent_memories(owner_id, content_hash);

CREATE TABLE IF NOT EXISTS memory_shares (
    owner_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    granted_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    PRIMARY KEY (owner_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_memory_shares_user ON memory_shares(user_id);

CREATE TABLE IF NOT EXISTS memory_quotas (
    user_id     INTEGER PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    quota_bytes INTEGER NOT NULL CHECK (quota_bytes > 0),
    updated_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
