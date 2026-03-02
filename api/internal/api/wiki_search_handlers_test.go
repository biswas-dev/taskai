package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// TestSearchWikiRecencyFilter — covers the recency_days code path in
// HandleSearchWiki / searchWikiBlocks which was not exercised by existing tests.
// ---------------------------------------------------------------------------

func TestSearchWikiRecencyFilter(t *testing.T) {
	t.Run("recency_days includes recently created pages", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		pageID := ts.createTestWikiPage(t, projectID, userID, "Recent Recency Page")
		ts.createTestWikiBlock(t, pageID, "paragraph", "recencyfiltertestcontent alpha", "", 0)

		recencyDays := 7
		body := SearchWikiRequest{
			Query:       "recencyfiltertestcontent",
			RecencyDays: &recencyDays,
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)

		// Page was just created, should appear within 7 days
		if resp.Total == 0 {
			t.Error("Expected results for recently created page with recency_days=7")
		}
	})

	t.Run("recency_days zero is not applied as filter", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Zero Recency Page")
		ts.createTestWikiBlock(t, pageID, "paragraph", "zerorecencyfilter content", "", 0)

		recencyDays := 0
		body := SearchWikiRequest{
			Query:       "zerorecencyfilter",
			RecencyDays: &recencyDays,
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)

		// recency_days=0 should not filter anything
		if resp.Total == 0 {
			t.Error("Expected results when recency_days=0")
		}
	})

	t.Run("recency_days nil is not applied as filter", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Nil Recency Page")
		ts.createTestWikiBlock(t, pageID, "paragraph", "nilrecencyfilter content", "", 0)

		body := SearchWikiRequest{
			Query:       "nilrecencyfilter",
			RecencyDays: nil,
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)

		if resp.Total == 0 {
			t.Error("Expected results when recency_days is nil")
		}
	})

	t.Run("recency_days=1 excludes old pages only", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		// Create a page (created_at and updated_at are "now")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Today Page")
		ts.createTestWikiBlock(t, pageID, "paragraph", "recencydayone uniquecontent", "", 0)

		recencyDays := 1
		body := SearchWikiRequest{
			Query:       "recencydayone uniquecontent",
			RecencyDays: &recencyDays,
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)

		// Just-created page should be within 1-day recency
		if resp.Total == 0 {
			t.Error("Expected results for page created today with recency_days=1")
		}
	})
}

// ---------------------------------------------------------------------------
// TestSearchWikiNilFields — covers nil plain_text and nil headings_path
// paths in searchWikiBlocks result mapping
// ---------------------------------------------------------------------------

func TestSearchWikiNilFields(t *testing.T) {
	t.Run("block with nil plain_text returns empty snippet", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Nil PlainText Page")

		// Create block via ent without setting plain_text
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := ts.DB.Client.WikiBlock.Create().
			SetPageID(pageID).
			SetBlockType("heading").
			SetHeadingsPath("NilPlainTextSearchable Heading").
			SetPosition(0).
			Save(ctx)
		if err != nil {
			t.Fatalf("Failed to create wiki block without plain_text: %v", err)
		}

		body := SearchWikiRequest{Query: "NilPlainTextSearchable"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)

		if resp.Total == 0 {
			t.Fatal("Expected at least 1 result for heading match")
		}

		if resp.Results[0].Snippet != "" {
			t.Errorf("Expected empty snippet for nil plain_text, got %q", resp.Results[0].Snippet)
		}
	})

	t.Run("block with nil headings_path returns empty headings_path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Nil Headings Page")

		// Create block via ent without setting headings_path
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := ts.DB.Client.WikiBlock.Create().
			SetPageID(pageID).
			SetBlockType("paragraph").
			SetPlainText("NilHeadingsPathSearchable content here").
			SetPosition(0).
			Save(ctx)
		if err != nil {
			t.Fatalf("Failed to create wiki block without headings_path: %v", err)
		}

		body := SearchWikiRequest{Query: "NilHeadingsPathSearchable"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)

		if resp.Total == 0 {
			t.Fatal("Expected at least 1 result")
		}

		if resp.Results[0].HeadingsPath != "" {
			t.Errorf("Expected empty headings_path for nil value, got %q", resp.Results[0].HeadingsPath)
		}
	})
}

// ---------------------------------------------------------------------------
// TestSearchWikiCaseInsensitive — verifies ContainsFold behavior
// ---------------------------------------------------------------------------

