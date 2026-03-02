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

func TestHandleListInvites(t *testing.T) {
	t.Run("empty list", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "inviter@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/invites", nil, userID, nil)
		ts.HandleListInvites(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp map[string]json.RawMessage
		DecodeJSON(t, rec, &resp)

		var invites []json.RawMessage
		if err := json.Unmarshal(resp["invites"], &invites); err != nil {
			t.Fatalf("Failed to unmarshal invites: %v", err)
		}

		if len(invites) != 0 {
			t.Errorf("Expected 0 invites, got %d", len(invites))
		}

		// Check invite_count is returned (default 3)
		var inviteCount float64
		if err := json.Unmarshal(resp["invite_count"], &inviteCount); err != nil {
			t.Fatalf("Failed to unmarshal invite_count: %v", err)
		}
		if inviteCount != 3 {
			t.Errorf("Expected invite_count 3, got %v", inviteCount)
		}
	})

	t.Run("with invites", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "inviter@example.com", "password123")

		// Create two invites
		ts.CreateTestInvite(t, userID)
		ts.CreateTestInvite(t, userID)

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/invites", nil, userID, nil)
		ts.HandleListInvites(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp map[string]json.RawMessage
		DecodeJSON(t, rec, &resp)

		var invites []json.RawMessage
		if err := json.Unmarshal(resp["invites"], &invites); err != nil {
			t.Fatalf("Failed to unmarshal invites: %v", err)
		}

		if len(invites) != 2 {
			t.Errorf("Expected 2 invites, got %d", len(invites))
		}
	})
}

func TestHandleCreateInvite(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "inviter@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/invites", nil, userID, nil)
		ts.HandleCreateInvite(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var resp map[string]interface{}
		DecodeJSON(t, rec, &resp)

		code, ok := resp["code"].(string)
		if !ok || code == "" {
			t.Errorf("Expected non-empty invite code, got %v", resp["code"])
		}

		expiresAt, ok := resp["expires_at"].(string)
		if !ok || expiresAt == "" {
			t.Errorf("Expected non-empty expires_at, got %v", resp["expires_at"])
		}

		// Verify invite count was decremented
		var inviteCount int
		err := ts.DB.QueryRow("SELECT invite_count FROM users WHERE id = ?", userID).Scan(&inviteCount)
		if err != nil {
			t.Fatalf("Failed to query invite count: %v", err)
		}
		if inviteCount != 2 {
			t.Errorf("Expected invite_count 2 after creating one invite, got %d", inviteCount)
		}
	})

	t.Run("out of invites", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "inviter@example.com", "password123")

		// Set invite_count to 0
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := ts.DB.ExecContext(ctx, `UPDATE users SET invite_count = 0 WHERE id = ?`, userID)
		if err != nil {
			t.Fatalf("Failed to update invite count: %v", err)
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/invites", nil, userID, nil)
		ts.HandleCreateInvite(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusForbidden)

		var errResp ErrorResponse
		DecodeJSON(t, rec, &errResp)

		if errResp.Error != "no invites remaining" {
			t.Errorf("Expected error 'no invites remaining', got '%s'", errResp.Error)
		}
		if errResp.Code != "no_invites" {
			t.Errorf("Expected code 'no_invites', got '%s'", errResp.Code)
		}
	})
}

