package api

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// addTeamMembership puts userID on teamID with the given team role and status.
func addTeamMembership(t *testing.T, ts *TestServer, teamID, userID int64, role, status string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, err := ts.DB.ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id, role, status, joined_at) VALUES (?, ?, ?, ?, CURRENT_TIMESTAMP)`,
		teamID, userID, role, status,
	); err != nil {
		t.Fatalf("Failed to add team member: %v", err)
	}
}

func projectTeamID(t *testing.T, ts *TestServer, projectID int64) *int64 {
	t.Helper()
	var teamID *int64
	if err := ts.DB.QueryRow(`SELECT team_id FROM projects WHERE id = ?`, projectID).Scan(&teamID); err != nil {
		t.Fatalf("Failed to read project team: %v", err)
	}
	return teamID
}

// TestHandleUpdateProject_MoveTeam covers who may move a project between
// teams: only the project's owner (projects.owner_id), and only into a team
// they belong to as owner or member.
func TestHandleUpdateProject_MoveTeam(t *testing.T) {
	type fixture struct {
		ts        *TestServer
		ownerID   int64
		coOwnerID int64
		editorID  int64
		outsider  int64
		projectID int64
		homeTeam  int64
		// Teams the owner belongs to in different ways.
		ownedTeam  int64
		memberTeam int64
		foreign    int64
		invited    int64
	}

	setup := func(t *testing.T) fixture {
		ts := NewTestServer(t)
		f := fixture{ts: ts}
		f.ownerID = ts.CreateTestUser(t, "owner@example.com", "password123")
		f.coOwnerID = ts.CreateTestUser(t, "coowner@example.com", "password123")
		f.editorID = ts.CreateTestUser(t, "editor@example.com", "password123")
		f.outsider = ts.CreateTestUser(t, "outsider@example.com", "password123")
		someoneElse := ts.CreateTestUser(t, "else@example.com", "password123")

		f.homeTeam = createTestTeamForUser(t, ts, f.ownerID)
		f.ownedTeam = createTestTeamForUser(t, ts, f.ownerID)
		f.memberTeam = createTestTeamForUser(t, ts, someoneElse)
		addTeamMembership(t, ts, f.memberTeam, f.ownerID, "member", "active")
		f.foreign = createTestTeamForUser(t, ts, someoneElse)
		f.invited = createTestTeamForUser(t, ts, someoneElse)
		addTeamMembership(t, ts, f.invited, f.ownerID, "member", "invited")
		// The co-owner and editor share every team with the owner, so a refusal
		// can only come from the project-owner rule.
		for _, team := range []int64{f.homeTeam, f.ownedTeam, f.memberTeam} {
			addTeamMembership(t, ts, team, f.coOwnerID, "member", "active")
			addTeamMembership(t, ts, team, f.editorID, "member", "active")
		}

		f.projectID = ts.CreateTestProject(t, f.ownerID, "Movable")
		if _, err := ts.DB.Exec(`UPDATE projects SET team_id = ? WHERE id = ?`, f.homeTeam, f.projectID); err != nil {
			t.Fatalf("Failed to set project team: %v", err)
		}
		ts.AddProjectMember(t, f.projectID, f.coOwnerID, f.ownerID, "owner")
		ts.AddProjectMember(t, f.projectID, f.editorID, f.ownerID, "editor")
		return f
	}

	tests := []struct {
		name       string
		caller     func(f fixture) int64
		target     func(f fixture) int64
		prepare    func(t *testing.T, f fixture)
		wantStatus int
		wantErr    string
	}{
		{
			name:       "owner moves to a team they own",
			caller:     func(f fixture) int64 { return f.ownerID },
			target:     func(f fixture) int64 { return f.ownedTeam },
			wantStatus: http.StatusOK,
		},
		{
			name:       "owner moves to a team they are only a member of",
			caller:     func(f fixture) int64 { return f.ownerID },
			target:     func(f fixture) int64 { return f.memberTeam },
			wantStatus: http.StatusOK,
		},
		{
			name:   "owner keeps the right even after their project role was lowered",
			caller: func(f fixture) int64 { return f.ownerID },
			target: func(f fixture) int64 { return f.ownedTeam },
			prepare: func(t *testing.T, f fixture) {
				if _, err := f.ts.DB.Exec(`UPDATE project_members SET role = 'viewer' WHERE project_id = ? AND user_id = ?`, f.projectID, f.ownerID); err != nil {
					t.Fatalf("Failed to demote owner: %v", err)
				}
			},
			wantStatus: http.StatusOK,
		},
		{
			name:       "someone with the Owner role who is not the project owner is refused",
			caller:     func(f fixture) int64 { return f.coOwnerID },
			target:     func(f fixture) int64 { return f.ownedTeam },
			wantStatus: http.StatusForbidden,
			wantErr:    "only the project owner can move this project to another team",
		},
		{
			name:       "an editor is refused",
			caller:     func(f fixture) int64 { return f.editorID },
			target:     func(f fixture) int64 { return f.memberTeam },
			wantStatus: http.StatusForbidden,
			wantErr:    "only the project owner can move this project to another team",
		},
		{
			name:       "owner cannot move into a team they are not on",
			caller:     func(f fixture) int64 { return f.ownerID },
			target:     func(f fixture) int64 { return f.foreign },
			wantStatus: http.StatusForbidden,
			wantErr:    "you can only move a project to a team you are a member of",
		},
		{
			name:       "an unaccepted team invitation does not count as membership",
			caller:     func(f fixture) int64 { return f.ownerID },
			target:     func(f fixture) int64 { return f.invited },
			wantStatus: http.StatusForbidden,
			wantErr:    "you can only move a project to a team you are a member of",
		},
		{
			name:       "a team that does not exist is refused",
			caller:     func(f fixture) int64 { return f.ownerID },
			target:     func(f fixture) int64 { return 999999 },
			wantStatus: http.StatusForbidden,
			wantErr:    "you can only move a project to a team you are a member of",
		},
		{
			name:       "someone outside the project cannot see it",
			caller:     func(f fixture) int64 { return f.outsider },
			target:     func(f fixture) int64 { return f.ownedTeam },
			wantStatus: http.StatusNotFound,
			wantErr:    "project not found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := setup(t)
			defer f.ts.Close()
			if tt.prepare != nil {
				tt.prepare(t, f)
			}
			target := tt.target(f)

			rec, req := f.ts.MakeAuthRequest(t, http.MethodPatch,
				fmt.Sprintf("/api/projects/%d", f.projectID),
				UpdateProjectRequest{TeamID: &target}, tt.caller(f),
				map[string]string{"id": fmt.Sprintf("%d", f.projectID)})
			f.ts.HandleUpdateProject(rec, req)

			if tt.wantStatus != http.StatusOK {
				AssertError(t, rec, tt.wantStatus, tt.wantErr, "")
				if got := projectTeamID(t, f.ts, f.projectID); got == nil || *got != f.homeTeam {
					t.Errorf("refused move changed the team: got %v, want %d", got, f.homeTeam)
				}
				return
			}

			AssertStatusCode(t, rec.Code, http.StatusOK)
			var p Project
			DecodeJSON(t, rec, &p)
			if p.TeamID == nil || *p.TeamID != target {
				t.Errorf("response team_id = %v, want %d", p.TeamID, target)
			}
			if got := projectTeamID(t, f.ts, f.projectID); got == nil || *got != target {
				t.Errorf("stored team_id = %v, want %d", got, target)
			}
		})
	}
}

// A co-owner can still edit the project's name; only moving is owner-only.
func TestHandleUpdateProject_CoOwnerCanStillRename(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	coOwnerID := ts.CreateTestUser(t, "coowner@example.com", "password123")
	projectID := ts.CreateTestProject(t, ownerID, "Original")
	ts.AddProjectMember(t, projectID, coOwnerID, ownerID, "owner")

	rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
		fmt.Sprintf("/api/projects/%d", projectID),
		UpdateProjectRequest{Name: stringPtr("Renamed")}, coOwnerID,
		map[string]string{"id": fmt.Sprintf("%d", projectID)})
	ts.HandleUpdateProject(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)
}
