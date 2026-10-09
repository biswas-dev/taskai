-- When a page's content was last turned into search blocks. The indexing
-- worker re-indexes any page whose updated_at is newer, so edits made while
-- the server was down or deploying are still picked up. NULL means never
-- indexed: every existing page is indexed once after this migration, which
-- also restores blocks lost while indexing read the (now compressed)
-- version table.
ALTER TABLE wiki_pages ADD COLUMN IF NOT EXISTS search_indexed_at TIMESTAMPTZ;
CREATE INDEX IF NOT EXISTS idx_wiki_pages_search_indexed ON wiki_pages(search_indexed_at);
