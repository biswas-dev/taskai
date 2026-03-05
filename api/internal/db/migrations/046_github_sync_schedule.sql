ALTER TABLE projects ADD COLUMN github_sync_hour INTEGER DEFAULT 0; -- 0-23
ALTER TABLE projects ADD COLUMN github_sync_day  INTEGER DEFAULT 0; -- weekly: 0-6 (Sun=0), monthly: 1-28
