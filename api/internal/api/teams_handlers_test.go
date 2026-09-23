package api

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"
)

// multiTeamFixture models one person working across two companies:
// owner runs "Elastio" (home team) and "Intelliviz"; elastioDev belongs only
// to Elastio; nakul and faiz belong to Intelliviz (nakul also has his own
// personal team, created after joining Intelliviz).
type multiTeamFixture struct {
	owner, elastioDev, nakul, faiz int64
	elastio, intelliviz, nakulTeam int64
	elastioDevMember, nakulMember  int64
}

func newMultiTeamFixture(t *testing.T, ts *TestServer) multiTeamFixture {
	t.Helper()
	var f multiTeamFixture
	f.owner = ts.CreateTestUser(t, "owner@example.com", "password123")
	f.elastioDev = ts.CreateTestUser(t, "bob.builder@elastio.example", "password123")
	f.nakul = ts.CreateTestUser(t, "nakul@intelliviz.example", "password123")
	f.faiz = ts.CreateTestUser(t, "faiz@intelliviz.example", "password123")

	f.elastio = createTestTeam(t, ts, f.owner, "Elastio")
	f.intelliviz = createTestTeam(t, ts, f.owner, "Intelliviz")
	f.elastioDevMember = addTeamMember(t, ts, f.elastio, f.elastioDev, "member")
	f.nakulMember = addTeamMember(t, ts, f.intelliviz, f.nakul, "admin")
	addTeamMember(t, ts, f.intelliviz, f.faiz, "member")
	f.nakulTeam = createTestTeam(t, ts, f.nakul, "Nakul's Team")
	return f
}

func teamParam(id int64) map[string]string {
	return map[string]string{"teamId": fmt.Sprintf("%d", id)}
}

func memberCount(t *testing.T, ts *TestServer, teamID, userID int64) int {
	t.Helper()
	var n int
	if err := ts.DB.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, userID).Scan(&n); err != nil {
		t.Fatalf("count team members: %v", err)
	}
	return n
}

func TestHandleListMyTeams(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newMultiTeamFixture(t, ts)

	tests := []struct {
		name      string
		userID    int64
		wantTeams map[string]bool // name -> is_home
	}{
		{"owner sees both companies", f.owner, map[string]bool{"Elastio": true, "Intelliviz": false}},
		{"nakul sees only his teams", f.nakul, map[string]bool{"Intelliviz": false, "Nakul's Team": true}},
		{"elastio dev sees only elastio", f.elastioDev, map[string]bool{"Elastio": true}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/teams", nil, tt.userID, nil)
			ts.HandleListMyTeams(rec, req)
			AssertStatusCode(t, rec.Code, http.StatusOK)

			var teams []TeamSummary
			DecodeJSON(t, rec, &teams)
			if len(teams) != len(tt.wantTeams) {
				t.Fatalf("got %d teams, want %d: %+v", len(teams), len(tt.wantTeams), teams)
			}
			for _, team := range teams {
				wantHome, ok := tt.wantTeams[team.Name]
				if !ok {
					t.Errorf("unexpected team %q", team.Name)
					continue
				}
				if team.IsHome != wantHome {
					t.Errorf("team %q is_home = %v, want %v", team.Name, team.IsHome, wantHome)
				}
			}
		})
	}
}

func TestHandleGetTeamMembers_ScopedToTeam(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newMultiTeamFixture(t, ts)

	tests := []struct {
		name       string
		userID     int64
		teamID     int64
		wantStatus int
		wantEmails []string
	}{
		{"nakul sees intelliviz roster", f.nakul, f.intelliviz, http.StatusOK,
			[]string{"owner@example.com", "nakul@intelliviz.example", "faiz@intelliviz.example"}},
		{"nakul cannot read elastio roster", f.nakul, f.elastio, http.StatusNotFound, nil},
		{"elastio dev cannot read intelliviz roster", f.elastioDev, f.intelliviz, http.StatusNotFound, nil},
		{"owner reads elastio roster", f.owner, f.elastio, http.StatusOK,
			[]string{"owner@example.com", "bob.builder@elastio.example"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/teams/x/members", nil, tt.userID, teamParam(tt.teamID))
			ts.HandleGetTeamMembers(rec, req)
			AssertStatusCode(t, rec.Code, tt.wantStatus)
			if tt.wantStatus != http.StatusOK {
				return
			}
			var members []TeamMember
			DecodeJSON(t, rec, &members)
			got := map[string]bool{}
			for _, m := range members {
				got[m.Email] = true
			}
			if len(got) != len(tt.wantEmails) {
				t.Fatalf("got members %v, want %v", got, tt.wantEmails)
			}
			for _, e := range tt.wantEmails {
				if !got[e] {
					t.Errorf("missing member %s", e)
				}
			}
		})
	}
}

