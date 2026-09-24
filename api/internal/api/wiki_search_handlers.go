package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"

	"taskai/ent"
	"taskai/ent/projectmember"
	"taskai/ent/wikiblock"
	"taskai/ent/wikipage"
)

// SearchWikiRequest represents a wiki search request
type SearchWikiRequest struct {
	Query       string  `json:"query"`
	ProjectID   *int64  `json:"project_id,omitempty"`
	ProjectIDs  []int64 `json:"project_ids,omitempty"` // search across multiple projects
	Limit       int     `json:"limit,omitempty"`
	RecencyDays *int    `json:"recency_days,omitempty"`
	Mode        string  `json:"mode,omitempty"` // "keyword" (ILIKE), "fts" (tsvector), "semantic" (vector), "hybrid" (fts+vector). Default: "hybrid"
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
		req.Limit = 10
	}
	if req.Limit > 100 {
		req.Limit = 100
	}

	// Default mode to "hybrid" (best accuracy: FTS + vector similarity via RRF).
	// Falls back to "fts" if embedding client is unavailable, "keyword" on SQLite.
	if req.Mode == "" {
		if s.db.Driver == "postgres" {
			if s.embeddingClient != nil {
				req.Mode = "hybrid"
			} else {
				req.Mode = "fts"
			}
		} else {
			req.Mode = "keyword"
		}
	}

	s.logger.Debug("Wiki search request",
		zap.String("query", req.Query),
		zap.String("mode", req.Mode),
		zap.Int64("user_id", userID),
	)

	var results []SearchResultBlock
	var err error

	switch req.Mode {
	case "semantic":
		results, err = s.searchWikiSemantic(ctx, userID, req)
	case "hybrid":
		results, err = s.searchWikiHybrid(ctx, userID, req)
	case "keyword":
		results, err = s.searchWikiBlocks(ctx, userID, req)
	default: // "fts"
		results, err = s.searchWikiFTS(ctx, userID, req)
		if err == nil && len(results) == 0 {
			// Fall back to ILIKE if FTS returns nothing (handles partial words)
			results, err = s.searchWikiBlocks(ctx, userID, req)
		}
	}
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

	// Filter by resolved project IDs (respects project_id, project_ids, or all accessible)
	effectiveProjects := resolveProjectIDs(req, accessibleProjects)
	query = query.Where(wikiblock.HasPageWith(wikipage.ProjectIDIn(effectiveProjects...), wikiVisiblePredicate(userID)))

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

