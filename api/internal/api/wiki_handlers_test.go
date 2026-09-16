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
// Pure function tests
// ---------------------------------------------------------------------------

func TestGenerateSlug(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  string
	}{
		{
			name:  "simple lowercase",
			title: "hello world",
			want:  "hello-world",
		},
		{
			name:  "mixed case",
			title: "Hello World",
			want:  "hello-world",
		},
		{
			name:  "special characters",
			title: "Hello! @World# $2024",
			want:  "hello-world-2024",
		},
		{
			name:  "leading and trailing spaces",
			title: "  hello world  ",
			want:  "hello-world",
		},
		{
			name:  "multiple consecutive spaces",
			title: "hello   world",
			want:  "hello-world",
		},
		{
			name:  "numbers preserved",
			title: "release v2.0.1",
			want:  "release-v2-0-1",
		},
		{
			name:  "unicode stripped",
			title: "caf\u00e9 menu",
			want:  "caf-menu",
		},
		{
			name:  "empty string",
			title: "",
			want:  "",
		},
		{
			name:  "only special characters",
			title: "!@#$%",
			want:  "",
		},
		{
			name:  "truncated to 100 chars",
			title: strings.Repeat("abcde ", 30),
			want:  strings.Repeat("abcde-", 16) + "abcd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := generateSlug(tt.title)
			if got != tt.want {
				t.Errorf("generateSlug(%q) = %q, want %q", tt.title, got, tt.want)
			}
		})
	}
}

func TestStripDrawEditMode(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "removes /edit from data-src",
			html: `<div class="godraw-embed" data-src="/draw/abc123/edit"></div>`,
			want: `<div class="godraw-embed" data-src="/draw/abc123"></div>`,
		},
		{
			name: "preserves non-edit data-src",
			html: `<div class="godraw-embed" data-src="/draw/abc123"></div>`,
			want: `<div class="godraw-embed" data-src="/draw/abc123"></div>`,
		},
		{
			name: "handles multiple embeds",
			html: `<div data-src="/draw/a/edit"></div><div data-src="/draw/b/edit"></div>`,
			want: `<div data-src="/draw/a"></div><div data-src="/draw/b"></div>`,
		},
		{
			name: "empty string",
			html: "",
			want: "",
		},
		{
			name: "no godraw content",
			html: `<p>Hello world</p>`,
			want: `<p>Hello world</p>`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := stripDrawEditMode(tt.html)
			if got != tt.want {
				t.Errorf("stripDrawEditMode() = %q, want %q", got, tt.want)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// Helper: create a wiki page directly in the database via ent
// ---------------------------------------------------------------------------

func (ts *TestServer) createTestWikiPage(t testing.TB, projectID, userID int64, title string) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	page, err := ts.DB.Client.WikiPage.Create().
		SetProjectID(projectID).
		SetTitle(title).
		SetSlug(generateSlug(title)).
		SetCreatedBy(userID).
		Save(ctx)
	if err != nil {
		t.Fatalf("Failed to create test wiki page: %v", err)
	}

	return page.ID
}

func (ts *TestServer) createTestWikiPageWithContent(t testing.TB, projectID, userID int64, title, content string) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	page, err := ts.DB.Client.WikiPage.Create().
		SetProjectID(projectID).
		SetTitle(title).
		SetSlug(generateSlug(title)).
		SetCreatedBy(userID).
		SetContent(content).
		Save(ctx)
	if err != nil {
		t.Fatalf("Failed to create test wiki page with content: %v", err)
	}

	return page.ID
}

// ---------------------------------------------------------------------------
// HandleListWikiPages
// ---------------------------------------------------------------------------