func TestHandleCreateTeam(t *testing.T) {
	tests := []struct {
		name       string
		body       map[string]string
		wantStatus int
	}{
		{"valid", map[string]string{"name": "  Intelliviz  "}, http.StatusCreated},
		{"empty name", map[string]string{"name": "   "}, http.StatusBadRequest},
		{"too long", map[string]string{"name": strings.Repeat("a", 101)}, http.StatusBadRequest},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()
			userID := ts.CreateTestUser(t, "creator@example.com", "password123")

			rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/teams", tt.body, userID, nil)
			ts.HandleCreateTeam(rec, req)
			AssertStatusCode(t, rec.Code, tt.wantStatus)
			if tt.wantStatus != http.StatusCreated {
				return
			}
			var team Team
			DecodeJSON(t, rec, &team)
			if team.Name != "Intelliviz" || team.OwnerID != userID {
				t.Errorf("unexpected team %+v", team)
			}
			var role string
			if err := ts.DB.QueryRowContext(context.Background(),
				`SELECT role FROM team_members WHERE team_id = ? AND user_id = ?`, team.ID, userID).Scan(&role); err != nil {
				t.Fatalf("creator membership missing: %v", err)
			}
			if role != "owner" {
				t.Errorf("creator role = %q, want owner", role)
			}
		})
	}
}

func TestHandleMoveTeamMember(t *testing.T) {
	tests := []struct {
		name       string
		caller     func(f multiTeamFixture) int64
		source     func(f multiTeamFixture) int64
		member     func(f multiTeamFixture) int64
		target     func(f multiTeamFixture) int64
		wantStatus int
	}{
		{
			name:       "owner moves elastio dev to intelliviz",
			caller:     func(f multiTeamFixture) int64 { return f.owner },
			source:     func(f multiTeamFixture) int64 { return f.elastio },
			member:     func(f multiTeamFixture) int64 { return f.elastioDevMember },
			target:     func(f multiTeamFixture) int64 { return f.intelliviz },
			wantStatus: http.StatusOK,
		},
		{
			name:       "admin of source only cannot move into a team they don't manage",
			caller:     func(f multiTeamFixture) int64 { return f.nakul },
			source:     func(f multiTeamFixture) int64 { return f.intelliviz },
			member:     func(f multiTeamFixture) int64 { return f.nakulMember },
			target:     func(f multiTeamFixture) int64 { return f.elastio },
			wantStatus: http.StatusForbidden,
		},
		{
			name:       "cannot target the same team",
			caller:     func(f multiTeamFixture) int64 { return f.owner },
			source:     func(f multiTeamFixture) int64 { return f.elastio },
			member:     func(f multiTeamFixture) int64 { return f.elastioDevMember },
			target:     func(f multiTeamFixture) int64 { return f.elastio },
			wantStatus: http.StatusBadRequest,
		},
		{
			name:       "plain member cannot move people",
			caller:     func(f multiTeamFixture) int64 { return f.faiz },
			source:     func(f multiTeamFixture) int64 { return f.intelliviz },
			member:     func(f multiTeamFixture) int64 { return f.nakulMember },
			target:     func(f multiTeamFixture) int64 { return f.nakulTeam },
			wantStatus: http.StatusForbidden,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()
			f := newMultiTeamFixture(t, ts)

			params := teamParam(tt.source(f))
			params["memberId"] = fmt.Sprintf("%d", tt.member(f))
			body := map[string]int64{"target_team_id": tt.target(f)}
			rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/teams/x/members/y/move", body, tt.caller(f), params)
			ts.HandleMoveTeamMember(rec, req)
			AssertStatusCode(t, rec.Code, tt.wantStatus)

			if tt.wantStatus == http.StatusOK {
				if n := memberCount(t, ts, f.elastio, f.elastioDev); n != 0 {
					t.Errorf("still in source team")
				}
				if n := memberCount(t, ts, f.intelliviz, f.elastioDev); n != 1 {
					t.Errorf("not added to target team")
				}
			}
		})
	}
}

