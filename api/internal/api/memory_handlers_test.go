package api

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"

	"taskai/internal/config"
	"taskai/internal/db"
	"taskai/internal/embeddings"
)

func TestNormalizeMemory(t *testing.T) {
	now := time.Date(2026, 11, 23, 0, 0, 0, 0, time.UTC)
	ok := MemoryWriteRequest{Kind: "gotcha", Title: "t", Body: "b", Scope: MemoryScope{Repo: "biswas-dev/taskai"}}
	with := func(f func(*MemoryWriteRequest)) MemoryWriteRequest { r := ok; f(&r); return r }
	tests := []struct {
		name    string
		req     MemoryWriteRequest
		wantErr string
	}{
		{"valid", ok, ""},
		{"title from body", with(func(r *MemoryWriteRequest) { r.Title, r.Body = "", "first line\nmore" }), ""},
		{"bad kind", with(func(r *MemoryWriteRequest) { r.Kind = "note" }), "kind must be"},
		{"no body", with(func(r *MemoryWriteRequest) { r.Body = " " }), "body is required"},
		{"body too long", with(func(r *MemoryWriteRequest) { r.Body = strings.Repeat("x", memoryMaxBody+1) }), "keep it under"},
		{"no scope", with(func(r *MemoryWriteRequest) { r.Scope = MemoryScope{} }), "repo or a folder"},
		{"bad repo", with(func(r *MemoryWriteRequest) { r.Scope.Repo = "taskai" }), "owner/name"},
		{"relative folder", with(func(r *MemoryWriteRequest) { r.Scope = MemoryScope{Folder: "src"} }), "absolute path"},
		{"issue without repo", with(func(r *MemoryWriteRequest) { r.Scope = MemoryScope{Folder: "/x", Issue: 3} }), "need scope.repo"},
		{"bad confidence", with(func(r *MemoryWriteRequest) { r.Confidence = "sure" }), "confidence"},
		{"bad status", with(func(r *MemoryWriteRequest) { r.Status = "compacted" }), "status must be"},
		{"origin id without host", with(func(r *MemoryWriteRequest) { r.OriginID = "7" }), "needs origin_host"},
		{"multi-line file", with(func(r *MemoryWriteRequest) { r.Scope.Files = []string{"a\nb"} }), "one line"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m, msg := normalizeMemory(tt.req, now)
			if tt.wantErr == "" && msg != "" || tt.wantErr != "" && !strings.Contains(msg, tt.wantErr) {
				t.Fatalf("msg = %q, want %q", msg, tt.wantErr)
			}
			if msg == "" && (m.hash == "" || m.SizeBytes <= memoryEmbeddingBytes) {
				t.Fatalf("hash %q size %d", m.hash, m.SizeBytes)
			}
		})
	}
	a, _ := normalizeMemory(ok, now)
	b, _ := normalizeMemory(with(func(r *MemoryWriteRequest) { r.Title, r.Body = "  T ", "B" }), now)
	if a.hash != b.hash {
		t.Error("hash should ignore case and spacing")
	}
}

