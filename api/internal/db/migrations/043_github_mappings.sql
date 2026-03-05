-- Persists GitHub sync mappings per project so they survive across sessions
-- and are used as defaults when syncing without explicit mappings in the request.

CREATE TABLE IF NOT EXISTS github_status_mappings (
    project_id   INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    status_key   TEXT    NOT NULL, -- e.g. "In Progress", "open", "closed", "label:bug"
    swim_lane_id INTEGER REFERENCES swim_lanes(id) ON DELETE SET NULL,
    PRIMARY KEY (project_id, status_key)
);

CREATE TABLE IF NOT EXISTS github_user_mappings (
    project_id   INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    github_login TEXT    NOT NULL,
    user_id      INTEGER REFERENCES users(id) ON DELETE SET NULL,
    PRIMARY KEY (project_id, github_login)
);