func TestHandleMoveTeamMember_CannotMoveOwner(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newMultiTeamFixture(t, ts)
	// Give the owner admin rights over Nakul's team so only the owner rule applies.
	addTeamMember(t, ts, f.nakulTeam, f.owner, "admin")

	var ownerMemberID int64
	if err := ts.DB.QueryRowContext(context.Background(),
		`SELECT id FROM team_members WHERE team_id = ? AND user_id = ?`, f.elastio, f.owner).Scan(&ownerMemberID); err != nil {
		t.Fatal(err)
	}
	params := teamParam(f.elastio)
	params["memberId"] = fmt.Sprintf("%d", ownerMemberID)
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/", map[string]int64{"target_team_id": f.nakulTeam}, f.owner, params)
	ts.HandleMoveTeamMember(rec, req)
	AssertError(t, rec, http.StatusForbidden, "owner", "forbidden")
}

func TestHandleLeaveTeam(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newMultiTeamFixture(t, ts)

	tests := []struct {
		name       string
		userID     int64
		teamID     int64
		wantStatus int
	}{
		{"owner cannot leave own team", f.owner, f.intelliviz, http.StatusConflict},
		{"elastio dev cannot leave their only team", f.elastioDev, f.elastio, http.StatusConflict},
		{"non-member gets not found", f.faiz, f.elastio, http.StatusNotFound},
		{"nakul leaves intelliviz", f.nakul, f.intelliviz, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/teams/x/leave", nil, tt.userID, teamParam(tt.teamID))
			ts.HandleLeaveTeam(rec, req)
			AssertStatusCode(t, rec.Code, tt.wantStatus)
		})
	}

	if n := memberCount(t, ts, f.intelliviz, f.nakul); n != 0 {
		t.Errorf("nakul still in intelliviz after leaving")
	}
}

func TestHandleDeleteTeam(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newMultiTeamFixture(t, ts)
	spare := createTestTeam(t, ts, f.owner, "Spare")
	addTeamMember(t, ts, spare, f.faiz, "member")
	withProject := createTestTeam(t, ts, f.owner, "Has Projects")
	projectID := ts.CreateTestProject(t, f.owner, "Client work")
	if _, err := ts.DB.ExecContext(context.Background(), `UPDATE projects SET team_id = ? WHERE id = ?`, withProject, projectID); err != nil {
		t.Fatal(err)
	}

	tests := []struct {
		name       string
		userID     int64
		teamID     int64
		wantStatus int
	}{
		{"home team is protected", f.owner, f.elastio, http.StatusConflict},
		{"team with projects is protected", f.owner, withProject, http.StatusConflict},
		{"admin cannot delete", f.nakul, f.intelliviz, http.StatusForbidden},
		{"owner deletes empty team", f.owner, spare, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/teams/x", nil, tt.userID, teamParam(tt.teamID))
			ts.HandleDeleteTeam(rec, req)
			AssertStatusCode(t, rec.Code, tt.wantStatus)
		})
	}

	if n := memberCount(t, ts, spare, f.faiz); n != 0 {
		t.Errorf("members of deleted team were not removed")
	}
}

func TestHandleSearchUsers_NoGlobalDirectory(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newMultiTeamFixture(t, ts)

	tests := []struct {
		name      string
		userID    int64
		teamID    int64
		query     string
		wantEmail string // empty means no results expected
	}{
		{"stranger not found by partial name", f.nakul, f.nakulTeam, "bob", ""},
		{"stranger not found by partial email", f.nakul, f.nakulTeam, "elastio", ""},
		{"stranger found by exact email", f.nakul, f.nakulTeam, "Bob.Builder@elastio.example", "bob.builder@elastio.example"},
		{"teammate from another shared team found by partial", f.nakul, f.nakulTeam, "faiz", "faiz@intelliviz.example"},
		{"owner finds elastio dev for intelliviz", f.owner, f.intelliviz, "bob", "bob.builder@elastio.example"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/teams/x/users/search?q="+tt.query, nil, tt.userID, teamParam(tt.teamID))
			ts.HandleSearchUsers(rec, req)
			AssertStatusCode(t, rec.Code, http.StatusOK)
			var results []UserSearchResult
			DecodeJSON(t, rec, &results)
			if tt.wantEmail == "" {
				if len(results) != 0 {
					t.Errorf("expected no results, got %+v", results)
				}
				return
			}
			if len(results) != 1 || results[0].Email != tt.wantEmail {
				t.Errorf("got %+v, want only %s", results, tt.wantEmail)
			}
		})
	}
}

