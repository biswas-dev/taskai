package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"io"
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
const wikiPageVersionRetentionLimit = 50
const wikiPageVersionEncodingGzip = "gzip"

// WikiPageResponse represents a wiki page in API responses
type WikiPageResponse struct {
	ID          int64     `json:"id"`
	ProjectID   int64     `json:"project_id"`
	Title       string    `json:"title"`
	Slug        string    `json:"slug"`
	ParentID    *int64    `json:"parent_id"`
	Position    int       `json:"position"`
	CreatedBy   int64     `json:"created_by"`
	CreatorName *string   `json:"creator_name,omitempty"`
	UpdatedBy   *int64    `json:"updated_by,omitempty"`
	UpdaterName *string   `json:"updater_name,omitempty"`
	AgentName   *string   `json:"agent_name,omitempty"`
	Content     *string   `json:"content,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// CreateWikiPageRequest represents a request to create a wiki page
type CreateWikiPageRequest struct {
	Title    string `json:"title"`
	ParentID *int64 `json:"parent_id,omitempty"`
}

// UpdateWikiPageRequest represents a request to update a wiki page
type UpdateWikiPageRequest struct {
	Title *string `json:"title,omitempty"`
	// ParentID moves the page: omit to leave unchanged, null to move to root.
	ParentID OptionalInt64 `json:"parent_id,omitzero"`
	// Position orders the page among its siblings.
	Position *int `json:"position,omitempty"`
}

// newWikiPageResponse maps an Ent page to its API representation.
func newWikiPageResponse(p *ent.WikiPage) WikiPageResponse {
	wp := WikiPageResponse{
		ID:        p.ID,
		ProjectID: p.ProjectID,
		Title:     p.Title,
		Slug:      p.Slug,
		ParentID:  p.ParentID,
		Position:  p.Position,
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
	return wp
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
		Order(ent.Asc(wikipage.FieldPosition), ent.Asc(wikipage.FieldTitle)).
		All(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch wiki pages",
			zap.Int64("project_id", projectID),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to fetch wiki pages", "internal_error")
		return
	}

	// Bulk-fetch agent_name (not in Ent schema) for all pages in this project
	agentNames := make(map[int64]*string)
	if len(pages) > 0 {
		rows, qErr := s.db.QueryContext(ctx, `SELECT id, agent_name FROM wiki_pages WHERE project_id = $1`, projectID)
		if qErr == nil {
			defer rows.Close()
			for rows.Next() {
				var id int64
				var an *string
				if scanErr := rows.Scan(&id, &an); scanErr == nil && an != nil {
					agentNames[id] = an
				}
			}
		}
	}

	// Convert to response format
	response := make([]WikiPageResponse, 0, len(pages))
	for _, p := range pages {
		wp := newWikiPageResponse(p)
		if an, ok := agentNames[p.ID]; ok {
			wp.AgentName = an
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

	if err := s.validateWikiParent(ctx, projectID, nil, req.ParentID); err != nil {
		respondWikiHierarchyError(w, err)
		return
	}
	position, err := s.nextWikiSiblingPosition(ctx, projectID, req.ParentID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to determine page position", "internal_error")
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
		SetNillableParentID(req.ParentID).
		SetPosition(position).
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

	respondJSON(w, http.StatusCreated, newWikiPageResponse(page))
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

	response := newWikiPageResponse(page)
	response.Content = &page.Content

	// Fetch agent_name (not in Ent schema)
	var agentName *string
	_ = s.db.QueryRowContext(ctx, `SELECT agent_name FROM wiki_pages WHERE id = $1`, page.ID).Scan(&agentName)
	response.AgentName = agentName

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

	if req.ParentID.Set {
		if err := s.validateWikiParent(ctx, page.ProjectID, &page.ID, req.ParentID.Value); err != nil {
			respondWikiHierarchyError(w, err)
			return
		}
		if req.ParentID.Value == nil {
			update.ClearParentID()
		} else {
			update.SetParentID(*req.ParentID.Value)
		}
		// Append to the end of the new sibling list unless a position was given.
		if req.Position == nil {
			position, err := s.nextWikiSiblingPosition(ctx, page.ProjectID, req.ParentID.Value)
			if err != nil {
				respondError(w, http.StatusInternalServerError, "failed to determine page position", "internal_error")
				return
			}
			update.SetPosition(position)
		}
	}
	if req.Position != nil {
		if *req.Position < 0 {
			respondError(w, http.StatusBadRequest, "position cannot be negative", "invalid_input")
			return
		}
		update.SetPosition(*req.Position)
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

	respondJSON(w, http.StatusOK, newWikiPageResponse(updatedPage))
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
		} else {
			lastContent, err := s.getWikiPageVersionContent(ctx, pageID, lastVersion.VersionNumber)
			if err != nil {
				return fmt.Errorf("fetch last wiki page version content: %w", err)
			}
			if isSignificantChange(lastContent, newContent) {
				// Large diff
				shouldVersion = true
			}
		}
	}

	if !shouldVersion {
		return nil
	}

	versionNum := 1
	if lastVersion != nil {
		versionNum = lastVersion.VersionNumber + 1
	}

	if err := s.insertCompressedWikiPageVersion(ctx, pageID, versionNum, newContent, hash, userID, agentName); err != nil {
		return err
	}

	return s.pruneWikiPageVersions(ctx, pageID)
}

func compressWikiPageVersionContent(content string) ([]byte, error) {
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(content)); err != nil {
		_ = zw.Close()
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func decompressWikiPageVersionContent(data []byte) (string, error) {
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return "", err
	}
	defer zr.Close()

	out, err := io.ReadAll(zr)
	if err != nil {
		return "", err
	}
	return string(out), nil
}

func decodeWikiPageVersionContent(content, encoding string, compressed []byte) (string, error) {
	if encoding != wikiPageVersionEncodingGzip || len(compressed) == 0 {
		return content, nil
	}
	return decompressWikiPageVersionContent(compressed)
}

func (s *Server) insertCompressedWikiPageVersion(ctx context.Context, pageID int64, versionNum int, content, hash string, userID int64, agentName *string) error {
	compressed, err := compressWikiPageVersionContent(content)
	if err != nil {
		return fmt.Errorf("compress wiki page version: %w", err)
	}

	var agent any
	if agentName != nil {
		agent = *agentName
	}

	query := s.db.Rebind(`
		INSERT INTO wiki_page_versions (
			wiki_page_id, version_number, content, content_hash, created_by,
			content_encoding, content_compressed, agent_name
		)
		VALUES (?, ?, '', ?, ?, ?, ?, ?)
	`)
	if _, err := s.db.ExecContext(ctx, query, pageID, versionNum, hash, userID, wikiPageVersionEncodingGzip, compressed, agent); err != nil {
		return fmt.Errorf("insert compressed wiki page version: %w", err)
	}
	return nil
}

func (s *Server) getWikiPageVersionContent(ctx context.Context, pageID int64, versionNumber int) (string, error) {
	query := s.db.Rebind(`
		SELECT content, COALESCE(content_encoding, 'plain'), content_compressed
		FROM wiki_page_versions
		WHERE wiki_page_id = ? AND version_number = ?
	`)
	var content string
	var encoding string
	var compressed []byte
	if err := s.db.QueryRowContext(ctx, query, pageID, versionNumber).Scan(&content, &encoding, &compressed); err != nil {
		return "", err
	}
	decoded, err := decodeWikiPageVersionContent(content, encoding, compressed)
	if err != nil {
		return "", fmt.Errorf("decode wiki page version content: %w", err)
	}
	return decoded, nil
}

func (s *Server) pruneWikiPageVersions(ctx context.Context, pageID int64) error {
	query := s.db.Rebind(`
		DELETE FROM wiki_page_versions
		WHERE wiki_page_id = ?
		  AND id IN (
			SELECT id FROM (
				SELECT id, ROW_NUMBER() OVER (PARTITION BY wiki_page_id ORDER BY version_number DESC) AS rn
				FROM wiki_page_versions
				WHERE wiki_page_id = ?
			) ranked
			WHERE rn > ?
		  )
	`)
	if _, err := s.db.ExecContext(ctx, query, pageID, pageID, wikiPageVersionRetentionLimit); err != nil {
		return fmt.Errorf("prune wiki page versions: %w", err)
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
	}
	content, err := s.getWikiPageVersionContent(ctx, pageID, versionNumber)
	if err != nil {
		s.logger.Error("Failed to decode wiki page version content",
			zap.Int64("page_id", pageID),
			zap.Int("version_number", versionNumber),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to fetch version content", "internal_error")
		return
	}
	resp.Content = content
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

	_, err = s.db.Client.WikiPageVersion.Query().
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

	content, err := s.getWikiPageVersionContent(ctx, pageID, versionNumber)
	if err != nil {
		s.logger.Error("Failed to decode wiki page version content for restore",
			zap.Int64("page_id", pageID),
			zap.Int("version_number", versionNumber),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to restore version", "internal_error")
		return
	}

	updatedPage, err := s.db.Client.WikiPage.UpdateOneID(pageID).
		SetContent(content).
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
	if err := s.maybeCreateVersion(ctx, pageID, userID, content, true, nil); err != nil {
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

// OptimizeWikiPageVersionStorage bounds version history and compresses retained
// legacy plaintext versions. It is safe to run repeatedly.
func (s *Server) OptimizeWikiPageVersionStorage(parentCtx context.Context) {
	ctx, cancel := context.WithTimeout(parentCtx, 2*time.Minute)
	defer cancel()

	rows, err := s.db.QueryContext(ctx, `SELECT DISTINCT wiki_page_id FROM wiki_page_versions`)
	if err != nil {
		s.logger.Warn("Failed to list wiki pages for version optimization", zap.Error(err))
		return
	}
	defer rows.Close()

	pageIDs := make([]int64, 0)
	for rows.Next() {
		var pageID int64
		if err := rows.Scan(&pageID); err != nil {
			s.logger.Warn("Failed to scan wiki page for version optimization", zap.Error(err))
			continue
		}
		pageIDs = append(pageIDs, pageID)
	}
	if err := rows.Err(); err != nil {
		s.logger.Warn("Failed while scanning wiki pages for version optimization", zap.Error(err))
		return
	}

	prunedPages := 0
	for _, pageID := range pageIDs {
		if err := s.pruneWikiPageVersions(ctx, pageID); err != nil {
			s.logger.Warn("Failed to prune wiki page versions", zap.Int64("page_id", pageID), zap.Error(err))
			continue
		}
		prunedPages++
	}

	type legacyVersion struct {
		id      int64
		content string
	}
	query := s.db.Rebind(`
		SELECT id, content
		FROM wiki_page_versions
		WHERE COALESCE(content_encoding, 'plain') = 'plain'
		  AND content <> ''
	`)
	legacyRows, err := s.db.QueryContext(ctx, query)
	if err != nil {
		s.logger.Warn("Failed to list plaintext wiki versions for compression", zap.Error(err))
		return
	}
	defer legacyRows.Close()

	legacy := make([]legacyVersion, 0)
	for legacyRows.Next() {
		var v legacyVersion
		if err := legacyRows.Scan(&v.id, &v.content); err != nil {
			s.logger.Warn("Failed to scan plaintext wiki version", zap.Error(err))
			continue
		}
		legacy = append(legacy, v)
	}
	if err := legacyRows.Err(); err != nil {
		s.logger.Warn("Failed while scanning plaintext wiki versions", zap.Error(err))
		return
	}

	compressedCount := 0
	for _, v := range legacy {
		compressed, err := compressWikiPageVersionContent(v.content)
		if err != nil {
			s.logger.Warn("Failed to compress wiki version", zap.Int64("version_id", v.id), zap.Error(err))
			continue
		}
		updateQuery := s.db.Rebind(`
			UPDATE wiki_page_versions
			SET content = '', content_encoding = ?, content_compressed = ?
			WHERE id = ?
		`)
		if _, err := s.db.ExecContext(ctx, updateQuery, wikiPageVersionEncodingGzip, compressed, v.id); err != nil {
			s.logger.Warn("Failed to store compressed wiki version", zap.Int64("version_id", v.id), zap.Error(err))
			continue
		}
		compressedCount++
	}

	s.logger.Info("Optimized wiki page version storage",
		zap.Int("pages_pruned", prunedPages),
		zap.Int("versions_compressed", compressedCount),
		zap.Int("retention_limit", wikiPageVersionRetentionLimit),
	)
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

	// Re-parent children to the deleted page's parent so no content is lost.
	reparent := s.db.Client.WikiPage.Update().Where(wikipage.ParentID(pageID))
	if page.ParentID == nil {
		reparent.ClearParentID()
	} else {
		reparent.SetParentID(*page.ParentID)
	}
	if _, err := reparent.Save(ctx); err != nil {
		s.logger.Error("Failed to re-parent child wiki pages",
			zap.Int64("page_id", pageID),
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to delete wiki page", "internal_error")
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
