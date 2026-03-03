package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// createTestTeamForUser creates a team and adds the user as an active owner member.
// This is needed because HandleCreateProject calls getUserTeamID which queries team_members.
func createTestTeamForUser(t *testing.T, ts *TestServer, userID int64) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO teams (name, owner_id, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"Test Team", userID,
	)
	if err != nil {
		t.Fatalf("Failed to create test team: %v", err)
	}

	teamID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get team ID: %v", err)
	}

	_, err = ts.DB.ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id, role, status, joined_at) VALUES (?, ?, 'owner', 'active', CURRENT_TIMESTAMP)`,
		teamID, userID,
	)
	if err != nil {
		t.Fatalf("Failed to add user to team: %v", err)
	}

	return teamID
}

func TestHandleListProjects(t *testing.T) {
	t.Run("empty list when user has no projects", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/projects", nil, userID, nil)
		ts.HandleListProjects(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var projects []Project
		DecodeJSON(t, rec, &projects)

		if len(projects) != 0 {
			t.Errorf("Expected 0 projects, got %d", len(projects))
		}
	})

	t.Run("returns projects user is a member of", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		_ = ts.CreateTestProject(t, userID, "Project Alpha")
		_ = ts.CreateTestProject(t, userID, "Project Beta")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/projects", nil, userID, nil)
		ts.HandleListProjects(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var projects []Project
		DecodeJSON(t, rec, &projects)

		if len(projects) != 2 {
			t.Errorf("Expected 2 projects, got %d", len(projects))
		}
	})

	t.Run("does not return projects user is not a member of", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		user1 := ts.CreateTestUser(t, "user1@example.com", "password123")
		user2 := ts.CreateTestUser(t, "user2@example.com", "password123")

		_ = ts.CreateTestProject(t, user1, "User1 Project")
		_ = ts.CreateTestProject(t, user2, "User2 Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/projects", nil, user1, nil)
		ts.HandleListProjects(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var projects []Project
		DecodeJSON(t, rec, &projects)

		if len(projects) != 1 {
			t.Errorf("Expected 1 project, got %d", len(projects))
		}
		if len(projects) > 0 && projects[0].Name != "User1 Project" {
			t.Errorf("Expected project name 'User1 Project', got %q", projects[0].Name)
		}
	})
}

func TestHandleCreateProject(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		createTestTeamForUser(t, ts, userID)

		body := CreateProjectRequest{
			Name:        "New Project",
			Description: stringPtr("A test project description"),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/projects", body, userID, nil)
		ts.HandleCreateProject(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var project Project
		DecodeJSON(t, rec, &project)

		if project.Name != "New Project" {
			t.Errorf("Expected project name 'New Project', got %q", project.Name)
		}
		if project.Description == nil || *project.Description != "A test project description" {
			t.Errorf("Expected description 'A test project description', got %v", project.Description)
		}
		if project.OwnerID != userID {
			t.Errorf("Expected owner_id %d, got %d", userID, project.OwnerID)
		}
	})

	t.Run("creates default swim lanes", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		createTestTeamForUser(t, ts, userID)

		body := CreateProjectRequest{Name: "Swim Lane Project"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/projects", body, userID, nil)
		ts.HandleCreateProject(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var project Project
		DecodeJSON(t, rec, &project)

		// Verify swim lanes were created
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var count int
		err := ts.DB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM swim_lanes WHERE project_id = ?`, project.ID,
		).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to count swim lanes: %v", err)
		}
		if count != 3 {
			t.Errorf("Expected 3 default swim lanes, got %d", count)
		}
	})

	t.Run("adds creator as owner member", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		createTestTeamForUser(t, ts, userID)

		body := CreateProjectRequest{Name: "Member Test Project"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/projects", body, userID, nil)
		ts.HandleCreateProject(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var project Project
		DecodeJSON(t, rec, &project)

		// Verify user is project member with owner role
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var role string
		err := ts.DB.QueryRowContext(ctx,
			`SELECT role FROM project_members WHERE project_id = ? AND user_id = ?`,
			project.ID, userID,
		).Scan(&role)
		if err != nil {
			t.Fatalf("Failed to query project member: %v", err)
		}
		if role != "owner" {
			t.Errorf("Expected creator role 'owner', got %q", role)
		}
	})

	t.Run("missing name returns validation error", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		createTestTeamForUser(t, ts, userID)

		body := CreateProjectRequest{Name: ""}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/projects", body, userID, nil)
		ts.HandleCreateProject(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "project name is required", "invalid_input")
	})

	t.Run("name too long returns validation error", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		createTestTeamForUser(t, ts, userID)

		longName := strings.Repeat("a", 256)
		body := CreateProjectRequest{Name: longName}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/projects", body, userID, nil)
		ts.HandleCreateProject(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "project name is too long", "invalid_input")
	})

	t.Run("without description creates project with nil description", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		createTestTeamForUser(t, ts, userID)

		body := CreateProjectRequest{Name: "No Desc Project"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/projects", body, userID, nil)
		ts.HandleCreateProject(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var project Project
		DecodeJSON(t, rec, &project)

		if project.Name != "No Desc Project" {
			t.Errorf("Expected project name 'No Desc Project', got %q", project.Name)
		}
		if project.Description != nil {
			t.Errorf("Expected nil description, got %v", *project.Description)
		}
	})
}

