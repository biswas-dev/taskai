-- Add agent_name to wiki pages and versions for AI agent attribution
-- NULL means human-edited, non-NULL means agent-edited (e.g. "Claude Code")

ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS agent_name VARCHAR(100);
ALTER TABLE wiki_page_versions ADD COLUMN IF NOT EXISTS agent_name VARCHAR(100);