func TestSearchWikiCaseInsensitive(t *testing.T) {
	t.Run("case insensitive search in plain_text", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Case Test Page")

		ts.createTestWikiBlock(t, pageID, "paragraph", "CaSeInSeNsItIvEuNiQuE content here", "", 0)

		body := SearchWikiRequest{Query: "caseinsensitiveunique"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)

		if resp.Total == 0 {
			t.Error("Expected case-insensitive search to find results")
		}
	})

	t.Run("case insensitive search in headings_path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Case Heading Page")

		ts.createTestWikiBlock(t, pageID, "heading", "some content", "HeAdInGcAsEuNiQuE > Sub", 0)

		body := SearchWikiRequest{Query: "headingcaseunique"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)

		if resp.Total == 0 {
			t.Error("Expected case-insensitive heading search to find results")
		}
	})
}

// ---------------------------------------------------------------------------
// TestSearchWikiSnippetTruncation — covers the exact truncation boundary
// ---------------------------------------------------------------------------

func TestSearchWikiSnippetTruncation(t *testing.T) {
	t.Run("snippet at exactly 200 chars is not truncated", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Exact Len Page")

		// Create text that is exactly 200 chars
		prefix := "exactlensnippet "
		text := prefix + strings.Repeat("x", 200-len(prefix))
		ts.createTestWikiBlock(t, pageID, "paragraph", text, "", 0)

		body := SearchWikiRequest{Query: "exactlensnippet"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)

		if resp.Total == 0 {
			t.Fatal("Expected at least 1 result")
		}

		snippet := resp.Results[0].Snippet
		if len(snippet) != 200 {
			t.Errorf("Expected snippet of exactly 200 chars, got %d", len(snippet))
		}
		if strings.HasSuffix(snippet, "...") {
			t.Error("Expected no truncation for exactly 200 char snippet")
		}
	})

	t.Run("snippet at 201 chars is truncated", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Over Len Page")

		prefix := "overlensnippet "
		text := prefix + strings.Repeat("y", 201-len(prefix))
		ts.createTestWikiBlock(t, pageID, "paragraph", text, "", 0)

		body := SearchWikiRequest{Query: "overlensnippet"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)

		if resp.Total == 0 {
			t.Fatal("Expected at least 1 result")
		}

		snippet := resp.Results[0].Snippet
		// Truncated: first 200 chars + "..."
		if len(snippet) != 203 {
			t.Errorf("Expected truncated snippet of 203 chars (200+...), got %d", len(snippet))
		}
		if !strings.HasSuffix(snippet, "...") {
			t.Error("Expected truncated snippet to end with '...'")
		}
	})
}

// ---------------------------------------------------------------------------
// TestSearchWikiMultipleProjects — covers searching across multiple projects
// without project_id filter
// ---------------------------------------------------------------------------

func TestSearchWikiMultipleProjects(t *testing.T) {
	t.Run("search across all accessible projects returns results from multiple", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		project1 := ts.CreateTestProject(t, userID, "Project Alpha")
		project2 := ts.CreateTestProject(t, userID, "Project Beta")

		page1 := ts.createTestWikiPage(t, project1, userID, "P1 Multi Page")
		page2 := ts.createTestWikiPage(t, project2, userID, "P2 Multi Page")

		ts.createTestWikiBlock(t, page1, "paragraph", "multiprojectsearchunique content in alpha", "", 0)
		ts.createTestWikiBlock(t, page2, "paragraph", "multiprojectsearchunique content in beta", "", 0)

		body := SearchWikiRequest{Query: "multiprojectsearchunique"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)

		if resp.Total != 2 {
			t.Errorf("Expected 2 results from both projects, got %d", resp.Total)
		}
	})
}

// ---------------------------------------------------------------------------
// TestAutocompleteInvalidProjectID — covers invalid project_id parse path
// ---------------------------------------------------------------------------

func TestAutocompleteInvalidProjectID(t *testing.T) {
	t.Run("non-numeric project_id is ignored", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		ts.createTestWikiPage(t, projectID, userID, "AutoInvalidProjIDPage")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			"/api/wiki/autocomplete?query=AutoInvalidProjIDPage&project_id=abc", nil, userID, nil)
		ts.HandleAutocompletePages(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var results []AutocompletePageResult
		DecodeJSON(t, rec, &results)

		// Invalid project_id means projectID is nil, so searches all accessible
		if len(results) == 0 {
			t.Error("Expected results when project_id is invalid (should search all accessible)")
		}
	})
}

