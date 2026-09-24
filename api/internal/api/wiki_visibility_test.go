package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
)

// withURLParams attaches chi URL params to an unauthenticated request.
func withURLParams(req *http.Request, params map[string]string) *http.Request {
	rctx := chi.NewRouteContext()
	for k, v := range params {
		rctx.URLParams.Add(k, v)
	}
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}

// wikiVisibilityFixture: owner owns the project; alice wrote a restricted
// "Secret Plan" page shared with bob; carol is a project member it isn't
// shared with; dave is not in the project at all.
type wikiVisibilityFixture struct {
	owner, alice, bob, carol, dave int64
	projectID, openPage, secret    int64
}

func newWikiVisibilityFixture(t *testing.T, ts *TestServer) wikiVisibilityFixture {
	t.Helper()
	var f wikiVisibilityFixture
	f.owner = ts.CreateTestUser(t, "owner@example.com", "password123")
	f.alice = ts.CreateTestUser(t, "alice@example.com", "password123")
	f.bob = ts.CreateTestUser(t, "bob@example.com", "password123")
	f.carol = ts.CreateTestUser(t, "carol@example.com", "password123")
	f.dave = ts.CreateTestUser(t, "dave@example.com", "password123")

	f.projectID = ts.CreateTestProject(t, f.owner, "Docs")
	for _, uid := range []int64{f.alice, f.bob, f.carol} {
		ts.AddProjectMember(t, f.projectID, uid, f.owner, "member")
	}

	ctx := context.Background()
	insert := func(title, slug string, createdBy int64, visibility string) int64 {
		res, err := ts.DB.ExecContext(ctx,
			`INSERT INTO wiki_pages (project_id, title, slug, created_by, visibility, content) VALUES (?, ?, ?, ?, ?, ?)`,
			f.projectID, title, slug, createdBy, visibility, "# "+title+"\n\n<img src=x onerror=alert(1)>")
		if err != nil {
			t.Fatal(err)
		}
		id, _ := res.LastInsertId()
		return id
	}
	f.openPage = insert("Onboarding Guide", "onboarding", f.alice, "project")
	f.secret = insert("Secret Plan", "secret-plan", f.alice, "restricted")
	if _, err := ts.DB.ExecContext(ctx, `INSERT INTO wiki_page_shares (page_id, user_id) VALUES (?, ?)`, f.secret, f.bob); err != nil {
		t.Fatal(err)
	}
	return f
}

func pageParam(id int64) map[string]string {
	return map[string]string{"pageId": fmt.Sprintf("%d", id)}
}

func TestWikiVisibility_GetPage(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newWikiVisibilityFixture(t, ts)

	tests := []struct {
		name       string
		userID     int64
		pageID     int64
		wantStatus int
	}{
		{"creator reads restricted page", f.alice, f.secret, http.StatusOK},
		{"shared user reads restricted page", f.bob, f.secret, http.StatusOK},
		{"project owner reads restricted page", f.owner, f.secret, http.StatusOK},
		{"member it isn't shared with is denied", f.carol, f.secret, http.StatusForbidden},
		{"non-member is denied", f.dave, f.secret, http.StatusForbidden},
		{"member reads project-visible page", f.carol, f.openPage, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/", nil, tt.userID, pageParam(tt.pageID))
			ts.HandleGetWikiPage(rec, req)
			AssertStatusCode(t, rec.Code, tt.wantStatus)

			rec, req = ts.MakeAuthRequest(t, http.MethodGet, "/", nil, tt.userID, pageParam(tt.pageID))
			ts.HandleGetWikiPageContent(rec, req)
			AssertStatusCode(t, rec.Code, tt.wantStatus)

			rec, req = ts.MakeAuthRequest(t, http.MethodGet, "/", nil, tt.userID, pageParam(tt.pageID))
			ts.HandleListWikiAnnotations(rec, req)
			AssertStatusCode(t, rec.Code, tt.wantStatus)
		})
	}
}

