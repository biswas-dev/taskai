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
	"taskai/ent/invite"
	"taskai/ent/teaminvitation"
	"taskai/ent/user"
	"taskai/internal/auth"
)

// SignupRequest represents the signup request payload
type SignupRequest struct {
	Email      string `json:"email"`
	Password   string `json:"password"`
	InviteCode string `json:"invite_code"`
	FirstName  string `json:"first_name"`
	LastName   string `json:"last_name"`
}

// LoginRequest represents the login request payload
type LoginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

// AuthResponse represents the authentication response
type AuthResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

// User represents a user
type User struct {
	ID          int64     `json:"id"`
	Email       string    `json:"email"`
	Name        string    `json:"name,omitempty"`
	FirstName   string    `json:"first_name,omitempty"`
	LastName    string    `json:"last_name,omitempty"`
	IsAdmin     bool      `json:"is_admin"`
	HasPassword bool      `json:"has_password"`
	CreatedAt   time.Time `json:"created_at"`
}

// HandleSignup creates a new user account
func (s *Server) HandleSignup(w http.ResponseWriter, r *http.Request) {
	var req SignupRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}

	// Validate input
	if err := validateSignupRequest(req); err != nil {
		respondError(w, http.StatusBadRequest, err.Error(), "validation_error")
		return
	}

	// Validate invite code
	if req.InviteCode == "" {
		respondError(w, http.StatusBadRequest, "invite code is required — you need a referral to create an account", "invite_required")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	// Verify invite code is valid using Ent
	inv, err := s.db.Client.Invite.Query().
		Where(invite.Code(req.InviteCode)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusBadRequest, "invalid invite code", "invalid_invite")
			return
		}
		s.logger.Error("Failed to validate invite code", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create user", "internal_error")
		return
	}
	if inv.UsedAt != nil {
		respondError(w, http.StatusBadRequest, "this invite has already been used", "invite_used")
		return
	}
	if inv.ExpiresAt != nil && time.Now().After(*inv.ExpiresAt) {
		respondError(w, http.StatusBadRequest, "this invite has expired", "invite_expired")
		return
	}

	// Hash password
	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		s.logger.Error("Failed to hash password", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create user", "internal_error")
		return
	}

	// Use Ent transaction
	tx, err := s.db.Client.Tx(ctx)
	if err != nil {
		s.logger.Error("Failed to begin transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create user", "internal_error")
		return
	}
	defer tx.Rollback()

	// Create user using Ent
	userCreate := tx.User.Create().
		SetEmail(req.Email).
		SetPasswordHash(hashedPassword)
	if req.FirstName != "" {
		userCreate = userCreate.SetFirstName(req.FirstName)
	}
	if req.LastName != "" {
		userCreate = userCreate.SetLastName(req.LastName)
	}
	// Also set legacy name field for backward compat
	fullName := strings.TrimSpace(req.FirstName + " " + req.LastName)
	if fullName != "" {
		userCreate = userCreate.SetName(fullName)
	}
	newUser, err := userCreate.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			respondError(w, http.StatusConflict, "email already exists", "email_exists")
			return
		}
		s.logger.Error("Failed to create user", zap.Error(err), zap.String("email", req.Email))
		respondError(w, http.StatusInternalServerError, "failed to create user", "internal_error")
		return
	}

	// Create team for the user
	teamName := userDisplayName(newUser) + "'s Team"

	team, err := tx.Team.Create().
		SetName(teamName).
		SetOwnerID(newUser.ID).
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to create team", zap.Error(err), zap.Int64("user_id", newUser.ID))
		respondError(w, http.StatusInternalServerError, "failed to create team", "internal_error")
		return
	}

	// Add user to team as owner
	_, err = tx.TeamMember.Create().
		SetTeamID(team.ID).
		SetUserID(newUser.ID).
		SetRole("owner").
		SetStatus("active").
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to add user to team", zap.Error(err), zap.Int64("user_id", newUser.ID))
		respondError(w, http.StatusInternalServerError, "failed to add user to team", "internal_error")
		return
	}

	// Mark invite as used
	now := time.Now()
	_, err = tx.Invite.UpdateOneID(inv.ID).
		SetInviteeID(newUser.ID).
		SetUsedAt(now).
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to mark invite as used", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create user", "internal_error")
		return
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to commit transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create user", "internal_error")
		return
	}

	s.logger.Info("User and team created",
		zap.Int64("user_id", newUser.ID),
		zap.Int64("team_id", team.ID),
		zap.String("email", newUser.Email),
	)

	// Auto-accept any pending team invitation linked to this invite code
	s.acceptPendingTeamInvitation(ctx, req.InviteCode, newUser.ID)

	// Convert Ent user to API user struct
	apiUser := entUserToAPI(newUser)

	// Generate JWT token
	token, err := auth.GenerateToken(apiUser.ID, apiUser.Email, s.config.JWTSecret, s.config.JWTExpiry())
	if err != nil {
		s.logger.Error("Failed to generate token", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to generate token", "internal_error")
		return
	}

	respondJSON(w, http.StatusCreated, AuthResponse{
		Token: token,
		User:  apiUser,
	})
}

