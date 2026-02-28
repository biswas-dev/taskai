package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/go-chi/chi/v5"
)

// createTestProjectForCloudinary creates a project owned by the given user and returns its ID
func createTestProjectForCloudinary(t *testing.T, ts *TestServer, ownerID int64) int64 {
	t.Helper()

	ctx := context.Background()
	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO projects (owner_id, name, description) VALUES (?, ?, ?)`,
		ownerID, "Test Project", "Test project for cloudinary tests",
	)
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get project ID: %v", err)
	}

	return id
}

// createTestTaskForCloudinary creates a task in the given project and returns its ID
func createTestTaskForCloudinary(t *testing.T, ts *TestServer, projectID int64) int64 {
	t.Helper()

	ctx := context.Background()
	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO tasks (project_id, title, status, priority) VALUES (?, ?, ?, ?)`,
		projectID, "Test Task", "todo", "medium",
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

// createTestAttachment inserts a task_attachments row and returns the attachment ID
func createTestAttachment(t *testing.T, ts *TestServer, taskID, userID, projectID int64, fileType, filename, altName string) int64 {
	t.Helper()

	ctx := context.Background()
	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO task_attachments (task_id, project_id, user_id, filename, alt_name, file_type, content_type, file_size, cloudinary_url, cloudinary_public_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		taskID, projectID, userID, filename, altName, fileType, "application/octet-stream", 1024,
		"https://res.cloudinary.com/test/"+filename, "test/"+filename,
	)
	if err != nil {
		t.Fatalf("Failed to create test attachment: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get attachment ID: %v", err)
	}

	return id
}

// addProjectMember adds a user as a project member
func addProjectMember(t *testing.T, ts *TestServer, projectID, userID, grantedBy int64, role string) {
	t.Helper()

	ctx := context.Background()
	_, err := ts.DB.ExecContext(ctx,
		`INSERT INTO project_members (project_id, user_id, role, granted_by) VALUES (?, ?, ?, ?)`,
		projectID, userID, role, grantedBy,
	)
	if err != nil {
		t.Fatalf("Failed to add project member: %v", err)
	}
}

// makeAuthRequest creates a request with auth context and optional chi URL params
func makeAuthRequest(t *testing.T, method, path string, body interface{}, userID int64, urlParams map[string]string) (*httptest.ResponseRecorder, *http.Request) {
	t.Helper()

	rec, req := MakeRequest(t, method, path, body, nil)

	// Set user context
	ctx := context.WithValue(req.Context(), UserIDKey, userID)

	// Set chi URL params if any
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

func TestHandleListAssets(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantCount  int
		setupFunc  func(t *testing.T, ts *TestServer) int64 // returns userID to use for request
		checkOwner bool
	}{
		{
			name:       "empty list when no attachments",
			wantStatus: http.StatusOK,
			wantCount:  0,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
		{
			name:       "returns own attachments with is_owner true",
			wantStatus: http.StatusOK,
			wantCount:  2,
			checkOwner: true,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo1.jpg", "My photo")
				createTestAttachment(t, ts, taskID, userID, projectID, "pdf", "doc.pdf", "My doc")
				return userID
			},
		},
		{
			name:       "returns shared project members attachments",
			wantStatus: http.StatusOK,
			wantCount:  3, // 2 own + 1 shared
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				user1ID := ts.CreateTestUser(t, "user1@example.com", "password123")
				user2ID := ts.CreateTestUser(t, "user2@example.com", "password123")

				projectID := createTestProjectForCloudinary(t, ts, user1ID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)

				// Add both users as project members
				addProjectMember(t, ts, projectID, user1ID, user1ID, "admin")
				addProjectMember(t, ts, projectID, user2ID, user1ID, "editor")

				// user1's attachments
				createTestAttachment(t, ts, taskID, user1ID, projectID, "image", "user1_photo.jpg", "User1 photo")
				createTestAttachment(t, ts, taskID, user1ID, projectID, "pdf", "user1_doc.pdf", "User1 doc")
				// user2's attachment (should appear for user1 via shared project)
				createTestAttachment(t, ts, taskID, user2ID, projectID, "image", "user2_photo.jpg", "User2 photo")

				return user1ID
			},
		},
		{
			name:       "filters by type=image",
			query:      "?type=image",
			wantStatus: http.StatusOK,
			wantCount:  1,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo.jpg", "Photo")
				createTestAttachment(t, ts, taskID, userID, projectID, "pdf", "doc.pdf", "Doc")
				createTestAttachment(t, ts, taskID, userID, projectID, "video", "clip.mp4", "Video")
				return userID
			},
		},
		{
			name:       "filters by type=video",
			query:      "?type=video",
			wantStatus: http.StatusOK,
			wantCount:  1,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo.jpg", "Photo")
				createTestAttachment(t, ts, taskID, userID, projectID, "video", "clip.mp4", "Video")
				return userID
			},
		},
		{
			name:       "filters by type=pdf",
			query:      "?type=pdf",
			wantStatus: http.StatusOK,
			wantCount:  1,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo.jpg", "Photo")
				createTestAttachment(t, ts, taskID, userID, projectID, "pdf", "report.pdf", "Report")
				return userID
			},
		},
		{
			name:       "search by query matches alt_name",
			query:      "?q=sunset",
			wantStatus: http.StatusOK,
			wantCount:  1,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "img001.jpg", "Beautiful sunset")
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "img002.jpg", "Mountain view")
				return userID
			},
		},
		{
			name:       "search by query matches filename",
			query:      "?q=report",
			wantStatus: http.StatusOK,
			wantCount:  1,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "pdf", "report_2024.pdf", "")
				createTestAttachment(t, ts, taskID, userID, projectID, "pdf", "invoice.pdf", "")
				return userID
			},
		},
		{
			name:       "pagination with limit and offset",
			query:      "?limit=2&offset=1",
			wantStatus: http.StatusOK,
			wantCount:  2,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "a.jpg", "A")
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "b.jpg", "B")
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "c.jpg", "C")
				return userID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID := tt.setupFunc(t, ts)

			path := "/api/assets"
			if tt.query != "" {
				path += tt.query
			}

			rec, req := makeAuthRequest(t, http.MethodGet, path, nil, userID, nil)
			ts.HandleListAssets(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			var assets []AssetResponse
			DecodeJSON(t, rec, &assets)

			if len(assets) != tt.wantCount {
				t.Errorf("Expected %d assets, got %d", tt.wantCount, len(assets))
			}

			if tt.checkOwner && len(assets) > 0 {
				for _, a := range assets {
					if a.UserID == userID && !a.IsOwner {
						t.Errorf("Expected is_owner=true for own asset %d", a.ID)
					}
				}
			}
		})
	}
}