func TestWikiVisibility_ListAndAutocomplete(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newWikiVisibilityFixture(t, ts)

	tests := []struct {
		name       string
		userID     int64
		seesSecret bool
	}{
		{"carol", f.carol, false},
		{"bob", f.bob, true},
		{"owner", f.owner, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/", nil, tt.userID,
				map[string]string{"projectId": fmt.Sprintf("%d", f.projectID)})
			ts.HandleListWikiPages(rec, req)
			AssertStatusCode(t, rec.Code, http.StatusOK)
			var pages []WikiPageResponse
			DecodeJSON(t, rec, &pages)
			found := false
			for _, p := range pages {
				if p.ID == f.secret {
					found = true
					if p.Visibility != wikiVisibilityRestricted {
						t.Errorf("visibility = %q", p.Visibility)
					}
				}
			}
			if found != tt.seesSecret {
				t.Errorf("list contains secret = %v, want %v", found, tt.seesSecret)
			}

			rec, req = ts.MakeAuthRequest(t, http.MethodGet, "/api/wiki/autocomplete?query=Secret", nil, tt.userID, nil)
			ts.HandleAutocompletePages(rec, req)
			AssertStatusCode(t, rec.Code, http.StatusOK)
			var results []AutocompletePageResult
			DecodeJSON(t, rec, &results)
			if (len(results) == 1) != tt.seesSecret {
				t.Errorf("autocomplete results = %+v, want secret visible = %v", results, tt.seesSecret)
			}
		})
	}
}

func TestWikiVisibility_UpdateSharing(t *testing.T) {
	tests := []struct {
		name       string
		caller     func(f wikiVisibilityFixture) int64
		visibility string
		userIDs    func(f wikiVisibilityFixture) []int64
		wantStatus int
	}{
		{"creator shares with carol", func(f wikiVisibilityFixture) int64 { return f.alice }, "restricted",
			func(f wikiVisibilityFixture) []int64 { return []int64{f.bob, f.carol} }, http.StatusOK},
		{"project owner can change it", func(f wikiVisibilityFixture) int64 { return f.owner }, "project",
			func(f wikiVisibilityFixture) []int64 { return nil }, http.StatusOK},
		{"shared user cannot change sharing", func(f wikiVisibilityFixture) int64 { return f.bob }, "project",
			func(f wikiVisibilityFixture) []int64 { return nil }, http.StatusForbidden},
		{"cannot share outside the project", func(f wikiVisibilityFixture) int64 { return f.alice }, "restricted",
			func(f wikiVisibilityFixture) []int64 { return []int64{f.dave} }, http.StatusBadRequest},
		{"invalid visibility", func(f wikiVisibilityFixture) int64 { return f.alice }, "everyone",
			func(f wikiVisibilityFixture) []int64 { return nil }, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()
			f := newWikiVisibilityFixture(t, ts)

			body := UpdateWikiSharingRequest{Visibility: tt.visibility, UserIDs: tt.userIDs(f)}
			rec, req := ts.MakeAuthRequest(t, http.MethodPut, "/", body, tt.caller(f), pageParam(f.secret))
			ts.HandleUpdateWikiSharing(rec, req)
			AssertStatusCode(t, rec.Code, tt.wantStatus)
			if tt.wantStatus != http.StatusOK {
				return
			}
			var resp WikiSharingResponse
			DecodeJSON(t, rec, &resp)
			if resp.Visibility != tt.visibility || len(resp.SharedWith) != len(body.UserIDs) {
				t.Errorf("unexpected sharing %+v", resp)
			}

			// Carol can read the page afterwards in both successful cases.
			rec, req = ts.MakeAuthRequest(t, http.MethodGet, "/", nil, f.carol, pageParam(f.secret))
			ts.HandleGetWikiPage(rec, req)
			AssertStatusCode(t, rec.Code, http.StatusOK)
		})
	}
}

