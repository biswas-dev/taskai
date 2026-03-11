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
	ID    int64  `json:"id"`
	Login string `json:"login"`
	Name  string `json:"name"`
}

type ghIssue struct {
	Number      int          `json:"number"`
	Title       string       `json:"title"`
	Body        string       `json:"body"`
	State       string       `json:"state"`
	StateReason *string      `json:"state_reason"` // "completed", "not_planned", "reopened", or nil
	UpdatedAt   string       `json:"updated_at"`   // RFC3339; used to skip comment fetch for unchanged issues
	Assignee    *ghUser      `json:"assignee"`
	Assignees   []ghUser     `json:"assignees"`
	Labels      []ghLabel    `json:"labels"`
	Milestone   *ghMilestone `json:"milestone"`
	PullRequest *struct{}    `json:"pull_request"` // non-nil means this is a PR, not an issue
	Reactions   *ghReactions `json:"reactions"`
	Repo        string       `json:"-"` // "owner/repo" — set by caller, not from JSON
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
	StatusName     string
	ItemID         string                // GraphQL item ID
	StartDate      string                // "YYYY-MM-DD" from Projects V2 date field, may be empty
	DueDate        string                // "YYYY-MM-DD", may be empty
	IterationTitle string                // Sprint/iteration name from Projects V2 iteration field
	Issue          *ghProjectItemContent // full issue data from GraphQL (title, body, assignees, labels, repo…)
}

type ghReactions struct {
	PlusOne  int `json:"+1"`
	MinusOne int `json:"-1"`
	Laugh    int `json:"laugh"`
	Hooray   int `json:"hooray"`
	Confused int `json:"confused"`
	Heart    int `json:"heart"`
	Rocket   int `json:"rocket"`
	Eyes     int `json:"eyes"`
}

// ghIssueComment is a comment on a GitHub issue.
type ghIssueComment struct {
	ID        int64        `json:"id"`
	Body      string       `json:"body"`
	User      *ghUser      `json:"user"`
	CreatedAt string       `json:"created_at"`
	Reactions *ghReactions `json:"reactions"`
}

type ghProjectItemContent struct {
	Number     int    `json:"number"` // issue number
	Title      string `json:"title"`
	Body       string `json:"body"`
	State      string `json:"state"`
	Repository *struct {
		Name  string `json:"name"`
		Owner struct {
			Login string `json:"login"`
		} `json:"owner"`
	} `json:"repository"`
	Assignees *struct {
		Nodes []ghUser `json:"nodes"`
	} `json:"assignees"`
	Labels *struct {
		Nodes []ghLabel `json:"nodes"`
	} `json:"labels"`
	Milestone *ghMilestone `json:"milestone"`
}

// Repo returns "owner/repo" from the GraphQL repository data, or empty string.
func (c *ghProjectItemContent) Repo() string {
	if c == nil || c.Repository == nil {
		return ""
	}
	return c.Repository.Owner.Login + "/" + c.Repository.Name
}

type ghProjectFieldValue struct {
	Name      string `json:"name"`      // selected option name (for single-select fields)
	Date      string `json:"date"`      // date value (for date fields)
	Title     string `json:"title"`     // iteration title (e.g. "Sprint 139")
	StartDate string `json:"startDate"` // iteration start date
	Duration  int    `json:"duration"`  // iteration duration in days
	Field     struct {
		ID   string `json:"id"`   // field GraphQL ID
		Name string `json:"name"` // field name (e.g. "Status")
	} `json:"field"`
}

// --- Request / Response types for our handlers ---

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
	ForceFullSync     bool                `json:"force_full_sync"`    // delete all GitHub-sourced data and re-import from scratch
}

