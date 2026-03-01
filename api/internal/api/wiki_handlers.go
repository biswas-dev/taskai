package api

import (
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"taskai/ent"
	"taskai/ent/wikipage"
)

// WikiPageResponse represents a wiki page in API responses
type WikiPageResponse struct {
	ID          int64     `json:"id"`
	ProjectID   int64     `json:"project_id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	CreatedBy   int64     `json:"created_by"`
	CreatorName *string   `json:"creator_name,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateWikiPageRequest represents a request to create a wiki page
type CreateWikiPageRequest struct {
	Title string `json:"title"`
}

// UpdateWikiPageRequest represents a request to update a wiki page
type UpdateWikiPageRequest struct {
	Title *string `json:"title,omitempty"`
}

// HandleListWikiPages returns all wiki pages for a project
func (s *Server) HandleListWikiPages(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	projectID, err := strconv.ParseInt(chi.URLParam(r, "projectId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "invalid_input")
		return
	}

	// Verify user has access to this project
	hasAccess, err := s.checkProjectAccess(ctx, userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	// Fetch all wiki pages for the project
	pages, err := s.db.Client.WikiPage.Query().
		Where(wikipage.ProjectID(projectID)).
		WithCreator().
		Order(ent.Asc(wikipage.FieldTitle)).
		All(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch wiki pages",
			zap.Int64("project_id", projectID),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to fetch wiki pages", "internal_error")
		return
	}

	// Convert to response format
	response := make([]WikiPageResponse, 0, len(pages))
	for _, p := range pages {
		wp := WikiPageResponse{
			ID:        p.ID,
			ProjectID: p.ProjectID,
			Title:     p.Title,
			Slug:      p.Slug,
			CreatedBy: p.CreatedBy,
			CreatedAt: p.CreatedAt,
			UpdatedAt: p.UpdatedAt,
		}
		if p.Edges.Creator != nil && p.Edges.Creator.Name != nil {
			wp.CreatorName = p.Edges.Creator.Name
		}
		response = append(response, wp)
	}

	respondJSON(w, http.StatusOK, response)
}

// generateSlug creates a URL-friendly slug from a title
func generateSlug(title string) string {
	// Convert to lowercase
	slug := strings.ToLower(title)
	// Replace spaces and special chars with hyphens
	reg := regexp.MustCompile("[^a-z0-9]+")
	slug = reg.ReplaceAllString(slug, "-")
	// Remove leading/trailing hyphens
	slug = strings.Trim(slug, "-")
	// Truncate to max 100 chars
	if len(slug) > 100 {
		slug = slug[:100]
	}
	return slug
}

// HandleCreateWikiPage creates a new wiki page
func (s *Server) HandleCreateWikiPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	projectID, err := strconv.ParseInt(chi.URLParam(r, "projectId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "invalid_input")
		return
	}

	// Verify user has access to this project
	hasAccess, err := s.checkProjectAccess(ctx, userID, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	var req CreateWikiPageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}

	// Validation
	if req.Title == "" {
		respondError(w, http.StatusBadRequest, "title is required", "invalid_input")
		return
	}
	if len(req.Title) > 500 {
		respondError(w, http.StatusBadRequest, "title is too long (max 500 characters)", "invalid_input")
		return
	}

	// Generate slug
	baseSlug := generateSlug(req.Title)
	slug := baseSlug

	// Ensure slug is unique within the project
	for i := 1; i < 100; i++ {
		exists, err := s.db.Client.WikiPage.Query().
			Where(
				wikipage.ProjectID(projectID),
				wikipage.Slug(slug),
			).
			Exist(ctx)
		if err != nil {
			respondError(w, http.StatusInternalServerError, "failed to check slug uniqueness", "internal_error")
			return
		}
		if !exists {
			break
		}
		slug = baseSlug + "-" + strconv.Itoa(i)
	}

	// Create wiki page
	page, err := s.db.Client.WikiPage.Create().
		SetProjectID(projectID).
		SetTitle(req.Title).
		SetSlug(slug).
		SetCreatedBy(userID).
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to create wiki page",
			zap.Int64("project_id", projectID),
			zap.String("title", req.Title),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to create wiki page", "internal_error")
		return
	}

	response := WikiPageResponse{
		ID:        page.ID,
		ProjectID: page.ProjectID,
		Title:     page.Title,
		Slug:      page.Slug,
		CreatedBy: page.CreatedBy,
		CreatedAt: page.CreatedAt,
		UpdatedAt: page.UpdatedAt,
	}

	respondJSON(w, http.StatusCreated, response)
}

// HandleGetWikiPage returns a single wiki page
func (s *Server) HandleGetWikiPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}

	// Fetch the wiki page
	page, err := s.db.Client.WikiPage.Query().
		Where(wikipage.ID(pageID)).
		WithCreator().
		WithProject().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "wiki page not found", "not_found")
			return
		}
		s.logger.Error("Failed to fetch wiki page",
			zap.Int64("page_id", pageID),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to fetch wiki page", "internal_error")
		return
	}

	// Verify user has access to the project
	hasAccess, err := s.checkProjectAccess(ctx, userID, page.ProjectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	response := WikiPageResponse{
		ID:        page.ID,
		ProjectID: page.ProjectID,
		Title:     page.Title,
		Slug:      page.Slug,
		CreatedBy: page.CreatedBy,
		CreatedAt: page.CreatedAt,
		UpdatedAt: page.UpdatedAt,
	}
	if page.Edges.Creator != nil && page.Edges.Creator.Name != nil {
		response.CreatorName = page.Edges.Creator.Name
	}

	respondJSON(w, http.StatusOK, response)
}

