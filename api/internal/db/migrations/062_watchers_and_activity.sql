-- Task watchers: users who want to be notified of all changes on a task
CREATE TABLE IF NOT EXISTS task_watchers (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id    INTEGER NOT NULL REFERENCES tasks(id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(task_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_task_watchers_task ON task_watchers(task_id);
CREATE INDEX IF NOT EXISTS idx_task_watchers_user ON task_watchers(user_id);

-- Wiki page watchers
CREATE TABLE IF NOT EXISTS wiki_page_watchers (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    page_id    INTEGER NOT NULL REFERENCES wiki_pages(id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    UNIQUE(page_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_wiki_watchers_page ON wiki_page_watchers(page_id);
CREATE INDEX IF NOT EXISTS idx_wiki_watchers_user ON wiki_page_watchers(user_id);

-- Project activity log: audit trail for all project actions
CREATE TABLE IF NOT EXISTS project_activity (
    id          INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id  INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    user_id     INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    action      TEXT NOT NULL,  -- 'task_created', 'task_status_changed', 'comment_added', etc.
    entity_type TEXT NOT NULL,  -- 'task', 'wiki_page', 'comment', 'project'
    entity_id   INTEGER NOT NULL,
    entity_title TEXT,          -- snapshot of title at time of action
    details     TEXT,           -- JSON with before/after values, e.g. {"from":"todo","to":"done"}
    created_at  DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_project_activity_project ON project_activity(project_id);
CREATE INDEX IF NOT EXISTS idx_project_activity_entity ON project_activity(entity_type, entity_id);
CREATE INDEX IF NOT EXISTS idx_project_activity_user ON project_activity(user_id);
CREATE INDEX IF NOT EXISTS idx_project_activity_created ON project_activity(created_at);