func TestWikiVisibility_ChildPagesInheritRestriction(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newWikiVisibilityFixture(t, ts)
	projectParam := map[string]string{"projectId": fmt.Sprintf("%d", f.projectID)}

	// Carol can't see the secret page, so she can't nest pages under it.
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/", CreateWikiPageRequest{Title: "Sneaky", ParentID: &f.secret}, f.carol, projectParam)
	ts.HandleCreateWikiPage(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusBadRequest)

	// Bob can; the child is restricted and alice (the parent's author) keeps access.
	rec, req = ts.MakeAuthRequest(t, http.MethodPost, "/", CreateWikiPageRequest{Title: "Secret Details", ParentID: &f.secret}, f.bob, projectParam)
	ts.HandleCreateWikiPage(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusCreated)
	var child WikiPageResponse
	DecodeJSON(t, rec, &child)
	if child.Visibility != wikiVisibilityRestricted {
		t.Fatalf("child visibility = %q, want restricted", child.Visibility)
	}

	for _, c := range []struct {
		userID int64
		want   int
	}{{f.alice, http.StatusOK}, {f.bob, http.StatusOK}, {f.owner, http.StatusOK}, {f.carol, http.StatusForbidden}} {
		rec, req = ts.MakeAuthRequest(t, http.MethodGet, "/", nil, c.userID, pageParam(child.ID))
		ts.HandleGetWikiPage(rec, req)
		AssertStatusCode(t, rec.Code, c.want)
	}
}

func TestWikiVisibility_PublicLink(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newWikiVisibilityFixture(t, ts)

	// Only managers can create the link.
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/", nil, f.bob, pageParam(f.secret))
	ts.HandleCreateWikiPublicLink(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusForbidden)

	rec, req = ts.MakeAuthRequest(t, http.MethodPost, "/", nil, f.alice, pageParam(f.secret))
	ts.HandleCreateWikiPublicLink(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)
	var link map[string]string
	DecodeJSON(t, rec, &link)
	token := link["public_token"]
	if len(token) < 20 {
		t.Fatalf("token too short: %q", token)
	}

	// The token is shown to managers only.
	rec, req = ts.MakeAuthRequest(t, http.MethodGet, "/", nil, f.bob, pageParam(f.secret))
	ts.HandleGetWikiSharing(rec, req)
	var bobView WikiSharingResponse
	DecodeJSON(t, rec, &bobView)
	if bobView.PublicToken != nil || bobView.CanManage {
		t.Errorf("bob should not see the token or manage: %+v", bobView)
	}

	// Anyone can read it, sanitized.
	rec, req = MakeRequest(t, http.MethodGet, "/", nil, nil)
	req = withURLParams(req, map[string]string{"token": token})
	ts.HandleGetPublicWikiPage(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)
	var pub PublicWikiPageResponse
	DecodeJSON(t, rec, &pub)
	if pub.Title != "Secret Plan" || pub.ProjectName != "Docs" || strings.Contains(pub.HTML, "onerror") {
		t.Errorf("unexpected public page %+v", pub)
	}

	// Revoking kills the link.
	rec, req = ts.MakeAuthRequest(t, http.MethodDelete, "/", nil, f.owner, pageParam(f.secret))
	ts.HandleDeleteWikiPublicLink(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	rec, req = MakeRequest(t, http.MethodGet, "/", nil, nil)
	req = withURLParams(req, map[string]string{"token": token})
	ts.HandleGetPublicWikiPage(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusNotFound)
}

func TestWikiVisibility_AnnotationCommentNeedsPageAccess(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newWikiVisibilityFixture(t, ts)
	ctx := context.Background()

	// Carol commented while the page was open to the project...
	res, err := ts.DB.ExecContext(ctx,
		`INSERT INTO wiki_annotations (wiki_page_id, author_id, start_offset, end_offset, selected_text, color) VALUES (?, ?, 0, 5, 'Secre', 'yellow')`,
		f.secret, f.carol)
	if err != nil {
		t.Fatal(err)
	}
	annotationID, _ := res.LastInsertId()
	res, err = ts.DB.ExecContext(ctx,
		`INSERT INTO wiki_annotation_comments (annotation_id, author_id, content) VALUES (?, ?, 'note')`,
		annotationID, f.carol)
	if err != nil {
		t.Fatal(err)
	}
	commentID, _ := res.LastInsertId()

	// ...but now that it's restricted she can no longer edit it.
	content := "edited"
	rec, req := ts.MakeAuthRequest(t, http.MethodPatch, "/", map[string]*string{"content": &content}, f.carol,
		map[string]string{"commentId": fmt.Sprintf("%d", commentID)})
	ts.HandleUpdateAnnotationComment(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusForbidden)
}
