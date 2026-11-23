-- Trigram index so keyword (partial-word) memory search stays fast; hybrid search merges it with full text and vectors.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX IF NOT EXISTS idx_agent_memories_trgm ON agent_memories USING GIN ((LOWER(title || ' ' || body)) gin_trgm_ops);
