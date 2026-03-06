package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"

	"taskai/ent"
	"taskai/ent/user"
	"taskai/ent/useractivity"
	"taskai/internal/auth"
)

// UserWithStats represents a user with admin and activity stats
type UserWithStats struct {
	ID             int64     `json:"id"`
	Email          string    `json:"email"`
	Name           string    `json:"name,omitempty"`
	FirstName      string    `json:"first_name,omitempty"`
	LastName       string    `json:"last_name,omitempty"`
	IsAdmin        bool      `json:"is_admin"`
	CreatedAt      time.Time `json:"created_at"`
	LoginCount     int       `json:"login_count"`
	LastLoginAt    *string   `json:"last_login_at"`
	LastLoginIP    *string   `json:"last_login_ip"`
	FailedAttempts int       `json:"failed_attempts"`
	InviteCount    int       `json:"invite_count"`
}

// UserActivity represents a user activity log entry
type UserActivity struct {
	ID           int64   `json:"id"`
	UserID       int64   `json:"user_id"`
	ActivityType string  `json:"activity_type"` // 'login', 'logout', 'failed_login'
	IPAddress    *string `json:"ip_address"`
	UserAgent    *string `json:"user_agent"`
	CreatedAt    string  `json:"created_at"`
}

// HandleGetUsers returns all users (admin only)
func (s *Server) HandleGetUsers(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}

	// Check if user is admin
	if !s.isAdmin(r.Context(), userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Get all non-deleted users
	deletedRows, _ := s.db.QueryContext(ctx, `SELECT id FROM users WHERE deleted_at IS NOT NULL`)
	deletedIDs := map[int64]bool{}
	if deletedRows != nil {
		for deletedRows.Next() {
			var id int64
			deletedRows.Scan(&id) //nolint:errcheck
			deletedIDs[id] = true
		}
		deletedRows.Close()
	}

	entUsers, err := s.db.Client.User.Query().
		Order(ent.Desc(user.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		s.logger.Error("Failed to query users", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get users", "internal_error")
		return
	}

	users := make([]UserWithStats, 0, len(entUsers))
	for _, u := range entUsers {
		if deletedIDs[u.ID] {
			continue
		}
		userStats := UserWithStats{
			ID:          u.ID,
			Email:       u.Email,
			Name:        userDisplayName(u),
			IsAdmin:     u.IsAdmin,
			CreatedAt:   u.CreatedAt,
			InviteCount: u.InviteCount,
		}
		if u.FirstName != nil {
			userStats.FirstName = *u.FirstName
		}
		if u.LastName != nil {
			userStats.LastName = *u.LastName
		}

		// Get login count and last login
		loginActivities, err := s.db.Client.UserActivity.Query().
			Where(
				useractivity.UserID(u.ID),
				useractivity.ActivityType("login"),
			).
			Order(ent.Desc(useractivity.FieldCreatedAt)).
			Limit(1).
			All(ctx)
		if err == nil {
			// Count all logins
			loginCount, _ := s.db.Client.UserActivity.Query().
				Where(
					useractivity.UserID(u.ID),
					useractivity.ActivityType("login"),
				).
				Count(ctx)
			userStats.LoginCount = loginCount

			// Get last login details
			if len(loginActivities) > 0 {
				lastLogin := loginActivities[0]
				lastLoginStr := lastLogin.CreatedAt.Format(time.RFC3339)
				userStats.LastLoginAt = &lastLoginStr
				userStats.LastLoginIP = lastLogin.IPAddress
			}
		}

		// Get failed login count
		failedCount, err := s.db.Client.UserActivity.Query().
			Where(
				useractivity.UserID(u.ID),
				useractivity.ActivityType("failed_login"),
			).
			Count(ctx)
		if err == nil {
			userStats.FailedAttempts = failedCount
		}

		users = append(users, userStats)
	}

	respondJSON(w, http.StatusOK, users)
}

// HandleGetUserActivity returns activity log for a specific user (admin only)
func (s *Server) HandleGetUserActivity(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}

	// Check if user is admin
	if !s.isAdmin(r.Context(), userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	// Get target user ID from path
	targetUserIDStr := r.PathValue("id")
	if targetUserIDStr == "" {
		respondError(w, http.StatusBadRequest, "user id required", "validation_error")
		return
	}

	var targetUserID int64
	if _, err := fmt.Sscanf(targetUserIDStr, "%d", &targetUserID); err != nil {
		respondError(w, http.StatusBadRequest, "invalid user id", "validation_error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	entActivities, err := s.db.Client.UserActivity.Query().
		Where(useractivity.UserID(targetUserID)).
		Order(ent.Desc(useractivity.FieldCreatedAt)).
		Limit(100).
		All(ctx)
	if err != nil {
		s.logger.Error("Failed to query user activity", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get activity", "internal_error")
		return
	}

	activities := make([]UserActivity, 0, len(entActivities))
	for _, ea := range entActivities {
		activities = append(activities, UserActivity{
			ID:           ea.ID,
			UserID:       ea.UserID,
			ActivityType: ea.ActivityType,
			IPAddress:    ea.IPAddress,
			UserAgent:    ea.UserAgent,
			CreatedAt:    ea.CreatedAt.Format(time.RFC3339),
		})
	}

	respondJSON(w, http.StatusOK, activities)
}

// HandleUpdateUserAdmin updates admin status of a user (admin only)
func (s *Server) HandleUpdateUserAdmin(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}

	// Check if user is admin
	if !s.isAdmin(r.Context(), userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	// Get target user ID from path
	targetUserIDStr := r.PathValue("id")
	if targetUserIDStr == "" {
		respondError(w, http.StatusBadRequest, "user id required", "validation_error")
		return
	}

	var targetUserID int64
	if _, err := fmt.Sscanf(targetUserIDStr, "%d", &targetUserID); err != nil {
		respondError(w, http.StatusBadRequest, "invalid user id", "validation_error")
		return
	}

	var req struct {
		IsAdmin bool `json:"is_admin"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	err := s.db.Client.User.UpdateOneID(targetUserID).
		SetIsAdmin(req.IsAdmin).
		Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "user not found", "not_found")
			return
		}
		s.logger.Error("Failed to update user admin status", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update user", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"id":       targetUserID,
		"is_admin": req.IsAdmin,
	})
}

// HandleDeleteUser soft-deletes a user: anonymizes their email (freeing it for re-invite)
// and marks deleted_at, preserving invite/activity history (admin only).
func (s *Server) HandleDeleteUser(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}

	if !s.isAdmin(r.Context(), userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	targetUserIDStr := r.PathValue("id")
	if targetUserIDStr == "" {
		respondError(w, http.StatusBadRequest, "user id required", "validation_error")
		return
	}

	var targetUserID int64
	if _, err := fmt.Sscanf(targetUserIDStr, "%d", &targetUserID); err != nil {
		respondError(w, http.StatusBadRequest, "invalid user id", "validation_error")
		return
	}

	if targetUserID == userID {
		respondError(w, http.StatusBadRequest, "cannot delete your own account", "validation_error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Check user exists and isn't already deleted
	var existingEmail string
	var deletedAt *time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT email, deleted_at FROM users WHERE id = $1`, targetUserID,
	).Scan(&existingEmail, &deletedAt)
	if err != nil {
		respondError(w, http.StatusNotFound, "user not found", "not_found")
		return
	}
	if deletedAt != nil {
		respondError(w, http.StatusBadRequest, "user is already deleted", "validation_error")
		return
	}

	// Soft delete: anonymize email to free it for re-invite, clear credentials
	anonymizedEmail := fmt.Sprintf("deleted-%d@deleted.invalid", targetUserID)
	now := time.Now().UTC()
	result, err := s.db.ExecContext(ctx,
		`UPDATE users SET email = $1, password_hash = 'DELETED', is_admin = false,
		 totp_secret = NULL, totp_enabled = false, backup_codes = NULL, deleted_at = $2
		 WHERE id = $3`,
		anonymizedEmail, now, targetUserID,
	)
	if err != nil {
		s.logger.Error("Failed to soft-delete user", zap.Error(err), zap.Int64("target_user_id", targetUserID))
		respondError(w, http.StatusInternalServerError, "failed to delete user", "internal_error")
		return
	}
	rows, _ := result.RowsAffected()
	if rows == 0 {
		respondError(w, http.StatusNotFound, "user not found", "not_found")
		return
	}

	s.logger.Info("Admin soft-deleted user", zap.Int64("admin_id", userID), zap.Int64("deleted_user_id", targetUserID))
	respondJSON(w, http.StatusOK, map[string]interface{}{"id": targetUserID, "deleted": true})
}

// isAdmin checks if a user is an admin
func (s *Server) isAdmin(ctx context.Context, userID int64) bool {
	userEntity, err := s.db.Client.User.Get(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to check admin status", zap.Error(err))
		return false
	}
	return userEntity.IsAdmin
}

// logUserActivity logs a user activity event using Ent
func (s *Server) logUserActivity(ctx context.Context, userID int64, activityType, ipAddress, userAgent string) error {
	creator := s.db.Client.UserActivity.Create().
		SetUserID(userID).
		SetActivityType(activityType)

	// Set optional fields only if not empty
	if ipAddress != "" {
		creator.SetIPAddress(ipAddress)
	}
	if userAgent != "" {
		creator.SetUserAgent(userAgent)
	}

	_, err := creator.Save(ctx)
	if err != nil {
		s.logger.Error("Failed to log user activity", zap.Error(err))
	}
	return err
}

// getClientIP extracts the client IP address from the request
func getClientIP(r *http.Request) string {
	// Check X-Forwarded-For header first (for proxies/load balancers)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		// XFF can contain multiple IPs, get the first one
		if idx := strings.Index(xff, ","); idx != -1 {
			return strings.TrimSpace(xff[:idx])
		}
		return strings.TrimSpace(xff)
	}

	// Check X-Real-IP header
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// HandleUpdateUserProfile updates first/last name for any user (admin only)
func (s *Server) HandleUpdateUserProfile(w http.ResponseWriter, r *http.Request) {
	adminID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}
	if !s.isAdmin(r.Context(), adminID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	targetUserIDStr := r.PathValue("id")
	var targetUserID int64
	if _, err := fmt.Sscanf(targetUserIDStr, "%d", &targetUserID); err != nil {
		respondError(w, http.StatusBadRequest, "invalid user id", "validation_error")
		return
	}

	var req struct {
		FirstName string `json:"first_name"`
		LastName  string `json:"last_name"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}
	if len(req.FirstName) > 50 || len(req.LastName) > 50 {
		respondError(w, http.StatusBadRequest, "name fields must be 50 characters or less", "validation_error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	update := s.db.Client.User.UpdateOneID(targetUserID).
		SetFirstName(req.FirstName).
		SetLastName(req.LastName)
	fullName := strings.TrimSpace(req.FirstName + " " + req.LastName)
	if fullName != "" {
		update = update.SetName(fullName)
	} else {
		update = update.ClearName()
	}
	entUser, err := update.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "user not found", "not_found")
			return
		}
		s.logger.Error("Failed to update user profile", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update profile", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, entUserToAPI(entUser))
}

// HandleAdminResetPassword resets a user's password (admin only).
// If send_email is true, generates a reset token and emails it.
// Otherwise, sets the password directly from the provided value.
func (s *Server) HandleAdminResetPassword(w http.ResponseWriter, r *http.Request) {
	adminID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}
	if !s.isAdmin(r.Context(), adminID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	targetUserIDStr := r.PathValue("id")
	var targetUserID int64
	if _, err := fmt.Sscanf(targetUserIDStr, "%d", &targetUserID); err != nil {
		respondError(w, http.StatusBadRequest, "invalid user id", "validation_error")
		return
	}

	var req struct {
		SendEmail bool   `json:"send_email"`
		Password  string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Look up target user
	entUser, err := s.db.Client.User.Get(ctx, targetUserID)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "user not found", "not_found")
			return
		}
		s.logger.Error("Failed to get user", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get user", "internal_error")
		return
	}

	if req.SendEmail {
		// Generate reset token and send email
		tokenBytes := make([]byte, 32)
		if _, err := rand.Read(tokenBytes); err != nil {
			s.logger.Error("Failed to generate reset token", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to generate reset token", "internal_error")
			return
		}
		token := hex.EncodeToString(tokenBytes)
		expiresAt := time.Now().Add(24 * time.Hour) // admin-generated tokens last 24h

		_, err = s.db.ExecContext(ctx,
			`INSERT INTO password_reset_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)`,
			targetUserID, token, expiresAt,
		)
		if err != nil {
			s.logger.Error("Failed to store reset token", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to store token", "internal_error")
			return
		}

		emailSvc := s.GetEmailService()
		if emailSvc == nil {
			respondError(w, http.StatusBadRequest, "email service not configured — set password directly instead", "email_not_configured")
			return
		}
		appURL := s.getAppURL()
		if err := emailSvc.SendPasswordReset(ctx, entUser.Email, token, appURL); err != nil {
			s.logger.Error("Failed to send password reset email", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to send reset email", "email_error")
			return
		}
		s.logger.Info("Admin sent password reset email", zap.Int64("admin_id", adminID), zap.Int64("target_user_id", targetUserID))
		respondJSON(w, http.StatusOK, map[string]string{"message": "Reset email sent to " + entUser.Email})
		return
	}

	// Set password directly
	if req.Password == "" {
		respondError(w, http.StatusBadRequest, "password or send_email required", "validation_error")
		return
	}
	if err := validatePasswordStrength(req.Password); err != nil {
		respondError(w, http.StatusBadRequest, err.Error(), "validation_error")
		return
	}
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		s.logger.Error("Failed to hash password", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to set password", "internal_error")
		return
	}
	if err := s.db.Client.User.UpdateOneID(targetUserID).SetPasswordHash(hashedPassword).Exec(ctx); err != nil {
		s.logger.Error("Failed to update password", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to set password", "internal_error")
		return
	}
	s.logger.Info("Admin set password for user", zap.Int64("admin_id", adminID), zap.Int64("target_user_id", targetUserID))
	respondJSON(w, http.StatusOK, map[string]string{"message": "Password updated successfully"})
}
