-- Cross-project sharing: allow sprints, tags, and assets to be shared across projects via pointer tables

CREATE TABLE sprint_project_refs (
    sprint_id INTEGER NOT NULL REFERENCES sprints(id) ON DELETE CASCADE,
    to_project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    shared_by INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    shared_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (sprint_id, to_project_id)
);
CREATE INDEX idx_sprint_project_refs_to_project ON sprint_project_refs(to_project_id);

CREATE TABLE tag_project_refs (
    tag_id INTEGER NOT NULL REFERENCES tags(id) ON DELETE CASCADE,
    to_project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    shared_by INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    shared_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (tag_id, to_project_id)
);
CREATE INDEX idx_tag_project_refs_to_project ON tag_project_refs(to_project_id);

CREATE TABLE attachment_project_refs (
    attachment_id INTEGER NOT NULL REFERENCES task_attachments(id) ON DELETE CASCADE,
    to_project_id INTEGER NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    shared_by INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    shared_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (attachment_id, to_project_id)
);
CREATE INDEX idx_attachment_project_refs_to_project ON attachment_project_refs(to_project_id);
