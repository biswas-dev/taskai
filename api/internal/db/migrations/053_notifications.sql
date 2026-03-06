-- Notifications for mentions, task comments, annotation comments, and replies
CREATE TABLE IF NOT EXISTS notifications (
    id            INTEGER PRIMARY KEY AUTOINCREMENT,
    recipient_id  INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sender_id     INTEGER REFERENCES users(id) ON DELETE SET NULL,
    type          TEXT NOT NULL,
    entity_type   TEXT NOT NULL,
    entity_id     INTEGER NOT NULL,
    project_id    INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    message       TEXT NOT NULL,
    link          TEXT NOT NULL,
    read_at       DATETIME,
    created_at    DATETIME NOT NULL DEFAULT (datetime('now'))
);

CREATE INDEX IF NOT EXISTS idx_notifications_recipient ON notifications(recipient_id);
CREATE INDEX IF NOT EXISTS idx_notifications_project   ON notifications(project_id);
