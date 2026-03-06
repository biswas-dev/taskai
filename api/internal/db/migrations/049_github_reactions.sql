CREATE TABLE github_reactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    task_id INTEGER REFERENCES tasks(id) ON DELETE CASCADE,
    task_comment_id INTEGER REFERENCES task_comments(id) ON DELETE CASCADE,
    reaction TEXT NOT NULL,
    count INTEGER NOT NULL DEFAULT 0,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE UNIQUE INDEX idx_github_reactions_task
    ON github_reactions (task_id, reaction)
    WHERE task_id IS NOT NULL;

CREATE UNIQUE INDEX idx_github_reactions_comment
    ON github_reactions (task_comment_id, reaction)
    WHERE task_comment_id IS NOT NULL;
