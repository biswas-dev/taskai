package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
)

type ShareRequest struct {
	ProjectID int64 `json:"project_id"`
}

// --- Sprint sharing ---

// HandleShareSprint shares a sprint to another project.
// Route: POST /api/sprints/:id/share
func (s *Server) HandleShareSprint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sprintID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint ID", "bad_request")
		return
	}

	var req ShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "bad_request")
		return
	}
	if req.ProjectID == 0 {
		respondError(w, http.StatusBadRequest, "project_id required", "validation_error")
		return
	}

	var sourceProjectID int64
	err = s.db.QueryRowContext(ctx, `SELECT project_id FROM sprints WHERE id = $1`, sprintID).Scan(&sourceProjectID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "sprint not found", "not_found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get sprint", "internal_error")
		return
	}
	if sourceProjectID == req.ProjectID {
		respondError(w, http.StatusBadRequest, "cannot share to the same project", "validation_error")
		return
	}

	srcAccess, _ := s.checkProjectAccess(ctx, userID, sourceProjectID)
	dstAccess, _ := s.checkProjectAccess(ctx, userID, req.ProjectID)
	if !srcAccess || !dstAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO sprint_project_refs (sprint_id, to_project_id, shared_by) VALUES ($1, $2, $3)
		 ON CONFLICT (sprint_id, to_project_id) DO NOTHING`,
		sprintID, req.ProjectID, userID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to share sprint", "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleUnshareSprint removes a sprint from a project it was shared to.
// Route: DELETE /api/sprints/:id/share/:projectId
func (s *Server) HandleUnshareSprint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	sprintID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid sprint ID", "bad_request")
		return
	}
	targetProjectID, err := strconv.ParseInt(chi.URLParam(r, "projectId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "bad_request")
		return
	}

	hasAccess, _ := s.checkProjectAccess(ctx, userID, targetProjectID)
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	_, err = s.db.ExecContext(ctx,
		`DELETE FROM sprint_project_refs WHERE sprint_id = $1 AND to_project_id = $2`,
		sprintID, targetProjectID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to unshare sprint", "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Tag sharing ---

// HandleShareTag shares a tag to another project.
// Route: POST /api/tags/:id/share
func (s *Server) HandleShareTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	tagID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid tag ID", "bad_request")
		return
	}

	var req ShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "bad_request")
		return
	}
	if req.ProjectID == 0 {
		respondError(w, http.StatusBadRequest, "project_id required", "validation_error")
		return
	}

	var sourceProjectID int64
	err = s.db.QueryRowContext(ctx, `SELECT project_id FROM tags WHERE id = $1`, tagID).Scan(&sourceProjectID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "tag not found", "not_found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get tag", "internal_error")
		return
	}
	if sourceProjectID == req.ProjectID {
		respondError(w, http.StatusBadRequest, "cannot share to the same project", "validation_error")
		return
	}

	srcAccess, _ := s.checkProjectAccess(ctx, userID, sourceProjectID)
	dstAccess, _ := s.checkProjectAccess(ctx, userID, req.ProjectID)
	if !srcAccess || !dstAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO tag_project_refs (tag_id, to_project_id, shared_by) VALUES ($1, $2, $3)
		 ON CONFLICT (tag_id, to_project_id) DO NOTHING`,
		tagID, req.ProjectID, userID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to share tag", "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleUnshareTag removes a tag from a project it was shared to.
// Route: DELETE /api/tags/:id/share/:projectId
func (s *Server) HandleUnshareTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	tagID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid tag ID", "bad_request")
		return
	}
	targetProjectID, err := strconv.ParseInt(chi.URLParam(r, "projectId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "bad_request")
		return
	}

	hasAccess, _ := s.checkProjectAccess(ctx, userID, targetProjectID)
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	_, err = s.db.ExecContext(ctx,
		`DELETE FROM tag_project_refs WHERE tag_id = $1 AND to_project_id = $2`,
		tagID, targetProjectID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to unshare tag", "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// --- Attachment sharing ---

// HandleShareAttachment shares an attachment/asset to another project.
// Route: POST /api/attachments/:id/share
func (s *Server) HandleShareAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	attachmentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid attachment ID", "bad_request")
		return
	}

	var req ShareRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "bad_request")
		return
	}
	if req.ProjectID == 0 {
		respondError(w, http.StatusBadRequest, "project_id required", "validation_error")
		return
	}

	var sourceProjectID int64
	err = s.db.QueryRowContext(ctx, `SELECT project_id FROM task_attachments WHERE id = $1`, attachmentID).Scan(&sourceProjectID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "attachment not found", "not_found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get attachment", "internal_error")
		return
	}
	if sourceProjectID == req.ProjectID {
		respondError(w, http.StatusBadRequest, "cannot share to the same project", "validation_error")
		return
	}

	srcAccess, _ := s.checkProjectAccess(ctx, userID, sourceProjectID)
	dstAccess, _ := s.checkProjectAccess(ctx, userID, req.ProjectID)
	if !srcAccess || !dstAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO attachment_project_refs (attachment_id, to_project_id, shared_by) VALUES ($1, $2, $3)
		 ON CONFLICT (attachment_id, to_project_id) DO NOTHING`,
		attachmentID, req.ProjectID, userID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to share attachment", "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// HandleUnshareAttachment removes an attachment from a project it was shared to.
// Route: DELETE /api/attachments/:id/share/:projectId
func (s *Server) HandleUnshareAttachment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	attachmentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid attachment ID", "bad_request")
		return
	}
	targetProjectID, err := strconv.ParseInt(chi.URLParam(r, "projectId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "bad_request")
		return
	}

	hasAccess, _ := s.checkProjectAccess(ctx, userID, targetProjectID)
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	_, err = s.db.ExecContext(ctx,
		`DELETE FROM attachment_project_refs WHERE attachment_id = $1 AND to_project_id = $2`,
		attachmentID, targetProjectID,
	)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to unshare attachment", "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
