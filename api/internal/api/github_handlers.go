package api

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// --- GitHub API response types ---

type ghMilestone struct {
	Number int    `json:"number"`
	Title  string `json:"title"`
	State  string `json:"state"`
	DueOn  string `json:"due_on"`
}

type ghLabel struct {
	Name  string `json:"name"`
	Color string `json:"color"`
}

type ghUser struct {
	Login string `json:"login"`
	Name  string `json:"name"`
}

type ghIssue struct {
	Number      int          `json:"number"`
	Title       string       `json:"title"`
	Body        string       `json:"body"`
	State       string       `json:"state"`
	StateReason *string      `json:"state_reason"` // "completed", "not_planned", "reopened", or nil
	Assignee    *ghUser      `json:"assignee"`
	Assignees   []ghUser     `json:"assignees"`
	Labels      []ghLabel    `json:"labels"`
	Milestone   *ghMilestone `json:"milestone"`
	PullRequest *struct{}    `json:"pull_request"` // non-nil means this is a PR, not an issue
}

// --- GitHub Projects V2 GraphQL types ---

type ghProjectV2 struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Fields struct {
		Nodes []ghProjectField `json:"nodes"`
	} `json:"fields"`
}

type ghProjectField struct {
	ID      string            `json:"id"`
	Name    string            `json:"name"`
	Options []ghProjectOption `json:"options"` // only present for single-select fields
}

type ghProjectOption struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ghProjectItem struct {
	ID      string                `json:"id"` // GraphQL item ID, needed for push
	Content *ghProjectItemContent `json:"content"`
	FieldValues struct {
		Nodes []ghProjectFieldValue `json:"nodes"`
	} `json:"fieldValues"`
}

// ghProjectInfo is returned by fetchProjectStatusColumns.
type ghProjectInfo struct {
	ProjectID string
	FieldID   string
	Options   []ghProjectOption
}

// ghProjectItemStatus groups an item's Projects V2 status with its item ID.
type ghProjectItemStatus struct {
	StatusName string
	ItemID     string // GraphQL item ID
}

// ghIssueComment is a comment on a GitHub issue.
type ghIssueComment struct {
	ID        int64   `json:"id"`
	Body      string  `json:"body"`
	User      *ghUser `json:"user"`
	CreatedAt string  `json:"created_at"`
}

type ghProjectItemContent struct {
	Number int `json:"number"` // issue number
}

type ghProjectFieldValue struct {
	Name  string `json:"name"`  // selected option name (for single-select fields)
	Field struct {
		Name string `json:"name"` // field name (e.g. "Status")
	} `json:"field"`
}

// --- Request / Response types for our handlers ---

// GitHubPreviewRequest allows overriding the stored token for a preview fetch.
type GitHubPreviewRequest struct {
	Token string `json:"token"`
}

// GitHubUserMatch represents a GitHub user with optional TaskAI mapping.
type GitHubUserMatch struct {
	Login         string  `json:"login"`
	Name          string  `json:"name"`
	MatchedUserID *int64  `json:"matched_user_id"`
	MatchedName   string  `json:"matched_name"`
}

// GitHubStatusMatch represents a discovered GitHub issue status with optional swim lane mapping.
type GitHubStatusMatch struct {
	Key           string `json:"key"`            // canonical key: "open", "closed", "closed:not_planned", or a Projects V2 column name
	Label         string `json:"label"`          // human-readable display name
	Source        string `json:"source"`         // "issue_state" or "project_column"
	IssueCount    int    `json:"issue_count"`    // number of issues with this status (0 for project_column in preview)
	MatchedLaneID *int64 `json:"matched_lane_id"` // auto-matched swim lane
	MatchedName   string `json:"matched_name"`
}

// GitHubPreviewResponse is returned by HandleGitHubPreview.
type GitHubPreviewResponse struct {
	MilestoneCount int                 `json:"milestone_count"`
	LabelCount     int                 `json:"label_count"`
	IssueCount     int                 `json:"issue_count"`
	GitHubUsers    []GitHubUserMatch   `json:"github_users"`
	Statuses       []GitHubStatusMatch `json:"statuses"`   // all unique statuses found: Projects V2 columns + issue states
	Milestones     []ghMilestone       `json:"milestones"` // full list for filter UI
	Labels         []ghLabel           `json:"labels"`     // full list for filter UI
}

// GitHubImportFilter allows filtering which issues are imported.
type GitHubImportFilter struct {
	MilestoneNumber *int     `json:"milestone_number"` // nil = all milestones
	Assignee        string   `json:"assignee"`         // "" = all, "none" = unassigned
	Labels          []string `json:"labels"`           // empty = all labels
	State           string   `json:"state"`            // "all", "open", "closed" (default "all")
}

// GitHubPullRequest is the body for HandleGitHubPull / HandleGitHubSync.
type GitHubPullRequest struct {
	Token             string              `json:"token"`
	PullSprints       bool                `json:"pull_sprints"`
	PullTags          bool                `json:"pull_tags"`
	PullTasks         bool                `json:"pull_tasks"`
	PullComments      bool                `json:"pull_comments"`
	UserAssignments   map[string]int64    `json:"user_assignments"`   // login → TaskAI user_id (0 = unassigned)
	StatusAssignments map[string]int64    `json:"status_assignments"` // status key → swim_lane_id (0 = use category fallback)
	Filter            *GitHubImportFilter `json:"filter"`             // optional filter for issues
}

// GitHubPullResponse is returned by HandleGitHubPull / HandleGitHubSync.
type GitHubPullResponse struct {
	CreatedSprints  int `json:"created_sprints"`
	CreatedTags     int `json:"created_tags"`
	CreatedTasks    int `json:"created_tasks"`
	SkippedTasks    int `json:"skipped_tasks"`
	CreatedComments int `json:"created_comments"`
}

// --- Helper ---

func fetchGitHubJSON(ctx context.Context, token, url string, dest interface{}) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github api error %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	return json.NewDecoder(resp.Body).Decode(dest)
}