// HandleUpdateWikiPage updates a wiki page
func (s *Server) HandleUpdateWikiPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}

	// Fetch the wiki page
	page, err := s.db.Client.WikiPage.Query().
		Where(wikipage.ID(pageID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "wiki page not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch wiki page", "internal_error")
		return
	}

	// Verify user has access to the project
	hasAccess, err := s.checkProjectAccess(ctx, userID, page.ProjectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	var req UpdateWikiPageRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}

	// Update wiki page
	update := s.db.Client.WikiPage.UpdateOneID(pageID)

	if req.Title != nil {
		if *req.Title == "" {
			respondError(w, http.StatusBadRequest, "title cannot be empty", "invalid_input")
			return
		}
		if len(*req.Title) > 500 {
			respondError(w, http.StatusBadRequest, "title is too long (max 500 characters)", "invalid_input")
			return
		}
		update.SetTitle(*req.Title)

		// Regenerate slug if title changed
		if *req.Title != page.Title {
			newSlug := generateSlug(*req.Title)
			update.SetSlug(newSlug)
		}
	}

	updatedPage, err := update.Save(ctx)
	if err != nil {
		s.logger.Error("Failed to update wiki page",
			zap.Int64("page_id", pageID),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to update wiki page", "internal_error")
		return
	}

	response := WikiPageResponse{
		ID:        updatedPage.ID,
		ProjectID: updatedPage.ProjectID,
		Title:     updatedPage.Title,
		Slug:      updatedPage.Slug,
		CreatedBy: updatedPage.CreatedBy,
		CreatedAt: updatedPage.CreatedAt,
		UpdatedAt: updatedPage.UpdatedAt,
	}

	respondJSON(w, http.StatusOK, response)
}

// UpdateWikiPageContentRequest represents a request to update wiki page content
type UpdateWikiPageContentRequest struct {
	Content string `json:"content"`
}

// WikiPageContentResponse represents wiki page content in API responses
type WikiPageContentResponse struct {
	PageID    int64     `json:"page_id"`
	Content   string    `json:"content"`
	UpdatedAt time.Time `json:"updated_at"`
}

// HandleGetWikiPageContent returns the content of a wiki page
func (s *Server) HandleGetWikiPageContent(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}

	// Fetch the wiki page
	page, err := s.db.Client.WikiPage.Query().
		Where(wikipage.ID(pageID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "wiki page not found", "not_found")
			return
		}
		s.logger.Error("Failed to fetch wiki page content",
			zap.Int64("page_id", pageID),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to fetch wiki page", "internal_error")
		return
	}

	// Verify user has access to the project
	hasAccess, err := s.checkProjectAccess(ctx, userID, page.ProjectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	respondJSON(w, http.StatusOK, WikiPageContentResponse{
		PageID:    page.ID,
		Content:   page.Content,
		UpdatedAt: page.UpdatedAt,
	})
}

// HandleUpdateWikiPageContent updates the content of a wiki page
func (s *Server) HandleUpdateWikiPageContent(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}

	// Fetch the wiki page
	page, err := s.db.Client.WikiPage.Query().
		Where(wikipage.ID(pageID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "wiki page not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch wiki page", "internal_error")
		return
	}

	// Verify user has access to the project
	hasAccess, err := s.checkProjectAccess(ctx, userID, page.ProjectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	var req UpdateWikiPageContentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}

	updatedPage, err := s.db.Client.WikiPage.UpdateOneID(pageID).
		SetContent(req.Content).
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to update wiki page content",
			zap.Int64("page_id", pageID),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to update wiki page content", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, WikiPageContentResponse{
		PageID:    updatedPage.ID,
		Content:   updatedPage.Content,
		UpdatedAt: updatedPage.UpdatedAt,
	})
}

// HandleDeleteWikiPage deletes a wiki page
func (s *Server) HandleDeleteWikiPage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}

	// Fetch the wiki page
	page, err := s.db.Client.WikiPage.Query().
		Where(wikipage.ID(pageID)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "wiki page not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch wiki page", "internal_error")
		return
	}

	// Verify user has access to the project
	hasAccess, err := s.checkProjectAccess(ctx, userID, page.ProjectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	// Delete the wiki page (cascades to related records)
	err = s.db.Client.WikiPage.DeleteOneID(pageID).Exec(ctx)
	if err != nil {
		s.logger.Error("Failed to delete wiki page",
			zap.Int64("page_id", pageID),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to delete wiki page", "internal_error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
