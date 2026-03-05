ALTER TABLE tasks ADD COLUMN start_date TEXT;
ALTER TABLE projects ADD COLUMN github_sync_interval TEXT; -- 'daily','weekly','monthly', NULL=disabled

CREATE TABLE github_sync_logs (
    id           INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id   INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    started_at   DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at DATETIME,
    status       TEXT NOT NULL DEFAULT 'running', -- 'running','success','failed'
    triggered_by TEXT NOT NULL DEFAULT 'manual',  -- 'manual','auto'
    created_tasks    INTEGER NOT NULL DEFAULT 0,
    updated_tasks    INTEGER NOT NULL DEFAULT 0,
    created_comments INTEGER NOT NULL DEFAULT 0,
    skipped_tasks    INTEGER NOT NULL DEFAULT 0,
    error_message    TEXT
);
CREATE INDEX idx_github_sync_logs_project ON github_sync_logs(project_id, started_at DESC);
