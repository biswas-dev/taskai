-- Per-page wiki visibility.
--   project    : every project member can see the page (default, existing behaviour)
--   restricted : only the page creator, the project owner and users in wiki_page_shares
-- public_token, when set, lets anyone with the link read the page (rarely used).
ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS visibility TEXT NOT NULL DEFAULT 'project';
ALTER TABLE wiki_pages DROP CONSTRAINT IF EXISTS wiki_pages_visibility_check;
ALTER TABLE wiki_pages ADD CONSTRAINT wiki_pages_visibility_check CHECK (visibility IN ('project', 'restricted'));
ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS public_token TEXT;

CREATE UNIQUE INDEX IF NOT EXISTS idx_wiki_pages_public_token ON wiki_pages(public_token) WHERE public_token IS NOT NULL;

CREATE TABLE IF NOT EXISTS wiki_page_shares (
    page_id BIGINT NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
    user_id BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    granted_by BIGINT REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (page_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_wiki_page_shares_user ON wiki_page_shares(user_id);
