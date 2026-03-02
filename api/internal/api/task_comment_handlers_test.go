package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHandleListTaskCommentsEmpty(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	rec, req := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/tasks/%d/comments", taskID), nil, userID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})

	ts.HandleListTaskComments(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var comments []TaskComment
	DecodeJSON(t, rec, &comments)

	if len(comments) != 0 {
		t.Errorf("Expected 0 comments, got %d", len(comments))
	}
}

func TestHandleListTaskCommentsWithComments(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Insert comments directly
	_, err := ts.DB.ExecContext(ctx,
		`INSERT INTO task_comments (task_id, user_id, comment) VALUES (?, ?, ?)`,
		taskID, userID, "First comment",
	)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	_, err = ts.DB.ExecContext(ctx,
		`INSERT INTO task_comments (task_id, user_id, comment) VALUES (?, ?, ?)`,
		taskID, userID, "Second comment",
	)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/tasks/%d/comments", taskID), nil, userID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})

	ts.HandleListTaskComments(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var comments []TaskComment
	DecodeJSON(t, rec, &comments)

	if len(comments) != 2 {
		t.Fatalf("Expected 2 comments, got %d", len(comments))
	}

	// Verify ordering (ASC by created_at)
	if comments[0].Comment != "First comment" {
		t.Errorf("Expected first comment 'First comment', got %q", comments[0].Comment)
	}
	if comments[1].Comment != "Second comment" {
		t.Errorf("Expected second comment 'Second comment', got %q", comments[1].Comment)
	}

	// Verify fields
	if comments[0].TaskID != taskID {
		t.Errorf("Expected task_id %d, got %d", taskID, comments[0].TaskID)
	}
	if comments[0].UserID != userID {
		t.Errorf("Expected user_id %d, got %d", userID, comments[0].UserID)
	}
	if comments[0].ID == 0 {
		t.Error("Expected non-zero comment ID")
	}
}

func TestHandleCreateTaskComment(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	body := CreateCommentRequest{
		Comment: "This is a new comment",
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/tasks/%d/comments", taskID), body, userID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})

	ts.HandleCreateTaskComment(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var comment TaskComment
	DecodeJSON(t, rec, &comment)

	if comment.Comment != "This is a new comment" {
		t.Errorf("Expected comment 'This is a new comment', got %q", comment.Comment)
	}
	if comment.TaskID != taskID {
		t.Errorf("Expected task_id %d, got %d", taskID, comment.TaskID)
	}
	if comment.UserID != userID {
		t.Errorf("Expected user_id %d, got %d", userID, comment.UserID)
	}
	if comment.ID == 0 {
		t.Error("Expected non-zero comment ID")
	}
}

func TestHandleCreateTaskCommentValidation(t *testing.T) {
	tests := []struct {
		name       string
		body       CreateCommentRequest
		wantStatus int
		wantError  string
	}{
		{
			name:       "empty comment",
			body:       CreateCommentRequest{Comment: ""},
			wantStatus: http.StatusBadRequest,
			wantError:  "comment is required",
		},
		{
			name:       "comment too long",
			body:       CreateCommentRequest{Comment: strings.Repeat("x", 5001)},
			wantStatus: http.StatusBadRequest,
			wantError:  "comment is too long",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID := ts.CreateTestUser(t, "test@example.com", "password123")
			projectID := ts.CreateTestProject(t, userID, "Test Project")
			taskID := ts.CreateTestTask(t, projectID, "Test Task")

			rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/tasks/%d/comments", taskID), tt.body, userID,
				map[string]string{"taskId": fmt.Sprintf("%d", taskID)})

			ts.HandleCreateTaskComment(rec, req)

			AssertError(t, rec, tt.wantStatus, tt.wantError, "invalid_input")
		})
	}
}

func TestHandleListTaskCommentsUnauthorized(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	user1ID := ts.CreateTestUser(t, "user1@example.com", "password123")
	user2ID := ts.CreateTestUser(t, "user2@example.com", "password123")
	projectID := ts.CreateTestProject(t, user1ID, "User1 Project")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	// user2 is NOT a member of user1's project
	rec, req := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/tasks/%d/comments", taskID), nil, user2ID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})

	ts.HandleListTaskComments(rec, req)

	AssertError(t, rec, http.StatusForbidden, "access denied", "forbidden")
}

func TestHandleCreateTaskCommentUnauthorized(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	user1ID := ts.CreateTestUser(t, "user1@example.com", "password123")
	user2ID := ts.CreateTestUser(t, "user2@example.com", "password123")
	projectID := ts.CreateTestProject(t, user1ID, "User1 Project")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	body := CreateCommentRequest{
		Comment: "Unauthorized comment",
	}

	// user2 is NOT a member of user1's project
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/tasks/%d/comments", taskID), body, user2ID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})

	ts.HandleCreateTaskComment(rec, req)

	AssertError(t, rec, http.StatusForbidden, "access denied", "forbidden")
}

func TestHandleListTaskCommentsInvalidTaskID(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/tasks/abc/comments", nil, userID,
		map[string]string{"taskId": "abc"})

	ts.HandleListTaskComments(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "invalid task ID", "invalid_input")
}

func TestHandleCreateTaskCommentInvalidTaskID(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")

	body := CreateCommentRequest{
		Comment: "Some comment",
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/tasks/abc/comments", body, userID,
		map[string]string{"taskId": "abc"})

	ts.HandleCreateTaskComment(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "invalid task ID", "invalid_input")
}

func TestHandleListTaskCommentsTaskNotFound(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/tasks/99999/comments", nil, userID,
		map[string]string{"taskId": "99999"})

	ts.HandleListTaskComments(rec, req)

	AssertError(t, rec, http.StatusNotFound, "task not found", "not_found")
}