func TestHandleValidateInvite(t *testing.T) {
	t.Run("valid code", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "inviter@example.com", "password123")
		code := ts.CreateTestInvite(t, userID)

		// ValidateInvite is a public endpoint - no auth required
		rec, req := MakeRequest(t, http.MethodGet, "/api/invites/validate?code="+code, nil, nil)
		ts.HandleValidateInvite(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var status InviteStatus
		DecodeJSON(t, rec, &status)

		if !status.Valid {
			t.Errorf("Expected invite to be valid, got invalid: %s", status.Message)
		}
	})

	t.Run("invalid code", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodGet, "/api/invites/validate?code=nonexistent-code", nil, nil)
		ts.HandleValidateInvite(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var status InviteStatus
		DecodeJSON(t, rec, &status)

		if status.Valid {
			t.Errorf("Expected invite to be invalid")
		}
		if status.Message != "invalid invite code" {
			t.Errorf("Expected message 'invalid invite code', got '%s'", status.Message)
		}
	})

	t.Run("used code", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "inviter@example.com", "password123")
		code := ts.CreateTestInvite(t, userID)

		// Mark the invite as used
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		_, err := ts.DB.ExecContext(ctx,
			`UPDATE invites SET used_at = CURRENT_TIMESTAMP, invitee_id = ? WHERE code = ?`,
			userID, code,
		)
		if err != nil {
			t.Fatalf("Failed to mark invite as used: %v", err)
		}

		rec, req := MakeRequest(t, http.MethodGet, "/api/invites/validate?code="+code, nil, nil)
		ts.HandleValidateInvite(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var status InviteStatus
		DecodeJSON(t, rec, &status)

		if status.Valid {
			t.Errorf("Expected used invite to be invalid")
		}
		if status.Message != "this invite has already been used" {
			t.Errorf("Expected message 'this invite has already been used', got '%s'", status.Message)
		}
	})

	t.Run("missing code parameter", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodGet, "/api/invites/validate", nil, nil)
		ts.HandleValidateInvite(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var status InviteStatus
		DecodeJSON(t, rec, &status)

		if status.Valid {
			t.Errorf("Expected invite to be invalid when no code provided")
		}
		if status.Message != "invite code is required" {
			t.Errorf("Expected message 'invite code is required', got '%s'", status.Message)
		}
	})
}

// makeAdmin sets a user as admin in the database
func makeAdmin(t *testing.T, ts *TestServer, userID int64) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ts.DB.ExecContext(ctx, `UPDATE users SET is_admin = 1 WHERE id = ?`, userID)
	if err != nil {
		t.Fatalf("Failed to make user admin: %v", err)
	}
}

func TestHandleListInvites_Unauthenticated(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	rec, req := MakeRequest(t, http.MethodGet, "/api/invites", nil, nil)
	ts.HandleListInvites(rec, req)

	AssertError(t, rec, http.StatusUnauthorized, "user not authenticated", "unauthorized")
}

func TestHandleCreateInvite_Unauthenticated(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	rec, req := MakeRequest(t, http.MethodPost, "/api/invites", nil, nil)
	ts.HandleCreateInvite(rec, req)

	AssertError(t, rec, http.StatusUnauthorized, "user not authenticated", "unauthorized")
}

