-- Notifications for mentions, task comments, annotation comments, and replies
CREATE TABLE IF NOT EXISTS notifications (
    id            BIGSERIAL PRIMARY KEY,
    recipient_id  BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    sender_id     BIGINT REFERENCES users(id) ON DELETE SET NULL,
    type          VARCHAR(50) NOT NULL,   -- mention, task_comment, annotation_comment, reply
    entity_type   VARCHAR(50) NOT NULL,   -- task_comment, annotation_comment, wiki_annotation
    entity_id     BIGINT NOT NULL,
    project_id    BIGINT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    message       TEXT NOT NULL,
    link          TEXT NOT NULL,
    read_at       TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_notifications_recipient   ON notifications(recipient_id);
CREATE INDEX IF NOT EXISTS idx_notifications_unread      ON notifications(recipient_id) WHERE read_at IS NULL;
CREATE INDEX IF NOT EXISTS idx_notifications_project     ON notifications(project_id);
