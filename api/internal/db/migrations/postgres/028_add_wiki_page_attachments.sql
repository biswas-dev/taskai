-- Wiki page attachments (mirrors task_attachments for wiki pages)
CREATE TABLE IF NOT EXISTS wiki_page_attachments (
    id BIGSERIAL PRIMARY KEY,
    wiki_page_id BIGINT NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
    project_id BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id),
    filename TEXT NOT NULL,
    alt_name TEXT NOT NULL DEFAULT '',
    file_type TEXT NOT NULL DEFAULT 'file',
    content_type TEXT NOT NULL DEFAULT '',
    file_size INTEGER NOT NULL DEFAULT 0,
    cloudinary_url TEXT NOT NULL,
    cloudinary_public_id TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_wiki_page_attachments_page_id ON wiki_page_attachments(wiki_page_id);
CREATE INDEX IF NOT EXISTS idx_wiki_page_attachments_project_id ON wiki_page_attachments(project_id);