func TestHandleListWikiPages(t *testing.T) {
	t.Run("returns empty list when no pages", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d/wiki", projectID), nil, userID,
			map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

		ts.HandleListWikiPages(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var pages []WikiPageResponse
		DecodeJSON(t, rec, &pages)

		if len(pages) != 0 {
			t.Errorf("Expected 0 pages, got %d", len(pages))
		}
	})

	t.Run("returns all pages for project sorted by title", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		ts.createTestWikiPage(t, projectID, userID, "Zebra Guide")
		ts.createTestWikiPage(t, projectID, userID, "Alpha Docs")
		ts.createTestWikiPage(t, projectID, userID, "Middle Page")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d/wiki", projectID), nil, userID,
			map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

		ts.HandleListWikiPages(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var pages []WikiPageResponse
		DecodeJSON(t, rec, &pages)

		if len(pages) != 3 {
			t.Fatalf("Expected 3 pages, got %d", len(pages))
		}

		// Should be sorted by title ascending
		if pages[0].Title != "Alpha Docs" {
			t.Errorf("Expected first page 'Alpha Docs', got %q", pages[0].Title)
		}
		if pages[1].Title != "Middle Page" {
			t.Errorf("Expected second page 'Middle Page', got %q", pages[1].Title)
		}
		if pages[2].Title != "Zebra Guide" {
			t.Errorf("Expected third page 'Zebra Guide', got %q", pages[2].Title)
		}
	})

	t.Run("does not return pages from other projects", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		project1 := ts.CreateTestProject(t, userID, "Project 1")
		project2 := ts.CreateTestProject(t, userID, "Project 2")

		ts.createTestWikiPage(t, project1, userID, "Page in Project 1")
		ts.createTestWikiPage(t, project2, userID, "Page in Project 2")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d/wiki", project1), nil, userID,
			map[string]string{"projectId": fmt.Sprintf("%d", project1)})

		ts.HandleListWikiPages(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var pages []WikiPageResponse
		DecodeJSON(t, rec, &pages)

		if len(pages) != 1 {
			t.Fatalf("Expected 1 page, got %d", len(pages))
		}
		if pages[0].Title != "Page in Project 1" {
			t.Errorf("Expected 'Page in Project 1', got %q", pages[0].Title)
		}
	})

	t.Run("invalid project ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			"/api/projects/abc/wiki", nil, userID,
			map[string]string{"projectId": "abc"})

		ts.HandleListWikiPages(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid project ID", "invalid_input")
	})

	t.Run("non-member gets 403", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		other := ts.CreateTestUser(t, "other@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Owner Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d/wiki", projectID), nil, other,
			map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

		ts.HandleListWikiPages(rec, req)

		AssertError(t, rec, http.StatusForbidden, "access denied", "forbidden")
	})
}

// ---------------------------------------------------------------------------
// HandleCreateWikiPage
// ---------------------------------------------------------------------------

func TestHandleCreateWikiPage(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		body := CreateWikiPageRequest{Title: "Getting Started"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/wiki", projectID), body, userID,
			map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

		ts.HandleCreateWikiPage(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var page WikiPageResponse
		DecodeJSON(t, rec, &page)

		if page.Title != "Getting Started" {
			t.Errorf("Expected title 'Getting Started', got %q", page.Title)
		}
		if page.Slug != "getting-started" {
			t.Errorf("Expected slug 'getting-started', got %q", page.Slug)
		}
		if page.ProjectID != projectID {
			t.Errorf("Expected project_id %d, got %d", projectID, page.ProjectID)
		}
		if page.CreatedBy != userID {
			t.Errorf("Expected created_by %d, got %d", userID, page.CreatedBy)
		}
	})

	t.Run("duplicate title generates unique slug", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		// Create first page
		ts.createTestWikiPage(t, projectID, userID, "Getting Started")

		// Create second page with same title
		body := CreateWikiPageRequest{Title: "Getting Started"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/wiki", projectID), body, userID,
			map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

		ts.HandleCreateWikiPage(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var page WikiPageResponse
		DecodeJSON(t, rec, &page)

		if page.Slug != "getting-started-1" {
			t.Errorf("Expected slug 'getting-started-1', got %q", page.Slug)
		}
	})

	t.Run("missing title returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		body := CreateWikiPageRequest{Title: ""}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/wiki", projectID), body, userID,
			map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

		ts.HandleCreateWikiPage(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "title is required", "invalid_input")
	})

	t.Run("title too long returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		longTitle := strings.Repeat("a", 501)
		body := CreateWikiPageRequest{Title: longTitle}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/wiki", projectID), body, userID,
			map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

		ts.HandleCreateWikiPage(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "title is too long", "invalid_input")
	})

	t.Run("invalid project ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		body := CreateWikiPageRequest{Title: "Test"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			"/api/projects/abc/wiki", body, userID,
			map[string]string{"projectId": "abc"})

		ts.HandleCreateWikiPage(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid project ID", "invalid_input")
	})

	t.Run("invalid JSON body returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/wiki", projectID), "not-json", userID,
			map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

		ts.HandleCreateWikiPage(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid request body", "invalid_input")
	})

	t.Run("non-member gets 403", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		other := ts.CreateTestUser(t, "other@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Owner Project")

		body := CreateWikiPageRequest{Title: "Unauthorized Page"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/wiki", projectID), body, other,
			map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

		ts.HandleCreateWikiPage(rec, req)

		AssertError(t, rec, http.StatusForbidden, "access denied", "forbidden")
	})
}