func TestHandleCreateTaskCommentTaskNotFound(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")

	body := CreateCommentRequest{
		Comment: "Comment on ghost task",
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/tasks/99999/comments", body, userID,
		map[string]string{"taskId": "99999"})

	ts.HandleCreateTaskComment(rec, req)

	AssertError(t, rec, http.StatusNotFound, "task not found", "not_found")
}

// ---------------------------------------------------------------------------
// HandleCreateTaskComment — invalid JSON body
// ---------------------------------------------------------------------------

func TestHandleCreateTaskCommentInvalidJSON(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/tasks/%d/comments", taskID),
		"not-json", userID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})

	ts.HandleCreateTaskComment(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "invalid request body", "invalid_input")
}

// ---------------------------------------------------------------------------
// HandleCreateTaskComment — boundary: comment at exactly max length (5000 chars)
// ---------------------------------------------------------------------------

func TestHandleCreateTaskCommentMaxLength(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	// Exactly 5000 characters should succeed
	body := CreateCommentRequest{
		Comment: strings.Repeat("x", 5000),
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/tasks/%d/comments", taskID), body, userID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})

	ts.HandleCreateTaskComment(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var comment TaskComment
	DecodeJSON(t, rec, &comment)

	if len(comment.Comment) != 5000 {
		t.Errorf("Expected comment length 5000, got %d", len(comment.Comment))
	}
}

// ---------------------------------------------------------------------------
// HandleCreateTaskComment — verify user_name in response
// ---------------------------------------------------------------------------

func TestHandleCreateTaskCommentWithUserName(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	// Set user's name
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := ts.DB.ExecContext(ctx, `UPDATE users SET name = 'Alice Johnson' WHERE id = ?`, userID)
	if err != nil {
		t.Fatalf("Failed to set user name: %v", err)
	}

	body := CreateCommentRequest{
		Comment: "Comment with user name",
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/tasks/%d/comments", taskID), body, userID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})

	ts.HandleCreateTaskComment(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var comment TaskComment
	DecodeJSON(t, rec, &comment)

	if comment.UserName == nil {
		t.Fatal("Expected user_name to be set in response")
	}
	if *comment.UserName != "Alice Johnson" {
		t.Errorf("Expected user_name 'Alice Johnson', got %q", *comment.UserName)
	}
}

// ---------------------------------------------------------------------------
// HandleListTaskComments — verify user_name populated
// ---------------------------------------------------------------------------

func TestHandleListTaskCommentsWithUserNames(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	// Set user's name
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_, err := ts.DB.ExecContext(ctx, `UPDATE users SET name = 'Bob Williams' WHERE id = ?`, userID)
	if err != nil {
		t.Fatalf("Failed to set user name: %v", err)
	}

	// Insert a comment directly
	_, err = ts.DB.ExecContext(ctx,
		`INSERT INTO task_comments (task_id, user_id, comment) VALUES (?, ?, ?)`,
		taskID, userID, "Test comment with name",
	)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/tasks/%d/comments", taskID), nil, userID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})

	ts.HandleListTaskComments(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var comments []TaskComment
	DecodeJSON(t, rec, &comments)

	if len(comments) != 1 {
		t.Fatalf("Expected 1 comment, got %d", len(comments))
	}

	if comments[0].UserName == nil {
		t.Fatal("Expected user_name to be set")
	}
	if *comments[0].UserName != "Bob Williams" {
		t.Errorf("Expected user_name 'Bob Williams', got %q", *comments[0].UserName)
	}
}

// ---------------------------------------------------------------------------
// Integration: create comment then list it
// ---------------------------------------------------------------------------

func TestCommentCreateThenList(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	// Create comment via handler
	createBody := CreateCommentRequest{Comment: "Integration test comment"}
	createRec, createReq := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/tasks/%d/comments", taskID),
		createBody, userID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})

	ts.HandleCreateTaskComment(createRec, createReq)
	AssertStatusCode(t, createRec.Code, http.StatusCreated)

	var created TaskComment
	DecodeJSON(t, createRec, &created)

	// List comments and verify
	listRec, listReq := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/tasks/%d/comments", taskID), nil, userID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})

	ts.HandleListTaskComments(listRec, listReq)
	AssertStatusCode(t, listRec.Code, http.StatusOK)

	var comments []TaskComment
	DecodeJSON(t, listRec, &comments)

	if len(comments) != 1 {
		t.Fatalf("Expected 1 comment, got %d", len(comments))
	}

	if comments[0].ID != created.ID {
		t.Errorf("Expected comment ID %d, got %d", created.ID, comments[0].ID)
	}
	if comments[0].Comment != "Integration test comment" {
		t.Errorf("Expected comment text 'Integration test comment', got %q", comments[0].Comment)
	}
	if comments[0].TaskID != taskID {
		t.Errorf("Expected task_id %d, got %d", taskID, comments[0].TaskID)
	}
	if comments[0].UserID != userID {
		t.Errorf("Expected user_id %d, got %d", userID, comments[0].UserID)
	}
}

// ---------------------------------------------------------------------------
// Multiple users commenting on the same task
// ---------------------------------------------------------------------------

func TestMultipleUsersCommenting(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	memberID := ts.CreateTestUser(t, "member@example.com", "password123")

	projectID := ts.CreateTestProject(t, ownerID, "Shared Project")
	ts.AddProjectMember(t, projectID, memberID, ownerID, "member")
	taskID := ts.CreateTestTask(t, projectID, "Shared Task")

	// Owner creates a comment
	body1 := CreateCommentRequest{Comment: "Owner's comment"}
	rec1, req1 := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/tasks/%d/comments", taskID),
		body1, ownerID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})
	ts.HandleCreateTaskComment(rec1, req1)
	AssertStatusCode(t, rec1.Code, http.StatusCreated)

	// Member creates a comment
	body2 := CreateCommentRequest{Comment: "Member's comment"}
	rec2, req2 := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/tasks/%d/comments", taskID),
		body2, memberID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})
	ts.HandleCreateTaskComment(rec2, req2)
	AssertStatusCode(t, rec2.Code, http.StatusCreated)

	// List all comments — should see both
	listRec, listReq := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/tasks/%d/comments", taskID), nil, ownerID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})
	ts.HandleListTaskComments(listRec, listReq)
	AssertStatusCode(t, listRec.Code, http.StatusOK)

	var comments []TaskComment
	DecodeJSON(t, listRec, &comments)

	if len(comments) != 2 {
		t.Fatalf("Expected 2 comments, got %d", len(comments))
	}

	// Verify comments are ordered by created_at ascending
	if comments[0].Comment != "Owner's comment" {
		t.Errorf("Expected first comment 'Owner's comment', got %q", comments[0].Comment)
	}
	if comments[1].Comment != "Member's comment" {
		t.Errorf("Expected second comment 'Member's comment', got %q", comments[1].Comment)
	}

	// Verify different user IDs
	if comments[0].UserID != ownerID {
		t.Errorf("Expected first comment user_id %d, got %d", ownerID, comments[0].UserID)
	}
	if comments[1].UserID != memberID {
		t.Errorf("Expected second comment user_id %d, got %d", memberID, comments[1].UserID)
	}
}

