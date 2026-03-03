package e2e

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"

	"taskai/internal/api"
	"taskai/internal/config"
	"taskai/internal/db"
)

// TestServer represents an in-process test server
type TestServer struct {
	Server  *http.Server
	BaseURL string
	DB      *sql.DB
	Client  *http.Client
	cleanup func()
}

// NewTestServer creates a new test server with in-memory database
func NewTestServer(t *testing.T) *TestServer {
	t.Helper()

	// Create temporary database
	tmpDB := fmt.Sprintf(":memory:")

	// Create server configuration
	cfg := &config.Config{
		Port:               "0", // Random port
		Env:                "test",
		DBPath:             tmpDB,
		MigrationsPath:     "../db/migrations",
		JWTSecret:          "test-secret-key-for-e2e-tests",
		JWTExpiryHours:     24,
		CORSAllowedOrigins: []string{"*"},
		LogLevel:           "error",
	}

	// Initialize logger
	logger := config.MustInitLogger(cfg.Env, cfg.LogLevel)

	// Initialize database
	dbCfg := db.Config{
		Driver:         "sqlite",
		DBPath:         tmpDB,
		DSN:            "",
		MigrationsPath: "../db/migrations",
	}

	database, err := db.New(dbCfg, logger)
	if err != nil {
		t.Fatalf("Failed to create database: %v", err)
	}

	// Create API server
	server := api.NewServer(database, cfg, logger)

	// Setup router
	r := chi.NewRouter()

	// Middleware stack
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Use(middleware.Timeout(30 * time.Second))

	// CORS configuration for tests
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins:   []string{"*"},
		AllowedMethods:   []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"},
		AllowedHeaders:   []string{"Accept", "Authorization", "Content-Type"},
		ExposedHeaders:   []string{"Link"},
		AllowCredentials: true,
		MaxAge:           300,
	}))

	// Public routes
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"message":"TaskAI API","version":"0.1.0"}`)
	})

	r.Get("/healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()

		if err := database.HealthCheck(ctx); err != nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusServiceUnavailable)
			fmt.Fprintf(w, `{"status":"error","message":"database unavailable"}`)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		fmt.Fprintf(w, `{"status":"ok","database":"connected"}`)
	})

	// API routes
	r.Route("/api", func(r chi.Router) {
		r.Get("/openapi", server.HandleOpenAPI)

		// Auth routes (public)
		r.Route("/auth", func(r chi.Router) {
			r.Post("/signup", server.HandleSignup)
			r.Post("/login", server.HandleLogin)
		})

		// Protected routes
		r.Group(func(r chi.Router) {
			r.Use(server.JWTAuth)

			r.Get("/me", server.HandleMe)

			// Project routes
			r.Get("/projects", server.HandleListProjects)
			r.Post("/projects", server.HandleCreateProject)
			r.Get("/projects/{id}", server.HandleGetProject)
			r.Patch("/projects/{id}", server.HandleUpdateProject)
			r.Delete("/projects/{id}", server.HandleDeleteProject)

			// Task routes
			r.Get("/projects/{projectId}/tasks", server.HandleListTasks)
			r.Post("/projects/{projectId}/tasks", server.HandleCreateTask)
			r.Patch("/tasks/{id}", server.HandleUpdateTask)
			r.Delete("/tasks/{id}", server.HandleDeleteTask)

			// Team routes
			r.Route("/team", func(r chi.Router) {
				r.Get("/", server.HandleGetMyTeam)
				r.Get("/members", server.HandleGetTeamMembers)
				r.Post("/invitations", server.HandleInviteTeamMember)
				r.Get("/invitations", server.HandleGetMyInvitations)
				r.Post("/invitations/{id}/accept", server.HandleAcceptInvitation)
				r.Post("/invitations/{id}/reject", server.HandleRejectInvitation)
				r.Delete("/members/{memberId}", server.HandleRemoveTeamMember)
			})
		})
	})

	// Find available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("Failed to find available port: %v", err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()

	// Create HTTP server
	httpServer := &http.Server{
		Addr:    fmt.Sprintf(":%d", port),
		Handler: r,
	}

	// Start server in background
	go func() {
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			t.Logf("Server error: %v", err)
		}
	}()

	// Wait for server to be ready
	baseURL := fmt.Sprintf("http://localhost:%d", port)
	client := &http.Client{Timeout: 10 * time.Second}
	
	for i := 0; i < 50; i++ {
		resp, err := client.Get(baseURL + "/healthz")
		if err == nil {
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				break
			}
		}
		time.Sleep(10 * time.Millisecond)
	}

	cleanup := func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		httpServer.Shutdown(ctx)
		database.Close()
	}

	return &TestServer{
		Server:  httpServer,
		BaseURL: baseURL,
		DB:      database.DB,
		Client:  client,
		cleanup: cleanup,
	}
}

// Close cleans up the test server
func (ts *TestServer) Close() {
	if ts.cleanup != nil {
		ts.cleanup()
	}
}

// Helper functions for API calls

// DoRequest makes an HTTP request and returns the response
func (ts *TestServer) DoRequest(method, path string, body interface{}, token string) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, err
		}
		bodyReader = bytes.NewBuffer(jsonBody)
	}

	req, err := http.NewRequest(method, ts.BaseURL+path, bodyReader)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}

	return ts.Client.Do(req)
}

// ParseJSON parses JSON response
func ParseJSON(resp *http.Response, v interface{}) error {
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(v)
}

// Signup creates a new user
func (ts *TestServer) Signup(email, password, inviteCode string) (string, map[string]interface{}, error) {
	body := map[string]string{
		"email":       email,
		"password":    password,
		"invite_code": inviteCode,
	}

	resp, err := ts.DoRequest("POST", "/api/auth/signup", body, "")
	if err != nil {
		return "", nil, err
	}

	if resp.StatusCode != http.StatusCreated {
		return "", nil, fmt.Errorf("signup failed with status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := ParseJSON(resp, &result); err != nil {
		return "", nil, err
	}

	token, ok := result["token"].(string)
	if !ok {
		return "", nil, fmt.Errorf("no token in response")
	}

	return token, result, nil
}

// CreateTestInvite creates a test invite code in the database
func (ts *TestServer) CreateTestInvite() (string, error) {
	// Create a test admin user first
	ctx := context.Background()

	// Insert directly into database to bypass invite requirement
	_, err := ts.DB.ExecContext(ctx, `
		INSERT INTO users (email, password_hash, is_admin, invite_count, created_at, updated_at)
		VALUES (?, ?, 1, 999, datetime('now'), datetime('now'))
	`, "admin@test.com", "$2a$10$dummy_hash_for_testing")
	if err != nil {
		return "", fmt.Errorf("failed to create admin user: %w", err)
	}

	// Get the admin user ID
	var adminID int64
	err = ts.DB.QueryRowContext(ctx, "SELECT id FROM users WHERE email = ?", "admin@test.com").Scan(&adminID)
	if err != nil {
		return "", fmt.Errorf("failed to get admin ID: %w", err)
	}

	// Create invite code
	inviteCode := "test-invite-code-123"
	expiresAt := time.Now().Add(24 * time.Hour)

	_, err = ts.DB.ExecContext(ctx, `
		INSERT INTO invites (code, inviter_id, expires_at, created_at)
		VALUES (?, ?, ?, datetime('now'))
	`, inviteCode, adminID, expiresAt)
	if err != nil {
		return "", fmt.Errorf("failed to create invite: %w", err)
	}

	return inviteCode, nil
}

// Login authenticates a user
func (ts *TestServer) Login(email, password string) (string, map[string]interface{}, error) {
	body := map[string]string{
		"email":    email,
		"password": password,
	}

	resp, err := ts.DoRequest("POST", "/api/auth/login", body, "")
	if err != nil {
		return "", nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return "", nil, fmt.Errorf("login failed with status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := ParseJSON(resp, &result); err != nil {
		return "", nil, err
	}

	token, ok := result["token"].(string)
	if !ok {
		return "", nil, fmt.Errorf("no token in response")
	}

	return token, result, nil
}

// CreateProject creates a new project
func (ts *TestServer) CreateProject(token, name, description string) (map[string]interface{}, error) {
	body := map[string]string{
		"name":        name,
		"description": description,
	}

	resp, err := ts.DoRequest("POST", "/api/projects", body, token)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("create project failed with status %d", resp.StatusCode)
	}

	var result map[string]interface{}
	if err := ParseJSON(resp, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// GetProjects retrieves all projects
func (ts *TestServer) GetProjects(token string) ([]map[string]interface{}, error) {
	resp, err := ts.DoRequest("GET", "/api/projects", nil, token)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("get projects failed with status %d", resp.StatusCode)
	}

	var result []map[string]interface{}
	if err := ParseJSON(resp, &result); err != nil {
		return nil, err
	}

	return result, nil
}

// Tests

func TestMain(m *testing.M) {
	// Run tests
	code := m.Run()
	os.Exit(code)
}

func TestCompleteUserJourney(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create test invite code
	inviteCode, err := ts.CreateTestInvite()
	if err != nil {
		t.Fatalf("Failed to create test invite: %v", err)
	}

	// Test data
	email := fmt.Sprintf("test-%d@example.com", time.Now().UnixNano())
	password := "TestPassword123!"

	var token string
	var userID float64

	t.Run("Signup", func(t *testing.T) {
		tok, result, err := ts.Signup(email, password, inviteCode)
		if err != nil {
			t.Fatalf("Signup failed: %v", err)
		}

		token = tok
		
		user, ok := result["user"].(map[string]interface{})
		if !ok {
			t.Fatal("No user in signup response")
		}

		userID = user["id"].(float64)
		
		if user["email"].(string) != email {
			t.Errorf("Expected email %s, got %s", email, user["email"])
		}
	})

	t.Run("Login", func(t *testing.T) {
		tok, result, err := ts.Login(email, password)
		if err != nil {
			t.Fatalf("Login failed: %v", err)
		}

		if tok == "" {
			t.Fatal("Login did not return token")
		}

		user, ok := result["user"].(map[string]interface{})
		if !ok {
			t.Fatal("No user in login response")
		}

		if user["email"].(string) != email {
			t.Errorf("Expected email %s, got %s", email, user["email"])
		}
	})

	t.Run("GetCurrentUser", func(t *testing.T) {
		resp, err := ts.DoRequest("GET", "/api/me", nil, token)
		if err != nil {
			t.Fatalf("Get current user failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var user map[string]interface{}
		if err := ParseJSON(resp, &user); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if user["id"].(float64) != userID {
			t.Errorf("Expected user ID %v, got %v", userID, user["id"])
		}
	})

	var project1ID float64
	var project2ID float64

	t.Run("CreateFirstProject", func(t *testing.T) {
		project, err := ts.CreateProject(token, "E2E Test Project 1", "First test project")
		if err != nil {
			t.Fatalf("Create project failed: %v", err)
		}

		project1ID = project["id"].(float64)

		if project["name"].(string) != "E2E Test Project 1" {
			t.Errorf("Expected name 'E2E Test Project 1', got %s", project["name"])
		}
	})

	t.Run("CreateSecondProject", func(t *testing.T) {
		project, err := ts.CreateProject(token, "E2E Test Project 2", "Second test project")
		if err != nil {
			t.Fatalf("Create project failed: %v", err)
		}

		project2ID = project["id"].(float64)
	})

	t.Run("ListProjects", func(t *testing.T) {
		projects, err := ts.GetProjects(token)
		if err != nil {
			t.Fatalf("Get projects failed: %v", err)
		}

		if len(projects) < 2 {
			t.Fatalf("Expected at least 2 projects, got %d", len(projects))
		}

		// Verify our projects are in the list
		found1, found2 := false, false
		for _, p := range projects {
			id := p["id"].(float64)
			if id == project1ID {
				found1 = true
			}
			if id == project2ID {
				found2 = true
			}
		}

		if !found1 || !found2 {
			t.Error("Created projects not found in list")
		}
	})

	t.Run("GetProject", func(t *testing.T) {
		path := fmt.Sprintf("/api/projects/%d", int(project1ID))
		resp, err := ts.DoRequest("GET", path, nil, token)
		if err != nil {
			t.Fatalf("Get project failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var project map[string]interface{}
		if err := ParseJSON(resp, &project); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if project["name"].(string) != "E2E Test Project 1" {
			t.Errorf("Expected name 'E2E Test Project 1', got %s", project["name"])
		}
	})

	t.Run("UpdateProject", func(t *testing.T) {
		path := fmt.Sprintf("/api/projects/%d", int(project1ID))
		body := map[string]string{
			"name": "Updated Project Name",
		}

		resp, err := ts.DoRequest("PATCH", path, body, token)
		if err != nil {
			t.Fatalf("Update project failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var project map[string]interface{}
		if err := ParseJSON(resp, &project); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if project["name"].(string) != "Updated Project Name" {
			t.Errorf("Expected name 'Updated Project Name', got %s", project["name"])
		}
	})

	var taskID float64

	t.Run("CreateTask", func(t *testing.T) {
		path := fmt.Sprintf("/api/projects/%d/tasks", int(project1ID))
		body := map[string]interface{}{
			"title":       "Test Task",
			"description": "Task description",
			"status":      "todo",
		}

		resp, err := ts.DoRequest("POST", path, body, token)
		if err != nil {
			t.Fatalf("Create task failed: %v", err)
		}

		if resp.StatusCode != http.StatusCreated {
			t.Fatalf("Expected status 201, got %d", resp.StatusCode)
		}

		var task map[string]interface{}
		if err := ParseJSON(resp, &task); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		taskID = task["id"].(float64)

		if task["title"].(string) != "Test Task" {
			t.Errorf("Expected title 'Test Task', got %s", task["title"])
		}
	})

	t.Run("ListTasks", func(t *testing.T) {
		path := fmt.Sprintf("/api/projects/%d/tasks", int(project1ID))
		resp, err := ts.DoRequest("GET", path, nil, token)
		if err != nil {
			t.Fatalf("Get tasks failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var tasks []map[string]interface{}
		if err := ParseJSON(resp, &tasks); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if len(tasks) == 0 {
			t.Fatal("Expected at least 1 task")
		}
	})

	t.Run("UpdateTask", func(t *testing.T) {
		path := fmt.Sprintf("/api/tasks/%d", int(taskID))
		body := map[string]interface{}{
			"status": "in_progress",
		}

		resp, err := ts.DoRequest("PATCH", path, body, token)
		if err != nil {
			t.Fatalf("Update task failed: %v", err)
		}

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("Expected status 200, got %d", resp.StatusCode)
		}

		var task map[string]interface{}
		if err := ParseJSON(resp, &task); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		if task["status"].(string) != "in_progress" {
			t.Errorf("Expected status 'in_progress', got %s", task["status"])
		}
	})

	t.Run("DeleteTask", func(t *testing.T) {
		path := fmt.Sprintf("/api/tasks/%d", int(taskID))
		resp, err := ts.DoRequest("DELETE", path, nil, token)
		if err != nil {
			t.Fatalf("Delete task failed: %v", err)
		}

		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("Expected status 204, got %d", resp.StatusCode)
		}
	})

	t.Run("DeleteProject", func(t *testing.T) {
		path := fmt.Sprintf("/api/projects/%d", int(project2ID))
		resp, err := ts.DoRequest("DELETE", path, nil, token)
		if err != nil {
			t.Fatalf("Delete project failed: %v", err)
		}

		if resp.StatusCode != http.StatusNoContent {
			t.Fatalf("Expected status 204, got %d", resp.StatusCode)
		}

		// Verify project is deleted
		projects, err := ts.GetProjects(token)
		if err != nil {
			t.Fatalf("Get projects failed: %v", err)
		}

		for _, p := range projects {
			if p["id"].(float64) == project2ID {
				t.Error("Deleted project still appears in list")
			}
		}
	})
}

func TestAuthorizationChecks(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create test invite codes
	inviteCode1, err := ts.CreateTestInvite()
	if err != nil {
		t.Fatalf("Failed to create test invite 1: %v", err)
	}

	// Create two users
	user1Email := fmt.Sprintf("user1-%d@example.com", time.Now().UnixNano())
	user2Email := fmt.Sprintf("user2-%d@example.com", time.Now().UnixNano()+1)
	password := "TestPassword123!"

	token1, _, err := ts.Signup(user1Email, password, inviteCode1)
	if err != nil {
		t.Fatalf("User 1 signup failed: %v", err)
	}

	// Create second invite for user 2
	inviteCode2 := inviteCode1 + "-2"
	ctx := context.Background()
	var adminID int64
	ts.DB.QueryRowContext(ctx, "SELECT id FROM users WHERE email = ?", "admin@test.com").Scan(&adminID)
	ts.DB.ExecContext(ctx, `INSERT INTO invites (code, inviter_id, expires_at, created_at) VALUES (?, ?, ?, datetime('now'))`,
		inviteCode2, adminID, time.Now().Add(24*time.Hour))

	token2, _, err := ts.Signup(user2Email, password, inviteCode2)
	if err != nil {
		t.Fatalf("User 2 signup failed: %v", err)
	}

	// User 1 creates a project
	project, err := ts.CreateProject(token1, "User 1 Project", "Private project")
	if err != nil {
		t.Fatalf("Create project failed: %v", err)
	}

	projectID := int(project["id"].(float64))

	t.Run("UserCannotAccessOtherUsersProject", func(t *testing.T) {
		path := fmt.Sprintf("/api/projects/%d", projectID)
		resp, err := ts.DoRequest("GET", path, nil, token2)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", resp.StatusCode)
		}
	})

	t.Run("UserCannotUpdateOtherUsersProject", func(t *testing.T) {
		path := fmt.Sprintf("/api/projects/%d", projectID)
		body := map[string]string{"name": "Hacked Project"}

		resp, err := ts.DoRequest("PATCH", path, body, token2)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("Expected status 404, got %d", resp.StatusCode)
		}
	})

	t.Run("UserCannotDeleteOtherUsersProject", func(t *testing.T) {
		path := fmt.Sprintf("/api/projects/%d", projectID)
		resp, err := ts.DoRequest("DELETE", path, nil, token2)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		// Should return 403 (Forbidden) because only project owner can delete
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("Expected status 403, got %d", resp.StatusCode)
		}
	})
}

func TestValidationErrors(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create test invite code
	inviteCode, err := ts.CreateTestInvite()
	if err != nil {
		t.Fatalf("Failed to create test invite: %v", err)
	}

	email := fmt.Sprintf("validation-test-%d@example.com", time.Now().UnixNano())
	password := "TestPassword123!"

	token, _, err := ts.Signup(email, password, inviteCode)
	if err != nil {
		t.Fatalf("Signup failed: %v", err)
	}

	t.Run("CreateProjectWithoutName", func(t *testing.T) {
		body := map[string]string{
			"description": "No name provided",
		}

		resp, err := ts.DoRequest("POST", "/api/projects", body, token)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})

	t.Run("CreateTaskWithInvalidStatus", func(t *testing.T) {
		// First create a project
		project, err := ts.CreateProject(token, "Test Project", "")
		if err != nil {
			t.Fatalf("Create project failed: %v", err)
		}

		projectID := int(project["id"].(float64))
		path := fmt.Sprintf("/api/projects/%d/tasks", projectID)

		body := map[string]interface{}{
			"title":  "Test Task",
			"status": "invalid_status",
		}

		resp, err := ts.DoRequest("POST", path, body, token)
		if err != nil {
			t.Fatalf("Request failed: %v", err)
		}

		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("Expected status 400, got %d", resp.StatusCode)
		}
	})
}

func TestTeamProjectAccess(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create test invite codes
	inviteCode1, err := ts.CreateTestInvite()
	if err != nil {
		t.Fatalf("Failed to create test invite 1: %v", err)
	}

	// Create first user
	user1Email := fmt.Sprintf("user1-%d@example.com", time.Now().UnixNano())
	user1Token, _, err := ts.Signup(user1Email, "password123", inviteCode1)
	if err != nil {
		t.Fatalf("Signup user1 failed: %v", err)
	}

	// Create second invite for user 2
	inviteCode2 := inviteCode1 + "-team"
	ctx := context.Background()
	var adminID int64
	ts.DB.QueryRowContext(ctx, "SELECT id FROM users WHERE email = ?", "admin@test.com").Scan(&adminID)
	ts.DB.ExecContext(ctx, `INSERT INTO invites (code, inviter_id, expires_at, created_at) VALUES (?, ?, ?, datetime('now'))`,
		inviteCode2, adminID, time.Now().Add(24*time.Hour))

	// Create a second user
	user2Email := fmt.Sprintf("user2-%d@example.com", time.Now().UnixNano())
	user2Token, _, err := ts.Signup(user2Email, "password123", inviteCode2)
	if err != nil {
		t.Fatalf("Signup user2 failed: %v", err)
	}

	// User1 creates a project
	project, err := ts.CreateProject(user1Token, "Shared Project", "Should be visible to team members")
	if err != nil {
		t.Fatalf("Create project failed: %v", err)
	}
	projectID := int64(project["id"].(float64))

	// User1 invites user2 to their team
	inviteBody := map[string]string{"email": user2Email}
	inviteResp, err := ts.DoRequest("POST", "/api/team/invitations", inviteBody, user1Token)
	if err != nil {
		t.Fatalf("Invite failed: %v", err)
	}
	if inviteResp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(inviteResp.Body)
		t.Fatalf("Expected status 201, got %d: %s", inviteResp.StatusCode, string(body))
	}

	// User2 gets their invitations
	listInvResp, err := ts.DoRequest("GET", "/api/team/invitations", nil, user2Token)
	if err != nil {
		t.Fatalf("List invitations failed: %v", err)
	}
	var invitations []map[string]interface{}
	if err := json.NewDecoder(listInvResp.Body).Decode(&invitations); err != nil {
		t.Fatalf("Decode invitations failed: %v", err)
	}
	if len(invitations) == 0 {
		t.Fatalf("Expected at least one invitation, got 0")
	}
	invitationID := int(invitations[0]["id"].(float64))

	// User2 accepts the invitation
	acceptPath := fmt.Sprintf("/api/team/invitations/%d/accept", invitationID)
	acceptResp, err := ts.DoRequest("POST", acceptPath, nil, user2Token)
	if err != nil {
		t.Fatalf("Accept invitation failed: %v", err)
	}
	if acceptResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(acceptResp.Body)
		t.Fatalf("Expected status 200, got %d: %s", acceptResp.StatusCode, string(body))
	}

	// User2 should NOT see the project yet — team membership alone grants no project access
	listProjectsResp, err := ts.DoRequest("GET", "/api/projects", nil, user2Token)
	if err != nil {
		t.Fatalf("List projects failed: %v", err)
	}
	var projects []map[string]interface{}
	if err := json.NewDecoder(listProjectsResp.Body).Decode(&projects); err != nil {
		t.Fatalf("Decode projects failed: %v", err)
	}

	for _, p := range projects {
		if int64(p["id"].(float64)) == projectID {
			t.Errorf("User2 should NOT see the project before being explicitly added (found project ID %d)", projectID)
			break
		}
	}

	// User1 creates a new project after user2 joined — user2 should NOT see this either
	newProject, err := ts.CreateProject(user1Token, "New Project After Join", "Created after user2 joined team")
	if err != nil {
		t.Fatalf("Create new project failed: %v", err)
	}
	newProjectID := int64(newProject["id"].(float64))

	listProjectsResp2, err := ts.DoRequest("GET", "/api/projects", nil, user2Token)
	if err != nil {
		t.Fatalf("List projects failed: %v", err)
	}
	var projects2 []map[string]interface{}
	if err := json.NewDecoder(listProjectsResp2.Body).Decode(&projects2); err != nil {
		t.Fatalf("Decode projects failed: %v", err)
	}

	for _, p := range projects2 {
		if int64(p["id"].(float64)) == newProjectID {
			t.Errorf("User2 should NOT see the new project before being explicitly added (found project ID %d)", newProjectID)
			break
		}
	}
}