func TestHandleListAssetsRequiresAuth(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	token := ts.GenerateTestToken(t, userID, "user@example.com")

	// With valid auth through middleware
	headers := map[string]string{
		"Authorization": fmt.Sprintf("Bearer %s", token),
	}
	rec, req := MakeRequest(t, http.MethodGet, "/api/assets", nil, headers)
	ts.JWTAuth(http.HandlerFunc(ts.HandleListAssets)).ServeHTTP(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	// Without auth through middleware
	rec2, req2 := MakeRequest(t, http.MethodGet, "/api/assets", nil, nil)
	ts.JWTAuth(http.HandlerFunc(ts.HandleListAssets)).ServeHTTP(rec2, req2)
	AssertStatusCode(t, rec2.Code, http.StatusUnauthorized)
}

func TestHandleDeleteAttachment(t *testing.T) {
	tests := []struct {
		name          string
		attachmentID  string
		wantStatus    int
		wantError     string
		wantErrorCode string
		setupFunc     func(t *testing.T, ts *TestServer) int64 // returns userID for request
	}{
		{
			name:       "owner can delete own attachment",
			wantStatus: http.StatusOK,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo.jpg", "Photo")
				return userID
			},
		},
		{
			name:          "non-owner gets 403",
			wantStatus:    http.StatusForbidden,
			wantError:     "you can only delete your own attachments",
			wantErrorCode: "forbidden",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
				otherID := ts.CreateTestUser(t, "other@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, ownerID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, ownerID, projectID, "image", "photo.jpg", "Photo")
				return otherID
			},
		},
		{
			name:          "invalid ID returns 400",
			attachmentID:  "abc",
			wantStatus:    http.StatusBadRequest,
			wantError:     "invalid attachment ID",
			wantErrorCode: "bad_request",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
		{
			name:          "non-existent ID returns 404",
			attachmentID:  "99999",
			wantStatus:    http.StatusNotFound,
			wantError:     "attachment not found",
			wantErrorCode: "not_found",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID := tt.setupFunc(t, ts)

			// Determine attachment ID
			attachmentID := tt.attachmentID
			if attachmentID == "" {
				attachmentID = "1" // the first auto-increment attachment
			}

			rec, req := makeAuthRequest(t, http.MethodDelete, "/api/attachments/"+attachmentID, nil, userID,
				map[string]string{"id": attachmentID})
			ts.HandleDeleteAttachment(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			if tt.wantError != "" {
				AssertError(t, rec, tt.wantStatus, tt.wantError, tt.wantErrorCode)
			}

			// Verify deletion actually happened for success case
			if tt.wantStatus == http.StatusOK {
				var resp map[string]string
				DecodeJSON(t, rec, &resp)
				if resp["message"] != "Attachment deleted" {
					t.Errorf("Expected deletion message, got %q", resp["message"])
				}
			}
		})
	}
}

func TestHandleUpdateAttachment(t *testing.T) {
	tests := []struct {
		name          string
		body          interface{}
		wantStatus    int
		wantError     string
		wantErrorCode string
		setupFunc     func(t *testing.T, ts *TestServer) (userID int64, attachmentID string)
	}{
		{
			name:       "owner updates alt_name",
			body:       UpdateAttachmentRequest{AltName: stringPtr("Updated alt text")},
			wantStatus: http.StatusOK,
			setupFunc: func(t *testing.T, ts *TestServer) (int64, string) {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				attachID := createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo.jpg", "Old alt")
				return userID, fmt.Sprintf("%d", attachID)
			},
		},
		{
			name:          "non-owner gets 403",
			body:          UpdateAttachmentRequest{AltName: stringPtr("Hacked")},
			wantStatus:    http.StatusForbidden,
			wantError:     "you can only update your own attachments",
			wantErrorCode: "forbidden",
			setupFunc: func(t *testing.T, ts *TestServer) (int64, string) {
				ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
				otherID := ts.CreateTestUser(t, "other@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, ownerID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				attachID := createTestAttachment(t, ts, taskID, ownerID, projectID, "image", "photo.jpg", "Original")
				return otherID, fmt.Sprintf("%d", attachID)
			},
		},
		{
			name:          "non-existent attachment returns 404",
			body:          UpdateAttachmentRequest{AltName: stringPtr("test")},
			wantStatus:    http.StatusNotFound,
			wantError:     "attachment not found",
			wantErrorCode: "not_found",
			setupFunc: func(t *testing.T, ts *TestServer) (int64, string) {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				return userID, "99999"
			},
		},
		{
			name:          "invalid ID returns 400",
			body:          UpdateAttachmentRequest{AltName: stringPtr("test")},
			wantStatus:    http.StatusBadRequest,
			wantError:     "invalid attachment ID",
			wantErrorCode: "bad_request",
			setupFunc: func(t *testing.T, ts *TestServer) (int64, string) {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				return userID, "invalid"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID, attachmentID := tt.setupFunc(t, ts)

			rec, req := makeAuthRequest(t, http.MethodPatch, "/api/attachments/"+attachmentID, tt.body, userID,
				map[string]string{"id": attachmentID})
			ts.HandleUpdateAttachment(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			if tt.wantError != "" {
				AssertError(t, rec, tt.wantStatus, tt.wantError, tt.wantErrorCode)
			}

			if tt.wantStatus == http.StatusOK {
				var resp TaskAttachment
				DecodeJSON(t, rec, &resp)
				if resp.AltName != "Updated alt text" {
					t.Errorf("Expected alt_name 'Updated alt text', got %q", resp.AltName)
				}
			}
		})
	}
}

func TestHandleGetStorageUsage(t *testing.T) {
	tests := []struct {
		name       string
		projectID  string
		wantStatus int
		wantCount  int
		setupFunc  func(t *testing.T, ts *TestServer) int64 // returns userID
	}{
		{
			name:       "returns storage usage per user",
			projectID:  "1",
			wantStatus: http.StatusOK,
			wantCount:  1,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "a.jpg", "A")
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "b.jpg", "B")
				return userID
			},
		},
		{
			name:       "returns multiple users",
			projectID:  "1",
			wantStatus: http.StatusOK,
			wantCount:  2,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				user1ID := ts.CreateTestUser(t, "user1@example.com", "password123")
				user2ID := ts.CreateTestUser(t, "user2@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, user1ID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, user1ID, projectID, "image", "a.jpg", "A")
				createTestAttachment(t, ts, taskID, user2ID, projectID, "pdf", "b.pdf", "B")
				return user1ID
			},
		},
		{
			name:       "empty project returns empty list",
			projectID:  "1",
			wantStatus: http.StatusOK,
			wantCount:  0,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				createTestProjectForCloudinary(t, ts, userID)
				return userID
			},
		},
		{
			name:       "invalid project ID returns 400",
			projectID:  "abc",
			wantStatus: http.StatusBadRequest,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID := tt.setupFunc(t, ts)

			rec, req := makeAuthRequest(t, http.MethodGet, "/api/projects/"+tt.projectID+"/storage", nil, userID,
				map[string]string{"id": tt.projectID})
			ts.HandleGetStorageUsage(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			if tt.wantStatus == http.StatusOK {
				var usage []StorageUsage
				DecodeJSON(t, rec, &usage)
				if len(usage) != tt.wantCount {
					t.Errorf("Expected %d usage entries, got %d", tt.wantCount, len(usage))
				}
			}
		})
	}
}

func TestHandleListImages(t *testing.T) {
	tests := []struct {
		name       string
		query      string
		wantStatus int
		wantCount  int
		setupFunc  func(t *testing.T, ts *TestServer) int64
	}{
		{
			name:       "returns only images",
			wantStatus: http.StatusOK,
			wantCount:  2,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo1.jpg", "Photo 1")
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo2.jpg", "Photo 2")
				createTestAttachment(t, ts, taskID, userID, projectID, "pdf", "doc.pdf", "Document")
				createTestAttachment(t, ts, taskID, userID, projectID, "video", "clip.mp4", "Video")
				return userID
			},
		},
		{
			name:       "search filters images",
			query:      "?q=sunset",
			wantStatus: http.StatusOK,
			wantCount:  1,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "sunset.jpg", "Beautiful sunset")
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "mountain.jpg", "Mountain view")
				return userID
			},
		},
		{
			name:       "empty result when no images",
			wantStatus: http.StatusOK,
			wantCount:  0,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "pdf", "doc.pdf", "Doc")
				return userID
			},
		},
		{
			name:       "includes shared project members images",
			wantStatus: http.StatusOK,
			wantCount:  2, // 1 own + 1 shared
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				user1ID := ts.CreateTestUser(t, "user1@example.com", "password123")
				user2ID := ts.CreateTestUser(t, "user2@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, user1ID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				addProjectMember(t, ts, projectID, user1ID, user1ID, "admin")
				addProjectMember(t, ts, projectID, user2ID, user1ID, "editor")
				createTestAttachment(t, ts, taskID, user1ID, projectID, "image", "user1.jpg", "User1 image")
				createTestAttachment(t, ts, taskID, user2ID, projectID, "image", "user2.jpg", "User2 image")
				return user1ID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID := tt.setupFunc(t, ts)

			path := "/api/images"
			if tt.query != "" {
				path += tt.query
			}

			rec, req := makeAuthRequest(t, http.MethodGet, path, nil, userID, nil)
			ts.HandleListImages(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			var images []TaskAttachment
			DecodeJSON(t, rec, &images)

			if len(images) != tt.wantCount {
				t.Errorf("Expected %d images, got %d", tt.wantCount, len(images))
			}

			// Verify all returned are images
			for _, img := range images {
				if img.FileType != "image" {
					t.Errorf("Expected file_type 'image', got %q", img.FileType)
				}
			}
		})
	}
}

