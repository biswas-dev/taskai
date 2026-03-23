package api

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"

	"taskai/ent"
	"taskai/ent/wikipage"
	"taskai/ent/wikipageversion"
)

const errInvalidRequestBody = "invalid request body"

// WikiPageResponse represents a wiki page in API responses
type WikiPageResponse struct {
	ID          int64     `json:"id"`
	ProjectID   int64     `json:"project_id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	CreatedBy   int64     `json:"created_by"`
	CreatorName *string   `json:"creator_name,omitempty"`
	UpdatedBy   *int64    `json:"updated_by,omitempty"`
	UpdaterName *string   `json:"updater_name,omitempty"`
	Content     *string   `json:"content,omitempty"`
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
		WithUpdater().
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
			UpdatedBy: p.UpdatedBy,
			CreatedAt: p.CreatedAt,
			UpdatedAt: p.UpdatedAt,
		}
		if p.Edges.Creator != nil && p.Edges.Creator.Name != nil {
			wp.CreatorName = p.Edges.Creator.Name
		}
		if p.Edges.Updater != nil && p.Edges.Updater.Name != nil {
			wp.UpdaterName = p.Edges.Updater.Name
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
		respondError(w, http.StatusBadRequest, errInvalidRequestBody, "invalid_input")
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
		WithUpdater().
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
		UpdatedBy: page.UpdatedBy,
		Content:   &page.Content,
		CreatedAt: page.CreatedAt,
		UpdatedAt: page.UpdatedAt,
	}
	if page.Edges.Creator != nil && page.Edges.Creator.Name != nil {
		response.CreatorName = page.Edges.Creator.Name
	}
	if page.Edges.Updater != nil && page.Edges.Updater.Name != nil {
		response.UpdaterName = page.Edges.Updater.Name
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
		respondError(w, http.StatusBadRequest, errInvalidRequestBody, "invalid_input")
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
	Content    string `json:"content"`
	ManualSave bool   `json:"manual_save"`
}

// WikiPageVersionResponse represents a wiki page version in API responses (without content)
type WikiPageVersionResponse struct {
	ID            int64     `json:"id"`
	WikiPageID    int64     `json:"wiki_page_id"`
	VersionNumber int       `json:"version_number"`
	ContentHash   string    `json:"content_hash"`
	CreatedBy     int64     `json:"created_by"`
	CreatorName   *string   `json:"creator_name,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// WikiPageVersionWithContentResponse includes the full content
type WikiPageVersionWithContentResponse struct {
	WikiPageVersionResponse
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
		respondError(w, http.StatusBadRequest, errInvalidRequestBody, "invalid_input")
		return
	}

	updatedPage, err := s.db.Client.WikiPage.UpdateOneID(pageID).
		SetContent(req.Content).
		SetUpdatedBy(userID).
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to update wiki page content",
			zap.Int64("page_id", pageID),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to update wiki page content", "internal_error")
		return
	}

	// Set agent_name via raw SQL if present (wiki_pages Ent schema doesn't have this field yet)
	agentName := GetAgentName(r)
	if agentName != nil {
		if _, err := s.db.ExecContext(ctx, `UPDATE wiki_pages SET agent_name = $1 WHERE id = $2`, *agentName, pageID); err != nil {
			s.logger.Warn("Failed to set agent_name on wiki page", zap.Error(err), zap.Int64("page_id", pageID))
		}
	} else {
		// Clear agent_name when human edits
		if _, err := s.db.ExecContext(ctx, `UPDATE wiki_pages SET agent_name = NULL WHERE id = $1`, pageID); err != nil {
			s.logger.Warn("Failed to clear agent_name on wiki page", zap.Error(err), zap.Int64("page_id", pageID))
		}
	}

	if err := s.maybeCreateVersion(ctx, pageID, userID, req.Content, req.ManualSave, agentName); err != nil {
		s.logger.Warn("Failed to create wiki page version",
			zap.Int64("page_id", pageID),
			zap.Error(err),
		)
	}

	// Sync knowledge graph links in background (best-effort).
	go s.syncGraphLinks(context.Background(), page.ProjectID, "wiki", pageID, nil, page.Title, req.Content)

	respondJSON(w, http.StatusOK, WikiPageContentResponse{
		PageID:    updatedPage.ID,
		Content:   updatedPage.Content,
		UpdatedAt: updatedPage.UpdatedAt,
	})
}

// maybeCreateVersion creates a version snapshot if versioning criteria are met.
func (s *Server) maybeCreateVersion(ctx context.Context, pageID, userID int64, newContent string, manualSave bool, agentName *string) error {
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(newContent)))

	// Fetch last version
	lastVersion, err := s.db.Client.WikiPageVersion.Query().
		Where(wikipageversion.WikiPageID(pageID)).
		Order(ent.Desc(wikipageversion.FieldVersionNumber)).
		First(ctx)
	if err != nil && !ent.IsNotFound(err) {
		return err
	}

	// Skip if content identical to last version
	if lastVersion != nil && lastVersion.ContentHash == hash {
		return nil
	}

	shouldVersion := manualSave
	if !shouldVersion {
		if lastVersion == nil {
			// First content — version it
			if newContent != "" {
				shouldVersion = true
			}
		} else if time.Since(lastVersion.CreatedAt) > 15*time.Minute {
			// Time-based snapshot
			shouldVersion = true
		} else if isSignificantChange(lastVersion.Content, newContent) {
			// Large diff
			shouldVersion = true
		}
	}

	if !shouldVersion {
		return nil
	}

	versionNum := 1
	if lastVersion != nil {
		versionNum = lastVersion.VersionNumber + 1
	}

	ver, err := s.db.Client.WikiPageVersion.Create().
		SetWikiPageID(pageID).
		SetVersionNumber(versionNum).
		SetContent(newContent).
		SetContentHash(hash).
		SetCreatedBy(userID).
		Save(ctx)
	if err != nil {
		return err
	}

	// Set agent_name via raw SQL (Ent schema doesn't have this field yet)
	if agentName != nil {
		if _, execErr := s.db.ExecContext(ctx, `UPDATE wiki_page_versions SET agent_name = $1 WHERE id = $2`, *agentName, ver.ID); execErr != nil {
			s.logger.Warn("Failed to set agent_name on wiki version", zap.Error(execErr), zap.Int64("version_id", ver.ID))
		}
	}
	return nil
}

