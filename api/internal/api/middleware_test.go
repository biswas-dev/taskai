package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"go.uber.org/zap"
)

func TestJWTAuthValidToken(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "jwt@example.com", "password123")
	token := ts.GenerateTestToken(t, userID, "jwt@example.com")

	// Create a handler that checks the context values were set
	var gotUserID int64
	var gotEmail string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = r.Context().Value(UserIDKey).(int64)
		gotEmail, _ = r.Context().Value(UserEmailKey).(string)
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	})

	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
	}
	rec, req := MakeRequest(t, http.MethodGet, "/api/protected", nil, headers)

	ts.JWTAuth(handler).ServeHTTP(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	if gotUserID != userID {
		t.Errorf("Expected UserIDKey=%d in context, got %d", userID, gotUserID)
	}
	if gotEmail != "jwt@example.com" {
		t.Errorf("Expected UserEmailKey=%q in context, got %q", "jwt@example.com", gotEmail)
	}
}

func TestJWTAuthMissingHeader(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called when authorization header is missing")
	})

	rec, req := MakeRequest(t, http.MethodGet, "/api/protected", nil, nil)

	ts.JWTAuth(handler).ServeHTTP(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusUnauthorized)
	AssertError(t, rec, http.StatusUnauthorized, "missing authorization header", "unauthorized")
}

func TestJWTAuthInvalidToken(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with invalid token")
	})

	headers := map[string]string{
		"Authorization": "Bearer invalid-token-here",
	}
	rec, req := MakeRequest(t, http.MethodGet, "/api/protected", nil, headers)

	ts.JWTAuth(handler).ServeHTTP(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusUnauthorized)
	AssertError(t, rec, http.StatusUnauthorized, "invalid or expired token", "unauthorized")
}

func TestJWTAuthMalformedHeader(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with malformed header")
	})

	headers := map[string]string{
		"Authorization": "justonepart",
	}
	rec, req := MakeRequest(t, http.MethodGet, "/api/protected", nil, headers)

	ts.JWTAuth(handler).ServeHTTP(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusUnauthorized)
	AssertError(t, rec, http.StatusUnauthorized, "invalid authorization header format", "unauthorized")
}

func TestJWTAuthUnsupportedAuthType(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with unsupported auth type")
	})

	headers := map[string]string{
		"Authorization": "Basic dXNlcjpwYXNz",
	}
	rec, req := MakeRequest(t, http.MethodGet, "/api/protected", nil, headers)

	ts.JWTAuth(handler).ServeHTTP(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusUnauthorized)
	AssertError(t, rec, http.StatusUnauthorized, "unsupported authorization type", "unauthorized")
}

func TestJWTAuthWithAPIKey(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "apikey@example.com", "password123")

	// Create an API key for the user
	key, err := ts.DB.CreateAPIKey(context.Background(), userID, "Test Key", nil)
	if err != nil {
		t.Fatalf("Failed to create API key: %v", err)
	}

	var gotUserID int64
	var gotEmail string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUserID, _ = r.Context().Value(UserIDKey).(int64)
		gotEmail, _ = r.Context().Value(UserEmailKey).(string)
		w.WriteHeader(http.StatusOK)
	})

	headers := map[string]string{
		"Authorization": fmt.Sprintf("ApiKey %s", key.Key),
	}
	rec, req := MakeRequest(t, http.MethodGet, "/api/protected", nil, headers)

	ts.JWTAuth(handler).ServeHTTP(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	if gotUserID != userID {
		t.Errorf("Expected UserIDKey=%d in context, got %d", userID, gotUserID)
	}
	if gotEmail != "apikey@example.com" {
		t.Errorf("Expected UserEmailKey=%q in context, got %q", "apikey@example.com", gotEmail)
	}
}

func TestJWTAuthWithInvalidAPIKey(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Handler should not be called with invalid API key")
	})

	headers := map[string]string{
		"Authorization": "ApiKey invalid-api-key-here",
	}
	rec, req := MakeRequest(t, http.MethodGet, "/api/protected", nil, headers)

	ts.JWTAuth(handler).ServeHTTP(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusUnauthorized)
	AssertError(t, rec, http.StatusUnauthorized, "invalid or expired API key", "unauthorized")
}

func TestRateLimitMiddleware(t *testing.T) {
	// Create a rate limiter that allows only 5 requests per minute
	limiter := RateLimitMiddleware(5)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	limitedHandler := limiter(handler)

	// First 5 requests should succeed (capacity = 5)
	for i := 0; i < 5; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.RemoteAddr = "192.168.1.100:12345"
		rec := httptest.NewRecorder()

		limitedHandler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Request %d: expected status 200, got %d", i+1, rec.Code)
		}
	}

	// 6th request should be rate limited
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "192.168.1.100:12345"
	rec := httptest.NewRecorder()

	limitedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Expected status 429, got %d", rec.Code)
	}

	var errResp ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&errResp); err != nil {
		t.Fatalf("Failed to decode error response: %v", err)
	}
	if errResp.Code != "rate_limit_exceeded" {
		t.Errorf("Expected error code 'rate_limit_exceeded', got %q", errResp.Code)
	}
}

