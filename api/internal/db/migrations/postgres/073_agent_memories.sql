-- Migration 073: Agent memories, owned by a user (not a project).
-- A memory is a short lesson an agent or person keeps: repo/issue/pr/folder/files are filterable scope.
-- origin_host + origin_id make upserts from several llm-brain hubs idempotent. status 'compacted' is a
-- tombstone left by compaction so a hub re-sending the same origin does not bring it back.

CREATE TABLE IF NOT EXISTS agent_memories (
    id                   BIGSERIAL PRIMARY KEY,
    owner_id             BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind                 TEXT NOT NULL CHECK (kind IN ('fact', 'decision', 'gotcha', 'runbook', 'handoff')),
    title                TEXT NOT NULL,
    body                 TEXT NOT NULL,
    repo                 TEXT NOT NULL DEFAULT '',
    issue                INTEGER NOT NULL DEFAULT 0,
    pr                   INTEGER NOT NULL DEFAULT 0,
    folder               TEXT NOT NULL DEFAULT '',
    files                TEXT NOT NULL DEFAULT '', -- newline-wrapped list: "\na.go\nb.go\n"
    agent                TEXT NOT NULL DEFAULT '',
    provider             TEXT NOT NULL DEFAULT '',
    host                 TEXT NOT NULL DEFAULT '',
    session              TEXT NOT NULL DEFAULT '',
    confidence           TEXT NOT NULL DEFAULT 'medium' CHECK (confidence IN ('low', 'medium', 'high')),
    supersedes           BIGINT,
    supersedes_origin_id TEXT NOT NULL DEFAULT '',
    superseded_by        BIGINT,
    status               TEXT NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'superseded', 'compacted')),
    expires_at           TIMESTAMPTZ,
    content_hash         TEXT NOT NULL,
    origin_host          TEXT NOT NULL DEFAULT '',
    origin_id            TEXT NOT NULL DEFAULT '',
    size_bytes           BIGINT NOT NULL DEFAULT 0,
    created_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at           TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    search_vector        tsvector GENERATED ALWAYS AS (
        setweight(to_tsvector('english', coalesce(title, '')), 'A') ||
        setweight(to_tsvector('english', coalesce(body, '')), 'B') ||
        setweight(to_tsvector('simple', coalesce(repo, '') || ' ' || coalesce(folder, '') || ' ' || coalesce(files, '')), 'C')
    ) STORED,
    embedding            vector(384),
    embedded_at          TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_agent_memories_origin ON agent_memories(owner_id, origin_host, origin_id) WHERE origin_id <> '';
CREATE INDEX IF NOT EXISTS idx_agent_memories_owner_status ON agent_memories(owner_id, status, repo);
CREATE INDEX IF NOT EXISTS idx_agent_memories_owner_hash ON agent_memories(owner_id, content_hash);
CREATE INDEX IF NOT EXISTS idx_agent_memories_search ON agent_memories USING GIN(search_vector);
CREATE INDEX IF NOT EXISTS idx_agent_memories_embedding ON agent_memories USING hnsw (embedding vector_cosine_ops) WITH (m = 16, ef_construction = 64);

-- Whose memories a user can read: the owner shares their whole store with someone.
CREATE TABLE IF NOT EXISTS memory_shares (
    owner_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    granted_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (owner_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_memory_shares_user ON memory_shares(user_id);

-- Per-user quota override set by an admin; no row means the default (1 GiB).
CREATE TABLE IF NOT EXISTS memory_quotas (
    user_id     BIGINT PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    quota_bytes BIGINT NOT NULL CHECK (quota_bytes > 0),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
