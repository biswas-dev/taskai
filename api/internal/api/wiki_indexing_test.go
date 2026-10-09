package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
	"time"
)

// saveWikiContent saves content through the real handler, which also writes a
// compressed version snapshot — the path that left indexing with nothing.
func saveWikiContent(t *testing.T, ts *TestServer, userID, pageID int64, content string) {
	t.Helper()
	rec, req := ts.MakeAuthRequest(t, http.MethodPut, "/api/wiki/pages/"+strconv.FormatInt(pageID, 10)+"/content",
		map[string]any{"content": content, "manual_save": true}, userID, map[string]string{"pageId": strconv.FormatInt(pageID, 10)})
	ts.HandleUpdateWikiPageContent(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)
}

func searchWiki(t *testing.T, ts *TestServer, userID int64, query string) SearchWikiResponse {
	t.Helper()
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", SearchWikiRequest{Query: query, Mode: "keyword"}, userID, nil)
	ts.HandleSearchWiki(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)
	var resp SearchWikiResponse
	DecodeJSON(t, rec, &resp)
	return resp
}

func pendingIndex(t *testing.T, ts *TestServer) int {
	t.Helper()
	pages, err := ts.pagesNeedingIndex(context.Background(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	return len(pages)
}

func TestWikiIndexingMakesSavedContentSearchable(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	ctx := context.Background()
	userID := ts.CreateTestUser(t, "indexer@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Docs")
	pageID := ts.createTestWikiPage(t, projectID, userID, "Deploy guide")

	saveWikiContent(t, ts, userID, pageID, "# Deploy\n\nRun the zerodowntime script.\n\n## Rollback\n\nUse the previous image tag.\n\n```bash\n# not a heading\necho hi\n```")
	if pendingIndex(t, ts) == 0 {
		t.Fatal("a saved page should need indexing")
	}
	ts.indexPages(ctx)
	if n := pendingIndex(t, ts); n != 0 {
		t.Fatalf("%d pages still pending after a pass", n)
	}

	resp := searchWiki(t, ts, userID, "zerodowntime")
	if resp.Total != 1 || resp.Results[0].PageID != pageID || resp.Results[0].HeadingsPath != "Deploy" {
		t.Fatalf("saved content not searchable: %+v", resp)
	}
	resp = searchWiki(t, ts, userID, "previous image tag")
	if resp.Total != 1 || resp.Results[0].HeadingsPath != "Deploy > Rollback" || !strings.Contains(resp.Results[0].Snippet, "# not a heading") {
		t.Fatalf("nested headings and fenced code: %+v", resp)
	}

	// An edit replaces the old blocks.
	saveWikiContent(t, ts, userID, pageID, "# Deploy\n\nUse the blue-green switch.")
	ts.indexPages(ctx)
	if searchWiki(t, ts, userID, "zerodowntime").Total != 0 || searchWiki(t, ts, userID, "blue-green").Total != 1 {
		t.Fatal("edit was not re-indexed")
	}
}

func TestWikiIndexingCatchesUpOnOldEdits(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	ctx := context.Background()
	userID := ts.CreateTestUser(t, "backlog@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Docs")
	var ids []int64
	for i := 0; i < indexBatch+5; i++ {
		id := ts.createTestWikiPageWithContent(t, projectID, userID, "Page "+strconv.Itoa(i), "backlogmarker page "+strconv.Itoa(i))
		ids = append(ids, id)
	}
	// Edited an hour ago — e.g. while the server was deploying. The old
	// worker only looked at the last three minutes and never indexed these.
	if _, err := ts.DB.ExecContext(ctx, `UPDATE wiki_pages SET updated_at = ?`, time.Now().Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	ts.indexPages(ctx)
	if n := pendingIndex(t, ts); n != 0 {
		t.Fatalf("%d pages left after one pass over a backlog larger than a batch", n)
	}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/search", SearchWikiRequest{Query: "backlogmarker", Mode: "keyword", Limit: 100}, userID, nil)
	ts.HandleSearchWiki(rec, req)
	var resp SearchWikiResponse
	DecodeJSON(t, rec, &resp)
	if resp.Total != len(ids) {
		t.Fatalf("found %d of %d backlog pages", resp.Total, len(ids))
	}
}

func TestWikiReindexWorksWithoutEmbeddings(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	userID := ts.CreateTestUser(t, "reindex@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Docs")
	pageID := ts.createTestWikiPageWithContent(t, projectID, userID, "Runbook", "reindexmarker steps")
	ts.indexPages(context.Background())
	// Lose the blocks, as the broken indexer did.
	if _, err := ts.DB.ExecContext(context.Background(), `DELETE FROM wiki_blocks WHERE page_id = ?`, pageID); err != nil {
		t.Fatal(err)
	}
	if searchWiki(t, ts, userID, "reindexmarker").Total != 0 {
		t.Fatal("setup: blocks should be gone")
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/wiki/reindex", nil, userID, nil)
	ts.HandleReindexWiki(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusAccepted)
	deadline := time.Now().Add(5 * time.Second)
	for searchWiki(t, ts, userID, "reindexmarker").Total == 0 {
		if time.Now().After(deadline) {
			t.Fatal("reindex without embeddings did not restore search")
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func TestMarkdownBlocks(t *testing.T) {
	blocks := markdownBlocks("Intro line\n\n# A\ntext a\n### C\ntext c\n## B\ntext b\n#hashtag stays text\n")
	want := []struct{ path, text string }{{"", "Intro line"}, {"A", "text a"}, {"A > C", "text c"}, {"A > B", "text b\n#hashtag stays text"}}
	if len(blocks) != len(want) {
		t.Fatalf("blocks: %+v", blocks)
	}
	for i, w := range want {
		if blocks[i].HeadingsPath != w.path || blocks[i].PlainText != w.text || blocks[i].Position != i {
			t.Errorf("block %d = %+v, want %+v", i, blocks[i], w)
		}
	}
	if blocks[0].Type != "paragraph" || blocks[1].Type != "section" {
		t.Errorf("types: %s %s", blocks[0].Type, blocks[1].Type)
	}
}
