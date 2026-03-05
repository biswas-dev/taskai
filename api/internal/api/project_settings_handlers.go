package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// ProjectMember represents a user with access to a project
type ProjectMember struct {
	ID        int       `json:"id"`
	ProjectID int       `json:"project_id"`
	UserID    int       `json:"user_id"`
	Email     string    `json:"email"`
	Name      *string   `json:"name,omitempty"`
	Role      string    `json:"role"`
	GrantedBy int       `json:"granted_by"`
	GrantedAt time.Time `json:"granted_at"`
}

// ProjectGitHubSettings represents GitHub integration settings
type ProjectGitHubSettings struct {
	RepoURL      string     `json:"github_repo_url"`
	Owner        string     `json:"github_owner"`
	RepoName     string     `json:"github_repo_name"`
	Branch       string     `json:"github_branch"`
	SyncEnabled  bool       `json:"github_sync_enabled"`
	PushEnabled  bool       `json:"github_push_enabled"`
	LastSync     *time.Time `json:"github_last_sync"`
	TokenSet     bool       `json:"github_token_set"`
	Login        *string    `json:"github_login"`
	ProjectURL   string     `json:"github_project_url"`   // optional explicit GitHub Projects V2 URL
	SyncInterval string     `json:"github_sync_interval"` // 'daily','weekly','monthly', '' = disabled
	SyncHour     int        `json:"github_sync_hour"`     // 0-23
	SyncDay      int        `json:"github_sync_day"`      // weekly: 0-6 (Sun=0), monthly: 1-28
}

// AddMemberRequest represents a request to add a member to a project
type AddMemberRequest struct {
	Email string `json:"email"`
	Role  string `json:"role"`
}

// UpdateMemberRoleRequest represents a request to update a member's role
type UpdateMemberRoleRequest struct {
	Role string `json:"role"`
}

// UpdateProjectGitHubRequest represents a request to update GitHub settings
type UpdateProjectGitHubRequest struct {
	RepoURL      string `json:"github_repo_url"`
	Owner        string `json:"github_owner"`
	RepoName     string `json:"github_repo_name"`
	Branch       string `json:"github_branch"`
	SyncEnabled  bool   `json:"github_sync_enabled"`
	PushEnabled  bool   `json:"github_push_enabled"`
	Token        string `json:"github_token"`
	ProjectURL   string `json:"github_project_url"`   // optional explicit GitHub Projects V2 URL
	SyncInterval string `json:"github_sync_interval"` // 'daily','weekly','monthly', '' = disabled
	SyncHour     int    `json:"github_sync_hour"`     // 0-23
	SyncDay      int    `json:"github_sync_day"`      // weekly: 0-6 (Sun=0), monthly: 1-28
}