// fetchGitHubGraphQL sends a GraphQL query to the GitHub API.
func fetchGitHubGraphQL(ctx context.Context, token, query string, variables map[string]interface{}, dest interface{}) error {
	payload := map[string]interface{}{"query": query}
	if variables != nil {
		payload["variables"] = variables
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.github.com/graphql", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github graphql error %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return json.NewDecoder(resp.Body).Decode(dest)
}

// fetchProjectStatusColumns fetches GitHub Projects V2 status columns for a repo.
// Checks both repo-linked projects AND owner-level projects (user or org).
// Returns project info (id, status field id, options) from the first project found, or nil if none.
func fetchProjectStatusColumns(ctx context.Context, token, owner, repo string) (*ghProjectInfo, error) {
	if token == "" {
		return nil, nil
	}
	// Query both repo-linked projects and owner-level projects (handles org and user owners)
	const q = `
query($owner: String!, $repo: String!) {
  repository(owner: $owner, name: $repo) {
    projectsV2(first: 10) {
      nodes {
        id
        fields(first: 20) {
          nodes {
            ... on ProjectV2SingleSelectField { id name options { id name } }
          }
        }
      }
    }
    owner {
      ... on Organization {
        projectsV2(first: 10) {
          nodes {
            id
            fields(first: 20) {
              nodes {
                ... on ProjectV2SingleSelectField { id name options { id name } }
              }
            }
          }
        }
      }
      ... on User {
        projectsV2(first: 10) {
          nodes {
            id
            fields(first: 20) {
              nodes {
                ... on ProjectV2SingleSelectField { id name options { id name } }
              }
            }
          }
        }
      }
    }
  }
}`
	var result struct {
		Data struct {
			Repository struct {
				ProjectsV2 struct {
					Nodes []ghProjectV2 `json:"nodes"`
				} `json:"projectsV2"`
				Owner struct {
					ProjectsV2 struct {
						Nodes []ghProjectV2 `json:"nodes"`
					} `json:"projectsV2"`
				} `json:"owner"`
			} `json:"repository"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := fetchGitHubGraphQL(ctx, token, q, map[string]interface{}{"owner": owner, "repo": repo}, &result); err != nil {
		return nil, err
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("graphql: %s", result.Errors[0].Message)
	}

	// Combine repo-linked and owner-level projects, deduplicated by project ID
	seen := map[string]bool{}
	var allProjects []ghProjectV2
	for _, p := range result.Data.Repository.ProjectsV2.Nodes {
		if !seen[p.ID] {
			seen[p.ID] = true
			allProjects = append(allProjects, p)
		}
	}
	for _, p := range result.Data.Repository.Owner.ProjectsV2.Nodes {
		if !seen[p.ID] {
			seen[p.ID] = true
			allProjects = append(allProjects, p)
		}
	}

	for _, proj := range allProjects {
		// Prefer a field named after common status names, fall back to first single-select field
		var best *ghProjectField
		for i := range proj.Fields.Nodes {
			f := &proj.Fields.Nodes[i]
			if len(f.Options) == 0 {
				continue
			}
			lower := strings.ToLower(f.Name)
			if strings.Contains(lower, "status") || strings.Contains(lower, "stage") ||
				strings.Contains(lower, "state") || strings.Contains(lower, "phase") ||
				strings.Contains(lower, "column") {
				best = f
				break
			}
			if best == nil {
				best = f
			}
		}
		if best != nil {
			return &ghProjectInfo{
				ProjectID: proj.ID,
				FieldID:   best.ID,
				Options:   best.Options,
			}, nil
		}
	}
	return nil, nil
}

// fetchProjectIssueStatuses builds a map of issue_number → status+itemID
// by paginating through all items of the given project.
func fetchProjectIssueStatuses(ctx context.Context, token, projectID string) (map[int]ghProjectItemStatus, error) {
	result := map[int]ghProjectItemStatus{}
	if token == "" || projectID == "" {
		return result, nil
	}
	const q = `
query($projectId: ID!, $cursor: String) {
  node(id: $projectId) {
    ... on ProjectV2 {
      items(first: 100, after: $cursor) {
        pageInfo { hasNextPage endCursor }
        nodes {
          id
          content {
            ... on Issue { number }
          }
          fieldValues(first: 10) {
            nodes {
              ... on ProjectV2ItemFieldSingleSelectValue {
                name
                field { ... on ProjectV2SingleSelectField { name } }
              }
            }
          }
        }
      }
    }
  }
}`
	type pageResult struct {
		Data struct {
			Node struct {
				Items struct {
					PageInfo struct {
						HasNextPage bool   `json:"hasNextPage"`
						EndCursor   string `json:"endCursor"`
					} `json:"pageInfo"`
					Nodes []ghProjectItem `json:"nodes"`
				} `json:"items"`
			} `json:"node"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}

	var cursor *string
	for page := 0; page < 20; page++ {
		vars := map[string]interface{}{"projectId": projectID}
		if cursor != nil {
			vars["cursor"] = *cursor
		}
		var pr pageResult
		if err := fetchGitHubGraphQL(ctx, token, q, vars, &pr); err != nil {
			return result, err
		}
		if len(pr.Errors) > 0 {
			return result, fmt.Errorf("graphql: %s", pr.Errors[0].Message)
		}
		for _, item := range pr.Data.Node.Items.Nodes {
			if item.Content == nil || item.Content.Number == 0 {
				continue
			}
			info := ghProjectItemStatus{ItemID: item.ID}
			for _, fv := range item.FieldValues.Nodes {
				if strings.EqualFold(fv.Field.Name, "status") && fv.Name != "" {
					info.StatusName = fv.Name
					break
				}
			}
			result[item.Content.Number] = info
		}
		if !pr.Data.Node.Items.PageInfo.HasNextPage {
			break
		}
		c := pr.Data.Node.Items.PageInfo.EndCursor
		cursor = &c
	}
	return result, nil
}

// fuzzyMatchColumn fuzzy-matches a GitHub column name to the best swim lane.
// Returns the swim_lane_id or 0 if no match found.
func fuzzyMatchColumn(columnName string, lanes []swimLaneInfo) (int64, string) {
	col := strings.ToLower(strings.TrimSpace(columnName))
	// Exact match first
	for _, l := range lanes {
		if strings.ToLower(l.Name) == col {
			return l.ID, l.Name
		}
	}
	// Substring: column name contains lane name or vice versa
	for _, l := range lanes {
		lname := strings.ToLower(l.Name)
		if strings.Contains(col, lname) || strings.Contains(lname, col) {
			return l.ID, l.Name
		}
	}
	// Keyword-based category match
	statusCat := ""
	switch {
	case strings.Contains(col, "todo") || strings.Contains(col, "to do") || col == "backlog" || strings.Contains(col, "triage"):
		statusCat = "todo"
	case strings.Contains(col, "progress") || strings.Contains(col, "doing") || strings.Contains(col, "review") || strings.Contains(col, "test") || strings.Contains(col, "qa"):
		statusCat = "in_progress"
	case strings.Contains(col, "done") || strings.Contains(col, "complete") || strings.Contains(col, "finish") || strings.Contains(col, "closed") || strings.Contains(col, "ship") || strings.Contains(col, "release"):
		statusCat = "done"
	}
	if statusCat != "" {
		for _, l := range lanes {
			if l.StatusCategory == statusCat {
				return l.ID, l.Name
			}
		}
	}
	return 0, ""
}

// issueStatusKey returns a canonical status key from a GitHub issue's state and state_reason.
// "open" for open issues, "closed:not_planned" for won't-fix, "closed" for everything else closed.
func issueStatusKey(state string, stateReason *string) string {
	if state == "open" {
		return "open"
	}
	if stateReason != nil && *stateReason == "not_planned" {
		return "closed:not_planned"
	}
	return "closed"
}

// statusLabelForKey returns a human-readable label for a status key.
func statusLabelForKey(key string) string {
	switch key {
	case "open":
		return "Open"
	case "closed":
		return "Closed"
	case "closed:not_planned":
		return "Closed (won't fix)"
	default:
		return key
	}
}

// isStatusLikeLabel returns true if a label name looks like a workflow status.
func isStatusLikeLabel(name string) bool {
	lower := strings.ToLower(strings.TrimSpace(name))
	keywords := []string{
		"progress", "wip", "work in progress",
		"review", "reviewing",
		"blocked", "blocking",
		"ready", "ready for",
		"doing", "doing now",
		"waiting", "on hold",
		"needs", "need",
		"triage", "triaged",
		"accepted", "approved", "rejected", "declined",
		"stale", "help wanted",
	}
	for _, kw := range keywords {
		if strings.Contains(lower, kw) {
			return true
		}
	}
	return false
}

// autoMatchStatusToLane finds the best swim lane for a status key:
// "open" → first todo lane, "closed*" → first done lane, otherwise fuzzy match.
func autoMatchStatusToLane(key string, lanes []swimLaneInfo) (int64, string) {
	switch key {
	case "open":
		for _, l := range lanes {
			if l.StatusCategory == "todo" {
				return l.ID, l.Name
			}
		}
	case "closed", "closed:not_planned":
		for _, l := range lanes {
			if l.StatusCategory == "done" {
				return l.ID, l.Name
			}
		}
	}
	return fuzzyMatchColumn(key, lanes)
}

type swimLaneInfo struct {
	ID             int64
	Name           string
	StatusCategory string
}

// loadSwimLaneInfos loads all swim lanes for a project as swimLaneInfo.
func (s *Server) loadSwimLaneInfos(ctx context.Context, projectID int) ([]swimLaneInfo, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, name, status_category FROM swim_lanes WHERE project_id = $1 ORDER BY position ASC
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var lanes []swimLaneInfo
	for rows.Next() {
		var l swimLaneInfo
		if err := rows.Scan(&l.ID, &l.Name, &l.StatusCategory); err != nil {
			continue
		}
		lanes = append(lanes, l)
	}
	return lanes, nil
}

// loadGitHubConfig loads owner, repo, and token for a project.
func (s *Server) loadGitHubConfig(projectID int) (owner, repo, token string, err error) {
	var tokenNull sql.NullString
	err = s.db.QueryRow(`
		SELECT COALESCE(github_owner,''), COALESCE(github_repo_name,''), github_token
		FROM projects WHERE id = $1
	`, projectID).Scan(&owner, &repo, &tokenNull)
	if tokenNull.Valid {
		token = tokenNull.String
	}
	return
}

// --- Handlers ---

// HandleGitHubPreview fetches GitHub data without importing anything.
// POST /api/projects/{id}/github/preview
// Streams Server-Sent Events so the client can show a progress bar.
func (s *Server) HandleGitHubPreview(w http.ResponseWriter, r *http.Request) {
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

	hasAccess, err := s.userHasProjectAccess(int(userID), projectID)
	if err != nil || !hasAccess {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var req GitHubPreviewRequest
	_ = json.NewDecoder(r.Body).Decode(&req)

	owner, repo, storedToken, err := s.loadGitHubConfig(projectID)
	if err != nil {
		http.Error(w, "Failed to load project config", http.StatusInternalServerError)
		return
	}
	if owner == "" || repo == "" {
		respondError(w, http.StatusBadRequest, "GitHub owner and repo name must be configured first", "missing_config")
		return
	}
	token := storedToken
	if req.Token != "" {
		token = req.Token
	}

	// Stream progress via SSE (same pattern as pull/sync handlers)
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	w.WriteHeader(http.StatusOK)
	flusher, canFlush := w.(http.Flusher)

	sendSSE := func(data map[string]interface{}) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", b)
		if canFlush {
			flusher.Flush()
		}
	}

	sendSSE(map[string]interface{}{
		"type": "progress", "stage": "milestones",
		"message": "Fetching milestones...", "current": 0, "total": 0,
	})

	// Use a background context for GitHub API calls — independent of the HTTP request
	// context so that client disconnects or proxy timeouts don't abort a long fetch.
	fetchCtx, fetchCancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer fetchCancel()

	base := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)

	// Fetch milestones
	var milestones []ghMilestone
	if err := fetchGitHubJSON(fetchCtx, token, base+"/milestones?state=all&per_page=100", &milestones); err != nil {
		s.logger.Error("Failed to fetch GitHub milestones", zap.Error(err))
		sendSSE(map[string]interface{}{"type": "error", "message": "Failed to fetch milestones: " + err.Error()})
		return
	}
	// Send milestones as a dedicated event so the done event stays small
	sendSSE(map[string]interface{}{"type": "milestones", "items": milestones})
	sendSSE(map[string]interface{}{
		"type": "progress", "stage": "labels",
		"message": fmt.Sprintf("Found %d milestones — fetching labels...", len(milestones)), "current": 1, "total": 0,
	})

	// Fetch labels
	var labels []ghLabel
	if err := fetchGitHubJSON(fetchCtx, token, base+"/labels?per_page=100", &labels); err != nil {
		s.logger.Error("Failed to fetch GitHub labels", zap.Error(err))
		sendSSE(map[string]interface{}{"type": "error", "message": "Failed to fetch labels: " + err.Error()})
		return
	}
	// Send labels as a dedicated event so the done event stays small
	sendSSE(map[string]interface{}{"type": "labels", "items": labels})
	sendSSE(map[string]interface{}{
		"type": "progress", "stage": "issues",
		"message": fmt.Sprintf("Found %d labels — fetching issues...", len(labels)), "current": 2, "total": 0,
	})

	// Fetch issues (paginate up to 10 pages)
	var allIssues []ghIssue
	for page := 1; page <= 10; page++ {
		var pageIssues []ghIssue
		url := fmt.Sprintf("%s/issues?state=all&per_page=100&page=%d", base, page)
		if err := fetchGitHubJSON(fetchCtx, token, url, &pageIssues); err != nil {
			s.logger.Error("Failed to fetch GitHub issues", zap.Int("page", page), zap.Error(err))
			sendSSE(map[string]interface{}{"type": "error", "message": "Failed to fetch issues: " + err.Error()})
			return
		}
		if len(pageIssues) == 0 {
			break
		}
		allIssues = append(allIssues, pageIssues...)
		sendSSE(map[string]interface{}{
			"type": "progress", "stage": "issues",
			"message": fmt.Sprintf("Fetched %d issues...", len(allIssues)), "current": len(allIssues), "total": 0,
		})
	}

	// Collect unique assignee logins
	loginSet := map[string]ghUser{}
	for _, issue := range allIssues {
		for _, a := range issue.Assignees {
			if a.Login != "" {
				loginSet[a.Login] = a
			}
		}
		if issue.Assignee != nil && issue.Assignee.Login != "" {
			loginSet[issue.Assignee.Login] = *issue.Assignee
		}
	}

	// Load all team members for auto-matching (not just project members)
	type memberInfo struct {
		UserID    int64
		Email     string
		Name      string
		FirstName string
		LastName  string
	}
	sendSSE(map[string]interface{}{
		"type": "progress", "stage": "matching",
		"message": fmt.Sprintf("Fetched %d issues — matching users...", len(allIssues)), "current": len(allIssues), "total": len(allIssues),
	})
	rows, err := s.db.QueryContext(fetchCtx, `
		SELECT DISTINCT u.id, u.email, COALESCE(u.name,''), COALESCE(u.first_name,''), COALESCE(u.last_name,'')
		FROM users u
		JOIN team_members tm ON tm.user_id = u.id
		WHERE tm.team_id = (SELECT team_id FROM projects WHERE id = $1)
	`, projectID)
	if err != nil {
		sendSSE(map[string]interface{}{"type": "error", "message": "Failed to load team members"})
		return
	}
	defer rows.Close()

	var members []memberInfo
	for rows.Next() {
		var m memberInfo
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.FirstName, &m.LastName); err != nil {
			continue
		}
		members = append(members, m)
	}
	rows.Close()

	// Auto-match GitHub users to TaskAI members
	ghUsers := make([]GitHubUserMatch, 0, len(loginSet))
	for login, ghU := range loginSet {
		match := GitHubUserMatch{Login: login, Name: ghU.Name}
		loginLower := strings.ToLower(login)
		nameLower := strings.ToLower(ghU.Name)
		for _, m := range members {
			emailUser := strings.ToLower(strings.Split(m.Email, "@")[0])
			fullName := strings.ToLower(strings.TrimSpace(m.FirstName + " " + m.LastName))
			if fullName == " " {
				fullName = ""
			}
			nameLowerM := strings.ToLower(m.Name)
			if loginLower == emailUser ||
				(nameLower != "" && (nameLower == nameLowerM || nameLower == fullName)) {
				uid := m.UserID
				match.MatchedUserID = &uid
				if m.FirstName != "" || m.LastName != "" {
					match.MatchedName = strings.TrimSpace(m.FirstName + " " + m.LastName)
				} else {
					match.MatchedName = m.Name
				}
				break
			}
		}
		ghUsers = append(ghUsers, match)
	}

	// Load swim lanes for auto-matching
	lanes, _ := s.loadSwimLaneInfos(fetchCtx, projectID)

	// Try to fetch GitHub Projects V2 column names (best-effort, ordered)
	var projColNames []string
	if projInfo, err := fetchProjectStatusColumns(fetchCtx, token, owner, repo); err == nil && projInfo != nil {
		for _, opt := range projInfo.Options {
			projColNames = append(projColNames, opt.Name)
		}
	}

	// Count issues per canonical state key (skip PRs)
	issueStateCounts := map[string]int{}
	for _, issue := range allIssues {
		if issue.PullRequest != nil {
			continue
		}
		key := issueStatusKey(issue.State, issue.StateReason)
		issueStateCounts[key]++
	}

	// Build unified statuses list: Projects V2 columns first, then issue state keys
	seen := map[string]bool{}
	var statuses []GitHubStatusMatch

	for _, name := range projColNames {
		if seen[name] {
			continue
		}
		seen[name] = true
		st := GitHubStatusMatch{Key: name, Label: name, Source: "project_column"}
		if laneID, laneName := autoMatchStatusToLane(name, lanes); laneID != 0 {
			st.MatchedLaneID = &laneID
			st.MatchedName = laneName
		}
		statuses = append(statuses, st)
	}

	// Add issue state keys in a stable order
	for _, key := range []string{"open", "closed", "closed:not_planned"} {
		count, exists := issueStateCounts[key]
		if !exists || seen[key] {
			continue
		}
		seen[key] = true
		st := GitHubStatusMatch{Key: key, Label: statusLabelForKey(key), Source: "issue_state", IssueCount: count}
		if laneID, laneName := autoMatchStatusToLane(key, lanes); laneID != 0 {
			st.MatchedLaneID = &laneID
			st.MatchedName = laneName
		}
		statuses = append(statuses, st)
	}
	// Any remaining unexpected state keys
	for key, count := range issueStateCounts {
		if seen[key] {
			continue
		}
		st := GitHubStatusMatch{Key: key, Label: statusLabelForKey(key), Source: "issue_state", IssueCount: count}
		if laneID, laneName := autoMatchStatusToLane(key, lanes); laneID != 0 {
			st.MatchedLaneID = &laneID
			st.MatchedName = laneName
		}
		statuses = append(statuses, st)
	}

	// Add status-like labels found across issues (source="label")
	labelCounts := map[string]int{}
	for _, issue := range allIssues {
		if issue.PullRequest != nil {
			continue
		}
		for _, lbl := range issue.Labels {
			if isStatusLikeLabel(lbl.Name) {
				labelCounts[lbl.Name]++
			}
		}
	}
	for name, count := range labelCounts {
		key := "label:" + name
		if seen[key] {
			continue
		}
		seen[key] = true
		st := GitHubStatusMatch{Key: key, Label: name, Source: "label", IssueCount: count}
		if laneID, laneName := autoMatchStatusToLane(name, lanes); laneID != 0 {
			st.MatchedLaneID = &laneID
			st.MatchedName = laneName
		}
		statuses = append(statuses, st)
	}

	// Milestones and Labels were already streamed as dedicated events above.
	// Keep the done event small so it passes through nginx/proxies reliably.
	sendSSE(map[string]interface{}{
		"type": "done",
		"result": GitHubPreviewResponse{
			MilestoneCount: len(milestones),
			LabelCount:     len(labels),
			IssueCount:     len(allIssues),
			GitHubUsers:    ghUsers,
			Statuses:       statuses,
		},
	})
}

