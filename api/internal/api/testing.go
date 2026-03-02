package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap/zaptest"

	"golang.org/x/crypto/bcrypt"

	"taskai/internal/auth"
	"taskai/internal/config"
	"taskai/internal/db"
)

// TestServer holds test server dependencies
type TestServer struct {
	*Server
	DB *db.DB
}

// NewTestServer creates a new test server with in-memory SQLite database.
// Accepts testing.TB so it can be used from both tests and benchmarks.
func NewTestServer(t testing.TB) *TestServer {
	t.Helper()

	// Use fast bcrypt for tests (MinCost=4 vs production Cost=12)
	auth.SetBcryptCost(bcrypt.MinCost)

	// Create test logger
	logger := zaptest.NewLogger(t)

	// Create in-memory database
	cfg := db.Config{
		DBPath:         ":memory:",
		MigrationsPath: "./../../internal/db/migrations",
	}

	database, err := db.New(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Create test config
	testCfg := &config.Config{
		JWTSecret:      "test-secret-key",
		JWTExpiryHours: 24,
	}

	server := NewServer(database, testCfg, logger)

	return &TestServer{
		Server: server,
		DB:     database,
	}
}

// Close cleans up test server resources
func (ts *TestServer) Close() {
	if ts.DB != nil {
		ts.DB.Close()
	}
}

// CreateTestUser creates a user for testing and returns the user ID
func (ts *TestServer) CreateTestUser(t testing.TB, email, password string) int64 {
	t.Helper()

	hashedPassword, err := auth.HashPassword(password)
	if err != nil {
		t.Fatalf("Failed to hash password: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	query := `INSERT INTO users (email, password_hash) VALUES (?, ?) RETURNING id`
	var userID int64
	err = ts.DB.QueryRowContext(ctx, query, email, hashedPassword).Scan(&userID)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}

	return userID
}

// CreateTestInvite creates a valid invite code for testing and returns the code
func (ts *TestServer) CreateTestInvite(t testing.TB, inviterID int64) string {
	t.Helper()

	code := "test-invite-" + fmt.Sprintf("%d", time.Now().UnixNano())

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ts.DB.ExecContext(ctx,
		`INSERT INTO invites (code, inviter_id) VALUES (?, ?)`,
		code, inviterID,
	)
	if err != nil {
		t.Fatalf("Failed to create test invite: %v", err)
	}

	return code
}

// GenerateTestToken generates a JWT token for testing
func (ts *TestServer) GenerateTestToken(t testing.TB, userID int64, email string) string {
	t.Helper()

	token, err := auth.GenerateToken(userID, email, ts.config.JWTSecret, ts.config.JWTExpiry())
	if err != nil {
		t.Fatalf("Failed to generate test token: %v", err)
	}

	return token
}

// MakeRequest is a helper to make HTTP requests in tests
// Returns both the ResponseRecorder and the Request for testing
func MakeRequest(t testing.TB, method, path string, body interface{}, headers map[string]string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()

	var reqBody []byte
	var err error
	if body != nil {
		reqBody, err = json.Marshal(body)
		if err != nil {
			t.Fatalf("Failed to marshal request body: %v", err)
		}
	}

	req := httptest.NewRequest(method, path, bytes.NewBuffer(reqBody))
	req.Header.Set("Content-Type", "application/json")

	for key, value := range headers {
		req.Header.Set(key, value)
	}

	return httptest.NewRecorder(), req
}

// DecodeJSON decodes a JSON response into the provided interface
func DecodeJSON(t testing.TB, rec *httptest.ResponseRecorder, v interface{}) {
	t.Helper()

	if err := json.NewDecoder(rec.Body).Decode(v); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}
}

// AssertStatusCode checks if the response status code matches expected
func AssertStatusCode(t testing.TB, got, want int) {
	t.Helper()

	if got != want {
		t.Errorf("Status code mismatch: got %d, want %d", got, want)
	}
}

// AssertJSONField checks if a JSON field has the expected value
func AssertJSONField(t testing.TB, data map[string]interface{}, field string, want interface{}) {
	t.Helper()

	got, ok := data[field]
	if !ok {
		t.Errorf("Field %q not found in response", field)
		return
	}

	if got != want {
		t.Errorf("Field %q mismatch: got %v, want %v", field, got, want)
	}
}

// CreateTestProject creates a project and adds the owner as a project member with 'owner' role
func (ts *TestServer) CreateTestProject(t testing.TB, ownerID int64, name string) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO projects (owner_id, name, description) VALUES (?, ?, ?)`,
		ownerID, name, "Test project description",
	)
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get project ID: %v", err)
	}

	// Add owner as project member
	_, err = ts.DB.ExecContext(ctx,
		`INSERT INTO project_members (project_id, user_id, role, granted_by) VALUES (?, ?, 'owner', ?)`,
		id, ownerID, ownerID,
	)
	if err != nil {
		t.Fatalf("Failed to add project member: %v", err)
	}

	return id
}

// CreateTestTask creates a task in the given project and returns the task ID
func (ts *TestServer) CreateTestTask(t testing.TB, projectID int64, title string) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Get next task_number for this project
	var nextNumber int64
	err := ts.DB.QueryRowContext(ctx, `SELECT COALESCE(MAX(task_number), 0) + 1 FROM tasks WHERE project_id = ?`, projectID).Scan(&nextNumber)
	if err != nil {
		t.Fatalf("Failed to get next task number: %v", err)
	}

	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO tasks (project_id, task_number, title, status, priority) VALUES (?, ?, ?, 'todo', 'medium')`,
		projectID, nextNumber, title,
	)
	if err != nil {
		t.Fatalf("Failed to create test task: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get task ID: %v", err)
	}

	return id
}