// ---------------------------------------------------------------------------
// HandleGetWikiPage
// ---------------------------------------------------------------------------

func TestHandleGetWikiPage(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Architecture Overview")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/wiki/pages/%d", pageID), nil, userID,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleGetWikiPage(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var page WikiPageResponse
		DecodeJSON(t, rec, &page)

		if page.ID != pageID {
			t.Errorf("Expected page ID %d, got %d", pageID, page.ID)
		}
		if page.Title != "Architecture Overview" {
			t.Errorf("Expected title 'Architecture Overview', got %q", page.Title)
		}
		if page.Slug != "architecture-overview" {
			t.Errorf("Expected slug 'architecture-overview', got %q", page.Slug)
		}
		if page.ProjectID != projectID {
			t.Errorf("Expected project_id %d, got %d", projectID, page.ProjectID)
		}
	})

	t.Run("not found returns 404", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			"/api/wiki/pages/99999", nil, userID,
			map[string]string{"pageId": "99999"})

		ts.HandleGetWikiPage(rec, req)

		AssertError(t, rec, http.StatusNotFound, "wiki page not found", "not_found")
	})

	t.Run("invalid page ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			"/api/wiki/pages/abc", nil, userID,
			map[string]string{"pageId": "abc"})

		ts.HandleGetWikiPage(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid page ID", "invalid_input")
	})

	t.Run("non-member gets 403", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		other := ts.CreateTestUser(t, "other@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Owner Project")
		pageID := ts.createTestWikiPage(t, projectID, owner, "Secret Docs")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/wiki/pages/%d", pageID), nil, other,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleGetWikiPage(rec, req)

		AssertError(t, rec, http.StatusForbidden, "access denied", "forbidden")
	})
}

// ---------------------------------------------------------------------------
// HandleUpdateWikiPage
// ---------------------------------------------------------------------------

func TestHandleUpdateWikiPage(t *testing.T) {
	t.Run("update title", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Old Title")

		body := UpdateWikiPageRequest{Title: stringPtr("New Title")}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/wiki/pages/%d", pageID), body, userID,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleUpdateWikiPage(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var page WikiPageResponse
		DecodeJSON(t, rec, &page)

		if page.Title != "New Title" {
			t.Errorf("Expected title 'New Title', got %q", page.Title)
		}
		if page.Slug != "new-title" {
			t.Errorf("Expected slug 'new-title', got %q", page.Slug)
		}
	})

	t.Run("slug unchanged when title unchanged", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Same Title")

		body := UpdateWikiPageRequest{Title: stringPtr("Same Title")}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/wiki/pages/%d", pageID), body, userID,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleUpdateWikiPage(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var page WikiPageResponse
		DecodeJSON(t, rec, &page)

		if page.Slug != "same-title" {
			t.Errorf("Expected slug 'same-title', got %q", page.Slug)
		}
	})

	t.Run("empty title returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Existing")

		body := UpdateWikiPageRequest{Title: stringPtr("")}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/wiki/pages/%d", pageID), body, userID,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleUpdateWikiPage(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "title cannot be empty", "invalid_input")
	})

	t.Run("title too long returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Existing")

		longTitle := strings.Repeat("x", 501)
		body := UpdateWikiPageRequest{Title: &longTitle}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/wiki/pages/%d", pageID), body, userID,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleUpdateWikiPage(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "title is too long", "invalid_input")
	})

	t.Run("not found returns 404", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		body := UpdateWikiPageRequest{Title: stringPtr("Updated")}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			"/api/wiki/pages/99999", body, userID,
			map[string]string{"pageId": "99999"})

		ts.HandleUpdateWikiPage(rec, req)

		AssertError(t, rec, http.StatusNotFound, "wiki page not found", "not_found")
	})

	t.Run("invalid page ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		body := UpdateWikiPageRequest{Title: stringPtr("Updated")}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			"/api/wiki/pages/abc", body, userID,
			map[string]string{"pageId": "abc"})

		ts.HandleUpdateWikiPage(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid page ID", "invalid_input")
	})

	t.Run("invalid JSON body returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Existing")

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/wiki/pages/%d", pageID), "not-json", userID,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleUpdateWikiPage(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid request body", "invalid_input")
	})

	t.Run("non-member gets 403", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		other := ts.CreateTestUser(t, "other@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Owner Project")
		pageID := ts.createTestWikiPage(t, projectID, owner, "Protected")

		body := UpdateWikiPageRequest{Title: stringPtr("Hacked")}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/wiki/pages/%d", pageID), body, other,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleUpdateWikiPage(rec, req)

		AssertError(t, rec, http.StatusForbidden, "access denied", "forbidden")
	})
}

