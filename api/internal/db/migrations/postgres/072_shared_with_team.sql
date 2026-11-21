-- Shared projects are open to every active member of their team (company).
-- Existing projects stay private so nobody gains access they did not have.
ALTER TABLE projects ADD COLUMN IF NOT EXISTS shared_with_team BOOLEAN NOT NULL DEFAULT FALSE;
