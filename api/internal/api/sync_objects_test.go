package api

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap/zaptest"

	"taskai/internal/config"
	"taskai/internal/db"
)

func TestValidSyncKey(t *testing.T) {
	tests := []struct {
		key  string
		want bool
	}{
		{"blobs/0a1b", true},
		{"manifests/host-1/seg_2.v1", true},
		{"a", true},
		{strings.Repeat("k", syncMaxKey), true},
		{strings.Repeat("k", syncMaxKey+1), false},
		{"", false},
		{"/a", false},
		{"a/", false},
		{"a//b", false},
		{"..", false},
		{"a/../b", false},
		{"./a", false},
		{"a/.", false},
		{"a b", false},
		{"a%2Fb", false},
		{"a\\b", false},
		{"ключ", false},
		{"...", true},
	}
	for _, tt := range tests {
		if got := validSyncKey(tt.key); got != tt.want {
			t.Errorf("validSyncKey(%q) = %v, want %v", tt.key, got, tt.want)
		}
	}
}

func TestSyncIfMatch(t *testing.T) {
	tests := []struct {
		header, current string
		want            bool
	}{
		{`"abc"`, "abc", true},
		{`abc`, "abc", true},
		{`W/"abc"`, "abc", true},
		{`"x", "abc"`, "abc", true},
		{`*`, "abc", true},
		{`*`, "", false},
		{`"abc"`, "", false},
		{`"abd"`, "abc", false},
	}
	for _, tt := range tests {
		if got := syncIfMatch(tt.header, tt.current); got != tt.want {
			t.Errorf("syncIfMatch(%q, %q) = %v, want %v", tt.header, tt.current, got, tt.want)
		}
	}
}

func TestSyncObjects(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	runSyncObjectTests(t, ts)
}

func TestPostgresSyncObjects(t *testing.T) {
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
	runSyncObjectTests(t, &TestServer{Server: s, DB: database})
}

// syncRouter mounts the sync routes as main.go does, with userID (0 = no auth) in the request context.
func syncRouter(s *Server, userID int64) http.Handler {
	r := chi.NewRouter()
	r.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if userID != 0 {
				req = req.WithContext(context.WithValue(req.Context(), UserIDKey, userID))
			}
			next.ServeHTTP(w, req)
		})
	})
	r.Get("/api/sync/objects", s.HandleListSyncObjects)
	r.Put("/api/sync/objects/*", s.HandlePutSyncObject)
	r.Get("/api/sync/objects/*", s.HandleGetSyncObject)
	r.Delete("/api/sync/objects/*", s.HandleDeleteSyncObject)
	return r
}