func TestHandleGetCollaborators_GroupedByTeam(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newMultiTeamFixture(t, ts)

	tests := []struct {
		name   string
		userID int64
		want   map[string]string // email -> team name
	}{
		{"nakul only sees intelliviz people", f.nakul, map[string]string{
			"owner@example.com":       "Intelliviz",
			"faiz@intelliviz.example": "Intelliviz",
		}},
		{"elastio dev never sees intelliviz people", f.elastioDev, map[string]string{
			"owner@example.com": "Elastio",
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/me/collaborators", nil, tt.userID, nil)
			ts.HandleGetCollaborators(rec, req)
			AssertStatusCode(t, rec.Code, http.StatusOK)
			var got []Collaborator
			DecodeJSON(t, rec, &got)
			if len(got) != len(tt.want) {
				t.Fatalf("got %+v, want %v", got, tt.want)
			}
			for _, c := range got {
				if tt.want[c.Email] != c.TeamName {
					t.Errorf("%s grouped under %q, want %q", c.Email, c.TeamName, tt.want[c.Email])
				}
			}
		})
	}

	// The owner sees each person under the team they share.
	rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/me/collaborators", nil, f.owner, nil)
	ts.HandleGetCollaborators(rec, req)
	var ownerView []Collaborator
	DecodeJSON(t, rec, &ownerView)
	if len(ownerView) != 3 || ownerView[0].TeamName != "Elastio" {
		t.Errorf("unexpected owner view %+v", ownerView)
	}
}

func TestGetUserTeamID_PrefersOwnedTeam(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newMultiTeamFixture(t, ts)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tests := []struct {
		name   string
		userID int64
		want   int64
	}{
		{"nakul joined intelliviz first but owns a team", f.nakul, f.nakulTeam},
		{"owner's oldest owned team", f.owner, f.elastio},
		{"member-only user gets the team they joined", f.faiz, f.intelliviz},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ts.getUserTeamID(ctx, tt.userID)
			if err != nil {
				t.Fatal(err)
			}
			if got != tt.want {
				t.Errorf("getUserTeamID = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestHandleCreateProject_TeamSelection(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newMultiTeamFixture(t, ts)

	tests := []struct {
		name       string
		teamID     *int64
		wantStatus int
		wantTeam   int64
	}{
		{"explicit team the caller belongs to", &f.intelliviz, http.StatusCreated, f.intelliviz},
		{"team the caller is not in", &f.elastio, http.StatusForbidden, 0},
		{"defaults to home team", nil, http.StatusCreated, f.nakulTeam},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			body := map[string]interface{}{"name": "Dashboard"}
			if tt.teamID != nil {
				body["team_id"] = *tt.teamID
			}
			rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/projects", body, f.nakul, nil)
			ts.HandleCreateProject(rec, req)
			AssertStatusCode(t, rec.Code, tt.wantStatus)
			if tt.wantStatus != http.StatusCreated {
				return
			}
			var p Project
			DecodeJSON(t, rec, &p)
			if p.TeamID == nil || *p.TeamID != tt.wantTeam {
				t.Errorf("project team = %v, want %d", p.TeamID, tt.wantTeam)
			}
		})
	}
}

func TestHandleUpdateProject_ChangeTeam(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()
	f := newMultiTeamFixture(t, ts)
	projectID := ts.CreateTestProject(t, f.owner, "Intelliviz App")
	ts.AddProjectMember(t, projectID, f.nakul, f.owner, "editor")
	idParam := map[string]string{"id": fmt.Sprintf("%d", projectID)}

	tests := []struct {
		name       string
		userID     int64
		teamID     int64
		wantStatus int
	}{
		{"editor cannot move project", f.nakul, f.intelliviz, http.StatusForbidden},
		{"owner cannot move into a team they're not in", f.owner, f.nakulTeam, http.StatusForbidden},
		{"owner moves project to intelliviz", f.owner, f.intelliviz, http.StatusOK},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec, req := ts.MakeAuthRequest(t, http.MethodPatch, "/api/projects/x", map[string]int64{"team_id": tt.teamID}, tt.userID, idParam)
			ts.HandleUpdateProject(rec, req)
			AssertStatusCode(t, rec.Code, tt.wantStatus)
		})
	}

	var teamID int64
	if err := ts.DB.QueryRowContext(context.Background(), `SELECT team_id FROM projects WHERE id = ?`, projectID).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	if teamID != f.intelliviz {
		t.Errorf("project team = %d, want %d", teamID, f.intelliviz)
	}
}
