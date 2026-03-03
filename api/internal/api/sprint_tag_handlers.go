package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"taskai/ent"
)

// Sprint represents a sprint
type Sprint struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Name      string    `json:"name"`
	Goal      string    `json:"goal,omitempty"`
	StartDate string    `json:"start_date,omitempty"`
	EndDate   string    `json:"end_date,omitempty"`
	Status    string    `json:"status"`
	IsShared  bool      `json:"is_shared,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Tag represents a tag
type Tag struct {
	ID        int       `json:"id"`
	UserID    int       `json:"user_id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	IsShared  bool      `json:"is_shared,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// CreateSprintRequest represents a request to create a sprint
type CreateSprintRequest struct {
	Name      string `json:"name"`
	Goal      string `json:"goal,omitempty"`
	StartDate string `json:"start_date,omitempty"`
	EndDate   string `json:"end_date,omitempty"`
	Status    string `json:"status,omitempty"`
}

// UpdateSprintRequest represents a request to update a sprint
type UpdateSprintRequest struct {
	Name      *string `json:"name,omitempty"`
	Goal      *string `json:"goal,omitempty"`
	StartDate *string `json:"start_date,omitempty"`
	EndDate   *string `json:"end_date,omitempty"`
	Status    *string `json:"status,omitempty"`
}

// CreateTagRequest represents a request to create a tag
type CreateTagRequest struct {
	Name  string `json:"name"`
	Color string `json:"color,omitempty"`
}

// UpdateTagRequest represents a request to update a tag
type UpdateTagRequest struct {
	Name  *string `json:"name,omitempty"`
	Color *string `json:"color,omitempty"`
}

// scanSprint scans a sprint row from a query result, handling nullable fields.
func scanSprint(row interface {
	Scan(...interface{}) error
}) (Sprint, error) {
	var sp Sprint
	var id, userID int64
	var goal *string
	var startDate, endDate *time.Time
	if err := row.Scan(&id, &userID, &sp.Name, &goal, &startDate, &endDate, &sp.Status, &sp.CreatedAt, &sp.UpdatedAt); err != nil {
		return sp, err
	}
	sp.ID = int(id)
	sp.UserID = int(userID)
	if goal != nil {
		sp.Goal = *goal
	}
	if startDate != nil {
		sp.StartDate = startDate.Format("2006-01-02")
	}
	if endDate != nil {
		sp.EndDate = endDate.Format("2006-01-02")
	}
	return sp, nil
}

const sprintSelectCols = `id, user_id, name, goal, start_date, end_date, status, created_at, updated_at`

