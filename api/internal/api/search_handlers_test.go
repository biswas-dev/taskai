package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// Pure function tests: resolveSearchTypes
// ---------------------------------------------------------------------------

func TestResolveSearchTypes(t *testing.T) {
	tests := []struct {
		name      string
		types     []string
		wantTasks bool
		wantWiki  bool
	}{
		{
			name:      "empty types searches everything",
			types:     []string{},
			wantTasks: true,
			wantWiki:  true,
		},
		{
			name:      "nil types searches everything",
			types:     nil,
			wantTasks: true,
			wantWiki:  true,
		},
		{
			name:      "tasks only",
			types:     []string{"tasks"},
			wantTasks: true,
			wantWiki:  false,
		},
		{
			name:      "wiki only",
			types:     []string{"wiki"},
			wantTasks: false,
			wantWiki:  true,
		},
		{
			name:      "both tasks and wiki",
			types:     []string{"tasks", "wiki"},
			wantTasks: true,
			wantWiki:  true,
		},
		{
			name:      "unknown type ignored",
			types:     []string{"unknown"},
			wantTasks: false,
			wantWiki:  false,
		},
		{
			name:      "mixed known and unknown types",
			types:     []string{"tasks", "unknown", "wiki"},
			wantTasks: true,
			wantWiki:  true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotTasks, gotWiki := resolveSearchTypes(tt.types)
			if gotTasks != tt.wantTasks {
				t.Errorf("resolveSearchTypes(%v) tasks = %v, want %v", tt.types, gotTasks, tt.wantTasks)
			}
			if gotWiki != tt.wantWiki {
				t.Errorf("resolveSearchTypes(%v) wiki = %v, want %v", tt.types, gotWiki, tt.wantWiki)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Pure function tests: normalizeSearchLimit
// ---------------------------------------------------------------------------

func TestNormalizeSearchLimit(t *testing.T) {
	tests := []struct {
		name  string
		limit int
		want  int
	}{
		{name: "zero defaults to 10", limit: 0, want: 10},
		{name: "negative defaults to 10", limit: -5, want: 10},
		{name: "valid limit preserved", limit: 25, want: 25},
		{name: "limit of 1", limit: 1, want: 1},
		{name: "limit of 50", limit: 50, want: 50},
		{name: "over 50 clamped to 50", limit: 51, want: 50},
		{name: "large value clamped to 50", limit: 1000, want: 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := normalizeSearchLimit(tt.limit)
			if got != tt.want {
				t.Errorf("normalizeSearchLimit(%d) = %d, want %d", tt.limit, got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: create a wiki block directly in the database via ent
// ---------------------------------------------------------------------------

func (ts *TestServer) createTestWikiBlock(t testing.TB, pageID int64, blockType, plainText, headingsPath string, position int) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	builder := ts.DB.Client.WikiBlock.Create().
		SetPageID(pageID).
		SetBlockType(blockType).
		SetPlainText(plainText).
		SetPosition(position)

	if headingsPath != "" {
		builder = builder.SetHeadingsPath(headingsPath)
	}

	block, err := builder.Save(ctx)
	if err != nil {
		t.Fatalf("Failed to create test wiki block: %v", err)
	}

	return block.ID
}

// ---------------------------------------------------------------------------
// Helper: create a task with a description
// ---------------------------------------------------------------------------

func (ts *TestServer) createTestTaskWithDescription(t testing.TB, projectID int64, title, description string) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var nextNumber int64
	err := ts.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(task_number), 0) + 1 FROM tasks WHERE project_id = ?`, projectID).Scan(&nextNumber)
	if err != nil {
		t.Fatalf("Failed to get next task number: %v", err)
	}

	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO tasks (project_id, task_number, title, description, status, priority) VALUES (?, ?, ?, ?, 'todo', 'medium')`,
		projectID, nextNumber, title, description,
	)
	if err != nil {
		t.Fatalf("Failed to create test task with description: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get task ID: %v", err)
	}

	return id
}

// ---------------------------------------------------------------------------
// HandleGlobalSearch
// ---------------------------------------------------------------------------

func TestHandleGlobalSearch(t *testing.T) {
	t.Run("requires authentication", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		// Make request without auth context
		rec, req := MakeRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "test",
		}, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusUnauthorized)
	})

	t.Run("requires query parameter", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
	})

	t.Run("returns empty results when no accessible projects", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "test",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) != 0 {
			t.Errorf("expected 0 tasks, got %d", len(resp.Tasks))
		}
		if len(resp.Wiki) != 0 {
			t.Errorf("expected 0 wiki results, got %d", len(resp.Wiki))
		}
	})

	t.Run("finds tasks by title", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		ts.CreateTestTask(t, projectID, "Fix login bug")
		ts.CreateTestTask(t, projectID, "Add dashboard feature")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "login",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(resp.Tasks))
		}
		if resp.Tasks[0].Title != "Fix login bug" {
			t.Errorf("expected title 'Fix login bug', got '%s'", resp.Tasks[0].Title)
		}
		if resp.Tasks[0].ProjectName != "Test Project" {
			t.Errorf("expected project name 'Test Project', got '%s'", resp.Tasks[0].ProjectName)
		}
		if resp.Tasks[0].TaskNumber != 1 {
			t.Errorf("expected task number 1, got %d", resp.Tasks[0].TaskNumber)
		}
	})

	t.Run("case insensitive search", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		ts.CreateTestTask(t, projectID, "Fix Login Bug")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "fix login",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(resp.Tasks))
		}
	})

	t.Run("finds tasks by description", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		// Create task with description
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := ts.DB.ExecContext(ctx,
			`INSERT INTO tasks (project_id, task_number, title, description, status, priority) VALUES (?, 1, 'Some task', 'This involves authentication flow', 'todo', 'medium')`,
			projectID,
		)
		if err != nil {
			t.Fatalf("Failed to create test task: %v", err)
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "authentication",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(resp.Tasks))
		}
		if resp.Tasks[0].Snippet != "This involves authentication flow" {
			t.Errorf("unexpected snippet: %s", resp.Tasks[0].Snippet)
		}
	})

	t.Run("respects project_id filter", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		project1 := ts.CreateTestProject(t, userID, "Project One")
		project2 := ts.CreateTestProject(t, userID, "Project Two")
		ts.CreateTestTask(t, project1, "Shared keyword task")
		ts.CreateTestTask(t, project2, "Shared keyword task")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]interface{}{
			"query":      "keyword",
			"project_id": project1,
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(resp.Tasks))
		}
		if resp.Tasks[0].ProjectID != project1 {
			t.Errorf("expected project ID %d, got %d", project1, resp.Tasks[0].ProjectID)
		}
	})

	t.Run("respects types filter", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		ts.CreateTestTask(t, projectID, "Some task with searchterm")

		// Search only wiki (should return no tasks)
		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]interface{}{
			"query": "searchterm",
			"types": []string{"wiki"},
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) != 0 {
			t.Errorf("expected 0 tasks when filtered to wiki only, got %d", len(resp.Tasks))
		}
	})

	t.Run("respects limit parameter", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		// Create 5 tasks
		for i := 0; i < 5; i++ {
			ts.CreateTestTask(t, projectID, "Findable task")
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]interface{}{
			"query": "findable",
			"limit": 2,
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) != 2 {
			t.Errorf("expected 2 tasks with limit=2, got %d", len(resp.Tasks))
		}
	})

	t.Run("does not return tasks from inaccessible projects", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		user1 := ts.CreateTestUser(t, "user1@example.com", "password123")
		user2 := ts.CreateTestUser(t, "user2@example.com", "password123")

		project1 := ts.CreateTestProject(t, user1, "User1 Project")
		project2 := ts.CreateTestProject(t, user2, "User2 Project")

		ts.CreateTestTask(t, project1, "Secret task one")
		ts.CreateTestTask(t, project2, "Secret task two")

		// User2 should not see user1's tasks
		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "secret",
		}, user2, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(resp.Tasks))
		}
		if resp.Tasks[0].ProjectID != project2 {
			t.Errorf("expected task from project %d, got project %d", project2, resp.Tasks[0].ProjectID)
		}
	})

	t.Run("clamps limit to max 50", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		ts.CreateTestTask(t, projectID, "Test task")

		body := map[string]interface{}{
			"query": "test",
			"limit": 100,
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", body, userID, nil)
		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		// Just verify it doesn't error -- the limit is clamped internally
		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)
	})

	t.Run("returns proper task fields", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "My Project")
		ts.CreateTestTask(t, projectID, "Important task")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "important",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		// Decode raw JSON to verify field names
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		// Verify both keys exist
		if _, ok := raw["tasks"]; !ok {
			t.Error("response missing 'tasks' key")
		}
		if _, ok := raw["wiki"]; !ok {
			t.Error("response missing 'wiki' key")
		}

		var resp GlobalSearchResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		if len(resp.Tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(resp.Tasks))
		}

		task := resp.Tasks[0]
		if task.ID == 0 {
			t.Error("task ID should not be 0")
		}
		if task.ProjectID != projectID {
			t.Errorf("expected project_id %d, got %d", projectID, task.ProjectID)
		}
		if task.ProjectName != "My Project" {
			t.Errorf("expected project_name 'My Project', got '%s'", task.ProjectName)
		}
		if task.Status != "todo" {
			t.Errorf("expected status 'todo', got '%s'", task.Status)
		}
		if task.Priority != "medium" {
			t.Errorf("expected priority 'medium', got '%s'", task.Priority)
		}
	})

	t.Run("invalid request body", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", "not json", userID, nil)
		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
	})

	t.Run("searches tasks only when types filter is tasks", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		ts.CreateTestTask(t, projectID, "Findable alpha task")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]interface{}{
			"query": "alpha",
			"types": []string{"tasks"},
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) != 1 {
			t.Errorf("expected 1 task, got %d", len(resp.Tasks))
		}
		// Wiki should be empty (but still present as empty array)
		if len(resp.Wiki) != 0 {
			t.Errorf("expected 0 wiki results when types=tasks, got %d", len(resp.Wiki))
		}
	})

	t.Run("description snippet truncated to 200 chars", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		longDesc := strings.Repeat("a", 300) + " uniquetruncateme"
		ts.createTestTaskWithDescription(t, projectID, "Task with long desc", longDesc)

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "long desc",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(resp.Tasks))
		}

		snippet := resp.Tasks[0].Snippet
		if len(snippet) > 203 { // 200 chars + "..."
			t.Errorf("snippet should be truncated, got length %d", len(snippet))
		}
		if !strings.HasSuffix(snippet, "...") {
			t.Errorf("truncated snippet should end with '...', got %q", snippet[len(snippet)-10:])
		}
	})

	t.Run("finds wiki blocks by plain text in global search", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Wiki Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Architecture Docs")
		ts.createTestWikiBlock(t, pageID, "paragraph", "This describes the microservices architecture", "Architecture", 0)

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "microservices",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Wiki) != 1 {
			t.Fatalf("expected 1 wiki result, got %d", len(resp.Wiki))
		}
		if resp.Wiki[0].PageTitle != "Architecture Docs" {
			t.Errorf("expected page title 'Architecture Docs', got %q", resp.Wiki[0].PageTitle)
		}
		if resp.Wiki[0].ProjectName != "Wiki Project" {
			t.Errorf("expected project name 'Wiki Project', got %q", resp.Wiki[0].ProjectName)
		}
		if resp.Wiki[0].Snippet != "This describes the microservices architecture" {
			t.Errorf("unexpected snippet: %q", resp.Wiki[0].Snippet)
		}
	})

	t.Run("finds wiki blocks by headings path in global search", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Guide")
		ts.createTestWikiBlock(t, pageID, "heading", "some content", "Getting Started > Installation > UniqueHeadingXyz", 0)

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "UniqueHeadingXyz",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Wiki) != 1 {
			t.Fatalf("expected 1 wiki result, got %d", len(resp.Wiki))
		}
		if resp.Wiki[0].HeadingsPath != "Getting Started > Installation > UniqueHeadingXyz" {
			t.Errorf("unexpected headings path: %q", resp.Wiki[0].HeadingsPath)
		}
	})

	t.Run("wiki results include page slug and project ID", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "API Reference")
		ts.createTestWikiBlock(t, pageID, "paragraph", "unique searchable endpoint content", "", 0)

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "unique searchable endpoint",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Wiki) != 1 {
			t.Fatalf("expected 1 wiki result, got %d", len(resp.Wiki))
		}
		if resp.Wiki[0].PageSlug != "api-reference" {
			t.Errorf("expected page slug 'api-reference', got %q", resp.Wiki[0].PageSlug)
		}
		if resp.Wiki[0].ProjectID != projectID {
			t.Errorf("expected project ID %d, got %d", projectID, resp.Wiki[0].ProjectID)
		}
		if resp.Wiki[0].PageID == 0 {
			t.Error("page ID should not be 0")
		}
	})

	t.Run("wiki snippet truncated to 200 chars in global search", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Long Content Page")
		longText := strings.Repeat("x", 250) + " wikisniptruncate"
		ts.createTestWikiBlock(t, pageID, "paragraph", longText, "", 0)

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "xxxxx",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Wiki) != 1 {
			t.Fatalf("expected 1 wiki result, got %d", len(resp.Wiki))
		}
		if len(resp.Wiki[0].Snippet) > 203 {
			t.Errorf("wiki snippet should be truncated, got length %d", len(resp.Wiki[0].Snippet))
		}
		if !strings.HasSuffix(resp.Wiki[0].Snippet, "...") {
			t.Errorf("truncated wiki snippet should end with '...', got %q", resp.Wiki[0].Snippet)
		}
	})

	t.Run("does not return wiki from inaccessible projects", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		user1 := ts.CreateTestUser(t, "user1@example.com", "password123")
		user2 := ts.CreateTestUser(t, "user2@example.com", "password123")

		project1 := ts.CreateTestProject(t, user1, "User1 Project")
		project2 := ts.CreateTestProject(t, user2, "User2 Project")

		page1 := ts.createTestWikiPage(t, project1, user1, "Secret Wiki One")
		page2 := ts.createTestWikiPage(t, project2, user2, "Secret Wiki Two")

		ts.createTestWikiBlock(t, page1, "paragraph", "crossproject secret wiki alpha", "", 0)
		ts.createTestWikiBlock(t, page2, "paragraph", "crossproject secret wiki alpha", "", 0)

		// User2 should only see their own wiki
		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "crossproject secret",
		}, user2, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Wiki) != 1 {
			t.Fatalf("expected 1 wiki result, got %d", len(resp.Wiki))
		}
		if resp.Wiki[0].ProjectID != project2 {
			t.Errorf("expected wiki from project %d, got project %d", project2, resp.Wiki[0].ProjectID)
		}
	})

	t.Run("wiki project_id filter limits wiki results", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		project1 := ts.CreateTestProject(t, userID, "Project A")
		project2 := ts.CreateTestProject(t, userID, "Project B")

		page1 := ts.createTestWikiPage(t, project1, userID, "Page A")
		page2 := ts.createTestWikiPage(t, project2, userID, "Page B")

		ts.createTestWikiBlock(t, page1, "paragraph", "filterable wiki content zxy", "", 0)
		ts.createTestWikiBlock(t, page2, "paragraph", "filterable wiki content zxy", "", 0)

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]interface{}{
			"query":      "filterable wiki",
			"project_id": project1,
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Wiki) != 1 {
			t.Fatalf("expected 1 wiki result with project filter, got %d", len(resp.Wiki))
		}
		if resp.Wiki[0].ProjectID != project1 {
			t.Errorf("expected wiki from project %d, got %d", project1, resp.Wiki[0].ProjectID)
		}
	})

	t.Run("combined tasks and wiki results", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Combined Project")

		ts.CreateTestTask(t, projectID, "Deploy the combinedunique feature")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Deployment Guide")
		ts.createTestWikiBlock(t, pageID, "paragraph", "Steps to deploy the combinedunique feature", "", 0)

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "combinedunique",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) < 1 {
			t.Errorf("expected at least 1 task, got %d", len(resp.Tasks))
		}
		if len(resp.Wiki) < 1 {
			t.Errorf("expected at least 1 wiki result, got %d", len(resp.Wiki))
		}
	})

	t.Run("default limit is 10", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		// Create 15 tasks matching the query
		for i := 0; i < 15; i++ {
			ts.CreateTestTask(t, projectID, fmt.Sprintf("Defaultlimit task %d", i))
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "defaultlimit",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) != 10 {
			t.Errorf("expected 10 tasks with default limit, got %d", len(resp.Tasks))
		}
	})

	t.Run("response always contains tasks and wiki arrays even when empty", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		_ = ts.CreateTestProject(t, userID, "Empty Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "nonexistent",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		// Parse raw JSON to verify arrays (not null)
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		tasksJSON := string(raw["tasks"])
		wikiJSON := string(raw["wiki"])

		if tasksJSON == "null" {
			t.Error("tasks should be empty array, not null")
		}
		if wikiJSON == "null" {
			t.Error("wiki should be empty array, not null")
		}
		if tasksJSON != "[]" {
			t.Errorf("expected tasks to be [], got %s", tasksJSON)
		}
		if wikiJSON != "[]" {
			t.Errorf("expected wiki to be [], got %s", wikiJSON)
		}
	})

	t.Run("task with nil description has empty snippet", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		// CreateTestTask creates without description
		ts.CreateTestTask(t, projectID, "Nodesc unique searchable task")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "nodesc unique",
		}, userID, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) != 1 {
			t.Fatalf("expected 1 task, got %d", len(resp.Tasks))
		}
		if resp.Tasks[0].Snippet != "" {
			t.Errorf("expected empty snippet for task without description, got %q", resp.Tasks[0].Snippet)
		}
	})

	t.Run("member of shared project can search across it", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		member := ts.CreateTestUser(t, "member@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Shared Project")
		ts.AddProjectMember(t, projectID, member, owner, "member")

		ts.CreateTestTask(t, projectID, "Shared crossaccess task alpha")

		// Member should see the task
		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/search", map[string]string{
			"query": "crossaccess",
		}, member, nil)

		ts.HandleGlobalSearch(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp GlobalSearchResponse
		DecodeJSON(t, rec, &resp)

		if len(resp.Tasks) != 1 {
			t.Fatalf("expected 1 task for shared project member, got %d", len(resp.Tasks))
		}
		if resp.Tasks[0].ProjectName != "Shared Project" {
			t.Errorf("expected project name 'Shared Project', got %q", resp.Tasks[0].ProjectName)
		}
	})
}

// ---------------------------------------------------------------------------
// mapTaskResults pure function test
// ---------------------------------------------------------------------------

func TestMapTaskResults(t *testing.T) {
	t.Run("empty input returns empty slice", func(t *testing.T) {
		results := mapTaskResults(nil, map[int64]string{})
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})
}

// ---------------------------------------------------------------------------
// mapWikiResults pure function test
// ---------------------------------------------------------------------------

func TestMapWikiResults(t *testing.T) {
	t.Run("empty input returns empty slice", func(t *testing.T) {
		results := mapWikiResults(nil, map[int64]string{})
		if len(results) != 0 {
			t.Errorf("expected 0 results, got %d", len(results))
		}
	})
}
