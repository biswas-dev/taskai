package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func int64Ptr(v int64) *int64 { return &v }

// createPageViaHandler creates a page through HandleCreateWikiPage and returns the response.
func (ts *TestServer) createPageViaHandler(t testing.TB, projectID, userID int64, title string, parentID *int64) (WikiPageResponse, int) {
	t.Helper()
	body := CreateWikiPageRequest{Title: title, ParentID: parentID}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost,
		fmt.Sprintf("/api/projects/%d/wiki/pages", projectID), body, userID,
		map[string]string{"projectId": fmt.Sprintf("%d", projectID)})
	ts.HandleCreateWikiPage(rec, req)
	var page WikiPageResponse
	if rec.Code == http.StatusCreated {
		DecodeJSON(t, rec, &page)
	}
	return page, rec.Code
}

// patchPageRaw sends a raw JSON PATCH body to HandleUpdateWikiPage.
func (ts *TestServer) patchPageRaw(t testing.TB, pageID, userID int64, rawBody string) (*WikiPageResponse, int, ErrorResponse) {
	t.Helper()
	rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
		fmt.Sprintf("/api/wiki/pages/%d", pageID), json.RawMessage(rawBody), userID,
		map[string]string{"pageId": fmt.Sprintf("%d", pageID)})
	ts.HandleUpdateWikiPage(rec, req)
	if rec.Code == http.StatusOK {
		var page WikiPageResponse
		DecodeJSON(t, rec, &page)
		return &page, rec.Code, ErrorResponse{}
	}
	var errResp ErrorResponse
	DecodeJSON(t, rec, &errResp)
	return nil, rec.Code, errResp
}

func TestOptionalInt64Unmarshal(t *testing.T) {
	tests := []struct {
		name      string
		body      string
		wantSet   bool
		wantNil   bool
		wantValue int64
		wantErr   bool
	}{
		{name: "omitted", body: `{}`, wantSet: false},
		{name: "explicit null", body: `{"parent_id": null}`, wantSet: true, wantNil: true},
		{name: "number", body: `{"parent_id": 42}`, wantSet: true, wantValue: 42},
		{name: "string is rejected", body: `{"parent_id": "42"}`, wantErr: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var req UpdateWikiPageRequest
			err := json.Unmarshal([]byte(tt.body), &req)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				return
			}
			if req.ParentID.Set != tt.wantSet {
				t.Errorf("Set = %v, want %v", req.ParentID.Set, tt.wantSet)
			}
			if tt.wantSet && tt.wantNil && req.ParentID.Value != nil {
				t.Errorf("Value = %v, want nil", *req.ParentID.Value)
			}
			if tt.wantSet && !tt.wantNil && (req.ParentID.Value == nil || *req.ParentID.Value != tt.wantValue) {
				t.Errorf("Value = %v, want %d", req.ParentID.Value, tt.wantValue)
			}
		})
	}

	t.Run("unset field is omitted when marshalled", func(t *testing.T) {
		out, err := json.Marshal(UpdateWikiPageRequest{Title: stringPtr("x")})
		if err != nil {
			t.Fatal(err)
		}
		if string(out) != `{"title":"x"}` {
			t.Errorf("unexpected JSON: %s", out)
		}
	})
}

