package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"

	"taskai/ent"
	"taskai/ent/project"
	"taskai/ent/task"
	"taskai/ent/wikiblock"
	"taskai/ent/wikipage"
)

// GlobalSearchRequest represents a global search request
type GlobalSearchRequest struct {
	Query     string   `json:"query"`
	ProjectID *int64   `json:"project_id,omitempty"`
	Types     []string `json:"types,omitempty"`
	Limit     int      `json:"limit,omitempty"`
}

// SearchTaskResult represents a task in global search results
type SearchTaskResult struct {
	ID                int64  `json:"id"`
	ProjectID         int64  `json:"project_id"`
	ProjectName       string `json:"project_name"`
	TaskNumber        int    `json:"task_number"`
	Title             string `json:"title"`
	Snippet           string `json:"snippet"`
	Status            string `json:"status"`
	Priority          string `json:"priority"`
	GithubIssueNumber *int    `json:"github_issue_number,omitempty"`
	GithubRepo        string  `json:"github_repo,omitempty"`
}

// GlobalSearchWikiResult represents a wiki page in global search results
type GlobalSearchWikiResult struct {
	PageID       int64  `json:"page_id"`
	PageTitle    string `json:"page_title"`
	PageSlug     string `json:"page_slug"`
	ProjectID    int64  `json:"project_id"`
	ProjectName  string `json:"project_name"`
	Snippet      string `json:"snippet"`
	HeadingsPath string `json:"headings_path,omitempty"`
}

// GlobalSearchResponse represents the global search response
type GlobalSearchResponse struct {
	Tasks []SearchTaskResult       `json:"tasks"`
	Wiki  []GlobalSearchWikiResult `json:"wiki"`
}

// resolveSearchTypes determines which entity types to search based on the request.
func resolveSearchTypes(types []string) (searchTasks, searchWiki bool) {
	if len(types) == 0 {
		return true, true
	}
	for _, t := range types {
		switch t {
		case "tasks":
			searchTasks = true
		case "wiki":
			searchWiki = true
		}
	}
	return searchTasks, searchWiki
}

// normalizeSearchLimit clamps the limit to [1, 50] with a default of 10.
func normalizeSearchLimit(limit int) int {
	if limit <= 0 {
		return 10
	}
	if limit > 50 {
		return 50
	}
	return limit
}

// executeParallelSearch runs task and wiki searches concurrently and assembles the response.
func (s *Server) executeParallelSearch(ctx context.Context, req GlobalSearchRequest, searchTasks, searchWiki bool, accessibleProjects []int64, projectNameMap map[int64]string) GlobalSearchResponse {
	var (
		taskResults []SearchTaskResult
		wikiResults []GlobalSearchWikiResult
		taskErr     error
		wikiErr     error
		wg          sync.WaitGroup
	)

	if searchTasks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			taskResults, taskErr = s.searchTasks(ctx, req, accessibleProjects, projectNameMap)
		}()
	}

	if searchWiki {
		wg.Add(1)
		go func() {
			defer wg.Done()
			wikiResults, wikiErr = s.searchWikiForGlobal(ctx, req, accessibleProjects, projectNameMap)
		}()
	}

	wg.Wait()

	if taskErr != nil {
		s.logger.Error("Failed to search tasks", zap.Error(taskErr), zap.String("query", req.Query))
	}
	if wikiErr != nil {
		s.logger.Error("Failed to search wiki", zap.Error(wikiErr), zap.String("query", req.Query))
	}

	response := GlobalSearchResponse{
		Tasks: taskResults,
		Wiki:  wikiResults,
	}
	if response.Tasks == nil {
		response.Tasks = []SearchTaskResult{}
	}
	if response.Wiki == nil {
		response.Wiki = []GlobalSearchWikiResult{}
	}
	return response
}