// GitHubPullResponse is returned by HandleGitHubPull / HandleGitHubSync.
type GitHubPullResponse struct {
	CreatedSprints  int `json:"created_sprints"`
	CreatedTags     int `json:"created_tags"`
	CreatedTasks    int `json:"created_tasks"`
	UpdatedTasks    int `json:"updated_tasks"`
	SkippedTasks    int `json:"skipped_tasks"`
	CreatedComments int `json:"created_comments"`
	PushedComments  int `json:"pushed_comments"`
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
    projectsV2(first: 50) {
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
        projectsV2(first: 50) {
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
        projectsV2(first: 50) {
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

// fetchProjectIssueStatuses builds a map of "owner/repo#number" → status+itemID+issue
// by paginating through all items of the given project.
// statusFieldID is the GraphQL ID of the status field (from fetchProjectStatusColumns); used for
// precise matching so fields named "Stage", "Phase", etc. work correctly.
func fetchProjectIssueStatuses(ctx context.Context, token, projectID, statusFieldID string, logger *zap.Logger) (map[string]ghProjectItemStatus, error) {
	result := map[string]ghProjectItemStatus{}
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
            ... on Issue {
              number
              title
              body
              state
              assignees(first: 10) { nodes { login } }
              labels(first: 20) { nodes { name color } }
              milestone { number title state }
              repository { name owner { login } }
            }
            ... on PullRequest {
              number
              title
              body
              state
              assignees(first: 10) { nodes { login } }
              labels(first: 20) { nodes { name color } }
              milestone { number title state }
              repository { name owner { login } }
            }
          }
          fieldValues(first: 20) {
            nodes {
              ... on ProjectV2ItemFieldSingleSelectValue {
                name
                field { ... on ProjectV2SingleSelectField { id name } }
              }
              ... on ProjectV2ItemFieldDateValue {
                date
                field { ... on ProjectV2Field { id name } }
              }
              ... on ProjectV2ItemFieldIterationValue {
                title
                startDate
                duration
                field { ... on ProjectV2IterationField { id name } }
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
	for page := 0; page < 300; page++ { // up to 30,000 project items
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
			info := ghProjectItemStatus{ItemID: item.ID, Issue: item.Content}
			for _, fv := range item.FieldValues.Nodes {
				// Capture status from single-select field
				if fv.Name != "" {
					// Match by field ID when available (handles "Stage", "Phase", etc.)
					// Fall back to name match "status" for backwards compat
					if (statusFieldID != "" && fv.Field.ID == statusFieldID) ||
						(statusFieldID == "" && strings.EqualFold(fv.Field.Name, "status")) {
						info.StatusName = fv.Name
					}
				}
				// Capture date fields
				if fv.Date != "" {
					lower := strings.ToLower(fv.Field.Name)
					if strings.Contains(lower, "start") {
						info.StartDate = fv.Date
					} else if strings.Contains(lower, "due") || strings.Contains(lower, "end") {
						info.DueDate = fv.Date
					}
				}
				// Capture iteration field (sprint) — Title is only set for
				// ProjectV2ItemFieldIterationValue, so any non-empty Title is an iteration.
				if fv.Title != "" {
					info.IterationTitle = fv.Title
				}
			}
			// Key by "owner/repo#number" to avoid cross-repo collisions
			key := fmt.Sprintf("%s#%d", item.Content.Repo(), item.Content.Number)
			result[key] = info
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

// loadGitHubConfig loads owner, repo, token, and optional project URL for a project.
func (s *Server) loadGitHubConfig(projectID int) (owner, repo, token, projectURL string, err error) {
	var tokenNull, projectURLNull sql.NullString
	err = s.db.QueryRow(`
		SELECT COALESCE(github_owner,''), COALESCE(github_repo_name,''), github_token, github_project_url
		FROM projects WHERE id = $1
	`, projectID).Scan(&owner, &repo, &tokenNull, &projectURLNull)
	if tokenNull.Valid {
		token = tokenNull.String
	}
	if projectURLNull.Valid {
		projectURL = strings.TrimSpace(projectURLNull.String)
	}
	return
}

// fetchProjectByURL fetches GitHub Projects V2 info for an explicit project URL.
// Supports https://github.com/orgs/{org}/projects/{num} and
//          https://github.com/users/{user}/projects/{num}
func fetchProjectByURL(ctx context.Context, token, projectURL string) (*ghProjectInfo, error) {
	// Parse: https://github.com/{orgs|users}/{name}/projects/{number}
	parts := strings.Split(strings.TrimPrefix(projectURL, "https://github.com/"), "/")
	if len(parts) < 4 || parts[2] != "projects" {
		return nil, fmt.Errorf("invalid GitHub project URL: %s", projectURL)
	}
	ownerType := parts[0] // "orgs" or "users"
	name := parts[1]
	number, err := strconv.Atoi(parts[3])
	if err != nil {
		return nil, fmt.Errorf("invalid project number in URL: %s", projectURL)
	}

	var q string
	var vars map[string]interface{}
	if ownerType == "orgs" {
		q = `query($org: String!, $number: Int!) {
  organization(login: $org) {
    projectV2(number: $number) {
      id
      fields(first: 20) {
        nodes {
          ... on ProjectV2SingleSelectField { id name options { id name } }
        }
      }
    }
  }
}`
		vars = map[string]interface{}{"org": name, "number": number}
	} else {
		q = `query($login: String!, $number: Int!) {
  user(login: $login) {
    projectV2(number: $number) {
      id
      fields(first: 20) {
        nodes {
          ... on ProjectV2SingleSelectField { id name options { id name } }
        }
      }
    }
  }
}`
		vars = map[string]interface{}{"login": name, "number": number}
	}

	var result struct {
		Data struct {
			Organization *struct {
				ProjectV2 *ghProjectV2 `json:"projectV2"`
			} `json:"organization"`
			User *struct {
				ProjectV2 *ghProjectV2 `json:"projectV2"`
			} `json:"user"`
		} `json:"data"`
		Errors []struct {
			Message string `json:"message"`
		} `json:"errors"`
	}
	if err := fetchGitHubGraphQL(ctx, token, q, vars, &result); err != nil {
		return nil, err
	}
	if len(result.Errors) > 0 {
		return nil, fmt.Errorf("graphql: %s", result.Errors[0].Message)
	}

	var proj *ghProjectV2
	if result.Data.Organization != nil {
		proj = result.Data.Organization.ProjectV2
	} else if result.Data.User != nil {
		proj = result.Data.User.ProjectV2
	}
	if proj == nil {
		return nil, fmt.Errorf("project not found at URL: %s", projectURL)
	}

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
	if best == nil {
		return nil, fmt.Errorf("no single-select status field found in project")
	}
	return &ghProjectInfo{ProjectID: proj.ID, FieldID: best.ID, Options: best.Options}, nil
}

// --- Handlers ---

// HandleGitHubPreview fetches GitHub data without importing anything.
// resolveGitHubCommentAuthor returns the TaskAI user ID and comment body for a GitHub comment.
// If the commenter has linked their GitHub account via OAuth, their TaskAI user ID is used
// and the body is returned as-is. Otherwise falls back to fallbackUserID with an attribution prefix.
func (s *Server) resolveGitHubCommentAuthor(ctx context.Context, gc ghIssueComment, fallbackUserID int64) (userID int64, body string) {
	if gc.User == nil || gc.User.ID == 0 {
		return fallbackUserID, gc.Body
	}
	var linkedUserID int64
	_ = s.db.QueryRowContext(ctx, `
		SELECT user_id FROM oauth_providers
		WHERE provider = 'github' AND provider_user_id = $1
		LIMIT 1
	`, strconv.FormatInt(gc.User.ID, 10)).Scan(&linkedUserID)
	if linkedUserID != 0 {
		return linkedUserID, gc.Body
	}
	body = gc.Body
	if gc.User.Login != "" {
		body = "**@" + gc.User.Login + "** (GitHub):\n\n" + gc.Body
	}
	return fallbackUserID, body
}

func (s *Server) upsertReactions(ctx context.Context, taskID, commentID int64, r *ghReactions) {
	if r == nil {
		return
	}
	counts := map[string]int{
		"+1": r.PlusOne, "-1": r.MinusOne, "laugh": r.Laugh,
		"hooray": r.Hooray, "confused": r.Confused, "heart": r.Heart,
		"rocket": r.Rocket, "eyes": r.Eyes,
	}
	for reaction, count := range counts {
		if taskID != 0 {
			_, _ = s.db.ExecContext(ctx, `
				INSERT INTO github_reactions (task_id, reaction, count)
				VALUES ($1, $2, $3)
				ON CONFLICT (task_id, reaction) WHERE task_id IS NOT NULL
				DO UPDATE SET count = EXCLUDED.count, updated_at = NOW()
			`, taskID, reaction, count)
		} else {
			_, _ = s.db.ExecContext(ctx, `
				INSERT INTO github_reactions (task_comment_id, reaction, count)
				VALUES ($1, $2, $3)
				ON CONFLICT (task_comment_id, reaction) WHERE task_comment_id IS NOT NULL
				DO UPDATE SET count = EXCLUDED.count, updated_at = NOW()
			`, commentID, reaction, count)
		}
	}
}

// HandleGitHubDiscoverMappings fetches the project board to discover unique
// assignees and status columns, auto-matches them to TaskAI members/swim lanes,
// and registers them into the mapping tables (ON CONFLICT DO NOTHING so manual
// overrides are preserved). Returns a JSON summary.
// POST /api/projects/{id}/github/discover-mappings
func (s *Server) HandleGitHubDiscoverMappings(w http.ResponseWriter, r *http.Request) {
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

	owner, repo, token, projectURL, err := s.loadGitHubConfig(projectID)
	if err != nil {
		http.Error(w, "Failed to load project config", http.StatusInternalServerError)
		return
	}
	if owner == "" || repo == "" {
		respondError(w, http.StatusBadRequest, "GitHub owner and repo name must be configured first", "missing_config")
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	// 1. Fetch project board status columns
	var projInfo *ghProjectInfo
	if projectURL != "" {
		projInfo, _ = fetchProjectByURL(ctx, token, projectURL)
	} else {
		projInfo, _ = fetchProjectStatusColumns(ctx, token, owner, repo)
	}

	// 2. Collect unique status column names and assignee logins from board items
	statusNames := map[string]struct{}{}
	loginSet := map[string]ghUser{}

	if projInfo != nil {
		// Add all column options as status names
		for _, opt := range projInfo.Options {
			statusNames[opt.Name] = struct{}{}
		}

		// Fetch board items to discover assignees and additional info
		issueColumnMap, err := fetchProjectIssueStatuses(ctx, token, projInfo.ProjectID, projInfo.FieldID, s.logger)
		if err == nil {
			for _, item := range issueColumnMap {
				if item.StatusName != "" {
					statusNames[item.StatusName] = struct{}{}
				}
				if item.Issue != nil && item.Issue.Assignees != nil {
					for _, a := range item.Issue.Assignees.Nodes {
						if a.Login != "" {
							loginSet[a.Login] = a
						}
					}
				}
			}
		}
	}

	// Add standard issue state keys
	for _, key := range []string{"open", "closed", "closed:not_planned"} {
		statusNames[key] = struct{}{}
	}

	// 3. If no board items found, try REST issues for user discovery (1 page)
	if len(loginSet) == 0 {
		base := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)
		var pageIssues []ghIssue
		if err := fetchGitHubJSON(ctx, token, base+"/issues?state=all&per_page=100&page=1", &pageIssues); err == nil {
			for _, iss := range pageIssues {
				if iss.PullRequest != nil {
					continue
				}
				for _, a := range iss.Assignees {
					if a.Login != "" {
						loginSet[a.Login] = a
					}
				}
				if iss.Assignee != nil && iss.Assignee.Login != "" {
					loginSet[iss.Assignee.Login] = *iss.Assignee
				}
			}
		}
	}

	// 4. Auto-match users against team members
	type memberInfo struct {
		UserID    int64
		Email     string
		Name      string
		FirstName string
		LastName  string
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT DISTINCT u.id, u.email, COALESCE(u.name,''), COALESCE(u.first_name,''), COALESCE(u.last_name,'')
		FROM users u
		JOIN team_members tm ON tm.user_id = u.id
		WHERE tm.team_id = (SELECT team_id FROM projects WHERE id = $1)
	`, projectID)
	if err != nil {
		respondError(w, http.StatusInternalServerError, "Failed to load team members", "db_error")
		return
	}
	var members []memberInfo
	for rows.Next() {
		var m memberInfo
		if err := rows.Scan(&m.UserID, &m.Email, &m.Name, &m.FirstName, &m.LastName); err == nil {
			members = append(members, m)
		}
	}
	rows.Close()

	// 5. Auto-match and register user mappings
	discoveredUsers := 0
	for login, ghU := range loginSet {
		var matchedUID interface{} = nil
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
				matchedUID = m.UserID
				break
			}
		}
		_, _ = s.db.ExecContext(ctx,
			s.db.Rebind(`INSERT INTO github_user_mappings (project_id, github_login, user_id)
				VALUES (?, ?, ?)
				ON CONFLICT(project_id, github_login) DO NOTHING`),
			projectID, login, matchedUID)
		discoveredUsers++
	}

	// 6. Auto-match and register status mappings
	lanes, _ := s.loadSwimLaneInfos(ctx, projectID)
	discoveredStatuses := 0
	for name := range statusNames {
		var laneVal interface{} = nil
		if laneID, _ := autoMatchStatusToLane(name, lanes); laneID != 0 {
			laneVal = laneID
		}
		_, _ = s.db.ExecContext(ctx,
			s.db.Rebind(`INSERT INTO github_status_mappings (project_id, status_key, swim_lane_id)
				VALUES (?, ?, ?)
				ON CONFLICT(project_id, status_key) DO NOTHING`),
			projectID, name, laneVal)
		discoveredStatuses++
	}

	// 7. Count totals from the tables
	var totalStatuses, totalUsers int
	_ = s.db.QueryRowContext(ctx,
		s.db.Rebind(`SELECT COUNT(*) FROM github_status_mappings WHERE project_id = ?`),
		projectID).Scan(&totalStatuses)
	_ = s.db.QueryRowContext(ctx,
		s.db.Rebind(`SELECT COUNT(*) FROM github_user_mappings WHERE project_id = ?`),
		projectID).Scan(&totalUsers)

	respondJSON(w, http.StatusOK, map[string]interface{}{
		"discovered_statuses": discoveredStatuses,
		"discovered_users":    discoveredUsers,
		"total_statuses":      totalStatuses,
		"total_users":         totalUsers,
	})
}

// POST /api/projects/{id}/github/sync
func (s *Server) HandleGitHubSync(w http.ResponseWriter, r *http.Request) {
	s.handleGitHubImport(w, r)
}

func (s *Server) handleGitHubImport(w http.ResponseWriter, r *http.Request) {
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

	// Merge in any previously saved mappings (request values take priority).
	req.StatusAssignments, req.UserAssignments = s.loadSavedGitHubMappings(
		r.Context(), int64(projectID), req.StatusAssignments, req.UserAssignments)

	owner, repo, storedToken, projectURL, err := s.loadGitHubConfig(projectID)
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
	// Disable the HTTP server's write deadline — imports can take minutes.
	http.NewResponseController(w).SetWriteDeadline(time.Time{}) //nolint:errcheck

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

	// Force full sync: delete all GitHub-sourced tasks and sprints, clear last-sync timestamp
	if req.ForceFullSync {
		progress("reset", "Clearing GitHub-imported tasks and sprints...", 0, 0)
		// Explicitly delete comments first — FK cascade may not fire reliably
		// across all DB drivers. Belt-and-suspenders cleanup before re-import.
		_, _ = s.db.ExecContext(ctx, `
			DELETE FROM task_comments
			WHERE task_id IN (
				SELECT id FROM tasks WHERE project_id=$1 AND github_issue_number IS NOT NULL
			)`, projectID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM tasks WHERE project_id=$1 AND github_issue_number IS NOT NULL`, projectID)
		_, _ = s.db.ExecContext(ctx, `DELETE FROM sprints WHERE project_id=$1 AND github_milestone_number IS NOT NULL`, projectID)
		_, _ = s.db.ExecContext(ctx, `UPDATE projects SET github_last_sync=NULL WHERE id=$1`, projectID)
		s.logger.Info("Force full sync: cleared GitHub data", zap.Int("project_id", projectID))
	}

	// Compute since parameter for efficient incremental sync.
	// Note: since is intentionally NOT applied to the issues list — only to comments.
	// This ensures assignee/status changes are always re-evaluated against current user mappings.
	sinceParam := ""
	if !req.ForceFullSync {
		var lastSync sql.NullTime
		_ = s.db.QueryRowContext(ctx, `SELECT github_last_sync FROM projects WHERE id=$1`, projectID).Scan(&lastSync)
		if lastSync.Valid {
			sinceParam = lastSync.Time.UTC().Format(time.RFC3339)
		}
	}

	// Determine sync mode for logging
	syncMode := "incremental_all"
	if req.ForceFullSync {
		syncMode = "full_all"
	} else if req.Filter != nil && req.Filter.State == "open" {
		syncMode = "incremental_open"
	} else if req.Filter != nil && req.Filter.State == "closed" {
		syncMode = "incremental_closed"
	}

	// Record sync log entry
	var syncLogID int64
	_ = s.db.QueryRowContext(ctx, `
		INSERT INTO github_sync_logs (project_id, triggered_by, sync_mode) VALUES ($1, 'manual', $2) RETURNING id
	`, projectID, syncMode).Scan(&syncLogID)

	finishSyncLog := func(result *GitHubPullResponse, syncErr error) {
		status := "success"
		var errMsg *string
		if syncErr != nil {
			status = "failed"
			msg := syncErr.Error()
			errMsg = &msg
		}
		if syncLogID != 0 {
			_, _ = s.db.ExecContext(context.Background(), `
				UPDATE github_sync_logs
				SET completed_at = $1, status = $2, error_message = $3,
				    created_tasks = $4, updated_tasks = $5, created_comments = $6, skipped_tasks = $7, pushed_comments = $8
				WHERE id = $9
			`, time.Now(), status, errMsg,
				result.CreatedTasks, result.UpdatedTasks, result.CreatedComments, result.SkippedTasks, result.PushedComments,
				syncLogID)
			// Purge old logs — keep last 100 per project
			_, _ = s.db.ExecContext(context.Background(), `
				DELETE FROM github_sync_logs WHERE project_id = $1 AND id NOT IN (
					SELECT id FROM github_sync_logs WHERE project_id = $1 ORDER BY started_at DESC LIMIT 100
				)
			`, projectID)
		}
	}

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
	// Track Projects V2 status keys not yet in the mapping table so we can register them.
	unknownStatusKeys := map[string]struct{}{}

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

			var existingID int64
			err := s.db.QueryRowContext(ctx, `
				SELECT id FROM sprints WHERE project_id = $1 AND github_milestone_number = $2
			`, projectID, m.Number).Scan(&existingID)

			if err == sql.ErrNoRows {
				// Try name-based match for manually-created sprints missing github_milestone_number.
				// Backfill the number so future syncs resolve correctly without creating duplicates.
				nameErr := s.db.QueryRowContext(ctx, `
					SELECT id FROM sprints WHERE project_id = $1 AND name = $2 AND github_milestone_number IS NULL
				`, projectID, m.Title).Scan(&existingID)
				if nameErr == nil {
					_, _ = s.db.ExecContext(ctx, `
						UPDATE sprints SET github_milestone_number = $1, status = $2, end_date = COALESCE($3, end_date) WHERE id = $4
					`, m.Number, status, dueDate, existingID)
				} else {
					// Insert new sprint from milestone
					err = s.db.QueryRowContext(ctx, `
						INSERT INTO sprints (user_id, project_id, name, status, end_date, github_milestone_number)
						VALUES ($1, $2, $3, $4, $5, $6)
						ON CONFLICT (project_id, github_milestone_number) DO NOTHING
						RETURNING id
					`, userID, projectID, m.Title, status, dueDate, m.Number).Scan(&existingID)
					if err == nil {
						result.CreatedSprints++
					}
				}
			} else if err == nil {
				// Update existing
				_, _ = s.db.ExecContext(ctx, `
					UPDATE sprints SET name = $1, status = $2, end_date = $3 WHERE id = $4
				`, m.Title, status, dueDate, existingID)
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
		}
	}

	// --- Import Tasks from Issues ---
	// allIssues is declared here so the comment section below can also use it for filtering.
	var allIssues []ghIssue
	if req.PullTasks {
		progress("issues", "Fetching issues...", 0, 0)
		// NOTE: GitHub's /issues endpoint returns both issues and pull requests.
		// We only handle issues for now. To add PR support, remove the PullRequest filter below.
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
			for i := range pageIssues {
				if pageIssues[i].PullRequest == nil {
					pageIssues[i].Repo = owner + "/" + repo
					allIssues = append(allIssues, pageIssues[i])
				}
			}
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

		// Build milestone→sprint_id maps from all sprints linked to GitHub milestones.
		// milestoneToSprintID: number-based (works for same-repo issues)
		// milestoneNameToSprintID: name-based fallback (needed for cross-repo issues
		// where milestone numbers differ per repo but names match)
		milestoneToSprintID := map[int]int64{}
		milestoneNameToSprintID := map[string]int64{}
		{
			rows, err := s.db.QueryContext(ctx, `
				SELECT github_milestone_number, name, id FROM sprints
				WHERE project_id = $1 AND github_milestone_number IS NOT NULL
			`, projectID)
			if err == nil {
				defer rows.Close()
				for rows.Next() {
					var mnum int
					var mname string
					var sid int64
					if err := rows.Scan(&mnum, &mname, &sid); err == nil {
						milestoneToSprintID[mnum] = sid
						milestoneNameToSprintID[mname] = sid
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

		// Build a lowercase-keyed copy of StatusAssignments for case-insensitive fallback.
		statusAssignmentsLower := map[string]int64{}
		for k, v := range req.StatusAssignments {
			statusAssignmentsLower[strings.ToLower(k)] = v
		}

		// Fetch GitHub Projects V2 issue→column+itemID map (best-effort; ignore errors)
		// Use the explicit project URL if configured; otherwise auto-detect from repo/org.
		issueColumnMap := map[string]ghProjectItemStatus{}
		var projInfo *ghProjectInfo
		if projectURL != "" {
			if pi, piErr := fetchProjectByURL(ctx, token, projectURL); piErr == nil {
				projInfo = pi
			} else {
				s.logger.Warn("Failed to fetch GitHub project by URL", zap.String("url", projectURL), zap.Error(piErr))
			}
		} else {
			if pi, piErr := fetchProjectStatusColumns(ctx, token, owner, repo); piErr == nil {
				projInfo = pi
			}
		}
		if projInfo != nil {
			// Persist project/field IDs so push operations can use them later
			_, _ = s.db.ExecContext(ctx,
				s.db.Rebind(`UPDATE projects SET github_project_id = ?, github_status_field_id = ? WHERE id = ?`),
				projInfo.ProjectID, projInfo.FieldID, projectID)

			if m, err := fetchProjectIssueStatuses(ctx, token, projInfo.ProjectID, projInfo.FieldID, s.logger); err == nil {
				issueColumnMap = m
			}

			// Persist github_option_id on swim lanes by matching column options → lanes
			lanes, _ := s.loadSwimLaneInfos(ctx, projectID)
			for _, opt := range projInfo.Options {
				if laneID, _ := fuzzyMatchColumn(opt.Name, lanes); laneID != 0 {
					optID := opt.ID
					_, _ = s.db.ExecContext(ctx,
						s.db.Rebind(`UPDATE swim_lanes SET github_option_id = ? WHERE id = ?`), optID, laneID)
				}
			}
		}

		// When a project board is configured, use its items as the primary issue source.
		// This captures issues from ALL repos on the board, not just the configured repo.
		if len(issueColumnMap) > 0 {
			allIssues = nil // clear any REST-fetched issues
			for _, item := range issueColumnMap {
				if item.Issue == nil || item.Issue.Number == 0 {
					continue
				}
				allIssues = append(allIssues, ghIssueFromProjectItem(item))
			}
			s.logger.Info("Using project board as issue source",
				zap.Int("project_id", projectID),
				zap.Int("board_issues", len(allIssues)),
			)
		}

		// Build iteration_title→sprint_id map from Projects V2 iteration values
		iterationToSprintID := map[string]int64{}
		{
			// Collect unique iteration titles from the project board
			iterationNames := map[string]struct{}{}
			for _, item := range issueColumnMap {
				if item.IterationTitle != "" {
					iterationNames[item.IterationTitle] = struct{}{}
				}
			}
			// Look up each iteration name as a sprint; create if missing
			for name := range iterationNames {
				var sid int64
				err := s.db.QueryRowContext(ctx, `SELECT id FROM sprints WHERE project_id = $1 AND name = $2`, projectID, name).Scan(&sid)
				if err == sql.ErrNoRows {
					err = s.db.QueryRowContext(ctx, `INSERT INTO sprints (project_id, name) VALUES ($1, $2) RETURNING id`, projectID, name).Scan(&sid)
				}
				if err == nil {
					iterationToSprintID[name] = sid
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

			// Resolve assignees — collect all mapped user IDs from issue.Assignees.
			// The first mapped user becomes the legacy assignee_id; all go into task_assignees.
			var assigneeID *int64
			var allAssigneeIDs []int64
			seen := map[int64]bool{}
			logins := make([]string, 0, len(issue.Assignees))
			if issue.Assignee != nil {
				logins = append(logins, issue.Assignee.Login)
			}
			for _, a := range issue.Assignees {
				if a.Login != "" && (len(logins) == 0 || logins[0] != a.Login) {
					logins = append(logins, a.Login)
				}
			}
			for _, login := range logins {
				if uid, ok := req.UserAssignments[login]; ok && uid != 0 && !seen[uid] {
					seen[uid] = true
					allAssigneeIDs = append(allAssigneeIDs, uid)
					if assigneeID == nil {
						assigneeID = &allAssigneeIDs[0]
					}
				}
			}

			// Resolve sprint: prefer Projects V2 iteration, fall back to milestone
			var sprintID *int64
			colKey := issueColumnKey(issue)
			if itemStatus, ok := issueColumnMap[colKey]; ok && itemStatus.IterationTitle != "" {
				if sid, ok := iterationToSprintID[itemStatus.IterationTitle]; ok {
					sprintID = &sid
				}
			}
			if sprintID == nil && issue.Milestone != nil {
				isPrimaryRepo := issue.Repo == owner+"/"+repo
				if isPrimaryRepo {
					// Same-repo: milestone numbers are reliable
					if sid, ok := milestoneToSprintID[issue.Milestone.Number]; ok {
						sprintID = &sid
					}
				}
				if sprintID == nil && issue.Milestone.Title != "" {
					// Cross-repo or number miss: match by milestone title
					if sid, ok := milestoneNameToSprintID[issue.Milestone.Title]; ok {
						sprintID = &sid
					}
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
			ghStartDate := ""
			ghDueDate := ""

			// 1. Projects V2 column
			if itemStatus, ok := issueColumnMap[colKey]; ok {
				ghItemID = itemStatus.ItemID
				ghStartDate = itemStatus.StartDate
				ghDueDate = itemStatus.DueDate
				if itemStatus.StatusName != "" {
					if laneID, ok := req.StatusAssignments[itemStatus.StatusName]; ok && laneID > 0 {
						swimLaneID = &laneID
					} else if laneID, ok := statusAssignmentsLower[strings.ToLower(itemStatus.StatusName)]; ok && laneID > 0 {
						swimLaneID = &laneID
					} else {
						// Status key not yet mapped — register it so it surfaces in the UI.
						unknownStatusKeys[itemStatus.StatusName] = struct{}{}
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

			var existingID int64
			err := s.db.QueryRowContext(ctx, `
				SELECT id FROM tasks WHERE project_id = $1 AND github_repo = $2 AND github_issue_number = $3
			`, projectID, issue.Repo, issue.Number).Scan(&existingID)

			if err == sql.ErrNoRows {
				// Insert new task
				err = s.db.QueryRowContext(ctx, `
					INSERT INTO tasks (project_id, task_number, title, description, status, priority, assignee_id, sprint_id, github_issue_number, github_repo, swim_lane_id, github_project_item_id, start_date, due_date)
					VALUES ($1, $2, $3, $4, $5, 'medium', $6, $7, $8, $9, $10, $11, $12, $13)
					ON CONFLICT (project_id, github_repo, github_issue_number) WHERE github_issue_number IS NOT NULL DO NOTHING
					RETURNING id
				`, projectID, nextNumber, issue.Title, description, taskStatus, assigneeID, sprintID, issue.Number, issue.Repo, swimLaneID, nullableStr(ghItemID), nullableStr(ghStartDate), nullableStr(ghDueDate)).Scan(&existingID)
				if err == nil {
					nextNumber++
					result.CreatedTasks++
					s.insertTaskTags(ctx, existingID, issue.Labels, labelToTagID)
					s.upsertReactions(ctx, existingID, 0, issue.Reactions)
					s.syncGitHubTaskAssignees(ctx, existingID, allAssigneeIDs)
				} else {
					result.SkippedTasks++
				}
			} else if err == nil {
				// Update existing
				_, _ = s.db.ExecContext(ctx, `
					UPDATE tasks SET title = $1, description = $2, status = $3, assignee_id = $4, sprint_id = $5, swim_lane_id = $6, github_project_item_id = COALESCE(NULLIF($7,''), github_project_item_id),
					start_date = COALESCE($8, start_date), due_date = COALESCE($9, due_date)
					WHERE id = $10
				`, issue.Title, description, taskStatus, assigneeID, sprintID, swimLaneID, ghItemID, nullableStr(ghStartDate), nullableStr(ghDueDate), existingID)
				_, _ = s.db.ExecContext(ctx, `DELETE FROM task_tags WHERE task_id = $1`, existingID)
				s.insertTaskTags(ctx, existingID, issue.Labels, labelToTagID)
				s.upsertReactions(ctx, existingID, 0, issue.Reactions)
				s.syncGitHubTaskAssignees(ctx, existingID, allAssigneeIDs)
				result.UpdatedTasks++
			}
		}
	}

	// --- Import Comments from Issues ---
	if req.PullComments {
		// Build a set of "repo#number" keys updated since sinceParam (from already-fetched allIssues).
		// We skip comment API calls for issues that haven't changed — but when we do fetch,
		// we fetch ALL comments (no since filter) so we never miss comments that predate our
		// last sync. ON CONFLICT handles deduplication.
		// If allIssues is empty (PullTasks was false), updatedIssues is nil — the skip check
		// below treats nil as "fetch all" so no comments are dropped.
		var updatedIssues map[string]bool
		if len(allIssues) > 0 {
			updatedIssues = map[string]bool{}
			for _, iss := range allIssues {
				if sinceParam == "" || iss.UpdatedAt >= sinceParam {
					updatedIssues[issueColumnKey(iss)] = true
				}
			}
		}

		// Collect tasks with github_issue_number in this project (including their repo)
		rows, err := s.db.QueryContext(ctx, `
			SELECT id, github_issue_number, github_repo FROM tasks
			WHERE project_id = $1 AND github_issue_number IS NOT NULL
		`, projectID)
		if err == nil {
			type taskRef struct {
				taskID   int64
				issueNum int
				repo     string
			}
			var taskRefs []taskRef
			for rows.Next() {
				var tr taskRef
				if err := rows.Scan(&tr.taskID, &tr.issueNum, &tr.repo); err == nil {
					taskRefs = append(taskRefs, tr)
				}
			}
			rows.Close()

			// Build set of tasks that already have GitHub comments imported.
			// Tasks with zero existing comments must always fetch — they may have missed
			// comments created before the first sync or before the since-filter was fixed.
			tasksWithComments := map[int64]bool{}
			crows, cerr := s.db.QueryContext(ctx, `
				SELECT DISTINCT task_id FROM task_comments
				WHERE task_id IN (SELECT id FROM tasks WHERE project_id = $1 AND github_issue_number IS NOT NULL)
				AND github_comment_id IS NOT NULL
			`, projectID)
			if cerr == nil {
				for crows.Next() {
					var tid int64
					if crows.Scan(&tid) == nil {
						tasksWithComments[tid] = true
					}
				}
				crows.Close()
			}

			progress("comments", fmt.Sprintf("Fetching comments for %d issues...", len(taskRefs)), 0, len(taskRefs))
			for i, tr := range taskRefs {
				if i%10 == 0 && i > 0 {
					progress("comments", fmt.Sprintf("Fetched comments for %d/%d issues...", i, len(taskRefs)), i, len(taskRefs))
				}
				// Skip issues not updated since last sync only if the task already has comments.
				// Tasks with zero comments must always fetch to catch comments that predate the sync.
				trKey := fmt.Sprintf("%s#%d", tr.repo, tr.issueNum)
				if sinceParam != "" && !updatedIssues[trKey] && tasksWithComments[tr.taskID] {
					continue
				}
				// Use the task's github_repo for the comment API URL (supports cross-repo issues)
				commentRepo := tr.repo
				if commentRepo == "" {
					commentRepo = owner + "/" + repo // fallback to project config
				}
				var ghComments []ghIssueComment
				// No since filter on comments — fetch all and rely on ON CONFLICT for dedup.
				// Paginate to handle issues with >100 comments.
				for page := 1; ; page++ {
					commentsURL := fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/comments?per_page=100&page=%d", commentRepo, tr.issueNum, page)
					var pageComments []ghIssueComment
					if err := fetchGitHubJSON(ctx, token, commentsURL, &pageComments); err != nil {
						break // best-effort
					}
					ghComments = append(ghComments, pageComments...)
					if len(pageComments) < 100 {
						break
					}
				}
				// Resolve owner once per task for unlinked GitHub users
				var ownerID int64
				_ = s.db.QueryRowContext(ctx, `SELECT user_id FROM project_members WHERE project_id = $1 AND role = 'owner' LIMIT 1`, projectID).Scan(&ownerID)
				if ownerID == 0 {
					_ = s.db.QueryRowContext(ctx, `SELECT user_id FROM project_members WHERE project_id = $1 LIMIT 1`, projectID).Scan(&ownerID)
				}
				for _, gc := range ghComments {
					if gc.Body == "" {
						continue
					}
					commentUserID, body := s.resolveGitHubCommentAuthor(ctx, gc, ownerID)
					var newCID int64
					err := s.db.QueryRowContext(ctx, `
						INSERT INTO task_comments (task_id, user_id, comment, github_comment_id)
						VALUES ($1, $2, $3, $4)
						ON CONFLICT (github_comment_id) WHERE github_comment_id IS NOT NULL DO NOTHING
						RETURNING id
					`, tr.taskID, commentUserID, body, gc.ID).Scan(&newCID)
					if err == nil {
						result.CreatedComments++
					} else if err != sql.ErrNoRows {
						s.logger.Warn("Failed to insert GitHub comment",
							zap.Int64("task_id", tr.taskID),
							zap.Int64("github_comment_id", gc.ID),
							zap.Error(err))
					}
					commentDBID := newCID
					if commentDBID == 0 && gc.ID != 0 {
						_ = s.db.QueryRowContext(ctx, `SELECT id FROM task_comments WHERE github_comment_id = $1`, gc.ID).Scan(&commentDBID)
					}
					s.upsertReactions(ctx, 0, commentDBID, gc.Reactions)
				}
			}
		}
	}

	// --- Push Unpushed TaskAI Comments to GitHub ---
	s.pushUnpushedComments(ctx, projectID, owner, repo, token, &result)

	// Update last sync timestamp
	_, _ = s.db.ExecContext(ctx, `UPDATE projects SET github_last_sync = $1 WHERE id = $2`, time.Now(), projectID)

	// Persist the mappings used in this sync so future syncs reuse them.
	// Also register any newly-discovered status keys so they surface in the mapping UI.
	s.registerUnknownStatusKeys(ctx, int64(projectID), unknownStatusKeys)
	s.saveGitHubMappings(ctx, int64(projectID), req.StatusAssignments, req.UserAssignments)

	finishSyncLog(&result, nil)
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

// syncGitHubTaskAssignees replaces the task_assignees rows for a GitHub-synced task
// with the resolved TaskAI user IDs. Safe to call with an empty slice (no-op).
func (s *Server) syncGitHubTaskAssignees(ctx context.Context, taskID int64, userIDs []int64) {
	if len(userIDs) == 0 {
		return
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM task_assignees WHERE task_id = $1`, taskID)
	for _, uid := range userIDs {
		_, _ = s.db.ExecContext(ctx, `
			INSERT INTO task_assignees (task_id, user_id) VALUES ($1, $2)
			ON CONFLICT (task_id, user_id) DO NOTHING
		`, taskID, uid)
	}
}

// nullableStr returns nil if s is empty, otherwise returns &s (for use as SQL NULL).
func nullableStr(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// issueColumnKey returns the map key for looking up a ghIssue in the issueColumnMap.
// Format: "owner/repo#number".
func issueColumnKey(issue ghIssue) string {
	return fmt.Sprintf("%s#%d", issue.Repo, issue.Number)
}

// ghIssueFromProjectItem converts a GraphQL project item to a ghIssue.
// This allows project board items (which span multiple repos) to be processed
// through the same pipeline as REST API issues.
func ghIssueFromProjectItem(item ghProjectItemStatus) ghIssue {
	c := item.Issue
	if c == nil {
		return ghIssue{}
	}
	issue := ghIssue{
		Number: c.Number,
		Title:  c.Title,
		Body:   c.Body,
		State:  strings.ToLower(c.State), // GraphQL returns "OPEN"/"CLOSED"; REST returns "open"/"closed"
		Repo:   c.Repo(),
	}
	if c.Milestone != nil {
		issue.Milestone = c.Milestone
	}
	if c.Assignees != nil {
		issue.Assignees = c.Assignees.Nodes
		if len(issue.Assignees) > 0 {
			issue.Assignee = &issue.Assignees[0]
		}
	}
	if c.Labels != nil {
		issue.Labels = c.Labels.Nodes
	}
	return issue
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

// pushUnpushedComments pushes all TaskAI comments that haven't been synced to GitHub yet.
// Called during both manual and auto sync. Only pushes if github_push_enabled is true.
func (s *Server) pushUnpushedComments(ctx context.Context, projectID int, owner, repo, token string, result *GitHubPullResponse) {
	var pushEnabled bool
	_ = s.db.QueryRowContext(ctx, `SELECT github_push_enabled FROM projects WHERE id = $1`, projectID).Scan(&pushEnabled)
	if !pushEnabled || owner == "" || token == "" {
		return
	}

	type unpushedComment struct {
		commentID   int64
		taskID      int64
		userID      int64
		comment     string
		issueNumber int64
		ghRepo      string // "owner/repo" from the task
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT tc.id, tc.task_id, tc.user_id, tc.comment, t.github_issue_number, t.github_repo
		FROM task_comments tc
		JOIN tasks t ON t.id = tc.task_id
		WHERE t.project_id = $1
		  AND t.github_issue_number IS NOT NULL
		  AND tc.github_comment_id IS NULL
	`, projectID)
	if err != nil {
		s.logger.Warn("Failed to query unpushed comments", zap.Int("project_id", projectID), zap.Error(err))
		return
	}
	var unpushed []unpushedComment
	for rows.Next() {
		var uc unpushedComment
		if err := rows.Scan(&uc.commentID, &uc.taskID, &uc.userID, &uc.comment, &uc.issueNumber, &uc.ghRepo); err == nil {
			unpushed = append(unpushed, uc)
		}
	}
	rows.Close()

	for _, uc := range unpushed {
		// Get user display name (no agent prefix — that's TaskAI-internal)
		var firstName, lastName, name sql.NullString
		_ = s.db.QueryRowContext(ctx, `SELECT first_name, last_name, name FROM users WHERE id = $1`, uc.userID).Scan(&firstName, &lastName, &name)
		displayName := ""
		if full := strings.TrimSpace(firstName.String + " " + lastName.String); full != "" {
			displayName = full
		} else if name.Valid && name.String != "" {
			displayName = name.String
		}

		ghBody := uc.comment
		if displayName != "" {
			ghBody = "**" + displayName + "** (via TaskAI):\n\n" + uc.comment
		}
		// Use the task's github_repo for cross-repo comment push; fall back to project config
		pushOwner, pushRepo := owner, repo
		if uc.ghRepo != "" {
			if parts := strings.SplitN(uc.ghRepo, "/", 2); len(parts) == 2 {
				pushOwner, pushRepo = parts[0], parts[1]
			}
		}
		ghCommentID, err := pushCommentToGitHub(ctx, token, pushOwner, pushRepo, uc.issueNumber, ghBody)
		if err != nil {
			s.logger.Warn("Failed to push unpushed comment to GitHub",
				zap.Int64("comment_id", uc.commentID),
				zap.Int64("task_id", uc.taskID),
				zap.Error(err))
			continue
		}
		_, _ = s.db.ExecContext(ctx, `UPDATE task_comments SET github_comment_id = $1 WHERE id = $2`, ghCommentID, uc.commentID)
		result.PushedComments++
	}
}

// pushReactionToGitHub adds a reaction to a GitHub issue or comment.
// targetType: "issue" or "comment". targetID: issue number or GitHub comment ID.
// Returns GitHub's reaction ID (needed for deletion).
func pushReactionToGitHub(ctx context.Context, token, owner, repo string, targetID int64, reaction, targetType string) (int64, error) {
	var url string
	if targetType == "comment" {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/comments/%d/reactions", owner, repo, targetID)
	} else {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/reactions", owner, repo, targetID)
	}
	payload, _ := json.Marshal(map[string]string{"content": reaction})
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
		return 0, fmt.Errorf("github reaction push error %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var created struct {
		ID int64 `json:"id"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&created); err != nil {
		return 0, err
	}
	return created.ID, nil
}

// deleteReactionFromGitHub removes a reaction from a GitHub issue or comment.
// 404 is treated as success (already deleted).
func deleteReactionFromGitHub(ctx context.Context, token, owner, repo string, targetID, reactionID int64, targetType string) error {
	var url string
	if targetType == "comment" {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/comments/%d/reactions/%d", owner, repo, targetID, reactionID)
	} else {
		url = fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d/reactions/%d", owner, repo, targetID, reactionID)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil // already gone
	}
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("github reaction delete error %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	return nil
}

// GitHubMappingsResponse is returned by HandleGetGitHubMappings.
type GitHubMappingsResponse struct {
	StatusMappings map[string]int64 `json:"status_mappings"` // github_status_key → swim_lane_id (0 = unset)
	UserMappings   map[string]int64 `json:"user_mappings"`   // github_login → user_id (0 = unset)
}

// HandleGetGitHubMappings returns the persisted sync mappings for a project.
// GET /api/projects/{id}/github/mappings
func (s *Server) HandleGetGitHubMappings(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "invalid_input")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	resp := GitHubMappingsResponse{
		StatusMappings: map[string]int64{},
		UserMappings:   map[string]int64{},
	}

	rows, err := s.db.QueryContext(ctx,
		s.db.Rebind(`SELECT status_key, COALESCE(swim_lane_id,0) FROM github_status_mappings WHERE project_id = ?`),
		projectID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var key string
			var laneID int64
			if err := rows.Scan(&key, &laneID); err == nil {
				resp.StatusMappings[key] = laneID
			}
		}
		rows.Close()
	}

	rows, err = s.db.QueryContext(ctx,
		s.db.Rebind(`SELECT github_login, COALESCE(user_id,0) FROM github_user_mappings WHERE project_id = ?`),
		projectID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var login string
			var uid int64
			if err := rows.Scan(&login, &uid); err == nil {
				resp.UserMappings[login] = uid
			}
		}
		rows.Close()
	}

	respondJSON(w, http.StatusOK, resp)
}

// HandleSaveGitHubMappings persists sync mappings for a project.
// PUT /api/projects/{id}/github/mappings
func (s *Server) HandleSaveGitHubMappings(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "invalid_input")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	var req GitHubMappingsResponse
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}

	s.saveGitHubMappings(ctx, int64(projectID), req.StatusMappings, req.UserMappings)
	respondJSON(w, http.StatusOK, map[string]string{"message": "mappings saved"})
}

// saveGitHubMappings upserts status and user mappings for a project.
func (s *Server) saveGitHubMappings(ctx context.Context, projectID int64, statusMappings map[string]int64, userMappings map[string]int64) {
	for key, laneID := range statusMappings {
		var laneVal interface{} = nil
		if laneID > 0 {
			laneVal = laneID
		}
		_, _ = s.db.ExecContext(ctx,
			s.db.Rebind(`INSERT INTO github_status_mappings (project_id, status_key, swim_lane_id)
				VALUES (?, ?, ?)
				ON CONFLICT(project_id, status_key) DO UPDATE SET swim_lane_id = excluded.swim_lane_id`),
			projectID, key, laneVal)
	}
	for login, uid := range userMappings {
		var uidVal interface{} = nil
		if uid > 0 {
			uidVal = uid
		}
		_, _ = s.db.ExecContext(ctx,
			s.db.Rebind(`INSERT INTO github_user_mappings (project_id, github_login, user_id)
				VALUES (?, ?, ?)
				ON CONFLICT(project_id, github_login) DO UPDATE SET user_id = excluded.user_id`),
			projectID, login, uidVal)
	}
}

// registerUnknownStatusKeys inserts newly-discovered Projects V2 status keys with a NULL
// swim_lane_id so they surface in the mapping UI. Uses ON CONFLICT DO NOTHING so it never
// overwrites an existing (user-configured) mapping.
func (s *Server) registerUnknownStatusKeys(ctx context.Context, projectID int64, keys map[string]struct{}) {
	for key := range keys {
		_, _ = s.db.ExecContext(ctx,
			s.db.Rebind(`INSERT INTO github_status_mappings (project_id, status_key, swim_lane_id)
				VALUES (?, ?, NULL)
				ON CONFLICT(project_id, status_key) DO NOTHING`),
			projectID, key)
	}
}

// loadSavedGitHubMappings loads persisted mappings from DB, merging them under request-supplied values.
// Request values take priority (user may have changed them in the UI).
func (s *Server) loadSavedGitHubMappings(ctx context.Context, projectID int64, reqStatus, reqUser map[string]int64) (status, user map[string]int64) {
	status = map[string]int64{}
	user = map[string]int64{}

	// Load saved status mappings as baseline
	rows, err := s.db.QueryContext(ctx,
		s.db.Rebind(`SELECT status_key, COALESCE(swim_lane_id,0) FROM github_status_mappings WHERE project_id = ?`),
		projectID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var key string
			var laneID int64
			if err := rows.Scan(&key, &laneID); err == nil {
				status[key] = laneID
			}
		}
		rows.Close()
	}

	// Load saved user mappings as baseline
	rows, err = s.db.QueryContext(ctx,
		s.db.Rebind(`SELECT github_login, COALESCE(user_id,0) FROM github_user_mappings WHERE project_id = ?`),
		projectID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var login string
			var uid int64
			if err := rows.Scan(&login, &uid); err == nil {
				user[login] = uid
			}
		}
		rows.Close()
	}

	// Request values override saved values
	for k, v := range reqStatus {
		status[k] = v
	}
	for k, v := range reqUser {
		user[k] = v
	}

	return status, user
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
		dueDate           sql.NullTime
		projectID         int64
		githubIssueNumber sql.NullInt64
		milestoneNumber   sql.NullInt64
		owner, repo       string
		tokenNull         sql.NullString
	)
	err = s.db.QueryRowContext(ctx, `
		SELECT t.title, t.description, t.due_date, t.project_id, t.github_issue_number,
		       (SELECT s.github_milestone_number FROM sprints s WHERE s.id = t.sprint_id LIMIT 1),
		       COALESCE(p.github_owner,''), COALESCE(p.github_repo_name,''),
		       p.github_token
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		WHERE t.id = $1
	`, taskID).Scan(&title, &description, &dueDate, &projectID, &githubIssueNumber, &milestoneNumber, &owner, &repo, &tokenNull)
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
	if dueDate.Valid {
		body += "\n\n**Due Date:** " + dueDate.Time.Format("2006-01-02")
	}

	// Reverse-map TaskAI assignees → GitHub logins via github_user_mappings.
	var assigneeLogins []string
	assigneeRows, err := s.db.QueryContext(ctx,
		s.db.Rebind(`SELECT gum.github_login
			FROM task_assignees ta
			JOIN github_user_mappings gum ON gum.project_id = ? AND gum.user_id = ta.user_id
			WHERE ta.task_id = ? AND gum.github_login IS NOT NULL`),
		projectID, taskID)
	if err == nil {
		defer assigneeRows.Close()
		for assigneeRows.Next() {
			var login string
			if assigneeRows.Scan(&login) == nil && login != "" {
				assigneeLogins = append(assigneeLogins, login)
			}
		}
		assigneeRows.Close()
	}

	payload := map[string]interface{}{
		"title": title,
		"body":  body,
	}
	if milestoneNumber.Valid {
		payload["milestone"] = milestoneNumber.Int64
	}
	if len(assigneeLogins) > 0 {
		payload["assignees"] = assigneeLogins
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

	owner, repo, token, _, err := s.loadGitHubConfig(projectID)
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
	// Disable the HTTP server's write deadline — push-all can take minutes.
	http.NewResponseController(w).SetWriteDeadline(time.Time{}) //nolint:errcheck
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

	// Pre-load assignee→github_login mappings for all tasks in this project.
	taskAssignees := map[int64][]string{}
	aRows, err := s.db.QueryContext(ctx,
		s.db.Rebind(`SELECT ta.task_id, gum.github_login
			FROM task_assignees ta
			JOIN github_user_mappings gum ON gum.project_id = ? AND gum.user_id = ta.user_id
			WHERE ta.task_id IN (SELECT id FROM tasks WHERE project_id = ?) AND gum.github_login IS NOT NULL`),
		projectID, projectID)
	if err == nil {
		defer aRows.Close()
		for aRows.Next() {
			var tid int64
			var login string
			if aRows.Scan(&tid, &login) == nil {
				taskAssignees[tid] = append(taskAssignees[tid], login)
			}
		}
		aRows.Close()
	}

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
		if logins := taskAssignees[t.ID]; len(logins) > 0 {
			payload["assignees"] = logins
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

// tryPushAssigneesToGitHub pushes the current task assignees to the linked GitHub issue.
// It's best-effort: errors are logged but do not affect the response.
func (s *Server) tryPushAssigneesToGitHub(ctx context.Context, taskID int64) {
	var (
		issueNumber int64
		projectID   int64
		owner, repo string
		token       string
		pushEnabled bool
	)
	err := s.db.QueryRowContext(ctx, `
		SELECT COALESCE(t.github_issue_number,0), t.project_id,
		       COALESCE(p.github_owner,''), COALESCE(p.github_repo_name,''),
		       COALESCE(p.github_token,''), p.github_push_enabled
		FROM tasks t
		JOIN projects p ON p.id = t.project_id
		WHERE t.id = $1
	`, taskID).Scan(&issueNumber, &projectID, &owner, &repo, &token, &pushEnabled)
	if err != nil || !pushEnabled || issueNumber == 0 || owner == "" || token == "" {
		return
	}

	// Collect GitHub logins from task_assignees join table
	var logins []string
	rows, err := s.db.QueryContext(ctx, `
		SELECT gum.github_login
		FROM task_assignees ta
		JOIN github_user_mappings gum ON gum.project_id = $1 AND gum.user_id = ta.user_id
		WHERE ta.task_id = $2 AND gum.github_login IS NOT NULL
	`, projectID, taskID)
	if err == nil {
		for rows.Next() {
			var login string
			if rows.Scan(&login) == nil && login != "" {
				logins = append(logins, login)
			}
		}
		rows.Close()
	}

	// Fall back to legacy assignee_id if no multi-assignees found
	if len(logins) == 0 {
		var login sql.NullString
		_ = s.db.QueryRowContext(ctx, `
			SELECT gum.github_login
			FROM tasks t
			JOIN github_user_mappings gum ON gum.project_id = t.project_id AND gum.user_id = t.assignee_id
			WHERE t.id = $1 AND t.assignee_id IS NOT NULL AND gum.github_login IS NOT NULL
		`, taskID).Scan(&login)
		if login.Valid && login.String != "" {
			logins = append(logins, login.String)
		}
	}

	// PATCH the GitHub issue — send empty slice to unassign all
	if logins == nil {
		logins = []string{}
	}
	apiURL := fmt.Sprintf("https://api.github.com/repos/%s/%s/issues/%d", owner, repo, issueNumber)
	payload := map[string]interface{}{"assignees": logins}
	data, _ := json.Marshal(payload)
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, apiURL, bytes.NewReader(data))
	if err != nil {
		return
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("X-GitHub-Api-Version", "2022-11-28")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		s.logger.Warn("Failed to push assignees to GitHub", zap.Int64("task_id", taskID), zap.Error(err))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		s.logger.Warn("GitHub assignee push error", zap.Int64("task_id", taskID),
			zap.Int("status", resp.StatusCode), zap.String("body", strings.TrimSpace(string(b))))
	}
}

// GitHubSyncLog represents one sync log entry.
type GitHubSyncLog struct {
	ID              int64      `json:"id"`
	ProjectID       int64      `json:"project_id"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	Status          string     `json:"status"`
	TriggeredBy     string     `json:"triggered_by"`
	SyncMode        string     `json:"sync_mode"`
	CreatedTasks    int        `json:"created_tasks"`
	UpdatedTasks    int        `json:"updated_tasks"`
	CreatedComments int        `json:"created_comments"`
	SkippedTasks    int        `json:"skipped_tasks"`
	PushedComments  int        `json:"pushed_comments"`
	ErrorMessage    *string    `json:"error_message,omitempty"`
}

// HandleGetGitHubSyncLogs returns recent sync log entries for a project.
// GET /api/projects/{id}/github/sync-logs
func (s *Server) HandleGetGitHubSyncLogs(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.Atoi(chi.URLParam(r, "id"))
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "invalid_input")
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

	rows, err := s.db.QueryContext(r.Context(), `
		SELECT id, project_id, started_at, completed_at, status, triggered_by, COALESCE(sync_mode, ''),
		       created_tasks, updated_tasks, created_comments, skipped_tasks, COALESCE(pushed_comments, 0), error_message
		FROM github_sync_logs
		WHERE project_id = $1
		ORDER BY started_at DESC
		LIMIT 10
	`, projectID)
	if err != nil {
		s.logger.Error("Failed to fetch sync logs", zap.Int("project_id", projectID), zap.Error(err))
		http.Error(w, "Failed to fetch sync logs", http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	logs := []GitHubSyncLog{}
	for rows.Next() {
		var l GitHubSyncLog
		var completedAt sql.NullTime
		var errMsg sql.NullString
		if err := rows.Scan(&l.ID, &l.ProjectID, &l.StartedAt, &completedAt, &l.Status, &l.TriggeredBy, &l.SyncMode,
			&l.CreatedTasks, &l.UpdatedTasks, &l.CreatedComments, &l.SkippedTasks, &l.PushedComments, &errMsg); err != nil {
			continue
		}
		if completedAt.Valid {
			l.CompletedAt = &completedAt.Time
		}
		if errMsg.Valid && errMsg.String != "" {
			l.ErrorMessage = &errMsg.String
		}
		logs = append(logs, l)
	}
	respondJSON(w, http.StatusOK, logs)
}

// shouldSync returns true if an auto-sync is due based on interval, hour, day and last sync time.
func shouldSync(interval string, hour, day int, lastSync sql.NullTime, now time.Time) bool {
	if !lastSync.Valid {
		return true // never synced → fire immediately
	}
	scheduled := scheduledWindowStart(interval, hour, day, now)
	if scheduled.IsZero() {
		return false
	}
	return lastSync.Time.Before(scheduled)
}

// scheduledWindowStart returns the most recent past scheduled-time occurrence ≤ now (UTC).
// Returns zero time if the interval is unrecognised.
func scheduledWindowStart(interval string, hour, day int, now time.Time) time.Time {
	now = now.UTC()
	switch interval {
	case "daily":
		t := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, time.UTC)
		if now.Before(t) {
			t = t.AddDate(0, 0, -1)
		}
		return t
	case "weekly":
		wd := time.Weekday(day % 7)
		t := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, time.UTC)
		daysBack := int(now.Weekday()) - int(wd)
		if daysBack < 0 {
			daysBack += 7
		}
		t = t.AddDate(0, 0, -daysBack)
		if now.Before(t) {
			t = t.AddDate(0, 0, -7)
		}
		return t
	case "monthly":
		if day < 1 {
			day = 1
		}
		if day > 28 {
			day = 28
		}
		t := time.Date(now.Year(), now.Month(), day, hour, 0, 0, 0, time.UTC)
		if now.Before(t) {
			t = t.AddDate(0, -1, 0)
		}
		return t
	}
	return time.Time{}
}

// StartGitHubSyncWorker starts a background goroutine that runs auto-sync
// every 15 minutes for projects with github_sync_interval set.
func (s *Server) StartGitHubSyncWorker(ctx context.Context) {
	ticker := time.NewTicker(15 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.runAutoSync(ctx)
		}
	}
}

type autoSyncProject struct {
	ID           int
	Owner        string
	Repo         string
	Token        string
	ProjectURL   string
	SyncInterval string
	SyncHour     int
	SyncDay      int
	LastSync     sql.NullTime
}

func (s *Server) runAutoSync(ctx context.Context) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, COALESCE(github_owner,''), COALESCE(github_repo_name,''),
		       COALESCE(github_token,''), COALESCE(github_project_url,''),
		       github_sync_interval, COALESCE(github_sync_hour,0), COALESCE(github_sync_day,0),
		       github_last_sync
		FROM projects
		WHERE github_sync_enabled = true
		  AND github_sync_interval IS NOT NULL
		  AND github_token IS NOT NULL
	`)
	if err != nil {
		s.logger.Error("auto-sync: failed to query projects", zap.Error(err))
		return
	}
	defer rows.Close()

	var projects []autoSyncProject
	for rows.Next() {
		var p autoSyncProject
		if err := rows.Scan(&p.ID, &p.Owner, &p.Repo, &p.Token, &p.ProjectURL, &p.SyncInterval, &p.SyncHour, &p.SyncDay, &p.LastSync); err != nil {
			continue
		}
		projects = append(projects, p)
	}
	rows.Close()

	now := time.Now()
	for _, p := range projects {
		if !shouldSync(p.SyncInterval, p.SyncHour, p.SyncDay, p.LastSync, now) {
			continue
		}
		proj := p // capture
		go func() {
			syncCtx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
			defer cancel()

			req, err := s.loadSavedGitHubMappingsForAutoSync(syncCtx, int64(proj.ID))
			if err != nil {
				s.logger.Warn("auto-sync: failed to load mappings", zap.Int("project_id", proj.ID), zap.Error(err))
				return
			}
			req.PullTasks = true
			req.PullComments = true

			s.runGitHubImportCore(syncCtx, proj.ID, proj.Owner, proj.Repo, proj.Token, proj.ProjectURL, req, "auto")
		}()
	}
}

func (s *Server) loadSavedGitHubMappingsForAutoSync(ctx context.Context, projectID int64) (GitHubPullRequest, error) {
	req := GitHubPullRequest{
		StatusAssignments: map[string]int64{},
		UserAssignments:   map[string]int64{},
	}
	req.StatusAssignments, req.UserAssignments = s.loadSavedGitHubMappings(ctx, projectID, nil, nil)
	return req, nil
}

// runGitHubImportCore performs the actual GitHub import without SSE streaming.
// Used by the auto-sync worker.
func (s *Server) runGitHubImportCore(ctx context.Context, projectID int, owner, repo, token, projectURL string, req GitHubPullRequest, triggeredBy string) *GitHubPullResponse {
	result := &GitHubPullResponse{}

	base := fmt.Sprintf("https://api.github.com/repos/%s/%s", owner, repo)

	// Compute since parameter for incremental sync
	sinceParam := ""
	{
		var lastSync sql.NullTime
		_ = s.db.QueryRowContext(ctx, `SELECT github_last_sync FROM projects WHERE id=$1`, projectID).Scan(&lastSync)
		if lastSync.Valid {
			sinceParam = lastSync.Time.UTC().Format(time.RFC3339)
		}
	}

	// Record sync log
	syncMode := "auto"
	var syncLogID int64
	_ = s.db.QueryRowContext(ctx,
		s.db.Rebind(`INSERT INTO github_sync_logs (project_id, triggered_by, sync_mode) VALUES (?, ?, ?) RETURNING id`),
		projectID, triggeredBy, syncMode).Scan(&syncLogID)

	defer func() {
		if syncLogID != 0 {
			_, _ = s.db.ExecContext(context.Background(), s.db.Rebind(`
				UPDATE github_sync_logs
				SET completed_at = ?, status = 'success',
				    created_tasks = ?, updated_tasks = ?, created_comments = ?, skipped_tasks = ?, pushed_comments = ?
				WHERE id = ?
			`), time.Now(), result.CreatedTasks, result.UpdatedTasks, result.CreatedComments, result.SkippedTasks, result.PushedComments, syncLogID)
			_, _ = s.db.ExecContext(context.Background(), s.db.Rebind(`
				DELETE FROM github_sync_logs WHERE project_id = ? AND id NOT IN (
					SELECT id FROM github_sync_logs WHERE project_id = ? ORDER BY started_at DESC LIMIT 100
				)
			`), projectID, projectID)
		}
	}()

	noop := func(stage, msg string, current, total int) {}
	_ = noop

	// --- Import Tasks from Issues ---
	// allIssues is declared here so the comment section below can also use it for filtering.
	var allIssues []ghIssue
	unknownStatusKeys := map[string]struct{}{}
	if req.PullTasks {
		buildIssueURL := func(page int) string {
			return fmt.Sprintf("%s/issues?state=all&per_page=100&page=%d", base, page)
		}

		// NOTE: GitHub's /issues endpoint returns both issues and pull requests.
		// We only handle issues for now. To add PR support, remove the PullRequest filter below.
		for page := 1; page <= 10; page++ {
			var pageIssues []ghIssue
			if err := fetchGitHubJSON(ctx, token, buildIssueURL(page), &pageIssues); err != nil {
				s.logger.Error("auto-sync: failed to fetch issues", zap.Int("project_id", projectID), zap.Error(err))
				return result
			}
			if len(pageIssues) == 0 {
				break
			}
			for i := range pageIssues {
				if pageIssues[i].PullRequest == nil {
					pageIssues[i].Repo = owner + "/" + repo
					allIssues = append(allIssues, pageIssues[i])
				}
			}
		}

		swimLaneByCategory := map[string]int64{}
		slRows, _ := s.db.QueryContext(ctx, `SELECT status_category, id FROM swim_lanes WHERE project_id = $1 ORDER BY position ASC`, projectID)
		if slRows != nil {
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

		issueColumnMap := map[string]ghProjectItemStatus{}
		var projInfo *ghProjectInfo
		if projectURL != "" {
			projInfo, _ = fetchProjectByURL(ctx, token, projectURL)
		} else {
			projInfo, _ = fetchProjectStatusColumns(ctx, token, owner, repo)
		}
		if projInfo != nil {
			if m, err := fetchProjectIssueStatuses(ctx, token, projInfo.ProjectID, projInfo.FieldID, s.logger); err == nil {
				issueColumnMap = m
			}
		}

		// When a project board is configured, use its items as the primary issue source.
		// This captures issues from ALL repos on the board, not just the configured repo.
		if len(issueColumnMap) > 0 {
			allIssues = nil
			for _, item := range issueColumnMap {
				if item.Issue == nil || item.Issue.Number == 0 {
					continue
				}
				allIssues = append(allIssues, ghIssueFromProjectItem(item))
			}
			s.logger.Info("auto-sync: using project board as issue source",
				zap.Int("project_id", projectID),
				zap.Int("board_issues", len(allIssues)),
			)
		}

		statusAssignmentsLower := map[string]int64{}
		for k, v := range req.StatusAssignments {
			statusAssignmentsLower[strings.ToLower(k)] = v
		}

		// Build milestone→sprint_id maps for sprint resolution.
		// milestoneToSprintID: number-based (same-repo issues)
		// milestoneNameToSprintID: name-based fallback (cross-repo issues)
		milestoneToSprintID := map[int]int64{}
		milestoneNameToSprintID := map[string]int64{}
		{
			msRows, msErr := s.db.QueryContext(ctx, `
				SELECT github_milestone_number, name, id FROM sprints
				WHERE project_id = $1 AND github_milestone_number IS NOT NULL
			`, projectID)
			if msErr == nil {
				for msRows.Next() {
					var mnum int
					var mname string
					var sid int64
					if err := msRows.Scan(&mnum, &mname, &sid); err == nil {
						milestoneToSprintID[mnum] = sid
						milestoneNameToSprintID[mname] = sid
					}
				}
				msRows.Close()
			}
		}

		// Build iteration_title→sprint_id map from Projects V2 iteration values
		iterationToSprintID := map[string]int64{}
		{
			iterationNames := map[string]struct{}{}
			for _, item := range issueColumnMap {
				if item.IterationTitle != "" {
					iterationNames[item.IterationTitle] = struct{}{}
				}
			}
			for name := range iterationNames {
				var sid int64
				err := s.db.QueryRowContext(ctx, `SELECT id FROM sprints WHERE project_id = $1 AND name = $2`, projectID, name).Scan(&sid)
				if err == sql.ErrNoRows {
					err = s.db.QueryRowContext(ctx, `INSERT INTO sprints (project_id, name) VALUES ($1, $2) RETURNING id`, projectID, name).Scan(&sid)
				}
				if err == nil {
					iterationToSprintID[name] = sid
				}
			}
		}

		var maxNumber sql.NullInt64
		_ = s.db.QueryRowContext(ctx, `SELECT MAX(task_number) FROM tasks WHERE project_id = $1`, projectID).Scan(&maxNumber)
		nextNumber := int64(1)
		if maxNumber.Valid {
			nextNumber = maxNumber.Int64 + 1
		}

		for _, issue := range allIssues {
			if issue.PullRequest != nil {
				continue
			}
			taskStatus := "todo"
			if issue.State == "closed" {
				taskStatus = "done"
			}

			// Resolve assignees — collect all mapped user IDs from issue.Assignees.
			var assigneeID *int64
			var allAssigneeIDs []int64
			seenAssignees := map[int64]bool{}
			logins := make([]string, 0, len(issue.Assignees))
			if issue.Assignee != nil {
				logins = append(logins, issue.Assignee.Login)
			}
			for _, a := range issue.Assignees {
				if a.Login != "" && (len(logins) == 0 || logins[0] != a.Login) {
					logins = append(logins, a.Login)
				}
			}
			for _, login := range logins {
				if uid, ok := req.UserAssignments[login]; ok && uid != 0 && !seenAssignees[uid] {
					seenAssignees[uid] = true
					allAssigneeIDs = append(allAssigneeIDs, uid)
					if assigneeID == nil {
						assigneeID = &allAssigneeIDs[0]
					}
				}
			}

			// Resolve sprint: prefer Projects V2 iteration, fall back to milestone
			var sprintID *int64
			colKey := issueColumnKey(issue)
			if itemStatus, ok := issueColumnMap[colKey]; ok && itemStatus.IterationTitle != "" {
				if sid, ok := iterationToSprintID[itemStatus.IterationTitle]; ok {
					sprintID = &sid
				}
			}
			if sprintID == nil && issue.Milestone != nil {
				isPrimaryRepo := issue.Repo == owner+"/"+repo
				if isPrimaryRepo {
					if sid, ok := milestoneToSprintID[issue.Milestone.Number]; ok {
						sprintID = &sid
					}
				}
				if sprintID == nil && issue.Milestone.Title != "" {
					if sid, ok := milestoneNameToSprintID[issue.Milestone.Title]; ok {
						sprintID = &sid
					}
				}
			}

			var swimLaneID *int64
			ghItemID := ""
			ghStartDate := ""
			ghDueDate := ""

			if itemStatus, ok := issueColumnMap[colKey]; ok {
				ghItemID = itemStatus.ItemID
				ghStartDate = itemStatus.StartDate
				ghDueDate = itemStatus.DueDate
				if itemStatus.StatusName != "" {
					if laneID, ok := req.StatusAssignments[itemStatus.StatusName]; ok && laneID > 0 {
						swimLaneID = &laneID
					} else if laneID, ok := statusAssignmentsLower[strings.ToLower(itemStatus.StatusName)]; ok && laneID > 0 {
						swimLaneID = &laneID
					} else {
						// Status key not yet mapped â register it so it surfaces in the UI.
						unknownStatusKeys[itemStatus.StatusName] = struct{}{}
					}
				}
			}
			if swimLaneID == nil {
				stateKey := issueStatusKey(issue.State, issue.StateReason)
				if laneID, ok := req.StatusAssignments[stateKey]; ok && laneID > 0 {
					swimLaneID = &laneID
				}
			}
			if swimLaneID == nil {
				if slID, ok := swimLaneByCategory[taskStatus]; ok {
					swimLaneID = &slID
				}
			}

			var existingID int64
			err := s.db.QueryRowContext(ctx, `SELECT id FROM tasks WHERE project_id = $1 AND github_repo = $2 AND github_issue_number = $3`, projectID, issue.Repo, issue.Number).Scan(&existingID)
			if err == sql.ErrNoRows {
				err = s.db.QueryRowContext(ctx, `
					INSERT INTO tasks (project_id, task_number, title, description, status, priority, assignee_id, sprint_id, github_issue_number, github_repo, swim_lane_id, github_project_item_id, start_date, due_date)
					VALUES ($1, $2, $3, $4, $5, 'medium', $6, $7, $8, $9, $10, $11, $12, $13)
					ON CONFLICT (project_id, github_repo, github_issue_number) WHERE github_issue_number IS NOT NULL DO NOTHING
					RETURNING id
				`, projectID, nextNumber, issue.Title, issue.Body, taskStatus, assigneeID, sprintID, issue.Number, issue.Repo, swimLaneID, nullableStr(ghItemID), nullableStr(ghStartDate), nullableStr(ghDueDate)).Scan(&existingID)
				if err == nil {
					nextNumber++
					result.CreatedTasks++
					s.upsertReactions(ctx, existingID, 0, issue.Reactions)
					s.syncGitHubTaskAssignees(ctx, existingID, allAssigneeIDs)
				} else {
					result.SkippedTasks++
				}
			} else if err == nil {
				_, _ = s.db.ExecContext(ctx, `
					UPDATE tasks SET title = $1, description = $2, status = $3, assignee_id = $4, sprint_id = $5, swim_lane_id = $6,
					github_project_item_id = COALESCE(NULLIF($7,''), github_project_item_id),
					start_date = COALESCE($8, start_date), due_date = COALESCE($9, due_date)
					WHERE id = $10
				`, issue.Title, issue.Body, taskStatus, assigneeID, sprintID, swimLaneID, ghItemID, nullableStr(ghStartDate), nullableStr(ghDueDate), existingID)
				s.upsertReactions(ctx, existingID, 0, issue.Reactions)
				s.syncGitHubTaskAssignees(ctx, existingID, allAssigneeIDs)
				result.UpdatedTasks++
			}
		}
	}

	// --- Import Comments ---
	if req.PullComments {
		// Build updated-issue set from already-fetched allIssues for efficient filtering.
		updatedIssues := map[string]bool{}
		for _, iss := range allIssues {
			if sinceParam == "" || iss.UpdatedAt >= sinceParam {
				updatedIssues[issueColumnKey(iss)] = true
			}
		}

		rows, err := s.db.QueryContext(ctx, `SELECT id, github_issue_number, github_repo FROM tasks WHERE project_id = $1 AND github_issue_number IS NOT NULL`, projectID)
		if err == nil {
			type taskRef struct {
				taskID   int64
				issueNum int
				repo     string
			}
			var taskRefs []taskRef
			for rows.Next() {
				var tr taskRef
				if err := rows.Scan(&tr.taskID, &tr.issueNum, &tr.repo); err == nil {
					taskRefs = append(taskRefs, tr)
				}
			}
			rows.Close()

			var ownerID int64
			_ = s.db.QueryRowContext(ctx, s.db.Rebind(`SELECT user_id FROM project_members WHERE project_id = ? AND role = 'owner' LIMIT 1`), projectID).Scan(&ownerID)

			// Build set of tasks that already have GitHub comments imported.
			tasksWithComments := map[int64]bool{}
			crows2, cerr2 := s.db.QueryContext(ctx, s.db.Rebind(`
				SELECT DISTINCT task_id FROM task_comments
				WHERE task_id IN (SELECT id FROM tasks WHERE project_id = ? AND github_issue_number IS NOT NULL)
				AND github_comment_id IS NOT NULL
			`), projectID)
			if cerr2 == nil {
				for crows2.Next() {
					var tid int64
					if crows2.Scan(&tid) == nil {
						tasksWithComments[tid] = true
					}
				}
				crows2.Close()
			}

			for _, tr := range taskRefs {
				// Skip issues not updated since last sync only if the task already has comments.
				trKey := fmt.Sprintf("%s#%d", tr.repo, tr.issueNum)
				if sinceParam != "" && !updatedIssues[trKey] && tasksWithComments[tr.taskID] {
					continue
				}
				// Use the task's github_repo for cross-repo comment fetch
				commentRepo := tr.repo
				if commentRepo == "" {
					commentRepo = owner + "/" + repo
				}
				commentsURL := fmt.Sprintf("https://api.github.com/repos/%s/issues/%d/comments?per_page=100", commentRepo, tr.issueNum)
				var ghComments []ghIssueComment
				if err := fetchGitHubJSON(ctx, token, commentsURL, &ghComments); err != nil {
					continue
				}
				for _, gc := range ghComments {
					if gc.Body == "" {
						continue
					}
					commentUserID, body := s.resolveGitHubCommentAuthor(ctx, gc, ownerID)
					var newCID int64
					err := s.db.QueryRowContext(ctx, `
						INSERT INTO task_comments (task_id, user_id, comment, github_comment_id)
						VALUES ($1, $2, $3, $4)
						ON CONFLICT (github_comment_id) WHERE github_comment_id IS NOT NULL DO NOTHING
						RETURNING id
					`, tr.taskID, commentUserID, body, gc.ID).Scan(&newCID)
					if err == nil {
						result.CreatedComments++
					} else if err != sql.ErrNoRows {
						s.logger.Warn("Failed to insert GitHub comment",
							zap.Int64("task_id", tr.taskID),
							zap.Int64("github_comment_id", gc.ID),
							zap.Error(err))
					}
					commentDBID := newCID
					if commentDBID == 0 && gc.ID != 0 {
						_ = s.db.QueryRowContext(ctx, `SELECT id FROM task_comments WHERE github_comment_id = $1`, gc.ID).Scan(&commentDBID)
					}
					s.upsertReactions(ctx, 0, commentDBID, gc.Reactions)
				}
			}
		}
	}

	// --- Push Unpushed TaskAI Comments to GitHub ---
	s.pushUnpushedComments(ctx, projectID, owner, repo, token, result)

	// Update last sync timestamp
	_, _ = s.db.ExecContext(ctx, `UPDATE projects SET github_last_sync = $1 WHERE id = $2`, time.Now(), projectID)
	s.registerUnknownStatusKeys(ctx, int64(projectID), unknownStatusKeys)
	s.saveGitHubMappings(ctx, int64(projectID), req.StatusAssignments, req.UserAssignments)

	return result
}