func TestParseMemoryPage(t *testing.T) {
	content := "**gotcha** · repo `biswas-dev/taskai` · issue #12 · `api/x.go` · confidence high · expires 2027-01-02\n\n" +
		"Body line one.\n\nBody --- line two.\n\n---\n\n" +
		"_2026-10-01 · by claude-code / claude on mbp · session `abc` · replaces memory 4 · llm-brain memory 9_\n"
	req, superseded, err := parseMemoryPage("[gotcha] biswas-dev/taskai#12: Partial index needs WHERE", content)
	if err != nil || superseded {
		t.Fatalf("err %v superseded %v", err, superseded)
	}
	want := MemoryWriteRequest{Kind: "gotcha", Title: "Partial index needs WHERE", Body: "Body line one.\n\nBody --- line two.",
		Scope:  MemoryScope{Repo: "biswas-dev/taskai", Issue: 12, Files: []string{"api/x.go"}},
		Author: MemoryAuthor{Agent: "claude-code", Provider: "claude", Host: "mbp", Session: "abc"}, Confidence: "high",
		SupersedesOriginID: "4", OriginID: "9"}
	got := req
	got.ExpiresAt, got.CreatedAt = nil, nil
	if g, w := fmt.Sprintf("%+v", got), fmt.Sprintf("%+v", want); g != w {
		t.Fatalf("got  %s\nwant %s", g, w)
	}
	if req.CreatedAt == nil || req.CreatedAt.Format("2006-01-02") != "2026-10-01" || req.ExpiresAt == nil {
		t.Fatalf("dates %v %v", req.CreatedAt, req.ExpiresAt)
	}

	if _, sup, err := parseMemoryPage("[superseded] [fact] /x: t", "> Superseded by memory 3.\n\n**fact** · folder `/x` · confidence medium\n\nb\n\n---\n\n_2026-01-01 on h · llm-brain memory 2_"); err != nil || !sup {
		t.Fatalf("superseded page: %v %v", sup, err)
	}
	if req, _, err := parseMemoryPage("[fact] o/r: t", "**fact** · repo `o/r` · issue # · PR #x\n\nb\n\n---\n\n_2026-01-01_"); err != nil || req.Scope.Issue != 0 || req.Scope.PR != 0 {
		t.Fatalf("malformed issue/PR must not panic or parse: %+v %v", req.Scope, err)
	}
	if _, _, err := parseMemoryPage("Meeting notes", "hello"); err == nil {
		t.Fatal("a non-memory page should not parse")
	}
}

func TestMemoryHandlers(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	runMemoryTests(t, ts)
}

func TestPostgresMemoryHandlers(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	database, err := db.New(db.Config{Driver: "postgres", DSN: dsn, MigrationsPath: "./../../internal/db/migrations"}, zaptest.NewLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s := NewServer(database, &config.Config{JWTSecret: "test-secret-key", JWTExpiryHours: 24}, zaptest.NewLogger(t))
	// A bag-of-words embedder: texts sharing words point the same way, enough to exercise the vector path.
	fake := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct{ Input []string }
		_ = json.NewDecoder(r.Body).Decode(&in)
		out := make([][]float32, len(in.Input))
		for i, text := range in.Input {
			v := make([]float32, 384)
			v[0] = 0.01
			for _, w := range memoryQueryWords(text) {
				h := fnv.New32a()
				h.Write([]byte(w))
				v[h.Sum32()%384]++
			}
			out[i] = v
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"embeddings": out})
	}))
	defer fake.Close()
	s.SetEmbeddingClient(embeddings.NewClient(fake.URL, "fake", zaptest.NewLogger(t)))
	runMemoryTests(t, &TestServer{Server: s, DB: database})
}

