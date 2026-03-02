package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"go.uber.org/zap"

	"taskai/ent"
	"taskai/ent/projectmember"
	"taskai/ent/wikiblock"
	"taskai/ent/wikipage"
)

// SearchWikiRequest represents a wiki search request
type SearchWikiRequest struct {
	Query       string `json:"query"`
	ProjectID   *int64 `json:"project_id,omitempty"`
	Limit       int    `json:"limit,omitempty"`
	RecencyDays *int   `json:"recency_days,omitempty"`
}

// SearchResultBlock represents a search result block
type SearchResultBlock struct {
	PageID       int64   `json:"page_id"`
	PageTitle    string  `json:"page_title"`
	PageSlug     string  `json:"page_slug"`
	BlockID      int64   `json:"block_id"`
	BlockType    string  `json:"block_type"`
	HeadingsPath string  `json:"headings_path,omitempty"`
	Snippet      string  `json:"snippet"`
	Rank         float64 `json:"rank,omitempty"`
}

// SearchWikiResponse represents a wiki search response
type SearchWikiResponse struct {
	Results []SearchResultBlock `json:"results"`
	Total   int                 `json:"total"`
}

// HandleSearchWiki performs full-text search across wiki content
func (s *Server) HandleSearchWiki(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	userID, ok := ctx.Value(UserIDKey).(int64)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req SearchWikiRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}

	if req.Query == "" {
		respondError(w, http.StatusBadRequest, "query parameter is required", "invalid_request")
		return
	}

	// Set defaults
	if req.Limit == 0 {
		req.Limit = 20
	}
	if req.Limit > 100 {
		req.Limit = 100
	}

	s.logger.Debug("Wiki search request",
		zap.String("query", req.Query),
		zap.Int64("user_id", userID),
	)

	// Build query based on database driver
	results, err := s.searchWikiBlocks(ctx, userID, req)
	if err != nil {
		s.logger.Error("Failed to search wiki",
			zap.Error(err),
			zap.String("query", req.Query),
		)
		respondError(w, http.StatusInternalServerError, "failed to search wiki", "internal_error")
		return
	}

	response := SearchWikiResponse{
		Results: results,
		Total:   len(results),
	}

	respondJSON(w, http.StatusOK, response)
}

// searchWikiBlocks performs the actual search query
func (s *Server) searchWikiBlocks(ctx context.Context, userID int64, req SearchWikiRequest) ([]SearchResultBlock, error) {
	// Get user's accessible project IDs
	accessibleProjects, err := s.getUserAccessibleProjects(ctx, userID)
	if err != nil {
		return nil, err
	}

	if len(accessibleProjects) == 0 {
		return []SearchResultBlock{}, nil
	}

	// Build base query — select only columns that exist in both SQLite and Postgres
	query := s.db.Client.WikiBlock.Query().
		Select(
			wikiblock.FieldID,
			wikiblock.FieldPageID,
			wikiblock.FieldBlockType,
			wikiblock.FieldLevel,
			wikiblock.FieldHeadingsPath,
			wikiblock.FieldPlainText,
			wikiblock.FieldPosition,
		).
		WithPage(func(q *ent.WikiPageQuery) {
			q.Select(wikipage.FieldID, wikipage.FieldTitle, wikipage.FieldSlug, wikipage.FieldProjectID)
		})

	// Filter by project if specified
	if req.ProjectID != nil {
		query = query.Where(wikiblock.HasPageWith(wikipage.ProjectID(*req.ProjectID)))
	} else {
		// Filter by accessible projects
		query = query.Where(wikiblock.HasPageWith(wikipage.ProjectIDIn(accessibleProjects...)))
	}

	// Use ContainsFold for case-insensitive search (generates ILIKE on Postgres)
	query = query.Where(wikiblock.Or(
		wikiblock.PlainTextContainsFold(req.Query),
		wikiblock.HeadingsPathContainsFold(req.Query),
	))

	// Apply recency filter if specified
	if req.RecencyDays != nil && *req.RecencyDays > 0 {
		cutoff := time.Now().AddDate(0, 0, -*req.RecencyDays)
		query = query.Where(wikiblock.HasPageWith(wikipage.UpdatedAtGTE(cutoff)))
	}

	// Execute query with limit
	blocks, err := query.
		Limit(req.Limit).
		All(ctx)
	if err != nil {
		return nil, err
	}

	// Convert to response format
	results := make([]SearchResultBlock, 0, len(blocks))
	for _, block := range blocks {
		page := block.Edges.Page
		if page == nil {
			continue
		}

		snippet := ""
		if block.PlainText != nil {
			snippet = *block.PlainText
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
		}

		headingsPath := ""
		if block.HeadingsPath != nil {
			headingsPath = *block.HeadingsPath
		}

		results = append(results, SearchResultBlock{
			PageID:       page.ID,
			PageTitle:    page.Title,
			PageSlug:     page.Slug,
			BlockID:      block.ID,
			BlockType:    block.BlockType,
			HeadingsPath: headingsPath,
			Snippet:      snippet,
		})
	}

	return results, nil
}

