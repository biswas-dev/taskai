package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"

	"taskai/ent"
	"taskai/ent/project"
	"taskai/ent/projectmember"
)

type Project struct {
	ID          int64     `json:"id"`
	OwnerID     int64     `json:"owner_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type CreateProjectRequest struct {
	Name        string  `json:"name"`
	Description *string `json:"description,omitempty"`
}

type UpdateProjectRequest struct {
	Name        *string `json:"name,omitempty"`
	Description *string `json:"description,omitempty"`
}

// HandleListProjects returns all projects the authenticated user has access to
func (s *Server) HandleListProjects(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	// Query projects where user is a member using Ent
	entProjects, err := s.db.Client.Project.Query().
		Where(project.HasMembersWith(projectmember.UserID(userID))).
		Order(ent.Desc(project.FieldUpdatedAt)).
		All(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to fetch projects", "internal_error")
		return
	}

	// Convert Ent projects to API projects
	projects := make([]Project, 0, len(entProjects))
	for _, ep := range entProjects {
		projects = append(projects, Project{
			ID:          ep.ID,
			OwnerID:     ep.OwnerID,
			Name:        ep.Name,
			Description: ep.Description,
			CreatedAt:   ep.CreatedAt,
			UpdatedAt:   ep.UpdatedAt,
		})
	}

	respondJSON(w, http.StatusOK, projects)
}

// HandleGetProject returns a single project by ID
func (s *Server) HandleGetProject(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "invalid_input")
		return
	}

	// Query project with authorization check using Ent
	ep, err := s.db.Client.Project.Query().
		Where(
			project.ID(projectID),
			project.HasMembersWith(projectmember.UserID(userID)),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "project not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch project", "internal_error")
		return
	}

	// Convert to API project
	p := Project{
		ID:          ep.ID,
		OwnerID:     ep.OwnerID,
		Name:        ep.Name,
		Description: ep.Description,
		CreatedAt:   ep.CreatedAt,
		UpdatedAt:   ep.UpdatedAt,
	}

	respondJSON(w, http.StatusOK, p)
}

// HandleCreateProject creates a new project
func (s *Server) HandleCreateProject(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	// Get user's team ID
	teamID, err := s.getUserTeamID(ctx, userID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get user team", "internal_error")
		return
	}

	var req CreateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}

	// Validation
	if req.Name == "" {
		respondError(w, http.StatusBadRequest, "project name is required", "invalid_input")
		return
	}
	if len(req.Name) > 255 {
		respondError(w, http.StatusBadRequest, "project name is too long (max 255 characters)", "invalid_input")
		return
	}

	// Use Ent transaction
	tx, err := s.db.Client.Tx(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create project", "internal_error")
		return
	}
	defer tx.Rollback()

	// Create project using Ent
	newProject, err := tx.Project.Create().
		SetOwnerID(userID).
		SetTeamID(teamID).
		SetName(req.Name).
		SetNillableDescription(req.Description).
		Save(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create project", "internal_error")
		return
	}

	// Add creator as owner member of the project
	_, err = tx.ProjectMember.Create().
		SetProjectID(newProject.ID).
		SetUserID(userID).
		SetRole("owner").
		SetGrantedBy(userID).
		Save(ctx)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to add project owner", "internal_error")
		return
	}

	// Create default swim lanes for the new project
	defaultSwimLanes := []struct {
		name           string
		color          string
		position       int
		statusCategory string
	}{
		{"To Do", "#6B7280", 0, "todo"},
		{"In Progress", "#3B82F6", 1, "in_progress"},
		{"Done", "#10B981", 2, "done"},
	}

	for _, sl := range defaultSwimLanes {
		_, err = tx.SwimLane.Create().
			SetProjectID(newProject.ID).
			SetName(sl.name).
			SetColor(sl.color).
			SetPosition(sl.position).
			SetStatusCategory(sl.statusCategory).
			Save(ctx)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to create default swim lanes", "internal_error")
			return
		}
	}

	// Commit transaction
	if err := tx.Commit(); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to create project", "internal_error")
		return
	}

	// Convert to API project
	p := Project{
		ID:          newProject.ID,
		OwnerID:     newProject.OwnerID,
		Name:        newProject.Name,
		Description: newProject.Description,
		CreatedAt:   newProject.CreatedAt,
		UpdatedAt:   newProject.UpdatedAt,
	}

	respondJSON(w, http.StatusCreated, p)
}

// HandleUpdateProject updates an existing project
func (s *Server) HandleUpdateProject(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "invalid_input")
		return
	}

	var req UpdateProjectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}

	// Check user has access and is owner or editor
	projectMember, err := s.db.Client.ProjectMember.Query().
		Where(
			projectmember.ProjectID(projectID),
			projectmember.UserID(userID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "project not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to check project access", "internal_error")
		return
	}

	// Only owners and editors can update projects
	if projectMember.Role != "owner" && projectMember.Role != "editor" {
		respondError(w, http.StatusForbidden, "only project owners and editors can update projects", "forbidden")
		return
	}

	// Validation
	if req.Name != nil {
		if *req.Name == "" {
			respondError(w, http.StatusBadRequest, "project name cannot be empty", "invalid_input")
			return
		}
		if len(*req.Name) > 255 {
			respondError(w, http.StatusBadRequest, "project name is too long (max 255 characters)", "invalid_input")
			return
		}
	}

	// Build update using Ent
	updateBuilder := s.db.Client.Project.UpdateOneID(projectID)

	if req.Name != nil {
		updateBuilder.SetName(*req.Name)
	}

	if req.Description != nil {
		updateBuilder.SetNillableDescription(req.Description)
	}

	updatedProject, err := updateBuilder.Save(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "project not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to update project", "internal_error")
		return
	}

	// Convert to API project
	p := Project{
		ID:          updatedProject.ID,
		OwnerID:     updatedProject.OwnerID,
		Name:        updatedProject.Name,
		Description: updatedProject.Description,
		CreatedAt:   updatedProject.CreatedAt,
		UpdatedAt:   updatedProject.UpdatedAt,
	}

	respondJSON(w, http.StatusOK, p)
}

// HandleDeleteProject deletes a project
func (s *Server) HandleDeleteProject(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "invalid_input")
		return
	}

	// Check if user is project owner
	projectEntity, err := s.db.Client.Project.Get(ctx, projectID)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "project not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to check project ownership", "internal_error")
		return
	}

	if projectEntity.OwnerID != userID {
		respondError(w, http.StatusForbidden, "only project owner can delete project", "forbidden")
		return
	}

	// Delete project using Ent
	err = s.db.Client.Project.DeleteOneID(projectID).Exec(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "project not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to delete project", "internal_error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
