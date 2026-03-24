-- Task watchers: users who want to be notified of all changes on a task
CREATE TABLE IF NOT EXISTS task_watchers (
    id         BIGSERIAL PRIMARY KEY,
    task_id    BIGINT NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(task_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_task_watchers_task ON task_watchers(task_id);
CREATE INDEX IF NOT EXISTS idx_task_watchers_user ON task_watchers(user_id);

-- Wiki page watchers
CREATE TABLE IF NOT EXISTS wiki_page_watchers (
    id         BIGSERIAL PRIMARY KEY,
    page_id    BIGINT NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
    user_id    BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(page_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_wiki_watchers_page ON wiki_page_watchers(page_id);
CREATE INDEX IF NOT EXISTS idx_wiki_watchers_user ON wiki_page_watchers(user_id);

-- Project activity log: audit trail for all project actions
CREATE TABLE IF NOT EXISTS project_activity (
    id          BIGSERIAL PRIMARY KEY,
    project_id  BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action      TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id   BIGINT NOT NULL,
    entity_title TEXT,
    details     JSONB,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_project_activity_project ON project_activity(project_id);
CREATE INDEX IF NOT EXISTS idx_project_activity_entity ON project_activity(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_project_activity_user ON project_activity(user_id);
CREATE INDEX IF NOT EXISTS idx_project_activity_created ON project_activity(created_at);
