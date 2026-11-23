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
		{"repo too long", with(func(r *MemoryWriteRequest) { r.Scope.Repo = "a/" + strings.Repeat("b", 600) }), "under 500 bytes"},
		{"root folder", with(func(r *MemoryWriteRequest) { r.Scope = MemoryScope{Folder: "/"} }), ""},
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
	f1, _ := normalizeMemory(with(func(r *MemoryWriteRequest) { r.Scope.Files = []string{"a,b"} }), now)
	f2, _ := normalizeMemory(with(func(r *MemoryWriteRequest) { r.Scope.Files = []string{"a", "b"} }), now)
	if f1.hash == f2.hash {
		t.Error(`files ["a,b"] and ["a","b"] must not hash the same`)
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

	if req, _, err := parseMemoryPage("[fact] /x: t", "**fact** · folder `/x`\n\nb\n\n---\n\n_2026-01-01 on mbp · by codex · llm-brain memory 3_"); err != nil || req.Author.Host != "mbp" || req.Author.Agent != "codex" {
		t.Fatalf("host before by: %+v %v", req.Author, err)
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
	var sharedProject int64
	if err := ts.DB.QueryRowContext(ctx, `INSERT INTO projects (owner_id, name) VALUES ($1, 'Team') RETURNING id`, owner).Scan(&sharedProject); err != nil {
		t.Fatal(err)
	}
	for _, u := range []int64{owner, friend} {
		if _, err := ts.DB.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role, granted_by) VALUES ($1, $2, 'member', $3)`, sharedProject, u, owner); err != nil {
			t.Fatal(err)
		}
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
		share := func(ids ...int64) int {
			rec, r := ts.MakeAuthRequest(t, http.MethodPut, "/api/memories/shares", map[string]any{"user_ids": ids}, owner, nil)
			ts.HandleSetMemoryShares(rec, r)
			return rec.Code
		}
		if code := share(outsider); code != http.StatusBadRequest {
			t.Fatalf("sharing with someone outside your teams and projects: %d", code)
		}
		if code := share(999999); code != http.StatusBadRequest {
			t.Fatalf("sharing with an unknown id: %d", code)
		}
		if code := share(friend); code != http.StatusOK {
			t.Fatalf("share with a project collaborator: %d", code)
		}
		rec, r := ts.MakeAuthRequest(t, http.MethodPut, "/api/memories/shares", map[string]any{}, owner, nil)
		ts.HandleSetMemoryShares(rec, r)
		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
		if code := share(make([]int64, memoryMaxShares+1)...); code != http.StatusBadRequest {
			t.Fatalf("too many shares: %d", code)
		}
		// A teammate (same company) can be added too.
		mate := user("mate")
		var team int64
		if err := ts.DB.QueryRowContext(ctx, `INSERT INTO teams (name, owner_id) VALUES ('Co', $1) RETURNING id`, owner).Scan(&team); err != nil {
			t.Fatal(err)
		}
		for _, u := range []int64{owner, mate} {
			if _, err := ts.DB.ExecContext(ctx, `INSERT INTO team_members (team_id, user_id, role, status) VALUES ($1, $2, 'member', 'active')`, team, u); err != nil {
				t.Fatal(err)
			}
		}
		if code := share(friend, mate); code != http.StatusOK {
			t.Fatalf("share with a teammate: %d", code)
		}
		rec, r = ts.MakeAuthRequest(t, http.MethodGet, "/api/memories/shares", nil, friend, nil)
		ts.HandleGetMemoryShares(rec, r)
		var shares struct {
			SharedWithMe []MemoryShareUser `json:"shared_with_me"`
		}
		DecodeJSON(t, rec, &shares)
		if len(shares.SharedWithMe) != 1 || shares.SharedWithMe[0].UserID != owner {
			t.Fatalf("shared_with_me: %+v (a body without user_ids must not wipe shares)", shares)
		}
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
		if res := search(owner, MemorySearchRequest{Query: "r_llback", Mode: "keyword"}); res.Total != 0 {
			t.Fatal("underscore in a keyword query must not act as a wildcard")
		}
		rec, r := ts.MakeAuthRequest(t, http.MethodPost, "/api/memories/search", MemorySearchRequest{Query: "rollback", Mode: "semantic"}, owner, nil)
		ts.HandleSearchMemories(rec, r)
		AssertError(t, rec, http.StatusBadRequest, "mode must be", "invalid_request")
		rec, r = ts.MakeAuthRequest(t, http.MethodPost, "/api/memories/search", MemorySearchRequest{Query: "rollback", Files: make([]string, memoryMaxFiles+1)}, owner, nil)
		ts.HandleSearchMemories(rec, r)
		AssertError(t, rec, http.StatusBadRequest, "at most", "validation_error")
		list := func(u int64, query string) (int, []Memory) {
			rec, r := ts.MakeAuthRequest(t, http.MethodGet, "/api/memories?"+query, nil, u, nil)
			ts.HandleListMemories(rec, r)
			var out struct{ Memories []Memory }
			if rec.Code == http.StatusOK {
				DecodeJSON(t, rec, &out)
			}
			return rec.Code, out.Memories
		}
		if code, ms := list(owner, "kind=runbook&repo=biswas-dev/other"); code != http.StatusOK || len(ms) != 1 {
			t.Fatalf("list filter: %d %d", code, len(ms))
		}
		if code, _ := list(owner, "owner_id=abc"); code != http.StatusBadRequest {
			t.Fatalf("bad owner_id: %d", code)
		}
		for _, bad := range []string{"issue=abc", "limit=-1", "offset=x"} {
			if code, _ := list(owner, bad); code != http.StatusBadRequest {
				t.Fatalf("%s: %d", bad, code)
			}
		}
		if code, ms := list(owner, "file=deploy/run_me.sh&limit=1"); code != http.StatusOK || len(ms) != 1 {
			t.Fatalf("file filter + limit: %d %d", code, len(ms))
		}
		_, active := list(owner, "")
		_, all := list(owner, "status=all")
		if len(all) <= len(active) {
			t.Fatalf("status=all should include superseded memories: %d vs %d", len(all), len(active))
		}
		if res := search(owner, MemorySearchRequest{Query: "deploy script", IncludeInactive: true}); res.Total == 0 {
			t.Fatal("include_inactive should find the superseded deploy memory")
		}
		if res := search(owner, MemorySearchRequest{Query: "r"}); res.Total == 0 {
			t.Fatal("a one-letter query should search, not fail")
		}
		if res := search(owner, MemorySearchRequest{Query: "---"}); res.Total != 0 {
			t.Fatalf("punctuation-only query: %+v", res)
		}
		rec, r = ts.MakeAuthRequest(t, http.MethodPost, "/api/memories/search", MemorySearchRequest{Query: "deploy", Issue: -1}, owner, nil)
		ts.HandleSearchMemories(rec, r)
		AssertError(t, rec, http.StatusBadRequest, "positive", "validation_error")
		if code, _ := list(outsider, fmt.Sprintf("owner_id=%d", owner)); code != http.StatusForbidden {
			t.Fatalf("unshared owner_id: %d", code)
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
			// A partial word: full text misses, vectors or the keyword fallback still put the memory first.
			if res := search(owner, MemorySearchRequest{Query: "rollbac"}); res.Total == 0 || !strings.Contains(res.Results[0].Title, "Rollback") {
				t.Fatalf("partial word: %+v", res)
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
		for _, bad := range []any{0, -5, memoryMaxQuota + 1} {
			if code := setQuota(admin, bad); code != http.StatusBadRequest {
				t.Fatalf("quota %v: %d", bad, code)
			}
		}
		rec0, r0 := ts.MakeAuthRequest(t, http.MethodPut, "/api/admin/users/1/memory-quota", map[string]any{"quota": 5}, admin, map[string]string{"id": "1"})
		ts.HandleAdminSetMemoryQuota(rec0, r0)
		AssertError(t, rec0, http.StatusBadRequest, "quota_bytes is required", "validation_error")
		sid := "999999"
		rec, r := ts.MakeAuthRequest(t, http.MethodPut, "/api/admin/users/"+sid+"/memory-quota", map[string]any{"quota_bytes": 10}, admin, map[string]string{"id": sid})
		ts.HandleAdminSetMemoryQuota(rec, r)
		AssertStatusCode(t, rec.Code, http.StatusNotFound)
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
		tmp := time.Now().UTC().Add(24 * time.Hour)
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
		n1 := mem("Partial index gotcha", "the github comment id index is partial so on conflict needs the where clause")
		n1.CreatedAt = &old
		write(u, n1)
		// Without an origin, a copy differing only in case and spacing is caught at write time.
		if _, dup, _ := write(u, mem("partial  INDEX gotcha", "The GitHub comment id index is partial so ON CONFLICT needs the WHERE clause")); dup.Outcome != "duplicate" {
			t.Fatalf("case-only copy: %+v", dup)
		}
		// Overlapping text that says the opposite: both stay.
		write(u, mem("Port", "Use PORT=3000."))
		write(u, mem("Port", "Do not use PORT=3000."))
		// Copies where neither dominates (one lasts longer, the other is more confident) both stay.
		x1 := mem("Cache TTL", "the cache ttl is one hour")
		x1.CreatedAt, x1.Confidence, x1.ExpiresAt = &old, "high", &tmp
		x1.OriginHost, x1.OriginID = "hub-a", "700"
		write(u, x1)
		x2 := mem("Cache TTL", "the cache ttl is one hour")
		x2.Confidence, x2.OriginHost, x2.OriginID = "low", "hub-b", "701"
		write(u, x2)
		// Same body, different instruction in the title: both stay.
		r1 := mem("Do not restart production workers", "deployment checklist: drain queues, then deploy")
		r1.CreatedAt = &old
		write(u, r1)
		write(u, mem("Restart production workers", "deployment checklist: drain queues, then deploy"))
		// The newer copy names fewer files: both stay, so a file filter still finds the older one.
		f1 := mem("Lock order", "take the owner lock first")
		f1.CreatedAt, f1.Scope.Files = &old, []string{"api/a.go", "api/b.go"}
		write(u, f1)
		write(u, mem("Lock order", "take the owner lock first"))
		// Two hubs' copies of one lesson fold into one; the folded origin is then ignored.
		h1 := mem("Two hubs", "the same lesson from two hubs")
		h1.CreatedAt, h1.OriginHost, h1.OriginID = &old, "hub-a", "500"
		write(u, h1)
		h2 := h1
		h2.CreatedAt, h2.OriginHost, h2.OriginID = nil, "hub-b", "900"
		write(u, h2)
		// A newer copy that expires folds in, and the kept one stays permanent and keeps the higher confidence.
		k1 := mem("Rotate logs", "logrotate runs nightly at two")
		k1.CreatedAt, k1.Confidence = &old, "high"
		k1.OriginHost, k1.OriginID = "hub-a", "600"
		write(u, k1)
		k2 := mem("Rotate logs", "logrotate runs nightly at two")
		k2.ExpiresAt = &tmp
		k2.OriginHost, k2.OriginID = "hub-b", "601"
		write(u, k2)
		// Same words, opposite meaning: both stay.
		p1 := mem("Copy data", "copy production data to staging")
		p1.CreatedAt = &old
		write(u, p1)
		write(u, mem("Copy data", "copy staging data to production"))
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
		if rep.SupersededRemoved != 1 || rep.ExpiredRemoved != 1 || rep.DuplicatesMerged != 2 ||
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
		if _, again, _ := write(u, h1); again.Outcome != "ignored" {
			t.Fatalf("re-sent folded copy from the first hub: %+v", again)
		}
		titles := func(res MemorySearchResponse) map[string]int {
			n := map[string]int{}
			for _, m := range res.Results {
				n[m.Title]++
			}
			return n
		}
		if got := titles(search(u, MemorySearchRequest{Query: "restart workers drain", Limit: 50})); got["Restart production workers"] != 1 || got["Do not restart production workers"] != 1 {
			t.Fatalf("both restart memories should survive: %v", got)
		}
		if got := titles(search(u, MemorySearchRequest{Query: "lock order", Limit: 50})); got["Lock order"] != 2 {
			t.Fatalf("both lock-order memories should survive: %v", got)
		}
		if got := titles(search(u, MemorySearchRequest{Query: "port 3000", Limit: 50})); got["Port"] != 2 {
			t.Fatalf("opposite instructions must both survive: %v", got)
		}
		logs := search(u, MemorySearchRequest{Query: "logrotate nightly"})
		if logs.Total != 1 || logs.Results[0].ExpiresAt != nil || logs.Results[0].Confidence != "high" || logs.Results[0].OriginID != "600" {
			t.Fatalf("the dominating copy (permanent, high) should be kept: %+v", logs)
		}
		if got := titles(search(u, MemorySearchRequest{Query: "cache ttl hour", Limit: 50})); got["Cache TTL"] != 2 {
			t.Fatalf("copies where neither dominates must both stay: %v", got)
		}
		if res := search(u, MemorySearchRequest{Query: "lock", Files: []string{"api/b.go"}}); res.Total != 1 {
			t.Fatalf("file filter lost the older memory: %+v", res)
		}
		rep = compact(u)
		if rep != (MemoryCompactReport{BytesBefore: rep.BytesBefore, BytesAfter: rep.BytesBefore}) {
			t.Fatalf("second compaction not a no-op: %+v", rep)
		}
		if a2, act2 := count(u); a2 != all || act2 != active {
			t.Fatalf("rows changed: %d/%d -> %d/%d", all, active, a2, act2)
		}
	})

	t.Run("origin replays: deleted stays deleted, superseded stays superseded, metadata updates apply", func(t *testing.T) {
		u := user("replay")
		a := mem("Use the deploy script", "Deploys go through script/server deploy")
		a.OriginHost, a.OriginID = "hub-a", "1"
		_, ar, _ := write(u, a)
		b := mem("Deploy via workflow", "Deploys go through GitHub Actions")
		b.OriginHost, b.OriginID, b.SupersedesOriginID = "hub-a", "2", "1"
		write(u, b)
		if _, again, _ := write(u, a); again.Memory.Status != "superseded" {
			t.Fatalf("replaying the original revived it: %+v", again.Memory)
		}

		exp := time.Now().UTC().Add(48 * time.Hour).Truncate(time.Second)
		b.ExpiresAt = &exp
		if _, upd, _ := write(u, b); upd.Outcome != "updated" || upd.Memory.ExpiresAt == nil {
			t.Fatalf("expiry-only change: %+v", upd)
		}

		// A -> B -> C by origin: replaying the middle one, unchanged, is idempotent.
		cc := mem("Deploy via release", "Deploys go through the release workflow")
		cc.OriginHost, cc.OriginID, cc.SupersedesOriginID = "hub-a", "4", "2"
		write(u, cc)
		if code, again, body := write(u, b); code != http.StatusOK || again.Memory.Status != "superseded" {
			t.Fatalf("replaying a superseded middle memory: %d %s", code, body)
		}
		if code, _, body := write(u, func() MemoryWriteRequest { r := b; r.SupersedesOriginID = "2"; return r }()); code != http.StatusBadRequest {
			t.Fatalf("self-supersede: %d %s", code, body)
		}

		// Out of order, two memories naming each other: the second is refused instead of hiding both.
		m1 := mem("Mutual one", "first of a pair")
		m1.OriginHost, m1.OriginID, m1.SupersedesOriginID = "hub-a", "20", "21"
		write(u, m1)
		m2 := mem("Mutual two", "second of a pair")
		m2.OriginHost, m2.OriginID, m2.SupersedesOriginID = "hub-a", "21", "20"
		if code, _, body := write(u, m2); code != http.StatusBadRequest {
			t.Fatalf("mutual supersede: %d %s", code, body)
		}

		// A more confident repeat (no origin) raises the kept memory's confidence.
		lc := mem("Hunch", "the queue may stall under load")
		lc.Confidence = "low"
		write(u, lc)
		lc.Confidence = "high"
		if _, up, _ := write(u, lc); up.Outcome != "duplicate" || up.Memory.Confidence != "high" {
			t.Fatalf("confidence upgrade on repeat: %+v", up.Memory)
		}

		// Deleting a memory with an origin is not undone when its hub sends it again.
		c := mem("Staging is ARM64", "Staging and UAT are aarch64")
		c.OriginHost, c.OriginID = "hub-a", "3"
		_, cr, _ := write(u, c)
		sid := fmt.Sprint(cr.Memory.ID)
		rec, r := ts.MakeAuthRequest(t, http.MethodDelete, "/api/memories/"+sid, nil, u, map[string]string{"id": sid})
		ts.HandleDeleteMemory(rec, r)
		AssertStatusCode(t, rec.Code, http.StatusNoContent)
		if code, _ := get(u, cr.Memory.ID); code != http.StatusNotFound {
			t.Fatalf("tombstone visible: %d", code)
		}
		if _, again, _ := write(u, c); again.Outcome != "ignored" {
			t.Fatalf("replay after delete: %+v", again)
		}
		usage := func() MemoryUsage {
			rec, r := ts.MakeAuthRequest(t, http.MethodGet, "/api/memories/usage", nil, u, nil)
			ts.HandleMemoryUsage(rec, r)
			AssertStatusCode(t, rec.Code, http.StatusOK)
			var out MemoryUsage
			DecodeJSON(t, rec, &out)
			return out
		}
		if got := usage(); got.Count != 5 || got.ActiveCount != 3 {
			t.Fatalf("usage should leave out the tombstone and count only active ones: %+v", got)
		}

		// An exact repeat that names what it supersedes still supersedes it.
		d := mem("Old tip", "Restart the worker by hand")
		_, dr, _ := write(u, d)
		e := mem("New tip", "The worker restarts itself")
		_, er, _ := write(u, e)
		e.Supersedes = dr.Memory.ID
		if _, rep, _ := write(u, e); rep.Outcome != "duplicate" || rep.Memory.ID != er.Memory.ID {
			t.Fatalf("repeat with supersedes: %+v", rep)
		}
		if _, old := get(u, dr.Memory.ID); old.Status != "superseded" {
			t.Fatalf("repeat did not supersede: %+v", old)
		}

		// A repeat that lasts longer extends the memory we keep.
		soon := time.Now().UTC().Add(time.Hour)
		g := mem("Freeze", "Deploy freeze this week")
		g.ExpiresAt = &soon
		write(u, g)
		g.ExpiresAt = nil
		if _, rep, _ := write(u, g); rep.Outcome != "duplicate" || rep.Memory.ExpiresAt != nil {
			t.Fatalf("permanent repeat should drop the expiry: %+v", rep.Memory)
		}

		// An expired memory does not swallow a fresh write of the same lesson.
		past := time.Now().UTC().Add(-time.Hour)
		f := mem("Handoff", "Pick up the review")
		f.ExpiresAt = &past
		_, fr, _ := write(u, f)
		f.ExpiresAt = nil
		if _, fresh, _ := write(u, f); fresh.Outcome != "created" || fresh.Memory.ID == fr.Memory.ID {
			t.Fatalf("fresh write after expiry: %+v", fresh)
		}
		if res := search(u, MemorySearchRequest{Query: "staging aarch64"}); res.Total != 0 {
			t.Fatalf("deleted memory still searchable: %+v", res)
		}
		gone := mem("x", "y")
		gone.Supersedes = cr.Memory.ID
		if code, _, _ := write(u, gone); code != http.StatusBadRequest {
			t.Fatalf("superseding a removed memory: %d", code)
		}
		_ = ar
	})

	t.Run("an original arriving after its successor was compacted stays superseded", func(t *testing.T) {
		u := user("late")
		b := mem("B", "second version")
		b.OriginHost, b.OriginID, b.SupersedesOriginID = "hub-a", "2", "1"
		write(u, b)
		c := mem("C", "third version")
		c.OriginHost, c.OriginID, c.SupersedesOriginID = "hub-a", "3", "2"
		write(u, c)
		compact(u)
		a := mem("A", "first version")
		a.OriginHost, a.OriginID = "hub-a", "1"
		if _, late, _ := write(u, a); late.Memory.Status != "superseded" {
			t.Fatalf("late original revived: %+v", late.Memory)
		}
	})

	t.Run("over quota: compaction runs before refusing, and the worker compacts past the high-water mark", func(t *testing.T) {
		u := user("autocompact")
		_, a, _ := write(u, mem("Old way", "Deploy by hand"))
		b := mem("New way", "Deploy by workflow")
		b.Supersedes = a.Memory.ID
		_, br, _ := write(u, b)
		if _, err := ts.DB.ExecContext(ctx, `INSERT INTO memory_quotas (user_id, quota_bytes) VALUES ($1, $2)`, u, br.Usage.UsedBytes+100); err != nil {
			t.Fatal(err)
		}
		// Room only once the superseded memory is gone.
		code, resp, body := write(u, mem("Third", "Needs the space the superseded memory holds"))
		if code != http.StatusCreated || resp.Usage.UsedBytes > resp.Usage.QuotaBytes {
			t.Fatalf("write after inline compaction: %d %s", code, body)
		}

		w := user("worker")
		_, wa, _ := write(w, mem("Old", "Deploy by hand"))
		wb := mem("New", "Deploy by workflow")
		wb.Supersedes = wa.Memory.ID
		_, wbr, _ := write(w, wb)
		if _, err := ts.DB.ExecContext(ctx, `INSERT INTO memory_quotas (user_id, quota_bytes) VALUES ($1, $2)`, w, wbr.Usage.UsedBytes); err != nil {
			t.Fatal(err)
		}
		ts.maintainMemories(ctx)
		if all, active := count(w); all != 1 || active != 1 {
			t.Fatalf("worker did not compact: %d rows, %d active", all, active)
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
		// Other Postgres tests count every page waiting to be indexed; leave none behind.
		t.Cleanup(func() { _, _ = ts.DB.ExecContext(ctx, `DELETE FROM projects WHERE id = $1`, pid) })
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
		rec, r := ts.MakeAuthRequest(t, http.MethodPost, "/api/memories/import-wiki", MemoryImportRequest{ProjectID: pid}, u, nil)
		ts.HandleImportWikiMemories(rec, r)
		AssertError(t, rec, http.StatusBadRequest, "origin_host", "validation_error")
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

func TestChunkMemories(t *testing.T) {
	big := strings.Repeat("x", memoryMaxBody/2)
	ms := []Memory{{Title: "a", Body: big}, {Title: "b", Body: big}, {Title: "c", Body: "small"}}
	chunks := chunkMemories(ms)
	if len(chunks) != 2 || len(chunks[0]) != 1 || len(chunks[1]) != 2 {
		t.Fatalf("chunks: %d", len(chunks))
	}
	for _, c := range chunks {
		size := len(memorySummaryHeader)
		for _, m := range c {
			size += len(memorySummaryEntry(m))
		}
		if size > memoryMaxBody {
			t.Fatalf("chunk of %d bytes exceeds %d", size, memoryMaxBody)
		}
	}
}
