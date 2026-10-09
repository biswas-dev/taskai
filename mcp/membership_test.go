package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// fakeTeam is an in-memory TaskAI: one project (2) in team 7, one wiki page (50).
type fakeTeam struct {
	mu         sync.Mutex
	project    []member
	team       []teamMember
	registered map[string]int64
	sharing    sharing
	teamID     *int64
	calls      []string
}

func newFakeTeam() *fakeTeam {
	team := int64(7)
	return &fakeTeam{registered: map[string]int64{}, teamID: &team, sharing: sharing{Visibility: "project", CanManage: true, CreatedBy: 1}}
}

func (f *fakeTeam) share(ids ...int64) {
	f.sharing.SharedWith = nil
	for _, id := range ids {
		f.sharing.SharedWith = append(f.sharing.SharedWith, struct {
			UserID   int64  `json:"user_id"`
			Email    string `json:"email"`
			UserName string `json:"user_name,omitempty"`
		}{UserID: id})
	}
}

func (f *fakeTeam) client(t *testing.T) *Client {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		reply := func(v any) { _ = json.NewEncoder(w).Encode(v) }
		switch r.Method + " " + r.URL.Path {
		case "GET /api/wiki/pages/50":
			reply(map[string]any{"id": 50, "project_id": 2})
		case "GET /api/wiki/pages/50/sharing":
			reply(f.sharing)
		case "PUT /api/wiki/pages/50/sharing":
			var ids []string
			var list []int64
			for _, v := range body["user_ids"].([]any) {
				list = append(list, int64(v.(float64)))
				ids = append(ids, fmt.Sprint(v))
			}
			f.calls = append(f.calls, fmt.Sprintf("updateWikiSharing %s [%s]", body["visibility"], strings.Join(ids, ",")))
			f.sharing.Visibility = body["visibility"].(string)
			f.share(list...)
			reply(f.sharing)
		case "GET /api/projects/2":
			reply(map[string]any{"id": 2, "name": "Elastio", "team_id": f.teamID})
		case "GET /api/projects/2/members":
			reply(f.project)
		case "POST /api/projects/2/members":
			email, role := body["email"].(string), body["role"].(string)
			f.calls = append(f.calls, "addProjectMember "+email+" "+role)
			for _, tm := range f.team {
				if tm.Email == email {
					f.project = append(f.project, member{UserID: tm.UserID, Email: tm.Email, Role: role})
					reply(map[string]any{"message": "Member added successfully"})
					return
				}
			}
			http.Error(w, `{"error":"User must be a member of this project's team"}`, http.StatusBadRequest)
		case "GET /api/teams/7/members":
			reply(f.team)
		case "POST /api/teams/7/members":
			id := int64(body["user_id"].(float64))
			f.calls = append(f.calls, fmt.Sprintf("addTeamMember %d", id))
			for email, uid := range f.registered {
				if uid == id {
					f.team = append(f.team, teamMember{UserID: id, Email: email})
					reply(map[string]any{"message": "member added"})
					return
				}
			}
			http.Error(w, `{"error":"user not found"}`, http.StatusNotFound)
		case "POST /api/teams/7/invite":
			email := body["email"].(string)
			f.calls = append(f.calls, "inviteTeamMember "+email)
			if uid, ok := f.registered[email]; ok {
				f.team = append(f.team, teamMember{UserID: uid, Email: email})
				reply(map[string]any{"status": "accepted"})
				return
			}
			reply(map[string]any{"status": "pending"})
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return &Client{BaseURL: srv.URL, APIKey: "k", HTTP: srv.Client()}
}

func expectCalls(t *testing.T, f *fakeTeam, want ...string) {
	t.Helper()
	if want == nil {
		want = []string{}
	}
	got := append([]string{}, f.calls...)
	if !reflect.DeepEqual(got, want) && !(len(got) == 0 && len(want) == 0) {
		t.Fatalf("calls = %q, want %q", got, want)
	}
}

func TestResolveMembersAndShareLists(t *testing.T) {
	members := []member{{UserID: 3, Email: "Amy@Example.com"}, {UserID: 4, Email: "bob@example.com"}}
	ids, unknownIDs, unknownEmails := resolveMembers(members, []int64{4, 99}, []string{" amy@example.com ", "gary@example.com"})
	if !reflect.DeepEqual(ids, []int64{4, 3}) || !reflect.DeepEqual(unknownIDs, []int64{99}) || !reflect.DeepEqual(unknownEmails, []string{"gary@example.com"}) {
		t.Fatalf("resolve: %v %v %v", ids, unknownIDs, unknownEmails)
	}
	if got := nextShareList([]int64{3, 4}, []int64{4, 5}, "add"); !reflect.DeepEqual(got, []int64{3, 4, 5}) {
		t.Fatalf("add: %v", got)
	}
	if got := nextShareList([]int64{3, 4}, []int64{5, 5}, "replace"); !reflect.DeepEqual(got, []int64{5}) {
		t.Fatalf("replace: %v", got)
	}
}

func TestUpdateWikiSharing(t *testing.T) {
	ctx := context.Background()
	setup := func() *fakeTeam {
		f := newFakeTeam()
		f.project = []member{{UserID: 3, Email: "amy@example.com"}, {UserID: 4, Email: "bob@example.com"}}
		f.sharing.Visibility = "restricted"
		f.share(3)
		return f
	}

	f := setup()
	s, _, err := updateWikiSharing(ctx, f.client(t), updateSharingIn{PageID: "50", Visibility: "restricted", Emails: []string{"BOB@example.com"}})
	if err != nil || len(s.SharedWith) != 2 {
		t.Fatalf("add by email: %v %+v", err, s)
	}
	expectCalls(t, f, "updateWikiSharing restricted [3,4]")

	f = setup()
	if _, _, err := updateWikiSharing(ctx, f.client(t), updateSharingIn{PageID: "50", Visibility: "restricted", UserIDs: []int64{4}, Mode: "replace"}); err != nil {
		t.Fatal(err)
	}
	expectCalls(t, f, "updateWikiSharing restricted [4]")

	f = setup()
	_, _, _ = updateWikiSharing(ctx, f.client(t), updateSharingIn{PageID: "50", Visibility: "project"})
	expectCalls(t, f, "updateWikiSharing project [3]")

	f = setup()
	_, _, _ = updateWikiSharing(ctx, f.client(t), updateSharingIn{PageID: "50", Visibility: "restricted", Mode: "replace"})
	expectCalls(t, f, "updateWikiSharing restricted []")

	f = newFakeTeam()
	f.project = []member{{UserID: 3, Email: "amy@example.com"}}
	_, _, err = updateWikiSharing(ctx, f.client(t), updateSharingIn{PageID: "50", Visibility: "restricted", Emails: []string{"amy@example.com", "gary@example.com"}})
	if err == nil || !strings.Contains(err.Error(), "Not members of project 2: gary@example.com") || !strings.Contains(err.Error(), "add_project_member") {
		t.Fatalf("non-member: %v", err)
	}
	expectCalls(t, f)

	f = newFakeTeam()
	f.sharing.CanManage = false
	if _, _, err := updateWikiSharing(ctx, f.client(t), updateSharingIn{PageID: "50", Visibility: "restricted", Emails: []string{"amy@example.com"}}); err == nil {
		t.Fatal("a viewer changed sharing")
	}
	expectCalls(t, f)
}

func TestAddProjectMember(t *testing.T) {
	ctx := context.Background()

	f := newFakeTeam()
	f.team = []teamMember{{UserID: 9, Email: "gary@example.com"}}
	r, err := addProjectMember(ctx, f.client(t), addProjectMemberIn{ProjectID: "2", Email: "Gary@Example.com", Role: "editor"})
	if err != nil || r["status"] != "added" || r["added_to_team"] != false || !reflect.DeepEqual(r["member"], map[string]any{"user_id": int64(9), "email": "gary@example.com", "name": nil, "role": "editor"}) {
		t.Fatalf("add team member: %v %v", err, r)
	}
	// The email as stored, since the API matches it exactly.
	expectCalls(t, f, "addProjectMember gary@example.com editor")

	f = newFakeTeam()
	f.project = []member{{UserID: 9, Email: "gary@example.com", Role: "viewer"}}
	r, _ = addProjectMember(ctx, f.client(t), addProjectMemberIn{ProjectID: "2", UserID: 9, Role: "owner"})
	if r["status"] != "already_member" || r["member"].(map[string]any)["role"] != "viewer" {
		t.Fatalf("existing member: %v", r)
	}
	expectCalls(t, f)

	f = newFakeTeam()
	f.registered["gary@example.com"] = 9
	if _, err := addProjectMember(ctx, f.client(t), addProjectMemberIn{ProjectID: "2", Email: "gary@example.com"}); err == nil || !strings.Contains(err.Error(), "add_to_team=true") {
		t.Fatalf("outside the team: %v", err)
	}
	expectCalls(t, f)

	r, err = addProjectMember(ctx, f.client(t), addProjectMemberIn{ProjectID: "2", Email: "gary@example.com", AddToTeam: true})
	if err != nil || r["status"] != "added" || r["added_to_team"] != true {
		t.Fatalf("add to team by email: %v %v", err, r)
	}
	expectCalls(t, f, "inviteTeamMember gary@example.com", "addProjectMember gary@example.com member")

	f = newFakeTeam()
	f.registered["gary@example.com"] = 9
	if r, err = addProjectMember(ctx, f.client(t), addProjectMemberIn{ProjectID: "2", UserID: 9, AddToTeam: true}); err != nil || r["status"] != "added" {
		t.Fatalf("add to team by id: %v %v", err, r)
	}
	expectCalls(t, f, "addTeamMember 9", "addProjectMember gary@example.com member")

	f = newFakeTeam()
	r, err = addProjectMember(ctx, f.client(t), addProjectMemberIn{ProjectID: "2", Email: "new@example.com", AddToTeam: true})
	if err != nil || r["status"] != "team_invitation_pending" || !strings.Contains(r["message"].(string), "NOT in project 2 yet") {
		t.Fatalf("pending invitation: %v %v", err, r)
	}
	expectCalls(t, f, "inviteTeamMember new@example.com")

	for _, in := range []addProjectMemberIn{{ProjectID: "2"}, {ProjectID: "2", Email: "a@b.co", UserID: 1}} {
		if _, err := addProjectMember(ctx, f.client(t), in); err == nil || !strings.Contains(err.Error(), "exactly one") {
			t.Fatalf("needs one of email or user_id: %v", err)
		}
	}
}
