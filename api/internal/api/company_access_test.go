package api

import (
	"fmt"
	"net/http"
	"testing"
)

func projectRole(t *testing.T, ts *TestServer, projectID, userID int64) string {
	t.Helper()
	var role string
	err := ts.DB.QueryRow(`SELECT role FROM project_members WHERE project_id = ? AND user_id = ?`, projectID, userID).Scan(&role)
	if err != nil {
		return ""
	}
	return role
}

func TestCompanyAccess(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	owner := ts.CreateTestUser(t, "ceo@example.com", "password123")
	admin := ts.CreateTestUser(t, "admin@example.com", "password123")
	joiner := ts.CreateTestUser(t, "joiner@example.com", "password123")
	guest := ts.CreateTestUser(t, "guest@example.com", "password123")
	team := createTestTeamForUser(t, ts, owner)
	addTeamMembership(t, ts, team, admin, "admin", "active")
	tid := fmt.Sprintf("%d", team)

	// A new project in the company is shared by default and reaches existing members.
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/projects", map[string]interface{}{"name": "Sales", "team_id": team}, owner, nil)
	ts.HandleCreateProject(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusCreated)
	var shared Project
	DecodeJSON(t, rec, &shared)
	if !shared.SharedWithTeam {
		t.Fatal("new project should be shared with the company")
	}

	rec, req = ts.MakeAuthRequest(t, http.MethodPost, "/api/projects", map[string]interface{}{"name": "Board", "team_id": team, "shared_with_team": false}, owner, nil)
	ts.HandleCreateProject(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusCreated)
	var private Project
	DecodeJSON(t, rec, &private)

	// A guest outside the company is an explicit exception on the private project.
	ts.AddProjectMember(t, private.ID, guest, owner, "viewer")

	// Joining the company grants the shared project only.
	rec, req = ts.MakeAuthRequest(t, http.MethodPost, "/api/teams/"+tid+"/members", map[string]interface{}{"user_id": joiner}, owner, map[string]string{"teamId": tid})
	ts.HandleAddTeamMember(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusCreated)

	tests := []struct {
		name    string
		project int64
		user    int64
		want    string
	}{
		{"admin gets editor on shared project", shared.ID, admin, "editor"},
		{"joiner gets member on shared project", shared.ID, joiner, "member"},
		{"private project stays closed to admin", private.ID, admin, ""},
		{"private project stays closed to joiner", private.ID, joiner, ""},
		{"guest keeps explicit access", private.ID, guest, "viewer"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := projectRole(t, ts, tt.project, tt.user); got != tt.want {
				t.Errorf("role = %q, want %q", got, tt.want)
			}
		})
	}

	t.Run("only the project owner can change sharing", func(t *testing.T) {
		pid := fmt.Sprintf("%d", shared.ID)
		rec, req := ts.MakeAuthRequest(t, http.MethodPatch, "/api/projects/"+pid, map[string]interface{}{"shared_with_team": false}, admin, map[string]string{"id": pid})
		ts.HandleUpdateProject(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusForbidden)
	})

	t.Run("sharing a private project opens it to the company", func(t *testing.T) {
		pid := fmt.Sprintf("%d", private.ID)
		rec, req := ts.MakeAuthRequest(t, http.MethodPatch, "/api/projects/"+pid, map[string]interface{}{"shared_with_team": true}, owner, map[string]string{"id": pid})
		ts.HandleUpdateProject(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusOK)
		if got := projectRole(t, ts, private.ID, joiner); got != "member" {
			t.Errorf("joiner role = %q, want member", got)
		}
	})

	t.Run("leaving the company removes shared access", func(t *testing.T) {
		var memberID int64
		if err := ts.DB.QueryRow(`SELECT id FROM team_members WHERE team_id = ? AND user_id = ?`, team, joiner).Scan(&memberID); err != nil {
			t.Fatal(err)
		}
		mid := fmt.Sprintf("%d", memberID)
		rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/teams/"+tid+"/members/"+mid, nil, owner, map[string]string{"teamId": tid, "memberId": mid})
		ts.HandleRemoveTeamMember(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusOK)
		if got := projectRole(t, ts, shared.ID, joiner); got != "" {
			t.Errorf("joiner still has %q on shared project", got)
		}
		if got := projectRole(t, ts, shared.ID, owner); got != "owner" {
			t.Errorf("owner lost access: %q", got)
		}
	})
}
