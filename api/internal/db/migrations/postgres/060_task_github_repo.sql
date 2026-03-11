-- Add github_repo column to tasks for cross-repo project board sync.
-- Stores "owner/repo" (e.g. "elastio/blue-stack") so issues from different repos
-- on the same project board don't collide on issue number.

ALTER TABLE tasks ADD COLUMN IF NOT EXISTS github_repo TEXT NOT NULL DEFAULT '';

-- Backfill existing tasks from their project's configured repo
UPDATE tasks SET github_repo = (
    SELECT p.github_owner || '/' || p.github_repo_name
    FROM projects p WHERE p.id = tasks.project_id
) WHERE github_issue_number IS NOT NULL AND github_repo = '';

-- Replace the old unique index with one that includes github_repo
DROP INDEX IF EXISTS idx_tasks_project_github_issue;
CREATE UNIQUE INDEX IF NOT EXISTS idx_tasks_project_github_repo_issue
    ON tasks (project_id, github_repo, github_issue_number)
    WHERE github_issue_number IS NOT NULL;