// HandleGitHubPull imports GitHub data, skipping already-imported items.
// POST /api/projects/{id}/github/pull
func (s *Server) HandleGitHubPull(w http.ResponseWriter, r *http.Request) {
	s.handleGitHubImport(w, r, false)
}

// HandleGitHubSync imports GitHub data, updating already-imported items.
// POST /api/projects/{id}/github/sync
func (s *Server) HandleGitHubSync(w http.ResponseWriter, r *http.Request) {
	s.handleGitHubImport(w, r, true)
}

func (s *Server) handleGitHubImport(w http.ResponseWriter, r *http.Request, doUpdate bool) {
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
	if err != nil || !isOwnerOrAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	var req GitHubPullRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	owner, repo, storedToken, err := s.loadGitHubConfig(projectID)
	if err != nil {
		http.Error(w, "Failed to load project config", http.StatusInternalServerError)
		return
	}
	if owner == "" || repo == "" {
		respondError(w, http.StatusBadRequest, "GitHub owner and repo name must be configured first", "missing_config")
		return
	}

	token := storedToken
	if req.Token != "" {
		// Save the new token
		token = req.Token
		if _, err := s.db.ExecContext(r.Context(), `UPDATE projects SET github_token = $1 WHERE id = $2`, token, projectID); err != nil {
			s.logger.Warn("Failed to save GitHub token", zap.Error(err))
		}
	}

	// --- Start SSE stream (must be before any writes) ---
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // prevents nginx buffering

	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	sendSSE := func(data map[string]interface{}) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}
	progress := func(stage, message string, current, total int) {
		sendSSE(map[string]interface{}{
			"type": "progress", "stage": stage,
			"message": message, "current": current, "total": total,
		})
	}

	ctx := r.Context()
	base := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)

	// buildIssueURL builds the GitHub issues endpoint URL with optional filter params.
	buildIssueURL := func(page int) string {
		state := "all"
		if req.Filter != nil && (req.Filter.State == "open" || req.Filter.State == "closed") {
			state = req.Filter.State
		}
		url := fmt.Sprintf("%s/issues?state=%s&per_page=100&page=%d", base, state, page)
		if req.Filter != nil {
			if req.Filter.MilestoneNumber != nil {
				url += fmt.Sprintf("&milestone=%d", *req.Filter.MilestoneNumber)
			}
			if req.Filter.Assignee != "" {
				url += "&assignee=" + req.Filter.Assignee
			}
			if len(req.Filter.Labels) > 0 {
				url += "&labels=" + strings.Join(req.Filter.Labels, ",")
			}
		}
		return url
	}

	var result GitHubPullResponse

	// --- Import Sprints from Milestones ---
	if req.PullSprints {
		progress("milestones", "Fetching milestones...", 0, 0)
		var milestones []ghMilestone
		if err := fetchGitHubJSON(ctx, token, base+"/milestones?state=all&per_page=100", &milestones); err != nil {
			s.logger.Error("Failed to fetch GitHub milestones", zap.Error(err))
			sendSSE(map[string]interface{}{"type": "error", "message": "Failed to fetch milestones: " + err.Error()})
			return
		}
		progress("milestones", fmt.Sprintf("Importing %d milestones...", len(milestones)), 0, len(milestones))

		for _, m := range milestones {
			status := "active"
			if m.State == "closed" {
				status = "completed"
			}

			var dueDate *string
			if m.DueOn != "" {
				t, err := time.Parse(time.RFC3339, m.DueOn)
				if err == nil {
					s := t.Format("2006-01-02")
					dueDate = &s
				}
			}

			if doUpdate {
				var existingID int64
				err := s.db.QueryRowContext(ctx, `
					SELECT id FROM sprints WHERE project_id = $1 AND github_milestone_number = $2
				`, projectID, m.Number).Scan(&existingID)

				if err == sql.ErrNoRows {
					// Insert new
					err = s.db.QueryRowContext(ctx, `
						INSERT INTO sprints (user_id, project_id, name, status, end_date, github_milestone_number)
						VALUES ($1, $2, $3, $4, $5, $6)
						ON CONFLICT (project_id, github_milestone_number) DO NOTHING
						RETURNING id
					`, userID, projectID, m.Title, status, dueDate, m.Number).Scan(&existingID)
					if err == nil {
						result.CreatedSprints++
					}
				} else if err == nil {
					// Update existing
					_, _ = s.db.ExecContext(ctx, `
						UPDATE sprints SET name = $1, status = $2, end_date = $3 WHERE id = $4
					`, m.Title, status, dueDate, existingID)
				}
			} else {
				var newID int64
				err := s.db.QueryRowContext(ctx, `
					INSERT INTO sprints (user_id, project_id, name, status, end_date, github_milestone_number)
					VALUES ($1, $2, $3, $4, $5, $6)
					ON CONFLICT (project_id, github_milestone_number) DO NOTHING
					RETURNING id
				`, userID, projectID, m.Title, status, dueDate, m.Number).Scan(&newID)
				if err == nil {
					result.CreatedSprints++
				}
			}
		}
	}

	// --- Import Tags from Labels ---
	if req.PullTags {
		progress("labels", "Fetching labels...", 0, 0)
		var labels []ghLabel
		if err := fetchGitHubJSON(ctx, token, base+"/labels?per_page=100", &labels); err != nil {
			s.logger.Error("Failed to fetch GitHub labels", zap.Error(err))
			sendSSE(map[string]interface{}{"type": "error", "message": "Failed to fetch labels: " + err.Error()})
			return
		}
		progress("labels", fmt.Sprintf("Importing %d labels...", len(labels)), 0, len(labels))

		for _, l := range labels {
			color := "#" + l.Color
			if l.Color == "" {
				color = "#6B7280"
			}

			if doUpdate {
				var existingID int64
				err := s.db.QueryRowContext(ctx, `
					SELECT id FROM tags WHERE project_id = $1 AND github_label_name = $2
				`, projectID, l.Name).Scan(&existingID)

				if err == sql.ErrNoRows {
					err = s.db.QueryRowContext(ctx, `
						INSERT INTO tags (user_id, project_id, name, color, github_label_name)
						VALUES ($1, $2, $3, $4, $5)
						ON CONFLICT (project_id, github_label_name) DO NOTHING
						RETURNING id
					`, userID, projectID, l.Name, color, l.Name).Scan(&existingID)
					if err == nil {
						result.CreatedTags++
					}
				} else if err == nil {
					_, _ = s.db.ExecContext(ctx, `UPDATE tags SET color = $1 WHERE id = $2`, color, existingID)
				}
			} else {
				var newID int64
				err := s.db.QueryRowContext(ctx, `
					INSERT INTO tags (user_id, project_id, name, color, github_label_name)
					VALUES ($1, $2, $3, $4, $5)
					ON CONFLICT (project_id, github_label_name) DO NOTHING
					RETURNING id
				`, userID, projectID, l.Name, color, l.Name).Scan(&newID)
				if err == nil {
					result.CreatedTags++
				}
			}
		}
	}

	// --- Import Tasks from Issues ---
	if req.PullTasks {
		progress("issues", "Fetching issues...", 0, 0)
		var allIssues []ghIssue
		for page := 1; page <= 10; page++ {
			var pageIssues []ghIssue
			if err := fetchGitHubJSON(ctx, token, buildIssueURL(page), &pageIssues); err != nil {
				s.logger.Error("Failed to fetch GitHub issues", zap.Int("page", page), zap.Error(err))
				sendSSE(map[string]interface{}{"type": "error", "message": "Failed to fetch issues: " + err.Error()})
				return
			}
			if len(pageIssues) == 0 {
				break
			}
			allIssues = append(allIssues, pageIssues...)
		}
		progress("issues", fmt.Sprintf("Processing %d issues...", len(allIssues)), 0, len(allIssues))

		// Build a label→tag_id map from newly imported tags
		labelToTagID := map[string]int64{}
		if req.PullTags {
			rows, err := s.db.QueryContext(ctx, `
				SELECT github_label_name, id FROM tags
				WHERE project_id = $1 AND github_label_name IS NOT NULL
			`, projectID)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var lname string
					var tid int64
					if err := rows.Scan(&lname, &tid); err == nil {
						labelToTagID[lname] = tid
					}
				}
				rows.Close()
			}
		}

		// Build a milestone_number→sprint_id map
		milestoneToSprintID := map[int]int64{}
		if req.PullSprints {
			rows, err := s.db.QueryContext(ctx, `
				SELECT github_milestone_number, id FROM sprints
				WHERE project_id = $1 AND github_milestone_number IS NOT NULL
			`, projectID)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var mnum int
					var sid int64
					if err := rows.Scan(&mnum, &sid); err == nil {
						milestoneToSprintID[mnum] = sid
					}
				}
				rows.Close()
			}
		}

		// Build status_category → swim_lane_id map for this project (fallback)
		swimLaneByCategory := map[string]int64{}
		slRows, err := s.db.QueryContext(ctx, `
			SELECT status_category, id FROM swim_lanes WHERE project_id = $1 ORDER BY position ASC
		`, projectID)
		if err == nil {
			defer slRows.Close()
			for slRows.Next() {
				var cat string
				var slID int64
				if err := slRows.Scan(&cat, &slID); err == nil {
					if _, exists := swimLaneByCategory[cat]; !exists {
						swimLaneByCategory[cat] = slID
					}
				}
			}
			slRows.Close()
		}

		// Fetch GitHub Projects V2 issue→column+itemID map (best-effort; ignore errors)
		issueColumnMap := map[int]ghProjectItemStatus{}
		if projInfo, err := fetchProjectStatusColumns(ctx, token, owner, repo); err == nil && projInfo != nil {
			// Persist project/field IDs so push operations can use them later
			_, _ = s.db.ExecContext(ctx, `
				UPDATE projects SET github_project_id = $1, github_status_field_id = $2 WHERE id = $3
			`, projInfo.ProjectID, projInfo.FieldID, projectID)

			if m, err := fetchProjectIssueStatuses(ctx, token, projInfo.ProjectID); err == nil {
				issueColumnMap = m
			}

			// Persist github_option_id on swim lanes by matching column options → lanes
			lanes, _ := s.loadSwimLaneInfos(ctx, projectID)
			for _, opt := range projInfo.Options {
				if laneID, _ := fuzzyMatchColumn(opt.Name, lanes); laneID != 0 {
					optID := opt.ID
					_, _ = s.db.ExecContext(ctx, `UPDATE swim_lanes SET github_option_id = $1 WHERE id = $2`, optID, laneID)
				}
			}
		}

		// Get next task_number baseline
		var maxNumber sql.NullInt64
		_ = s.db.QueryRowContext(ctx, `SELECT MAX(task_number) FROM tasks WHERE project_id = $1`, projectID).Scan(&maxNumber)
		nextNumber := int64(1)
		if maxNumber.Valid {
			nextNumber = maxNumber.Int64 + 1
		}

		for i, issue := range allIssues {
			if i%25 == 0 && i > 0 {
				progress("issues", fmt.Sprintf("Processed %d/%d issues...", i, len(allIssues)), i, len(allIssues))
			}
			taskStatus := "todo"
			if issue.State == "closed" {
				taskStatus = "done"
			}

			// Resolve assignee
			var assigneeID *int64
			primaryLogin := ""
			if issue.Assignee != nil {
				primaryLogin = issue.Assignee.Login
			} else if len(issue.Assignees) > 0 {
				primaryLogin = issue.Assignees[0].Login
			}
			if primaryLogin != "" {
				if uid, ok := req.UserAssignments[primaryLogin]; ok && uid != 0 {
					assigneeID = &uid
				}
			}

			// Resolve sprint
			var sprintID *int64
			if issue.Milestone != nil {
				if sid, ok := milestoneToSprintID[issue.Milestone.Number]; ok {
					sprintID = &sid
				}
			}

			description := issue.Body

			// Resolve swim lane with priority:
			// 1. GitHub Projects V2 column name
			// 2. Status-like label
			// 3. Issue state key (open / closed / closed:not_planned)
			// 4. Category fallback
			var swimLaneID *int64
			ghItemID := ""

			// 1. Projects V2 column
			if itemStatus, ok := issueColumnMap[issue.Number]; ok {
				ghItemID = itemStatus.ItemID
				if itemStatus.StatusName != "" {
					if laneID, ok := req.StatusAssignments[itemStatus.StatusName]; ok && laneID > 0 {
						swimLaneID = &laneID
					}
				}
			}
			// 2. Status-like labels
			if swimLaneID == nil {
				for _, lbl := range issue.Labels {
					if laneID, ok := req.StatusAssignments["label:"+lbl.Name]; ok && laneID > 0 {
						swimLaneID = &laneID
						break
					}
				}
			}
			// 3. Issue state key
			if swimLaneID == nil {
				stateKey := issueStatusKey(issue.State, issue.StateReason)
				if laneID, ok := req.StatusAssignments[stateKey]; ok && laneID > 0 {
					swimLaneID = &laneID
				}
			}
			// 4. Category fallback
			if swimLaneID == nil {
				if slID, ok := swimLaneByCategory[taskStatus]; ok {
					swimLaneID = &slID
				}
			}

			if doUpdate {
				var existingID int64
				err := s.db.QueryRowContext(ctx, `
					SELECT id FROM tasks WHERE project_id = $1 AND github_issue_number = $2
				`, projectID, issue.Number).Scan(&existingID)

				if err == sql.ErrNoRows {
					// Insert new task
					err = s.db.QueryRowContext(ctx, `
						INSERT INTO tasks (project_id, task_number, title, description, status, priority, assignee_id, sprint_id, github_issue_number, swim_lane_id, github_project_item_id)
						VALUES ($1, $2, $3, $4, $5, 'medium', $6, $7, $8, $9, $10)
						ON CONFLICT (project_id, github_issue_number) DO NOTHING
						RETURNING id
					`, projectID, nextNumber, issue.Title, description, taskStatus, assigneeID, sprintID, issue.Number, swimLaneID, nullableStr(ghItemID)).Scan(&existingID)
					if err == nil {
						nextNumber++
						result.CreatedTasks++
						s.insertTaskTags(ctx, existingID, issue.Labels, labelToTagID)
					} else {
						result.SkippedTasks++
					}
				} else if err == nil {
					// Update existing
					_, _ = s.db.ExecContext(ctx, `
						UPDATE tasks SET title = $1, description = $2, status = $3, assignee_id = $4, sprint_id = $5, swim_lane_id = $6, github_project_item_id = COALESCE(NULLIF($7,''), github_project_item_id)
						WHERE id = $8
					`, issue.Title, description, taskStatus, assigneeID, sprintID, swimLaneID, ghItemID, existingID)
					// Refresh tags
					_, _ = s.db.ExecContext(ctx, `DELETE FROM task_tags WHERE task_id = $1`, existingID)
					s.insertTaskTags(ctx, existingID, issue.Labels, labelToTagID)
				}
			} else {
				var newID int64
				err := s.db.QueryRowContext(ctx, `
					INSERT INTO tasks (project_id, task_number, title, description, status, priority, assignee_id, sprint_id, github_issue_number, swim_lane_id, github_project_item_id)
					VALUES ($1, $2, $3, $4, $5, 'medium', $6, $7, $8, $9, $10)
					ON CONFLICT (project_id, github_issue_number) DO NOTHING
					RETURNING id
				`, projectID, nextNumber, issue.Title, description, taskStatus, assigneeID, sprintID, issue.Number, swimLaneID, nullableStr(ghItemID)).Scan(&newID)
				if err == nil {
					nextNumber++
					result.CreatedTasks++
					s.insertTaskTags(ctx, newID, issue.Labels, labelToTagID)
				} else {
					result.SkippedTasks++
				}
			}
		}
	}

	// --- Import Comments from Issues ---
	if req.PullComments {
		var issueNumbers []int
		// Collect tasks with github_issue_number in this project
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, github_issue_number FROM tasks
			WHERE project_id = $1 AND github_issue_number IS NOT NULL
		`, projectID)
		if err == nil {
			type taskRef struct {
				taskID   int64
				issueNum int
			}
			var taskRefs []taskRef
			for rows.Next() {
				var tr taskRef
				if err := rows.Scan(&tr.taskID, &tr.issueNum); err == nil {
					taskRefs = append(taskRefs, tr)
					issueNumbers = append(issueNumbers, tr.issueNum)
				}
			}
			rows.Close()
			_ = issueNumbers // suppress unused warning

			progress("comments", fmt.Sprintf("Fetching comments for %d issues...", len(taskRefs)), 0, len(taskRefs))
			for i, tr := range taskRefs {
				if i%10 == 0 && i > 0 {
					progress("comments", fmt.Sprintf("Fetched comments for %d/%d issues...", i, len(taskRefs)), i, len(taskRefs))
				}
				var ghComments []ghIssueComment
				url := fmt.Sprintf("%s/issues/%d/comments?per_page=100", base, tr.issueNum)
				if err := fetchGitHubJSON(ctx, token, url, &ghComments); err != nil {
					continue // best-effort
				}
				for _, gc := range ghComments {
					if gc.Body == "" {
						continue
					}
					// Format: prefix with GitHub username for attribution
					login := ""
					if gc.User != nil {
						login = gc.User.Login
					}
					body := gc.Body
					if login != "" {
						body = "**@" + login + "** (GitHub):\n\n" + gc.Body
					}
					// Use the first project user as comment author (system import)
					var ownerID int64
					_ = s.db.QueryRowContext(ctx, `SELECT user_id FROM project_members WHERE project_id = $1 AND role = 'owner' LIMIT 1`, projectID).Scan(&ownerID)
					if ownerID == 0 {
						_ = s.db.QueryRowContext(ctx, `SELECT user_id FROM project_members WHERE project_id = $1 LIMIT 1`, projectID).Scan(&ownerID)
					}
					var newCID int64
					err := s.db.QueryRowContext(ctx, `
						INSERT INTO task_comments (task_id, user_id, comment, github_comment_id)
						VALUES ($1, $2, $3, $4)
						ON CONFLICT (github_comment_id) DO NOTHING
						RETURNING id
					`, tr.taskID, ownerID, body, gc.ID).Scan(&newCID)
					if err == nil {
						result.CreatedComments++
					}
				}
			}
		}
	}

	// Update last sync timestamp
	_, _ = s.db.ExecContext(ctx, `UPDATE projects SET github_last_sync = $1 WHERE id = $2`, time.Now(), projectID)

	sendSSE(map[string]interface{}{"type": "done", "result": result})
}

// insertTaskTags inserts tag associations for a task based on issue labels.
func (s *Server) insertTaskTags(ctx context.Context, taskID int64, labels []ghLabel, labelToTagID map[string]int64) {
	for _, lbl := range labels {
		tagID, ok := labelToTagID[lbl.Name]
		if !ok {
			continue
		}
		_, _ = s.db.ExecContext(ctx, `
			INSERT INTO task_tags (task_id, tag_id) VALUES ($1, $2)
			ON CONFLICT DO NOTHING
		`, taskID, tagID)
	}
}

// nullableStr returns nil if s is empty, otherwise returns &s (for use as SQL NULL).
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// pushCommentToGitHub creates a comment on a GitHub issue and returns the new comment ID.
func pushCommentToGitHub(ctx context.Context, token, owner, repo string, issueNumber int64, body string) (int64, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/comments", owner, repo, issueNumber)
	payload, _ := json.Marshal(map[string]string{"body": body})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(payload))
	if err != nil {
		return 0, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("github comment push error %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return 0, err
	}
	return created.ID, nil
}

// pushSwimLaneStatusToGitHub updates a GitHub Projects V2 item's status field.
func pushSwimLaneStatusToGitHub(ctx context.Context, token, projectID, fieldID, itemID, optionID string) error {
	const mutation = `
mutation($projectId: ID!, $itemId: ID!, $fieldId: ID!, $optionId: String!) {
  updateProjectV2ItemFieldValue(input: {
    projectId: $projectId
    itemId: $itemId
    fieldId: $fieldId
    value: { singleSelectOptionId: $optionId }
  }) {
    projectV2Item { id }
  }
}`
	var result struct {
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	vars := map[string]interface{}{
		"projectId": projectID,
		"itemId":    itemID,
		"fieldId":   fieldID,
		"optionId":  optionID,
	}
	if err := fetchGitHubGraphQL(ctx, token, mutation, vars, &result); err != nil {
		return err
	}
	if len(result.Errors) > 0 {
		return fmt.Errorf("graphql mutation: %s", result.Errors[0].Message)
	}
	return nil
}

// tryPushCommentToGitHub pushes a newly created TaskAI comment to GitHub as an issue comment.
// It's best-effort: errors are logged but do not affect the response.
func (s *Server) tryPushCommentToGitHub(ctx context.Context, taskID, commentID int64, body, commenterName string) {
	var (
		issueNumber int64
		owner, repo string
		token       string
		pushEnabled bool
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(t.github_issue_number,0),
		       COALESCE(p.github_owner,''), COALESCE(p.github_repo_name,''),
		       COALESCE(p.github_token,''), p.github_push_enabled
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		WHERE t.id = $1
	`, taskID).Scan(&issueNumber, &owner, &repo, &token, &pushEnabled)
	if err != nil || !pushEnabled || issueNumber == 0 || owner == "" || token == "" {
		return
	}
	ghBody := body
	if commenterName != "" {
		ghBody = "**" + commenterName + "** (via TaskAI):\n\n" + body
	}
	ghCommentID, err := pushCommentToGitHub(ctx, token, owner, repo, issueNumber, ghBody)
	if err != nil {
		s.logger.Warn("Failed to push comment to GitHub", zap.Int64("task_id", taskID), zap.Error(err))
		return
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE task_comments SET github_comment_id = $1 WHERE id = $2`, ghCommentID, commentID)
}

// HandleGitHubPushTask creates or updates a GitHub issue for a TaskAI task.
// POST /api/tasks/{taskId}/github/push
func (s *Server) HandleGitHubPushTask(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	userID, ok := GetUserID(r)
	if !ok {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	taskID, err := strconv.ParseInt(chi.URLParam(r, "taskId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid task ID", "invalid_input")
		return
	}

	// Load task + project github config in one query
	var (
		title             string
		description       sql.NullString
		projectID         int64
		githubIssueNumber sql.NullInt64
		milestoneNumber   sql.NullInt64
		owner, repo       string
		tokenNull         sql.NullString
	)
	err = s.db.QueryRowContext(ctx, `
		SELECT t.title, t.description, t.project_id, t.github_issue_number,
		       (SELECT s.github_milestone_number FROM sprints s WHERE s.id = t.sprint_id LIMIT 1),
		       COALESCE(p.github_owner,''), COALESCE(p.github_repo_name,''),
		       p.github_token
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		WHERE t.id = $1
	`, taskID).Scan(&title, &description, &projectID, &githubIssueNumber, &milestoneNumber, &owner, &repo, &tokenNull)
	if err != nil {
		if err == sql.ErrNoRows {
			respondError(w, http.StatusNotFound, "task not found", "not_found")
		} else {
			respondError(w, http.StatusInternalServerError, "failed to load task", "internal_error")
		}
		return
	}

	token := ""
	if tokenNull.Valid {
		token = tokenNull.String
	}

	// Auth check
	hasAccess, _ := s.checkProjectAccess(ctx, userID, projectID)
	if !hasAccess {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	if owner == "" || repo == "" || token == "" {
		respondError(w, http.StatusBadRequest, "GitHub not configured for this project", "missing_config")
		return
	}

	body := ""
	if description.Valid {
		body = description.String
	}

	payload := map[string]interface{}{
		"title": title,
		"body":  body,
	}
	if milestoneNumber.Valid {
		payload["milestone"] = milestoneNumber.Int64
	}

	var issueNumber int64
	var htmlURL string

	if githubIssueNumber.Valid && githubIssueNumber.Int64 > 0 {
		// Update existing issue
		issueNumber = githubIssueNumber.Int64
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d", owner, repo, issueNumber)
		data, _ := json.Marshal(payload)
		req2, _ := http.NewRequestWithContext(ctx, http.MethodPatch, apiURL, bytes.NewReader(data))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Authorization", "Bearer "+token)
		req2.Header.Set("Accept", "application/vnd.github+json")
		req2.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		resp, err := http.DefaultClient.Do(req2)
		if err != nil {
			respondError(w, http.StatusBadGateway, "failed to update GitHub issue: "+err.Error(), "github_error")
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(resp.Body)
			respondError(w, http.StatusBadGateway, "GitHub API error: "+strings.TrimSpace(string(b)), "github_error")
			return
		}
		var result struct {
			Number  int    `json:"number"`
			HTMLURL string `json:"html_url"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err == nil {
			htmlURL = result.HTMLURL
		}
	} else {
		// Create new issue
		apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", owner, repo)
		data, _ := json.Marshal(payload)
		req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(data))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Authorization", "Bearer "+token)
		req2.Header.Set("Accept", "application/vnd.github+json")
		req2.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		resp, err := http.DefaultClient.Do(req2)
		if err != nil {
			respondError(w, http.StatusBadGateway, "failed to create GitHub issue: "+err.Error(), "github_error")
			return
		}
		defer resp.Body.Close()
		if resp.StatusCode >= 400 {
			b, _ := io.ReadAll(resp.Body)
			respondError(w, http.StatusBadGateway, "GitHub API error: "+strings.TrimSpace(string(b)), "github_error")
			return
		}
		var result struct {
			Number  int    `json:"number"`
			HTMLURL string `json:"html_url"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to parse GitHub response", "internal_error")
			return
		}
		issueNumber = int64(result.Number)
		htmlURL = result.HTMLURL
		// Save issue number back to task
		_, _ = s.db.ExecContext(ctx, `UPDATE tasks SET github_issue_number = $1 WHERE id = $2`, issueNumber, taskID)
	}

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"issue_number": issueNumber,
		"html_url":     htmlURL,
	})
}

// HandleGitHubPushAll creates GitHub issues for all tasks without a linked issue.
// POST /api/projects/{id}/github/push-all (SSE)
func (s *Server) HandleGitHubPushAll(w http.ResponseWriter, r *http.Request) {
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
	if err != nil || !isOwnerOrAdmin {
		http.Error(w, "Forbidden", http.StatusForbidden)
		return
	}

	owner, repo, token, err := s.loadGitHubConfig(projectID)
	if err != nil || owner == "" || repo == "" || token == "" {
		respondError(w, http.StatusBadRequest, "GitHub not configured for this project", "missing_config")
		return
	}

	// Start SSE
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, canFlush := w.(http.Flusher)
	if !canFlush {
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}
	sendSSE := func(data map[string]interface{}) {
		b, _ := json.Marshal(data)
		fmt.Fprintf(w, "data: %s\n\n", b)
		flusher.Flush()
	}

	ctx := r.Context()
	type pushTask struct {
		ID          int64
		Title       string
		Description sql.NullString
		Milestone   sql.NullInt64
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT t.id, t.title, t.description,
		       (SELECT s.github_milestone_number FROM sprints s WHERE s.id = t.sprint_id LIMIT 1)
		FROM tasks t
		WHERE t.project_id = $1 AND t.github_issue_number IS NULL
		ORDER BY t.id
	`, projectID)
	if err != nil {
		sendSSE(map[string]interface{}{"type": "error", "message": "Failed to load tasks: " + err.Error()})
		return
	}

	var tasks []pushTask
	for rows.Next() {
		var t pushTask
		if err := rows.Scan(&t.ID, &t.Title, &t.Description, &t.Milestone); err == nil {
			tasks = append(tasks, t)
		}
	}
	rows.Close()

	total := len(tasks)
	sendSSE(map[string]interface{}{
		"type": "progress", "stage": "start",
		"message": fmt.Sprintf("Found %d new tasks to push to GitHub", total),
		"current": 0, "total": total,
	})

	created := 0
	failed := 0
	apiBase := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues", owner, repo)

	for i, t := range tasks {
		payload := map[string]interface{}{
			"title": t.Title,
			"body":  "",
		}
		if t.Description.Valid {
			payload["body"] = t.Description.String
		}
		if t.Milestone.Valid {
			payload["milestone"] = t.Milestone.Int64
		}

		data, _ := json.Marshal(payload)
		req2, _ := http.NewRequestWithContext(ctx, http.MethodPost, apiBase, bytes.NewReader(data))
		req2.Header.Set("Content-Type", "application/json")
		req2.Header.Set("Authorization", "Bearer "+token)
		req2.Header.Set("Accept", "application/vnd.github+json")
		req2.Header.Set("X-GitHub-Api-Version", "2022-11-28")
		resp, err := http.DefaultClient.Do(req2)
		if err != nil {
			failed++
		} else {
			if resp.StatusCode >= 400 {
				failed++
				_, _ = io.Copy(io.Discard, resp.Body)
			} else {
				var result struct {
					Number int `json:"number"`
				}
				_ = json.NewDecoder(resp.Body).Decode(&result)
				if result.Number > 0 {
					_, _ = s.db.ExecContext(ctx, `UPDATE tasks SET github_issue_number = $1 WHERE id = $2`, result.Number, t.ID)
					created++
				} else {
					failed++
				}
			}
			resp.Body.Close()
		}

		if (i+1)%5 == 0 || i+1 == total {
			sendSSE(map[string]interface{}{
				"type": "progress", "stage": "tasks",
				"message": fmt.Sprintf("Pushed %d/%d tasks to GitHub...", i+1, total),
				"current": i + 1, "total": total,
			})
		}
	}

	sendSSE(map[string]interface{}{
		"type": "done",
		"result": map[string]interface{}{
			"created_tasks": created,
			"skipped_tasks": failed,
		},
	})
}