// HandleLogin authenticates a user and returns a JWT token
func (s *Server) HandleLogin(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}

	// Validate input
	if req.Email == "" || req.Password == "" {
		respondError(w, http.StatusBadRequest, "email and password are required", "validation_error")
		return
	}

	// Get user from database using Ent
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	entUser, err := s.db.Client.User.Query().
		Where(user.Email(req.Email)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			// Log failed login attempt
			respondError(w, http.StatusUnauthorized, "invalid email or password", "invalid_credentials")
			return
		}
		s.logger.Error("Failed to query user", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to authenticate", "internal_error")
		return
	}

	passwordHash := entUser.PasswordHash

	// Verify password
	if err := auth.VerifyPassword(passwordHash, req.Password); err != nil {
		// Log failed login attempt
		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			s.logUserActivity(ctx, entUser.ID, "failed_login", getClientIP(r), r.UserAgent())
		}()
		respondError(w, http.StatusUnauthorized, "invalid email or password", "invalid_credentials")
		return
	}

	// Log successful login
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		s.logUserActivity(ctx, entUser.ID, "login", getClientIP(r), r.UserAgent())
	}()

	// Convert Ent user to API user struct
	apiUser := entUserToAPI(entUser)

	// Generate JWT token
	token, err := auth.GenerateToken(apiUser.ID, apiUser.Email, s.config.JWTSecret, s.config.JWTExpiry())
	if err != nil {
		s.logger.Error("Failed to generate token", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to generate token", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, AuthResponse{
		Token: token,
		User:  apiUser,
	})
}

// HandleMe returns the current authenticated user
func (s *Server) HandleMe(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}

	// Get user from database using Ent
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	entUser, err := s.db.Client.User.Get(ctx, userID)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "user not found", "not_found")
			return
		}
		s.logger.Error("Failed to query user", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get user", "internal_error")
		return
	}

	apiUser := entUserToAPI(entUser)

	// Determine whether the user has a real password (auth_provider = "password").
	var authProvider string
	if err := s.db.QueryRowContext(ctx,
		s.db.Rebind(`SELECT auth_provider FROM users WHERE id = ? LIMIT 1`), userID,
	).Scan(&authProvider); err != nil {
		authProvider = "password" // safe default
	}
	apiUser.HasPassword = authProvider == "password"

	respondJSON(w, http.StatusOK, apiUser)
}

// UpdateProfileRequest represents the update profile request
type UpdateProfileRequest struct {
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
}

// HandleUpdateProfile updates the current user's profile
func (s *Server) HandleUpdateProfile(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}

	var req UpdateProfileRequest
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}

	// Validate names
	if len(req.FirstName) > 50 {
		respondError(w, http.StatusBadRequest, "first name must be 50 characters or less", "validation_error")
		return
	}
	if len(req.LastName) > 50 {
		respondError(w, http.StatusBadRequest, "last name must be 50 characters or less", "validation_error")
		return
	}

	// Update user using Ent
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	update := s.db.Client.User.UpdateOneID(userID).
		SetFirstName(req.FirstName).
		SetLastName(req.LastName)
	// Also update legacy name field
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