// HandleListSprints returns all sprints for a project.
// Route: GET /api/projects/{id}/sprints
func (s *Server) HandleListSprints(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "bad_request")
		return
	}

	hasAccess, err := s.checkProjectAccess(ctx, userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to check access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "Forbidden", "forbidden")
		return
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT `+sprintSelectCols+`, 0 as is_shared
		 FROM sprints WHERE project_id = $1
		 UNION
		 SELECT `+sprintSelectCols+`, 1 as is_shared
		 FROM sprints WHERE id IN (
		     SELECT sprint_id FROM sprint_project_refs WHERE to_project_id = $1
		 )
		 ORDER BY start_date DESC, created_at DESC`,
		projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch sprints", "internal_error")
		return
	}
	defer rows.Close()

	sprints := []Sprint{}
	for rows.Next() {
		var sp Sprint
		var id, userID int64
		var goal *string
		var startDate, endDate *time.Time
		var isSharedInt int
		if err := rows.Scan(&id, &userID, &sp.Name, &goal, &startDate, &endDate, &sp.Status, &sp.CreatedAt, &sp.UpdatedAt, &isSharedInt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan sprint", "internal_error")
			return
		}
		sp.ID = int(id)
		sp.UserID = int(userID)
		if goal != nil {
			sp.Goal = *goal
		}
		if startDate != nil {
			sp.StartDate = startDate.Format("2006-01-02")
		}
		if endDate != nil {
			sp.EndDate = endDate.Format("2006-01-02")
		}
		sp.IsShared = isSharedInt == 1
		sprints = append(sprints, sp)
	}

	respondJSON(w, http.StatusOK, sprints)
}

// HandleCreateSprint creates a new sprint in a project.
// Route: POST /api/projects/{id}/sprints
func (s *Server) HandleCreateSprint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "bad_request")
		return
	}

	hasAccess, err := s.checkProjectAccess(ctx, userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to check access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "Forbidden", "forbidden")
		return
	}

	var req CreateSprintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Sprint name is required", http.StatusBadRequest)
		return
	}

	status := req.Status
	if status == "" {
		status = "planned"
	}
	if status != "planned" && status != "active" && status != "completed" {
		http.Error(w, "Invalid status. Must be planned, active, or completed", http.StatusBadRequest)
		return
	}

	var goalVal *string
	if req.Goal != "" {
		goalVal = &req.Goal
	}
	var startDateVal *string
	if req.StartDate != "" {
		startDateVal = &req.StartDate
	}
	var endDateVal *string
	if req.EndDate != "" {
		endDateVal = &req.EndDate
	}

	var newID int64
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO sprints (user_id, name, goal, start_date, end_date, status, project_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		userID, req.Name, goalVal, startDateVal, endDateVal, status, projectID,
	).Scan(&newID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create sprint", "internal_error")
		return
	}

	sp, err := scanSprint(s.db.QueryRowContext(ctx,
		`SELECT `+sprintSelectCols+` FROM sprints WHERE id = $1`, newID))
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch new sprint", "internal_error")
		return
	}

	respondJSON(w, http.StatusCreated, sp)
}

// HandleUpdateSprint updates a sprint (verifies project membership via stored project_id).
// Route: PATCH /api/sprints/{id}
func (s *Server) HandleUpdateSprint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sprintID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid sprint ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Look up the sprint's project_id for authorization
	var projectID *int64
	err = s.db.QueryRowContext(ctx, `SELECT project_id FROM sprints WHERE id = $1`, sprintID).Scan(&projectID)
	if err == sql.ErrNoRows {
		http.Error(w, "Sprint not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// If sprint has a project_id, verify membership
	if projectID != nil {
		hasAccess, err := s.checkProjectAccess(ctx, userID, *projectID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !hasAccess {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	var req UpdateSprintRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Build update using Ent
	updateBuilder := s.db.Client.Sprint.UpdateOneID(sprintID)
	hasUpdates := false

	if req.Name != nil {
		updateBuilder.SetName(*req.Name)
		hasUpdates = true
	}
	if req.Goal != nil {
		updateBuilder.SetGoal(*req.Goal)
		hasUpdates = true
	}
	if req.StartDate != nil {
		if *req.StartDate != "" {
			startDate, err := time.Parse("2006-01-02", *req.StartDate)
			if err == nil {
				updateBuilder.SetStartDate(startDate)
				hasUpdates = true
			}
		}
	}
	if req.EndDate != nil {
		if *req.EndDate != "" {
			endDate, err := time.Parse("2006-01-02", *req.EndDate)
			if err == nil {
				updateBuilder.SetEndDate(endDate)
				hasUpdates = true
			}
		}
	}
	if req.Status != nil {
		if *req.Status != "planned" && *req.Status != "active" && *req.Status != "completed" {
			http.Error(w, "Invalid status", http.StatusBadRequest)
			return
		}
		updateBuilder.SetStatus(*req.Status)
		hasUpdates = true
	}

	if !hasUpdates {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	updatedSprint, err := updateBuilder.Save(ctx)
	if err != nil {
		http.Error(w, "Failed to update sprint", http.StatusInternalServerError)
		return
	}

	sprint := Sprint{
		ID:        int(updatedSprint.ID),
		UserID:    int(updatedSprint.UserID),
		Name:      updatedSprint.Name,
		Status:    updatedSprint.Status,
		CreatedAt: updatedSprint.CreatedAt,
		UpdatedAt: updatedSprint.UpdatedAt,
	}
	if updatedSprint.Goal != nil {
		sprint.Goal = *updatedSprint.Goal
	}
	if updatedSprint.StartDate != nil {
		sprint.StartDate = updatedSprint.StartDate.Format("2006-01-02")
	}
	if updatedSprint.EndDate != nil {
		sprint.EndDate = updatedSprint.EndDate.Format("2006-01-02")
	}

	respondJSON(w, http.StatusOK, sprint)
}

// HandleDeleteSprint deletes a sprint (verifies project membership via stored project_id).
// Route: DELETE /api/sprints/{id}
func (s *Server) HandleDeleteSprint(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	sprintID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid sprint ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Look up the sprint's project_id for authorization
	var projectID *int64
	err = s.db.QueryRowContext(ctx, `SELECT project_id FROM sprints WHERE id = $1`, sprintID).Scan(&projectID)
	if err == sql.ErrNoRows {
		http.Error(w, "Sprint not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// If sprint has a project_id, verify membership
	if projectID != nil {
		hasAccess, err := s.checkProjectAccess(ctx, userID, *projectID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !hasAccess {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	err = s.db.Client.Sprint.DeleteOneID(sprintID).Exec(ctx)
	if err != nil {
		http.Error(w, "Failed to delete sprint", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Sprint deleted successfully"})
}

// HandleListTags returns all tags for a project.
// Route: GET /api/projects/{id}/tags
func (s *Server) HandleListTags(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "bad_request")
		return
	}

	hasAccess, err := s.checkProjectAccess(ctx, userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to check access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "Forbidden", "forbidden")
		return
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT id, user_id, name, color, created_at, 0 as is_shared
		 FROM tags WHERE project_id = $1
		 UNION
		 SELECT id, user_id, name, color, created_at, 1 as is_shared
		 FROM tags WHERE id IN (
		     SELECT tag_id FROM tag_project_refs WHERE to_project_id = $1
		 )
		 ORDER BY name ASC`,
		projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch tags", "internal_error")
		return
	}
	defer rows.Close()

	tags := []Tag{}
	for rows.Next() {
		var t Tag
		var id, userID int64
		var isSharedInt int
		if err := rows.Scan(&id, &userID, &t.Name, &t.Color, &t.CreatedAt, &isSharedInt); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan tag", "internal_error")
			return
		}
		t.ID = int(id)
		t.UserID = int(userID)
		t.IsShared = isSharedInt == 1
		tags = append(tags, t)
	}

	respondJSON(w, http.StatusOK, tags)
}