// ---------------------------------------------------------------------------
// Project member (non-owner) can create and list comments
// ---------------------------------------------------------------------------

func TestProjectMemberCanComment(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	memberID := ts.CreateTestUser(t, "member@example.com", "password123")

	projectID := ts.CreateTestProject(t, ownerID, "Test Project")
	ts.AddProjectMember(t, projectID, memberID, ownerID, "member")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	// Member creates a comment
	body := CreateCommentRequest{Comment: "Member comment"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/tasks/%d/comments", taskID),
		body, memberID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})
	ts.HandleCreateTaskComment(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var comment TaskComment
	DecodeJSON(t, rec, &comment)

	if comment.Comment != "Member comment" {
		t.Errorf("Expected comment 'Member comment', got %q", comment.Comment)
	}
	if comment.UserID != memberID {
		t.Errorf("Expected user_id %d, got %d", memberID, comment.UserID)
	}

	// Member can also list comments
	listRec, listReq := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/tasks/%d/comments", taskID), nil, memberID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})
	ts.HandleListTaskComments(listRec, listReq)
	AssertStatusCode(t, listRec.Code, http.StatusOK)

	var comments []TaskComment
	DecodeJSON(t, listRec, &comments)

	if len(comments) != 1 {
		t.Errorf("Expected 1 comment, got %d", len(comments))
	}
}