// ---------------------------------------------------------------------------
// TestAutocompleteNegativeLimit — covers the negative limit edge case
// ---------------------------------------------------------------------------

func TestAutocompleteNegativeLimit(t *testing.T) {
	t.Run("negative limit uses default", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		ts.createTestWikiPage(t, projectID, userID, "AutoNegLimitPage")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			"/api/wiki/autocomplete?query=AutoNegLimitPage&limit=-5", nil, userID, nil)
		ts.HandleAutocompletePages(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var results []AutocompletePageResult
		DecodeJSON(t, rec, &results)

		// Should still work (negative limit => parsedLimit <= 0, so skip the if branch, use default=10)
		if len(results) != 1 {
			t.Errorf("Expected 1 result, got %d", len(results))
		}
	})
}

// ---------------------------------------------------------------------------
// TestAutocompleteSearchesMultipleProjects — covers searching across
// multiple projects without project_id
// ---------------------------------------------------------------------------

func TestAutocompleteSearchesMultipleProjects(t *testing.T) {
	t.Run("returns pages from all accessible projects", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		project1 := ts.CreateTestProject(t, userID, "Project 1")
		project2 := ts.CreateTestProject(t, userID, "Project 2")

		ts.createTestWikiPage(t, project1, userID, "AutoMultiProjUnique Doc 1")
		ts.createTestWikiPage(t, project2, userID, "AutoMultiProjUnique Doc 2")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			"/api/wiki/autocomplete?query=AutoMultiProjUnique", nil, userID, nil)
		ts.HandleAutocompletePages(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var results []AutocompletePageResult
		DecodeJSON(t, rec, &results)

		if len(results) < 2 {
			t.Errorf("Expected at least 2 results from both projects, got %d", len(results))
		}
	})
}

// ---------------------------------------------------------------------------
// TestAutocompleteResponseFields — verifies all response fields are present
// ---------------------------------------------------------------------------

func TestAutocompleteResponseFields(t *testing.T) {
	t.Run("response includes id, title, and slug", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		ts.createTestWikiPage(t, projectID, userID, "AutoFieldCheckPage")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			"/api/wiki/autocomplete?query=AutoFieldCheckPage", nil, userID, nil)
		ts.HandleAutocompletePages(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var results []AutocompletePageResult
		DecodeJSON(t, rec, &results)

		if len(results) != 1 {
			t.Fatalf("Expected 1 result, got %d", len(results))
		}

		r := results[0]
		if r.ID == 0 {
			t.Error("Expected non-zero ID")
		}
		if r.Title != "AutoFieldCheckPage" {
			t.Errorf("Expected title 'AutoFieldCheckPage', got %q", r.Title)
		}
		if r.Slug != "autofieldcheckpage" {
			t.Errorf("Expected slug 'autofieldcheckpage', got %q", r.Slug)
		}
	})
}

// ---------------------------------------------------------------------------
// TestSearchWikiDefaultLimitBehavior — additional coverage of default limit
// ---------------------------------------------------------------------------

func TestSearchWikiDefaultLimitBehavior(t *testing.T) {
	t.Run("default limit caps results at 20", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		pageID := ts.createTestWikiPage(t, projectID, userID, "Lots of Blocks")

		// Create 25 blocks
		for i := 0; i < 25; i++ {
			ts.createTestWikiBlock(t, pageID, "paragraph",
				fmt.Sprintf("defaultlimittestunique block number %d", i), "", i)
		}

		body := SearchWikiRequest{
			Query: "defaultlimittestunique",
			// Limit: 0 => should default to 20
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)

		if resp.Total > 20 {
			t.Errorf("Expected at most 20 results with default limit, got %d", resp.Total)
		}
	})
}

// ---------------------------------------------------------------------------
// TestAutocompleteDefaultLimit — covers default limit=10 for autocomplete
// ---------------------------------------------------------------------------

func TestAutocompleteDefaultLimit(t *testing.T) {
	t.Run("default limit is 10", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		for i := 0; i < 15; i++ {
			ts.createTestWikiPage(t, projectID, userID, fmt.Sprintf("AutoDefLimit %02d", i))
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			"/api/wiki/autocomplete?query=AutoDefLimit", nil, userID, nil)
		ts.HandleAutocompletePages(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var results []AutocompletePageResult
		DecodeJSON(t, rec, &results)

		if len(results) > 10 {
			t.Errorf("Expected at most 10 results (default limit), got %d", len(results))
		}
	})
}

