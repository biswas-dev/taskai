package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// createTestTeam creates a team and team_member (owner) row for the given user, returns teamID
func createTestTeam(t *testing.T, ts *TestServer, ownerID int64, teamName string) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO teams (name, owner_id, created_at, updated_at) VALUES (?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		teamName, ownerID,
	)
	if err != nil {
		t.Fatalf("Failed to create test team: %v", err)
	}

	teamID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get team ID: %v", err)
	}

	// Add the owner as an active team member
	_, err = ts.DB.ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id, role, status, joined_at) VALUES (?, ?, 'owner', 'active', CURRENT_TIMESTAMP)`,
		teamID, ownerID,
	)
	if err != nil {
		t.Fatalf("Failed to add owner to team: %v", err)
	}

	return teamID
}

// addTeamMember adds a user to a team with the specified role
func addTeamMember(t *testing.T, ts *TestServer, teamID, userID int64, role string) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO team_members (team_id, user_id, role, status, joined_at) VALUES (?, ?, ?, 'active', CURRENT_TIMESTAMP)`,
		teamID, userID, role,
	)
	if err != nil {
		t.Fatalf("Failed to add team member: %v", err)
	}

	memberID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get member ID: %v", err)
	}

	return memberID
}

// createTestTeamInvitation creates a pending invitation and returns the invitation ID
func createTestTeamInvitation(t *testing.T, ts *TestServer, teamID, inviterID int64, inviteeEmail string, inviteeID *int64) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO team_invitations (team_id, inviter_id, invitee_email, invitee_id, status, created_at)
		 VALUES (?, ?, ?, ?, 'pending', CURRENT_TIMESTAMP)`,
		teamID, inviterID, inviteeEmail, inviteeID,
	)
	if err != nil {
		t.Fatalf("Failed to create test team invitation: %v", err)
	}

	invID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get invitation ID: %v", err)
	}

	return invID
}

// createTestTeamInvitationWithToken creates a pending invitation with an acceptance token
func createTestTeamInvitationWithToken(t *testing.T, ts *TestServer, teamID, inviterID int64, inviteeEmail string, inviteeID *int64, token string, expiresIn string) int64 {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.DB.ExecContext(ctx,
		fmt.Sprintf(`INSERT INTO team_invitations (team_id, inviter_id, invitee_email, invitee_id, status, created_at, acceptance_token, token_expires_at)
		 VALUES (?, ?, ?, ?, 'pending', CURRENT_TIMESTAMP, ?, datetime('now', '%s'))`, expiresIn),
		teamID, inviterID, inviteeEmail, inviteeID, token,
	)
	if err != nil {
		t.Fatalf("Failed to create test team invitation with token: %v", err)
	}

	invID, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("Failed to get invitation ID: %v", err)
	}

	return invID
}

func TestHandleGetMyTeam(t *testing.T) {
	tests := []struct {
		name       string
		setup      func(t *testing.T, ts *TestServer) int64
		wantStatus int
		wantError  string
	}{
		{
			name: "user has a team",
			setup: func(t *testing.T, ts *TestServer) int64 {
				userID := ts.CreateTestUser(t, "owner@example.com", "password123")
				createTestTeam(t, ts, userID, "My Team")
				return userID
			},
			wantStatus: http.StatusOK,
		},
		{
			name: "user has no team",
			setup: func(t *testing.T, ts *TestServer) int64 {
				return ts.CreateTestUser(t, "lonely@example.com", "password123")
			},
			wantStatus: http.StatusNotFound,
			wantError:  "no active team found",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID := tt.setup(t, ts)

			rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/team", nil, userID, nil)
			ts.HandleGetMyTeam(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			if tt.wantError != "" {
				AssertError(t, rec, tt.wantStatus, tt.wantError, "")
			} else {
				var team Team
				DecodeJSON(t, rec, &team)

				if team.ID == 0 {
					t.Error("Expected team ID to be set")
				}
				if team.Name != "My Team" {
					t.Errorf("Expected team name 'My Team', got %q", team.Name)
				}
				if team.OwnerID != userID {
					t.Errorf("Expected owner ID %d, got %d", userID, team.OwnerID)
				}
			}
		})
	}
}

func TestHandleGetTeamMembers(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	memberID := ts.CreateTestUser(t, "member@example.com", "password123")

	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	addTeamMember(t, ts, teamID, memberID, "member")

	rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/team/members", nil, ownerID, nil)
	ts.HandleGetTeamMembers(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var members []TeamMember
	DecodeJSON(t, rec, &members)

	if len(members) != 2 {
		t.Fatalf("Expected 2 members, got %d", len(members))
	}

	// Members should be ordered by role DESC then joined_at ASC
	// owner comes first alphabetically DESC
	foundOwner := false
	foundMember := false
	for _, m := range members {
		if m.Role == "owner" && m.UserID == ownerID {
			foundOwner = true
		}
		if m.Role == "member" && m.UserID == memberID {
			foundMember = true
		}
	}

	if !foundOwner {
		t.Error("Expected to find owner in team members")
	}
	if !foundMember {
		t.Error("Expected to find member in team members")
	}
}

func TestHandleGetTeamMembersNoTeam(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "lonely@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/team/members", nil, userID, nil)
	ts.HandleGetTeamMembers(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusNotFound)
}

func TestHandleInviteTeamMember(t *testing.T) {
	tests := []struct {
		name           string
		setup          func(t *testing.T, ts *TestServer) (inviterID int64)
		email          string
		wantStatus     int
		wantInvStatus  string // expected invitation status field; defaults to "pending"
		wantError      string
		wantErrorCode  string
	}{
		{
			name: "happy path - invite new user",
			setup: func(t *testing.T, ts *TestServer) int64 {
				ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
				createTestTeam(t, ts, ownerID, "Test Team")
				return ownerID
			},
			email:         "newmember@example.com",
			wantStatus:    http.StatusCreated,
			wantInvStatus: "pending",
		},
		{
			name: "invite existing user",
			setup: func(t *testing.T, ts *TestServer) int64 {
				ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
				createTestTeam(t, ts, ownerID, "Test Team")
				// Create the invitee as an existing user
				ts.CreateTestUser(t, "existing@example.com", "password123")
				return ownerID
			},
			email:         "existing@example.com",
			wantStatus:    http.StatusCreated,
			wantInvStatus: "accepted",
		},
		{
			name: "invalid email",
			setup: func(t *testing.T, ts *TestServer) int64 {
				ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
				createTestTeam(t, ts, ownerID, "Test Team")
				return ownerID
			},
			email:         "notanemail",
			wantStatus:    http.StatusBadRequest,
			wantError:     "valid email is required",
			wantErrorCode: "invalid_input",
		},
		{
			name: "empty email",
			setup: func(t *testing.T, ts *TestServer) int64 {
				ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
				createTestTeam(t, ts, ownerID, "Test Team")
				return ownerID
			},
			email:         "",
			wantStatus:    http.StatusBadRequest,
			wantError:     "valid email is required",
			wantErrorCode: "invalid_input",
		},
		{
			name: "non-owner tries to invite",
			setup: func(t *testing.T, ts *TestServer) int64 {
				ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
				memberUserID := ts.CreateTestUser(t, "member@example.com", "password123")
				teamID := createTestTeam(t, ts, ownerID, "Test Team")
				addTeamMember(t, ts, teamID, memberUserID, "member")
				return memberUserID
			},
			email:         "invitee@example.com",
			wantStatus:    http.StatusForbidden,
			wantError:     "only team owners and admins can invite",
			wantErrorCode: "forbidden",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			inviterID := tt.setup(t, ts)

			body := InviteTeamMemberRequest{Email: tt.email}
			rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invite", body, inviterID, nil)
			ts.HandleInviteTeamMember(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			if tt.wantError != "" {
				AssertError(t, rec, tt.wantStatus, tt.wantError, tt.wantErrorCode)
			} else {
				var inv TeamInvitation
				DecodeJSON(t, rec, &inv)

				if inv.ID == 0 {
					t.Error("Expected invitation ID to be set")
				}
				if inv.InviteeEmail != tt.email {
					t.Errorf("Expected invitee email %q, got %q", tt.email, inv.InviteeEmail)
				}
				wantInvStatus := tt.wantInvStatus
				if wantInvStatus == "" {
					wantInvStatus = "pending"
				}
				if inv.Status != wantInvStatus {
					t.Errorf("Expected invitation status %q, got %q", wantInvStatus, inv.Status)
				}
			}
		})
	}
}

func TestHandleInviteTeamMemberDuplicate(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	existingMemberID := ts.CreateTestUser(t, "member@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	addTeamMember(t, ts, teamID, existingMemberID, "member")

	// Try to invite someone who is already a member
	body := InviteTeamMemberRequest{Email: "member@example.com"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invite", body, ownerID, nil)
	ts.HandleInviteTeamMember(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusConflict)
	AssertError(t, rec, http.StatusConflict, "already a team member", "already_member")
}

func TestHandleInviteTeamMemberDuplicatePending(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")

	// Create a pending invitation first
	createTestTeamInvitation(t, ts, teamID, ownerID, "invitee@example.com", nil)

	// Try to invite the same email again
	body := InviteTeamMemberRequest{Email: "invitee@example.com"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invite", body, ownerID, nil)
	ts.HandleInviteTeamMember(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusConflict)
	AssertError(t, rec, http.StatusConflict, "pending invitation already exists", "invitation_exists")
}

func TestHandleRemoveTeamMember(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	memberUserID := ts.CreateTestUser(t, "member@example.com", "password123")

	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	memberRowID := addTeamMember(t, ts, teamID, memberUserID, "member")

	rec, req := ts.MakeAuthRequest(t, http.MethodDelete, fmt.Sprintf("/api/team/members/%d", memberRowID), nil, ownerID,
		map[string]string{"memberId": fmt.Sprintf("%d", memberRowID)})

	ts.HandleRemoveTeamMember(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var resp map[string]string
	DecodeJSON(t, rec, &resp)
	if resp["message"] != "member removed" {
		t.Errorf("Expected message 'member removed', got %q", resp["message"])
	}

	// Verify member was removed from DB
	var count int
	err := ts.DB.QueryRow("SELECT COUNT(*) FROM team_members WHERE id = ?", memberRowID).Scan(&count)
	if err != nil {
		t.Fatalf("Failed to query member count: %v", err)
	}
	if count != 0 {
		t.Error("Team member was not removed from database")
	}
}

func TestHandleRemoveTeamMemberCannotRemoveOwner(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")

	// Get the owner's team_member row ID
	var ownerMemberRowID int64
	err := ts.DB.QueryRow("SELECT id FROM team_members WHERE team_id = ? AND user_id = ?", teamID, ownerID).Scan(&ownerMemberRowID)
	if err != nil {
		t.Fatalf("Failed to get owner member row ID: %v", err)
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodDelete, fmt.Sprintf("/api/team/members/%d", ownerMemberRowID), nil, ownerID,
		map[string]string{"memberId": fmt.Sprintf("%d", ownerMemberRowID)})

	ts.HandleRemoveTeamMember(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusForbidden)
	AssertError(t, rec, http.StatusForbidden, "cannot remove team owner", "forbidden")
}

func TestHandleRemoveTeamMemberNonOwner(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	member1ID := ts.CreateTestUser(t, "member1@example.com", "password123")
	member2ID := ts.CreateTestUser(t, "member2@example.com", "password123")

	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	addTeamMember(t, ts, teamID, member1ID, "member")
	member2RowID := addTeamMember(t, ts, teamID, member2ID, "member")

	// member1 (not owner/admin) tries to remove member2
	rec, req := ts.MakeAuthRequest(t, http.MethodDelete, fmt.Sprintf("/api/team/members/%d", member2RowID), nil, member1ID,
		map[string]string{"memberId": fmt.Sprintf("%d", member2RowID)})

	ts.HandleRemoveTeamMember(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusForbidden)
	AssertError(t, rec, http.StatusForbidden, "only team owners and admins can remove", "forbidden")
}

func TestHandleGetMyInvitations(t *testing.T) {
	tests := []struct {
		name            string
		setup           func(t *testing.T, ts *TestServer) (userID int64, email string)
		wantStatus      int
		wantInviteCount int
	}{
		{
			name: "no pending invitations",
			setup: func(t *testing.T, ts *TestServer) (int64, string) {
				userID := ts.CreateTestUser(t, "user@example.com", "password123")
				return userID, "user@example.com"
			},
			wantStatus:      http.StatusOK,
			wantInviteCount: 0,
		},
		{
			name: "with pending invitations",
			setup: func(t *testing.T, ts *TestServer) (int64, string) {
				ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
				inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")

				teamID := createTestTeam(t, ts, ownerID, "Test Team")
				createTestTeamInvitation(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID)

				return inviteeID, "invitee@example.com"
			},
			wantStatus:      http.StatusOK,
			wantInviteCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			userID, email := tt.setup(t, ts)

			rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/team/invitations", nil, userID, nil)
			// Add email to context since HandleGetMyInvitations requires it
			ctx := context.WithValue(req.Context(), UserEmailKey, email)
			req = req.WithContext(ctx)

			ts.HandleGetMyInvitations(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			var invitations []TeamInvitation
			DecodeJSON(t, rec, &invitations)

			if len(invitations) != tt.wantInviteCount {
				t.Errorf("Expected %d invitations, got %d", tt.wantInviteCount, len(invitations))
			}
		})
	}
}

func TestHandleAcceptInvitation(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")

	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	invitationID := createTestTeamInvitation(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID)

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/team/invitations/%d/accept", invitationID), nil, inviteeID,
		map[string]string{"id": fmt.Sprintf("%d", invitationID)})
	// Add email to context
	ctx := context.WithValue(req.Context(), UserEmailKey, "invitee@example.com")
	req = req.WithContext(ctx)

	ts.HandleAcceptInvitation(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var resp map[string]string
	DecodeJSON(t, rec, &resp)
	if resp["message"] != "invitation accepted" {
		t.Errorf("Expected message 'invitation accepted', got %q", resp["message"])
	}

	// Verify invitation status was updated
	var status string
	err := ts.DB.QueryRow("SELECT status FROM team_invitations WHERE id = ?", invitationID).Scan(&status)
	if err != nil {
		t.Fatalf("Failed to query invitation status: %v", err)
	}
	if status != "accepted" {
		t.Errorf("Expected invitation status 'accepted', got %q", status)
	}

	// Verify user was added to team
	var memberCount int
	err = ts.DB.QueryRow("SELECT COUNT(*) FROM team_members WHERE team_id = ? AND user_id = ?", teamID, inviteeID).Scan(&memberCount)
	if err != nil {
		t.Fatalf("Failed to query team member: %v", err)
	}
	if memberCount != 1 {
		t.Errorf("Expected invitee to be a team member, count=%d", memberCount)
	}
}

func TestHandleAcceptInvitationNotFound(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invitations/99999/accept", nil, userID,
		map[string]string{"id": "99999"})
	ctx := context.WithValue(req.Context(), UserEmailKey, "user@example.com")
	req = req.WithContext(ctx)

	ts.HandleAcceptInvitation(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusNotFound)
	AssertError(t, rec, http.StatusNotFound, "invitation not found", "not_found")
}

func TestHandleAcceptInvitationAlreadyResponded(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")

	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	invitationID := createTestTeamInvitation(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID)

	// Mark invitation as already accepted
	_, err := ts.DB.Exec("UPDATE team_invitations SET status = 'accepted' WHERE id = ?", invitationID)
	if err != nil {
		t.Fatalf("Failed to update invitation: %v", err)
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/team/invitations/%d/accept", invitationID), nil, inviteeID,
		map[string]string{"id": fmt.Sprintf("%d", invitationID)})
	ctx := context.WithValue(req.Context(), UserEmailKey, "invitee@example.com")
	req = req.WithContext(ctx)

	ts.HandleAcceptInvitation(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusConflict)
	AssertError(t, rec, http.StatusConflict, "already responded", "already_responded")
}

func TestHandleRejectInvitation(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")

	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	invitationID := createTestTeamInvitation(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID)

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/team/invitations/%d/reject", invitationID), nil, inviteeID,
		map[string]string{"id": fmt.Sprintf("%d", invitationID)})
	ctx := context.WithValue(req.Context(), UserEmailKey, "invitee@example.com")
	req = req.WithContext(ctx)

	ts.HandleRejectInvitation(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var resp map[string]string
	DecodeJSON(t, rec, &resp)
	if resp["message"] != "invitation rejected" {
		t.Errorf("Expected message 'invitation rejected', got %q", resp["message"])
	}

	// Verify invitation status was updated
	var status string
	err := ts.DB.QueryRow("SELECT status FROM team_invitations WHERE id = ?", invitationID).Scan(&status)
	if err != nil {
		t.Fatalf("Failed to query invitation status: %v", err)
	}
	if status != "rejected" {
		t.Errorf("Expected invitation status 'rejected', got %q", status)
	}
}

func TestHandleRejectInvitationNotForUser(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")
	otherID := ts.CreateTestUser(t, "other@example.com", "password123")

	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	invitationID := createTestTeamInvitation(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID)

	// A different user tries to reject the invitation
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/team/invitations/%d/reject", invitationID), nil, otherID,
		map[string]string{"id": fmt.Sprintf("%d", invitationID)})
	ctx := context.WithValue(req.Context(), UserEmailKey, "other@example.com")
	req = req.WithContext(ctx)

	ts.HandleRejectInvitation(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusForbidden)
	AssertError(t, rec, http.StatusForbidden, "invitation is not for you", "forbidden")
}

func TestHandleRejectInvitationAlreadyResponded(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")

	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	invitationID := createTestTeamInvitation(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID)

	// Mark invitation as already rejected
	_, err := ts.DB.Exec("UPDATE team_invitations SET status = 'rejected' WHERE id = ?", invitationID)
	if err != nil {
		t.Fatalf("Failed to update invitation: %v", err)
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/team/invitations/%d/reject", invitationID), nil, inviteeID,
		map[string]string{"id": fmt.Sprintf("%d", invitationID)})
	ctx := context.WithValue(req.Context(), UserEmailKey, "invitee@example.com")
	req = req.WithContext(ctx)

	ts.HandleRejectInvitation(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusConflict)
	AssertError(t, rec, http.StatusConflict, "already responded", "already_responded")
}

func TestHandleAcceptInvitationDoesNotAddToTeamProjects(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")

	teamID := createTestTeam(t, ts, ownerID, "Test Team")

	// Create a project that belongs to this team
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := ts.DB.ExecContext(ctx,
		`INSERT INTO projects (owner_id, name, description, team_id) VALUES (?, ?, ?, ?)`,
		ownerID, "Team Project", "A project", teamID,
	)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	projectID, _ := result.LastInsertId()

	// Add owner as project member
	_, err = ts.DB.ExecContext(ctx,
		`INSERT INTO project_members (project_id, user_id, role, granted_by) VALUES (?, ?, 'owner', ?)`,
		projectID, ownerID, ownerID,
	)
	if err != nil {
		t.Fatalf("Failed to add project member: %v", err)
	}

	invitationID := createTestTeamInvitation(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID)

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, fmt.Sprintf("/api/team/invitations/%d/accept", invitationID), nil, inviteeID,
		map[string]string{"id": fmt.Sprintf("%d", invitationID)})
	rctx := context.WithValue(req.Context(), UserEmailKey, "invitee@example.com")
	req = req.WithContext(rctx)

	ts.HandleAcceptInvitation(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	// Verify user was NOT added to the team project (explicit access only)
	var projectMemberCount int
	err = ts.DB.QueryRow("SELECT COUNT(*) FROM project_members WHERE project_id = ? AND user_id = ?", projectID, inviteeID).Scan(&projectMemberCount)
	if err != nil {
		t.Fatalf("Failed to query project member: %v", err)
	}
	if projectMemberCount != 0 {
		t.Errorf("Expected invitee NOT to be a project member after accepting team invitation, count=%d", projectMemberCount)
	}
}

func TestIsValidEmail(t *testing.T) {
	tests := []struct {
		email string
		valid bool
	}{
		{"user@example.com", true},
		{"a@b.c", true},
		{"user@domain", true},
		{"", false},
		{"ab", false},
		{"noatsign", false},
		{"@nolocalpart", false},
		{"trailingat@", false},
		{"two@@ats", false},
	}

	for _, tt := range tests {
		t.Run(tt.email, func(t *testing.T) {
			got := isValidEmail(tt.email)
			if got != tt.valid {
				t.Errorf("isValidEmail(%q) = %v, want %v", tt.email, got, tt.valid)
			}
		})
	}
}

// Ensure the test file does not break JSON decoding by verifying empty array responses
func TestHandleGetMyInvitationsEmptyResponse(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "nobody@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/team/invitations", nil, userID, nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "nobody@example.com")
	req = req.WithContext(ctx)

	ts.HandleGetMyInvitations(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var raw json.RawMessage
	DecodeJSON(t, rec, &raw)

	var invitations []TeamInvitation
	if err := json.Unmarshal(raw, &invitations); err != nil {
		t.Fatalf("Failed to unmarshal invitations: %v", err)
	}

	if len(invitations) != 0 {
		t.Errorf("Expected 0 invitations, got %d", len(invitations))
	}
}

func TestHandleInviteTeamMember_InvalidBody(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "owner@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/teams/invitations", "not-json", userID, nil)
	ts.HandleInviteTeamMember(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "invalid request body", "invalid_input")
}

func TestHandleInviteTeamMember_EmptyEmail(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "owner@example.com", "password123")

	body := InviteTeamMemberRequest{Email: ""}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/teams/invitations", body, userID, nil)
	ts.HandleInviteTeamMember(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "valid email is required", "invalid_input")
}

func TestHandleInviteTeamMember_InvalidEmail(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "owner@example.com", "password123")

	body := InviteTeamMemberRequest{Email: "notanemail"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/teams/invitations", body, userID, nil)
	ts.HandleInviteTeamMember(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "valid email is required", "invalid_input")
}

func TestHandleInviteTeamMember_NoTeam(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create user without a team
	userID := ts.CreateTestUser(t, "orphan@example.com", "password123")

	body := InviteTeamMemberRequest{Email: "invitee@example.com"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/teams/invitations", body, userID, nil)
	ts.HandleInviteTeamMember(rec, req)

	AssertError(t, rec, http.StatusNotFound, "no active team found", "not_found")
}

func TestHandleRemoveTeamMember_InvalidMemberID(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "owner@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/teams/members/abc", nil, userID, map[string]string{"memberId": "abc"})
	ts.HandleRemoveTeamMember(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "invalid member ID", "invalid_input")
}

func TestHandleRemoveTeamMember_NoTeam(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "orphan@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/teams/members/1", nil, userID, map[string]string{"memberId": "1"})
	ts.HandleRemoveTeamMember(rec, req)

	AssertError(t, rec, http.StatusNotFound, "no active team found", "not_found")
}

func TestHandleAcceptInvitation_InvalidID(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/teams/invitations/abc/accept", nil, userID, map[string]string{"id": "abc"})
	ctx := context.WithValue(req.Context(), UserEmailKey, "user@example.com")
	req = req.WithContext(ctx)
	ts.HandleAcceptInvitation(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "invalid invitation ID", "invalid_input")
}

func TestHandleRejectInvitation_InvalidID(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/teams/invitations/abc/reject", nil, userID, map[string]string{"id": "abc"})
	ctx := context.WithValue(req.Context(), UserEmailKey, "user@example.com")
	req = req.WithContext(ctx)
	ts.HandleRejectInvitation(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "invalid invitation ID", "invalid_input")
}

func TestHandleRejectInvitation_NotFound(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/teams/invitations/99999/reject", nil, userID, map[string]string{"id": "99999"})
	ctx := context.WithValue(req.Context(), UserEmailKey, "user@example.com")
	req = req.WithContext(ctx)
	ts.HandleRejectInvitation(rec, req)

	AssertError(t, rec, http.StatusNotFound, "invitation not found", "not_found")
}

func TestHandleAcceptInvitation_NotFound(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/teams/invitations/99999/accept", nil, userID, map[string]string{"id": "99999"})
	ctx := context.WithValue(req.Context(), UserEmailKey, "user@example.com")
	req = req.WithContext(ctx)
	ts.HandleAcceptInvitation(rec, req)

	AssertError(t, rec, http.StatusNotFound, "invitation not found", "not_found")
}

func TestHandleGetMyTeam_NoTeam(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "orphan@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/teams/my", nil, userID, nil)
	ts.HandleGetMyTeam(rec, req)

	AssertError(t, rec, http.StatusNotFound, "no active team found", "not_found")
}

func TestHandleGetTeamMembers_NoTeam(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "orphan@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/teams/members", nil, userID, nil)
	ts.HandleGetTeamMembers(rec, req)

	AssertError(t, rec, http.StatusNotFound, "no active team found", "not_found")
}

func TestHandleRemoveTeamMember_MemberNotFound(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	createTestTeam(t, ts, ownerID, "Test Team")

	rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/teams/members/99999", nil, ownerID, map[string]string{"memberId": "99999"})
	ts.HandleRemoveTeamMember(rec, req)

	AssertError(t, rec, http.StatusNotFound, "member not found", "not_found")
}

// --- Token-based acceptance tests ---

func TestHandleGetInvitationByToken_MissingToken(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	rec, req := MakeRequest(t, http.MethodGet, "/api/team/invitations/by-token", nil, nil)
	ts.HandleGetInvitationByToken(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "token is required", "invalid_input")
}

func TestHandleGetInvitationByToken_InvalidToken(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	rec, req := MakeRequest(t, http.MethodGet, "/api/team/invitations/by-token?token=invalid123", nil, nil)
	ts.HandleGetInvitationByToken(rec, req)

	AssertError(t, rec, http.StatusNotFound, "invitation not found or token is invalid", "not_found")
}

func TestHandleGetInvitationByToken_ExpiredToken(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	createTestTeamInvitationWithToken(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID, "expired-token", "-1 day")

	rec, req := MakeRequest(t, http.MethodGet, "/api/team/invitations/by-token?token=expired-token", nil, nil)
	ts.HandleGetInvitationByToken(rec, req)

	AssertError(t, rec, http.StatusGone, "invitation link has expired", "token_expired")
}

func TestHandleGetInvitationByToken_AlreadyAccepted(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	invID := createTestTeamInvitationWithToken(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID, "accepted-token", "+7 days")

	// Mark as accepted
	ctx := context.Background()
	ts.DB.ExecContext(ctx, `UPDATE team_invitations SET status = 'accepted' WHERE id = ?`, invID)

	rec, req := MakeRequest(t, http.MethodGet, "/api/team/invitations/by-token?token=accepted-token", nil, nil)
	ts.HandleGetInvitationByToken(rec, req)

	AssertError(t, rec, http.StatusConflict, "invitation has already been accepted", "already_responded")
}

func TestHandleGetInvitationByToken_ValidExistingUser(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	createTestTeamInvitationWithToken(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID, "valid-token-123", "+7 days")

	rec, req := MakeRequest(t, http.MethodGet, "/api/team/invitations/by-token?token=valid-token-123", nil, nil)
	ts.HandleGetInvitationByToken(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var resp TokenInvitationResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	if resp.TeamName != "Test Team" {
		t.Errorf("Expected team name 'Test Team', got '%s'", resp.TeamName)
	}
	if resp.InviteeEmail != "invitee@example.com" {
		t.Errorf("Expected invitee email 'invitee@example.com', got '%s'", resp.InviteeEmail)
	}
	if resp.RequiresSignup {
		t.Error("Expected requires_signup to be false for existing user")
	}
	if resp.Status != "pending" {
		t.Errorf("Expected status 'pending', got '%s'", resp.Status)
	}
}

func TestHandleGetInvitationByToken_NewUserWithInviteCode(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	invID := createTestTeamInvitationWithToken(t, ts, teamID, ownerID, "newuser@example.com", nil, "new-user-token", "+7 days")

	// Set invite code on the invitation
	ctx := context.Background()
	ts.DB.ExecContext(ctx, `UPDATE team_invitations SET invite_code = 'SIGNUPCODE123' WHERE id = ?`, invID)

	rec, req := MakeRequest(t, http.MethodGet, "/api/team/invitations/by-token?token=new-user-token", nil, nil)
	ts.HandleGetInvitationByToken(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var resp TokenInvitationResponse
	json.NewDecoder(rec.Body).Decode(&resp)

	if !resp.RequiresSignup {
		t.Error("Expected requires_signup to be true for new user")
	}
	if resp.InviteCode != "SIGNUPCODE123" {
		t.Errorf("Expected invite_code 'SIGNUPCODE123', got '%s'", resp.InviteCode)
	}
}

func TestHandleAcceptInvitationByToken_MissingToken(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invitations/accept-by-token", map[string]string{}, userID, nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "user@example.com")
	req = req.WithContext(ctx)
	ts.HandleAcceptInvitationByToken(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "token is required", "invalid_input")
}

func TestHandleAcceptInvitationByToken_InvalidToken(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "user@example.com", "password123")

	body := struct{ Token string }{Token: "nonexistent-token"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invitations/accept-by-token", body, userID, nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "user@example.com")
	req = req.WithContext(ctx)
	ts.HandleAcceptInvitationByToken(rec, req)

	AssertError(t, rec, http.StatusNotFound, "invitation not found or token is invalid", "not_found")
}

func TestHandleAcceptInvitationByToken_ExpiredToken(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	createTestTeamInvitationWithToken(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID, "expired-accept-token", "-1 day")

	body := struct{ Token string }{Token: "expired-accept-token"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invitations/accept-by-token", body, inviteeID, nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "invitee@example.com")
	req = req.WithContext(ctx)
	ts.HandleAcceptInvitationByToken(rec, req)

	AssertError(t, rec, http.StatusGone, "invitation link has expired", "token_expired")
}

func TestHandleAcceptInvitationByToken_WrongEmail(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")
	wrongUserID := ts.CreateTestUser(t, "wrong@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	createTestTeamInvitationWithToken(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID, "wrong-email-token", "+7 days")

	body := struct{ Token string }{Token: "wrong-email-token"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invitations/accept-by-token", body, wrongUserID, nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "wrong@example.com")
	req = req.WithContext(ctx)
	ts.HandleAcceptInvitationByToken(rec, req)

	AssertError(t, rec, http.StatusForbidden, "this invitation is for a different email address", "forbidden")
}

func TestHandleAcceptInvitationByToken_AlreadyAccepted(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	invID := createTestTeamInvitationWithToken(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID, "already-accepted-token", "+7 days")

	ctx := context.Background()
	ts.DB.ExecContext(ctx, `UPDATE team_invitations SET status = 'accepted' WHERE id = ?`, invID)

	body := struct{ Token string }{Token: "already-accepted-token"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invitations/accept-by-token", body, inviteeID, nil)
	rctx := context.WithValue(req.Context(), UserEmailKey, "invitee@example.com")
	req = req.WithContext(rctx)
	ts.HandleAcceptInvitationByToken(rec, req)

	AssertError(t, rec, http.StatusConflict, "invitation has already been accepted", "already_responded")
}

func TestHandleAcceptInvitationByToken_Success(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	createTestTeamInvitationWithToken(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID, "success-token", "+7 days")

	body := struct{ Token string }{Token: "success-token"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invitations/accept-by-token", body, inviteeID, nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "invitee@example.com")
	req = req.WithContext(ctx)
	ts.HandleAcceptInvitationByToken(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var resp map[string]string
	json.NewDecoder(rec.Body).Decode(&resp)
	if resp["message"] != "invitation accepted" {
		t.Errorf("Expected message 'invitation accepted', got '%s'", resp["message"])
	}

	// Verify team membership was created
	bgCtx := context.Background()
	var memberCount int
	ts.DB.QueryRowContext(bgCtx, `SELECT COUNT(*) FROM team_members WHERE team_id = ? AND user_id = ?`, teamID, inviteeID).Scan(&memberCount)
	if memberCount != 1 {
		t.Errorf("Expected 1 team member row, got %d", memberCount)
	}

	// Verify invitation was marked as accepted
	var status string
	ts.DB.QueryRowContext(bgCtx, `SELECT status FROM team_invitations WHERE acceptance_token = ?`, "success-token").Scan(&status)
	if status != "accepted" {
		t.Errorf("Expected invitation status 'accepted', got '%s'", status)
	}
}

func TestHandleAcceptInvitationByToken_DoesNotAddToProjects(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")

	// Create a project belonging to this team
	bgCtx := context.Background()
	projResult, _ := ts.DB.ExecContext(bgCtx,
		`INSERT INTO projects (name, owner_id, team_id, created_at, updated_at) VALUES ('Team Project', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		ownerID, teamID,
	)
	projID, _ := projResult.LastInsertId()

	createTestTeamInvitationWithToken(t, ts, teamID, ownerID, "invitee@example.com", &inviteeID, "project-token", "+7 days")

	body := struct{ Token string }{Token: "project-token"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invitations/accept-by-token", body, inviteeID, nil)
	ctx := context.WithValue(req.Context(), UserEmailKey, "invitee@example.com")
	req = req.WithContext(ctx)
	ts.HandleAcceptInvitationByToken(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	// Verify user was NOT added to the project (explicit access only)
	var projMemberCount int
	ts.DB.QueryRowContext(bgCtx, `SELECT COUNT(*) FROM project_members WHERE project_id = ? AND user_id = ?`, projID, inviteeID).Scan(&projMemberCount)
	if projMemberCount != 0 {
		t.Errorf("Expected 0 project member rows after accepting token invite, got %d", projMemberCount)
	}
}

func TestHandleInviteTeamMember_GeneratesToken(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")

	body := InviteTeamMemberRequest{Email: "newinvitee@example.com"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/teams/invitations", body, ownerID, nil)
	ts.HandleInviteTeamMember(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	// Verify acceptance_token was generated
	bgCtx := context.Background()
	var token string
	var tokenExpires time.Time
	err := ts.DB.QueryRowContext(bgCtx,
		`SELECT acceptance_token, token_expires_at FROM team_invitations WHERE team_id = ? AND invitee_email = ?`,
		teamID, "newinvitee@example.com",
	).Scan(&token, &tokenExpires)
	if err != nil {
		t.Fatalf("Failed to query invitation: %v", err)
	}
	if token == "" {
		t.Error("Expected acceptance_token to be generated, got empty")
	}
	if tokenExpires.Before(time.Now()) {
		t.Error("Expected token_expires_at to be in the future")
	}
}

func TestHandleAddTeamMemberDoesNotAddToProjects(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	newMemberID := ts.CreateTestUser(t, "newmember@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")

	// Create a project belonging to this team
	bgCtx := context.Background()
	projResult, err := ts.DB.ExecContext(bgCtx,
		`INSERT INTO projects (name, owner_id, team_id, created_at, updated_at) VALUES ('Team Project', ?, ?, CURRENT_TIMESTAMP, CURRENT_TIMESTAMP)`,
		ownerID, teamID,
	)
	if err != nil {
		t.Fatalf("Failed to create project: %v", err)
	}
	projID, _ := projResult.LastInsertId()

	// Add owner as project member
	_, err = ts.DB.ExecContext(bgCtx,
		`INSERT INTO project_members (project_id, user_id, role, granted_by) VALUES (?, ?, 'owner', ?)`,
		projID, ownerID, ownerID,
	)
	if err != nil {
		t.Fatalf("Failed to add project owner member: %v", err)
	}

	body := AddTeamMemberRequest{UserID: newMemberID}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/members", body, ownerID, nil)
	ts.HandleAddTeamMember(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	// Verify new member was NOT added to the project (explicit access only)
	var projMemberCount int
	ts.DB.QueryRowContext(bgCtx, `SELECT COUNT(*) FROM project_members WHERE project_id = ? AND user_id = ?`, projID, newMemberID).Scan(&projMemberCount)
	if projMemberCount != 0 {
		t.Errorf("Expected 0 project member rows after direct team add, got %d", projMemberCount)
	}
}