// ---------------------------------------------------------------------------
// HandleCreateTaskComment — whitespace-only comment should be rejected
// (tests that the empty check catches whitespace after trimming, or not)
// ---------------------------------------------------------------------------

func TestHandleCreateTaskCommentWhitespaceOnly(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	// The handler checks for empty string, whitespace-only may or may not be rejected
	// This tests the actual behavior
	body := CreateCommentRequest{Comment: "   "}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/tasks/%d/comments", taskID),
		body, userID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})
	ts.HandleCreateTaskComment(rec, req)

	// The handler does not trim whitespace, so "   " is not empty — it should succeed
	AssertStatusCode(t, rec.Code, http.StatusCreated)
}

// ---------------------------------------------------------------------------
// HandleListTaskComments — comments from different tasks are isolated
// ---------------------------------------------------------------------------

func TestHandleListTaskCommentsIsolatedPerTask(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	task1ID := ts.CreateTestTask(t, projectID, "Task 1")
	task2ID := ts.CreateTestTask(t, projectID, "Task 2")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Insert comments on task1
	_, err := ts.DB.ExecContext(ctx,
		`INSERT INTO task_comments (task_id, user_id, comment) VALUES (?, ?, ?)`,
		task1ID, userID, "Comment on task 1",
	)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	// Insert comments on task2
	_, err = ts.DB.ExecContext(ctx,
		`INSERT INTO task_comments (task_id, user_id, comment) VALUES (?, ?, ?)`,
		task2ID, userID, "Comment on task 2",
	)
	if err != nil {
		t.Fatalf("Failed to create comment: %v", err)
	}

	// List comments for task1 — should only see task1's comment
	rec, req := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/tasks/%d/comments", task1ID), nil, userID,
		map[string]string{"taskId": fmt.Sprintf("%d", task1ID)})
	ts.HandleListTaskComments(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	var comments []TaskComment
	DecodeJSON(t, rec, &comments)

	if len(comments) != 1 {
		t.Fatalf("Expected 1 comment for task1, got %d", len(comments))
	}
	if comments[0].Comment != "Comment on task 1" {
		t.Errorf("Expected 'Comment on task 1', got %q", comments[0].Comment)
	}

	// List comments for task2 — should only see task2's comment
	rec2, req2 := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/tasks/%d/comments", task2ID), nil, userID,
		map[string]string{"taskId": fmt.Sprintf("%d", task2ID)})
	ts.HandleListTaskComments(rec2, req2)
	AssertStatusCode(t, rec2.Code, http.StatusOK)

	var comments2 []TaskComment
	DecodeJSON(t, rec2, &comments2)

	if len(comments2) != 1 {
		t.Fatalf("Expected 1 comment for task2, got %d", len(comments2))
	}
	if comments2[0].Comment != "Comment on task 2" {
		t.Errorf("Expected 'Comment on task 2', got %q", comments2[0].Comment)
	}
}