// searchWikiFTS uses PostgreSQL tsvector full-text search with ranking.
func (s *Server) searchWikiFTS(ctx context.Context, userID int64, req SearchWikiRequest) ([]SearchResultBlock, error) {
	accessibleProjects, err := s.getUserAccessibleProjects(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(accessibleProjects) == 0 {
		return []SearchResultBlock{}, nil
	}

	// Build project filter
	effectiveProjects := resolveProjectIDs(req, accessibleProjects)
	projectFilter, args := buildProjectFilterFromIDs(effectiveProjects, userID)

	// plainto_tsquery handles user input safely (no special syntax needed)
	nextArg := len(args) + 1
	query := fmt.Sprintf(`
		SELECT wb.id, wb.page_id, wb.block_type, wb.headings_path, wb.plain_text,
		       wp.title AS page_title, wp.slug AS page_slug,
		       ts_rank(wb.search_vector, plainto_tsquery('english', $%d)) AS rank
		FROM wiki_blocks wb
		JOIN wiki_pages wp ON wb.page_id = wp.id
		WHERE wb.search_vector @@ plainto_tsquery('english', $%d)
		  AND %s
	`, nextArg, nextArg, projectFilter)
	args = append(args, req.Query)
	nextArg++

	if req.RecencyDays != nil && *req.RecencyDays > 0 {
		query += fmt.Sprintf(` AND wp.updated_at >= $%d`, nextArg)
		args = append(args, time.Now().AddDate(0, 0, -*req.RecencyDays))
	}

	query += ` ORDER BY rank DESC LIMIT ` + strconv.Itoa(req.Limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSearchResults(rows)
}

// searchWikiSemantic uses pgvector cosine similarity search.
func (s *Server) searchWikiSemantic(ctx context.Context, userID int64, req SearchWikiRequest) ([]SearchResultBlock, error) {
	if s.embeddingClient == nil {
		// Fall back to FTS if embeddings are not available
		return s.searchWikiFTS(ctx, userID, req)
	}

	accessibleProjects, err := s.getUserAccessibleProjects(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(accessibleProjects) == 0 {
		return []SearchResultBlock{}, nil
	}

	// Embed the query
	queryVector, err := s.embeddingClient.Embed(ctx, req.Query)
	if err != nil {
		s.logger.Warn("Embedding query failed, falling back to FTS",
			zap.Error(err),
		)
		return s.searchWikiFTS(ctx, userID, req)
	}

	vectorStr := float32SliceToVectorString(queryVector)

	// Build project filter
	effectiveProjects := resolveProjectIDs(req, accessibleProjects)
	projectFilter, args := buildProjectFilterFromIDs(effectiveProjects, userID)
	nextArg := len(args) + 1

	query := fmt.Sprintf(`
		SELECT wb.id, wb.page_id, wb.block_type, wb.headings_path, wb.plain_text,
		       wp.title AS page_title, wp.slug AS page_slug,
		       1 - (wb.embedding <=> $%d::vector) AS rank
		FROM wiki_blocks wb
		JOIN wiki_pages wp ON wb.page_id = wp.id
		WHERE wb.embedding IS NOT NULL
		  AND %s
	`, nextArg, projectFilter)
	args = append(args, vectorStr)
	nextArg++

	if req.RecencyDays != nil && *req.RecencyDays > 0 {
		query += fmt.Sprintf(` AND wp.updated_at >= $%d`, nextArg)
		args = append(args, time.Now().AddDate(0, 0, -*req.RecencyDays))
	}

	query += ` ORDER BY rank DESC LIMIT ` + strconv.Itoa(req.Limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSearchResults(rows)
}

// searchWikiHybrid combines FTS and vector search with score fusion.
func (s *Server) searchWikiHybrid(ctx context.Context, userID int64, req SearchWikiRequest) ([]SearchResultBlock, error) {
	if s.embeddingClient == nil {
		return s.searchWikiFTS(ctx, userID, req)
	}

	accessibleProjects, err := s.getUserAccessibleProjects(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(accessibleProjects) == 0 {
		return []SearchResultBlock{}, nil
	}

	// Embed the query
	queryVector, err := s.embeddingClient.Embed(ctx, req.Query)
	if err != nil {
		s.logger.Warn("Embedding query failed, falling back to FTS",
			zap.Error(err),
		)
		return s.searchWikiFTS(ctx, userID, req)
	}

	vectorStr := float32SliceToVectorString(queryVector)

	// Build project filter
	effectiveProjects := resolveProjectIDs(req, accessibleProjects)
	projectFilter, args := buildProjectFilterFromIDs(effectiveProjects, userID)
	nextArg := len(args) + 1

	// Reciprocal Rank Fusion (RRF): merges FTS and vector rankings without needing score normalization.
	// rrf_score = 1/(k+fts_rank) + 1/(k+vector_rank), k=60 is standard.
	query := fmt.Sprintf(`
		WITH fts AS (
			SELECT wb.id, ROW_NUMBER() OVER (ORDER BY ts_rank(wb.search_vector, plainto_tsquery('english', $%d)) DESC) AS rn
			FROM wiki_blocks wb
			JOIN wiki_pages wp ON wb.page_id = wp.id
			WHERE wb.search_vector @@ plainto_tsquery('english', $%d)
			  AND %s
		`, nextArg, nextArg, projectFilter)
	args = append(args, req.Query)
	nextArg++

	if req.RecencyDays != nil && *req.RecencyDays > 0 {
		query += fmt.Sprintf(` AND wp.updated_at >= $%d`, nextArg)
		args = append(args, time.Now().AddDate(0, 0, -*req.RecencyDays))
		nextArg++
	}

	query += fmt.Sprintf(`
			LIMIT %d
		),
		vec AS (
			SELECT wb.id, ROW_NUMBER() OVER (ORDER BY wb.embedding <=> $%d::vector) AS rn
			FROM wiki_blocks wb
			JOIN wiki_pages wp ON wb.page_id = wp.id
			WHERE wb.embedding IS NOT NULL
			  AND %s
	`, req.Limit*2, nextArg, projectFilter)
	args = append(args, vectorStr)
	nextArg++

	if req.RecencyDays != nil && *req.RecencyDays > 0 {
		query += fmt.Sprintf(` AND wp.updated_at >= $%d`, nextArg)
		args = append(args, time.Now().AddDate(0, 0, -*req.RecencyDays))
	}

	query += fmt.Sprintf(`
			LIMIT %d
		),
		combined AS (
			SELECT COALESCE(fts.id, vec.id) AS id,
			       COALESCE(1.0/(60+fts.rn), 0) + COALESCE(1.0/(60+vec.rn), 0) AS rrf_score
			FROM fts
			FULL OUTER JOIN vec ON fts.id = vec.id
		)
		SELECT wb.id, wb.page_id, wb.block_type, wb.headings_path, wb.plain_text,
		       wp.title AS page_title, wp.slug AS page_slug,
		       c.rrf_score AS rank
		FROM combined c
		JOIN wiki_blocks wb ON wb.id = c.id
		JOIN wiki_pages wp ON wb.page_id = wp.id
		ORDER BY c.rrf_score DESC
		LIMIT %d
	`, req.Limit*2, req.Limit)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanSearchResults(rows)
}

// resolveProjectIDs returns the effective project IDs for a search request,
// intersecting any explicit project_id/project_ids with the user's accessible projects.
func resolveProjectIDs(req SearchWikiRequest, accessibleProjects []int64) []int64 {
	if req.ProjectID != nil {
		return []int64{*req.ProjectID}
	}
	if len(req.ProjectIDs) > 0 {
		// Intersect requested IDs with accessible IDs
		accessSet := make(map[int64]bool, len(accessibleProjects))
		for _, id := range accessibleProjects {
			accessSet[id] = true
		}
		filtered := make([]int64, 0, len(req.ProjectIDs))
		for _, id := range req.ProjectIDs {
			if accessSet[id] {
				filtered = append(filtered, id)
			}
		}
		if len(filtered) > 0 {
			return filtered
		}
		// None of the requested IDs are accessible — fall through to all accessible
	}
	return accessibleProjects
}

// buildProjectFilterFromIDs returns a SQL WHERE clause limiting wiki pages
// (aliased wp) to the given projects and to pages the user may see.
func buildProjectFilterFromIDs(projectIDs []int64, userID int64) (string, []interface{}) {
	placeholders := make([]string, len(projectIDs))
	args := make([]interface{}, 0, len(projectIDs)+1)
	for i, pid := range projectIDs {
		placeholders[i] = fmt.Sprintf("$%d", i+1)
		args = append(args, pid)
	}
	args = append(args, userID)
	userParam := fmt.Sprintf("$%d", len(args))
	return fmt.Sprintf("wp.project_id IN (%s) AND %s", strings.Join(placeholders, ","), wikiVisibleSQL("wp", userParam)), args
}

// scanSearchResults scans rows from FTS/semantic/hybrid queries into SearchResultBlock slices.
func scanSearchResults(rows *sql.Rows) ([]SearchResultBlock, error) {
	var results []SearchResultBlock
	for rows.Next() {
		var (
			blockID      int64
			pageID       int64
			blockType    string
			headingsPath sql.NullString
			plainText    sql.NullString
			pageTitle    string
			pageSlug     string
			rank         float64
		)
		if err := rows.Scan(&blockID, &pageID, &blockType, &headingsPath, &plainText, &pageTitle, &pageSlug, &rank); err != nil {
			return nil, err
		}
		snippet := plainText.String
		if len(snippet) > 200 {
			snippet = snippet[:200] + "..."
		}
		results = append(results, SearchResultBlock{
			PageID:       pageID,
			PageTitle:    pageTitle,
			PageSlug:     pageSlug,
			BlockID:      blockID,
			BlockType:    blockType,
			HeadingsPath: headingsPath.String,
			Snippet:      snippet,
			Rank:         rank,
		})
	}
	if results == nil {
		results = []SearchResultBlock{}
	}
	return results, rows.Err()
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

	// Only projects the user belongs to, narrowed to one project if asked,
	// and only pages visible to them.
	pageQuery = pageQuery.Where(wikipage.ProjectIDIn(accessibleProjects...), wikiVisiblePredicate(userID))
	if projectID != nil {
		pageQuery = pageQuery.Where(wikipage.ProjectID(*projectID))
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