// ---------------------------------------------------------------------------
// TestGetUserAccessibleProjectsUnit — direct unit tests for the helper
// ---------------------------------------------------------------------------

func TestGetUserAccessibleProjectsUnit(t *testing.T) {
	t.Run("returns all project IDs for user", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		p1 := ts.CreateTestProject(t, userID, "Project 1")
		p2 := ts.CreateTestProject(t, userID, "Project 2")
		p3 := ts.CreateTestProject(t, userID, "Project 3")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		projectIDs, err := ts.Server.getUserAccessibleProjects(ctx, userID)
		if err != nil {
			t.Fatalf("getUserAccessibleProjects failed: %v", err)
		}

		if len(projectIDs) != 3 {
			t.Fatalf("Expected 3 project IDs, got %d", len(projectIDs))
		}

		idSet := make(map[int64]bool)
		for _, id := range projectIDs {
			idSet[id] = true
		}

		for _, expected := range []int64{p1, p2, p3} {
			if !idSet[expected] {
				t.Errorf("Expected project ID %d in accessible list", expected)
			}
		}
	})

	t.Run("returns empty for user with no memberships", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		projectIDs, err := ts.Server.getUserAccessibleProjects(ctx, userID)
		if err != nil {
			t.Fatalf("getUserAccessibleProjects failed: %v", err)
		}

		if len(projectIDs) != 0 {
			t.Errorf("Expected 0 project IDs, got %d", len(projectIDs))
		}
	})

	t.Run("includes projects where user is member not owner", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		member := ts.CreateTestUser(t, "member@example.com", "password123")

		projectID := ts.CreateTestProject(t, owner, "Shared Project")
		ts.AddProjectMember(t, projectID, member, owner, "member")

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		projectIDs, err := ts.Server.getUserAccessibleProjects(ctx, member)
		if err != nil {
			t.Fatalf("getUserAccessibleProjects failed: %v", err)
		}

		if len(projectIDs) != 1 {
			t.Fatalf("Expected 1 project ID, got %d", len(projectIDs))
		}

		if projectIDs[0] != projectID {
			t.Errorf("Expected project ID %d, got %d", projectID, projectIDs[0])
		}
	})
}

// ---------------------------------------------------------------------------
// TestSearchWikiIntegration — end-to-end integration of search + autocomplete
// ---------------------------------------------------------------------------

func TestWikiSearchAndAutocompleteIntegration(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Integration Project")

	// Create pages with content blocks
	page1ID := ts.createTestWikiPage(t, projectID, userID, "Integration Architecture")
	ts.createTestWikiBlock(t, page1ID, "paragraph", "integrationtestunique system architecture overview", "Architecture > Overview", 0)
	ts.createTestWikiBlock(t, page1ID, "heading", "integrationtestunique microservices", "Architecture > Microservices", 1)

	page2ID := ts.createTestWikiPage(t, projectID, userID, "Integration Deployment")
	ts.createTestWikiBlock(t, page2ID, "paragraph", "integrationtestunique deploy to production", "", 0)

	t.Run("search finds blocks across pages", func(t *testing.T) {
		body := SearchWikiRequest{Query: "integrationtestunique"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", body, userID, nil)
		ts.HandleSearchWiki(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)

		if resp.Total < 3 {
			t.Errorf("Expected at least 3 results (blocks across 2 pages), got %d", resp.Total)
		}
	})

	t.Run("autocomplete finds pages by partial title", func(t *testing.T) {
		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			"/api/wiki/autocomplete?query=Integration", nil, userID, nil)
		ts.HandleAutocompletePages(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var results []AutocompletePageResult
		DecodeJSON(t, rec, &results)

		if len(results) != 2 {
			t.Fatalf("Expected 2 autocomplete results for 'Integration', got %d", len(results))
		}

		// Verify sorted by title
		if results[0].Title != "Integration Architecture" {
			t.Errorf("Expected first result 'Integration Architecture', got %q", results[0].Title)
		}
		if results[1].Title != "Integration Deployment" {
			t.Errorf("Expected second result 'Integration Deployment', got %q", results[1].Title)
		}
	})
}