func TestRateLimitMiddlewareDifferentIPs(t *testing.T) {
	limiter := RateLimitMiddleware(2)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	limitedHandler := limiter(handler)

	// Use up the rate limit for IP1
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
		req.RemoteAddr = "10.0.0.1:12345"
		rec := httptest.NewRecorder()
		limitedHandler.ServeHTTP(rec, req)
		if rec.Code != http.StatusOK {
			t.Errorf("IP1 request %d: expected 200, got %d", i+1, rec.Code)
		}
	}

	// IP1 should now be rate limited
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	rec := httptest.NewRecorder()
	limitedHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("IP1 should be rate limited, got %d", rec.Code)
	}

	// IP2 should still be allowed
	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	rec = httptest.NewRecorder()
	limitedHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("IP2 should not be rate limited, got %d", rec.Code)
	}
}

func TestRateLimitMiddlewareXForwardedFor(t *testing.T) {
	limiter := RateLimitMiddleware(1)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	limitedHandler := limiter(handler)

	// First request with X-Forwarded-For header
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	rec := httptest.NewRecorder()
	limitedHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("First request should succeed, got %d", rec.Code)
	}

	// Second request from the same forwarded IP should be limited
	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.50, 70.41.3.18")
	rec = httptest.NewRecorder()
	limitedHandler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Second request should be rate limited, got %d", rec.Code)
	}

	// X-Forwarded-For can be forged, so a new value from the same connection is still limited.
	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "10.0.0.1:12346"
	req.Header.Set("X-Forwarded-For", "198.51.100.7")
	rec = httptest.NewRecorder()
	limitedHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("A forged X-Forwarded-For must not get a new bucket, got %d", rec.Code)
	}
}

func TestRateLimitMiddlewareXRealIP(t *testing.T) {
	limiter := RateLimitMiddleware(1)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	limitedHandler := limiter(handler)

	// First request with X-Real-IP header
	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Real-IP", "198.51.100.10")
	rec := httptest.NewRecorder()
	limitedHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Errorf("First request should succeed, got %d", rec.Code)
	}

	// Second request from the same real IP should be limited
	req = httptest.NewRequest(http.MethodGet, "/api/test", nil)
	req.RemoteAddr = "10.0.0.1:12345"
	req.Header.Set("X-Real-IP", "198.51.100.10")
	rec = httptest.NewRecorder()
	limitedHandler.ServeHTTP(rec, req)
	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Second request should be rate limited, got %d", rec.Code)
	}
}

func TestLoggerMiddleware(t *testing.T) {
	var handlerCalled bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("logged"))
	})

	loggedHandler := ZapLogger(zap.NewNop())(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	rec := httptest.NewRecorder()

	loggedHandler.ServeHTTP(rec, req)

	if !handlerCalled {
		t.Error("Expected handler to be called through logger middleware")
	}

	AssertStatusCode(t, rec.Code, http.StatusOK)

	if rec.Body.String() != "logged" {
		t.Errorf("Expected body 'logged', got %q", rec.Body.String())
	}
}

func TestLoggerMiddlewareCapturesStatusCode(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte("not found"))
	})

	loggedHandler := ZapLogger(zap.NewNop())(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/missing", nil)
	rec := httptest.NewRecorder()

	loggedHandler.ServeHTTP(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusNotFound)
}

func TestResponseWriterWriteHeader(t *testing.T) {
	rec := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rec, statusCode: http.StatusOK}

	rw.WriteHeader(http.StatusCreated)

	if rw.statusCode != http.StatusCreated {
		t.Errorf("Expected status code %d, got %d", http.StatusCreated, rw.statusCode)
	}
}

