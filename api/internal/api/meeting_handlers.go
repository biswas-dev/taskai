package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// MeetingAttendee is a project member who attended a meeting.
type MeetingAttendee struct {
	UserID int64  `json:"user_id"`
	Name   string `json:"name"`
	Email  string `json:"email"`
}

// Meeting is a project's minutes of meeting.
type Meeting struct {
	ID        int64             `json:"id"`
	ProjectID int64             `json:"project_id"`
	Title     string            `json:"title"`
	StartsAt  time.Time         `json:"starts_at"`
	EndsAt    time.Time         `json:"ends_at"`
	Agenda    string            `json:"agenda"`
	Minutes   string            `json:"minutes"`
	CreatedBy *int64            `json:"created_by,omitempty"`
	Attendees []MeetingAttendee `json:"attendees"`
	CreatedAt time.Time         `json:"created_at"`
	UpdatedAt time.Time         `json:"updated_at"`
}

// CreateMeetingRequest creates a meeting. Omitting attendee_ids invites every project member.
type CreateMeetingRequest struct {
	Title       string    `json:"title"`
	StartsAt    time.Time `json:"starts_at"`
	EndsAt      time.Time `json:"ends_at"`
	Agenda      string    `json:"agenda"`
	Minutes     string    `json:"minutes"`
	AttendeeIDs *[]int64  `json:"attendee_ids,omitempty"`
}

// UpdateMeetingRequest patches a meeting; attendee_ids replaces the whole list.
type UpdateMeetingRequest struct {
	Title       *string    `json:"title,omitempty"`
	StartsAt    *time.Time `json:"starts_at,omitempty"`
	EndsAt      *time.Time `json:"ends_at,omitempty"`
	Agenda      *string    `json:"agenda,omitempty"`
	Minutes     *string    `json:"minutes,omitempty"`
	AttendeeIDs *[]int64   `json:"attendee_ids,omitempty"`
}

const (
	maxMeetingTitle = 200
	maxMeetingText  = 200_000
	meetingCols     = `id, project_id, title, starts_at, ends_at, agenda, minutes, created_by, created_at, updated_at`
)

var errMeetingAttendee = errors.New("attendees must be members of this project")

func validateMeeting(title string, startsAt, endsAt time.Time, agenda, minutes string) string {
	switch {
	case title == "":
		return "title is required"
	case len(title) > maxMeetingTitle:
		return "title is too long"
	case startsAt.IsZero() || endsAt.IsZero():
		return "starts_at and ends_at are required"
	case !endsAt.After(startsAt):
		return "ends_at must be after starts_at"
	case len(agenda) > maxMeetingText || len(minutes) > maxMeetingText:
		return "agenda or minutes is too long"
	}
	return ""
}