func TestHandleGetProject(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d", projectID), nil, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleGetProject(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var project Project
		DecodeJSON(t, rec, &project)

		if project.ID != projectID {
			t.Errorf("Expected project ID %d, got %d", projectID, project.ID)
		}
		if project.Name != "Test Project" {
			t.Errorf("Expected project name 'Test Project', got %q", project.Name)
		}
		if project.OwnerID != userID {
			t.Errorf("Expected owner_id %d, got %d", userID, project.OwnerID)
		}
	})

	t.Run("not found returns 404", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/projects/9999", nil, userID,
			map[string]string{"id": "9999"})

		ts.HandleGetProject(rec, req)

		AssertError(t, rec, http.StatusNotFound, "project not found", "not_found")
	})

	t.Run("invalid ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/projects/abc", nil, userID,
			map[string]string{"id": "abc"})

		ts.HandleGetProject(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid project ID", "invalid_input")
	})

	t.Run("user cannot access project they are not a member of", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		other := ts.CreateTestUser(t, "other@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Owner Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d", projectID), nil, other,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleGetProject(rec, req)

		AssertError(t, rec, http.StatusNotFound, "project not found", "not_found")
	})
}

func TestHandleUpdateProject(t *testing.T) {
	t.Run("update name", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Original Name")

		body := UpdateProjectRequest{
			Name: stringPtr("Updated Name"),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d", projectID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleUpdateProject(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var project Project
		DecodeJSON(t, rec, &project)

		if project.Name != "Updated Name" {
			t.Errorf("Expected project name 'Updated Name', got %q", project.Name)
		}
	})

	t.Run("update description", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		body := UpdateProjectRequest{
			Description: stringPtr("New description"),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d", projectID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleUpdateProject(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var project Project
		DecodeJSON(t, rec, &project)

		if project.Description == nil || *project.Description != "New description" {
			t.Errorf("Expected description 'New description', got %v", project.Description)
		}
	})

	t.Run("update both name and description", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		body := UpdateProjectRequest{
			Name:        stringPtr("New Name"),
			Description: stringPtr("New description"),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d", projectID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleUpdateProject(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var project Project
		DecodeJSON(t, rec, &project)

		if project.Name != "New Name" {
			t.Errorf("Expected project name 'New Name', got %q", project.Name)
		}
		if project.Description == nil || *project.Description != "New description" {
			t.Errorf("Expected description 'New description', got %v", project.Description)
		}
	})

	t.Run("empty name returns validation error", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		body := UpdateProjectRequest{
			Name: stringPtr(""),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d", projectID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleUpdateProject(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "project name cannot be empty", "invalid_input")
	})

	t.Run("name too long returns validation error", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Test Project")

		longName := strings.Repeat("x", 256)
		body := UpdateProjectRequest{
			Name: stringPtr(longName),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d", projectID), body, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleUpdateProject(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "project name is too long", "invalid_input")
	})

	t.Run("not found returns 404", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		body := UpdateProjectRequest{
			Name: stringPtr("Updated"),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch, "/api/projects/9999", body, userID,
			map[string]string{"id": "9999"})

		ts.HandleUpdateProject(rec, req)

		AssertError(t, rec, http.StatusNotFound, "project not found", "not_found")
	})

	t.Run("invalid ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		body := UpdateProjectRequest{
			Name: stringPtr("Updated"),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch, "/api/projects/abc", body, userID,
			map[string]string{"id": "abc"})

		ts.HandleUpdateProject(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid project ID", "invalid_input")
	})

	t.Run("non-member cannot update project", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		other := ts.CreateTestUser(t, "other@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Owner Project")

		body := UpdateProjectRequest{
			Name: stringPtr("Hijacked"),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d", projectID), body, other,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleUpdateProject(rec, req)

		AssertError(t, rec, http.StatusNotFound, "project not found", "not_found")
	})

	t.Run("member with viewer role cannot update project", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		viewer := ts.CreateTestUser(t, "viewer@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Restricted Project")

		ts.AddProjectMember(t, projectID, viewer, owner, "viewer")

		body := UpdateProjectRequest{
			Name: stringPtr("Viewer Edit"),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d", projectID), body, viewer,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleUpdateProject(rec, req)

		AssertError(t, rec, http.StatusForbidden, "only project owners and editors can update", "forbidden")
	})

	t.Run("editor can update project", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		editor := ts.CreateTestUser(t, "editor@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Collaborative Project")

		ts.AddProjectMember(t, projectID, editor, owner, "editor")

		body := UpdateProjectRequest{
			Name: stringPtr("Editor Update"),
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d", projectID), body, editor,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleUpdateProject(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var project Project
		DecodeJSON(t, rec, &project)

		if project.Name != "Editor Update" {
			t.Errorf("Expected project name 'Editor Update', got %q", project.Name)
		}
	})
}