// AutocompletePageRequest represents a page autocomplete request
type AutocompletePageRequest struct {
	Query     string `json:"query"`
	ProjectID *int64 `json:"project_id,omitempty"`
	Limit     int    `json:"limit,omitempty"`
}

// AutocompletePageResult represents an autocomplete result
type AutocompletePageResult struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Slug  string `json:"slug"`
}

// HandleAutocompletePages provides fuzzy page title autocomplete
func (s *Server) HandleAutocompletePages(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := ctx.Value(UserIDKey).(int64)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	// Parse query parameters
	query := r.URL.Query().Get("query")
	if query == "" {
		respondJSON(w, http.StatusOK, []AutocompletePageResult{})
		return
	}

	limit := 10
	if limitStr := r.URL.Query().Get("limit"); limitStr != "" {
		if parsedLimit, err := strconv.Atoi(limitStr); err == nil && parsedLimit > 0 {
			limit = parsedLimit
			if limit > 50 {
				limit = 50
			}
		}
	}

	var projectID *int64
	if projectIDStr := r.URL.Query().Get("project_id"); projectIDStr != "" {
		if parsedID, err := strconv.ParseInt(projectIDStr, 10, 64); err == nil {
			projectID = &parsedID
		}
	}

	// Get user's accessible projects
	accessibleProjects, err := s.getUserAccessibleProjects(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get accessible projects",
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to get accessible projects", "internal_error")
		return
	}

	if len(accessibleProjects) == 0 {
		respondJSON(w, http.StatusOK, []AutocompletePageResult{})
		return
	}

	// Build query
	pageQuery := s.db.Client.WikiPage.Query()

	// Filter by project if specified
	if projectID != nil {
		pageQuery = pageQuery.Where(wikipage.ProjectID(*projectID))
	} else {
		pageQuery = pageQuery.Where(wikipage.ProjectIDIn(accessibleProjects...))
	}

	// Simple title contains search (can be enhanced with trigram similarity for Postgres)
	pageQuery = pageQuery.Where(wikipage.TitleContains(query))

	// Execute query
	pages, err := pageQuery.
		Limit(limit).
		Order(ent.Asc(wikipage.FieldTitle)).
		All(ctx)
	if err != nil {
		s.logger.Error("Failed to autocomplete pages",
			zap.Error(err),
		)
		respondError(w, http.StatusInternalServerError, "failed to autocomplete pages", "internal_error")
		return
	}

	// Convert to response format
	results := make([]AutocompletePageResult, len(pages))
	for i, page := range pages {
		results[i] = AutocompletePageResult{
			ID:    page.ID,
			Title: page.Title,
			Slug:  page.Slug,
		}
	}

	respondJSON(w, http.StatusOK, results)
}

// getUserAccessibleProjects returns the list of project IDs the user has access to
// via a single query on the project_members table (avoids N+1 queries).
func (s *Server) getUserAccessibleProjects(ctx context.Context, userID int64) ([]int64, error) {
	members, err := s.db.Client.ProjectMember.Query().
		Where(projectmember.UserID(userID)).
		Select(projectmember.FieldProjectID).
		All(ctx)
	if err != nil {
		return nil, err
	}

	projectIDs := make([]int64, 0, len(members))
	for _, m := range members {
		projectIDs = append(projectIDs, m.ProjectID)
	}

	return projectIDs, nil
}
