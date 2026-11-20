package api

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"testing"

	"go.uber.org/zap/zaptest"

	"taskai/internal/config"
	"taskai/internal/db"
)

func TestMeetingHandlers(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	owner := ts.CreateTestUser(t, "owner@example.com", "password123")
	member := ts.CreateTestUser(t, "member@example.com", "password123")
	outsider := ts.CreateTestUser(t, "outsider@example.com", "password123")
	projectID := ts.CreateTestProject(t, owner, "Engineering")
	ts.AddProjectMember(t, projectID, member, owner, "member")
	runMeetingTests(t, ts, owner, member, outsider, projectID)
}

func TestPostgresMeetingHandlers(t *testing.T) {
	dsn := os.Getenv("TEST_POSTGRES_DSN")
	if dsn == "" {
		t.Skip("TEST_POSTGRES_DSN not set")
	}
	ctx := context.Background()
	database, err := db.New(db.Config{Driver: "postgres", DSN: dsn, MigrationsPath: "./../../internal/db/migrations"}, zaptest.NewLogger(t))
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	s := NewServer(database, &config.Config{JWTSecret: "test-secret-key", JWTExpiryHours: 24}, zaptest.NewLogger(t))
	ts := &TestServer{Server: s, DB: database}

	var ids [3]int64
	for i, email := range []string{"pg-mtg-owner@example.com", "pg-mtg-member@example.com", "pg-mtg-out@example.com"} {
		if err := database.QueryRowContext(ctx, `INSERT INTO users (email, password_hash) VALUES ($1, 'x') RETURNING id`, email).Scan(&ids[i]); err != nil {
			t.Fatal(err)
		}
	}
	var projectID int64
	if err := database.QueryRowContext(ctx, `INSERT INTO projects (owner_id, name) VALUES ($1, 'Engineering') RETURNING id`, ids[0]).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	for i, role := range []string{"owner", "member"} {
		if _, err := database.ExecContext(ctx, `INSERT INTO project_members (project_id, user_id, role, granted_by) VALUES ($1, $2, $3, $4)`, projectID, ids[i], role, ids[0]); err != nil {
			t.Fatal(err)
		}
	}
	runMeetingTests(t, ts, ids[0], ids[1], ids[2], projectID)
}

func runMeetingTests(t *testing.T, ts *TestServer, owner, member, outsider, projectID int64) {
	pid := fmt.Sprintf("%d", projectID)
	path := "/api/projects/" + pid + "/meetings"

	create := func(user int64, body map[string]interface{}) (int, Meeting) {
		rec, req := ts.MakeAuthRequest(t, http.MethodPost, path, body, user, map[string]string{"id": pid})
		ts.HandleCreateMeeting(rec, req)
		var m Meeting
		if rec.Code == http.StatusCreated {
			DecodeJSON(t, rec, &m)
		}
		return rec.Code, m
	}
	valid := func(extra map[string]interface{}) map[string]interface{} {
		b := map[string]interface{}{"title": "Superteam sync", "starts_at": "2026-11-20T13:30:00-05:00", "ends_at": "2026-11-20T14:00:00-05:00"}
		for k, v := range extra {
			b[k] = v
		}
		return b
	}

	tests := []struct {
		name          string
		user          int64
		body          map[string]interface{}
		wantStatus    int
		wantAttendees int
	}{
		{"defaults to every project member", owner, valid(nil), http.StatusCreated, 2},
		{"explicit attendees", member, valid(map[string]interface{}{"attendee_ids": []int64{member}}), http.StatusCreated, 1},
		{"outsider forbidden", outsider, valid(nil), http.StatusForbidden, 0},
		{"non-member attendee rejected", owner, valid(map[string]interface{}{"attendee_ids": []int64{outsider}}), http.StatusBadRequest, 0},
		{"missing title", owner, valid(map[string]interface{}{"title": "  "}), http.StatusBadRequest, 0},
		{"ends before start", owner, valid(map[string]interface{}{"ends_at": "2026-11-20T13:00:00-05:00"}), http.StatusBadRequest, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, m := create(tt.user, tt.body)
			AssertStatusCode(t, status, tt.wantStatus)
			if status == http.StatusCreated && len(m.Attendees) != tt.wantAttendees {
				t.Errorf("attendees = %d, want %d", len(m.Attendees), tt.wantAttendees)
			}
		})
	}

	_, m := create(owner, valid(map[string]interface{}{"agenda": "1. Intros"}))
	mid := fmt.Sprintf("%d", m.ID)
	if m.StartsAt.UTC().Hour() != 18 || m.StartsAt.Minute() != 30 {
		t.Errorf("starts_at = %v, want 18:30 UTC", m.StartsAt)
	}

	t.Run("update minutes and attendees", func(t *testing.T) {
		body := map[string]interface{}{"minutes": "Decided X", "attendee_ids": []int64{owner}}
		rec, req := ts.MakeAuthRequest(t, http.MethodPatch, "/api/meetings/"+mid, body, member, map[string]string{"id": mid})
		ts.HandleUpdateMeeting(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusOK)
		var got Meeting
		DecodeJSON(t, rec, &got)
		if got.Minutes != "Decided X" || got.Agenda != "1. Intros" || len(got.Attendees) != 1 {
			t.Errorf("unexpected meeting after update: %+v", got)
		}
	})

	t.Run("list newest first, outsider cannot get", func(t *testing.T) {
		rec, req := ts.MakeAuthRequest(t, http.MethodGet, path, nil, member, map[string]string{"id": pid})
		ts.HandleListMeetings(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusOK)
		var list []Meeting
		DecodeJSON(t, rec, &list)
		if len(list) != 3 {
			t.Fatalf("list = %d meetings, want 3", len(list))
		}
		rec, req = ts.MakeAuthRequest(t, http.MethodGet, "/api/meetings/"+mid, nil, outsider, map[string]string{"id": mid})
		ts.HandleGetMeeting(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusForbidden)
	})

	t.Run("delete", func(t *testing.T) {
		rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/meetings/"+mid, nil, owner, map[string]string{"id": mid})
		ts.HandleDeleteMeeting(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusNoContent)
		rec, req = ts.MakeAuthRequest(t, http.MethodGet, "/api/meetings/"+mid, nil, owner, map[string]string{"id": mid})
		ts.HandleGetMeeting(rec, req)
		AssertStatusCode(t, rec.Code, http.StatusNotFound)
	})
}