func TestHandleDeleteAttachmentVerifyRemoval(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	taskID := createTestTaskForCloudinary(t, ts, projectID)
	attachID := createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo.jpg", "Photo")

	attachmentID := fmt.Sprintf("%d", attachID)

	// Delete the attachment
	rec, req := makeAuthRequest(t, http.MethodDelete, "/api/attachments/"+attachmentID, nil, userID,
		map[string]string{"id": attachmentID})
	ts.HandleDeleteAttachment(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	// Verify it's gone by trying to delete again
	rec2, req2 := makeAuthRequest(t, http.MethodDelete, "/api/attachments/"+attachmentID, nil, userID,
		map[string]string{"id": attachmentID})
	ts.HandleDeleteAttachment(rec2, req2)
	AssertStatusCode(t, rec2.Code, http.StatusNotFound)
}

func TestHandleListAssetsOwnerFlag(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	user1ID := ts.CreateTestUser(t, "user1@example.com", "password123")
	user2ID := ts.CreateTestUser(t, "user2@example.com", "password123")

	projectID := createTestProjectForCloudinary(t, ts, user1ID)
	taskID := createTestTaskForCloudinary(t, ts, projectID)

	addProjectMember(t, ts, projectID, user1ID, user1ID, "admin")
	addProjectMember(t, ts, projectID, user2ID, user1ID, "editor")

	createTestAttachment(t, ts, taskID, user1ID, projectID, "image", "mine.jpg", "My photo")
	createTestAttachment(t, ts, taskID, user2ID, projectID, "image", "theirs.jpg", "Their photo")

	// Request as user1
	rec, req := makeAuthRequest(t, http.MethodGet, "/api/assets", nil, user1ID, nil)
	ts.HandleListAssets(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	var assets []json.RawMessage
	DecodeJSON(t, rec, &assets)

	if len(assets) != 2 {
		t.Fatalf("Expected 2 assets, got %d", len(assets))
	}

	for _, raw := range assets {
		var a struct {
			UserID  int64 `json:"user_id"`
			IsOwner bool  `json:"is_owner"`
		}
		if err := json.Unmarshal(raw, &a); err != nil {
			t.Fatal(err)
		}
		if a.UserID == user1ID && !a.IsOwner {
			t.Error("Expected is_owner=true for user1's asset")
		}
		if a.UserID == user2ID && a.IsOwner {
			t.Error("Expected is_owner=false for user2's asset (viewed by user1)")
		}
	}
}

// createTestCloudinaryCredential inserts a cloudinary_credentials row for the given user
func createTestCloudinaryCredential(t *testing.T, ts *TestServer, userID int64, cloudName, apiKey, apiSecret string) {
	t.Helper()

	ctx := context.Background()
	_, err := ts.DB.ExecContext(ctx,
		`INSERT INTO cloudinary_credentials (user_id, cloud_name, api_key, api_secret, max_file_size_mb, status)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		userID, cloudName, apiKey, apiSecret, 10, "unknown",
	)
	if err != nil {
		t.Fatalf("Failed to create test cloudinary credential: %v", err)
	}
}

func TestHandleGetCloudinaryCredential(t *testing.T) {
	tests := []struct {
		name           string
		wantStatus     int
		wantEmpty      bool
		wantCloudName  string
		setupFunc      func(t *testing.T, ts *TestServer) int64
	}{
		{
			name:       "returns empty object when no credentials exist",
			wantStatus: http.StatusOK,
			wantEmpty:  true,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
		{
			name:          "returns stored credentials",
			wantStatus:    http.StatusOK,
			wantCloudName: "my-cloud",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				createTestCloudinaryCredential(t, ts, userID, "my-cloud", "key123", "secret456")
				return userID
			},
		},
		{
			name:          "returns only own credentials not other users",
			wantStatus:    http.StatusOK,
			wantEmpty:     true,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				otherID := ts.CreateTestUser(t, "other@example.com", "password123")
				createTestCloudinaryCredential(t, ts, otherID, "other-cloud", "otherkey", "othersecret")
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
		{
			name:          "returns credential fields correctly",
			wantStatus:    http.StatusOK,
			wantCloudName: "test-cloud",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				createTestCloudinaryCredential(t, ts, userID, "test-cloud", "api-key-123", "api-secret-456")
				return userID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID := tt.setupFunc(t, ts)

			rec, req := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/credentials", nil, userID, nil)
			ts.HandleGetCloudinaryCredential(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			if tt.wantEmpty {
				var resp map[string]interface{}
				DecodeJSON(t, rec, &resp)
				if len(resp) != 0 {
					t.Errorf("Expected empty object, got %v", resp)
				}
			} else if tt.wantCloudName != "" {
				var cred CloudinaryCredential
				DecodeJSON(t, rec, &cred)
				if cred.CloudName != tt.wantCloudName {
					t.Errorf("Expected cloud_name %q, got %q", tt.wantCloudName, cred.CloudName)
				}
				if cred.UserID != userID {
					t.Errorf("Expected user_id %d, got %d", userID, cred.UserID)
				}
				if cred.ID == 0 {
					t.Error("Expected non-zero credential ID")
				}
				if cred.MaxFileSizeMB == 0 {
					t.Error("Expected non-zero max_file_size_mb")
				}
			}
		})
	}
}

func TestHandleSaveCloudinaryCredential(t *testing.T) {
	tests := []struct {
		name          string
		body          interface{}
		wantStatus    int
		wantError     string
		wantErrorCode string
		checkResp     func(t *testing.T, cred CloudinaryCredential)
		setupFunc     func(t *testing.T, ts *TestServer) int64
	}{
		{
			name: "saves new credentials successfully",
			body: SaveCloudinaryCredentialRequest{
				CloudName: "my-cloud",
				APIKey:    "my-key",
				APISecret: "my-secret",
			},
			wantStatus: http.StatusOK,
			checkResp: func(t *testing.T, cred CloudinaryCredential) {
				if cred.CloudName != "my-cloud" {
					t.Errorf("Expected cloud_name 'my-cloud', got %q", cred.CloudName)
				}
				if cred.APIKey != "my-key" {
					t.Errorf("Expected api_key 'my-key', got %q", cred.APIKey)
				}
				if cred.MaxFileSizeMB != 10 {
					t.Errorf("Expected default max_file_size_mb 10, got %d", cred.MaxFileSizeMB)
				}
			},
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
		{
			name: "saves credentials with custom max file size",
			body: SaveCloudinaryCredentialRequest{
				CloudName:     "my-cloud",
				APIKey:        "my-key",
				APISecret:     "my-secret",
				MaxFileSizeMB: intPtr(25),
			},
			wantStatus: http.StatusOK,
			checkResp: func(t *testing.T, cred CloudinaryCredential) {
				if cred.MaxFileSizeMB != 25 {
					t.Errorf("Expected max_file_size_mb 25, got %d", cred.MaxFileSizeMB)
				}
			},
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
		{
			name: "updates existing credentials via upsert",
			body: SaveCloudinaryCredentialRequest{
				CloudName: "updated-cloud",
				APIKey:    "updated-key",
				APISecret: "updated-secret",
			},
			wantStatus: http.StatusOK,
			checkResp: func(t *testing.T, cred CloudinaryCredential) {
				if cred.CloudName != "updated-cloud" {
					t.Errorf("Expected cloud_name 'updated-cloud', got %q", cred.CloudName)
				}
				if cred.APIKey != "updated-key" {
					t.Errorf("Expected api_key 'updated-key', got %q", cred.APIKey)
				}
			},
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				createTestCloudinaryCredential(t, ts, userID, "old-cloud", "old-key", "old-secret")
				return userID
			},
		},
		{
			name:          "missing cloud_name returns validation error",
			body:          SaveCloudinaryCredentialRequest{APIKey: "key", APISecret: "secret"},
			wantStatus:    http.StatusBadRequest,
			wantError:     "cloud_name, api_key, and api_secret are required",
			wantErrorCode: "validation_error",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
		{
			name:          "missing api_key returns validation error",
			body:          SaveCloudinaryCredentialRequest{CloudName: "cloud", APISecret: "secret"},
			wantStatus:    http.StatusBadRequest,
			wantError:     "cloud_name, api_key, and api_secret are required",
			wantErrorCode: "validation_error",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
		{
			name:          "missing api_secret returns validation error",
			body:          SaveCloudinaryCredentialRequest{CloudName: "cloud", APIKey: "key"},
			wantStatus:    http.StatusBadRequest,
			wantError:     "cloud_name, api_key, and api_secret are required",
			wantErrorCode: "validation_error",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
		{
			name:          "empty body returns bad request",
			body:          "not json",
			wantStatus:    http.StatusBadRequest,
			wantError:     "invalid request body",
			wantErrorCode: "bad_request",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
		{
			name:          "all fields empty returns validation error",
			body:          SaveCloudinaryCredentialRequest{},
			wantStatus:    http.StatusBadRequest,
			wantError:     "cloud_name, api_key, and api_secret are required",
			wantErrorCode: "validation_error",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID := tt.setupFunc(t, ts)

			rec, req := makeAuthRequest(t, http.MethodPost, "/api/cloudinary/credentials", tt.body, userID, nil)
			ts.HandleSaveCloudinaryCredential(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			if tt.wantError != "" {
				AssertError(t, rec, tt.wantStatus, tt.wantError, tt.wantErrorCode)
			}

			if tt.checkResp != nil && tt.wantStatus == http.StatusOK {
				var cred CloudinaryCredential
				DecodeJSON(t, rec, &cred)
				tt.checkResp(t, cred)
				if cred.UserID != userID {
					t.Errorf("Expected user_id %d, got %d", userID, cred.UserID)
				}
			}
		})
	}
}

func TestHandleSaveCloudinaryCredentialPersistence(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")

	// Save credentials
	body := SaveCloudinaryCredentialRequest{
		CloudName: "persist-cloud",
		APIKey:    "persist-key",
		APISecret: "persist-secret",
	}
	rec, req := makeAuthRequest(t, http.MethodPost, "/api/cloudinary/credentials", body, userID, nil)
	ts.HandleSaveCloudinaryCredential(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	// Verify by fetching via GET
	rec2, req2 := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/credentials", nil, userID, nil)
	ts.HandleGetCloudinaryCredential(rec2, req2)
	AssertStatusCode(t, rec2.Code, http.StatusOK)

	var cred CloudinaryCredential
	DecodeJSON(t, rec2, &cred)
	if cred.CloudName != "persist-cloud" {
		t.Errorf("Expected cloud_name 'persist-cloud', got %q", cred.CloudName)
	}
	if cred.APIKey != "persist-key" {
		t.Errorf("Expected api_key 'persist-key', got %q", cred.APIKey)
	}
}

func TestHandleDeleteCloudinaryCredential(t *testing.T) {
	tests := []struct {
		name       string
		wantStatus int
		setupFunc  func(t *testing.T, ts *TestServer) int64
	}{
		{
			name:       "deletes existing credentials",
			wantStatus: http.StatusOK,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				createTestCloudinaryCredential(t, ts, userID, "my-cloud", "my-key", "my-secret")
				return userID
			},
		},
		{
			name:       "succeeds even when no credentials exist",
			wantStatus: http.StatusOK,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
		{
			name:       "does not delete other users credentials",
			wantStatus: http.StatusOK,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				otherID := ts.CreateTestUser(t, "other@example.com", "password123")
				createTestCloudinaryCredential(t, ts, otherID, "other-cloud", "other-key", "other-secret")
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID := tt.setupFunc(t, ts)

			rec, req := makeAuthRequest(t, http.MethodDelete, "/api/cloudinary/credentials", nil, userID, nil)
			ts.HandleDeleteCloudinaryCredential(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			var resp map[string]string
			DecodeJSON(t, rec, &resp)
			if resp["message"] != "Cloudinary credentials deleted" {
				t.Errorf("Expected deletion message, got %q", resp["message"])
			}
		})
	}
}

func TestHandleDeleteCloudinaryCredentialVerifyRemoval(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	createTestCloudinaryCredential(t, ts, userID, "my-cloud", "my-key", "my-secret")

	// Verify credentials exist via GET
	rec1, req1 := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/credentials", nil, userID, nil)
	ts.HandleGetCloudinaryCredential(rec1, req1)
	AssertStatusCode(t, rec1.Code, http.StatusOK)

	var credBefore CloudinaryCredential
	DecodeJSON(t, rec1, &credBefore)
	if credBefore.CloudName != "my-cloud" {
		t.Fatalf("Expected credentials to exist before deletion")
	}

	// Delete credentials
	rec2, req2 := makeAuthRequest(t, http.MethodDelete, "/api/cloudinary/credentials", nil, userID, nil)
	ts.HandleDeleteCloudinaryCredential(rec2, req2)
	AssertStatusCode(t, rec2.Code, http.StatusOK)

	// Verify credentials are gone via GET (should return empty object)
	rec3, req3 := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/credentials", nil, userID, nil)
	ts.HandleGetCloudinaryCredential(rec3, req3)
	AssertStatusCode(t, rec3.Code, http.StatusOK)

	var credAfter map[string]interface{}
	DecodeJSON(t, rec3, &credAfter)
	if len(credAfter) != 0 {
		t.Errorf("Expected empty object after deletion, got %v", credAfter)
	}
}

func TestHandleDeleteCloudinaryCredentialDoesNotAffectOthers(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	user1ID := ts.CreateTestUser(t, "user1@example.com", "password123")
	user2ID := ts.CreateTestUser(t, "user2@example.com", "password123")
	createTestCloudinaryCredential(t, ts, user1ID, "cloud-1", "key-1", "secret-1")
	createTestCloudinaryCredential(t, ts, user2ID, "cloud-2", "key-2", "secret-2")

	// User1 deletes their credentials
	rec, req := makeAuthRequest(t, http.MethodDelete, "/api/cloudinary/credentials", nil, user1ID, nil)
	ts.HandleDeleteCloudinaryCredential(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	// Verify user2's credentials are still there
	rec2, req2 := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/credentials", nil, user2ID, nil)
	ts.HandleGetCloudinaryCredential(rec2, req2)
	AssertStatusCode(t, rec2.Code, http.StatusOK)

	var cred CloudinaryCredential
	DecodeJSON(t, rec2, &cred)
	if cred.CloudName != "cloud-2" {
		t.Errorf("Expected user2's cloud_name 'cloud-2', got %q", cred.CloudName)
	}
}

func TestHandleListTaskAttachments(t *testing.T) {
	tests := []struct {
		name       string
		wantStatus int
		wantCount  int
		taskID     string
		setupFunc  func(t *testing.T, ts *TestServer) int64
	}{
		{
			name:       "returns attachments for a task",
			wantStatus: http.StatusOK,
			wantCount:  2,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo.jpg", "Photo")
				createTestAttachment(t, ts, taskID, userID, projectID, "pdf", "doc.pdf", "Document")
				return userID
			},
		},
		{
			name:       "returns empty list when no attachments",
			wantStatus: http.StatusOK,
			wantCount:  0,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				createTestTaskForCloudinary(t, ts, projectID) // task with no attachments
				return userID
			},
		},
		{
			name:       "returns only attachments for the specified task",
			wantStatus: http.StatusOK,
			wantCount:  1,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				task1ID := createTestTaskForCloudinary(t, ts, projectID)
				task2ID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, task1ID, userID, projectID, "image", "task1_photo.jpg", "Task 1 photo")
				createTestAttachment(t, ts, task2ID, userID, projectID, "image", "task2_photo.jpg", "Task 2 photo")
				return userID
			},
		},
		{
			name:       "invalid task ID returns 400",
			taskID:     "abc",
			wantStatus: http.StatusBadRequest,
			wantCount:  0,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
		{
			name:       "returns attachments from multiple users on same task",
			wantStatus: http.StatusOK,
			wantCount:  3,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				user1ID := ts.CreateTestUser(t, "user1@example.com", "password123")
				user2ID := ts.CreateTestUser(t, "user2@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, user1ID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, user1ID, projectID, "image", "u1_photo.jpg", "U1 photo")
				createTestAttachment(t, ts, taskID, user1ID, projectID, "pdf", "u1_doc.pdf", "U1 doc")
				createTestAttachment(t, ts, taskID, user2ID, projectID, "image", "u2_photo.jpg", "U2 photo")
				return user1ID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID := tt.setupFunc(t, ts)

			taskID := tt.taskID
			if taskID == "" {
				taskID = "1" // first auto-increment task
			}

			rec, req := makeAuthRequest(t, http.MethodGet, "/api/tasks/"+taskID+"/attachments", nil, userID,
				map[string]string{"taskId": taskID})
			ts.HandleListTaskAttachments(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			if tt.wantStatus == http.StatusOK {
				var attachments []TaskAttachment
				DecodeJSON(t, rec, &attachments)
				if len(attachments) != tt.wantCount {
					t.Errorf("Expected %d attachments, got %d", tt.wantCount, len(attachments))
				}

				// Verify ordering is by created_at DESC (latest first)
				for i := 1; i < len(attachments); i++ {
					if attachments[i].CreatedAt.After(attachments[i-1].CreatedAt) {
						t.Errorf("Attachments not ordered by created_at DESC: index %d (%v) is after index %d (%v)",
							i, attachments[i].CreatedAt, i-1, attachments[i-1].CreatedAt)
					}
				}
			}
		})
	}
}

func TestHandleListTaskAttachmentsFieldValues(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	taskID := createTestTaskForCloudinary(t, ts, projectID)
	createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo.jpg", "My photo")

	taskIDStr := fmt.Sprintf("%d", taskID)
	rec, req := makeAuthRequest(t, http.MethodGet, "/api/tasks/"+taskIDStr+"/attachments", nil, userID,
		map[string]string{"taskId": taskIDStr})
	ts.HandleListTaskAttachments(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var attachments []TaskAttachment
	DecodeJSON(t, rec, &attachments)

	if len(attachments) != 1 {
		t.Fatalf("Expected 1 attachment, got %d", len(attachments))
	}

	a := attachments[0]
	if a.TaskID != taskID {
		t.Errorf("Expected task_id %d, got %d", taskID, a.TaskID)
	}
	if a.ProjectID != projectID {
		t.Errorf("Expected project_id %d, got %d", projectID, a.ProjectID)
	}
	if a.UserID != userID {
		t.Errorf("Expected user_id %d, got %d", userID, a.UserID)
	}
	if a.Filename != "photo.jpg" {
		t.Errorf("Expected filename 'photo.jpg', got %q", a.Filename)
	}
	if a.AltName != "My photo" {
		t.Errorf("Expected alt_name 'My photo', got %q", a.AltName)
	}
	if a.FileType != "image" {
		t.Errorf("Expected file_type 'image', got %q", a.FileType)
	}
	if a.CloudinaryURL == "" {
		t.Error("Expected non-empty cloudinary_url")
	}
	if a.CloudinaryPublicID == "" {
		t.Error("Expected non-empty cloudinary_public_id")
	}
}

func TestHandleCreateTaskAttachment(t *testing.T) {
	tests := []struct {
		name          string
		body          interface{}
		taskID        string
		wantStatus    int
		wantError     string
		wantErrorCode string
		checkResp     func(t *testing.T, a TaskAttachment, userID, taskID, projectID int64)
		setupFunc     func(t *testing.T, ts *TestServer) (userID int64, taskID string, projectID int64)
	}{
		{
			name: "creates attachment successfully",
			body: CreateAttachmentRequest{
				Filename:           "screenshot.png",
				AltName:            "Login page screenshot",
				FileType:           "image",
				ContentType:        "image/png",
				FileSize:           2048,
				CloudinaryURL:      "https://res.cloudinary.com/test/image/upload/screenshot.png",
				CloudinaryPublicID: "test/screenshot",
			},
			wantStatus: http.StatusCreated,
			checkResp: func(t *testing.T, a TaskAttachment, userID, taskID, projectID int64) {
				if a.Filename != "screenshot.png" {
					t.Errorf("Expected filename 'screenshot.png', got %q", a.Filename)
				}
				if a.AltName != "Login page screenshot" {
					t.Errorf("Expected alt_name 'Login page screenshot', got %q", a.AltName)
				}
				if a.FileType != "image" {
					t.Errorf("Expected file_type 'image', got %q", a.FileType)
				}
				if a.ContentType != "image/png" {
					t.Errorf("Expected content_type 'image/png', got %q", a.ContentType)
				}
				if a.FileSize != 2048 {
					t.Errorf("Expected file_size 2048, got %d", a.FileSize)
				}
				if a.CloudinaryURL != "https://res.cloudinary.com/test/image/upload/screenshot.png" {
					t.Errorf("Expected cloudinary_url, got %q", a.CloudinaryURL)
				}
				if a.CloudinaryPublicID != "test/screenshot" {
					t.Errorf("Expected cloudinary_public_id 'test/screenshot', got %q", a.CloudinaryPublicID)
				}
				if a.UserID != userID {
					t.Errorf("Expected user_id %d, got %d", userID, a.UserID)
				}
			},
			setupFunc: func(t *testing.T, ts *TestServer) (int64, string, int64) {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				return userID, fmt.Sprintf("%d", taskID), projectID
			},
		},
		{
			name: "creates attachment with minimal fields",
			body: CreateAttachmentRequest{
				Filename:           "file.bin",
				CloudinaryURL:      "https://res.cloudinary.com/test/raw/upload/file.bin",
				CloudinaryPublicID: "test/file",
			},
			wantStatus: http.StatusCreated,
			checkResp: func(t *testing.T, a TaskAttachment, userID, taskID, projectID int64) {
				if a.Filename != "file.bin" {
					t.Errorf("Expected filename 'file.bin', got %q", a.Filename)
				}
				if a.ID == 0 {
					t.Error("Expected non-zero attachment ID")
				}
			},
			setupFunc: func(t *testing.T, ts *TestServer) (int64, string, int64) {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				return userID, fmt.Sprintf("%d", taskID), projectID
			},
		},
		{
			name:          "missing filename returns validation error",
			body:          CreateAttachmentRequest{CloudinaryURL: "https://example.com", CloudinaryPublicID: "test/id"},
			wantStatus:    http.StatusBadRequest,
			wantError:     "filename, cloudinary_url, and cloudinary_public_id are required",
			wantErrorCode: "validation_error",
			setupFunc: func(t *testing.T, ts *TestServer) (int64, string, int64) {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				return userID, fmt.Sprintf("%d", taskID), projectID
			},
		},
		{
			name:          "missing cloudinary_url returns validation error",
			body:          CreateAttachmentRequest{Filename: "file.jpg", CloudinaryPublicID: "test/id"},
			wantStatus:    http.StatusBadRequest,
			wantError:     "filename, cloudinary_url, and cloudinary_public_id are required",
			wantErrorCode: "validation_error",
			setupFunc: func(t *testing.T, ts *TestServer) (int64, string, int64) {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				return userID, fmt.Sprintf("%d", taskID), projectID
			},
		},
		{
			name:          "missing cloudinary_public_id returns validation error",
			body:          CreateAttachmentRequest{Filename: "file.jpg", CloudinaryURL: "https://example.com"},
			wantStatus:    http.StatusBadRequest,
			wantError:     "filename, cloudinary_url, and cloudinary_public_id are required",
			wantErrorCode: "validation_error",
			setupFunc: func(t *testing.T, ts *TestServer) (int64, string, int64) {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				return userID, fmt.Sprintf("%d", taskID), projectID
			},
		},
		{
			name:          "invalid task ID returns 400",
			taskID:        "abc",
			body:          CreateAttachmentRequest{Filename: "f.jpg", CloudinaryURL: "url", CloudinaryPublicID: "id"},
			wantStatus:    http.StatusBadRequest,
			wantError:     "invalid task ID",
			wantErrorCode: "bad_request",
			setupFunc: func(t *testing.T, ts *TestServer) (int64, string, int64) {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				return userID, "abc", 0
			},
		},
		{
			name:          "non-existent task returns 404",
			taskID:        "99999",
			body:          CreateAttachmentRequest{Filename: "f.jpg", CloudinaryURL: "url", CloudinaryPublicID: "id"},
			wantStatus:    http.StatusNotFound,
			wantError:     "task not found",
			wantErrorCode: "not_found",
			setupFunc: func(t *testing.T, ts *TestServer) (int64, string, int64) {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				return userID, "99999", 0
			},
		},
		{
			name:          "invalid request body returns bad request",
			body:          "not json",
			wantStatus:    http.StatusBadRequest,
			wantError:     "invalid request body",
			wantErrorCode: "bad_request",
			setupFunc: func(t *testing.T, ts *TestServer) (int64, string, int64) {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				return userID, fmt.Sprintf("%d", taskID), projectID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID, taskIDStr, projectID := tt.setupFunc(t, ts)

			if tt.taskID != "" {
				taskIDStr = tt.taskID
			}

			rec, req := makeAuthRequest(t, http.MethodPost, "/api/tasks/"+taskIDStr+"/attachments", tt.body, userID,
				map[string]string{"taskId": taskIDStr})
			ts.HandleCreateTaskAttachment(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			if tt.wantError != "" {
				AssertError(t, rec, tt.wantStatus, tt.wantError, tt.wantErrorCode)
			}

			if tt.checkResp != nil && tt.wantStatus == http.StatusCreated {
				var a TaskAttachment
				DecodeJSON(t, rec, &a)
				taskIDInt, _ := strconv.ParseInt(taskIDStr, 10, 64)
				tt.checkResp(t, a, userID, taskIDInt, projectID)
			}
		})
	}
}

func TestHandleCreateTaskAttachmentSetsProjectID(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	taskID := createTestTaskForCloudinary(t, ts, projectID)

	body := CreateAttachmentRequest{
		Filename:           "test.jpg",
		CloudinaryURL:      "https://res.cloudinary.com/test/test.jpg",
		CloudinaryPublicID: "test/test",
	}

	taskIDStr := fmt.Sprintf("%d", taskID)
	rec, req := makeAuthRequest(t, http.MethodPost, "/api/tasks/"+taskIDStr+"/attachments", body, userID,
		map[string]string{"taskId": taskIDStr})
	ts.HandleCreateTaskAttachment(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var a TaskAttachment
	DecodeJSON(t, rec, &a)

	// project_id should be automatically set from the task's project
	if a.ProjectID != projectID {
		t.Errorf("Expected project_id %d (from task), got %d", projectID, a.ProjectID)
	}
	if a.TaskID != taskID {
		t.Errorf("Expected task_id %d, got %d", taskID, a.TaskID)
	}
}

func TestHandleDeleteTaskAttachment(t *testing.T) {
	tests := []struct {
		name          string
		attachmentID  string
		wantStatus    int
		wantError     string
		wantErrorCode string
		setupFunc     func(t *testing.T, ts *TestServer) int64
	}{
		{
			name:       "owner can delete own attachment",
			wantStatus: http.StatusOK,
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo.jpg", "Photo")
				return userID
			},
		},
		{
			name:          "non-owner cannot delete others attachment",
			wantStatus:    http.StatusForbidden,
			wantError:     "you can only delete your own attachments",
			wantErrorCode: "forbidden",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
				otherID := ts.CreateTestUser(t, "other@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, ownerID)
				taskID := createTestTaskForCloudinary(t, ts, projectID)
				createTestAttachment(t, ts, taskID, ownerID, projectID, "image", "photo.jpg", "Photo")
				return otherID
			},
		},
		{
			name:          "invalid attachment ID returns 400",
			attachmentID:  "xyz",
			wantStatus:    http.StatusBadRequest,
			wantError:     "invalid attachment ID",
			wantErrorCode: "bad_request",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
		{
			name:          "non-existent attachment returns 404",
			attachmentID:  "99999",
			wantStatus:    http.StatusNotFound,
			wantError:     "attachment not found",
			wantErrorCode: "not_found",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "user@example.com", "password123")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID := tt.setupFunc(t, ts)

			attachmentID := tt.attachmentID
			if attachmentID == "" {
				attachmentID = "1" // first auto-increment attachment
			}

			rec, req := makeAuthRequest(t, http.MethodDelete, "/api/tasks/1/attachments/"+attachmentID, nil, userID,
				map[string]string{"attachmentId": attachmentID})
			ts.HandleDeleteTaskAttachment(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			if tt.wantError != "" {
				AssertError(t, rec, tt.wantStatus, tt.wantError, tt.wantErrorCode)
			}

			if tt.wantStatus == http.StatusOK {
				var resp map[string]string
				DecodeJSON(t, rec, &resp)
				if resp["message"] != "Attachment deleted" {
					t.Errorf("Expected deletion message, got %q", resp["message"])
				}
			}
		})
	}
}

func TestHandleDeleteTaskAttachmentVerifyRemoval(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	taskID := createTestTaskForCloudinary(t, ts, projectID)
	attachID := createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo.jpg", "Photo")

	attachmentIDStr := fmt.Sprintf("%d", attachID)

	// Delete via HandleDeleteTaskAttachment
	rec, req := makeAuthRequest(t, http.MethodDelete, "/api/tasks/1/attachments/"+attachmentIDStr, nil, userID,
		map[string]string{"attachmentId": attachmentIDStr})
	ts.HandleDeleteTaskAttachment(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	// Verify it is gone: querying task attachments should return empty
	taskIDStr := fmt.Sprintf("%d", taskID)
	rec2, req2 := makeAuthRequest(t, http.MethodGet, "/api/tasks/"+taskIDStr+"/attachments", nil, userID,
		map[string]string{"taskId": taskIDStr})
	ts.HandleListTaskAttachments(rec2, req2)
	AssertStatusCode(t, rec2.Code, http.StatusOK)

	var attachments []TaskAttachment
	DecodeJSON(t, rec2, &attachments)
	if len(attachments) != 0 {
		t.Errorf("Expected 0 attachments after deletion, got %d", len(attachments))
	}
}

func TestHandleCreateTaskAttachmentPersistence(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	taskID := createTestTaskForCloudinary(t, ts, projectID)

	body := CreateAttachmentRequest{
		Filename:           "report.pdf",
		AltName:            "Q4 Report",
		FileType:           "pdf",
		ContentType:        "application/pdf",
		FileSize:           5120,
		CloudinaryURL:      "https://res.cloudinary.com/test/raw/upload/report.pdf",
		CloudinaryPublicID: "test/report",
	}

	taskIDStr := fmt.Sprintf("%d", taskID)
	rec, req := makeAuthRequest(t, http.MethodPost, "/api/tasks/"+taskIDStr+"/attachments", body, userID,
		map[string]string{"taskId": taskIDStr})
	ts.HandleCreateTaskAttachment(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusCreated)

	// Verify the attachment appears in list
	rec2, req2 := makeAuthRequest(t, http.MethodGet, "/api/tasks/"+taskIDStr+"/attachments", nil, userID,
		map[string]string{"taskId": taskIDStr})
	ts.HandleListTaskAttachments(rec2, req2)
	AssertStatusCode(t, rec2.Code, http.StatusOK)

	var attachments []TaskAttachment
	DecodeJSON(t, rec2, &attachments)
	if len(attachments) != 1 {
		t.Fatalf("Expected 1 attachment, got %d", len(attachments))
	}

	a := attachments[0]
	if a.Filename != "report.pdf" {
		t.Errorf("Expected filename 'report.pdf', got %q", a.Filename)
	}
	if a.AltName != "Q4 Report" {
		t.Errorf("Expected alt_name 'Q4 Report', got %q", a.AltName)
	}
	if a.FileSize != 5120 {
		t.Errorf("Expected file_size 5120, got %d", a.FileSize)
	}
}

func TestHandleGetUploadSignature(t *testing.T) {
	tests := []struct {
		name          string
		queryParams   string // appended to path
		wantStatus    int
		wantError     string
		wantErrorCode string
		checkResp     func(t *testing.T, resp UploadSignatureResponse)
		setupFunc     func(t *testing.T, ts *TestServer) int64
	}{
		{
			name:       "generates signature with stored credentials and folder structure",
			wantStatus: http.StatusOK,
			checkResp: func(t *testing.T, resp UploadSignatureResponse) {
				if resp.Signature == "" {
					t.Error("Expected non-empty signature")
				}
				if resp.Timestamp == 0 {
					t.Error("Expected non-zero timestamp")
				}
				if resp.CloudName != "my-cloud" {
					t.Errorf("Expected cloud_name 'my-cloud', got %q", resp.CloudName)
				}
				if resp.APIKey != "my-api-key" {
					t.Errorf("Expected api_key 'my-api-key', got %q", resp.APIKey)
				}
				if resp.Folder != "taskai/test-project" {
					t.Errorf("Expected folder 'taskai/test-project', got %q", resp.Folder)
				}
				// public_id should be {taskID}_01
				if resp.PublicID == "" {
					t.Error("Expected non-empty public_id")
				}
			},
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				createTestCloudinaryCredential(t, ts, userID, "my-cloud", "my-api-key", "my-api-secret")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				createTestTaskForCloudinary(t, ts, projectID)
				return userID
			},
		},
		{
			name:          "returns error when no credentials exist",
			wantStatus:    http.StatusBadRequest,
			wantError:     "no Cloudinary credentials configured",
			wantErrorCode: "no_credentials",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := createTestProjectForCloudinary(t, ts, userID)
				createTestTaskForCloudinary(t, ts, projectID)
				return userID
			},
		},
		{
			name:          "missing task_id and page_id returns 400",
			queryParams:   "", // will override default
			wantStatus:    http.StatusBadRequest,
			wantError:     "task_id or page_id query parameter is required",
			wantErrorCode: "bad_request",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				createTestCloudinaryCredential(t, ts, userID, "cloud", "key", "secret")
				return userID
			},
		},
		{
			name:          "invalid task_id returns 400",
			queryParams:   "?task_id=abc",
			wantStatus:    http.StatusBadRequest,
			wantError:     "invalid task_id",
			wantErrorCode: "bad_request",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				createTestCloudinaryCredential(t, ts, userID, "cloud", "key", "secret")
				return userID
			},
		},
		{
			name:          "non-existent task returns 404",
			queryParams:   "?task_id=99999",
			wantStatus:    http.StatusNotFound,
			wantError:     "task not found",
			wantErrorCode: "not_found",
			setupFunc: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				createTestCloudinaryCredential(t, ts, userID, "cloud", "key", "secret")
				return userID
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID := tt.setupFunc(t, ts)

			// Build path: default includes ?task_id=1 unless overridden
			path := "/api/cloudinary/upload-signature?task_id=1"
			if tt.queryParams != "" {
				path = "/api/cloudinary/upload-signature" + tt.queryParams
			} else if tt.name == "missing task_id and page_id returns 400" {
				path = "/api/cloudinary/upload-signature"
			}

			rec, req := makeAuthRequest(t, http.MethodGet, path, nil, userID, nil)
			ts.HandleGetUploadSignature(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			if tt.wantError != "" {
				AssertError(t, rec, tt.wantStatus, tt.wantError, tt.wantErrorCode)
			}

			if tt.checkResp != nil && tt.wantStatus == http.StatusOK {
				var resp UploadSignatureResponse
				DecodeJSON(t, rec, &resp)
				tt.checkResp(t, resp)
			}
		})
	}
}

func TestHandleGetUploadSignatureDeterministic(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	createTestCloudinaryCredential(t, ts, userID, "cloud", "key", "secret")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	taskID := createTestTaskForCloudinary(t, ts, projectID)
	taskIDStr := strconv.FormatInt(taskID, 10)

	// Get two signatures - they should have the same cloud_name, api_key, folder, and public_id
	rec1, req1 := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/upload-signature?task_id="+taskIDStr, nil, userID, nil)
	ts.HandleGetUploadSignature(rec1, req1)
	AssertStatusCode(t, rec1.Code, http.StatusOK)

	var resp1 UploadSignatureResponse
	DecodeJSON(t, rec1, &resp1)

	rec2, req2 := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/upload-signature?task_id="+taskIDStr, nil, userID, nil)
	ts.HandleGetUploadSignature(rec2, req2)
	AssertStatusCode(t, rec2.Code, http.StatusOK)

	var resp2 UploadSignatureResponse
	DecodeJSON(t, rec2, &resp2)

	if resp1.CloudName != resp2.CloudName {
		t.Errorf("Cloud names should match: %q vs %q", resp1.CloudName, resp2.CloudName)
	}
	if resp1.APIKey != resp2.APIKey {
		t.Errorf("API keys should match: %q vs %q", resp1.APIKey, resp2.APIKey)
	}
	if resp1.Folder != resp2.Folder {
		t.Errorf("Folders should match: %q vs %q", resp1.Folder, resp2.Folder)
	}
	if resp1.PublicID != resp2.PublicID {
		t.Errorf("Public IDs should match: %q vs %q", resp1.PublicID, resp2.PublicID)
	}
}

func TestHandleTestCloudinaryConnection(t *testing.T) {
	t.Run("returns error when no credentials exist", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "user@example.com", "password123")

		rec, req := makeAuthRequest(t, http.MethodPost, "/api/cloudinary/test", nil, userID, nil)
		ts.HandleTestCloudinaryConnection(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
		AssertError(t, rec, http.StatusBadRequest, "no Cloudinary credentials configured", "no_credentials")
	})

	t.Run("tests connection with stored credentials and updates status", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "user@example.com", "password123")
		createTestCloudinaryCredential(t, ts, userID, "fake-cloud", "fake-key", "fake-secret")

		rec, req := makeAuthRequest(t, http.MethodPost, "/api/cloudinary/test", nil, userID, nil)
		ts.HandleTestCloudinaryConnection(rec, req)

		// With fake credentials, the external API call will fail, but the handler
		// should still return 200 with the updated credential status
		AssertStatusCode(t, rec.Code, http.StatusOK)

		var cred CloudinaryCredential
		DecodeJSON(t, rec, &cred)

		// Should have updated status to "error" since credentials are fake
		if cred.Status != "error" {
			t.Errorf("Expected status 'error' for fake credentials, got %q", cred.Status)
		}
		if cred.LastError == "" {
			t.Error("Expected non-empty last_error for failed connection")
		}
		if cred.ConsecutiveFailures != 1 {
			t.Errorf("Expected consecutive_failures=1, got %d", cred.ConsecutiveFailures)
		}
		if cred.LastCheckedAt == nil {
			t.Error("Expected last_checked_at to be set")
		}
	})
}

func TestHandleTestCloudinaryConnection_SuspendedAfterMultipleFailures(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	createTestCloudinaryCredential(t, ts, userID, "fake-cloud", "fake-key", "fake-secret")

	// Set consecutive_failures to 4 so next failure triggers suspension
	ctx := context.Background()
	_, err := ts.DB.ExecContext(ctx,
		`UPDATE cloudinary_credentials SET consecutive_failures = 4 WHERE user_id = ?`, userID)
	if err != nil {
		t.Fatalf("Failed to update consecutive_failures: %v", err)
	}

	rec, req := makeAuthRequest(t, http.MethodPost, "/api/cloudinary/test", nil, userID, nil)
	ts.HandleTestCloudinaryConnection(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var cred CloudinaryCredential
	DecodeJSON(t, rec, &cred)

	if cred.Status != "suspended" {
		t.Errorf("Expected status 'suspended' after 5 consecutive failures, got %q", cred.Status)
	}
	if cred.ConsecutiveFailures != 5 {
		t.Errorf("Expected consecutive_failures=5, got %d", cred.ConsecutiveFailures)
	}
}

func TestHandleUpdateAttachment_InvalidBody(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	taskID := createTestTaskForCloudinary(t, ts, projectID)
	attachID := createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo.jpg", "Alt")

	attachmentIDStr := fmt.Sprintf("%d", attachID)
	rec, req := makeAuthRequest(t, http.MethodPatch, "/api/attachments/"+attachmentIDStr, "not-json", userID,
		map[string]string{"id": attachmentIDStr})
	ts.HandleUpdateAttachment(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "invalid request body", "bad_request")
}

func TestSlugifyProjectName(t *testing.T) {
	tests := []struct {
		name      string
		input     string
		projectID int64
		want      string
	}{
		{"simple lowercase", "My Project", 1, "my-project"},
		{"special characters", "Hello! World@#2024", 2, "hello-world-2024"},
		{"consecutive hyphens", "my---project", 3, "my-project"},
		{"leading/trailing special", "  --My Project--  ", 4, "my-project"},
		{"unicode chars replaced", "Projet Tache", 5, "projet-tache"},
		{"empty after slugify", "!!!", 6, "project-6"},
		{"all spaces", "   ", 7, "project-7"},
		{"empty string", "", 8, "project-8"},
		{"already slugified", "my-project", 9, "my-project"},
		{"numbers only", "123", 10, "123"},
		{"mixed case with numbers", "TaskAI v2.0", 11, "taskai-v2-0"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slugifyProjectName(tt.input, tt.projectID)
			if got != tt.want {
				t.Errorf("slugifyProjectName(%q, %d) = %q, want %q", tt.input, tt.projectID, got, tt.want)
			}
		})
	}
}

func TestHandleGetUploadSignature_FolderAndPublicID(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	createTestCloudinaryCredential(t, ts, userID, "cloud", "key", "secret")

	// Create project with a specific name
	ctx := context.Background()
	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO projects (owner_id, name, description) VALUES (?, ?, ?)`,
		userID, "My Cool Project!", "desc")
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	projectID, _ := result.LastInsertId()

	taskResult, err := ts.DB.ExecContext(ctx,
		`INSERT INTO tasks (project_id, title, status, priority) VALUES (?, ?, ?, ?)`,
		projectID, "Task 1", "todo", "medium")
	if err != nil {
		t.Fatalf("Failed to create task: %v", err)
	}
	taskID, _ := taskResult.LastInsertId()
	taskIDStr := strconv.FormatInt(taskID, 10)

	// First upload: should be _01
	rec, req := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/upload-signature?task_id="+taskIDStr, nil, userID, nil)
	ts.HandleGetUploadSignature(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	var resp UploadSignatureResponse
	DecodeJSON(t, rec, &resp)

	expectedFolder := "taskai/my-cool-project"
	expectedPublicID := fmt.Sprintf("%d_01", taskID)

	if resp.Folder != expectedFolder {
		t.Errorf("Expected folder %q, got %q", expectedFolder, resp.Folder)
	}
	if resp.PublicID != expectedPublicID {
		t.Errorf("Expected public_id %q, got %q", expectedPublicID, resp.PublicID)
	}

	// Add an attachment, then get signature again — should be _02
	createTestAttachment(t, ts, taskID, userID, projectID, "image", "photo.jpg", "Photo")

	rec2, req2 := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/upload-signature?task_id="+taskIDStr, nil, userID, nil)
	ts.HandleGetUploadSignature(rec2, req2)
	AssertStatusCode(t, rec2.Code, http.StatusOK)

	var resp2 UploadSignatureResponse
	DecodeJSON(t, rec2, &resp2)

	expectedPublicID2 := fmt.Sprintf("%d_02", taskID)
	if resp2.PublicID != expectedPublicID2 {
		t.Errorf("Expected public_id %q after 1 attachment, got %q", expectedPublicID2, resp2.PublicID)
	}
}

func TestHandleGetUploadSignature_AttachmentLimitExceeded(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	createTestCloudinaryCredential(t, ts, userID, "cloud", "key", "secret")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	taskID := createTestTaskForCloudinary(t, ts, projectID)

	// Insert 99 attachments directly
	ctx := context.Background()
	for i := 0; i < 99; i++ {
		_, err := ts.DB.ExecContext(ctx,
			`INSERT INTO task_attachments (task_id, project_id, user_id, filename, alt_name, file_type, content_type, file_size, cloudinary_url, cloudinary_public_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			taskID, projectID, userID, fmt.Sprintf("file_%02d.jpg", i+1), "", "image", "image/jpeg", 1024,
			fmt.Sprintf("https://res.cloudinary.com/test/file_%02d.jpg", i+1),
			fmt.Sprintf("taskai/test-project/%d_%02d", taskID, i+1),
		)
		if err != nil {
			t.Fatalf("Failed to insert attachment %d: %v", i+1, err)
		}
	}

	taskIDStr := strconv.FormatInt(taskID, 10)
	rec, req := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/upload-signature?task_id="+taskIDStr, nil, userID, nil)
	ts.HandleGetUploadSignature(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "maximum 99 attachments per task", "attachment_limit_exceeded")
}

func TestHandleCreateTaskAttachment_AttachmentLimitExceeded(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	taskID := createTestTaskForCloudinary(t, ts, projectID)

	// Insert 99 attachments directly
	ctx := context.Background()
	for i := 0; i < 99; i++ {
		_, err := ts.DB.ExecContext(ctx,
			`INSERT INTO task_attachments (task_id, project_id, user_id, filename, alt_name, file_type, content_type, file_size, cloudinary_url, cloudinary_public_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			taskID, projectID, userID, fmt.Sprintf("file_%02d.jpg", i+1), "", "image", "image/jpeg", 1024,
			fmt.Sprintf("https://res.cloudinary.com/test/file_%02d.jpg", i+1),
			fmt.Sprintf("taskai/test-project/%d_%02d", taskID, i+1),
		)
		if err != nil {
			t.Fatalf("Failed to insert attachment %d: %v", i+1, err)
		}
	}

	// Try to create the 100th via the handler
	body := CreateAttachmentRequest{
		Filename:           "overflow.jpg",
		CloudinaryURL:      "https://res.cloudinary.com/test/overflow.jpg",
		CloudinaryPublicID: "test/overflow",
	}
	taskIDStr := strconv.FormatInt(taskID, 10)
	rec, req := makeAuthRequest(t, http.MethodPost, "/api/tasks/"+taskIDStr+"/attachments", body, userID,
		map[string]string{"taskId": taskIDStr})
	ts.HandleCreateTaskAttachment(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "maximum 99 attachments per task", "attachment_limit_exceeded")
}

// --- Wiki page attachment tests ---

// createTestWikiPage creates a wiki page in the given project and returns its ID
func createTestWikiPage(t *testing.T, ts *TestServer, projectID, createdBy int64) int64 {
	t.Helper()

	ctx := context.Background()
	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO wiki_pages (project_id, title, slug, created_by) VALUES (?, ?, ?, ?)`,
		projectID, "Test Wiki Page", "test-wiki-page", createdBy,
	)
	if err != nil {
		t.Fatalf("Failed to create test wiki page: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get wiki page ID: %v", err)
	}

	return id
}

// createTestWikiPageAttachment inserts a wiki_page_attachments row and returns the attachment ID
func createTestWikiPageAttachment(t *testing.T, ts *TestServer, pageID, userID, projectID int64, fileType, filename, altName string) int64 {
	t.Helper()

	ctx := context.Background()
	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO wiki_page_attachments (wiki_page_id, project_id, user_id, filename, alt_name, file_type, content_type, file_size, cloudinary_url, cloudinary_public_id)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		pageID, projectID, userID, filename, altName, fileType, "application/octet-stream", 1024,
		"https://res.cloudinary.com/test/"+filename, "test/"+filename,
	)
	if err != nil {
		t.Fatalf("Failed to create test wiki page attachment: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get attachment ID: %v", err)
	}

	return id
}

func TestHandleGetUploadSignature_WikiPage(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	createTestCloudinaryCredential(t, ts, userID, "cloud", "key", "secret")

	ctx := context.Background()
	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO projects (owner_id, name, description) VALUES (?, ?, ?)`,
		userID, "Wiki Project", "desc")
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	projectID, _ := result.LastInsertId()
	pageID := createTestWikiPage(t, ts, projectID, userID)
	pageIDStr := strconv.FormatInt(pageID, 10)

	// First upload: should be w{pageID}_001
	rec, req := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/upload-signature?page_id="+pageIDStr, nil, userID, nil)
	ts.HandleGetUploadSignature(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	var resp UploadSignatureResponse
	DecodeJSON(t, rec, &resp)

	expectedFolder := "taskai/wiki-project"
	expectedPublicID := fmt.Sprintf("w%d_001", pageID)

	if resp.Folder != expectedFolder {
		t.Errorf("Expected folder %q, got %q", expectedFolder, resp.Folder)
	}
	if resp.PublicID != expectedPublicID {
		t.Errorf("Expected public_id %q, got %q", expectedPublicID, resp.PublicID)
	}

	// Add an attachment, then get signature again — should be _002
	createTestWikiPageAttachment(t, ts, pageID, userID, projectID, "image", "photo.jpg", "Photo")

	rec2, req2 := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/upload-signature?page_id="+pageIDStr, nil, userID, nil)
	ts.HandleGetUploadSignature(rec2, req2)
	AssertStatusCode(t, rec2.Code, http.StatusOK)

	var resp2 UploadSignatureResponse
	DecodeJSON(t, rec2, &resp2)

	expectedPublicID2 := fmt.Sprintf("w%d_002", pageID)
	if resp2.PublicID != expectedPublicID2 {
		t.Errorf("Expected public_id %q after 1 attachment, got %q", expectedPublicID2, resp2.PublicID)
	}
}

func TestHandleGetUploadSignature_BothTaskAndPageReturns400(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	createTestCloudinaryCredential(t, ts, userID, "cloud", "key", "secret")

	rec, req := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/upload-signature?task_id=1&page_id=1", nil, userID, nil)
	ts.HandleGetUploadSignature(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "provide either task_id or page_id, not both", "bad_request")
}

func TestHandleListWikiPageAttachments(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	pageID := createTestWikiPage(t, ts, projectID, userID)
	pageIDStr := strconv.FormatInt(pageID, 10)

	// Create some attachments
	createTestWikiPageAttachment(t, ts, pageID, userID, projectID, "image", "img1.jpg", "Image 1")
	createTestWikiPageAttachment(t, ts, pageID, userID, projectID, "image", "img2.jpg", "Image 2")

	rec, req := makeAuthRequest(t, http.MethodGet, "/api/wiki/pages/"+pageIDStr+"/attachments", nil, userID,
		map[string]string{"pageId": pageIDStr})
	ts.HandleListWikiPageAttachments(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var attachments []WikiPageAttachment
	DecodeJSON(t, rec, &attachments)

	if len(attachments) != 2 {
		t.Errorf("Expected 2 attachments, got %d", len(attachments))
	}
}

func TestHandleCreateWikiPageAttachment(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	pageID := createTestWikiPage(t, ts, projectID, userID)
	pageIDStr := strconv.FormatInt(pageID, 10)

	body := CreateAttachmentRequest{
		Filename:           "test.jpg",
		AltName:            "Test image",
		FileType:           "image",
		ContentType:        "image/jpeg",
		FileSize:           2048,
		CloudinaryURL:      "https://res.cloudinary.com/test/test.jpg",
		CloudinaryPublicID: "taskai/test-project/w1_001",
	}

	rec, req := makeAuthRequest(t, http.MethodPost, "/api/wiki/pages/"+pageIDStr+"/attachments", body, userID,
		map[string]string{"pageId": pageIDStr})
	ts.HandleCreateWikiPageAttachment(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var attachment WikiPageAttachment
	DecodeJSON(t, rec, &attachment)

	if attachment.WikiPageID != pageID {
		t.Errorf("Expected wiki_page_id %d, got %d", pageID, attachment.WikiPageID)
	}
	if attachment.ProjectID != projectID {
		t.Errorf("Expected project_id %d, got %d", projectID, attachment.ProjectID)
	}
	if attachment.Filename != "test.jpg" {
		t.Errorf("Expected filename 'test.jpg', got %q", attachment.Filename)
	}
	if attachment.AltName != "Test image" {
		t.Errorf("Expected alt_name 'Test image', got %q", attachment.AltName)
	}
}

func TestHandleCreateWikiPageAttachment_ValidationError(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	pageID := createTestWikiPage(t, ts, projectID, userID)
	pageIDStr := strconv.FormatInt(pageID, 10)

	// Missing required fields
	body := CreateAttachmentRequest{
		Filename: "test.jpg",
	}

	rec, req := makeAuthRequest(t, http.MethodPost, "/api/wiki/pages/"+pageIDStr+"/attachments", body, userID,
		map[string]string{"pageId": pageIDStr})
	ts.HandleCreateWikiPageAttachment(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "filename, cloudinary_url, and cloudinary_public_id are required", "validation_error")
}

func TestHandleCreateWikiPageAttachment_LimitExceeded(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	pageID := createTestWikiPage(t, ts, projectID, userID)
	pageIDStr := strconv.FormatInt(pageID, 10)

	// Insert 100 attachments directly
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		_, err := ts.DB.ExecContext(ctx,
			`INSERT INTO wiki_page_attachments (wiki_page_id, project_id, user_id, filename, alt_name, file_type, content_type, file_size, cloudinary_url, cloudinary_public_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			pageID, projectID, userID, fmt.Sprintf("file_%03d.jpg", i+1), "", "image", "image/jpeg", 1024,
			fmt.Sprintf("https://res.cloudinary.com/test/file_%03d.jpg", i+1),
			fmt.Sprintf("taskai/test-project/w%d_%03d", pageID, i+1),
		)
		if err != nil {
			t.Fatalf("Failed to insert attachment %d: %v", i+1, err)
		}
	}

	body := CreateAttachmentRequest{
		Filename:           "overflow.jpg",
		CloudinaryURL:      "https://res.cloudinary.com/test/overflow.jpg",
		CloudinaryPublicID: "test/overflow",
	}

	rec, req := makeAuthRequest(t, http.MethodPost, "/api/wiki/pages/"+pageIDStr+"/attachments", body, userID,
		map[string]string{"pageId": pageIDStr})
	ts.HandleCreateWikiPageAttachment(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "maximum 100 attachments per wiki page", "attachment_limit_exceeded")
}

func TestHandleDeleteWikiPageAttachment(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	pageID := createTestWikiPage(t, ts, projectID, userID)

	attachmentID := createTestWikiPageAttachment(t, ts, pageID, userID, projectID, "image", "img.jpg", "My image")
	attachmentIDStr := strconv.FormatInt(attachmentID, 10)

	rec, req := makeAuthRequest(t, http.MethodDelete, "/api/wiki/attachments/"+attachmentIDStr, nil, userID,
		map[string]string{"attachmentId": attachmentIDStr})
	ts.HandleDeleteWikiPageAttachment(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	// Verify it's actually deleted
	var count int
	err := ts.DB.QueryRow(`SELECT COUNT(*) FROM wiki_page_attachments WHERE id = ?`, attachmentID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to count: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected attachment to be deleted, but count is %d", count)
	}
}

func TestHandleDeleteWikiPageAttachment_OwnershipCheck(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	user1ID := ts.CreateTestUser(t, "user1@example.com", "password123")
	user2ID := ts.CreateTestUser(t, "user2@example.com", "password123")
	projectID := createTestProjectForCloudinary(t, ts, user1ID)
	pageID := createTestWikiPage(t, ts, projectID, user1ID)

	// Create attachment owned by user1
	attachmentID := createTestWikiPageAttachment(t, ts, pageID, user1ID, projectID, "image", "img.jpg", "My image")
	attachmentIDStr := strconv.FormatInt(attachmentID, 10)

	// Try to delete as user2
	rec, req := makeAuthRequest(t, http.MethodDelete, "/api/wiki/attachments/"+attachmentIDStr, nil, user2ID,
		map[string]string{"attachmentId": attachmentIDStr})
	ts.HandleDeleteWikiPageAttachment(rec, req)

	AssertError(t, rec, http.StatusForbidden, "you can only delete your own attachments", "forbidden")
}

func TestHandleDeleteWikiPageAttachment_NotFound(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")

	rec, req := makeAuthRequest(t, http.MethodDelete, "/api/wiki/attachments/99999", nil, userID,
		map[string]string{"attachmentId": "99999"})
	ts.HandleDeleteWikiPageAttachment(rec, req)

	AssertError(t, rec, http.StatusNotFound, "attachment not found", "not_found")
}

func TestHandleGetUploadSignature_WikiPageLimitExceeded(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")
	createTestCloudinaryCredential(t, ts, userID, "cloud", "key", "secret")
	projectID := createTestProjectForCloudinary(t, ts, userID)
	pageID := createTestWikiPage(t, ts, projectID, userID)

	// Insert 100 wiki page attachments
	ctx := context.Background()
	for i := 0; i < 100; i++ {
		_, err := ts.DB.ExecContext(ctx,
			`INSERT INTO wiki_page_attachments (wiki_page_id, project_id, user_id, filename, alt_name, file_type, content_type, file_size, cloudinary_url, cloudinary_public_id)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			pageID, projectID, userID, fmt.Sprintf("file_%03d.jpg", i+1), "", "image", "image/jpeg", 1024,
			fmt.Sprintf("https://res.cloudinary.com/test/file_%03d.jpg", i+1),
			fmt.Sprintf("taskai/test-project/w%d_%03d", pageID, i+1),
		)
		if err != nil {
			t.Fatalf("Failed to insert attachment %d: %v", i+1, err)
		}
	}

	pageIDStr := strconv.FormatInt(pageID, 10)
	rec, req := makeAuthRequest(t, http.MethodGet, "/api/cloudinary/upload-signature?page_id="+pageIDStr, nil, userID, nil)
	ts.HandleGetUploadSignature(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "maximum 100 attachments per wiki page", "attachment_limit_exceeded")
}