// AddProjectMember adds a user as a project member with the specified role
func (ts *TestServer) AddProjectMember(t testing.TB, projectID, userID, grantedBy int64, role string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ts.DB.ExecContext(ctx,
		`INSERT INTO project_members (project_id, user_id, role, granted_by) VALUES (?, ?, ?, ?)`,
		projectID, userID, role, grantedBy,
	)
	if err != nil {
		t.Fatalf("Failed to add project member: %v", err)
	}
}

// MakeAuthRequest creates an HTTP request with auth context (UserIDKey) and optional chi URL params
func (ts *TestServer) MakeAuthRequest(t testing.TB, method, path string, body interface{}, userID int64, urlParams map[string]string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()

	rec, req := MakeRequest(t, method, path, body, nil)

	ctx := context.WithValue(req.Context(), UserIDKey, userID)

	if len(urlParams) > 0 {
		rctx := chi.NewRouteContext()
		for k, v := range urlParams {
			rctx.URLParams.Add(k, v)
		}
		ctx = context.WithValue(ctx, chi.RouteCtxKey, rctx)
	}

	req = req.WithContext(ctx)
	return rec, req
}

// AssertError checks if the error response matches expected error and code
func AssertError(t testing.TB, rec *httptest.ResponseRecorder, wantCode int, wantErrorContains, wantCodeContains string) {
	t.Helper()

	AssertStatusCode(t, rec.Code, wantCode)

	var errResp ErrorResponse
	DecodeJSON(t, rec, &errResp)

	if wantErrorContains != "" && !contains(errResp.Error, wantErrorContains) {
		t.Errorf("Error message %q does not contain %q", errResp.Error, wantErrorContains)
	}

	if wantCodeContains != "" && !contains(errResp.Code, wantCodeContains) {
		t.Errorf("Error code %q does not contain %q", errResp.Code, wantCodeContains)
	}
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || indexOf(s, substr) >= 0)
}

// indexOf returns the index of the first occurrence of substr in s, or -1
func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