// ---------------------------------------------------------------------------
// HandleCreateTaskComment — response fields verification
// ---------------------------------------------------------------------------

func TestHandleCreateTaskCommentResponseFields(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	body := CreateCommentRequest{Comment: "Field verification comment"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/tasks/%d/comments", taskID),
		body, userID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})

	ts.HandleCreateTaskComment(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var comment TaskComment
	DecodeJSON(t, rec, &comment)

	// Verify all required fields are set
	if comment.ID == 0 {
		t.Error("Expected non-zero comment ID")
	}
	if comment.TaskID != taskID {
		t.Errorf("Expected task_id %d, got %d", taskID, comment.TaskID)
	}
	if comment.UserID != userID {
		t.Errorf("Expected user_id %d, got %d", userID, comment.UserID)
	}
	if comment.Comment != "Field verification comment" {
		t.Errorf("Expected comment text 'Field verification comment', got %q", comment.Comment)
	}
	if comment.CreatedAt.IsZero() {
		t.Error("Expected non-zero created_at")
	}
	if comment.UpdatedAt.IsZero() {
		t.Error("Expected non-zero updated_at")
	}
}

// ---------------------------------------------------------------------------
// HandleCreateTaskComment — multiple rapid creates
// ---------------------------------------------------------------------------

func TestHandleCreateTaskCommentMultiple(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")
	taskID := ts.CreateTestTask(t, projectID, "Test Task")

	// Create 5 comments
	for i := 1; i <= 5; i++ {
		body := CreateCommentRequest{Comment: fmt.Sprintf("Comment #%d", i)}
		rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/tasks/%d/comments", taskID),
			body, userID,
			map[string]string{"taskId": fmt.Sprintf("%d", taskID)})
		ts.HandleCreateTaskComment(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusCreated)
	}

	// List all — should see all 5
	listRec, listReq := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/tasks/%d/comments", taskID), nil, userID,
		map[string]string{"taskId": fmt.Sprintf("%d", taskID)})
	ts.HandleListTaskComments(listRec, listReq)
	AssertStatusCode(t, listRec.Code, http.StatusOK)

	var comments []TaskComment
	DecodeJSON(t, listRec, &comments)

	if len(comments) != 5 {
		t.Fatalf("Expected 5 comments, got %d", len(comments))
	}

	// Verify ordering
	for i, c := range comments {
		expected := fmt.Sprintf("Comment #%d", i+1)
		if c.Comment != expected {
			t.Errorf("Comment %d: expected %q, got %q", i, expected, c.Comment)
		}
	}
}
