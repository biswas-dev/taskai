package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// createTestSprint inserts a sprint with the given project_id and returns the sprint ID.
func createTestSprint(t *testing.T, ts *TestServer, userID, projectID int64, name, status string) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO sprints (user_id, project_id, name, status) VALUES (?, ?, ?, ?)`,
		userID, projectID, name, status,
	)
	if err != nil {
		t.Fatalf("Failed to create test sprint: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get sprint ID: %v", err)
	}

	return id
}

// createTestTag inserts a tag with the given project_id and returns the tag ID.
func createTestTag(t *testing.T, ts *TestServer, userID, projectID int64, name, color string) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO tags (user_id, project_id, name, color) VALUES (?, ?, ?, ?)`,
		userID, projectID, name, color,
	)
	if err != nil {
		t.Fatalf("Failed to create test tag: %v", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get tag ID: %v", err)
	}

	return id
}

// --- Sprint Tests ---

func TestHandleListSprints(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/projects/%d/sprints", projectID), nil, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleListSprints(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var sprints []json.RawMessage
		DecodeJSON(t, rec, &sprints)

		if len(sprints) != 0 {
			t.Errorf("Expected 0 sprints, got %d", len(sprints))
		}
	})

	t.Run("with sprints", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		createTestSprint(t, ts, userID, projectID, "Sprint 1", "active")
		createTestSprint(t, ts, userID, projectID, "Sprint 2", "planned")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/projects/%d/sprints", projectID), nil, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleListSprints(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var sprints []Sprint
		DecodeJSON(t, rec, &sprints)

		if len(sprints) != 2 {
			t.Fatalf("Expected 2 sprints, got %d", len(sprints))
		}
	})

	t.Run("forbidden for non-member", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		otherID := ts.CreateTestUser(t, "other@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Private Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/projects/%d/sprints", projectID), nil, otherID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleListSprints(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusForbidden)
	})
}

func TestHandleCreateSprint(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		body := CreateSprintRequest{
			Name:   "Sprint Alpha",
			Goal:   "Deliver MVP",
			Status: "active",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/projects/%d/sprints", projectID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleCreateSprint(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var sprint Sprint
		DecodeJSON(t, rec, &sprint)

		if sprint.Name != "Sprint Alpha" {
			t.Errorf("Expected name 'Sprint Alpha', got '%s'", sprint.Name)
		}
		if sprint.Goal != "Deliver MVP" {
			t.Errorf("Expected goal 'Deliver MVP', got '%s'", sprint.Goal)
		}
		if sprint.Status != "active" {
			t.Errorf("Expected status 'active', got '%s'", sprint.Status)
		}
	})

	t.Run("default status is planned", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		body := CreateSprintRequest{
			Name: "Sprint Beta",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/projects/%d/sprints", projectID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleCreateSprint(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var sprint Sprint
		DecodeJSON(t, rec, &sprint)

		if sprint.Status != "planned" {
			t.Errorf("Expected default status 'planned', got '%s'", sprint.Status)
		}
	})

	t.Run("missing name", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		body := CreateSprintRequest{
			Name: "",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/projects/%d/sprints", projectID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleCreateSprint(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)

		respBody := strings.TrimSpace(rec.Body.String())
		if !strings.Contains(respBody, "Sprint name is required") {
			t.Errorf("Expected error about missing name, got: %s", respBody)
		}
	})

	t.Run("invalid status", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		body := CreateSprintRequest{
			Name:   "Sprint Gamma",
			Status: "invalid_status",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/projects/%d/sprints", projectID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleCreateSprint(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)

		respBody := strings.TrimSpace(rec.Body.String())
		if !strings.Contains(respBody, "Invalid status") {
			t.Errorf("Expected error about invalid status, got: %s", respBody)
		}
	})
}

