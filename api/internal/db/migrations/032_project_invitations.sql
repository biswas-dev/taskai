-- Replace old email-based project_invitations with user_id-based invite/accept flow
DROP TABLE IF EXISTS project_invitations;

CREATE TABLE project_invitations (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    project_id INTEGER NOT NULL,
    inviter_id INTEGER NOT NULL,
    invitee_user_id INTEGER NOT NULL,
    role TEXT NOT NULL DEFAULT 'member',
    status TEXT NOT NULL DEFAULT 'pending',
    invited_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    responded_at TIMESTAMP,
    last_sent_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (inviter_id) REFERENCES users(id) ON DELETE CASCADE,
    FOREIGN KEY (invitee_user_id) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(project_id, invitee_user_id)
);

CREATE INDEX idx_project_invitations_project_id ON project_invitations(project_id);
CREATE INDEX idx_project_invitations_invitee_user_id ON project_invitations(invitee_user_id);
CREATE INDEX idx_project_invitations_status ON project_invitations(status);
