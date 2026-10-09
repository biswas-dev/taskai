//go:build postgres

// Wiki indexing against a real PostgreSQL (row locks, timestamptz, the
// generated tsvector column and FTS search). Opt-in:
//
//	TEST_POSTGRES_DSN=postgres://... go test -tags postgres -run Postgres ./internal/api/
package api

import (
	"context"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"taskai/internal/config"
	"taskai/internal/db"
)

func TestPostgresWikiIndexing(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	ctx := context.Background()
	database, err := db.New(db.Config{Driver: "postgres", DSN: dsn, MigrationsPath: "./../../internal/db/migrations"}, zaptest.NewLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s := NewServer(database, &config.Config{JWTSecret: "test-secret-key", JWTExpiryHours: 24}, zaptest.NewLogger(t))
	ts := &TestServer{Server: s, DB: database}

	var userID, projectID, pageID int64
	must := func(err error) {
		t.Helper()
		if err != nil {
			t.Fatal(err)
		}
	}
	must(database.QueryRowContext(ctx, `INSERT INTO users (email, password_hash) VALUES ('pg-index@example.com', 'x') RETURNING id`).Scan(&userID))
	must(database.QueryRowContext(ctx, `INSERT INTO projects (owner_id, name) VALUES ($1, 'Docs') RETURNING id`, userID).Scan(&projectID))
	_, err = database.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role, granted_by) VALUES ($1, $2, 'owner', $2)`, projectID, userID)
	must(err)
	must(database.QueryRowContext(ctx, `INSERT INTO wiki_pages (project_id, title, slug, created_by) VALUES ($1, 'Deploy guide', 'deploy-guide', $2) RETURNING id`, projectID, userID).Scan(&pageID))

	save := func(content string) {
		rec, req := ts.MakeAuthRequest(t, http.MethodPut, "/api/wiki/pages/"+strconv.FormatInt(pageID, 10)+"/content",
			map[string]any{"content": content, "manual_save": true}, userID, map[string]string{"pageId": strconv.FormatInt(pageID, 10)})
		s.HandleUpdateWikiPageContent(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusOK)
	}
	search := func(q, mode string) SearchWikiResponse {
		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", SearchWikiRequest{Query: q, Mode: mode}, userID, nil)
		s.HandleSearchWiki(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusOK)
		var resp SearchWikiResponse
		DecodeJSON(t, rec, &resp)
		return resp
	}
	pending := func() int {
		pages, err := s.pagesNeedingIndex(ctx, 100)
		must(err)
		return len(pages)
	}

	save("# Deploy\n\nRun the zerodowntime rollout.\n\n## Rollback\n\nRestore the previous image tag.")
	if pending() != 1 {
		t.Fatal("saved page should be pending")
	}
	s.indexPages(ctx)
	if n := pending(); n != 0 {
		t.Fatalf("%d pending after indexing", n)
	}
	for _, mode := range []string{"keyword", "fts"} {
		if r := search("zerodowntime", mode); r.Total != 1 || r.Results[0].HeadingsPath != "Deploy" {
			t.Fatalf("%s search: %+v", mode, r)
		}
	}
	if r := search("restore previous image", "fts"); r.Total != 1 || r.Results[0].HeadingsPath != "Deploy > Rollback" {
		t.Fatalf("fts across words: %+v", r)
	}

	// An edit after indexing is picked up; so is a backlog of old edits.
	time.Sleep(10 * time.Millisecond)
	save("# Deploy\n\nUse the bluegreen switch.")
	if pending() != 1 {
		t.Fatal("edit should be pending")
	}
	_, err = database.ExecContext(ctx, `UPDATE wiki_pages SET updated_at = NOW() - INTERVAL '2 hours', search_indexed_at = NULL`)
	must(err)
	s.indexPages(ctx)
	if pending() != 0 || search("zerodowntime", "fts").Total != 0 || search("bluegreen", "fts").Total != 1 {
		t.Fatal("re-index after an old edit")
	}
}
