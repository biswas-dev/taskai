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

// TaskWatcher represents a user watching a task
type TaskWatcher struct {
	UserID    int64     `json:"user_id"`
	UserName  *string   `json:"user_name,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// HandleListTaskWatchers returns all watchers for a task
func (s *Server) HandleListTaskWatchers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	taskID, err := strconv.ParseInt(chi.URLParam(r, "taskId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid task ID", "invalid_input")
		return
	}

	// Get task and verify project access
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
		SELECT tw.user_id, COALESCE(u.first_name || ' ' || u.last_name, u.name, u.email) AS user_name, tw.created_at
		FROM task_watchers tw
		JOIN users u ON u.id = tw.user_id
		WHERE tw.task_id = $1
		ORDER BY tw.created_at
	`, taskID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch watchers", "internal_error")
		return
	}
	defer rows.Close()

	watchers := []TaskWatcher{}
	for rows.Next() {
		var w TaskWatcher
		var name string
		if rows.Scan(&w.UserID, &name, &w.CreatedAt) == nil {
			w.UserName = &name
			watchers = append(watchers, w)
		}
	}

	respondJSON(w, http.StatusOK, watchers)
}

// HandleToggleTaskWatcher adds or removes the current user as a watcher
func (s *Server) HandleToggleTaskWatcher(w http.ResponseWriter, r *http.Request) {
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

	// Check if already watching
	var exists int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_watchers WHERE task_id = $1 AND user_id = $2`, taskID, userID).Scan(&exists)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to check watcher status", "internal_error")
		return
	}

	if exists > 0 {
		// Unwatch
		_, err = s.db.ExecContext(ctx, `DELETE FROM task_watchers WHERE task_id = $1 AND user_id = $2`, taskID, userID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to unwatch", "internal_error")
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{"watching": false})
	} else {
		// Watch
		_, err = s.db.ExecContext(ctx, `INSERT INTO task_watchers (task_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, taskID, userID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to watch", "internal_error")
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{"watching": true})
	}
}

// addTaskWatcher silently adds a user as a watcher (for auto-watch on comment/create)
func (s *Server) addTaskWatcher(ctx context.Context, taskID, userID int64) {
	_, err := s.db.ExecContext(ctx, `INSERT INTO task_watchers (task_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, taskID, userID)
	if err != nil {
		s.logger.Debug("Failed to auto-add task watcher", zap.Error(err))
	}
}

// getTaskWatcherIDs returns all user IDs watching a task (excluding a given user)
func (s *Server) getTaskWatcherIDs(ctx context.Context, taskID, excludeUserID int64) []int64 {
	rows, err := s.db.QueryContext(ctx, `SELECT user_id FROM task_watchers WHERE task_id = $1 AND user_id != $2`, taskID, excludeUserID)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// HandleListWikiPageWatchers returns all watchers for a wiki page
func (s *Server) HandleListWikiPageWatchers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}

	// Verify project access via page
	var projectID int64
	err = s.db.QueryRowContext(ctx, `SELECT project_id FROM wiki_pages WHERE id = $1`, pageID).Scan(&projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "page not found", "not_found")
		return
	}
	hasAccess, _ := s.checkProjectAccess(ctx, userID, projectID)
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT wpw.user_id, COALESCE(u.first_name || ' ' || u.last_name, u.name, u.email) AS user_name, wpw.created_at
		FROM wiki_page_watchers wpw
		JOIN users u ON u.id = wpw.user_id
		WHERE wpw.page_id = $1
		ORDER BY wpw.created_at
	`, pageID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch watchers", "internal_error")
		return
	}
	defer rows.Close()

	watchers := []TaskWatcher{}
	for rows.Next() {
		var wt TaskWatcher
		var name string
		if rows.Scan(&wt.UserID, &name, &wt.CreatedAt) == nil {
			wt.UserName = &name
			watchers = append(watchers, wt)
		}
	}

	respondJSON(w, http.StatusOK, watchers)
}

// HandleToggleWikiPageWatcher adds or removes the current user as a watcher
func (s *Server) HandleToggleWikiPageWatcher(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}

	var projectID int64
	err = s.db.QueryRowContext(ctx, `SELECT project_id FROM wiki_pages WHERE id = $1`, pageID).Scan(&projectID)
	if err != nil {
		respondError(w, http.StatusNotFound, "page not found", "not_found")
		return
	}
	hasAccess, _ := s.checkProjectAccess(ctx, userID, projectID)
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	var exists int
	err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wiki_page_watchers WHERE page_id = $1 AND user_id = $2`, pageID, userID).Scan(&exists)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to check watcher status", "internal_error")
		return
	}

	if exists > 0 {
		_, err = s.db.ExecContext(ctx, `DELETE FROM wiki_page_watchers WHERE page_id = $1 AND user_id = $2`, pageID, userID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to unwatch", "internal_error")
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{"watching": false})
	} else {
		_, err = s.db.ExecContext(ctx, `INSERT INTO wiki_page_watchers (page_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, pageID, userID)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to watch", "internal_error")
			return
		}
		respondJSON(w, http.StatusOK, map[string]interface{}{"watching": true})
	}
}

// addWikiPageWatcher silently adds a user as a watcher
func (s *Server) addWikiPageWatcher(ctx context.Context, pageID, userID int64) {
	_, err := s.db.ExecContext(ctx, `INSERT INTO wiki_page_watchers (page_id, user_id) VALUES ($1, $2) ON CONFLICT DO NOTHING`, pageID, userID)
	if err != nil {
		s.logger.Debug("Failed to auto-add wiki page watcher", zap.Error(err))
	}
}

// HandleGetWatchStatus returns whether the current user is watching a task or wiki page
func (s *Server) HandleGetWatchStatus(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	var req struct {
		EntityType string `json:"entity_type"` // "task" or "wiki_page"
		EntityID   int64  `json:"entity_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request", "invalid_input")
		return
	}

	var watching int
	var err error
	switch req.EntityType {
	case "task":
		err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM task_watchers WHERE task_id = $1 AND user_id = $2`, req.EntityID, userID).Scan(&watching)
	case "wiki_page":
		err = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM wiki_page_watchers WHERE page_id = $1 AND user_id = $2`, req.EntityID, userID).Scan(&watching)
	default:
		respondError(w, http.StatusBadRequest, "invalid entity_type", "invalid_input")
		return
	}

	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to check watch status", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{"watching": watching > 0})
}