func TestGetUserID(t *testing.T) {
	tests := []struct {
		name    string
		ctx     context.Context
		wantID  int64
		wantOK  bool
	}{
		{
			name:   "valid user ID in context",
			ctx:    context.WithValue(context.Background(), UserIDKey, int64(42)),
			wantID: 42,
			wantOK: true,
		},
		{
			name:   "no user ID in context",
			ctx:    context.Background(),
			wantID: 0,
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(tt.ctx)
			gotID, gotOK := GetUserID(req)

			if gotID != tt.wantID {
				t.Errorf("GetUserID() ID = %d, want %d", gotID, tt.wantID)
			}
			if gotOK != tt.wantOK {
				t.Errorf("GetUserID() ok = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

func TestGetUserEmail(t *testing.T) {
	tests := []struct {
		name      string
		ctx       context.Context
		wantEmail string
		wantOK    bool
	}{
		{
			name:      "valid email in context",
			ctx:       context.WithValue(context.Background(), UserEmailKey, "user@example.com"),
			wantEmail: "user@example.com",
			wantOK:    true,
		},
		{
			name:      "no email in context",
			ctx:       context.Background(),
			wantEmail: "",
			wantOK:    false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil).WithContext(tt.ctx)
			gotEmail, gotOK := GetUserEmail(req)

			if gotEmail != tt.wantEmail {
				t.Errorf("GetUserEmail() email = %q, want %q", gotEmail, tt.wantEmail)
			}
			if gotOK != tt.wantOK {
				t.Errorf("GetUserEmail() ok = %v, want %v", gotOK, tt.wantOK)
			}
		})
	}
}

func TestTokenBucket(t *testing.T) {
	bucket := newTokenBucket(3, 1.0)

	// Should allow first 3 requests
	for i := 0; i < 3; i++ {
		if !bucket.allow() {
			t.Errorf("Request %d should be allowed", i+1)
		}
	}

	// 4th request should be denied
	if bucket.allow() {
		t.Error("Request 4 should be denied (bucket empty)")
	}
}

func TestUserRateLimitMiddlewareNonPositiveIsDefault(t *testing.T) {
	for _, n := range []int{0, -60} {
		limited := UserRateLimitMiddleware(n)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
		for i := 0; i < 600; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/sync/objects", nil).WithContext(context.WithValue(context.Background(), UserIDKey, int64(1)))
			rec := httptest.NewRecorder()
			limited.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Fatalf("limit %d: request %d got %d, want the default of 600 per minute", n, i+1, rec.Code)
			}
		}
		// The bucket refills 10 tokens per second, so a slow loop can earn a few more: drain until 429.
		var rec *httptest.ResponseRecorder
		for i := 0; i < 1000; i++ {
			req := httptest.NewRequest(http.MethodGet, "/api/sync/objects", nil).WithContext(context.WithValue(context.Background(), UserIDKey, int64(1)))
			rec = httptest.NewRecorder()
			limited.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				break
			}
		}
		if rec.Code != http.StatusTooManyRequests || rec.Header().Get("Retry-After") != "1" {
			t.Fatalf("limit %d: got %d Retry-After %q, want 429 and 1", n, rec.Code, rec.Header().Get("Retry-After"))
		}
	}
}

func TestUserRateLimitMiddleware(t *testing.T) {
	limited := UserRateLimitMiddleware(2)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	call := func(userID int64, ip string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/sync/objects", nil)
		req.RemoteAddr = ip
		if userID != 0 {
			req = req.WithContext(context.WithValue(req.Context(), UserIDKey, userID))
		}
		rec := httptest.NewRecorder()
		limited.ServeHTTP(rec, req)
		return rec
	}
	tests := []struct {
		name   string
		userID int64
		ip     string
		want   int
	}{
		{"user 1 first", 1, "10.0.0.1:1", http.StatusOK},
		{"user 1 second, other IP", 1, "10.0.0.2:1", http.StatusOK},
		{"user 1 third: one bucket per user, not per IP", 1, "10.0.0.3:1", http.StatusTooManyRequests},
		{"user 2 has its own bucket, same IP", 2, "10.0.0.1:1", http.StatusOK},
		{"no user", 0, "10.0.0.9:1", http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := call(tt.userID, tt.ip)
			if rec.Code != tt.want {
				t.Fatalf("status %d, want %d", rec.Code, tt.want)
			}
			if tt.want == http.StatusTooManyRequests && rec.Header().Get("Retry-After") != "30" {
				t.Fatalf("Retry-After %q, want 30 (2 per minute)", rec.Header().Get("Retry-After"))
			}
		})
	}
}