func TestHandleDeleteProject(t *testing.T) {
	t.Run("happy path - owner can delete", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")
		projectID := ts.CreateTestProject(t, userID, "Delete Me")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			fmt.Sprintf("/api/projects/%d", projectID), nil, userID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleDeleteProject(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusNoContent)

		// Verify project is actually deleted
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var count int
		err := ts.DB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM projects WHERE id = ?`, projectID,
		).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to verify deletion: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected project to be deleted, but found %d rows", count)
		}
	})

	t.Run("not found returns 404", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/projects/9999", nil, userID,
			map[string]string{"id": "9999"})

		ts.HandleDeleteProject(rec, req)

		AssertError(t, rec, http.StatusNotFound, "project not found", "not_found")
	})

	t.Run("invalid ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "test@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/projects/abc", nil, userID,
			map[string]string{"id": "abc"})

		ts.HandleDeleteProject(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid project ID", "invalid_input")
	})

	t.Run("non-owner cannot delete project", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		member := ts.CreateTestUser(t, "member@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Protected Project")

		ts.AddProjectMember(t, projectID, member, owner, "member")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			fmt.Sprintf("/api/projects/%d", projectID), nil, member,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleDeleteProject(rec, req)

		AssertError(t, rec, http.StatusForbidden, "only project owner can delete", "forbidden")

		// Verify project still exists
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var count int
		err := ts.DB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM projects WHERE id = ?`, projectID,
		).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to verify project exists: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected project to still exist, but found %d rows", count)
		}
	})

	t.Run("editor cannot delete project", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		owner := ts.CreateTestUser(t, "owner@example.com", "password123")
		editor := ts.CreateTestUser(t, "editor@example.com", "password123")
		projectID := ts.CreateTestProject(t, owner, "Editor Restricted")

		ts.AddProjectMember(t, projectID, editor, owner, "editor")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			fmt.Sprintf("/api/projects/%d", projectID), nil, editor,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleDeleteProject(rec, req)

		AssertError(t, rec, http.StatusForbidden, "only project owner can delete", "forbidden")
	})
}

func TestHandleCreateProject_InvalidBody(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	createTestTeamForUser(t, ts, userID)

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/projects", "not-json", userID, nil)
	ts.HandleCreateProject(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "invalid request body", "invalid_input")
}

func TestHandleCreateProject_NoTeam(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create user WITHOUT a team
	userID := ts.CreateTestUser(t, "test@example.com", "password123")

	body := CreateProjectRequest{Name: "Orphan Project"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/projects", body, userID, nil)
	ts.HandleCreateProject(rec, req)

	AssertError(t, rec, http.StatusInternalServerError, "failed to get user team", "internal_error")
}

func TestHandleCreateProject_DoesNotAddTeamMembers(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	memberID := ts.CreateTestUser(t, "member@example.com", "password123")
	teamID := createTestTeamForUser(t, ts, ownerID)

	// Add second user to the team
	ctx := context.Background()
	_, err := ts.DB.ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id, role, status, joined_at) VALUES (?, ?, 'member', 'active', CURRENT_TIMESTAMP)`,
		teamID, memberID,
	)
	if err != nil {
		t.Fatalf("Failed to add team member: %v", err)
	}

	body := CreateProjectRequest{Name: "Team Project"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/projects", body, ownerID, nil)
	ts.HandleCreateProject(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var project Project
	DecodeJSON(t, rec, &project)

	// Verify team member was NOT auto-added to the project (explicit access only)
	var count int
	err = ts.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM project_members WHERE project_id = ? AND user_id = ?`,
		project.ID, memberID,
	).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to check membership: %v", err)
	}
	if count != 0 {
		t.Errorf("Expected team member NOT to be auto-added to project, but got count %d", count)
	}

	// Verify only the creator is a member
	var totalCount int
	err = ts.DB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM project_members WHERE project_id = ?`,
		project.ID,
	).Scan(&totalCount)
	if err != nil {
		t.Fatalf("Failed to check total membership: %v", err)
	}
	if totalCount != 1 {
		t.Errorf("Expected only 1 project member (the creator), got %d", totalCount)
	}
}

func TestHandleUpdateProject_InvalidBody(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "test@example.com", "password123")
	projectID := ts.CreateTestProject(t, userID, "Test Project")

	rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
		fmt.Sprintf("/api/projects/%d", projectID), "not-json", userID,
		map[string]string{"id": fmt.Sprintf("%d", projectID)})

	ts.HandleUpdateProject(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "invalid request body", "invalid_input")
}