// HandleCreateTag creates a new tag in a project.
// Route: POST /api/projects/{id}/tags
func (s *Server) HandleCreateTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "bad_request")
		return
	}

	hasAccess, err := s.checkProjectAccess(ctx, userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to check access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "Forbidden", "forbidden")
		return
	}

	var req CreateTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Name == "" {
		http.Error(w, "Tag name is required", http.StatusBadRequest)
		return
	}

	color := req.Color
	if color == "" {
		color = "#3B82F6"
	}

	var newID int64
	err = s.db.QueryRowContext(ctx,
		`INSERT INTO tags (user_id, name, color, project_id)
		 VALUES ($1, $2, $3, $4) RETURNING id`,
		userID, req.Name, color, projectID,
	).Scan(&newID)
	if err != nil {
		if isUniqueConstraintError(err) {
			http.Error(w, "Failed to create tag. Tag name must be unique.", http.StatusConflict)
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to create tag", "internal_error")
		return
	}

	var t Tag
	var id, uid int64
	err = s.db.QueryRowContext(ctx,
		`SELECT id, user_id, name, color, created_at FROM tags WHERE id = $1`, newID,
	).Scan(&id, &uid, &t.Name, &t.Color, &t.CreatedAt)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch new tag", "internal_error")
		return
	}
	t.ID = int(id)
	t.UserID = int(uid)

	respondJSON(w, http.StatusCreated, t)
}

// HandleUpdateTag updates a tag (verifies project membership via stored project_id).
// Route: PATCH /api/tags/{id}
func (s *Server) HandleUpdateTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tagID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid tag ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Look up the tag's project_id for authorization
	var projectID *int64
	err = s.db.QueryRowContext(ctx, `SELECT project_id FROM tags WHERE id = $1`, tagID).Scan(&projectID)
	if err == sql.ErrNoRows {
		http.Error(w, "Tag not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// If tag has a project_id, verify membership
	if projectID != nil {
		hasAccess, err := s.checkProjectAccess(ctx, userID, *projectID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !hasAccess {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	var req UpdateTagRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Build update using Ent
	updateBuilder := s.db.Client.Tag.UpdateOneID(tagID)
	hasUpdates := false

	if req.Name != nil {
		updateBuilder.SetName(*req.Name)
		hasUpdates = true
	}
	if req.Color != nil {
		updateBuilder.SetColor(*req.Color)
		hasUpdates = true
	}

	if !hasUpdates {
		http.Error(w, "No fields to update", http.StatusBadRequest)
		return
	}

	updatedTag, err := updateBuilder.Save(ctx)
	if err != nil {
		http.Error(w, "Failed to update tag", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, Tag{
		ID:        int(updatedTag.ID),
		UserID:    int(updatedTag.UserID),
		Name:      updatedTag.Name,
		Color:     updatedTag.Color,
		CreatedAt: updatedTag.CreatedAt,
	})
}

// HandleDeleteTag deletes a tag (verifies project membership via stored project_id).
// Route: DELETE /api/tags/{id}
func (s *Server) HandleDeleteTag(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	tagID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid tag ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Look up the tag's project_id for authorization
	var projectID *int64
	err = s.db.QueryRowContext(ctx, `SELECT project_id FROM tags WHERE id = $1`, tagID).Scan(&projectID)
	if err == sql.ErrNoRows {
		http.Error(w, "Tag not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// If tag has a project_id, verify membership
	if projectID != nil {
		hasAccess, err := s.checkProjectAccess(ctx, userID, *projectID)
		if err != nil {
			http.Error(w, "Internal server error", http.StatusInternalServerError)
			return
		}
		if !hasAccess {
			http.Error(w, "Forbidden", http.StatusForbidden)
			return
		}
	}

	err = s.db.Client.Tag.DeleteOneID(tagID).Exec(ctx)
	if err != nil {
		http.Error(w, "Failed to delete tag", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Tag deleted successfully"})
}

// isUniqueConstraintError checks if an error is a unique constraint violation.
func isUniqueConstraintError(err error) bool {
	if err == nil {
		return false
	}
	if ent.IsConstraintError(err) {
		return true
	}
	s := err.Error()
	return strings.Contains(s, "UNIQUE constraint") || strings.Contains(s, "unique constraint") || strings.Contains(s, "duplicate key")
}
