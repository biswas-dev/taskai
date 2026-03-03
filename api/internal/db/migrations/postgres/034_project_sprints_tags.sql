-- Project-scope sprints and tags: add project_id, update tags unique constraint

-- Add project_id to sprints
ALTER TABLE sprints ADD COLUMN IF NOT EXISTS project_id BIGINT REFERENCES projects(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_sprints_project_id ON sprints(project_id);

-- Add project_id to tags
ALTER TABLE tags ADD COLUMN IF NOT EXISTS project_id BIGINT REFERENCES projects(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_tags_project_id ON tags(project_id);

-- Drop old unique constraint (user_id, name) and replace with (project_id, name)
ALTER TABLE tags DROP CONSTRAINT IF EXISTS tags_user_id_name_key;

-- Unique tag name per project (only for project-scoped tags)
CREATE UNIQUE INDEX IF NOT EXISTS idx_tags_project_name ON tags(project_id, name)
    WHERE project_id IS NOT NULL;