func TestAuthFailureLimit(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	userID := ts.CreateTestUser(t, "authfail@test.com", "password123")
	good := ts.GenerateTestToken(t, userID, "authfail@test.com")
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	h := AuthFailureLimit(3, ts.JWTAuth)(ok) // as main.go mounts it
	call := func(ip, auth string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/api/sync/objects", nil)
		req.RemoteAddr = ip
		if auth != "" {
			req.Header.Set("Authorization", auth)
		}
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec
	}
	tests := []struct {
		name, ip, auth string
		want           int
	}{
		{"good token costs nothing", "10.1.0.1:1", "Bearer " + good, http.StatusOK},
		{"good token again", "10.1.0.1:1", "Bearer " + good, http.StatusOK},
		{"good token again", "10.1.0.1:1", "Bearer " + good, http.StatusOK},
		{"good token again", "10.1.0.1:1", "Bearer " + good, http.StatusOK},
		{"bad key 1", "10.1.0.2:1", "ApiKey guess-1", http.StatusUnauthorized},
		{"no auth 2", "10.1.0.2:1", "", http.StatusUnauthorized},
		{"bad bearer 3", "10.1.0.2:1", "Bearer nope", http.StatusUnauthorized},
		{"4th failure from that IP is refused before the key is checked", "10.1.0.2:1", "ApiKey guess-4", http.StatusTooManyRequests},
		{"that IP waits even with a good token", "10.1.0.2:1", "Bearer " + good, http.StatusTooManyRequests},
		{"another IP is not affected", "10.1.0.3:1", "ApiKey guess-1", http.StatusUnauthorized},
		{"good IP still fine", "10.1.0.1:1", "Bearer " + good, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := call(tt.ip, tt.auth)
			if rec.Code != tt.want {
				t.Fatalf("status %d, want %d: %s", rec.Code, tt.want, rec.Body.String())
			}
			if tt.want == http.StatusTooManyRequests && rec.Header().Get("Retry-After") != "20" {
				t.Fatalf("Retry-After %q, want 20 (3 per minute)", rec.Header().Get("Retry-After"))
			}
		})
	}
}

func TestAuthFailureLimitConcurrentGuesses(t *testing.T) {
	// A slow auth that always fails: the guesses that get a token wait here until all requests have arrived.
	release := make(chan struct{})
	entered := make(chan struct{}, 50)
	slowFail := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			entered <- struct{}{}
			<-release
			respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		})
	}
	h := AuthFailureLimit(3, slowFail)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	const n = 50
	codes := make(chan int, n)
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			req := httptest.NewRequest(http.MethodGet, "/api/sync/objects", nil)
			req.RemoteAddr = fmt.Sprintf("10.2.0.1:%d", 40000+i) // a new source port per connection
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, req)
			codes <- rec.Code
		}(i)
	}
	for i := 0; i < 3; i++ {
		<-entered // the three guesses that got a token are inside auth
	}
	// The other 47 are refused without waiting for auth; collect them before auth answers.
	refused := 0
	for i := 0; i < n-3; i++ {
		if c := <-codes; c == http.StatusTooManyRequests {
			refused++
		} else {
			t.Fatalf("a guess beyond the budget got %d, want 429", c)
		}
	}
	close(release)
	wg.Wait()
	close(codes)
	checked := 0
	for c := range codes {
		if c == http.StatusUnauthorized {
			checked++
		}
	}
	if checked != 3 || refused != n-3 {
		t.Fatalf("%d guesses reached auth and %d were refused, want 3 and %d", checked, refused, n-3)
	}
}

func TestAuthFailureLimitSharedAcrossGroups(t *testing.T) {
	fail := func(http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		})
	}
	limited := AuthFailureLimit(3, fail) // one instance, as main.go uses for the sync and protected groups
	syncGroup := limited(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	protected := limited(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	call := func(h http.Handler) int {
		req := httptest.NewRequest(http.MethodGet, "/api/x", nil)
		req.RemoteAddr = "10.3.0.1:1"
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	got := []int{call(syncGroup), call(syncGroup), call(protected), call(protected), call(syncGroup)}
	want := []int{401, 401, 401, 429, 429}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("request %d: got %v, want %v", i+1, got, want)
		}
	}
}

func TestClientIP(t *testing.T) {
	tests := []struct {
		name, remote, xff, xri, want string
	}{
		{"port is removed", "10.0.0.1:40001", "", "", "10.0.0.1"},
		{"IPv6 port is removed", "[2001:db8::1]:443", "", "", "2001:db8::1"},
		{"X-Forwarded-For is ignored (forgeable)", "10.0.0.1:1", " 1.2.3.4 , 5.6.7.8", "", "10.0.0.1"},
		{"blank X-Forwarded-For is ignored", "10.0.0.1:1", " ", "", "10.0.0.1"},
		{"X-Real-IP wins, trimmed", "10.0.0.1:1", "1.2.3.4", " 9.9.9.9 ", "9.9.9.9"},
		{"blank X-Real-IP falls back", "10.0.0.1:1", "", " ", "10.0.0.1"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/", nil)
			req.RemoteAddr = tt.remote
			if tt.xff != "" {
				req.Header.Set("X-Forwarded-For", tt.xff)
			}
			if tt.xri != "" {
				req.Header.Set("X-Real-IP", tt.xri)
			}
			if got := clientIP(req); got != tt.want {
				t.Fatalf("clientIP = %q, want %q", got, tt.want)
			}
		})
	}
}
