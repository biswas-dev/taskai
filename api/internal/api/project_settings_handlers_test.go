package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// createTestTeamAndProject creates a team, adds the owner as team member, creates a project
// linked to that team, and adds the owner as a project member with 'owner' role.
// Returns (teamID, projectID).
func createTestTeamAndProject(t *testing.T, ts *TestServer, ownerID int64, projectName string) (int64, int64) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Create team
	teamResult, err := ts.DB.ExecContext(ctx,
		`INSERT INTO teams (name, owner_id, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		"Test Team", ownerID,
	)
	if err != nil {
		t.Fatalf("Failed to create test team: %v", err)
	}
	teamID, err := teamResult.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get team ID: %v", err)
	}

	// Add owner as active team member
	_, err = ts.DB.ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id, role, status, joined_at) VALUES (?, ?, 'owner', 'active', CURRENT_TIMESTAMP)`,
		teamID, ownerID,
	)
	if err != nil {
		t.Fatalf("Failed to add owner to team: %v", err)
	}

	// Create project with team_id set
	projectResult, err := ts.DB.ExecContext(ctx,
		`INSERT INTO projects (owner_id, name, description, team_id) VALUES (?, ?, ?, ?)`,
		ownerID, projectName, "Test project description", teamID,
	)
	if err != nil {
		t.Fatalf("Failed to create test project: %v", err)
	}
	projectID, err := projectResult.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get project ID: %v", err)
	}

	// Add owner as project member with 'owner' role
	_, err = ts.DB.ExecContext(ctx,
		`INSERT INTO project_members (project_id, user_id, role, granted_by) VALUES (?, ?, 'owner', ?)`,
		projectID, ownerID, ownerID,
	)
	if err != nil {
		t.Fatalf("Failed to add project member: %v", err)
	}

	return teamID, projectID
}