// requireProjectMember resolves the caller and checks they belong to the project.
func (s *Server) requireProjectMember(w http.ResponseWriter, r *http.Request, projectID int64) (int64, bool) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return 0, false
	}
	hasAccess, err := s.checkProjectAccess(r.Context(), userID, projectID)
	if err != nil {
		s.logger.Error("Failed to check project access", zap.Int64("project_id", projectID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to check access", "internal_error")
		return 0, false
	}
	if !hasAccess {
		respondError(w, http.StatusForbidden, "forbidden", "forbidden")
		return 0, false
	}
	return userID, true
}

// meetingForRequest loads the meeting's project from {id} and checks membership.
func (s *Server) meetingForRequest(w http.ResponseWriter, r *http.Request) (meetingID, projectID, userID int64, ok bool) {
	meetingID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid meeting ID", "bad_request")
		return 0, 0, 0, false
	}
	err = s.db.QueryRowContext(r.Context(), `SELECT project_id FROM meetings WHERE id = $1`, meetingID).Scan(&projectID)
	if errors.Is(err, sql.ErrNoRows) {
		respondError(w, http.StatusNotFound, "meeting not found", "not_found")
		return 0, 0, 0, false
	}
	if err != nil {
		s.logger.Error("Failed to load meeting", zap.Int64("meeting_id", meetingID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to load meeting", "internal_error")
		return 0, 0, 0, false
	}
	userID, ok = s.requireProjectMember(w, r, projectID)
	return meetingID, projectID, userID, ok
}

// setMeetingAttendees replaces a meeting's attendees; nil ids means every project member.
func setMeetingAttendees(ctx context.Context, tx *sql.Tx, meetingID, projectID int64, ids *[]int64) error {
	if _, err := tx.ExecContext(ctx, `DELETE FROM meeting_attendees WHERE meeting_id = $1`, meetingID); err != nil {
		return err
	}
	if ids == nil {
		_, err := tx.ExecContext(ctx,
			`INSERT INTO meeting_attendees (meeting_id, user_id) SELECT CAST($1 AS BIGINT), user_id FROM project_members WHERE project_id = $2`,
			meetingID, projectID)
		return err
	}
	seen := map[int64]bool{}
	for _, uid := range *ids {
		if seen[uid] {
			continue
		}
		seen[uid] = true
		var n int
		if err := tx.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM project_members WHERE project_id = $1 AND user_id = $2`, projectID, uid).Scan(&n); err != nil {
			return err
		}
		if n == 0 {
			return errMeetingAttendee
		}
		if _, err := tx.ExecContext(ctx,
			`INSERT INTO meeting_attendees (meeting_id, user_id) VALUES ($1, $2)`, meetingID, uid); err != nil {
			return err
		}
	}
	return nil
}

// loadMeetings fetches meetings (by project or by id) with their attendees.
func (s *Server) loadMeetings(ctx context.Context, where string, arg int64) ([]Meeting, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT `+meetingCols+` FROM meetings WHERE `+where+` = $1 ORDER BY starts_at DESC, id DESC`, arg)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	meetings := []Meeting{}
	index := map[int64]int{}
	for rows.Next() {
		var m Meeting
		if err := rows.Scan(&m.ID, &m.ProjectID, &m.Title, &m.StartsAt, &m.EndsAt, &m.Agenda, &m.Minutes,
			&m.CreatedBy, &m.CreatedAt, &m.UpdatedAt); err != nil {
			return nil, err
		}
		m.Attendees = []MeetingAttendee{}
		index[m.ID] = len(meetings)
		meetings = append(meetings, m)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(meetings) == 0 {
		return meetings, nil
	}

	arows, err := s.db.QueryContext(ctx,
		`SELECT ma.meeting_id, u.id, u.email, COALESCE(u.first_name, ''), COALESCE(u.last_name, '')
		 FROM meeting_attendees ma
		 JOIN meetings m ON m.id = ma.meeting_id
		 JOIN users u ON u.id = ma.user_id
		 WHERE m.`+where+` = $1
		 ORDER BY u.first_name, u.email`, arg)
	if err != nil {
		return nil, err
	}
	defer arows.Close()
	for arows.Next() {
		var meetingID int64
		var a MeetingAttendee
		var first, last string
		if err := arows.Scan(&meetingID, &a.UserID, &a.Email, &first, &last); err != nil {
			return nil, err
		}
		a.Name = strings.TrimSpace(first + " " + last)
		if a.Name == "" {
			a.Name = a.Email
		}
		if i, ok := index[meetingID]; ok {
			meetings[i].Attendees = append(meetings[i].Attendees, a)
		}
	}
	return meetings, arows.Err()
}

func (s *Server) respondMeeting(w http.ResponseWriter, ctx context.Context, status int, meetingID int64) {
	meetings, err := s.loadMeetings(ctx, "id", meetingID)
	if err != nil || len(meetings) == 0 {
		s.logger.Error("Failed to fetch meeting", zap.Int64("meeting_id", meetingID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch meeting", "internal_error")
		return
	}
	respondJSON(w, status, meetings[0])
}

// HandleListMeetings lists a project's meetings, newest first.
// Route: GET /api/projects/{id}/meetings
func (s *Server) HandleListMeetings(w http.ResponseWriter, r *http.Request) {
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "bad_request")
		return
	}
	if _, ok := s.requireProjectMember(w, r, projectID); !ok {
		return
	}
	meetings, err := s.loadMeetings(r.Context(), "project_id", projectID)
	if err != nil {
		s.logger.Error("Failed to list meetings", zap.Int64("project_id", projectID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list meetings", "internal_error")
		return
	}
	respondJSON(w, http.StatusOK, meetings)
}

// HandleCreateMeeting records a meeting in a project.
// Route: POST /api/projects/{id}/meetings
func (s *Server) HandleCreateMeeting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "bad_request")
		return
	}
	userID, ok := s.requireProjectMember(w, r, projectID)
	if !ok {
		return
	}

	var req CreateMeetingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "bad_request")
		return
	}
	req.Title = strings.TrimSpace(req.Title)
	if msg := validateMeeting(req.Title, req.StartsAt, req.EndsAt, req.Agenda, req.Minutes); msg != "" {
		respondError(w, http.StatusBadRequest, msg, "validation_error")
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logger.Error("Failed to begin transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create meeting", "internal_error")
		return
	}
	defer tx.Rollback() //nolint:errcheck

	var meetingID int64
	if err := tx.QueryRowContext(ctx,
		`INSERT INTO meetings (project_id, title, starts_at, ends_at, agenda, minutes, created_by)
		 VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
		projectID, req.Title, req.StartsAt.UTC(), req.EndsAt.UTC(), req.Agenda, req.Minutes, userID,
	).Scan(&meetingID); err != nil {
		s.logger.Error("Failed to insert meeting", zap.Int64("project_id", projectID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create meeting", "internal_error")
		return
	}
	if err := setMeetingAttendees(ctx, tx, meetingID, projectID, req.AttendeeIDs); err != nil {
		s.respondAttendeeError(w, err)
		return
	}
	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to commit meeting", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create meeting", "internal_error")
		return
	}
	s.respondMeeting(w, ctx, http.StatusCreated, meetingID)
}

func (s *Server) respondAttendeeError(w http.ResponseWriter, err error) {
	if errors.Is(err, errMeetingAttendee) {
		respondError(w, http.StatusBadRequest, err.Error(), "not_project_member")
		return
	}
	s.logger.Error("Failed to set meeting attendees", zap.Error(err))
	respondError(w, http.StatusInternalServerError, "failed to save attendees", "internal_error")
}

// HandleGetMeeting returns one meeting.
// Route: GET /api/meetings/{id}
func (s *Server) HandleGetMeeting(w http.ResponseWriter, r *http.Request) {
	meetingID, _, _, ok := s.meetingForRequest(w, r)
	if !ok {
		return
	}
	s.respondMeeting(w, r.Context(), http.StatusOK, meetingID)
}

// HandleUpdateMeeting edits a meeting's details, agenda, minutes or attendees.
// Route: PATCH /api/meetings/{id}
func (s *Server) HandleUpdateMeeting(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	meetingID, projectID, _, ok := s.meetingForRequest(w, r)
	if !ok {
		return
	}
	var req UpdateMeetingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "bad_request")
		return
	}

	current, err := s.loadMeetings(ctx, "id", meetingID)
	if err != nil || len(current) == 0 {
		s.logger.Error("Failed to load meeting for update", zap.Int64("meeting_id", meetingID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update meeting", "internal_error")
		return
	}
	m := current[0]
	if req.Title != nil {
		m.Title = strings.TrimSpace(*req.Title)
	}
	if req.StartsAt != nil {
		m.StartsAt = *req.StartsAt
	}
	if req.EndsAt != nil {
		m.EndsAt = *req.EndsAt
	}
	if req.Agenda != nil {
		m.Agenda = *req.Agenda
	}
	if req.Minutes != nil {
		m.Minutes = *req.Minutes
	}
	if msg := validateMeeting(m.Title, m.StartsAt, m.EndsAt, m.Agenda, m.Minutes); msg != "" {
		respondError(w, http.StatusBadRequest, msg, "validation_error")
		return
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		s.logger.Error("Failed to begin transaction", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update meeting", "internal_error")
		return
	}
	defer tx.Rollback() //nolint:errcheck

	if _, err := tx.ExecContext(ctx,
		`UPDATE meetings SET title = $1, starts_at = $2, ends_at = $3, agenda = $4, minutes = $5, updated_at = $6 WHERE id = $7`,
		m.Title, m.StartsAt.UTC(), m.EndsAt.UTC(), m.Agenda, m.Minutes, time.Now().UTC(), meetingID,
	); err != nil {
		s.logger.Error("Failed to update meeting", zap.Int64("meeting_id", meetingID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update meeting", "internal_error")
		return
	}
	if req.AttendeeIDs != nil {
		if err := setMeetingAttendees(ctx, tx, meetingID, projectID, req.AttendeeIDs); err != nil {
			s.respondAttendeeError(w, err)
			return
		}
	}
	if err := tx.Commit(); err != nil {
		s.logger.Error("Failed to commit meeting update", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update meeting", "internal_error")
		return
	}
	s.respondMeeting(w, ctx, http.StatusOK, meetingID)
}

// HandleDeleteMeeting removes a meeting and its attendee list.
// Route: DELETE /api/meetings/{id}
func (s *Server) HandleDeleteMeeting(w http.ResponseWriter, r *http.Request) {
	meetingID, _, _, ok := s.meetingForRequest(w, r)
	if !ok {
		return
	}
	// Explicit attendee delete: SQLite test DBs may run without foreign_keys enforcement.
	_, err := s.db.ExecContext(r.Context(), `DELETE FROM meeting_attendees WHERE meeting_id = $1`, meetingID)
	if err == nil {
		_, err = s.db.ExecContext(r.Context(), `DELETE FROM meetings WHERE id = $1`, meetingID)
	}
	if err != nil {
		s.logger.Error("Failed to delete meeting", zap.Int64("meeting_id", meetingID), zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to delete meeting", "internal_error")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
