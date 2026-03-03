package api

import (
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// ProjectDrawing represents a drawing registered to a project
type ProjectDrawing struct {
	ID        int64  `json:"id"`
	ProjectID int64  `json:"project_id"`
	DrawID    string `json:"draw_id"`
	CreatedBy int64  `json:"created_by"`
	CreatedAt string `json:"created_at"`
}

type RegisterDrawingRequest struct {
	DrawID string `json:"draw_id"`
}

// HandleListProjectDrawings returns all draw IDs registered to a project.
// Only project members can access this.
func (s *Server) HandleListProjectDrawings(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "bad_request")
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	hasAccess, err := s.checkProjectAccess(r.Context(), userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to check access", "internal_error")
		return
	}
	if !hasAccess {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	rows, err := s.db.QueryContext(r.Context(),
		`SELECT id, project_id, draw_id, created_by, created_at
		 FROM project_drawings
		 WHERE project_id = $1
		 ORDER BY created_at DESC`,
		projectID,
	)
	if err != nil {
		s.logger.Error("Failed to list project drawings", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list drawings", "internal_error")
		return
	}
	defer rows.Close()

	drawings := []ProjectDrawing{}
	for rows.Next() {
		var d ProjectDrawing
		if err := rows.Scan(&d.ID, &d.ProjectID, &d.DrawID, &d.CreatedBy, &d.CreatedAt); err != nil {
			s.logger.Warn("Failed to scan project drawing", zap.Error(err))
			continue
		}
		drawings = append(drawings, d)
	}

	respondJSON(w, http.StatusOK, drawings)
}

// HandleRegisterProjectDrawing registers a go-draw draw_id with a project.
// Called from the frontend immediately after creating a drawing.
func (s *Server) HandleRegisterProjectDrawing(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "bad_request")
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	hasAccess, err := s.checkProjectAccess(r.Context(), userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to check access", "internal_error")
		return
	}
	if !hasAccess {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var req RegisterDrawingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.DrawID == "" {
		respondError(w, http.StatusBadRequest, "draw_id required", "bad_request")
		return
	}

	_, err = s.db.ExecContext(r.Context(),
		`INSERT INTO project_drawings (project_id, draw_id, created_by)
		 VALUES ($1, $2, $3)
		 ON CONFLICT(draw_id) DO NOTHING`,
		projectID, req.DrawID, userID,
	)
	if err != nil {
		s.logger.Error("Failed to register drawing",
			zap.String("draw_id", req.DrawID),
			zap.Int64("project_id", projectID),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to register drawing", "internal_error")
		return
	}

	respondJSON(w, http.StatusCreated, map[string]string{"message": "registered"})
}