func TestHandleCreateInvite_AdminUnlimited(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
	makeAdmin(t, ts, adminID)

	// Set invite_count to 0 — admin should still be able to create
	ctx := context.Background()
	_, err := ts.DB.ExecContext(ctx, `UPDATE users SET invite_count = 0 WHERE id = ?`, adminID)
	if err != nil {
		t.Fatalf("Failed to update invite count: %v", err)
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/invites", nil, adminID, nil)
	ts.HandleCreateInvite(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var resp map[string]interface{}
	DecodeJSON(t, rec, &resp)
	if resp["code"] == nil || resp["code"].(string) == "" {
		t.Error("Expected non-empty invite code from admin")
	}
}

func TestHandleValidateInvite_Expired(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	inviterID := ts.CreateTestUser(t, "inviter@example.com", "password123")
	code := ts.CreateTestInvite(t, inviterID)

	// Set expiry to the past
	ctx := context.Background()
	pastTime := time.Now().Add(-1 * time.Hour).Format(time.RFC3339)
	_, err := ts.DB.ExecContext(ctx, `UPDATE invites SET expires_at = ? WHERE code = ?`, pastTime, code)
	if err != nil {
		t.Fatalf("Failed to set expiry: %v", err)
	}

	rec, req := MakeRequest(t, http.MethodGet, "/api/invites/validate?code="+code, nil, nil)
	ts.HandleValidateInvite(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var status InviteStatus
	DecodeJSON(t, rec, &status)
	if status.Valid {
		t.Error("Expected expired invite to be invalid")
	}
	if status.Message != "this invite has expired" {
		t.Errorf("Expected 'this invite has expired', got %q", status.Message)
	}
}

func TestHandleValidateInvite_ValidWithInviterName(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	inviterID := ts.CreateTestUser(t, "inviter@example.com", "password123")

	// Set name for inviter
	ctx := context.Background()
	_, err := ts.DB.ExecContext(ctx, `UPDATE users SET name = 'John Doe' WHERE id = ?`, inviterID)
	if err != nil {
		t.Fatalf("Failed to update user name: %v", err)
	}

	code := ts.CreateTestInvite(t, inviterID)

	rec, req := MakeRequest(t, http.MethodGet, "/api/invites/validate?code="+code, nil, nil)
	ts.HandleValidateInvite(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var status InviteStatus
	DecodeJSON(t, rec, &status)
	if !status.Valid {
		t.Error("Expected valid invite")
	}
	if status.InviterName != "John Doe" {
		t.Errorf("Expected inviter name 'John Doe', got %q", status.InviterName)
	}
}

func TestHandleListInvites_WithUsedInvite(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	inviterID := ts.CreateTestUser(t, "inviter@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")
	code := ts.CreateTestInvite(t, inviterID)

	// Mark invite as used with an invitee
	ctx := context.Background()
	_, err := ts.DB.ExecContext(ctx,
		`UPDATE invites SET used_at = CURRENT_TIMESTAMP, invitee_id = ? WHERE code = ?`,
		inviteeID, code,
	)
	if err != nil {
		t.Fatalf("Failed to mark invite: %v", err)
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/invites", nil, inviterID, nil)
	ts.HandleListInvites(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var resp map[string]json.RawMessage
	DecodeJSON(t, rec, &resp)

	var invites []json.RawMessage
	if err := json.Unmarshal(resp["invites"], &invites); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}
	if len(invites) != 1 {
		t.Errorf("Expected 1 invite, got %d", len(invites))
	}
}

func TestHandleAdminBoostInvites(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func(*TestServer) (adminID int64, targetUserID int64)
		targetIDPath  string       // the {id} in the path, set from setupFunc if empty
		body          interface{}
		isAdmin       bool
		wantStatus    int
		wantError     string
		wantErrorCode string
		wantCount     int
		noAuth        bool
	}{
		{
			name: "admin boosts invites successfully",
			setupFunc: func(ts *TestServer) (int64, int64) {
				admin := ts.CreateTestUser(t, "admin@example.com", "password123")
				makeAdmin(t, ts, admin)
				target := ts.CreateTestUser(t, "target@example.com", "password123")
				return admin, target
			},
			body:       map[string]interface{}{"invite_count": 10},
			isAdmin:    true,
			wantStatus: http.StatusOK,
			wantCount:  10,
		},
		{
			name: "admin sets invite count to zero",
			setupFunc: func(ts *TestServer) (int64, int64) {
				admin := ts.CreateTestUser(t, "admin@example.com", "password123")
				makeAdmin(t, ts, admin)
				target := ts.CreateTestUser(t, "target@example.com", "password123")
				return admin, target
			},
			body:       map[string]interface{}{"invite_count": 0},
			isAdmin:    true,
			wantStatus: http.StatusOK,
			wantCount:  0,
		},
		{
			name: "non-admin forbidden",
			setupFunc: func(ts *TestServer) (int64, int64) {
				user := ts.CreateTestUser(t, "user@example.com", "password123")
				target := ts.CreateTestUser(t, "target@example.com", "password123")
				return user, target
			},
			body:          map[string]interface{}{"invite_count": 10},
			isAdmin:       false,
			wantStatus:    http.StatusForbidden,
			wantError:     "admin access required",
			wantErrorCode: "forbidden",
		},
		{
			name:          "unauthenticated request",
			noAuth:        true,
			body:          map[string]interface{}{"invite_count": 10},
			targetIDPath:  "1",
			wantStatus:    http.StatusUnauthorized,
			wantError:     "user not authenticated",
			wantErrorCode: "unauthorized",
		},
		{
			name: "negative invite count",
			setupFunc: func(ts *TestServer) (int64, int64) {
				admin := ts.CreateTestUser(t, "admin@example.com", "password123")
				makeAdmin(t, ts, admin)
				target := ts.CreateTestUser(t, "target@example.com", "password123")
				return admin, target
			},
			body:          map[string]interface{}{"invite_count": -1},
			isAdmin:       true,
			wantStatus:    http.StatusBadRequest,
			wantError:     "invite count must be non-negative",
			wantErrorCode: "validation_error",
		},
		{
			name: "invalid user id in path",
			setupFunc: func(ts *TestServer) (int64, int64) {
				admin := ts.CreateTestUser(t, "admin@example.com", "password123")
				makeAdmin(t, ts, admin)
				return admin, 0
			},
			targetIDPath:  "not-a-number",
			body:          map[string]interface{}{"invite_count": 5},
			isAdmin:       true,
			wantStatus:    http.StatusBadRequest,
			wantError:     "invalid user id",
			wantErrorCode: "validation_error",
		},
		{
			name: "target user not found",
			setupFunc: func(ts *TestServer) (int64, int64) {
				admin := ts.CreateTestUser(t, "admin@example.com", "password123")
				makeAdmin(t, ts, admin)
				return admin, 0
			},
			targetIDPath:  "99999",
			body:          map[string]interface{}{"invite_count": 5},
			isAdmin:       true,
			wantStatus:    http.StatusNotFound,
			wantError:     "user not found",
			wantErrorCode: "not_found",
		},
		{
			name: "invalid request body",
			setupFunc: func(ts *TestServer) (int64, int64) {
				admin := ts.CreateTestUser(t, "admin@example.com", "password123")
				makeAdmin(t, ts, admin)
				target := ts.CreateTestUser(t, "target@example.com", "password123")
				return admin, target
			},
			body:          "not-json",
			isAdmin:       true,
			wantStatus:    http.StatusBadRequest,
			wantError:     "invalid request body",
			wantErrorCode: "invalid_request",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ts := NewTestServer(t)
			defer ts.Close()

			if tt.noAuth {
				rec, req := MakeRequest(t, http.MethodPatch, "/api/admin/users/1/invites", tt.body, nil)
				req.SetPathValue("id", tt.targetIDPath)
				ts.HandleAdminBoostInvites(rec, req)

				AssertStatusCode(t, rec.Code, tt.wantStatus)
				if tt.wantError != "" {
					AssertError(t, rec, tt.wantStatus, tt.wantError, tt.wantErrorCode)
				}
				return
			}

			adminID, targetUserID := tt.setupFunc(ts)

			pathID := tt.targetIDPath
			if pathID == "" {
				pathID = fmt.Sprintf("%d", targetUserID)
			}

			rec, req := ts.MakeAuthRequest(t, http.MethodPatch, "/api/admin/users/"+pathID+"/invites", tt.body, adminID, nil)
			req.SetPathValue("id", pathID)
			ts.HandleAdminBoostInvites(rec, req)

			AssertStatusCode(t, rec.Code, tt.wantStatus)

			if tt.wantError != "" {
				AssertError(t, rec, tt.wantStatus, tt.wantError, tt.wantErrorCode)
			} else {
				var resp map[string]interface{}
				DecodeJSON(t, rec, &resp)

				gotID := int64(resp["id"].(float64))
				if gotID != targetUserID {
					t.Errorf("Response id = %d, want %d", gotID, targetUserID)
				}

				gotCount := int(resp["invite_count"].(float64))
				if gotCount != tt.wantCount {
					t.Errorf("Response invite_count = %d, want %d", gotCount, tt.wantCount)
				}

				// Verify it was persisted in the database
				ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
				defer cancel()

				var dbCount int
				err := ts.DB.QueryRowContext(ctx, `SELECT invite_count FROM users WHERE id = ?`, targetUserID).Scan(&dbCount)
				if err != nil {
					t.Fatalf("Failed to query invite count: %v", err)
				}
				if dbCount != tt.wantCount {
					t.Errorf("DB invite_count = %d, want %d", dbCount, tt.wantCount)
				}
			}
		})
	}
}

// ---------------------------------------------------------------------------
// HandleListInvites — response field coverage
// ---------------------------------------------------------------------------

func TestHandleListInvites_WithExpiresAt(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	inviterID := ts.CreateTestUser(t, "inviter@example.com", "password123")
	code := ts.CreateTestInvite(t, inviterID)

	// Set expires_at on the invite
	ctx := context.Background()
	expiresAt := time.Now().Add(7 * 24 * time.Hour).Format(time.RFC3339)
	_, err := ts.DB.ExecContext(ctx, `UPDATE invites SET expires_at = ? WHERE code = ?`, expiresAt, code)
	if err != nil {
		t.Fatalf("Failed to set expires_at: %v", err)
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/invites", nil, inviterID, nil)
	ts.HandleListInvites(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var resp map[string]json.RawMessage
	DecodeJSON(t, rec, &resp)

	var invites []map[string]interface{}
	if err := json.Unmarshal(resp["invites"], &invites); err != nil {
		t.Fatalf("Failed to unmarshal invites: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("Expected 1 invite, got %d", len(invites))
	}

	if invites[0]["expires_at"] == nil {
		t.Error("Expected expires_at to be set in the response")
	}
	if invites[0]["code"] == nil || invites[0]["code"].(string) == "" {
		t.Error("Expected non-empty code in the response")
	}
	if invites[0]["created_at"] == nil || invites[0]["created_at"].(string) == "" {
		t.Error("Expected non-empty created_at in the response")
	}
}

func TestHandleListInvites_WithInviteeName(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	inviterID := ts.CreateTestUser(t, "inviter@example.com", "password123")
	inviteeID := ts.CreateTestUser(t, "invitee@example.com", "password123")

	// Set name for invitee
	ctx := context.Background()
	_, err := ts.DB.ExecContext(ctx, `UPDATE users SET name = 'Jane Smith' WHERE id = ?`, inviteeID)
	if err != nil {
		t.Fatalf("Failed to set invitee name: %v", err)
	}

	code := ts.CreateTestInvite(t, inviterID)

	// Mark invite as used with invitee
	_, err = ts.DB.ExecContext(ctx,
		`UPDATE invites SET used_at = CURRENT_TIMESTAMP, invitee_id = ? WHERE code = ?`,
		inviteeID, code,
	)
	if err != nil {
		t.Fatalf("Failed to mark invite as used: %v", err)
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/invites", nil, inviterID, nil)
	ts.HandleListInvites(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var resp map[string]json.RawMessage
	DecodeJSON(t, rec, &resp)

	var invites []map[string]interface{}
	if err := json.Unmarshal(resp["invites"], &invites); err != nil {
		t.Fatalf("Failed to unmarshal invites: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("Expected 1 invite, got %d", len(invites))
	}

	// Verify invitee_name is populated
	inviteeName, ok := invites[0]["invitee_name"].(string)
	if !ok || inviteeName == "" {
		t.Error("Expected invitee_name to be set in the response")
	}
	if inviteeName != "Jane Smith" {
		t.Errorf("Expected invitee_name 'Jane Smith', got %q", inviteeName)
	}

	// Verify used_at is populated
	if invites[0]["used_at"] == nil {
		t.Error("Expected used_at to be set in the response")
	}

	// Verify invitee_id is populated
	inviteeIDResp, ok := invites[0]["invitee_id"].(float64)
	if !ok {
		t.Error("Expected invitee_id to be set in the response")
	} else if int64(inviteeIDResp) != inviteeID {
		t.Errorf("Expected invitee_id %d, got %v", inviteeID, inviteeIDResp)
	}
}

func TestHandleListInvites_IsAdminField(t *testing.T) {
	t.Run("non-admin user", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "user@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/invites", nil, userID, nil)
		ts.HandleListInvites(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp map[string]interface{}
		DecodeJSON(t, rec, &resp)

		isAdmin, ok := resp["is_admin"].(bool)
		if !ok {
			t.Fatal("Expected is_admin field in response")
		}
		if isAdmin {
			t.Error("Expected is_admin to be false for regular user")
		}
	})

	t.Run("admin user", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		makeAdmin(t, ts, adminID)

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/invites", nil, adminID, nil)
		ts.HandleListInvites(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp map[string]interface{}
		DecodeJSON(t, rec, &resp)

		isAdmin, ok := resp["is_admin"].(bool)
		if !ok {
			t.Fatal("Expected is_admin field in response")
		}
		if !isAdmin {
			t.Error("Expected is_admin to be true for admin user")
		}
	})
}

// ---------------------------------------------------------------------------
// HandleCreateInvite — additional coverage
// ---------------------------------------------------------------------------

func TestHandleCreateInvite_WithEmailNoService(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "inviter@example.com", "password123")

	// Provide email in request body but no email service configured
	body := map[string]interface{}{
		"email": "friend@example.com",
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/invites", body, userID, nil)
	ts.HandleCreateInvite(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var resp map[string]interface{}
	DecodeJSON(t, rec, &resp)

	// email_sent should be false because no email service is configured
	emailSent, ok := resp["email_sent"].(bool)
	if !ok {
		t.Fatal("Expected email_sent field in response")
	}
	if emailSent {
		t.Error("Expected email_sent to be false when no email service is configured")
	}

	// Code should still be returned
	code, ok := resp["code"].(string)
	if !ok || code == "" {
		t.Error("Expected non-empty invite code")
	}
}

func TestHandleCreateInvite_AdminDoesNotDecrementCount(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
	makeAdmin(t, ts, adminID)

	// Set a known invite count
	ctx := context.Background()
	_, err := ts.DB.ExecContext(ctx, `UPDATE users SET invite_count = 5 WHERE id = ?`, adminID)
	if err != nil {
		t.Fatalf("Failed to update invite count: %v", err)
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/invites", nil, adminID, nil)
	ts.HandleCreateInvite(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	// Verify invite count was NOT decremented for admin
	var inviteCount int
	err = ts.DB.QueryRow("SELECT invite_count FROM users WHERE id = ?", adminID).Scan(&inviteCount)
	if err != nil {
		t.Fatalf("Failed to query invite count: %v", err)
	}
	if inviteCount != 5 {
		t.Errorf("Expected admin invite_count to remain 5 (not decremented), got %d", inviteCount)
	}
}

func TestHandleCreateInvite_EmailSentFieldFalseWithoutEmail(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "inviter@example.com", "password123")

	// No email in request body
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/invites", nil, userID, nil)
	ts.HandleCreateInvite(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var resp map[string]interface{}
	DecodeJSON(t, rec, &resp)

	emailSent, ok := resp["email_sent"].(bool)
	if !ok {
		t.Fatal("Expected email_sent field in response")
	}
	if emailSent {
		t.Error("Expected email_sent to be false when no email provided")
	}
}

func TestHandleCreateInvite_ConsecutiveInvitesDecrementCount(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "inviter@example.com", "password123")

	// Default invite_count is 3
	for i := 0; i < 3; i++ {
		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/invites", nil, userID, nil)
		ts.HandleCreateInvite(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusCreated)
	}

	// Verify count is now 0
	var inviteCount int
	err := ts.DB.QueryRow("SELECT invite_count FROM users WHERE id = ?", userID).Scan(&inviteCount)
	if err != nil {
		t.Fatalf("Failed to query invite count: %v", err)
	}
	if inviteCount != 0 {
		t.Errorf("Expected invite_count 0 after 3 invites, got %d", inviteCount)
	}

	// Fourth invite should fail
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/invites", nil, userID, nil)
	ts.HandleCreateInvite(rec, req)

	AssertError(t, rec, http.StatusForbidden, "no invites remaining", "no_invites")
}

// ---------------------------------------------------------------------------
// HandleValidateInvite — inviter without name (nil name edge case)
// ---------------------------------------------------------------------------

func TestHandleValidateInvite_InviterWithoutName(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Create user without setting a name
	inviterID := ts.CreateTestUser(t, "noname@example.com", "password123")
	code := ts.CreateTestInvite(t, inviterID)

	rec, req := MakeRequest(t, http.MethodGet, "/api/invites/validate?code="+code, nil, nil)
	ts.HandleValidateInvite(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var status InviteStatus
	DecodeJSON(t, rec, &status)

	if !status.Valid {
		t.Errorf("Expected invite to be valid, got invalid: %s", status.Message)
	}
	// InviterName may be empty or email-based, just should not panic
}

// ---------------------------------------------------------------------------
// HandleAdminBoostInvites — empty path id
// ---------------------------------------------------------------------------

func TestHandleAdminBoostInvites_EmptyPathID(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
	makeAdmin(t, ts, adminID)

	body := map[string]interface{}{"invite_count": 5}

	rec, req := ts.MakeAuthRequest(t, http.MethodPatch, "/api/admin/users//invites", body, adminID, nil)
	req.SetPathValue("id", "")
	ts.HandleAdminBoostInvites(rec, req)

	AssertError(t, rec, http.StatusBadRequest, "user id required", "validation_error")
}

// ---------------------------------------------------------------------------
// Integration: create invite then verify it appears in list
// ---------------------------------------------------------------------------

func TestInviteCreateThenList(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "inviter@example.com", "password123")

	// Create invite via handler
	createRec, createReq := ts.MakeAuthRequest(t, http.MethodPost, "/api/invites", nil, userID, nil)
	ts.HandleCreateInvite(createRec, createReq)
	AssertStatusCode(t, createRec.Code, http.StatusCreated)

	var createResp map[string]interface{}
	DecodeJSON(t, createRec, &createResp)
	createdCode := createResp["code"].(string)

	// List invites
	listRec, listReq := ts.MakeAuthRequest(t, http.MethodGet, "/api/invites", nil, userID, nil)
	ts.HandleListInvites(listRec, listReq)
	AssertStatusCode(t, listRec.Code, http.StatusOK)

	var listResp map[string]json.RawMessage
	DecodeJSON(t, listRec, &listResp)

	var invites []map[string]interface{}
	if err := json.Unmarshal(listResp["invites"], &invites); err != nil {
		t.Fatalf("Failed to unmarshal invites: %v", err)
	}
	if len(invites) != 1 {
		t.Fatalf("Expected 1 invite, got %d", len(invites))
	}

	if invites[0]["code"].(string) != createdCode {
		t.Errorf("Expected code %q in list, got %q", createdCode, invites[0]["code"])
	}
}

// ---------------------------------------------------------------------------
// Integration: create invite then validate it
// ---------------------------------------------------------------------------

func TestInviteCreateThenValidate(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "inviter@example.com", "password123")

	// Create invite via handler
	createRec, createReq := ts.MakeAuthRequest(t, http.MethodPost, "/api/invites", nil, userID, nil)
	ts.HandleCreateInvite(createRec, createReq)
	AssertStatusCode(t, createRec.Code, http.StatusCreated)

	var createResp map[string]interface{}
	DecodeJSON(t, createRec, &createResp)
	createdCode := createResp["code"].(string)

	// Validate the created invite
	validateRec, validateReq := MakeRequest(t, http.MethodGet, "/api/invites/validate?code="+createdCode, nil, nil)
	ts.HandleValidateInvite(validateRec, validateReq)
	AssertStatusCode(t, validateRec.Code, http.StatusOK)

	var status InviteStatus
	DecodeJSON(t, validateRec, &status)

	if !status.Valid {
		t.Errorf("Expected newly created invite to be valid, got invalid: %s", status.Message)
	}
}

// ---------------------------------------------------------------------------
// HandleListInvites — does not show other users' invites
// ---------------------------------------------------------------------------

func TestHandleListInvites_IsolatedPerUser(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	user1ID := ts.CreateTestUser(t, "user1@example.com", "password123")
	user2ID := ts.CreateTestUser(t, "user2@example.com", "password123")

	// Create invites for both users
	ts.CreateTestInvite(t, user1ID)
	ts.CreateTestInvite(t, user1ID)
	ts.CreateTestInvite(t, user2ID)

	// List invites for user1 — should only see their own
	rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/invites", nil, user1ID, nil)
	ts.HandleListInvites(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	var resp map[string]json.RawMessage
	DecodeJSON(t, rec, &resp)

	var invites []json.RawMessage
	if err := json.Unmarshal(resp["invites"], &invites); err != nil {
		t.Fatalf("Failed to unmarshal invites: %v", err)
	}
	if len(invites) != 2 {
		t.Errorf("Expected user1 to see 2 invites, got %d", len(invites))
	}

	// List invites for user2 — should only see their own
	rec2, req2 := ts.MakeAuthRequest(t, http.MethodGet, "/api/invites", nil, user2ID, nil)
	ts.HandleListInvites(rec2, req2)
	AssertStatusCode(t, rec2.Code, http.StatusOK)

	var resp2 map[string]json.RawMessage
	DecodeJSON(t, rec2, &resp2)

	var invites2 []json.RawMessage
	if err := json.Unmarshal(resp2["invites"], &invites2); err != nil {
		t.Fatalf("Failed to unmarshal invites: %v", err)
	}
	if len(invites2) != 1 {
		t.Errorf("Expected user2 to see 1 invite, got %d", len(invites2))
	}
}

// ---------------------------------------------------------------------------
// HandleAdminBoostInvites — large invite count
// ---------------------------------------------------------------------------

func TestHandleAdminBoostInvites_LargeCount(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
	makeAdmin(t, ts, adminID)
	targetID := ts.CreateTestUser(t, "target@example.com", "password123")

	body := map[string]interface{}{"invite_count": 1000}

	rec, req := ts.MakeAuthRequest(t, http.MethodPatch,
		fmt.Sprintf("/api/admin/users/%d/invites", targetID), body, adminID, nil)
	req.SetPathValue("id", fmt.Sprintf("%d", targetID))
	ts.HandleAdminBoostInvites(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	var resp map[string]interface{}
	DecodeJSON(t, rec, &resp)

	gotCount := int(resp["invite_count"].(float64))
	if gotCount != 1000 {
		t.Errorf("Expected invite_count 1000, got %d", gotCount)
	}
}

// ---------------------------------------------------------------------------
// generateInviteCode — unit test
// ---------------------------------------------------------------------------

func TestGenerateInviteCode(t *testing.T) {
	code1, err := generateInviteCode()
	if err != nil {
		t.Fatalf("generateInviteCode() returned error: %v", err)
	}
	if code1 == "" {
		t.Error("Expected non-empty invite code")
	}
	// Base64 URL encoding of 18 bytes = 24 chars
	if len(code1) != 24 {
		t.Errorf("Expected code length 24, got %d", len(code1))
	}

	// Ensure codes are unique
	code2, err := generateInviteCode()
	if err != nil {
		t.Fatalf("generateInviteCode() returned error: %v", err)
	}
	if code1 == code2 {
		t.Error("Expected two generated codes to be different")
	}

	// Ensure no padding characters
	if strings.Contains(code1, "=") {
		t.Errorf("Expected no padding in invite code, got %q", code1)
	}
}