func TestWikiHierarchyCreate(t *testing.T) {
	t.Run("child page records parent and sibling position", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()
		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		root, code := ts.createPageViaHandler(t, projectID, userID, "Root", nil)
		AssertStatusCode(t, code, http.StatusCreated)
		if root.ParentID != nil {
			t.Fatalf("root page should have nil parent, got %d", *root.ParentID)
		}

		c1, code := ts.createPageViaHandler(t, projectID, userID, "Child 1", int64Ptr(root.ID))
		AssertStatusCode(t, code, http.StatusCreated)
		c2, code := ts.createPageViaHandler(t, projectID, userID, "Child 2", int64Ptr(root.ID))
		AssertStatusCode(t, code, http.StatusCreated)

		if c1.ParentID == nil || *c1.ParentID != root.ID {
			t.Errorf("child 1 parent = %v, want %d", c1.ParentID, root.ID)
		}
		if c1.Position != 0 || c2.Position != 1 {
			t.Errorf("sibling positions = %d, %d; want 0, 1", c1.Position, c2.Position)
		}
	})

	t.Run("parent from another project is rejected", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()
		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		p1 := ts.CreateTestProject(t, userID, "Project 1")
		p2 := ts.CreateTestProject(t, userID, "Project 2")
		foreign, _ := ts.createPageViaHandler(t, p2, userID, "Foreign", nil)

		body := CreateWikiPageRequest{Title: "Orphan", ParentID: int64Ptr(foreign.ID)}
		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/wiki/pages", p1), body, userID,
			map[string]string{"projectId": fmt.Sprintf("%d", p1)})
		ts.HandleCreateWikiPage(rec, req)
		AssertError(t, rec, http.StatusBadRequest, "parent page not found", "invalid_parent")
	})

	t.Run("nesting is capped at six levels", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()
		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		var parent *int64
		for level := 1; level <= wikiMaxDepth; level++ {
			page, code := ts.createPageViaHandler(t, projectID, userID, fmt.Sprintf("Level %d", level), parent)
			if code != http.StatusCreated {
				t.Fatalf("level %d: expected 201, got %d", level, code)
			}
			parent = int64Ptr(page.ID)
		}

		body := CreateWikiPageRequest{Title: "Level 7", ParentID: parent}
		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/wiki/pages", projectID), body, userID,
			map[string]string{"projectId": fmt.Sprintf("%d", projectID)})
		ts.HandleCreateWikiPage(rec, req)
		AssertError(t, rec, http.StatusBadRequest, "6 levels", "max_depth_exceeded")
	})
}

