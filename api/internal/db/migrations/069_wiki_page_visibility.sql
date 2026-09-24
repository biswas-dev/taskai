-- Per-page wiki visibility.
--   project    : every project member can see the page (default, existing behaviour)
--   restricted : only the page creator, the project owner and users in wiki_page_shares
-- public_token, when set, lets anyone with the link read the page (rarely used).
ALTER TABLE wiki_pages ADD COLUMN visibility TEXT NOT NULL DEFAULT 'project' CHECK (visibility IN ('project', 'restricted'));
ALTER TABLE wiki_pages ADD COLUMN public_token TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_wiki_pages_public_token ON wiki_pages(public_token) WHERE public_token IS NOT NULL;

CREATE TABLE IF NOT EXISTS wiki_page_shares (
    page_id INTEGER NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    granted_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    PRIMARY KEY (page_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_wiki_page_shares_user ON wiki_page_shares(user_id);