func TestHandleUpdateSprint(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		sprintID := createTestSprint(t, ts, userID, projectID, "Original Sprint", "planned")

		body := UpdateSprintRequest{
			Name:   stringPtr("Updated Sprint"),
			Status: stringPtr("active"),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch, fmt.Sprintf("/api/sprints/%d", sprintID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", sprintID)})
		ts.HandleUpdateSprint(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var sprint Sprint
		DecodeJSON(t, rec, &sprint)

		if sprint.Name != "Updated Sprint" {
			t.Errorf("Expected name 'Updated Sprint', got '%s'", sprint.Name)
		}
		if sprint.Status != "active" {
			t.Errorf("Expected status 'active', got '%s'", sprint.Status)
		}
	})

	t.Run("not found", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		body := UpdateSprintRequest{
			Name: stringPtr("Updated"),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch, "/api/sprints/99999", body, userID,
			map[string]string{"id": "99999"})
		ts.HandleUpdateSprint(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusNotFound)
	})
}

func TestHandleDeleteSprint(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		sprintID := createTestSprint(t, ts, userID, projectID, "Sprint to Delete", "planned")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete, fmt.Sprintf("/api/sprints/%d", sprintID), nil, userID,
			map[string]string{"id": fmt.Sprintf("%d", sprintID)})
		ts.HandleDeleteSprint(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		// Verify sprint was deleted
		var count int
		err := ts.DB.QueryRow("SELECT COUNT(*) FROM sprints WHERE id = ?", sprintID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query sprint count: %v", err)
		}
		if count != 0 {
			t.Errorf("Sprint was not deleted from database")
		}
	})

	t.Run("not found", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/sprints/99999", nil, userID,
			map[string]string{"id": "99999"})
		ts.HandleDeleteSprint(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusNotFound)
	})
}

// --- Tag Tests ---

func TestHandleListTags(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/projects/%d/tags", projectID), nil, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleListTags(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var tags []json.RawMessage
		DecodeJSON(t, rec, &tags)

		if len(tags) != 0 {
			t.Errorf("Expected 0 tags, got %d", len(tags))
		}
	})

	t.Run("with tags", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		createTestTag(t, ts, userID, projectID, "bug", "#FF0000")
		createTestTag(t, ts, userID, projectID, "feature", "#00FF00")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/projects/%d/tags", projectID), nil, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleListTags(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var tags []Tag
		DecodeJSON(t, rec, &tags)

		if len(tags) != 2 {
			t.Fatalf("Expected 2 tags, got %d", len(tags))
		}

		// Tags should be ordered by name ASC
		if tags[0].Name != "bug" {
			t.Errorf("Expected first tag name 'bug', got '%s'", tags[0].Name)
		}
		if tags[1].Name != "feature" {
			t.Errorf("Expected second tag name 'feature', got '%s'", tags[1].Name)
		}
	})

	t.Run("isolated between projects", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectA := ts.CreateTestProject(t, userID, "Project A")
		projectB := ts.CreateTestProject(t, userID, "Project B")

		createTestTag(t, ts, userID, projectA, "tag-a", "#FF0000")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, fmt.Sprintf("/api/projects/%d/tags", projectB), nil, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectB)})
		ts.HandleListTags(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var tags []Tag
		DecodeJSON(t, rec, &tags)

		if len(tags) != 0 {
			t.Errorf("Expected 0 tags in project B, got %d", len(tags))
		}
	})
}