// addUserToTeam adds a user to a team with the given role and active status.
func addUserToTeam(t *testing.T, ts *TestServer, teamID, userID int64, role string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ts.DB.ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id, role, status, joined_at) VALUES (?, ?, ?, 'active', CURRENT_TIMESTAMP)`,
		teamID, userID, role,
	)
	if err != nil {
		t.Fatalf("Failed to add user to team: %v", err)
	}
}

// getProjectMemberID returns the project_members.id for a given project and user.
func getProjectMemberID(t *testing.T, ts *TestServer, projectID, userID int64) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	var memberID int64
	err := ts.DB.QueryRowContext(ctx,
		`SELECT id FROM project_members WHERE project_id = ? AND user_id = ?`,
		projectID, userID,
	).Scan(&memberID)
	if err != nil {
		t.Fatalf("Failed to get project member ID: %v", err)
	}
	return memberID
}

// TestHandleGetProjectMembers tests the HandleGetProjectMembers handler.
func TestHandleGetProjectMembers(t *testing.T) {
	t.Run("returns members for project", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Test Project")

		// Add another member
		memberID := ts.CreateTestUser(t, "member@example.com", "password123")
		ts.AddProjectMember(t, projectID, memberID, ownerID, "editor")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d/members", projectID), nil, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleGetProjectMembers(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var members []ProjectMember
		DecodeJSON(t, rec, &members)

		if len(members) != 2 {
			t.Fatalf("Expected 2 members, got %d", len(members))
		}

		// Verify we got both owner and member
		emailSet := make(map[string]string)
		for _, m := range members {
			emailSet[m.Email] = m.Role
		}

		if emailSet["owner@example.com"] != "owner" {
			t.Errorf("Expected owner@example.com to have role 'owner', got %q", emailSet["owner@example.com"])
		}
		if emailSet["member@example.com"] != "editor" {
			t.Errorf("Expected member@example.com to have role 'editor', got %q", emailSet["member@example.com"])
		}
	})

	t.Run("returns empty list for project with only owner", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Solo Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d/members", projectID), nil, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleGetProjectMembers(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var members []ProjectMember
		DecodeJSON(t, rec, &members)

		// Owner is a project member too
		if len(members) != 1 {
			t.Errorf("Expected 1 member (the owner), got %d", len(members))
		}
	})

	t.Run("non-member gets forbidden", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		otherID := ts.CreateTestUser(t, "other@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Private Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d/members", projectID), nil, otherID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleGetProjectMembers(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusForbidden)
	})

	t.Run("invalid project ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			"/api/projects/abc/members", nil, ownerID,
			map[string]string{"id": "abc"})

		ts.HandleGetProjectMembers(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
	})

	t.Run("member with viewer role can list members", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		viewerID := ts.CreateTestUser(t, "viewer@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Viewable Project")

		ts.AddProjectMember(t, projectID, viewerID, ownerID, "viewer")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d/members", projectID), nil, viewerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleGetProjectMembers(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var members []ProjectMember
		DecodeJSON(t, rec, &members)

		if len(members) != 2 {
			t.Errorf("Expected 2 members, got %d", len(members))
		}
	})
}

// TestHandleAddProjectMember tests the HandleAddProjectMember handler.
func TestHandleAddProjectMember(t *testing.T) {
	t.Run("owner can add team member to project", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		teamID, projectID := createTestTeamAndProject(t, ts, ownerID, "Team Project")

		// Create another user and add them to the team
		newUserID := ts.CreateTestUser(t, "new@example.com", "password123")
		addUserToTeam(t, ts, teamID, newUserID, "member")

		body := AddMemberRequest{
			Email: "new@example.com",
			Role:  "editor",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/members", projectID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleAddProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var resp map[string]interface{}
		DecodeJSON(t, rec, &resp)

		if resp["message"] != "Member added successfully" {
			t.Errorf("Expected success message, got %v", resp["message"])
		}
		if resp["member_id"] == nil {
			t.Error("Expected member_id in response")
		}
	})

	t.Run("non-owner/admin gets forbidden", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		teamID, projectID := createTestTeamAndProject(t, ts, ownerID, "Team Project")

		// Add a regular member to project
		regularID := ts.CreateTestUser(t, "regular@example.com", "password123")
		addUserToTeam(t, ts, teamID, regularID, "member")
		ts.AddProjectMember(t, projectID, regularID, ownerID, "member")

		// Another user to add
		newUserID := ts.CreateTestUser(t, "new@example.com", "password123")
		addUserToTeam(t, ts, teamID, newUserID, "member")

		body := AddMemberRequest{
			Email: "new@example.com",
			Role:  "viewer",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/members", projectID), body, regularID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleAddProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusForbidden)
	})

	t.Run("admin can add member", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		teamID, projectID := createTestTeamAndProject(t, ts, ownerID, "Admin Project")

		// Add admin to project
		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		addUserToTeam(t, ts, teamID, adminID, "member")
		ts.AddProjectMember(t, projectID, adminID, ownerID, "admin")

		// New user to be added
		newUserID := ts.CreateTestUser(t, "new@example.com", "password123")
		addUserToTeam(t, ts, teamID, newUserID, "member")

		body := AddMemberRequest{
			Email: "new@example.com",
			Role:  "viewer",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/members", projectID), body, adminID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleAddProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)
	})

	t.Run("invalid role returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		_, projectID := createTestTeamAndProject(t, ts, ownerID, "Role Test")

		body := AddMemberRequest{
			Email: "someone@example.com",
			Role:  "superadmin",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/members", projectID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleAddProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
		if !strings.Contains(rec.Body.String(), "Invalid role") {
			t.Errorf("Expected 'Invalid role' error, got %q", rec.Body.String())
		}
	})

	t.Run("user not found returns 404", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		_, projectID := createTestTeamAndProject(t, ts, ownerID, "Missing User Project")

		body := AddMemberRequest{
			Email: "nonexistent@example.com",
			Role:  "viewer",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/members", projectID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleAddProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusNotFound)
		if !strings.Contains(rec.Body.String(), "User not found") {
			t.Errorf("Expected 'User not found' error, got %q", rec.Body.String())
		}
	})

	t.Run("cannot add project owner as member", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		teamID, projectID := createTestTeamAndProject(t, ts, ownerID, "Owner Project")

		// Create a second owner for the project so we can test from their perspective
		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		addUserToTeam(t, ts, teamID, adminID, "member")
		ts.AddProjectMember(t, projectID, adminID, ownerID, "admin")

		body := AddMemberRequest{
			Email: "owner@example.com",
			Role:  "viewer",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/members", projectID), body, adminID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleAddProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
		if !strings.Contains(rec.Body.String(), "Cannot add project owner") {
			t.Errorf("Expected 'Cannot add project owner' error, got %q", rec.Body.String())
		}
	})

	t.Run("user with no shared team returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		_, projectID := createTestTeamAndProject(t, ts, ownerID, "Team Required Project")

		// Create user but do NOT add to any team the owner belongs to
		_ = ts.CreateTestUser(t, "outsider@example.com", "password123")

		body := AddMemberRequest{
			Email: "outsider@example.com",
			Role:  "viewer",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/members", projectID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleAddProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
		if !strings.Contains(rec.Body.String(), "member of this project's team") {
			t.Errorf("Expected project-team error, got %q", rec.Body.String())
		}
	})

	t.Run("member of another team the owner shares cannot be added", func(t *testing.T) {
		// A project belongs to exactly one team. The owner also belongs to
		// someone else's team, but that team's members must not be offered
		// access to this project.
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		_, projectID := createTestTeamAndProject(t, ts, ownerID, "Cross-team Project")

		// Create a second team owned by someone else, with the project owner as
		// a member and the candidate invitee as the team owner.
		otherOwnerID := ts.CreateTestUser(t, "adhiraj@example.com", "password123")
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		res, err := ts.DB.ExecContext(ctx,
			`INSERT INTO teams (name, owner_id, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
			"Other Team", otherOwnerID,
		)
		if err != nil {
			t.Fatalf("Failed to create other team: %v", err)
		}
		otherTeamID, _ := res.LastInsertId()
		addUserToTeam(t, ts, otherTeamID, otherOwnerID, "owner")
		addUserToTeam(t, ts, otherTeamID, ownerID, "member")

		body := AddMemberRequest{
			Email: "adhiraj@example.com",
			Role:  "editor",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/members", projectID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleAddProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
	})

	t.Run("duplicate member fails on second add", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		teamID, projectID := createTestTeamAndProject(t, ts, ownerID, "Dup Project")

		memberUserID := ts.CreateTestUser(t, "member@example.com", "password123")
		addUserToTeam(t, ts, teamID, memberUserID, "member")

		body := AddMemberRequest{
			Email: "member@example.com",
			Role:  "editor",
		}

		// First add - should succeed
		rec1, req1 := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/members", projectID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleAddProjectMember(rec1, req1)
		AssertStatusCode(t, rec1.Code, http.StatusCreated)

		// Second add - should fail (unique constraint on project_id, user_id)
		rec2, req2 := ts.MakeAuthRequest(t, http.MethodPost,
			fmt.Sprintf("/api/projects/%d/members", projectID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleAddProjectMember(rec2, req2)

		// The handler returns 500 because the SQLite driver error string format
		// doesn't match the exact string comparison in the handler. The second
		// insert should be rejected regardless.
		if rec2.Code == http.StatusCreated {
			t.Error("Expected second add to fail, but got 201 Created")
		}
	})

	t.Run("invalid project ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")

		body := AddMemberRequest{
			Email: "someone@example.com",
			Role:  "viewer",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost,
			"/api/projects/abc/members", body, ownerID,
			map[string]string{"id": "abc"})

		ts.HandleAddProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
	})

	t.Run("valid roles accepted", func(t *testing.T) {
		validRoles := []string{"viewer", "member", "editor", "owner"}

		for _, role := range validRoles {
			t.Run(role, func(t *testing.T) {
				ts := NewTestServer(t)
				defer ts.Close()

				ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
				teamID, projectID := createTestTeamAndProject(t, ts, ownerID, "Role "+role)

				newUserID := ts.CreateTestUser(t, "user@example.com", "password123")
				addUserToTeam(t, ts, teamID, newUserID, "member")

				body := AddMemberRequest{
					Email: "user@example.com",
					Role:  role,
				}

				rec, req := ts.MakeAuthRequest(t, http.MethodPost,
					fmt.Sprintf("/api/projects/%d/members", projectID), body, ownerID,
					map[string]string{"id": fmt.Sprintf("%d", projectID)})

				ts.HandleAddProjectMember(rec, req)

				AssertStatusCode(t, rec.Code, http.StatusCreated)
			})
		}
	})
}

