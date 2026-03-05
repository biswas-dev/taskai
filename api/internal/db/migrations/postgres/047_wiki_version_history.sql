-- Add updated_by to wiki_pages
ALTER TABLE wiki_pages ADD COLUMN updated_by INTEGER REFERENCES users(id);

-- Create wiki_page_versions table
CREATE TABLE IF NOT EXISTS wiki_page_versions (
    id BIGSERIAL PRIMARY KEY,
    wiki_page_id BIGINT NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
    version_number INTEGER NOT NULL,
    content TEXT NOT NULL DEFAULT '',
    content_hash TEXT NOT NULL DEFAULT '',
    created_by BIGINT NOT NULL REFERENCES users(id),
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(wiki_page_id, version_number)
);
CREATE INDEX IF NOT EXISTS idx_wiki_page_versions_page_id ON wiki_page_versions(wiki_page_id);
