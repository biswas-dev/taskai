-- Project-scope sprints and tags: add project_id, update tags unique constraint

-- Add project_id to sprints
ALTER TABLE sprints ADD COLUMN project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_sprints_project_id ON sprints(project_id);

-- Recreate tags table to drop UNIQUE(user_id, name) and add project_id
-- SQLite doesn't support DROP CONSTRAINT, so we rebuild the table.
CREATE TABLE tags_new (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    name TEXT NOT NULL,
    color TEXT NOT NULL DEFAULT '#3B82F6',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    team_id INTEGER REFERENCES teams(id) ON DELETE CASCADE,
    project_id INTEGER REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (user_id) REFERENCES users(id) ON DELETE CASCADE
);

INSERT INTO tags_new (id, user_id, name, color, created_at, team_id, project_id)
SELECT id, user_id, name, color, created_at, team_id, NULL FROM tags;

DROP TABLE tags;
ALTER TABLE tags_new RENAME TO tags;

CREATE INDEX IF NOT EXISTS idx_tags_user_id ON tags(user_id);
CREATE INDEX IF NOT EXISTS idx_tags_team_id ON tags(team_id);
CREATE INDEX IF NOT EXISTS idx_tags_project_id ON tags(project_id);

-- Unique tag name per project (only for project-scoped tags)
CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_project_name ON tags(project_id, name)
    WHERE project_id IS NOT NULL;