func TestHandleCreateTag(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		body := CreateTagRequest{
			Name:  "urgent",
			Color: "#FF0000",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/projects/%d/tags", projectID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleCreateTag(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var tag Tag
		DecodeJSON(t, rec, &tag)

		if tag.Name != "urgent" {
			t.Errorf("Expected name 'urgent', got '%s'", tag.Name)
		}
		if tag.Color != "#FF0000" {
			t.Errorf("Expected color '#FF0000', got '%s'", tag.Color)
		}
	})

	t.Run("default color", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		body := CreateTagRequest{
			Name: "enhancement",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/projects/%d/tags", projectID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleCreateTag(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var tag Tag
		DecodeJSON(t, rec, &tag)

		if tag.Color != "#3B82F6" {
			t.Errorf("Expected default color '#3B82F6', got '%s'", tag.Color)
		}
	})

	t.Run("duplicate name in same project fails", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		body := CreateTagRequest{
			Name:  "duplicate",
			Color: "#FF0000",
		}

		// First create should succeed
		rec1, req1 := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/projects/%d/tags", projectID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleCreateTag(rec1, req1)
		AssertStatusCode(t, rec1.Code, http.StatusCreated)

		// Second create with same name in same project should fail
		rec2, req2 := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/projects/%d/tags", projectID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleCreateTag(rec2, req2)
		AssertStatusCode(t, rec2.Code, http.StatusConflict)

		respBody := strings.TrimSpace(rec2.Body.String())
		if !strings.Contains(respBody, "unique") && !strings.Contains(respBody, "Tag name must be unique") {
			t.Errorf("Expected error about unique constraint, got: %s", respBody)
		}
	})
}

func TestHandleUpdateTag(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		tagID := createTestTag(t, ts, userID, projectID, "old-tag", "#000000")

		body := UpdateTagRequest{
			Name:  stringPtr("new-tag"),
			Color: stringPtr("#FFFFFF"),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch, fmt.Sprintf("/api/tags/%d", tagID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", tagID)})
		ts.HandleUpdateTag(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var tag Tag
		DecodeJSON(t, rec, &tag)

		if tag.Name != "new-tag" {
			t.Errorf("Expected name 'new-tag', got '%s'", tag.Name)
		}
		if tag.Color != "#FFFFFF" {
			t.Errorf("Expected color '#FFFFFF', got '%s'", tag.Color)
		}
	})

	t.Run("not found", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		body := UpdateTagRequest{
			Name: stringPtr("updated"),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch, "/api/tags/99999", body, userID,
			map[string]string{"id": "99999"})
		ts.HandleUpdateTag(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusNotFound)
	})
}

func TestHandleDeleteTag(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")
		tagID := createTestTag(t, ts, userID, projectID, "to-delete", "#FF0000")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete, fmt.Sprintf("/api/tags/%d", tagID), nil, userID,
			map[string]string{"id": fmt.Sprintf("%d", tagID)})
		ts.HandleDeleteTag(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		// Verify tag was deleted
		var count int
		err := ts.DB.QueryRow("SELECT COUNT(*) FROM tags WHERE id = ?", tagID).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query tag count: %v", err)
		}
		if count != 0 {
			t.Errorf("Tag was not deleted from database")
		}
	})

	t.Run("not found", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/tags/99999", nil, userID,
			map[string]string{"id": "99999"})
		ts.HandleDeleteTag(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusNotFound)
	})
}

// --- Additional coverage tests ---

func TestHandleCreateSprint_InvalidBody(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/projects/%d/sprints", projectID), "not-json", userID,
		map[string]string{"id": fmt.Sprintf("%d", projectID)})
	ts.HandleCreateSprint(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusBadRequest)
}

func TestHandleUpdateSprint_InvalidID(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")

	body := UpdateSprintRequest{Name: stringPtr("Updated")}
	rec, req := ts.MakeAuthRequest(t, http.MethodPatch, "/api/sprints/abc", body, userID,
		map[string]string{"id": "abc"})
	ts.HandleUpdateSprint(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusBadRequest)
}

func TestHandleDeleteSprint_InvalidID(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/sprints/abc", nil, userID,
		map[string]string{"id": "abc"})
	ts.HandleDeleteSprint(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusBadRequest)
}

func TestHandleUpdateTag_InvalidID(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")

	body := UpdateTagRequest{Name: stringPtr("Updated")}
	rec, req := ts.MakeAuthRequest(t, http.MethodPatch, "/api/tags/abc", body, userID,
		map[string]string{"id": "abc"})
	ts.HandleUpdateTag(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusBadRequest)
}

func TestHandleDeleteTag_InvalidID(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/tags/abc", nil, userID,
		map[string]string{"id": "abc"})
	ts.HandleDeleteTag(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusBadRequest)
}