func TestWikiHierarchyMove(t *testing.T) {
	setup := func(t *testing.T) (*TestServer, int64, WikiPageResponse, WikiPageResponse, WikiPageResponse) {
		ts := NewTestServer(t)
		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		a, _ := ts.createPageViaHandler(t, projectID, userID, "A", nil)
		b, _ := ts.createPageViaHandler(t, projectID, userID, "B", int64Ptr(a.ID))
		c, _ := ts.createPageViaHandler(t, projectID, userID, "C", int64Ptr(b.ID))
		return ts, userID, a, b, c
	}

	t.Run("move page under a new parent", func(t *testing.T) {
		ts, userID, a, _, c := setup(t)
		defer ts.Close()
		page, code, _ := ts.patchPageRaw(t, c.ID, userID, fmt.Sprintf(`{"parent_id": %d}`, a.ID))
		AssertStatusCode(t, code, http.StatusOK)
		if page.ParentID == nil || *page.ParentID != a.ID {
			t.Errorf("parent = %v, want %d", page.ParentID, a.ID)
		}
		// B already occupies position 0 under A, so C is appended after it.
		if page.Position != 1 {
			t.Errorf("position = %d, want 1", page.Position)
		}
	})

	t.Run("explicit null moves page to root", func(t *testing.T) {
		ts, userID, _, b, _ := setup(t)
		defer ts.Close()
		page, code, _ := ts.patchPageRaw(t, b.ID, userID, `{"parent_id": null}`)
		AssertStatusCode(t, code, http.StatusOK)
		if page.ParentID != nil {
			t.Errorf("parent = %d, want nil", *page.ParentID)
		}
	})

	t.Run("omitted parent leaves hierarchy untouched", func(t *testing.T) {
		ts, userID, _, b, c := setup(t)
		defer ts.Close()
		page, code, _ := ts.patchPageRaw(t, c.ID, userID, `{"title": "C renamed"}`)
		AssertStatusCode(t, code, http.StatusOK)
		if page.ParentID == nil || *page.ParentID != b.ID {
			t.Errorf("parent = %v, want %d", page.ParentID, b.ID)
		}
	})

	t.Run("cannot nest under itself or a descendant", func(t *testing.T) {
		ts, userID, a, _, c := setup(t)
		defer ts.Close()
		tests := []struct {
			name   string
			page   int64
			parent int64
		}{
			{"self", a.ID, a.ID},
			{"descendant", a.ID, c.ID},
		}
		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				_, code, errResp := ts.patchPageRaw(t, tt.page, userID, fmt.Sprintf(`{"parent_id": %d}`, tt.parent))
				if code != http.StatusBadRequest || errResp.Code != "invalid_parent" {
					t.Errorf("got %d %q, want 400 invalid_parent", code, errResp.Code)
				}
			})
		}
	})

	t.Run("moving a subtree cannot exceed six levels", func(t *testing.T) {
		ts, userID, a, _, _ := setup(t)
		defer ts.Close()
		// Build a separate 4-deep chain: D1 > D2 > D3 > D4
		var parent *int64
		var d1 WikiPageResponse
		for i := 1; i <= 4; i++ {
			p, _ := ts.createPageViaHandler(t, a.ProjectID, userID, fmt.Sprintf("D%d", i), parent)
			if i == 1 {
				d1 = p
			}
			parent = int64Ptr(p.ID)
		}
		// A > B > C is 3 deep; placing a 4-high subtree under C would be 7 levels.
		var c WikiPageResponse
		pages := ts.listPages(t, a.ProjectID, userID)
		for _, p := range pages {
			if p.Title == "C" {
				c = p
			}
		}
		_, code, errResp := ts.patchPageRaw(t, d1.ID, userID, fmt.Sprintf(`{"parent_id": %d}`, c.ID))
		if code != http.StatusBadRequest || errResp.Code != "max_depth_exceeded" {
			t.Errorf("got %d %q, want 400 max_depth_exceeded", code, errResp.Code)
		}
		// Under B (depth 2) it fits exactly: 2 + 4 = 6.
		var b WikiPageResponse
		for _, p := range pages {
			if p.Title == "B" {
				b = p
			}
		}
		_, code, _ = ts.patchPageRaw(t, d1.ID, userID, fmt.Sprintf(`{"parent_id": %d}`, b.ID))
		AssertStatusCode(t, code, http.StatusOK)
	})

	t.Run("negative position is rejected", func(t *testing.T) {
		ts, userID, a, _, _ := setup(t)
		defer ts.Close()
		_, code, errResp := ts.patchPageRaw(t, a.ID, userID, `{"position": -1}`)
		if code != http.StatusBadRequest || errResp.Code != "invalid_input" {
			t.Errorf("got %d %q, want 400 invalid_input", code, errResp.Code)
		}
	})
}

func (ts *TestServer) listPages(t testing.TB, projectID, userID int64) []WikiPageResponse {
	t.Helper()
	rec, req := ts.MakeAuthRequest(t, http.MethodGet,
		fmt.Sprintf("/api/projects/%d/wiki/pages", projectID), nil, userID,
		map[string]string{"projectId": fmt.Sprintf("%d", projectID)})
	ts.HandleListWikiPages(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)
	var pages []WikiPageResponse
	DecodeJSON(t, rec, &pages)
	return pages
}

func TestWikiHierarchyDeleteReparentsChildren(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	a, _ := ts.createPageViaHandler(t, projectID, userID, "A", nil)
	b, _ := ts.createPageViaHandler(t, projectID, userID, "B", int64Ptr(a.ID))
	c, _ := ts.createPageViaHandler(t, projectID, userID, "C", int64Ptr(b.ID))

	rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
		fmt.Sprintf("/api/wiki/pages/%d", b.ID), nil, userID,
		map[string]string{"pageId": fmt.Sprintf("%d", b.ID)})
	ts.HandleDeleteWikiPage(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusNoContent)

	pages := ts.listPages(t, projectID, userID)
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages after delete, got %d", len(pages))
	}
	for _, p := range pages {
		if p.ID == c.ID {
			if p.ParentID == nil || *p.ParentID != a.ID {
				t.Errorf("C should be re-parented to A, got parent %v", p.ParentID)
			}
		}
	}
}
