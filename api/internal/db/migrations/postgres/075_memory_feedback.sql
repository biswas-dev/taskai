-- Migration 075: memory strength and confidence (docs/research/memory-strength.md), behind MEMORY_STRENGTH.
-- use_count counts the write as the first use; last_used_at is the latest "useful" or "confirmed" signal.
-- confirmed_count and wrong_count are evidence votes, one per signal and credential (memory_feedback keeps the
-- votes only; "useful" is counted, not logged).
-- An upsert by origin never touches these columns, so evidence survives a hub re-sending the memory.
ALTER TABLE agent_memories ADD COLUMN IF NOT EXISTS use_count INTEGER NOT NULL DEFAULT 1;
ALTER TABLE agent_memories ADD COLUMN IF NOT EXISTS last_used_at TIMESTAMPTZ;
ALTER TABLE agent_memories ADD COLUMN IF NOT EXISTS confirmed_count INTEGER NOT NULL DEFAULT 0;
ALTER TABLE agent_memories ADD COLUMN IF NOT EXISTS wrong_count INTEGER NOT NULL DEFAULT 0;

CREATE TABLE IF NOT EXISTS memory_feedback (
    id         BIGSERIAL PRIMARY KEY,
    memory_id  BIGINT NOT NULL REFERENCES agent_memories(id) ON DELETE CASCADE,
    owner_id   BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    signal     TEXT NOT NULL CHECK (signal IN ('confirmed', 'wrong')),
    actor      TEXT NOT NULL, -- "key:<api key id>" or "session"
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- One evidence vote per memory, signal and credential. "useful" is only a counter (use_count, last_used_at).
CREATE UNIQUE INDEX IF NOT EXISTS idx_memory_feedback_vote ON memory_feedback(memory_id, signal, actor);
