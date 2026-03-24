package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// ActivityEntry represents a project activity log entry
type ActivityEntry struct {
	ID          int64           `json:"id"`
	ProjectID   int64           `json:"project_id"`
	UserID      int64           `json:"user_id"`
	UserName    *string         `json:"user_name,omitempty"`
	Action      string          `json:"action"`
	EntityType  string          `json:"entity_type"`
	EntityID    int64           `json:"entity_id"`
	EntityTitle *string         `json:"entity_title,omitempty"`
	Details     json.RawMessage `json:"details,omitempty"`
	CreatedAt   time.Time       `json:"created_at"`
}

// logActivity records a project activity event (non-blocking, best-effort)
func (s *Server) logActivity(ctx context.Context, projectID, userID int64, action, entityType string, entityID int64, entityTitle string, details map[string]interface{}) {
	go func() {
		bgCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var detailsJSON []byte
		if details != nil {
			detailsJSON, _ = json.Marshal(details)
		}

		_, err := s.db.ExecContext(bgCtx, `
			INSERT INTO project_activity (project_id, user_id, action, entity_type, entity_id, entity_title, details)
			VALUES ($1, $2, $3, $4, $5, $6, $7)
		`, projectID, userID, action, entityType, entityID, entityTitle, string(detailsJSON))
		if err != nil {
			s.logger.Error("Failed to log activity", zap.Error(err), zap.String("action", action))
		}
	}()
}

// HandleListProjectActivity returns activity log for a project
func (s *Server) HandleListProjectActivity(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "invalid_input")
		return
	}

	hasAccess, _ := s.checkProjectAccess(ctx, userID, projectID)
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	// Optional filters
	entityType := r.URL.Query().Get("entity_type")
	entityIDStr := r.URL.Query().Get("entity_id")
	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 200 {
		limit = l
	}
	before := r.URL.Query().Get("before") // cursor-based pagination

	query := `
		SELECT pa.id, pa.project_id, pa.user_id,
		       COALESCE(u.first_name || ' ' || u.last_name, u.name, u.email) AS user_name,
		       pa.action, pa.entity_type, pa.entity_id, pa.entity_title, pa.details, pa.created_at
		FROM project_activity pa
		JOIN users u ON u.id = pa.user_id
		WHERE pa.project_id = $1
	`
	args := []interface{}{projectID}
	argIdx := 2

	if entityType != "" {
		query += ` AND pa.entity_type = $` + strconv.Itoa(argIdx)
		args = append(args, entityType)
		argIdx++
	}
	if entityIDStr != "" {
		if eid, err := strconv.ParseInt(entityIDStr, 10, 64); err == nil {
			query += ` AND pa.entity_id = $` + strconv.Itoa(argIdx)
			args = append(args, eid)
			argIdx++
		}
	}
	if before != "" {
		query += ` AND pa.created_at < $` + strconv.Itoa(argIdx)
		args = append(args, before)
		argIdx++
	}

	query += ` ORDER BY pa.created_at DESC LIMIT $` + strconv.Itoa(argIdx)
	args = append(args, limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		s.logger.Error("Failed to query activity log", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch activity", "internal_error")
		return
	}
	defer rows.Close()

	entries := []ActivityEntry{}
	for rows.Next() {
		var e ActivityEntry
		var userName, entityTitle, details *string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.UserID, &userName, &e.Action, &e.EntityType, &e.EntityID, &entityTitle, &details, &e.CreatedAt); err != nil {
			continue
		}
		e.UserName = userName
		e.EntityTitle = entityTitle
		if details != nil && *details != "" {
			e.Details = json.RawMessage(*details)
		}
		entries = append(entries, e)
	}

	respondJSON(w, http.StatusOK, entries)
}

// HandleListTaskActivity returns activity log for a specific task
func (s *Server) HandleListTaskActivity(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	taskID, err := strconv.ParseInt(chi.URLParam(r, "taskId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid task ID", "invalid_input")
		return
	}

	taskEntity, err := s.db.Client.Task.Get(ctx, taskID)
	if err != nil {
		respondError(w, http.StatusNotFound, "task not found", "not_found")
		return
	}

	hasAccess, _ := s.checkProjectAccess(ctx, userID, taskEntity.ProjectID)
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT pa.id, pa.project_id, pa.user_id,
		       COALESCE(u.first_name || ' ' || u.last_name, u.name, u.email) AS user_name,
		       pa.action, pa.entity_type, pa.entity_id, pa.entity_title, pa.details, pa.created_at
		FROM project_activity pa
		JOIN users u ON u.id = pa.user_id
		WHERE pa.entity_type = 'task' AND pa.entity_id = $1
		ORDER BY pa.created_at DESC
		LIMIT 100
	`, taskID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch activity", "internal_error")
		return
	}
	defer rows.Close()

	entries := []ActivityEntry{}
	for rows.Next() {
		var e ActivityEntry
		var userName, entityTitle, details *string
		if err := rows.Scan(&e.ID, &e.ProjectID, &e.UserID, &userName, &e.Action, &e.EntityType, &e.EntityID, &entityTitle, &details, &e.CreatedAt); err != nil {
			continue
		}
		e.UserName = userName
		e.EntityTitle = entityTitle
		if details != nil && *details != "" {
			e.Details = json.RawMessage(*details)
		}
		entries = append(entries, e)
	}

	respondJSON(w, http.StatusOK, entries)
}
