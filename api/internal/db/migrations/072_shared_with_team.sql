-- Shared projects are open to every active member of their team (company).
-- Existing projects stay private so nobody gains access they did not have.
ALTER TABLE projects ADD COLUMN shared_with_team BOOLEAN NOT NULL DEFAULT 0;
-- Marks access granted by company membership, so leaving the company removes
-- only that and never a hand-granted role.
ALTER TABLE project_members ADD COLUMN via_team BOOLEAN NOT NULL DEFAULT 0;
