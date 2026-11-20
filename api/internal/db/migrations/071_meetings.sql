-- Migration 071: Meeting minutes, project-scoped, with attendees

CREATE TABLE IF NOT EXISTS meetings (
    id         INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    title      TEXT NOT NULL,
    starts_at  DATETIME NOT NULL,
    ends_at    DATETIME NOT NULL,
    agenda     TEXT NOT NULL DEFAULT '',
    minutes    TEXT NOT NULL DEFAULT '',
    created_by INTEGER REFERENCES users(id) ON DELETE SET NULL,
    created_at DATETIME NOT NULL DEFAULT (datetime('now')),
    updated_at DATETIME NOT NULL DEFAULT (datetime('now'))
);
CREATE INDEX IF NOT EXISTS idx_meetings_project_starts ON meetings(project_id, starts_at);

CREATE TABLE IF NOT EXISTS meeting_attendees (
    meeting_id INTEGER NOT NULL REFERENCES meetings(id) ON DELETE CASCADE,
    user_id    INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    PRIMARY KEY (meeting_id, user_id)
);
CREATE INDEX IF NOT EXISTS idx_meeting_attendees_user ON meeting_attendees(user_id);