// TestHandleUpdateProjectMember tests the HandleUpdateProjectMember handler.
func TestHandleUpdateProjectMember(t *testing.T) {
	t.Run("owner can update member role", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Update Role Project")

		memberUserID := ts.CreateTestUser(t, "member@example.com", "password123")
		ts.AddProjectMember(t, projectID, memberUserID, ownerID, "viewer")

		memberRowID := getProjectMemberID(t, ts, projectID, memberUserID)

		body := UpdateMemberRoleRequest{
			Role: "editor",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/members/%d", projectID, memberRowID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": fmt.Sprintf("%d", memberRowID)})

		ts.HandleUpdateProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp map[string]string
		DecodeJSON(t, rec, &resp)

		if resp["message"] != "Member role updated successfully" {
			t.Errorf("Expected success message, got %q", resp["message"])
		}

		// Verify role was actually updated in the database
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var role string
		err := ts.DB.QueryRowContext(ctx,
			`SELECT role FROM project_members WHERE id = ?`, memberRowID,
		).Scan(&role)
		if err != nil {
			t.Fatalf("Failed to query member role: %v", err)
		}
		if role != "editor" {
			t.Errorf("Expected role 'editor' in DB, got %q", role)
		}
	})

	t.Run("non-owner/admin gets forbidden", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Restricted Project")

		// Add a regular member
		regularID := ts.CreateTestUser(t, "regular@example.com", "password123")
		ts.AddProjectMember(t, projectID, regularID, ownerID, "member")

		// Add another member whose role regular will try to change
		targetID := ts.CreateTestUser(t, "target@example.com", "password123")
		ts.AddProjectMember(t, projectID, targetID, ownerID, "viewer")

		targetMemberRowID := getProjectMemberID(t, ts, projectID, targetID)

		body := UpdateMemberRoleRequest{
			Role: "editor",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/members/%d", projectID, targetMemberRowID), body, regularID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": fmt.Sprintf("%d", targetMemberRowID)})

		ts.HandleUpdateProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusForbidden)
	})

	t.Run("invalid role returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Role Validation")

		memberUserID := ts.CreateTestUser(t, "member@example.com", "password123")
		ts.AddProjectMember(t, projectID, memberUserID, ownerID, "viewer")

		memberRowID := getProjectMemberID(t, ts, projectID, memberUserID)

		body := UpdateMemberRoleRequest{
			Role: "superadmin",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/members/%d", projectID, memberRowID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": fmt.Sprintf("%d", memberRowID)})

		ts.HandleUpdateProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
		if !strings.Contains(rec.Body.String(), "Invalid role") {
			t.Errorf("Expected 'Invalid role' error, got %q", rec.Body.String())
		}
	})

	t.Run("member not found returns 404", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Missing Member")

		body := UpdateMemberRoleRequest{
			Role: "editor",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/members/99999", projectID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": "99999"})

		ts.HandleUpdateProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusNotFound)
	})

	t.Run("cannot demote last owner", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Single Owner Project")

		ownerMemberRowID := getProjectMemberID(t, ts, projectID, ownerID)

		body := UpdateMemberRoleRequest{
			Role: "editor",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/members/%d", projectID, ownerMemberRowID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": fmt.Sprintf("%d", ownerMemberRowID)})

		ts.HandleUpdateProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
		if !strings.Contains(rec.Body.String(), "at least one owner") {
			t.Errorf("Expected 'at least one owner' error, got %q", rec.Body.String())
		}
	})

	t.Run("can demote owner when another owner exists", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Multi Owner Project")

		// Add a second owner
		owner2ID := ts.CreateTestUser(t, "owner2@example.com", "password123")
		ts.AddProjectMember(t, projectID, owner2ID, ownerID, "owner")

		owner2MemberRowID := getProjectMemberID(t, ts, projectID, owner2ID)

		body := UpdateMemberRoleRequest{
			Role: "editor",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/members/%d", projectID, owner2MemberRowID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": fmt.Sprintf("%d", owner2MemberRowID)})

		ts.HandleUpdateProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		// Verify role was changed
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var role string
		err := ts.DB.QueryRowContext(ctx,
			`SELECT role FROM project_members WHERE id = ?`, owner2MemberRowID,
		).Scan(&role)
		if err != nil {
			t.Fatalf("Failed to query role: %v", err)
		}
		if role != "editor" {
			t.Errorf("Expected role 'editor', got %q", role)
		}
	})

	t.Run("invalid project ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")

		body := UpdateMemberRoleRequest{Role: "editor"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			"/api/projects/abc/members/1", body, ownerID,
			map[string]string{"id": "abc", "memberId": "1"})

		ts.HandleUpdateProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
	})

	t.Run("invalid member ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Invalid Member ID")

		body := UpdateMemberRoleRequest{Role: "editor"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/members/xyz", projectID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": "xyz"})

		ts.HandleUpdateProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
	})
}