// ---------------------------------------------------------------------------
// HandleDeleteWikiPage
// ---------------------------------------------------------------------------

func TestHandleDeleteWikiPage(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Delete Me")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			fmt.Sprintf("/api/wiki/pages/%d", pageID), nil, userID,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleDeleteWikiPage(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusNoContent)

		// Verify page was deleted
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var count int
		err := ts.DB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM wiki_pages WHERE id = ?`, pageID,
		).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to verify deletion: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected wiki page to be deleted, but found %d rows", count)
		}
	})

	t.Run("not found returns 404", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			"/api/wiki/pages/99999", nil, userID,
			map[string]string{"pageId": "99999"})

		ts.HandleDeleteWikiPage(rec, req)

		AssertError(t, rec, http.StatusNotFound, "wiki page not found", "not_found")
	})

	t.Run("invalid page ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			"/api/wiki/pages/abc", nil, userID,
			map[string]string{"pageId": "abc"})

		ts.HandleDeleteWikiPage(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid page ID", "invalid_input")
	})

	t.Run("non-member gets 403", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		other := ts.CreateTestUser(t, "other@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Owner Project")
		pageID := ts.createTestWikiPage(t, projectID, owner, "Protected Page")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			fmt.Sprintf("/api/wiki/pages/%d", pageID), nil, other,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleDeleteWikiPage(rec, req)

		AssertError(t, rec, http.StatusForbidden, "access denied", "forbidden")
	})
}

// ---------------------------------------------------------------------------
// HandleGetWikiPageContent
// ---------------------------------------------------------------------------

func TestHandleGetWikiPageContent(t *testing.T) {
	t.Run("happy path with content", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPageWithContent(t, projectID, userID, "Docs", "# Hello World\n\nSome content here.")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/wiki/pages/%d/content", pageID), nil, userID,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleGetWikiPageContent(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var content WikiPageContentResponse
		DecodeJSON(t, rec, &content)

		if content.PageID != pageID {
			t.Errorf("Expected page_id %d, got %d", pageID, content.PageID)
		}
		if content.Content != "# Hello World\n\nSome content here." {
			t.Errorf("Expected content '# Hello World\\n\\nSome content here.', got %q", content.Content)
		}
	})

	t.Run("empty content for new page", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Empty Page")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/wiki/pages/%d/content", pageID), nil, userID,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleGetWikiPageContent(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var content WikiPageContentResponse
		DecodeJSON(t, rec, &content)

		if content.Content != "" {
			t.Errorf("Expected empty content, got %q", content.Content)
		}
	})

	t.Run("not found returns 404", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			"/api/wiki/pages/99999/content", nil, userID,
			map[string]string{"pageId": "99999"})

		ts.HandleGetWikiPageContent(rec, req)

		AssertError(t, rec, http.StatusNotFound, "wiki page not found", "not_found")
	})

	t.Run("invalid page ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			"/api/wiki/pages/abc/content", nil, userID,
			map[string]string{"pageId": "abc"})

		ts.HandleGetWikiPageContent(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid page ID", "invalid_input")
	})

	t.Run("non-member gets 403", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		other := ts.CreateTestUser(t, "other@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Owner Project")
		pageID := ts.createTestWikiPageWithContent(t, projectID, owner, "Secret", "classified content")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/wiki/pages/%d/content", pageID), nil, other,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleGetWikiPageContent(rec, req)

		AssertError(t, rec, http.StatusForbidden, "access denied", "forbidden")
	})
}

// ---------------------------------------------------------------------------
// HandleUpdateWikiPageContent
// ---------------------------------------------------------------------------

func TestHandleUpdateWikiPageContent(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Docs")

		body := UpdateWikiPageContentRequest{Content: "# Updated\n\nNew content."}

		rec, req := ts.MakeAuthRequest(t, http.MethodPut,
			fmt.Sprintf("/api/wiki/pages/%d/content", pageID), body, userID,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleUpdateWikiPageContent(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var content WikiPageContentResponse
		DecodeJSON(t, rec, &content)

		if content.PageID != pageID {
			t.Errorf("Expected page_id %d, got %d", pageID, content.PageID)
		}
		if content.Content != "# Updated\n\nNew content." {
			t.Errorf("Expected updated content, got %q", content.Content)
		}
	})

	t.Run("set content to empty string", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPageWithContent(t, projectID, userID, "Docs", "original content")

		body := UpdateWikiPageContentRequest{Content: ""}

		rec, req := ts.MakeAuthRequest(t, http.MethodPut,
			fmt.Sprintf("/api/wiki/pages/%d/content", pageID), body, userID,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleUpdateWikiPageContent(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var content WikiPageContentResponse
		DecodeJSON(t, rec, &content)

		if content.Content != "" {
			t.Errorf("Expected empty content, got %q", content.Content)
		}
	})

	t.Run("not found returns 404", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		body := UpdateWikiPageContentRequest{Content: "something"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPut,
			"/api/wiki/pages/99999/content", body, userID,
			map[string]string{"pageId": "99999"})

		ts.HandleUpdateWikiPageContent(rec, req)

		AssertError(t, rec, http.StatusNotFound, "wiki page not found", "not_found")
	})

	t.Run("invalid page ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		body := UpdateWikiPageContentRequest{Content: "something"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPut,
			"/api/wiki/pages/abc/content", body, userID,
			map[string]string{"pageId": "abc"})

		ts.HandleUpdateWikiPageContent(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid page ID", "invalid_input")
	})

	t.Run("invalid JSON body returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Docs")

		rec, req := ts.MakeAuthRequest(t, http.MethodPut,
			fmt.Sprintf("/api/wiki/pages/%d/content", pageID), "not-json", userID,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleUpdateWikiPageContent(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid request body", "invalid_input")
	})

	t.Run("non-member gets 403", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		other := ts.CreateTestUser(t, "other@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Owner Project")
		pageID := ts.createTestWikiPage(t, projectID, owner, "Protected")

		body := UpdateWikiPageContentRequest{Content: "hacked"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPut,
			fmt.Sprintf("/api/wiki/pages/%d/content", pageID), body, other,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleUpdateWikiPageContent(rec, req)

		AssertError(t, rec, http.StatusForbidden, "access denied", "forbidden")
	})
}

// ---------------------------------------------------------------------------
// HandleWikiPreview
// ---------------------------------------------------------------------------

func TestHandleWikiPreview(t *testing.T) {
	t.Run("renders markdown to HTML", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		body := wikiPreviewRequest{Content: "# Hello\n\nSome **bold** text."}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			"/api/wiki/preview", body, userID, nil)

		ts.HandleWikiPreview(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp wikiPreviewResponse
		DecodeJSON(t, rec, &resp)

		if resp.HTML == "" {
			t.Error("Expected non-empty HTML response")
		}

		// Check that basic markdown rendered
		if !strings.Contains(resp.HTML, "Hello") {
			t.Errorf("Expected HTML to contain 'Hello', got %q", resp.HTML)
		}
		if !strings.Contains(resp.HTML, "<strong>") || !strings.Contains(resp.HTML, "bold") {
			t.Errorf("Expected HTML to contain bold text, got %q", resp.HTML)
		}
	})

	t.Run("empty content returns valid response", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		body := wikiPreviewRequest{Content: ""}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			"/api/wiki/preview", body, userID, nil)

		ts.HandleWikiPreview(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp wikiPreviewResponse
		DecodeJSON(t, rec, &resp)

		// Should return valid JSON regardless of empty input
		_ = resp.HTML
	})

	t.Run("invalid JSON returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			"/api/wiki/preview", "not-json", userID, nil)

		ts.HandleWikiPreview(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid JSON body", "invalid_input")
	})

	t.Run("strips draw edit mode from rendered HTML", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		body := wikiPreviewRequest{Content: "Some text with a drawing reference."}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			"/api/wiki/preview", body, userID, nil)

		ts.HandleWikiPreview(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp wikiPreviewResponse
		DecodeJSON(t, rec, &resp)

		// Ensure no /edit in the output
		if strings.Contains(resp.HTML, `/edit"`) {
			t.Errorf("Expected /edit to be stripped from HTML, got %q", resp.HTML)
		}
	})
}

