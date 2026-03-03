-- Track which project each go-draw drawing belongs to for project-level isolation
CREATE TABLE IF NOT EXISTS project_drawings (
    id SERIAL PRIMARY KEY,
    project_id INTEGER NOT NULL,
    draw_id TEXT NOT NULL,
    created_by INTEGER NOT NULL,
    created_at TIMESTAMP NOT NULL DEFAULT CURRENT_TIMESTAMP,
    FOREIGN KEY (project_id) REFERENCES projects(id) ON DELETE CASCADE,
    FOREIGN KEY (created_by) REFERENCES users(id) ON DELETE CASCADE,
    UNIQUE(draw_id)
);

CREATE INDEX IF NOT EXISTS idx_project_drawings_project_id ON project_drawings(project_id);
CREATE INDEX IF NOT EXISTS idx_project_drawings_draw_id ON project_drawings(draw_id);
