CREATE TABLE IF NOT EXISTS github_status_mappings (
    project_id   INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    status_key   TEXT    NOT NULL,
    swim_lane_id INTEGER REFERENCES swim_lanes(id) ON DELETE SET NULL,
    PRIMARY KEY (project_id, status_key)
);

CREATE TABLE IF NOT EXISTS github_user_mappings (
    project_id   INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    github_login TEXT    NOT NULL,
    user_id      INTEGER REFERENCES users(id) ON DELETE SET NULL,
    PRIMARY KEY (project_id, github_login)
);