// ---------------------------------------------------------------------------
// Integration: Create then List
// ---------------------------------------------------------------------------

func TestWikiCreateThenList(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")

	// Create two pages
	for _, title := range []string{"Setup Guide", "API Reference"} {
		body := CreateWikiPageRequest{Title: title}
		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/wiki", projectID), body, userID,
			map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

		ts.HandleCreateWikiPage(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusCreated)
	}

	// List them
	rec, req := ts.MakeAuthRequest(t, http.MethodGet,
		fmt.Sprintf("/api/projects/%d/wiki", projectID), nil, userID,
		map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

	ts.HandleListWikiPages(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	var pages []WikiPageResponse
	DecodeJSON(t, rec, &pages)

	if len(pages) != 2 {
		t.Fatalf("Expected 2 pages, got %d", len(pages))
	}

	// Verify sorted by sibling position (creation order), then title
	if pages[0].Title != "Setup Guide" {
		t.Errorf("Expected first page 'Setup Guide', got %q", pages[0].Title)
	}
	if pages[1].Title != "API Reference" {
		t.Errorf("Expected second page 'API Reference', got %q", pages[1].Title)
	}
	if pages[0].Position != 0 || pages[1].Position != 1 {
		t.Errorf("Expected positions 0 and 1, got %d and %d", pages[0].Position, pages[1].Position)
	}
}

// ---------------------------------------------------------------------------
// Integration: Create, Update Content, Read Content
// ---------------------------------------------------------------------------

func TestWikiContentRoundTrip(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")

	// Create page
	createBody := CreateWikiPageRequest{Title: "My Page"}
	createRec, createReq := ts.MakeAuthRequest(t, http.MethodPost,
		fmt.Sprintf("/api/projects/%d/wiki", projectID), createBody, userID,
		map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

	ts.HandleCreateWikiPage(createRec, createReq)
	AssertStatusCode(t, createRec.Code, http.StatusCreated)

	var created WikiPageResponse
	DecodeJSON(t, createRec, &created)

	// Update content
	updateBody := UpdateWikiPageContentRequest{Content: "# Introduction\n\nWelcome to the wiki."}
	updateRec, updateReq := ts.MakeAuthRequest(t, http.MethodPut,
		fmt.Sprintf("/api/wiki/pages/%d/content", created.ID), updateBody, userID,
		map[string]string{"pageId": fmt.Sprintf("%d", created.ID)})

	ts.HandleUpdateWikiPageContent(updateRec, updateReq)
	AssertStatusCode(t, updateRec.Code, http.StatusOK)

	// Read it back
	getRec, getReq := ts.MakeAuthRequest(t, http.MethodGet,
		fmt.Sprintf("/api/wiki/pages/%d/content", created.ID), nil, userID,
		map[string]string{"pageId": fmt.Sprintf("%d", created.ID)})

	ts.HandleGetWikiPageContent(getRec, getReq)
	AssertStatusCode(t, getRec.Code, http.StatusOK)

	var content WikiPageContentResponse
	DecodeJSON(t, getRec, &content)

	if content.Content != "# Introduction\n\nWelcome to the wiki." {
		t.Errorf("Content round-trip failed, got %q", content.Content)
	}
}

func TestWikiVersionHistoryCompressedAndRetained(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "versions@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Version Project")
	pageID := ts.createTestWikiPage(t, projectID, userID, "Versioned Page")

	for i := 1; i <= 55; i++ {
		body := UpdateWikiPageContentRequest{
			Content:    fmt.Sprintf("# Version %02d\n\n%s", i, strings.Repeat("substantial content ", 40)),
			ManualSave: true,
		}
		rec, req := ts.MakeAuthRequest(t, http.MethodPut,
			fmt.Sprintf("/api/wiki/pages/%d/content", pageID), body, userID,
			map[string]string{"pageId": fmt.Sprintf("%d", pageID)})

		ts.HandleUpdateWikiPageContent(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusOK)
	}

	listRec, listReq := ts.MakeAuthRequest(t, http.MethodGet,
		fmt.Sprintf("/api/wiki/pages/%d/versions", pageID), nil, userID,
		map[string]string{"pageId": fmt.Sprintf("%d", pageID)})
	ts.HandleListWikiPageVersions(listRec, listReq)
	AssertStatusCode(t, listRec.Code, http.StatusOK)

	var versions []WikiPageVersionResponse
	DecodeJSON(t, listRec, &versions)
	if len(versions) != wikiPageVersionRetentionLimit {
		t.Fatalf("expected %d retained versions, got %d", wikiPageVersionRetentionLimit, len(versions))
	}
	if versions[0].VersionNumber != 55 {
		t.Fatalf("expected newest retained version 55, got %d", versions[0].VersionNumber)
	}
	if versions[len(versions)-1].VersionNumber != 6 {
		t.Fatalf("expected oldest retained version 6, got %d", versions[len(versions)-1].VersionNumber)
	}

	var plaintextRows int
	if err := ts.DB.QueryRow(`SELECT COUNT(*) FROM wiki_page_versions WHERE wiki_page_id = ? AND content <> ''`, pageID).Scan(&plaintextRows); err != nil {
		t.Fatalf("failed to count plaintext version rows: %v", err)
	}
	if plaintextRows != 0 {
		t.Fatalf("expected all retained versions to store compressed content only, got %d plaintext rows", plaintextRows)
	}

	getRec, getReq := ts.MakeAuthRequest(t, http.MethodGet,
		fmt.Sprintf("/api/wiki/pages/%d/versions/6", pageID), nil, userID,
		map[string]string{"pageId": fmt.Sprintf("%d", pageID), "versionNumber": "6"})
	ts.HandleGetWikiPageVersion(getRec, getReq)
	AssertStatusCode(t, getRec.Code, http.StatusOK)

	var version WikiPageVersionWithContentResponse
	DecodeJSON(t, getRec, &version)
	if !strings.Contains(version.Content, "# Version 06") {
		t.Fatalf("expected decompressed version 6 content, got %q", version.Content)
	}
}

func TestOptimizeWikiPageVersionStoragePrunesAndCompressesLegacyRows(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "legacy-versions@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Legacy Version Project")
	pageID := ts.createTestWikiPage(t, projectID, userID, "Legacy Versioned Page")

	for i := 1; i <= 60; i++ {
		content := fmt.Sprintf("# Legacy Version %02d\n\n%s", i, strings.Repeat("legacy content ", 30))
		if _, err := ts.DB.Exec(
			`INSERT INTO wiki_page_versions (wiki_page_id, version_number, content, content_hash, created_by) VALUES (?, ?, ?, ?, ?)`,
			pageID, i, content, fmt.Sprintf("hash-%02d", i), userID,
		); err != nil {
			t.Fatalf("failed to insert legacy version %d: %v", i, err)
		}
	}

	ts.OptimizeWikiPageVersionStorage(context.Background())

	var retained int
	if err := ts.DB.QueryRow(`SELECT COUNT(*) FROM wiki_page_versions WHERE wiki_page_id = ?`, pageID).Scan(&retained); err != nil {
		t.Fatalf("failed to count retained versions: %v", err)
	}
	if retained != wikiPageVersionRetentionLimit {
		t.Fatalf("expected %d retained versions, got %d", wikiPageVersionRetentionLimit, retained)
	}

	var oldest int
	if err := ts.DB.QueryRow(`SELECT MIN(version_number) FROM wiki_page_versions WHERE wiki_page_id = ?`, pageID).Scan(&oldest); err != nil {
		t.Fatalf("failed to get oldest retained version: %v", err)
	}
	if oldest != 11 {
		t.Fatalf("expected oldest retained version 11, got %d", oldest)
	}

	var plaintextRows int
	if err := ts.DB.QueryRow(`SELECT COUNT(*) FROM wiki_page_versions WHERE wiki_page_id = ? AND content <> ''`, pageID).Scan(&plaintextRows); err != nil {
		t.Fatalf("failed to count plaintext version rows: %v", err)
	}
	if plaintextRows != 0 {
		t.Fatalf("expected optimizer to compress all retained legacy versions, got %d plaintext rows", plaintextRows)
	}

	getRec, getReq := ts.MakeAuthRequest(t, http.MethodGet,
		fmt.Sprintf("/api/wiki/pages/%d/versions/60", pageID), nil, userID,
		map[string]string{"pageId": fmt.Sprintf("%d", pageID), "versionNumber": "60"})
	ts.HandleGetWikiPageVersion(getRec, getReq)
	AssertStatusCode(t, getRec.Code, http.StatusOK)

	var version WikiPageVersionWithContentResponse
	DecodeJSON(t, getRec, &version)
	if !strings.Contains(version.Content, "# Legacy Version 60") {
		t.Fatalf("expected decompressed legacy version content, got %q", version.Content)
	}
}

// ---------------------------------------------------------------------------
// Integration: Create then Delete, then verify gone
// ---------------------------------------------------------------------------

func TestWikiCreateThenDelete(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")

	// Create page
	body := CreateWikiPageRequest{Title: "Temporary"}
	createRec, createReq := ts.MakeAuthRequest(t, http.MethodPost,
		fmt.Sprintf("/api/projects/%d/wiki", projectID), body, userID,
		map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

	ts.HandleCreateWikiPage(createRec, createReq)
	AssertStatusCode(t, createRec.Code, http.StatusCreated)

	var created WikiPageResponse
	DecodeJSON(t, createRec, &created)

	// Delete it
	delRec, delReq := ts.MakeAuthRequest(t, http.MethodDelete,
		fmt.Sprintf("/api/wiki/pages/%d", created.ID), nil, userID,
		map[string]string{"pageId": fmt.Sprintf("%d", created.ID)})

	ts.HandleDeleteWikiPage(delRec, delReq)
	AssertStatusCode(t, delRec.Code, http.StatusNoContent)

	// Confirm it is gone
	getRec, getReq := ts.MakeAuthRequest(t, http.MethodGet,
		fmt.Sprintf("/api/wiki/pages/%d", created.ID), nil, userID,
		map[string]string{"pageId": fmt.Sprintf("%d", created.ID)})

	ts.HandleGetWikiPage(getRec, getReq)
	AssertError(t, getRec, http.StatusNotFound, "wiki page not found", "not_found")
}

// ---------------------------------------------------------------------------
// HandleListWikiPages response structure validation
// ---------------------------------------------------------------------------

func TestHandleListWikiPagesResponseFields(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	ts.createTestWikiPage(t, projectID, userID, "Field Check")

	rec, req := ts.MakeAuthRequest(t, http.MethodGet,
		fmt.Sprintf("/api/projects/%d/wiki", projectID), nil, userID,
		map[string]string{"projectId": fmt.Sprintf("%d", projectID)})

	ts.HandleListWikiPages(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	// Parse as raw JSON to verify field presence
	var rawPages []json.RawMessage
	DecodeJSON(t, rec, &rawPages)

	if len(rawPages) != 1 {
		t.Fatalf("Expected 1 page, got %d", len(rawPages))
	}

	var fields map[string]interface{}
	if err := json.Unmarshal(rawPages[0], &fields); err != nil {
		t.Fatalf("Failed to unmarshal page fields: %v", err)
	}

	requiredFields := []string{"id", "project_id", "title", "slug", "created_by", "created_at", "updated_at"}
	for _, f := range requiredFields {
		if _, ok := fields[f]; !ok {
			t.Errorf("Missing required field %q in wiki page response", f)
		}
	}
}