// isSignificantChange returns true when the diff is >15% of old or >500 chars changed.
func isSignificantChange(oldContent, newContent string) bool {
	oldLines := strings.Split(oldContent, "\n")
	newLines := strings.Split(newContent, "\n")

	oldSet := make(map[string]int, len(oldLines))
	for _, l := range oldLines {
		oldSet[l]++
	}

	charsChanged := 0
	for _, l := range newLines {
		if oldSet[l] > 0 {
			oldSet[l]--
		} else {
			charsChanged += len(l)
		}
	}
	// Also count lines removed from old
	for l, cnt := range oldSet {
		charsChanged += len(l) * cnt
	}

	if charsChanged > 500 {
		return true
	}
	if len(oldContent) > 0 && float64(charsChanged) > 0.15*float64(len(oldContent)) {
		return true
	}
	return false
}

// HandleListWikiPageVersions returns all versions for a wiki page (no content body).
func (s *Server) HandleListWikiPageVersions(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}

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

	hasAccess, err := s.checkProjectAccess(ctx, userID, page.ProjectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	versions, err := s.db.Client.WikiPageVersion.Query().
		Where(wikipageversion.WikiPageID(pageID)).
		WithCreator().
		Order(ent.Desc(wikipageversion.FieldVersionNumber)).
		All(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch wiki page versions",
			zap.Int64("page_id", pageID),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to fetch versions", "internal_error")
		return
	}

	resp := make([]WikiPageVersionResponse, 0, len(versions))
	for _, v := range versions {
		r := WikiPageVersionResponse{
			ID:            v.ID,
			WikiPageID:    v.WikiPageID,
			VersionNumber: v.VersionNumber,
			ContentHash:   v.ContentHash,
			CreatedBy:     v.CreatedBy,
			CreatedAt:     v.CreatedAt,
		}
		if v.Edges.Creator != nil && v.Edges.Creator.Name != nil {
			r.CreatorName = v.Edges.Creator.Name
		}
		resp = append(resp, r)
	}

	respondJSON(w, http.StatusOK, resp)
}

// HandleGetWikiPageVersion returns a single version with full content.
func (s *Server) HandleGetWikiPageVersion(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}
	versionNumber, err := strconv.Atoi(chi.URLParam(r, "versionNumber"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid version number", "invalid_input")
		return
	}

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

	hasAccess, err := s.checkProjectAccess(ctx, userID, page.ProjectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	version, err := s.db.Client.WikiPageVersion.Query().
		Where(
			wikipageversion.WikiPageID(pageID),
			wikipageversion.VersionNumber(versionNumber),
		).
		WithCreator().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "version not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch version", "internal_error")
		return
	}

	resp := WikiPageVersionWithContentResponse{
		WikiPageVersionResponse: WikiPageVersionResponse{
			ID:            version.ID,
			WikiPageID:    version.WikiPageID,
			VersionNumber: version.VersionNumber,
			ContentHash:   version.ContentHash,
			CreatedBy:     version.CreatedBy,
			CreatedAt:     version.CreatedAt,
		},
		Content: version.Content,
	}
	if version.Edges.Creator != nil && version.Edges.Creator.Name != nil {
		resp.CreatorName = version.Edges.Creator.Name
	}

	respondJSON(w, http.StatusOK, resp)
}

// HandleRestoreWikiPageVersion restores a wiki page to a previous version.
func (s *Server) HandleRestoreWikiPageVersion(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "invalid_input")
		return
	}
	versionNumber, err := strconv.Atoi(chi.URLParam(r, "versionNumber"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid version number", "invalid_input")
		return
	}

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

	hasAccess, err := s.checkProjectAccess(ctx, userID, page.ProjectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify project access", "internal_error")
		return
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	version, err := s.db.Client.WikiPageVersion.Query().
		Where(
			wikipageversion.WikiPageID(pageID),
			wikipageversion.VersionNumber(versionNumber),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			respondError(w, http.StatusNotFound, "version not found", "not_found")
			return
		}
		respondError(w, http.StatusInternalServerError, "failed to fetch version", "internal_error")
		return
	}

	updatedPage, err := s.db.Client.WikiPage.UpdateOneID(pageID).
		SetContent(version.Content).
		SetUpdatedBy(userID).
		Save(ctx)
	if err != nil {
		s.logger.Error("Failed to restore wiki page version",
			zap.Int64("page_id", pageID),
			zap.Int("version_number", versionNumber),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to restore version", "internal_error")
		return
	}

	// Create a new version for the restore action
	if err := s.maybeCreateVersion(ctx, pageID, userID, version.Content, true, nil); err != nil {
		s.logger.Warn("Failed to create version after restore",
			zap.Int64("page_id", pageID),
			zap.Error(err),
		)
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
