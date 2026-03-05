ALTER TABLE tasks ADD COLUMN IF NOT EXISTS start_date TIMESTAMPTZ;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS github_sync_interval TEXT; -- 'daily','weekly','monthly', NULL=disabled

CREATE TABLE IF NOT EXISTS github_sync_logs (
    id           BIGSERIAL PRIMARY KEY,
    project_id   BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    started_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    completed_at TIMESTAMPTZ,
    status       TEXT NOT NULL DEFAULT 'running', -- 'running','success','failed'
    triggered_by TEXT NOT NULL DEFAULT 'manual',  -- 'manual','auto'
    created_tasks    INTEGER NOT NULL DEFAULT 0,
    updated_tasks    INTEGER NOT NULL DEFAULT 0,
    created_comments INTEGER NOT NULL DEFAULT 0,
    skipped_tasks    INTEGER NOT NULL DEFAULT 0,
    error_message    TEXT
);
CREATE INDEX IF NOT EXISTS idx_github_sync_logs_project ON github_sync_logs(project_id, started_at DESC);
