package api

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// MemoryShareUser is someone on either side of a share.
type MemoryShareUser struct {
	UserID int64  `json:"user_id"`
	Email  string `json:"email"`
}

func (s *Server) memoryShareUsers(ctx context.Context, query string, userID int64) ([]MemoryShareUser, error) {
	rows, err := s.db.QueryContext(ctx, query, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MemoryShareUser{}
	for rows.Next() {
		var u MemoryShareUser
		if err := rows.Scan(&u.UserID, &u.Email); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Server) respondMemoryShares(w http.ResponseWriter, ctx context.Context, userID int64) {
	with, err := s.memoryShareUsers(ctx, `SELECT u.id, u.email FROM memory_shares ms JOIN users u ON u.id = ms.user_id
		WHERE ms.owner_id = $1 ORDER BY u.email`, userID)
	var me []MemoryShareUser
	if err == nil {
		me, err = s.memoryShareUsers(ctx, `SELECT u.id, u.email FROM memory_shares ms JOIN users u ON u.id = ms.owner_id
			WHERE ms.user_id = $1 ORDER BY u.email`, userID)
	}
	if err != nil {
		s.logger.Error("Failed to list memory shares", zap.Int64("user_id", userID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list shares", "internal_error")
		return
	}
	respondJSON(w, http.StatusOK, map[string]any{"shared_with": with, "shared_with_me": me})
}

// HandleGetMemoryShares lists who the caller shares memories with, and who shares with them (GET /api/memories/shares).
func (s *Server) HandleGetMemoryShares(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	s.respondMemoryShares(w, r.Context(), userID)
}

// HandleSetMemoryShares replaces the list of people who can read the caller's memories (PUT /api/memories/shares).
func (s *Server) HandleSetMemoryShares(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	var req struct {
		UserIDs *[]int64 `json:"user_ids"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}
	if req.UserIDs == nil {
		respondError(w, http.StatusBadRequest, "user_ids is required ([] removes every share)", "validation_error")
		return
	}
	if len(*req.UserIDs) > memoryMaxShares {
		respondError(w, http.StatusBadRequest, fmt.Sprintf("share with at most %d people", memoryMaxShares), "validation_error")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update shares", "internal_error")
		return
	}
	defer tx.Rollback() //nolint:errcheck
	if _, err = tx.ExecContext(ctx, `DELETE FROM memory_shares WHERE owner_id = $1`, userID); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to update shares", "internal_error")
		return
	}
	seen := map[int64]bool{userID: true}
	for _, id := range *req.UserIDs {
		if seen[id] {
			continue
		}
		seen[id] = true
		// Only people the owner already works with (a team or a project in common), so ids cannot be probed for emails.
		var n int
		if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM users u WHERE u.id = $2 AND u.deleted_at IS NULL AND (
			EXISTS (SELECT 1 FROM team_members a JOIN team_members b ON a.team_id = b.team_id
				WHERE a.user_id = $1 AND b.user_id = u.id AND a.status = 'active' AND b.status = 'active')
			OR EXISTS (SELECT 1 FROM project_members a JOIN project_members b ON a.project_id = b.project_id
				WHERE a.user_id = $1 AND b.user_id = u.id))`, userID, id).Scan(&n); err != nil {
			s.logger.Error("Failed to check memory share target", zap.Int64("user_id", userID), zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to update shares", "internal_error")
			return
		}
		if n == 0 {
			respondError(w, http.StatusBadRequest, fmt.Sprintf("user %d is not in a team or project with you", id), "validation_error")
			return
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO memory_shares (owner_id, user_id, granted_by) VALUES ($1, $2, $1)`, userID, id); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to update shares", "internal_error")
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to update memory shares", zap.Int64("user_id", userID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update shares", "internal_error")
		return
	}
	s.respondMemoryShares(w, ctx, userID)
}

// HandleAdminSetMemoryQuota sets a user's memory quota; null restores the default (PUT /api/admin/users/{id}/memory-quota).
func (s *Server) HandleAdminSetMemoryQuota(w http.ResponseWriter, r *http.Request) {
	callerID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}
	if !s.isAdmin(r.Context(), callerID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}
	target, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user id", "validation_error")
		return
	}
	var req struct {
		QuotaBytes *int64 `json:"quota_bytes"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}
	if req.QuotaBytes != nil && (*req.QuotaBytes <= 0 || *req.QuotaBytes > memoryMaxQuota) {
		respondError(w, http.StatusBadRequest, "quota_bytes must be between 1 and 1 TiB, or null for the default", "validation_error")
		return
	}
	ctx := r.Context()
	var n int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE id = $1`, target).Scan(&n); err != nil {
		s.logger.Error("Failed to look up user for memory quota", zap.Int64("user_id", target), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to set quota", "internal_error")
		return
	}
	if n == 0 {
		respondError(w, http.StatusNotFound, "user not found", "not_found")
		return
	}
	if req.QuotaBytes == nil {
		_, err = s.db.ExecContext(ctx, `DELETE FROM memory_quotas WHERE user_id = $1`, target)
	} else {
		_, err = s.db.ExecContext(ctx, `INSERT INTO memory_quotas (user_id, quota_bytes, updated_at) VALUES ($1, $2, $3)
			ON CONFLICT (user_id) DO UPDATE SET quota_bytes = excluded.quota_bytes, updated_at = excluded.updated_at`,
			target, *req.QuotaBytes, time.Now().UTC())
	}
	if err != nil {
		s.logger.Error("Failed to set memory quota", zap.Int64("user_id", target), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to set quota", "internal_error")
		return
	}
	usage, err := s.memoryUsage(ctx, s.db, target)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to read usage", "internal_error")
		return
	}
	respondJSON(w, http.StatusOK, usage)
}