func runMemoryTests(t *testing.T, ts *TestServer) {
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	user := func(name string) int64 {
		var id int64
		if err := ts.DB.QueryRowContext(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, 'x') RETURNING id`,
			name+suffix+"@example.com").Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	owner, friend, outsider, admin := user("owner"), user("friend"), user("outsider"), user("admin")
	if _, err := ts.DB.ExecContext(ctx, `UPDATE users SET is_admin = true WHERE id = $1`, admin); err != nil {
		t.Fatal(err)
	}

	write := func(u int64, req MemoryWriteRequest) (int, MemoryWriteResponse, string) {
		t.Helper()
		rec, r := ts.MakeAuthRequest(t, http.MethodPost, "/api/memories", req, u, nil)
		ts.HandleWriteMemory(rec, r)
		var resp MemoryWriteResponse
		if rec.Code < 300 {
			DecodeJSON(t, rec, &resp)
		}
		return rec.Code, resp, rec.Body.String()
	}
	mem := func(title, body string) MemoryWriteRequest {
		return MemoryWriteRequest{Kind: "gotcha", Title: title, Body: body, Scope: MemoryScope{Repo: "biswas-dev/taskai", Files: []string{"api/db.go"}},
			Author: MemoryAuthor{Agent: "claude-code", Provider: "claude", Host: "mbp"}}
	}
	get := func(u, id int64) (int, Memory) {
		sid := fmt.Sprint(id)
		rec, r := ts.MakeAuthRequest(t, http.MethodGet, "/api/memories/"+sid, nil, u, map[string]string{"id": sid})
		ts.HandleGetMemory(rec, r)
		var m Memory
		if rec.Code == http.StatusOK {
			DecodeJSON(t, rec, &m)
		}
		return rec.Code, m
	}
	search := func(u int64, req MemorySearchRequest) MemorySearchResponse {
		t.Helper()
		rec, r := ts.MakeAuthRequest(t, http.MethodPost, "/api/memories/search", req, u, nil)
		ts.HandleSearchMemories(rec, r)
		AssertStatusCode(t, rec.Code, http.StatusOK)
		var resp MemorySearchResponse
		DecodeJSON(t, rec, &resp)
		return resp
	}
	compact := func(u int64) MemoryCompactReport {
		t.Helper()
		rec, r := ts.MakeAuthRequest(t, http.MethodPost, "/api/memories/compact", nil, u, nil)
		ts.HandleCompactMemories(rec, r)
		AssertStatusCode(t, rec.Code, http.StatusOK)
		var rep MemoryCompactReport
		DecodeJSON(t, rec, &rep)
		return rep
	}
	count := func(u int64) (all, active int) {
		t.Helper()
		if err := ts.DB.QueryRowContext(ctx, `SELECT COUNT(*), COALESCE(SUM(CASE WHEN status = 'active' THEN 1 ELSE 0 END), 0)
			FROM agent_memories WHERE owner_id = $1`, u).Scan(&all, &active); err != nil {
			t.Fatal(err)
		}
		return all, active
	}

	t.Run("idempotent upsert by origin and by content", func(t *testing.T) {
		req := mem("Partial unique index", "ON CONFLICT must repeat the index WHERE clause")
		req.OriginHost, req.OriginID = "hub-a", "1"
		code, first, _ := write(owner, req)
		if code != http.StatusCreated || first.Outcome != "created" || first.Usage.UsedBytes == 0 {
			t.Fatalf("first write %d %+v", code, first)
		}
		code, again, _ := write(owner, req)
		if code != http.StatusOK || again.Outcome != "duplicate" || again.Memory.ID != first.Memory.ID {
			t.Fatalf("repeat %d %+v", code, again)
		}
		noOrigin := mem("partial  UNIQUE index", "on conflict must repeat the index where clause")
		if _, dup, _ := write(owner, noOrigin); dup.Outcome != "duplicate" || dup.Memory.ID != first.Memory.ID {
			t.Fatalf("same content without origin %+v", dup)
		}
		req.Body += " (or Postgres errors)"
		if _, upd, _ := write(owner, req); upd.Outcome != "updated" || upd.Memory.ID != first.Memory.ID || !strings.Contains(upd.Memory.Body, "Postgres") {
			t.Fatalf("changed body %+v", upd)
		}
		if all, _ := count(owner); all != 1 {
			t.Fatalf("%d rows, want 1", all)
		}
	})

	t.Run("supersede by id and by origin arriving out of order", func(t *testing.T) {
		_, a, _ := write(owner, mem("Deploy via script", "Use script/server deploy"))
		b := mem("Deploy via workflow", "Deploys go through GitHub Actions now")
		b.Supersedes = a.Memory.ID
		_, bresp, _ := write(owner, b)
		if _, old := get(owner, a.Memory.ID); old.Status != "superseded" || old.SupersededBy == nil || *old.SupersededBy != bresp.Memory.ID {
			t.Fatalf("old after supersede %+v", old)
		}
		if code, _, _ := write(owner, func() MemoryWriteRequest { r := mem("x", "y"); r.Supersedes = 999999; return r }()); code != http.StatusBadRequest {
			t.Fatalf("unknown supersedes: %d", code)
		}

		newer := mem("Staging is ARM64", "Both staging and UAT are aarch64")
		newer.OriginHost, newer.OriginID, newer.SupersedesOriginID = "hub-a", "11", "10"
		_, n, _ := write(owner, newer)
		older := mem("Staging arch", "Staging runs on x86")
		older.OriginHost, older.OriginID = "hub-a", "10"
		_, o, _ := write(owner, older)
		if o.Memory.Status != "superseded" || o.Memory.SupersededBy == nil || *o.Memory.SupersededBy != n.Memory.ID {
			t.Fatalf("late older memory %+v", o.Memory)
		}
		if _, nn := get(owner, n.Memory.ID); nn.Supersedes == nil || *nn.Supersedes != o.Memory.ID {
			t.Fatalf("newer should point at older: %+v", nn)
		}
	})

	t.Run("sharing: read and search, never delete", func(t *testing.T) {
		_, m, _ := write(owner, mem("Lint rewrites files", "make lint runs go fmt on unrelated files"))
		if code, _ := get(friend, m.Memory.ID); code != http.StatusNotFound {
			t.Fatalf("unshared get %d", code)
		}
		rec, r := ts.MakeAuthRequest(t, http.MethodPut, "/api/memories/shares", map[string]any{"user_ids": []int64{friend}}, owner, nil)
		ts.HandleSetMemoryShares(rec, r)
		AssertStatusCode(t, rec.Code, http.StatusOK)
		if code, _ := get(friend, m.Memory.ID); code != http.StatusOK {
			t.Fatalf("shared get %d", code)
		}
		if code, _ := get(outsider, m.Memory.ID); code != http.StatusNotFound {
			t.Fatalf("outsider get %d", code)
		}
		if res := search(friend, MemorySearchRequest{Query: "lint fmt", Repo: "BISWAS-DEV/taskai"}); res.Total == 0 || res.Results[0].ID != m.Memory.ID {
			t.Fatalf("friend search %+v", res)
		}
		if res := search(outsider, MemorySearchRequest{Query: "lint fmt"}); res.Total != 0 {
			t.Fatalf("outsider search found %d", res.Total)
		}
		rec, r = ts.MakeAuthRequest(t, http.MethodPost, "/api/memories/search", MemorySearchRequest{Query: "lint", OwnerID: owner}, outsider, nil)
		ts.HandleSearchMemories(rec, r)
		AssertStatusCode(t, rec.Code, http.StatusForbidden)

		sid := fmt.Sprint(m.Memory.ID)
		rec, r = ts.MakeAuthRequest(t, http.MethodDelete, "/api/memories/"+sid, nil, friend, map[string]string{"id": sid})
		ts.HandleDeleteMemory(rec, r)
		AssertStatusCode(t, rec.Code, http.StatusForbidden)
		rec, r = ts.MakeAuthRequest(t, http.MethodDelete, "/api/memories/"+sid, nil, owner, map[string]string{"id": sid})
		ts.HandleDeleteMemory(rec, r)
		AssertStatusCode(t, rec.Code, http.StatusNoContent)
		if code, _ := get(owner, m.Memory.ID); code != http.StatusNotFound {
			t.Fatalf("deleted memory still there: %d", code)
		}
	})

	t.Run("search filters", func(t *testing.T) {
		d := mem("Rollback restores image tag", "Rollback restores the previous image tag")
		d.Kind, d.Scope = "runbook", MemoryScope{Repo: "biswas-dev/other", Files: []string{"deploy/run_me.sh"}}
		write(owner, d)
		if res := search(owner, MemorySearchRequest{Query: "rollback image", Kind: "runbook"}); res.Total != 1 {
			t.Fatalf("kind filter %+v", res)
		}
		if res := search(owner, MemorySearchRequest{Query: "rollback image", Repo: "biswas-dev/taskai"}); res.Total != 0 {
			t.Fatalf("repo filter %+v", res)
		}
		if res := search(owner, MemorySearchRequest{Query: "rollback", Files: []string{"deploy/run_me.sh"}}); res.Total != 1 {
			t.Fatalf("files filter %+v", res)
		}
		if res := search(owner, MemorySearchRequest{Query: "rollback", Files: []string{"deploy/runXme.sh"}}); res.Total != 0 {
			t.Fatal("underscore in a file filter must not act as a wildcard")
		}
		if ts.DB.Driver == "postgres" {
			ts.embedPendingMemories(ctx)
			var missing int
			if err := ts.DB.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_memories WHERE owner_id = $1 AND status = 'active' AND embedding IS NULL`, owner).Scan(&missing); err != nil || missing != 0 {
				t.Fatalf("%d active memories without embeddings (%v)", missing, err)
			}
			if res := search(owner, MemorySearchRequest{Query: "how do I roll back the image"}); res.Mode != "hybrid" || res.Total == 0 || !strings.Contains(res.Results[0].Title, "Rollback") {
				t.Fatalf("hybrid search %+v", res)
			}
		}
	})

	t.Run("quota: admin only, warn at high water, refuse past it", func(t *testing.T) {
		u := user("quota")
		setQuota := func(caller int64, quota any) int {
			sid := fmt.Sprint(u)
			rec, r := ts.MakeAuthRequest(t, http.MethodPut, "/api/admin/users/"+sid+"/memory-quota", map[string]any{"quota_bytes": quota}, caller, map[string]string{"id": sid})
			ts.HandleAdminSetMemoryQuota(rec, r)
			return rec.Code
		}
		if code := setQuota(u, 10); code != http.StatusForbidden {
			t.Fatalf("non-admin set quota: %d", code)
		}
		_, first, _ := write(u, mem("one", "first memory"))
		if first.Usage.QuotaBytes != memoryDefaultQuota || first.Usage.Warning != "" {
			t.Fatalf("default usage %+v", first.Usage)
		}
		if code := setQuota(admin, first.Usage.UsedBytes*2+10); code != http.StatusOK {
			t.Fatalf("admin set quota: %d", code)
		}
		_, second, _ := write(u, mem("two", "second memory"))
		if second.Usage.Warning == "" || second.Usage.Percent < memoryHighWater {
			t.Fatalf("expected a high-water warning: %+v", second.Usage)
		}
		code, _, body := write(u, mem("three", "third memory, over quota"))
		if code != http.StatusRequestEntityTooLarge || !strings.Contains(body, "memory_quota_exceeded") {
			t.Fatalf("over quota: %d %s", code, body)
		}
		if all, _ := count(u); all != 2 {
			t.Fatalf("refused write stored something: %d rows", all)
		}
		if code := setQuota(admin, nil); code != http.StatusOK {
			t.Fatalf("reset quota: %d", code)
		}
		if code, _, _ := write(u, mem("three", "third memory, over quota")); code != http.StatusCreated {
			t.Fatalf("after reset: %d", code)
		}
	})

	t.Run("compaction frees space, keeps knowledge, is idempotent", func(t *testing.T) {
		u := user("compact")
		old := time.Now().UTC().Add(-200 * 24 * time.Hour)
		past := time.Now().UTC().Add(-time.Hour)
		// superseded (with origin: becomes a tombstone)
		a := mem("Old way", "Deploy by hand")
		a.OriginHost, a.OriginID = "hub-a", "100"
		_, ar, _ := write(u, a)
		b := mem("New way", "Deploy by workflow")
		b.Supersedes = ar.Memory.ID
		write(u, b)
		// expired
		e := mem("Handoff", "Pick up the PR review")
		e.Kind, e.ExpiresAt = "handoff", &past
		write(u, e)
		// near-duplicates in one scope: the newer is kept
		n1 := mem("Partial index gotcha", "the github comment id index is partial so on conflict needs the where clause too")
		n1.CreatedAt = &old
		write(u, n1)
		write(u, mem("Partial index gotcha again", "the github comment id index is partial so on conflict needs the where clause"))
		// old low-confidence cluster in one scope and kind
		hunches := []string{"Maybe the redis cache is stale after deploy", "Websocket reconnects drop presence for a minute",
			"Thumbnail worker skips webp uploads"}
		for i, h := range hunches {
			l := mem(fmt.Sprintf("Hunch %d", i), h)
			l.Kind, l.Confidence, l.Scope = "fact", "low", MemoryScope{Folder: "/srv/app"}
			l.CreatedAt = &old
			write(u, l)
		}

		rep := compact(u)
		if rep.SupersededRemoved != 1 || rep.ExpiredRemoved != 1 || rep.DuplicatesMerged != 1 ||
			rep.LowConfidenceSummarised != 3 || rep.SummariesCreated != 1 || rep.FreedBytes <= 0 {
			t.Fatalf("first compaction %+v", rep)
		}
		res := search(u, MemorySearchRequest{Query: "websocket thumbnail redis"})
		if res.Total != 1 || !strings.Contains(res.Results[0].Body, hunches[0]) || !strings.Contains(res.Results[0].Body, hunches[2]) {
			t.Fatalf("summary lost knowledge: %+v", res)
		}
		all, active := count(u)

		// A hub re-sending the compacted memory is ignored, and a second run frees nothing.
		if _, again, _ := write(u, a); again.Outcome != "ignored" {
			t.Fatalf("re-sent compacted memory: %+v", again)
		}
		rep = compact(u)
		if rep != (MemoryCompactReport{BytesBefore: rep.BytesBefore, BytesAfter: rep.BytesBefore}) {
			t.Fatalf("second compaction not a no-op: %+v", rep)
		}
		if a2, act2 := count(u); a2 != all || act2 != active {
			t.Fatalf("rows changed: %d/%d -> %d/%d", all, active, a2, act2)
		}
	})

	t.Run("import wiki memory pages twice, then a hub upsert converges", func(t *testing.T) {
		u := user("import")
		var pid int64
		if err := ts.DB.QueryRowContext(ctx, `INSERT INTO projects (owner_id, name) VALUES ($1, 'Agent memory') RETURNING id`, u).Scan(&pid); err != nil {
			t.Fatal(err)
		}
		if _, err := ts.DB.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role, granted_by) VALUES ($1, $2, 'owner', $2)`, pid, u); err != nil {
			t.Fatal(err)
		}
		pages := [][2]string{
			{"[fact] biswas-dev/taskai: Prod is Postgres", "**fact** · repo `biswas-dev/taskai` · confidence high\n\nEvery environment uses PostgreSQL.\n\n---\n\n_2026-09-01 · by claude-code / claude on mbp · llm-brain memory 41_\n"},
			{"[superseded] [fact] biswas-dev/taskai: Prod is SQLite", "> Superseded by memory 41. Kept for history; agents no longer see it.\n\n**fact** · repo `biswas-dev/taskai` · confidence low\n\nSQLite.\n\n---\n\n_2026-08-01 on mbp · llm-brain memory 40_\n"},
			{"Unrelated page", "hello"},
		}
		for i, p := range pages {
			if _, err := ts.DB.ExecContext(ctx, `INSERT INTO wiki_pages (project_id, title, slug, content, created_by) VALUES ($1, $2, $3, $4, $5)`,
				pid, p[0], fmt.Sprintf("p%d", i), p[1], u); err != nil {
				t.Fatal(err)
			}
		}
		run := func(caller int64) (int, MemoryImportReport) {
			rec, r := ts.MakeAuthRequest(t, http.MethodPost, "/api/memories/import-wiki", MemoryImportRequest{ProjectID: pid, OriginHost: "hub-a"}, caller, nil)
			ts.HandleImportWikiMemories(rec, r)
			var rep MemoryImportReport
			if rec.Code == http.StatusOK {
				DecodeJSON(t, rec, &rep)
			}
			return rec.Code, rep
		}
		if code, _ := run(outsider); code != http.StatusForbidden {
			t.Fatalf("outsider import %d", code)
		}
		if _, rep := run(u); rep.Imported != 1 || rep.SkippedSuperseded != 1 || rep.Failed != 1 {
			t.Fatalf("first import %+v", rep)
		}
		if _, rep := run(u); rep.Imported != 0 || rep.Duplicates != 1 {
			t.Fatalf("second import %+v", rep)
		}
		hub := MemoryWriteRequest{Kind: "fact", Title: "Prod is Postgres", Body: "Every environment uses PostgreSQL.",
			Scope: MemoryScope{Repo: "biswas-dev/taskai"}, Confidence: "high", OriginHost: "hub-a", OriginID: "41"}
		if _, resp, _ := write(u, hub); resp.Outcome != "duplicate" {
			t.Fatalf("hub upsert after import: %+v", resp)
		}
		if all, _ := count(u); all != 1 {
			t.Fatalf("%d rows after import + hub upsert, want 1", all)
		}
	})
}