// HandleGlobalSearch performs search across tasks and wiki pages
func (s *Server) HandleGlobalSearch(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	userID, ok := ctx.Value(UserIDKey).(int64)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req GlobalSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_request")
		return
	}

	if req.Query == "" {
		respondError(w, http.StatusBadRequest, "query parameter is required", "invalid_request")
		return
	}

	req.Limit = normalizeSearchLimit(req.Limit)
	searchTasks, searchWiki := resolveSearchTypes(req.Types)

	s.logger.Debug("Global search request",
		zap.String("query", req.Query),
		zap.Int64("user_id", userID),
		zap.Bool("search_tasks", searchTasks),
		zap.Bool("search_wiki", searchWiki),
	)

	// Get user's accessible project IDs
	accessibleProjects, err := s.getUserAccessibleProjects(ctx, userID)
	if err != nil {
		s.logger.Error("Failed to get accessible projects", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to search", "internal_error")
		return
	}

	if len(accessibleProjects) == 0 {
		respondJSON(w, http.StatusOK, GlobalSearchResponse{
			Tasks: []SearchTaskResult{},
			Wiki:  []GlobalSearchWikiResult{},
		})
		return
	}

	// Build project name lookup map
	projectNameMap, err := s.buildProjectNameMap(ctx, accessibleProjects)
	if err != nil {
		s.logger.Error("Failed to load project names", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to search", "internal_error")
		return
	}

	response := s.executeParallelSearch(ctx, req, searchTasks, searchWiki, accessibleProjects, projectNameMap)
	respondJSON(w, http.StatusOK, response)
}

// searchTasks searches for tasks matching the query, using Postgres FTS when available
func (s *Server) searchTasks(ctx context.Context, req GlobalSearchRequest, accessibleProjects []int64, projectNameMap map[int64]string) ([]SearchTaskResult, error) {
	if s.config.DBDriver == "postgres" {
		return s.searchTasksPostgres(ctx, req, accessibleProjects, projectNameMap)
	}
	return s.searchTasksSQLite(ctx, req, accessibleProjects, projectNameMap)
}

// searchTasksSQLite uses Ent ORM with ContainsFold (LIKE) for SQLite
func (s *Server) searchTasksSQLite(ctx context.Context, req GlobalSearchRequest, accessibleProjects []int64, projectNameMap map[int64]string) ([]SearchTaskResult, error) {
	query := s.db.Client.Task.Query().
		Where(
			task.Or(
				task.TitleContainsFold(req.Query),
				task.DescriptionContainsFold(req.Query),
			),
		)

	// Filter by project
	if req.ProjectID != nil {
		query = query.Where(task.ProjectID(*req.ProjectID))
	} else {
		query = query.Where(task.ProjectIDIn(accessibleProjects...))
	}

	tasks, err := query.
		Limit(req.Limit).
		Order(ent.Desc(task.FieldUpdatedAt)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	return mapTaskResults(tasks, projectNameMap), nil
}

// searchTasksPostgres uses tsvector + GIN index with ts_rank for relevance ordering
func (s *Server) searchTasksPostgres(ctx context.Context, req GlobalSearchRequest, accessibleProjects []int64, projectNameMap map[int64]string) ([]SearchTaskResult, error) {
	// Build parameterized query
	// $1 = query (for FTS), $2 = query (for ILIKE fallback), $3 = limit
	args := []interface{}{req.Query, req.Query, req.Limit}

	// Check if query looks like a GitHub issue number for exact match
	issueNumFilter := ""
	var issueNum int
	if _, err := fmt.Sscanf(req.Query, "%d", &issueNum); err == nil && strings.TrimSpace(req.Query) == fmt.Sprintf("%d", issueNum) {
		issueNumFilter = fmt.Sprintf("OR t.github_issue_number = $%d", len(args)+1)
		args = append(args, issueNum)
	}

	projectArgStart := len(args) + 1
	var projectFilter string
	if req.ProjectID != nil {
		projectFilter = fmt.Sprintf("AND t.project_id = $%d", projectArgStart)
		args = append(args, *req.ProjectID)
	} else {
		placeholders := make([]string, len(accessibleProjects))
		for i, pid := range accessibleProjects {
			placeholders[i] = fmt.Sprintf("$%d", projectArgStart+i)
			args = append(args, pid)
		}
		projectFilter = fmt.Sprintf("AND t.project_id IN (%s)", strings.Join(placeholders, ","))
	}

	sqlQuery := fmt.Sprintf(`
		SELECT t.id, t.project_id, t.task_number, t.title, t.description, t.status, t.priority,
		       t.github_issue_number, t.github_repo,
		       ts_rank(t.search_vector, plainto_tsquery('english', $1)) AS rank
		FROM tasks t
		WHERE (
			t.search_vector @@ plainto_tsquery('english', $1)
			OR t.title ILIKE '%%' || $2 || '%%'
			%s
		)
		%s
		ORDER BY rank DESC, t.updated_at DESC
		LIMIT $3
	`, issueNumFilter, projectFilter)

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres task search: %w", err)
	}
	defer rows.Close()

	results := make([]SearchTaskResult, 0)
	for rows.Next() {
		var (
			id                int64
			projectID         int64
			taskNumber        sql.NullInt32
			title             string
			description       sql.NullString
			status            string
			priority          string
			githubIssueNumber sql.NullInt32
			githubRepo        sql.NullString
			rank              float64
		)
		if err := rows.Scan(&id, &projectID, &taskNumber, &title, &description, &status, &priority, &githubIssueNumber, &githubRepo, &rank); err != nil {
			return nil, fmt.Errorf("scan task row: %w", err)
		}

		snippet := ""
		if description.Valid {
			snippet = description.String
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
		}

		tn := 0
		if taskNumber.Valid {
			tn = int(taskNumber.Int32)
		}

		var ghNum *int
		if githubIssueNumber.Valid {
			n := int(githubIssueNumber.Int32)
			ghNum = &n
		}

		ghRepoStr := ""
		if githubRepo.Valid {
			ghRepoStr = githubRepo.String
		}

		results = append(results, SearchTaskResult{
			ID:                id,
			ProjectID:         projectID,
			ProjectName:       projectNameMap[projectID],
			TaskNumber:        tn,
			Title:             title,
			Snippet:           snippet,
			Status:            status,
			Priority:          priority,
			GithubIssueNumber: ghNum,
			GithubRepo:        ghRepoStr,
		})
	}

	return results, rows.Err()
}

// searchWikiForGlobal searches wiki blocks, using Postgres FTS when available
func (s *Server) searchWikiForGlobal(ctx context.Context, req GlobalSearchRequest, accessibleProjects []int64, projectNameMap map[int64]string) ([]GlobalSearchWikiResult, error) {
	if s.config.DBDriver == "postgres" {
		return s.searchWikiPostgres(ctx, req, accessibleProjects, projectNameMap)
	}
	return s.searchWikiSQLite(ctx, req, accessibleProjects, projectNameMap)
}

// searchWikiSQLite uses Ent ORM with ContainsFold (LIKE) for SQLite
func (s *Server) searchWikiSQLite(ctx context.Context, req GlobalSearchRequest, accessibleProjects []int64, projectNameMap map[int64]string) ([]GlobalSearchWikiResult, error) {
	query := s.db.Client.WikiBlock.Query().
		// Select only columns that exist in both SQLite and Postgres (excludes search_text/search_vector)
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

	// Filter by project
	if req.ProjectID != nil {
		query = query.Where(wikiblock.HasPageWith(wikipage.ProjectID(*req.ProjectID)))
	} else {
		query = query.Where(wikiblock.HasPageWith(wikipage.ProjectIDIn(accessibleProjects...)))
	}

	// Use ContainsFold for case-insensitive search (generates ILIKE on Postgres)
	query = query.Where(wikiblock.Or(
		wikiblock.PlainTextContainsFold(req.Query),
		wikiblock.HeadingsPathContainsFold(req.Query),
	))

	blocks, err := query.
		Limit(req.Limit).
		All(ctx)
	if err != nil {
		return nil, err
	}

	return mapWikiResults(blocks, projectNameMap), nil
}

// searchWikiPostgres uses tsvector + GIN index with ts_rank for relevance ordering
func (s *Server) searchWikiPostgres(ctx context.Context, req GlobalSearchRequest, accessibleProjects []int64, projectNameMap map[int64]string) ([]GlobalSearchWikiResult, error) {
	// $1 = query (for FTS), $2 = query (for ILIKE fallback), $3 = limit
	args := []interface{}{req.Query, req.Query, req.Limit}

	var projectFilter string
	if req.ProjectID != nil {
		projectFilter = "AND wp.project_id = $4"
		args = append(args, *req.ProjectID)
	} else {
		placeholders := make([]string, len(accessibleProjects))
		for i, pid := range accessibleProjects {
			placeholders[i] = fmt.Sprintf("$%d", 4+i)
			args = append(args, pid)
		}
		projectFilter = fmt.Sprintf("AND wp.project_id IN (%s)", strings.Join(placeholders, ","))
	}

	// Use DISTINCT ON to return one result per wiki page (highest-ranking block)
	sqlQuery := fmt.Sprintf(`
		SELECT DISTINCT ON (wp.id)
			wp.id AS page_id, wp.title AS page_title, wp.slug AS page_slug,
			wp.project_id, wb.plain_text, wb.headings_path,
			ts_rank(wb.search_vector, plainto_tsquery('english', $1)) AS rank
		FROM wiki_blocks wb
		JOIN wiki_pages wp ON wb.page_id = wp.id
		WHERE (
			wb.search_vector @@ plainto_tsquery('english', $1)
			OR wb.plain_text ILIKE '%%' || $2 || '%%'
		)
		%s
		ORDER BY wp.id, rank DESC
		LIMIT $3
	`, projectFilter)

	rows, err := s.db.QueryContext(ctx, sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("postgres wiki search: %w", err)
	}
	defer rows.Close()

	results := make([]GlobalSearchWikiResult, 0)
	for rows.Next() {
		var (
			pageID       int64
			pageTitle    string
			pageSlug     string
			projectID    int64
			plainText    sql.NullString
			headingsPath sql.NullString
			rank         float64
		)
		if err := rows.Scan(&pageID, &pageTitle, &pageSlug, &projectID, &plainText, &headingsPath, &rank); err != nil {
			return nil, fmt.Errorf("scan wiki row: %w", err)
		}

		snippet := ""
		if plainText.Valid {
			snippet = plainText.String
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
		}

		hp := ""
		if headingsPath.Valid {
			hp = headingsPath.String
		}

		results = append(results, GlobalSearchWikiResult{
			PageID:       pageID,
			PageTitle:    pageTitle,
			PageSlug:     pageSlug,
			ProjectID:    projectID,
			ProjectName:  projectNameMap[projectID],
			Snippet:      snippet,
			HeadingsPath: hp,
		})
	}

	return results, rows.Err()
}

// mapTaskResults converts Ent task entities to SearchTaskResult slice
func mapTaskResults(tasks []*ent.Task, projectNameMap map[int64]string) []SearchTaskResult {
	results := make([]SearchTaskResult, 0, len(tasks))
	for _, t := range tasks {
		snippet := ""
		if t.Description != nil {
			snippet = *t.Description
			if len(snippet) > 200 {
				snippet = snippet[:200] + "..."
			}
		}

		taskNumber := 0
		if t.TaskNumber != nil {
			taskNumber = *t.TaskNumber
		}

		results = append(results, SearchTaskResult{
			ID:          t.ID,
			ProjectID:   t.ProjectID,
			ProjectName: projectNameMap[t.ProjectID],
			TaskNumber:  taskNumber,
			Title:       t.Title,
			Snippet:     snippet,
			Status:      t.Status,
			Priority:    t.Priority,
		})
	}
	return results
}

// mapWikiResults converts Ent wiki block entities to GlobalSearchWikiResult slice
func mapWikiResults(blocks []*ent.WikiBlock, projectNameMap map[int64]string) []GlobalSearchWikiResult {
	results := make([]GlobalSearchWikiResult, 0, len(blocks))
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

		results = append(results, GlobalSearchWikiResult{
			PageID:       page.ID,
			PageTitle:    page.Title,
			PageSlug:     page.Slug,
			ProjectID:    page.ProjectID,
			ProjectName:  projectNameMap[page.ProjectID],
			Snippet:      snippet,
			HeadingsPath: headingsPath,
		})
	}
	return results
}

// buildProjectNameMap loads project names for the given IDs into a map
func (s *Server) buildProjectNameMap(ctx context.Context, projectIDs []int64) (map[int64]string, error) {
	projects, err := s.db.Client.Project.Query().
		Where(project.IDIn(projectIDs...)).
		All(ctx)
	if err != nil {
		return nil, err
	}

	nameMap := make(map[int64]string, len(projects))
	for _, p := range projects {
		nameMap[p.ID] = p.Name
	}

	return nameMap, nil
}
