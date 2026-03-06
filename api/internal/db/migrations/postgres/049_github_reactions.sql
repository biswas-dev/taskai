CREATE TABLE IF NOT EXISTS github_reactions (
    id BIGSERIAL PRIMARY KEY,
    task_id BIGINT REFERENCES tasks(id) ON DELETE CASCADE,
    task_comment_id BIGINT REFERENCES task_comments(id) ON DELETE CASCADE,
    reaction VARCHAR(20) NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT NOW()
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_github_reactions_task
    ON github_reactions (task_id, reaction)
    WHERE task_id IS NOT NULL;

CREATE UNIQUE INDEX IF NOT EXISTS idx_github_reactions_comment
    ON github_reactions (task_comment_id, reaction)
    WHERE task_comment_id IS NOT NULL;
