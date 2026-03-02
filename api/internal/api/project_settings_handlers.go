package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
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
	LastSync     *time.Time `json:"github_last_sync"`
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
	RepoURL     string `json:"github_repo_url"`
	Owner       string `json:"github_owner"`
	RepoName    string `json:"github_repo_name"`
	Branch      string `json:"github_branch"`
	SyncEnabled bool   `json:"github_sync_enabled"`
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
	var syncEnabled int
	var lastSync sql.NullTime

	err = s.db.QueryRow(`
		SELECT
			COALESCE(github_repo_url, ''),
			COALESCE(github_owner, ''),
			COALESCE(github_repo_name, ''),
			COALESCE(github_branch, 'main'),
			github_sync_enabled,
			github_last_sync
		FROM projects
		WHERE id = $1
	`, projectID).Scan(
		&settings.RepoURL,
		&settings.Owner,
		&settings.RepoName,
		&settings.Branch,
		&syncEnabled,
		&lastSync,
	)

	if err != nil {
		http.Error(w, "Failed to fetch GitHub settings", http.StatusInternalServerError)
		return
	}

	settings.SyncEnabled = syncEnabled == 1
	if lastSync.Valid {
		settings.LastSync = &lastSync.Time
	}

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

	syncEnabled := 0
	if req.SyncEnabled {
		syncEnabled = 1
	}

	_, err = s.db.Exec(`
		UPDATE projects
		SET
			github_repo_url = $1,
			github_owner = $2,
			github_repo_name = $3,
			github_branch = $4,
			github_sync_enabled = $5
		WHERE id = $6
	`, req.RepoURL, req.Owner, req.RepoName, req.Branch, syncEnabled, projectID)

	if err != nil {
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