func runSyncObjectTests(t *testing.T, ts *TestServer) {
	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	user := func(name string) int64 {
		var id int64
		if err := ts.DB.QueryRowContext(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, 'x') RETURNING id`,
			name+suffix+"@sync.test").Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	alice, bob := user("alice"), user("bob")
	do := func(userID int64, method, path string, body []byte, hdr map[string]string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, bytes.NewReader(body))
		for k, v := range hdr {
			req.Header.Set(k, v)
		}
		rec := httptest.NewRecorder()
		syncRouter(ts.Server, userID).ServeHTTP(rec, req)
		return rec
	}
	etagOf := func(b []byte) string { s := sha256.Sum256(b); return `"` + hex.EncodeToString(s[:]) + `"` }
	wantStatus := func(t *testing.T, rec *httptest.ResponseRecorder, want int) {
		t.Helper()
		if rec.Code != want {
			t.Fatalf("status %d, want %d: %s", rec.Code, want, rec.Body.String())
		}
	}
	list := func(t *testing.T, userID int64, query string) SyncObjectList {
		t.Helper()
		rec := do(userID, "GET", "/api/sync/objects"+query, nil, nil)
		wantStatus(t, rec, http.StatusOK)
		var out SyncObjectList
		if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
			t.Fatal(err)
		}
		return out
	}
	keys := func(l SyncObjectList) string {
		ks := make([]string, len(l.Objects))
		for i, o := range l.Objects {
			ks[i] = o.Key
		}
		return strings.Join(ks, ",")
	}

	t.Run("no auth is 401", func(t *testing.T) {
		for _, m := range []string{"PUT", "GET", "DELETE"} {
			wantStatus(t, do(0, m, "/api/sync/objects/a", []byte("x"), nil), http.StatusUnauthorized)
		}
		wantStatus(t, do(0, "GET", "/api/sync/objects", nil, nil), http.StatusUnauthorized)
	})

	t.Run("invalid keys are 400", func(t *testing.T) {
		for _, p := range []string{"/api/sync/objects/", "/api/sync/objects/a//b", "/api/sync/objects/a/../b", "/api/sync/objects/a%2Fb", "/api/sync/objects/a%20b", "/api/sync/objects/" + strings.Repeat("k", 257)} {
			rec := do(alice, "PUT", p, []byte("x"), nil)
			wantStatus(t, rec, http.StatusBadRequest)
			if !strings.Contains(rec.Body.String(), "invalid_key") {
				t.Fatalf("%s: %s", p, rec.Body.String())
			}
		}
	})

	t.Run("put, get, replace with exact bytes and etag", func(t *testing.T) {
		body := []byte{0, 1, 2, 0xff, 'x', '\n'}
		rec := do(alice, "PUT", "/api/sync/objects/blobs/one", body, map[string]string{"Content-Type": "application/octet-stream"})
		wantStatus(t, rec, http.StatusCreated)
		if rec.Header().Get("ETag") != etagOf(body) {
			t.Fatalf("etag %q, want %q", rec.Header().Get("ETag"), etagOf(body))
		}
		got := do(alice, "GET", "/api/sync/objects/blobs/one", nil, nil)
		wantStatus(t, got, http.StatusOK)
		if !bytes.Equal(got.Body.Bytes(), body) || got.Header().Get("ETag") != etagOf(body) || got.Header().Get("Content-Type") != "application/octet-stream" {
			t.Fatalf("got %v %q %q", got.Body.Bytes(), got.Header().Get("ETag"), got.Header().Get("Content-Type"))
		}
		wantStatus(t, do(alice, "PUT", "/api/sync/objects/blobs/one", []byte("v2"), nil), http.StatusOK)
		if got := do(alice, "GET", "/api/sync/objects/blobs/one", nil, nil); got.Body.String() != "v2" {
			t.Fatalf("after upsert: %q", got.Body.String())
		}
		wantStatus(t, do(alice, "GET", "/api/sync/objects/blobs/missing", nil, nil), http.StatusNotFound)
	})

	t.Run("preconditions", func(t *testing.T) {
		create := map[string]string{"If-None-Match": "*"}
		wantStatus(t, do(alice, "PUT", "/api/sync/objects/p/a", []byte("1"), create), http.StatusCreated)
		wantStatus(t, do(alice, "PUT", "/api/sync/objects/p/a", []byte("2"), create), http.StatusPreconditionFailed)
		wantStatus(t, do(alice, "PUT", "/api/sync/objects/p/a", []byte("2"), map[string]string{"If-Match": etagOf([]byte("x"))}), http.StatusPreconditionFailed)
		wantStatus(t, do(alice, "PUT", "/api/sync/objects/p/a", []byte("2"), map[string]string{"If-Match": etagOf([]byte("1"))}), http.StatusOK)
		wantStatus(t, do(alice, "PUT", "/api/sync/objects/p/new", []byte("2"), map[string]string{"If-Match": "*"}), http.StatusPreconditionFailed)
		wantStatus(t, do(alice, "PUT", "/api/sync/objects/p/a", []byte("3"), map[string]string{"If-None-Match": `"abc"`}), http.StatusBadRequest)
		wantStatus(t, do(alice, "PUT", "/api/sync/objects/p/a", []byte("3"), map[string]string{"If-None-Match": "*", "If-Match": "*"}), http.StatusBadRequest)
		if got := do(alice, "GET", "/api/sync/objects/p/a", nil, nil); got.Body.String() != "2" {
			t.Fatalf("a failed precondition changed the object: %q", got.Body.String())
		}
		// Repeated header lines are one list (RFC 9110): the second line matches.
		req := httptest.NewRequest("PUT", "/api/sync/objects/p/a", strings.NewReader("4"))
		req.Header.Add("If-Match", etagOf([]byte("old")))
		req.Header.Add("If-Match", etagOf([]byte("2")))
		rec := httptest.NewRecorder()
		syncRouter(ts.Server, alice).ServeHTTP(rec, req)
		wantStatus(t, rec, http.StatusOK)
		req = httptest.NewRequest("PUT", "/api/sync/objects/p/a", strings.NewReader("5"))
		req.Header.Add("If-None-Match", "*")
		req.Header.Add("If-None-Match", `"abc"`)
		rec = httptest.NewRecorder()
		syncRouter(ts.Server, alice).ServeHTTP(rec, req)
		wantStatus(t, rec, http.StatusBadRequest)
	})

	t.Run("concurrent creates: exactly one wins", func(t *testing.T) {
		const n = 8
		codes := make(chan int, n)
		var wg sync.WaitGroup
		for i := 0; i < n; i++ {
			wg.Add(1)
			go func(i int) {
				defer wg.Done()
				codes <- do(alice, "PUT", "/api/sync/objects/race/one", []byte{byte(i)}, map[string]string{"If-None-Match": "*"}).Code
			}(i)
		}
		wg.Wait()
		close(codes)
		won := 0
		for c := range codes {
			switch c {
			case http.StatusCreated:
				won++
			case http.StatusPreconditionFailed:
			default:
				t.Fatalf("status %d", c)
			}
		}
		if won != 1 {
			t.Fatalf("%d creates won, want 1", won)
		}
	})

	t.Run("delete", func(t *testing.T) {
		wantStatus(t, do(alice, "PUT", "/api/sync/objects/d/a", []byte("1"), nil), http.StatusCreated)
		wantStatus(t, do(alice, "DELETE", "/api/sync/objects/d/a", nil, map[string]string{"If-Match": etagOf([]byte("0"))}), http.StatusPreconditionFailed)
		wantStatus(t, do(alice, "DELETE", "/api/sync/objects/d/a", nil, map[string]string{"If-Match": etagOf([]byte("1"))}), http.StatusNoContent)
		wantStatus(t, do(alice, "GET", "/api/sync/objects/d/a", nil, nil), http.StatusNotFound)
		wantStatus(t, do(alice, "DELETE", "/api/sync/objects/d/a", nil, nil), http.StatusNoContent)
		wantStatus(t, do(alice, "DELETE", "/api/sync/objects/d/a", nil, map[string]string{"If-Match": "*"}), http.StatusPreconditionFailed)
	})

	t.Run("size limit", func(t *testing.T) {
		big := bytes.Repeat([]byte{7}, 2<<20) // above the API's general 1 MiB body limit
		wantStatus(t, do(alice, "PUT", "/api/sync/objects/size/big", big, nil), http.StatusCreated)
		if got := do(alice, "GET", "/api/sync/objects/size/big", nil, nil); !bytes.Equal(got.Body.Bytes(), big) {
			t.Fatalf("big object came back with %d bytes", got.Body.Len())
		}
		rec := do(alice, "PUT", "/api/sync/objects/size/huge", make([]byte, syncMaxObject+1), nil)
		wantStatus(t, rec, http.StatusRequestEntityTooLarge)
		if !strings.Contains(rec.Body.String(), "object_too_large") {
			t.Fatal(rec.Body.String())
		}
		// Unknown length (chunked): the handler's own read limit refuses it.
		req := httptest.NewRequest("PUT", "/api/sync/objects/size/chunked", io.NopCloser(bytes.NewReader(make([]byte, syncMaxObject+1))))
		req.ContentLength = -1
		rec = httptest.NewRecorder()
		syncRouter(ts.Server, alice).ServeHTTP(rec, req)
		wantStatus(t, rec, http.StatusRequestEntityTooLarge)
		wantStatus(t, do(alice, "PUT", "/api/sync/objects/size/max", make([]byte, syncMaxObject), nil), http.StatusCreated)
		wantStatus(t, do(alice, "DELETE", "/api/sync/objects/size/big", nil, nil), http.StatusNoContent)
		wantStatus(t, do(alice, "DELETE", "/api/sync/objects/size/max", nil, nil), http.StatusNoContent)
	})

	t.Run("quota charges only growth", func(t *testing.T) {
		carol := user("carol")
		old := ts.Server.config.SyncQuotaBytes
		ts.Server.config.SyncQuotaBytes = 2*syncRowOverhead + 100
		defer func() { ts.Server.config.SyncQuotaBytes = old }()
		wantStatus(t, do(carol, "PUT", "/api/sync/objects/q/a", make([]byte, 60), nil), http.StatusCreated)
		wantStatus(t, do(carol, "PUT", "/api/sync/objects/q/a", make([]byte, 90), nil), http.StatusOK)
		rec := do(carol, "PUT", "/api/sync/objects/q/b", make([]byte, 20), nil)
		wantStatus(t, rec, http.StatusRequestEntityTooLarge)
		if !strings.Contains(rec.Body.String(), "sync_quota_exceeded") {
			t.Fatal(rec.Body.String())
		}
		wantStatus(t, do(carol, "PUT", "/api/sync/objects/q/b", make([]byte, 10), nil), http.StatusCreated)
		wantStatus(t, do(bob, "PUT", "/api/sync/objects/q/b", make([]byte, 50), nil), http.StatusCreated) // quota is per user
		erin := user("erin")
		wantStatus(t, do(erin, "PUT", "/api/sync/objects/e/1", nil, nil), http.StatusCreated)
		wantStatus(t, do(erin, "PUT", "/api/sync/objects/e/2", nil, nil), http.StatusCreated)
		wantStatus(t, do(erin, "PUT", "/api/sync/objects/e/3", nil, nil), http.StatusRequestEntityTooLarge) // empty objects are not free
	})

	t.Run("list pages by key with an exact prefix", func(t *testing.T) {
		dave := user("dave")
		for _, k := range []string{"m/2", "m/1", "m/3", "m/a_b", "m/axb", "M/1", "n/1", "m2"} {
			wantStatus(t, do(dave, "PUT", "/api/sync/objects/"+k, []byte(k), nil), http.StatusCreated)
		}
		first := list(t, dave, "?prefix=m/&limit=2")
		if keys(first) != "m/1,m/2" || first.Next != "m/2" || first.Objects[0].Size != 3 || first.Objects[0].ETag != strings.Trim(etagOf([]byte("m/1")), `"`) || first.Objects[0].UpdatedAt.IsZero() {
			t.Fatalf("first page %+v", first)
		}
		second := list(t, dave, "?prefix=m/&limit=2&after="+first.Next)
		if keys(second) != "m/3,m/a_b" || second.Next != "m/a_b" {
			t.Fatalf("second page %+v", second)
		}
		last := list(t, dave, "?prefix=m/&limit=2&after="+second.Next)
		if keys(last) != "m/axb" || last.Next != "" {
			t.Fatalf("last page %+v", last)
		}
		if got := keys(list(t, dave, "?prefix=m/a_")); got != "m/a_b" {
			t.Fatalf("underscore is not a wildcard: %s", got)
		}
		if got := keys(list(t, dave, "")); got != "M/1,m/1,m/2,m/3,m/a_b,m/axb,m2,n/1" {
			t.Fatalf("all keys in byte order: %s", got)
		}
		if got := list(t, dave, "?prefix=zzz"); got.Objects == nil || len(got.Objects) != 0 {
			t.Fatalf("empty list must be [], got %+v", got)
		}
		for _, q := range []string{"?limit=0", "?limit=1001", "?limit=x", "?prefix=a%25", "?after=a%20b", "?prefix=" + strings.Repeat("a", 257)} {
			wantStatus(t, do(dave, "GET", "/api/sync/objects"+q, nil, nil), http.StatusBadRequest)
		}
	})

	t.Run("other users' objects are invisible", func(t *testing.T) {
		wantStatus(t, do(alice, "PUT", "/api/sync/objects/private/x", []byte("secret"), nil), http.StatusCreated)
		wantStatus(t, do(bob, "GET", "/api/sync/objects/private/x", nil, nil), http.StatusNotFound)
		if got := list(t, bob, "?prefix=private/"); len(got.Objects) != 0 {
			t.Fatalf("bob sees %+v", got)
		}
		wantStatus(t, do(bob, "PUT", "/api/sync/objects/private/x", []byte("bob"), map[string]string{"If-None-Match": "*"}), http.StatusCreated)
		wantStatus(t, do(bob, "DELETE", "/api/sync/objects/private/x", nil, nil), http.StatusNoContent)
		if got := do(alice, "GET", "/api/sync/objects/private/x", nil, nil); got.Body.String() != "secret" {
			t.Fatalf("bob changed alice's object: %q", got.Body.String())
		}
	})
}