// HandleGetProjectMembers returns all members of a project
func (s *Server) HandleGetProjectMembers(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if user has access to this project
	hasAccess, err := s.userHasProjectAccess(int(userID), projectID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !hasAccess {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	query := `
		SELECT pm.id, pm.project_id, pm.user_id, u.email,
		       COALESCE(NULLIF(TRIM(COALESCE(u.first_name,'') || ' ' || COALESCE(u.last_name,'')), ''), u.name) as name,
		       pm.role, pm.granted_by, pm.granted_at
		FROM project_members pm
		JOIN users u ON pm.user_id = u.id
		WHERE pm.project_id = $1
		ORDER BY pm.role DESC, pm.granted_at ASC
	`

	rows, err := s.db.Query(query, projectID)
	if err != nil {
		http.Error(w, "Failed to fetch members", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	members := []ProjectMember{}
	for rows.Next() {
		var m ProjectMember
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.Email, &m.Name, &m.Role, &m.GrantedBy, &m.GrantedAt); err != nil {
			http.Error(w, "Failed to scan member", http.StatusInternalServerError)
			return
		}
		members = append(members, m)
	}

	respondJSON(w, http.StatusOK, members)
}

// HandleAddProjectMember adds a member to a project
func (s *Server) HandleAddProjectMember(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if user is owner or admin of the project
	isOwnerOrAdmin, err := s.userIsProjectOwnerOrAdmin(int(userID), projectID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !isOwnerOrAdmin {
		http.Error(w, "Forbidden - only project owners and admins can add members", http.StatusForbidden)
		return
	}

	var req AddMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate role
	if req.Role != "viewer" && req.Role != "member" && req.Role != "editor" && req.Role != "owner" {
		http.Error(w, "Invalid role. Must be viewer, member, editor, or owner", http.StatusBadRequest)
		return
	}

	// Find user by email
	var memberUserID int
	err = s.db.QueryRow("SELECT id FROM users WHERE email = $1", req.Email).Scan(&memberUserID)
	if err == sql.ErrNoRows {
		http.Error(w, "User not found. Only registered users can be added.", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Failed to find user", http.StatusInternalServerError)
		return
	}

	// Check if user is the owner (can't add owner as member)
	var ownerID int
	err = s.db.QueryRow("SELECT owner_id FROM projects WHERE id = $1", projectID).Scan(&ownerID)
	if err != nil {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}
	if memberUserID == ownerID {
		http.Error(w, "Cannot add project owner as a member", http.StatusBadRequest)
		return
	}

	// Check if user is in the same team
	var teamID int
	err = s.db.QueryRow("SELECT team_id FROM projects WHERE id = $1", projectID).Scan(&teamID)
	if err != nil {
		http.Error(w, "Failed to get project team", http.StatusInternalServerError)
		return
	}

	var memberInTeam bool
	err = s.db.QueryRow(`
		SELECT EXISTS(
			SELECT 1 FROM team_members
			WHERE team_id = $1 AND user_id = $2 AND status = 'active'
		)
	`, teamID, memberUserID).Scan(&memberInTeam)
	if err != nil {
		http.Error(w, "Failed to check team membership", http.StatusInternalServerError)
		return
	}
	if !memberInTeam {
		http.Error(w, "User must be a member of the team to access this project", http.StatusBadRequest)
		return
	}

	// Insert member
	var memberID int64
	err = s.db.QueryRow(`
		INSERT INTO project_members (project_id, user_id, role, granted_by, granted_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		RETURNING id
	`, projectID, memberUserID, req.Role, userID).Scan(&memberID)

	if err != nil {
		if strings.Contains(err.Error(), "unique constraint") || strings.Contains(err.Error(), "UNIQUE constraint") {
			http.Error(w, "User is already a member of this project", http.StatusConflict)
			return
		}
		http.Error(w, "Failed to add member", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"message":   "Member added successfully",
		"member_id": memberID,
	})
}

// HandleUpdateProjectMember updates a member's role
func (s *Server) HandleUpdateProjectMember(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	memberID, err := strconv.Atoi(chi.URLParam(r, "memberId"))
	if err != nil {
		http.Error(w, "Invalid member ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if user is owner or admin
	isOwnerOrAdmin, err := s.userIsProjectOwnerOrAdmin(int(userID), projectID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !isOwnerOrAdmin {
		http.Error(w, "Forbidden - only project owners and admins can update member roles", http.StatusForbidden)
		return
	}

	var req UpdateMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Validate role
	if req.Role != "viewer" && req.Role != "member" && req.Role != "editor" && req.Role != "owner" {
		http.Error(w, "Invalid role. Must be viewer, member, editor, or owner", http.StatusBadRequest)
		return
	}

	// Check if changing from owner to another role
	var currentRole string
	err = s.db.QueryRow(`SELECT role FROM project_members WHERE id = $1 AND project_id = $2`, memberID, projectID).Scan(&currentRole)
	if err != nil {
		http.Error(w, "Member not found", http.StatusNotFound)
		return
	}

	// If changing from owner, ensure at least one other owner exists
	if currentRole == "owner" && req.Role != "owner" {
		var ownerCount int
		err = s.db.QueryRow(`SELECT COUNT(*) FROM project_members WHERE project_id = $1 AND role = 'owner'`, projectID).Scan(&ownerCount)
		if err != nil {
			http.Error(w, "Failed to check owner count", http.StatusInternalServerError)
			return
		}
		if ownerCount <= 1 {
			http.Error(w, "Cannot change role - project must have at least one owner", http.StatusBadRequest)
			return
		}
	}

	// Update member role
	_, err = s.db.Exec(`
		UPDATE project_members
		SET role = $1
		WHERE id = $2 AND project_id = $3
	`, req.Role, memberID, projectID)

	if err != nil {
		http.Error(w, "Failed to update member role", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Member role updated successfully"})
}

// HandleRemoveProjectMember removes a member from a project
func (s *Server) HandleRemoveProjectMember(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	memberID, err := strconv.Atoi(chi.URLParam(r, "memberId"))
	if err != nil {
		http.Error(w, "Invalid member ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if user is owner or admin
	isOwnerOrAdmin, err := s.userIsProjectOwnerOrAdmin(int(userID), projectID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !isOwnerOrAdmin {
		http.Error(w, "Forbidden - only project owners and admins can remove members", http.StatusForbidden)
		return
	}

	// Check if member being removed is an owner
	var memberRole string
	err = s.db.QueryRow(`SELECT role FROM project_members WHERE id = $1 AND project_id = $2`, memberID, projectID).Scan(&memberRole)
	if err != nil {
		http.Error(w, "Member not found", http.StatusNotFound)
		return
	}

	// If removing an owner, ensure at least one other owner exists
	if memberRole == "owner" {
		var ownerCount int
		err = s.db.QueryRow(`SELECT COUNT(*) FROM project_members WHERE project_id = $1 AND role = 'owner'`, projectID).Scan(&ownerCount)
		if err != nil {
			http.Error(w, "Failed to check owner count", http.StatusInternalServerError)
			return
		}
		if ownerCount <= 1 {
			http.Error(w, "Cannot remove the last owner - project must have at least one owner", http.StatusBadRequest)
			return
		}
	}

	// Delete member
	_, err = s.db.Exec(`
		DELETE FROM project_members
		WHERE id = $1 AND project_id = $2
	`, memberID, projectID)

	if err != nil {
		http.Error(w, "Failed to remove member", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Member removed successfully"})
}

// HandleGetProjectGitHubSettings returns GitHub settings for a project
func (s *Server) HandleGetProjectGitHubSettings(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if user has access
	hasAccess, err := s.userHasProjectAccess(int(userID), projectID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !hasAccess {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var settings ProjectGitHubSettings
	var lastSync sql.NullTime
	var token sql.NullString
	var loginNull sql.NullString
	var projectURLNull sql.NullString
	var syncIntervalNull sql.NullString

	err = s.db.QueryRow(`
		SELECT
			COALESCE(github_repo_url, ''),
			COALESCE(github_owner, ''),
			COALESCE(github_repo_name, ''),
			COALESCE(github_branch, 'main'),
			github_sync_enabled,
			github_push_enabled,
			github_last_sync,
			github_token,
			github_login,
			github_project_url,
			COALESCE(github_sync_interval, ''),
			COALESCE(github_sync_hour, 0),
			COALESCE(github_sync_day, 0)
		FROM projects
		WHERE id = $1
	`, projectID).Scan(
		&settings.RepoURL,
		&settings.Owner,
		&settings.RepoName,
		&settings.Branch,
		&settings.SyncEnabled,
		&settings.PushEnabled,
		&lastSync,
		&token,
		&loginNull,
		&projectURLNull,
		&syncIntervalNull,
		&settings.SyncHour,
		&settings.SyncDay,
	)

	if err != nil {
		s.logger.Error("Failed to fetch GitHub settings", zap.Int("project_id", projectID), zap.Error(err))
		http.Error(w, "Failed to fetch GitHub settings", http.StatusInternalServerError)
		return
	}

	if lastSync.Valid {
		settings.LastSync = &lastSync.Time
	}
	settings.TokenSet = token.Valid && token.String != ""
	if loginNull.Valid && loginNull.String != "" {
		settings.Login = &loginNull.String
	}
	if projectURLNull.Valid {
		settings.ProjectURL = projectURLNull.String
	}
	settings.SyncInterval = syncIntervalNull.String

	respondJSON(w, http.StatusOK, settings)
}

// HandleUpdateProjectGitHubSettings updates GitHub settings for a project
func (s *Server) HandleUpdateProjectGitHubSettings(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Check if user is owner or admin
	isOwnerOrAdmin, err := s.userIsProjectOwnerOrAdmin(int(userID), projectID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !isOwnerOrAdmin {
		http.Error(w, "Forbidden - only project owners and admins can update GitHub settings", http.StatusForbidden)
		return
	}

	var req UpdateProjectGitHubRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}
	req.ProjectURL = strings.TrimSpace(req.ProjectURL)

	if req.Token != "" {
		_, err = s.db.Exec(`
			UPDATE projects
			SET
				github_repo_url = $1,
				github_owner = $2,
				github_repo_name = $3,
				github_branch = $4,
				github_sync_enabled = $5,
				github_push_enabled = $6,
				github_token = $7,
				github_project_url = NULLIF($8, ''),
				github_sync_interval = NULLIF($9, ''),
				github_sync_hour = $10,
				github_sync_day = $11
			WHERE id = $12
		`, req.RepoURL, req.Owner, req.RepoName, req.Branch, req.SyncEnabled, req.PushEnabled, req.Token, req.ProjectURL, req.SyncInterval, req.SyncHour, req.SyncDay, projectID)
	} else {
		_, err = s.db.Exec(`
			UPDATE projects
			SET
				github_repo_url = $1,
				github_owner = $2,
				github_repo_name = $3,
				github_branch = $4,
				github_sync_enabled = $5,
				github_push_enabled = $6,
				github_project_url = NULLIF($7, ''),
				github_sync_interval = NULLIF($8, ''),
				github_sync_hour = $9,
				github_sync_day = $10
			WHERE id = $11
		`, req.RepoURL, req.Owner, req.RepoName, req.Branch, req.SyncEnabled, req.PushEnabled, req.ProjectURL, req.SyncInterval, req.SyncHour, req.SyncDay, projectID)
	}

	if err != nil {
		s.logger.Error("Failed to update GitHub settings", zap.Int("project_id", projectID), zap.Error(err))
		http.Error(w, "Failed to update GitHub settings", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "GitHub settings updated successfully"})
}

// Helper function to check if user has access to a project
func (s *Server) userHasProjectAccess(userID, projectID int) (bool, error) {
	// Check if user is owner
	var ownerID int
	err := s.db.QueryRow("SELECT owner_id FROM projects WHERE id = $1", projectID).Scan(&ownerID)
	if err != nil {
		return false, err
	}
	if ownerID == userID {
		return true, nil
	}

	// Check if user is a member
	var memberID int
	err = s.db.QueryRow("SELECT id FROM project_members WHERE project_id = $1 AND user_id = $2", projectID, userID).Scan(&memberID)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return true, nil
}

// Helper function to check if user is owner or admin of a project
func (s *Server) userIsProjectOwnerOrAdmin(userID, projectID int) (bool, error) {
	// Check if user is owner
	var ownerID int
	err := s.db.QueryRow("SELECT owner_id FROM projects WHERE id = $1", projectID).Scan(&ownerID)
	if err != nil {
		return false, err
	}
	if ownerID == userID {
		return true, nil
	}

	// Check if user is an admin member
	var role string
	err = s.db.QueryRow("SELECT role FROM project_members WHERE project_id = $1 AND user_id = $2", projectID, userID).Scan(&role)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, err
	}

	return role == "admin", nil
}

// ProjectInvitation represents a project membership invitation
type ProjectInvitation struct {
	ID            int64      `json:"id"`
	ProjectID     int64      `json:"project_id"`
	ProjectName   string     `json:"project_name,omitempty"`
	InviterID     int64      `json:"inviter_id"`
	InviterName   string     `json:"inviter_name,omitempty"`
	InviteeUserID int64      `json:"invitee_user_id"`
	InviteeName   string     `json:"invitee_name,omitempty"`
	InviteeEmail  string     `json:"invitee_email,omitempty"`
	Role          string     `json:"role"`
	Status        string     `json:"status"`
	InvitedAt     time.Time  `json:"invited_at"`
	RespondedAt   *time.Time `json:"responded_at,omitempty"`
	LastSentAt    time.Time  `json:"last_sent_at"`
	CanResend     bool       `json:"can_resend"`
}

// InviteProjectMemberRequest is the request body for inviting a project member
type InviteProjectMemberRequest struct {
	UserID int64  `json:"user_id"`
	Role   string `json:"role"`
}

// HandleInviteProjectMember creates a project invitation for a team member
func (s *Server) HandleInviteProjectMember(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	isOwnerOrAdmin, err := s.userIsProjectOwnerOrAdmin(int(userID), projectID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !isOwnerOrAdmin {
		http.Error(w, "Forbidden - only project owners and admins can invite members", http.StatusForbidden)
		return
	}

	var req InviteProjectMemberRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	if req.Role != "viewer" && req.Role != "member" && req.Role != "editor" && req.Role != "owner" {
		http.Error(w, "Invalid role. Must be viewer, member, editor, or owner", http.StatusBadRequest)
		return
	}

	// Get project info
	var projectName string
	var teamID int
	err = s.db.QueryRow("SELECT name, team_id FROM projects WHERE id = $1", projectID).Scan(&projectName, &teamID)
	if err == sql.ErrNoRows {
		http.Error(w, "Project not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Cannot invite the project owner
	var ownerID int
	if err := s.db.QueryRow("SELECT owner_id FROM projects WHERE id = $1", projectID).Scan(&ownerID); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if int(req.UserID) == ownerID {
		http.Error(w, "Cannot invite project owner", http.StatusBadRequest)
		return
	}

	// Invitee must be an active team member
	var inTeam bool
	if err := s.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM team_members WHERE team_id = $1 AND user_id = $2 AND status = 'active')
	`, teamID, req.UserID).Scan(&inTeam); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !inTeam {
		http.Error(w, "User must be an active team member to be invited to this project", http.StatusBadRequest)
		return
	}

	// Cannot invite existing project member
	var alreadyMember bool
	if err := s.db.QueryRow(`
		SELECT EXISTS(SELECT 1 FROM project_members WHERE project_id = $1 AND user_id = $2)
	`, projectID, req.UserID).Scan(&alreadyMember); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if alreadyMember {
		http.Error(w, "User is already a member of this project", http.StatusConflict)
		return
	}

	// Create invitation (upsert: update last_sent_at if re-inviting after withdraw/reject)
	var invID int64
	err = s.db.QueryRow(`
		INSERT INTO project_invitations (project_id, inviter_id, invitee_user_id, role, status, invited_at, last_sent_at)
		VALUES ($1, $2, $3, $4, 'pending', CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)
		ON CONFLICT(project_id, invitee_user_id) DO UPDATE
		  SET status = 'pending', inviter_id = $2, role = $4, invited_at = CURRENT_TIMESTAMP, last_sent_at = CURRENT_TIMESTAMP, responded_at = NULL
		RETURNING id
	`, projectID, userID, req.UserID, req.Role).Scan(&invID)
	if err != nil {
		s.logger.Error("Failed to create project invitation", zap.Error(err))
		http.Error(w, "Failed to create invitation", http.StatusInternalServerError)
		return
	}

	// Send email if service is available
	if emailSvc := s.GetEmailService(); emailSvc != nil {
		ctx := r.Context()
		var inviterName, inviteeEmail string
		_ = s.db.QueryRow(`
			SELECT COALESCE(NULLIF(TRIM(COALESCE(first_name,'') || ' ' || COALESCE(last_name,'')), ''), name, email) FROM users WHERE id = $1
		`, userID).Scan(&inviterName)
		_ = s.db.QueryRow("SELECT email FROM users WHERE id = $1", req.UserID).Scan(&inviteeEmail)
		appURL := s.getAppURL()
		if err := emailSvc.SendProjectMemberInvitation(ctx, inviteeEmail, inviterName, projectName, appURL); err != nil {
			s.logger.Warn("Failed to send project invitation email",
				zap.String("to", inviteeEmail),
				zap.Error(err),
			)
		}
	}

	// Notify invitee via WebSocket if they're connected
	s.BroadcastToUser(int64(req.UserID), "project_invitation", map[string]interface{}{
		"invitation_id": invID,
		"project_name":  projectName,
	})

	respondJSON(w, http.StatusCreated, map[string]interface{}{
		"message":       "Invitation sent",
		"invitation_id": invID,
	})
}

// HandleGetProjectInvitations returns all invitations for a project
func (s *Server) HandleGetProjectInvitations(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "Invalid project ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	isOwnerOrAdmin, err := s.userIsProjectOwnerOrAdmin(int(userID), projectID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !isOwnerOrAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	rows, err := s.db.Query(`
		SELECT pi.id, pi.project_id, p.name, pi.inviter_id,
		       COALESCE(NULLIF(TRIM(COALESCE(inv.first_name,'') || ' ' || COALESCE(inv.last_name,'')), ''), inv.name, inv.email),
		       pi.invitee_user_id,
		       COALESCE(NULLIF(TRIM(COALESCE(tee.first_name,'') || ' ' || COALESCE(tee.last_name,'')), ''), tee.name, tee.email),
		       tee.email,
		       pi.role, pi.status, pi.invited_at, pi.responded_at, pi.last_sent_at
		FROM project_invitations pi
		JOIN projects p ON pi.project_id = p.id
		JOIN users inv ON pi.inviter_id = inv.id
		JOIN users tee ON pi.invitee_user_id = tee.id
		WHERE pi.project_id = $1
		ORDER BY pi.invited_at DESC
	`, projectID)
	if err != nil {
		http.Error(w, "Failed to fetch invitations", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	now := time.Now()
	invitations := []ProjectInvitation{}
	for rows.Next() {
		var inv ProjectInvitation
		var respondedAt sql.NullTime
		if err := rows.Scan(
			&inv.ID, &inv.ProjectID, &inv.ProjectName,
			&inv.InviterID, &inv.InviterName,
			&inv.InviteeUserID, &inv.InviteeName, &inv.InviteeEmail,
			&inv.Role, &inv.Status, &inv.InvitedAt, &respondedAt, &inv.LastSentAt,
		); err != nil {
			http.Error(w, "Failed to scan invitation", http.StatusInternalServerError)
			return
		}
		if respondedAt.Valid {
			inv.RespondedAt = &respondedAt.Time
		}
		inv.CanResend = inv.Status == "pending" && now.Sub(inv.LastSentAt) >= 48*time.Hour
		invitations = append(invitations, inv)
	}

	respondJSON(w, http.StatusOK, invitations)
}

// HandleAcceptProjectInvitation accepts a project invitation
func (s *Server) HandleAcceptProjectInvitation(w http.ResponseWriter, r *http.Request) {
	invID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid invitation ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	// Load invitation and verify invitee
	var projectID int64
	var inviterID int64
	var role string
	var status string
	err = s.db.QueryRow(`
		SELECT project_id, inviter_id, role, status FROM project_invitations WHERE id = $1
	`, invID).Scan(&projectID, &inviterID, &role, &status)
	if err == sql.ErrNoRows {
		http.Error(w, "Invitation not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Verify this invitation belongs to the current user
	var inviteeUserID int64
	if err := s.db.QueryRow("SELECT invitee_user_id FROM project_invitations WHERE id = $1", invID).Scan(&inviteeUserID); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if inviteeUserID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if status != "pending" {
		http.Error(w, "Invitation is no longer pending", http.StatusConflict)
		return
	}

	// Update status and insert project member in a transaction
	tx, err := s.db.Begin()
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	defer tx.Rollback()

	_, err = tx.Exec(`
		UPDATE project_invitations SET status = 'accepted', responded_at = CURRENT_TIMESTAMP WHERE id = $1
	`, invID)
	if err != nil {
		http.Error(w, "Failed to update invitation", http.StatusInternalServerError)
		return
	}

	_, err = tx.Exec(`
		INSERT INTO project_members (project_id, user_id, role, granted_by, granted_at)
		VALUES ($1, $2, $3, $4, CURRENT_TIMESTAMP)
		ON CONFLICT(project_id, user_id) DO NOTHING
	`, projectID, userID, role, inviterID)
	if err != nil {
		http.Error(w, "Failed to add project member", http.StatusInternalServerError)
		return
	}

	if err := tx.Commit(); err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Notify the user via WebSocket — triggers sidebar refresh on the client
	s.BroadcastToUser(userID, "project_membership", map[string]interface{}{
		"project_id": projectID,
	})

	respondJSON(w, http.StatusOK, map[string]string{"message": "Invitation accepted"})
}

// HandleRejectProjectInvitation rejects a project invitation
func (s *Server) HandleRejectProjectInvitation(w http.ResponseWriter, r *http.Request) {
	invID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid invitation ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var inviteeUserID int64
	var status string
	err = s.db.QueryRow("SELECT invitee_user_id, status FROM project_invitations WHERE id = $1", invID).Scan(&inviteeUserID, &status)
	if err == sql.ErrNoRows {
		http.Error(w, "Invitation not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if inviteeUserID != userID {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}
	if status != "pending" {
		http.Error(w, "Invitation is no longer pending", http.StatusConflict)
		return
	}

	_, err = s.db.Exec(`
		UPDATE project_invitations SET status = 'rejected', responded_at = CURRENT_TIMESTAMP WHERE id = $1
	`, invID)
	if err != nil {
		http.Error(w, "Failed to reject invitation", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Invitation rejected"})
}

// HandleWithdrawProjectInvitation withdraws (deletes) a project invitation
func (s *Server) HandleWithdrawProjectInvitation(w http.ResponseWriter, r *http.Request) {
	invID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid invitation ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var projectID int64
	err = s.db.QueryRow("SELECT project_id FROM project_invitations WHERE id = $1", invID).Scan(&projectID)
	if err == sql.ErrNoRows {
		http.Error(w, "Invitation not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	isOwnerOrAdmin, err := s.userIsProjectOwnerOrAdmin(int(userID), int(projectID))
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !isOwnerOrAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	_, err = s.db.Exec(`
		UPDATE project_invitations SET status = 'withdrawn' WHERE id = $1
	`, invID)
	if err != nil {
		http.Error(w, "Failed to withdraw invitation", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Invitation withdrawn"})
}

// HandleResendProjectInvitation resends a project invitation email (rate limited to once per 2 days)
func (s *Server) HandleResendProjectInvitation(w http.ResponseWriter, r *http.Request) {
	invID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		http.Error(w, "Invalid invitation ID", http.StatusBadRequest)
		return
	}

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var projectID int64
	var status string
	var lastSentAt time.Time
	err = s.db.QueryRow(`
		SELECT project_id, status, last_sent_at FROM project_invitations WHERE id = $1
	`, invID).Scan(&projectID, &status, &lastSentAt)
	if err == sql.ErrNoRows {
		http.Error(w, "Invitation not found", http.StatusNotFound)
		return
	}
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	if status != "pending" {
		http.Error(w, "Can only resend pending invitations", http.StatusBadRequest)
		return
	}

	isOwnerOrAdmin, err := s.userIsProjectOwnerOrAdmin(int(userID), int(projectID))
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	if !isOwnerOrAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	if time.Since(lastSentAt) < 48*time.Hour {
		http.Error(w, "Can only resend once every 2 days", http.StatusTooManyRequests)
		return
	}

	_, err = s.db.Exec("UPDATE project_invitations SET last_sent_at = CURRENT_TIMESTAMP WHERE id = $1", invID)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	// Send email
	if emailSvc := s.GetEmailService(); emailSvc != nil {
		ctx := r.Context()
		var inviteeEmail, inviterName, projectName string
		_ = s.db.QueryRow(`
			SELECT tee.email,
			       COALESCE(NULLIF(TRIM(COALESCE(inv.first_name,'') || ' ' || COALESCE(inv.last_name,'')), ''), inv.name, inv.email),
			       p.name
			FROM project_invitations pi
			JOIN users tee ON pi.invitee_user_id = tee.id
			JOIN users inv ON pi.inviter_id = inv.id
			JOIN projects p ON pi.project_id = p.id
			WHERE pi.id = $1
		`, invID).Scan(&inviteeEmail, &inviterName, &projectName)
		appURL := s.getAppURL()
		if err := emailSvc.SendProjectMemberInvitation(ctx, inviteeEmail, inviterName, projectName, appURL); err != nil {
			s.logger.Warn("Failed to resend project invitation email",
				zap.String("to", inviteeEmail),
				zap.Error(err),
			)
		}
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Invitation resent"})
}

// HandleGetMyProjectInvitations returns pending project invitations for the current user
func (s *Server) HandleGetMyProjectInvitations(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	rows, err := s.db.Query(`
		SELECT pi.id, pi.project_id, p.name, pi.inviter_id,
		       COALESCE(NULLIF(TRIM(COALESCE(inv.first_name,'') || ' ' || COALESCE(inv.last_name,'')), ''), inv.name, inv.email),
		       pi.invitee_user_id,
		       COALESCE(NULLIF(TRIM(COALESCE(tee.first_name,'') || ' ' || COALESCE(tee.last_name,'')), ''), tee.name, tee.email),
		       tee.email,
		       pi.role, pi.status, pi.invited_at, pi.responded_at, pi.last_sent_at
		FROM project_invitations pi
		JOIN projects p ON pi.project_id = p.id
		JOIN users inv ON pi.inviter_id = inv.id
		JOIN users tee ON pi.invitee_user_id = tee.id
		WHERE pi.invitee_user_id = $1 AND pi.status = 'pending'
		ORDER BY pi.invited_at DESC
	`, userID)
	if err != nil {
		http.Error(w, "Failed to fetch invitations", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	now := time.Now()
	invitations := []ProjectInvitation{}
	for rows.Next() {
		var inv ProjectInvitation
		var respondedAt sql.NullTime
		if err := rows.Scan(
			&inv.ID, &inv.ProjectID, &inv.ProjectName,
			&inv.InviterID, &inv.InviterName,
			&inv.InviteeUserID, &inv.InviteeName, &inv.InviteeEmail,
			&inv.Role, &inv.Status, &inv.InvitedAt, &respondedAt, &inv.LastSentAt,
		); err != nil {
			http.Error(w, "Failed to scan invitation", http.StatusInternalServerError)
			return
		}
		if respondedAt.Valid {
			inv.RespondedAt = &respondedAt.Time
		}
		inv.CanResend = now.Sub(inv.LastSentAt) >= 48*time.Hour
		invitations = append(invitations, inv)
	}

	respondJSON(w, http.StatusOK, invitations)
}

// HandleGetMyProjectInvitationCount returns count of pending project invitations for the current user
func (s *Server) HandleGetMyProjectInvitationCount(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM project_invitations WHERE invitee_user_id = $1 AND status = 'pending'
	`, userID).Scan(&count)
	if err != nil {
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}

	respondJSON(w, http.StatusOK, map[string]int{"count": count})
}
