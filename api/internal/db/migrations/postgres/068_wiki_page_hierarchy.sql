-- Nested wiki pages: each page may have a parent within the same project.
-- Depth is enforced in the API layer (max 6 levels).
ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS parent_id BIGINT REFERENCES wiki_pages(id) ON DELETE SET NULL;
ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS position INTEGER NOT NULL DEFAULT 0;

CREATE INDEX IF NOT EXISTS idx_wiki_pages_project_parent ON wiki_pages(project_id, parent_id, position);