// tryPushSwimLaneToGitHub pushes a task's new swim lane status to GitHub Projects V2.
// It's best-effort: errors are logged but do not affect the response.
func (s *Server) tryPushSwimLaneToGitHub(ctx context.Context, taskID int64, newLaneID *int64) {
	if newLaneID == nil {
		return
	}
	var (
		itemID, projectID, fieldID, optionID string
		token                                 string
		pushEnabled                           bool
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(t.github_project_item_id,''),
		       COALESCE(p.github_project_id,''), COALESCE(p.github_status_field_id,''),
		       COALESCE(sl.github_option_id,''),
		       COALESCE(p.github_token,''), p.github_push_enabled
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		JOIN swim_lanes sl ON sl.id = $2
		WHERE t.id = $1
	`, taskID, *newLaneID).Scan(&itemID, &projectID, &fieldID, &optionID, &token, &pushEnabled)
	if err != nil || !pushEnabled || itemID == "" || projectID == "" || fieldID == "" || optionID == "" || token == "" {
		return
	}
	if err := pushSwimLaneStatusToGitHub(ctx, token, projectID, fieldID, itemID, optionID); err != nil {
		s.logger.Warn("Failed to push swim lane status to GitHub", zap.Int64("task_id", taskID), zap.Error(err))
	}
}