// validateSignupRequest validates the signup request
func validateSignupRequest(req SignupRequest) error {
	// Validate email
	if req.Email == "" {
		return fmt.Errorf("email is required")
	}
	if !strings.Contains(req.Email, "@") || !strings.Contains(req.Email, ".") {
		return fmt.Errorf("invalid email format")
	}

	// Validate password strength
	if err := validatePasswordStrength(req.Password); err != nil {
		return err
	}

	return nil
}

// validatePasswordStrength ensures password meets security requirements
func validatePasswordStrength(password string) error {
	if password == "" {
		return fmt.Errorf("password is required")
	}

	if len(password) < 8 {
		return fmt.Errorf("password must be at least 8 characters")
	}

	// Check for at least one digit
	hasDigit := false
	for _, ch := range password {
		if ch >= '0' && ch <= '9' {
			hasDigit = true
			break
		}
	}
	if !hasDigit {
		return fmt.Errorf("password must contain at least one digit")
	}

	// Check for at least one letter (uppercase or lowercase)
	hasLetter := false
	for _, ch := range password {
		if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') {
			hasLetter = true
			break
		}
	}
	if !hasLetter {
		return fmt.Errorf("password must contain at least one letter")
	}

	return nil
}

// userDisplayName builds "First Last" from an Ent user, falling back to name then email.
func userDisplayName(u *ent.User) string {
	first := ""
	last := ""
	if u.FirstName != nil {
		first = *u.FirstName
	}
	if u.LastName != nil {
		last = *u.LastName
	}
	full := strings.TrimSpace(first + " " + last)
	if full != "" {
		return full
	}
	if u.Name != nil && *u.Name != "" {
		return *u.Name
	}
	return u.Email
}

// userDisplayNamePtr returns a pointer to the display name, or nil if empty.
func userDisplayNamePtr(u *ent.User) *string {
	first := ""
	last := ""
	if u.FirstName != nil {
		first = *u.FirstName
	}
	if u.LastName != nil {
		last = *u.LastName
	}
	full := strings.TrimSpace(first + " " + last)
	if full != "" {
		return &full
	}
	if u.Name != nil && *u.Name != "" {
		return u.Name
	}
	return nil
}

// entUserToAPI converts an Ent user to the API User struct.
func entUserToAPI(u *ent.User) User {
	apiUser := User{
		ID:        u.ID,
		Email:     u.Email,
		IsAdmin:   u.IsAdmin,
		CreatedAt: u.CreatedAt,
	}
	if u.FirstName != nil {
		apiUser.FirstName = *u.FirstName
	}
	if u.LastName != nil {
		apiUser.LastName = *u.LastName
	}
	// Compute Name from first_name + last_name, fallback to name field
	full := strings.TrimSpace(apiUser.FirstName + " " + apiUser.LastName)
	if full != "" {
		apiUser.Name = full
	} else if u.Name != nil {
		apiUser.Name = *u.Name
	}
	return apiUser
}

// acceptPendingTeamInvitation looks up a TeamInvitation by its invite_code and, if pending, accepts it
// and adds the new user to the team. Errors are logged but do not fail the signup.
func (s *Server) acceptPendingTeamInvitation(ctx context.Context, inviteCode string, newUserID int64) {
	entInv, err := s.db.Client.TeamInvitation.Query().
		Where(
			teaminvitation.InviteCode(inviteCode),
			teaminvitation.Status("pending"),
		).
		Only(ctx)
	if err != nil {
		// Not found or already accepted — nothing to do
		return
	}

	now := time.Now()
	tx, err := s.db.Client.Tx(ctx)
	if err != nil {
		s.logger.Error("acceptPendingTeamInvitation: begin tx failed", zap.Error(err))
		return
	}
	defer tx.Rollback()

	_, err = tx.TeamInvitation.UpdateOneID(entInv.ID).
		SetStatus("accepted").
		SetInviteeID(newUserID).
		SetRespondedAt(now).
		Save(ctx)
	if err != nil {
		s.logger.Error("acceptPendingTeamInvitation: update invitation failed", zap.Error(err))
		return
	}

	_, err = tx.TeamMember.Create().
		SetTeamID(entInv.TeamID).
		SetUserID(newUserID).
		SetRole("member").
		SetStatus("active").
		Save(ctx)
	if err != nil {
		s.logger.Error("acceptPendingTeamInvitation: create team member failed", zap.Error(err))
		return
	}

	if err := tx.Commit(); err != nil {
		s.logger.Error("acceptPendingTeamInvitation: commit failed", zap.Error(err))
		return
	}

	s.logger.Info("Team invitation auto-accepted on signup",
		zap.Int64("invitation_id", entInv.ID),
		zap.Int64("team_id", entInv.TeamID),
		zap.Int64("new_user_id", newUserID),
	)
}