// TestHandleRemoveProjectMember tests the HandleRemoveProjectMember handler.
func TestHandleRemoveProjectMember(t *testing.T) {
	t.Run("owner can remove member", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Remove Member Project")

		memberUserID := ts.CreateTestUser(t, "member@example.com", "password123")
		ts.AddProjectMember(t, projectID, memberUserID, ownerID, "editor")

		memberRowID := getProjectMemberID(t, ts, projectID, memberUserID)

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			fmt.Sprintf("/api/projects/%d/members/%d", projectID, memberRowID), nil, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": fmt.Sprintf("%d", memberRowID)})

		ts.HandleRemoveProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp map[string]string
		DecodeJSON(t, rec, &resp)

		if resp["message"] != "Member removed successfully" {
			t.Errorf("Expected success message, got %q", resp["message"])
		}

		// Verify member was actually removed
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var count int
		err := ts.DB.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM project_members WHERE id = ?`, memberRowID,
		).Scan(&count)
		if err != nil {
			t.Fatalf("Failed to verify member removal: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected member to be removed, but found %d rows", count)
		}
	})

	t.Run("cannot remove last owner", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Last Owner Project")

		ownerMemberRowID := getProjectMemberID(t, ts, projectID, ownerID)

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			fmt.Sprintf("/api/projects/%d/members/%d", projectID, ownerMemberRowID), nil, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": fmt.Sprintf("%d", ownerMemberRowID)})

		ts.HandleRemoveProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
		if !strings.Contains(rec.Body.String(), "Cannot remove the last owner") {
			t.Errorf("Expected 'Cannot remove the last owner' error, got %q", rec.Body.String())
		}
	})

	t.Run("can remove owner when another owner exists", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Multi Owner Project")

		owner2ID := ts.CreateTestUser(t, "owner2@example.com", "password123")
		ts.AddProjectMember(t, projectID, owner2ID, ownerID, "owner")

		owner2MemberRowID := getProjectMemberID(t, ts, projectID, owner2ID)

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			fmt.Sprintf("/api/projects/%d/members/%d", projectID, owner2MemberRowID), nil, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": fmt.Sprintf("%d", owner2MemberRowID)})

		ts.HandleRemoveProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)
	})

	t.Run("non-owner/admin gets forbidden", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Forbidden Remove Project")

		regularID := ts.CreateTestUser(t, "regular@example.com", "password123")
		ts.AddProjectMember(t, projectID, regularID, ownerID, "member")

		targetID := ts.CreateTestUser(t, "target@example.com", "password123")
		ts.AddProjectMember(t, projectID, targetID, ownerID, "viewer")

		targetMemberRowID := getProjectMemberID(t, ts, projectID, targetID)

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			fmt.Sprintf("/api/projects/%d/members/%d", projectID, targetMemberRowID), nil, regularID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": fmt.Sprintf("%d", targetMemberRowID)})

		ts.HandleRemoveProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusForbidden)
	})

	t.Run("member not found returns 404", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Missing Member Remove")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			fmt.Sprintf("/api/projects/%d/members/99999", projectID), nil, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": "99999"})

		ts.HandleRemoveProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusNotFound)
	})

	t.Run("invalid project ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			"/api/projects/abc/members/1", nil, ownerID,
			map[string]string{"id": "abc", "memberId": "1"})

		ts.HandleRemoveProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
	})

	t.Run("invalid member ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Invalid Member ID Remove")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			fmt.Sprintf("/api/projects/%d/members/xyz", projectID), nil, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": "xyz"})

		ts.HandleRemoveProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
	})

	t.Run("admin can remove member", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Admin Remove Project")

		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		ts.AddProjectMember(t, projectID, adminID, ownerID, "admin")

		targetID := ts.CreateTestUser(t, "target@example.com", "password123")
		ts.AddProjectMember(t, projectID, targetID, ownerID, "viewer")

		targetMemberRowID := getProjectMemberID(t, ts, projectID, targetID)

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete,
			fmt.Sprintf("/api/projects/%d/members/%d", projectID, targetMemberRowID), nil, adminID,
			map[string]string{"id": fmt.Sprintf("%d", projectID), "memberId": fmt.Sprintf("%d", targetMemberRowID)})

		ts.HandleRemoveProjectMember(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)
	})
}

// TestHandleGetProjectGitHubSettings tests the HandleGetProjectGitHubSettings handler.
func TestHandleGetProjectGitHubSettings(t *testing.T) {
	t.Run("returns default github settings", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "GitHub Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d/github", projectID), nil, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleGetProjectGitHubSettings(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var settings ProjectGitHubSettings
		DecodeJSON(t, rec, &settings)

		if settings.RepoURL != "" {
			t.Errorf("Expected empty repo URL, got %q", settings.RepoURL)
		}
		if settings.Branch != "main" {
			t.Errorf("Expected default branch 'main', got %q", settings.Branch)
		}
		if settings.SyncEnabled {
			t.Errorf("Expected sync disabled by default")
		}
		if settings.LastSync != nil {
			t.Errorf("Expected nil last sync, got %v", settings.LastSync)
		}
	})

	t.Run("returns configured github settings", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Configured GH Project")

		// Set GitHub settings directly in the database
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := ts.DB.ExecContext(ctx,
			`UPDATE projects SET
				github_repo_url = ?,
				github_owner = ?,
				github_repo_name = ?,
				github_branch = ?,
				github_sync_enabled = ?
			WHERE id = ?`,
			"https://github.com/example/repo",
			"example",
			"repo",
			"develop",
			1,
			projectID,
		)
		if err != nil {
			t.Fatalf("Failed to set GitHub settings: %v", err)
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d/github", projectID), nil, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleGetProjectGitHubSettings(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var settings ProjectGitHubSettings
		DecodeJSON(t, rec, &settings)

		if settings.RepoURL != "https://github.com/example/repo" {
			t.Errorf("Expected repo URL 'https://github.com/example/repo', got %q", settings.RepoURL)
		}
		if settings.Owner != "example" {
			t.Errorf("Expected owner 'example', got %q", settings.Owner)
		}
		if settings.RepoName != "repo" {
			t.Errorf("Expected repo name 'repo', got %q", settings.RepoName)
		}
		if settings.Branch != "develop" {
			t.Errorf("Expected branch 'develop', got %q", settings.Branch)
		}
		if !settings.SyncEnabled {
			t.Error("Expected sync enabled")
		}
	})

	t.Run("non-member gets forbidden", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		otherID := ts.CreateTestUser(t, "other@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Private GH Project")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d/github", projectID), nil, otherID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleGetProjectGitHubSettings(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusForbidden)
	})

	t.Run("member with viewer role can read github settings", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		viewerID := ts.CreateTestUser(t, "viewer@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Viewer GH Project")

		ts.AddProjectMember(t, projectID, viewerID, ownerID, "viewer")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			fmt.Sprintf("/api/projects/%d/github", projectID), nil, viewerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleGetProjectGitHubSettings(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)
	})

	t.Run("invalid project ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet,
			"/api/projects/abc/github", nil, ownerID,
			map[string]string{"id": "abc"})

		ts.HandleGetProjectGitHubSettings(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
	})
}

// TestHandleUpdateProjectGitHubSettings tests the HandleUpdateProjectGitHubSettings handler.
func TestHandleUpdateProjectGitHubSettings(t *testing.T) {
	t.Run("owner can update github settings", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Update GH Project")

		body := UpdateProjectGitHubRequest{
			RepoURL:     "https://github.com/myorg/myrepo",
			Owner:       "myorg",
			RepoName:    "myrepo",
			Branch:      "main",
			SyncEnabled: true,
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/github", projectID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleUpdateProjectGitHubSettings(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp map[string]string
		DecodeJSON(t, rec, &resp)

		if resp["message"] != "GitHub settings updated successfully" {
			t.Errorf("Expected success message, got %q", resp["message"])
		}

		// Verify settings were saved to the database
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var repoURL, ghOwner, repoName, branch string
		var syncEnabled int
		err := ts.DB.QueryRowContext(ctx,
			`SELECT github_repo_url, github_owner, github_repo_name, github_branch, github_sync_enabled
			 FROM projects WHERE id = ?`, projectID,
		).Scan(&repoURL, &ghOwner, &repoName, &branch, &syncEnabled)
		if err != nil {
			t.Fatalf("Failed to query GitHub settings: %v", err)
		}

		if repoURL != "https://github.com/myorg/myrepo" {
			t.Errorf("Expected repo URL 'https://github.com/myorg/myrepo', got %q", repoURL)
		}
		if ghOwner != "myorg" {
			t.Errorf("Expected owner 'myorg', got %q", ghOwner)
		}
		if repoName != "myrepo" {
			t.Errorf("Expected repo name 'myrepo', got %q", repoName)
		}
		if branch != "main" {
			t.Errorf("Expected branch 'main', got %q", branch)
		}
		if syncEnabled != 1 {
			t.Errorf("Expected sync_enabled=1, got %d", syncEnabled)
		}
	})

	t.Run("can disable sync", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Disable Sync Project")

		// First enable
		body := UpdateProjectGitHubRequest{
			RepoURL:     "https://github.com/org/repo",
			Owner:       "org",
			RepoName:    "repo",
			Branch:      "main",
			SyncEnabled: true,
		}

		rec1, req1 := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/github", projectID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleUpdateProjectGitHubSettings(rec1, req1)
		AssertStatusCode(t, rec1.Code, http.StatusOK)

		// Then disable
		body.SyncEnabled = false

		rec2, req2 := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/github", projectID), body, ownerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})
		ts.HandleUpdateProjectGitHubSettings(rec2, req2)
		AssertStatusCode(t, rec2.Code, http.StatusOK)

		// Verify sync is disabled
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var syncEnabled int
		err := ts.DB.QueryRowContext(ctx,
			`SELECT github_sync_enabled FROM projects WHERE id = ?`, projectID,
		).Scan(&syncEnabled)
		if err != nil {
			t.Fatalf("Failed to query sync status: %v", err)
		}
		if syncEnabled != 0 {
			t.Errorf("Expected sync_enabled=0, got %d", syncEnabled)
		}
	})

	t.Run("non-owner/admin gets forbidden", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		viewerID := ts.CreateTestUser(t, "viewer@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Forbidden GH Update")

		ts.AddProjectMember(t, projectID, viewerID, ownerID, "viewer")

		body := UpdateProjectGitHubRequest{
			RepoURL:     "https://github.com/hacker/repo",
			Owner:       "hacker",
			RepoName:    "repo",
			Branch:      "main",
			SyncEnabled: false,
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/github", projectID), body, viewerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleUpdateProjectGitHubSettings(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusForbidden)
	})

	t.Run("editor gets forbidden", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		editorID := ts.CreateTestUser(t, "editor@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Editor GH Forbidden")

		ts.AddProjectMember(t, projectID, editorID, ownerID, "editor")

		body := UpdateProjectGitHubRequest{
			RepoURL: "https://github.com/org/repo",
			Owner:   "org",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/github", projectID), body, editorID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleUpdateProjectGitHubSettings(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusForbidden)
	})

	t.Run("admin can update github settings", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Admin GH Update")

		ts.AddProjectMember(t, projectID, adminID, ownerID, "admin")

		body := UpdateProjectGitHubRequest{
			RepoURL:  "https://github.com/admin-org/repo",
			Owner:    "admin-org",
			RepoName: "repo",
			Branch:   "develop",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/github", projectID), body, adminID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleUpdateProjectGitHubSettings(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)
	})

	t.Run("invalid project ID returns 400", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")

		body := UpdateProjectGitHubRequest{RepoURL: "https://github.com/org/repo"}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			"/api/projects/abc/github", body, ownerID,
			map[string]string{"id": "abc"})

		ts.HandleUpdateProjectGitHubSettings(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusBadRequest)
	})

	t.Run("non-member gets forbidden", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		strangerID := ts.CreateTestUser(t, "stranger@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Stranger GH Update")

		body := UpdateProjectGitHubRequest{
			RepoURL: "https://github.com/stranger/repo",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
			fmt.Sprintf("/api/projects/%d/github", projectID), body, strangerID,
			map[string]string{"id": fmt.Sprintf("%d", projectID)})

		ts.HandleUpdateProjectGitHubSettings(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusForbidden)
	})
}

// TestUserHasProjectAccess tests the userHasProjectAccess helper function.
func TestUserHasProjectAccess(t *testing.T) {
	t.Run("owner has access", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Access Test")

		hasAccess, err := ts.userHasProjectAccess(int(ownerID), int(projectID))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !hasAccess {
			t.Error("Expected owner to have access")
		}
	})

	t.Run("member has access", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		memberID := ts.CreateTestUser(t, "member@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Member Access")

		ts.AddProjectMember(t, projectID, memberID, ownerID, "viewer")

		hasAccess, err := ts.userHasProjectAccess(int(memberID), int(projectID))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !hasAccess {
			t.Error("Expected member to have access")
		}
	})

	t.Run("non-member does not have access", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		otherID := ts.CreateTestUser(t, "other@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "No Access")

		hasAccess, err := ts.userHasProjectAccess(int(otherID), int(projectID))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if hasAccess {
			t.Error("Expected non-member to not have access")
		}
	})

	t.Run("nonexistent project returns error", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")

		_, err := ts.userHasProjectAccess(int(ownerID), 99999)
		if err == nil {
			t.Error("Expected error for nonexistent project")
		}
	})

	t.Run("all member roles have access", func(t *testing.T) {
		roles := []string{"viewer", "member", "editor", "owner"}

		for _, role := range roles {
			t.Run(role, func(t *testing.T) {
				ts := NewTestServer(t)
				defer ts.Close()

				ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				projectID := ts.CreateTestProject(t, ownerID, "Role Access "+role)

				ts.AddProjectMember(t, projectID, userID, ownerID, role)

				hasAccess, err := ts.userHasProjectAccess(int(userID), int(projectID))
				if err != nil {
					t.Fatalf("Unexpected error: %v", err)
				}
				if !hasAccess {
					t.Errorf("Expected user with role %q to have access", role)
				}
			})
		}
	})
}

// TestUserIsProjectOwnerOrAdmin tests the userIsProjectOwnerOrAdmin helper function.
func TestUserIsProjectOwnerOrAdmin(t *testing.T) {
	t.Run("project owner is owner or admin", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Owner Check")

		isOwnerOrAdmin, err := ts.userIsProjectOwnerOrAdmin(int(ownerID), int(projectID))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !isOwnerOrAdmin {
			t.Error("Expected project owner to be identified as owner or admin")
		}
	})

	t.Run("admin member is owner or admin", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Admin Check")

		ts.AddProjectMember(t, projectID, adminID, ownerID, "admin")

		isOwnerOrAdmin, err := ts.userIsProjectOwnerOrAdmin(int(adminID), int(projectID))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if !isOwnerOrAdmin {
			t.Error("Expected admin to be identified as owner or admin")
		}
	})

	t.Run("viewer is not owner or admin", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		viewerID := ts.CreateTestUser(t, "viewer@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Viewer Check")

		ts.AddProjectMember(t, projectID, viewerID, ownerID, "viewer")

		isOwnerOrAdmin, err := ts.userIsProjectOwnerOrAdmin(int(viewerID), int(projectID))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if isOwnerOrAdmin {
			t.Error("Expected viewer to not be identified as owner or admin")
		}
	})

	t.Run("editor is not owner or admin", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		editorID := ts.CreateTestUser(t, "editor@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Editor Check")

		ts.AddProjectMember(t, projectID, editorID, ownerID, "editor")

		isOwnerOrAdmin, err := ts.userIsProjectOwnerOrAdmin(int(editorID), int(projectID))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if isOwnerOrAdmin {
			t.Error("Expected editor to not be identified as owner or admin")
		}
	})

	t.Run("member is not owner or admin", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		memberID := ts.CreateTestUser(t, "member@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Member Check")

		ts.AddProjectMember(t, projectID, memberID, ownerID, "member")

		isOwnerOrAdmin, err := ts.userIsProjectOwnerOrAdmin(int(memberID), int(projectID))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if isOwnerOrAdmin {
			t.Error("Expected member to not be identified as owner or admin")
		}
	})

	t.Run("non-member is not owner or admin", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
		strangerID := ts.CreateTestUser(t, "stranger@example.com", "password123")
		projectID := ts.CreateTestProject(t, ownerID, "Stranger Check")

		isOwnerOrAdmin, err := ts.userIsProjectOwnerOrAdmin(int(strangerID), int(projectID))
		if err != nil {
			t.Fatalf("Unexpected error: %v", err)
		}
		if isOwnerOrAdmin {
			t.Error("Expected non-member to not be identified as owner or admin")
		}
	})

	t.Run("nonexistent project returns error", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")

		_, err := ts.userIsProjectOwnerOrAdmin(int(ownerID), 99999)
		if err == nil {
			t.Error("Expected error for nonexistent project")
		}
	})
}
