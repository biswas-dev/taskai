-- Migration 075: memory strength and confidence. See postgres/075 for the notes.
ALTER TABLE agent_memories ADD COLUMN use_count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_memories ADD COLUMN last_used_at DATETIME;
ALTER TABLE agent_memories ADD COLUMN confirmed_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_memories ADD COLUMN wrong_count INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS memory_feedback (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    memory_id  INTEGER NOT NULL REFERENCES agent_memories(id) ON DELETE CASCADE,
    owner_id   INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    signal     TEXT NOT NULL CHECK (signal IN ('useful', 'confirmed', 'wrong')),
    actor      TEXT NOT NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_feedback_vote ON memory_feedback(memory_id, signal, actor) WHERE signal <> 'useful';
CREATE INDEX IF NOT EXISTS idx_memory_feedback_memory ON memory_feedback(memory_id, created_at);