// HandleForgotPassword generates a one-time password reset token and sends a reset email.
// Always returns 200 to prevent email enumeration.
func (s *Server) HandleForgotPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Email string `json:"email"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	if req.Email == "" {
		respondError(w, http.StatusBadRequest, "email is required", "validation_error")
		return
	}

	ok200 := map[string]string{"message": "If that email is registered, a reset link has been sent"}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	entUser, err := s.db.Client.User.Query().Where(user.Email(req.Email)).Only(ctx)
	if err != nil {
		// Not found or error — respond 200 to prevent enumeration
		respondJSON(w, http.StatusOK, ok200)
		return
	}

	// Generate 32-byte random token (hex encoded = 64 chars)
	tokenBytes := make([]byte, 32)
	if _, err := rand.Read(tokenBytes); err != nil {
		s.logger.Error("Failed to generate reset token", zap.Error(err))
		respondJSON(w, http.StatusOK, ok200)
		return
	}
	token := hex.EncodeToString(tokenBytes)
	expiresAt := time.Now().Add(1 * time.Hour)

	_, err = s.db.ExecContext(ctx,
		`INSERT INTO password_reset_tokens (user_id, token, expires_at) VALUES ($1, $2, $3)`,
		entUser.ID, token, expiresAt,
	)
	if err != nil {
		s.logger.Error("Failed to store reset token", zap.Error(err))
		respondJSON(w, http.StatusOK, ok200)
		return
	}

	if emailSvc := s.GetEmailService(); emailSvc != nil {
		appURL := s.getAppURL()
		go func() {
			ctx2, cancel2 := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel2()
			if err := emailSvc.SendPasswordReset(ctx2, entUser.Email, token, appURL); err != nil {
				s.logger.Error("Failed to send password reset email", zap.Error(err), zap.String("email", entUser.Email))
			}
		}()
	}

	respondJSON(w, http.StatusOK, ok200)
}

// HandleResetPassword validates a reset token and sets the new password.
func (s *Server) HandleResetPassword(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Token    string `json:"token"`
		Password string `json:"password"`
	}
	if err := decodeJSON(r, &req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}
	if req.Token == "" {
		respondError(w, http.StatusBadRequest, "token is required", "validation_error")
		return
	}
	if err := validatePasswordStrength(req.Password); err != nil {
		respondError(w, http.StatusBadRequest, err.Error(), "validation_error")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var userID int64
	var expiresAt time.Time
	var usedAt *time.Time
	err := s.db.QueryRowContext(ctx,
		`SELECT user_id, expires_at, used_at FROM password_reset_tokens WHERE token = $1`,
		req.Token,
	).Scan(&userID, &expiresAt, &usedAt)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid or expired reset link", "invalid_token")
		return
	}
	if usedAt != nil {
		respondError(w, http.StatusBadRequest, "this reset link has already been used", "token_used")
		return
	}
	if time.Now().After(expiresAt) {
		respondError(w, http.StatusBadRequest, "this reset link has expired — please request a new one", "token_expired")
		return
	}

	hashedPassword, err := auth.HashPassword(req.Password)
	if err != nil {
		s.logger.Error("Failed to hash password", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to reset password", "internal_error")
		return
	}

	if err := s.db.Client.User.UpdateOneID(userID).SetPasswordHash(hashedPassword).Exec(ctx); err != nil {
		s.logger.Error("Failed to update password", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to reset password", "internal_error")
		return
	}

	// Mark token as used
	now := time.Now()
	_, _ = s.db.ExecContext(ctx, `UPDATE password_reset_tokens SET used_at = $1 WHERE token = $2`, now, req.Token)

	s.logger.Info("Password reset completed", zap.Int64("user_id", userID))
	respondJSON(w, http.StatusOK, map[string]string{"message": "Password reset successfully"})
}
