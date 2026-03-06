package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// AppNotification is a notification delivered to a user.
type AppNotification struct {
	ID          int64      `json:"id"`
	SenderID    *int64     `json:"sender_id,omitempty"`
	SenderName  *string    `json:"sender_name,omitempty"`
	Type        string     `json:"type"`
	EntityType  string     `json:"entity_type"`
	EntityID    int64      `json:"entity_id"`
	ProjectID   int64      `json:"project_id"`
	ProjectName *string    `json:"project_name,omitempty"`
	Message     string     `json:"message"`
	Link        string     `json:"link"`
	ReadAt      *time.Time `json:"read_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

// notificationCountResponse is returned by the count endpoint.
type notificationCountResponse struct {
	Count int `json:"count"`
}

// HandleListNotifications returns the authenticated user's notifications (newest first).
func (s *Server) HandleListNotifications(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	rows, err := s.db.QueryContext(ctx, `
		SELECT n.id, n.sender_id, u.name, n.type, n.entity_type, n.entity_id,
		       n.project_id, p.name, n.message, n.link, n.read_at, n.created_at
		FROM notifications n
		LEFT JOIN users u ON u.id = n.sender_id
		LEFT JOIN projects p ON p.id = n.project_id
		WHERE n.recipient_id = $1
		ORDER BY n.created_at DESC
		LIMIT 50
	`, userID)
	if err != nil {
		s.logger.Error("Failed to fetch notifications", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch notifications", "internal_error")
		return
	}
	defer rows.Close()

	notifications := []AppNotification{}
	for rows.Next() {
		var n AppNotification
		if err := rows.Scan(
			&n.ID, &n.SenderID, &n.SenderName, &n.Type, &n.EntityType, &n.EntityID,
			&n.ProjectID, &n.ProjectName, &n.Message, &n.Link, &n.ReadAt, &n.CreatedAt,
		); err != nil {
			continue
		}
		notifications = append(notifications, n)
	}

	respondJSON(w, http.StatusOK, notifications)
}

// HandleGetNotificationCount returns the count of unread notifications.
func (s *Server) HandleGetNotificationCount(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	var count int
	if err := s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE recipient_id = $1 AND read_at IS NULL`,
		userID,
	).Scan(&count); err != nil {
		respondError(w, http.StatusInternalServerError, "failed to get count", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, notificationCountResponse{Count: count})
}

// HandleMarkNotificationsRead marks specific notifications as read.
func (s *Server) HandleMarkNotificationsRead(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	var req struct {
		IDs []int64 `json:"ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || len(req.IDs) == 0 {
		respondError(w, http.StatusBadRequest, "ids array required", "invalid_input")
		return
	}

	// Build IN clause manually (safe: values are int64)
	placeholders := make([]string, len(req.IDs))
	args := make([]interface{}, 0, len(req.IDs)+1)
	args = append(args, userID)
	for i, id := range req.IDs {
		args = append(args, id)
		placeholders[i] = fmt.Sprintf("$%d", i+2)
	}

	query := fmt.Sprintf(
		`UPDATE notifications SET read_at = NOW() WHERE recipient_id = $1 AND id IN (%s) AND read_at IS NULL`,
		strings.Join(placeholders, ","),
	)

	if _, err := s.db.ExecContext(ctx, query, args...); err != nil {
		s.logger.Error("Failed to mark notifications read", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update notifications", "internal_error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// HandleMarkAllNotificationsRead marks all notifications as read for the user.
func (s *Server) HandleMarkAllNotificationsRead(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	if _, err := s.db.ExecContext(ctx,
		`UPDATE notifications SET read_at = NOW() WHERE recipient_id = $1 AND read_at IS NULL`,
		userID,
	); err != nil {
		s.logger.Error("Failed to mark all notifications read", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to update notifications", "internal_error")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

// ── Internal helpers ──────────────────────────────────────────────────────────

// createNotification inserts a notification record. Best-effort (non-blocking).
func (s *Server) createNotification(
	ctx context.Context,
	recipientID, senderID, projectID, entityID int64,
	notifType, entityType, message, link string,
) {
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO notifications (recipient_id, sender_id, type, entity_type, entity_id, project_id, message, link)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`, recipientID, senderID, notifType, entityType, entityID, projectID, message, link); err != nil {
		s.logger.Warn("Failed to create notification",
			zap.Error(err),
			zap.String("type", notifType),
			zap.Int64("recipient", recipientID),
		)
		return
	}
	// Push real-time event
	s.BroadcastToUser(recipientID, "notification", map[string]string{"type": notifType})
}

// mentionRegex matches @username patterns (word chars, hyphens, dots).
var mentionRegex = regexp.MustCompile(`@([\w.\-]+)`)

// extractMentionedUserIDs finds @username patterns in content and returns the
// user IDs of mentioned project members (excluding the sender).
func (s *Server) extractMentionedUserIDs(ctx context.Context, content string, projectID, senderID int64) []int64 {
	matches := mentionRegex.FindAllStringSubmatch(content, -1)
	if len(matches) == 0 {
		return nil
	}

	// Collect unique usernames
	seen := map[string]bool{}
	usernames := []string{}
	for _, m := range matches {
		un := m[1]
		if !seen[un] {
			seen[un] = true
			usernames = append(usernames, un)
		}
	}
	if len(usernames) == 0 {
		return nil
	}

	// Build IN clause (safe: usernames are from regex \w+)
	placeholders := make([]string, len(usernames))
	args := make([]interface{}, 0, len(usernames)+2)
	args = append(args, projectID, senderID)
	for i, un := range usernames {
		args = append(args, un)
		placeholders[i] = fmt.Sprintf("$%d", i+3)
	}

	query := fmt.Sprintf(`
		SELECT u.id FROM users u
		JOIN project_members pm ON pm.user_id = u.id
		WHERE pm.project_id = $1
		  AND u.id != $2
		  AND u.user_name IN (%s)
	`, strings.Join(placeholders, ","))

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil
	}
	defer rows.Close()

	var ids []int64
	for rows.Next() {
		var id int64
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids
}

// notifyMentions creates mention notifications for all mentioned users.
func (s *Server) notifyMentions(
	ctx context.Context,
	content string,
	projectID, senderID, entityID int64,
	entityType, senderName, link string,
) {
	userIDs := s.extractMentionedUserIDs(ctx, content, projectID, senderID)
	for _, uid := range userIDs {
		msg := senderName + " mentioned you"
		s.createNotification(ctx, uid, senderID, projectID, entityID, "mention", entityType, msg, link)
	}
}

// int64ToStr converts an int64 to string (for room names etc.)
func int64ToStr(id int64) string {
	return strconv.FormatInt(id, 10)
}

// notifyTaskComment sends notifications after a new task comment:
// - @mentions in the comment content
// - task assignee (if not the commenter)
// - other users who previously commented on the task
func (s *Server) notifyTaskComment(
	ctx context.Context,
	taskID, projectID, commenterID, commentID int64,
	taskTitle, content, commenterName, link string,
	assigneeID *int64,
) {
	notified := map[int64]bool{commenterID: true}

	// Notify assignee
	if assigneeID != nil && !notified[*assigneeID] {
		notified[*assigneeID] = true
		msg := commenterName + " commented on task: " + taskTitle
		s.createNotification(ctx, *assigneeID, commenterID, projectID, commentID, "task_comment", "task_comment", msg, link)
	}

	// Notify previous commenters (distinct users)
	// Collect IDs first, then close rows before calling createNotification
	// (avoids holding the DB connection open while trying to acquire another for INSERT)
	rows, err := s.db.QueryContext(ctx,
		`SELECT DISTINCT user_id FROM task_comments WHERE task_id = $1 AND id != $2`,
		taskID, commentID,
	)
	if err == nil {
		var prevCommenters []int64
		for rows.Next() {
			var uid int64
			if rows.Scan(&uid) == nil && !notified[uid] {
				prevCommenters = append(prevCommenters, uid)
			}
		}
		rows.Close()
		for _, uid := range prevCommenters {
			notified[uid] = true
			msg := commenterName + " commented on task: " + taskTitle
			s.createNotification(ctx, uid, commenterID, projectID, commentID, "task_comment", "task_comment", msg, link)
		}
	}

	// Notify @mentions
	s.notifyMentions(ctx, content, projectID, commenterID, commentID, "task_comment", commenterName, link)
}

// notifyAnnotationComment sends notifications after a new annotation comment:
// - annotation author (if not the commenter)
// - parent comment author (if a reply)
// - @mentions in the content
func (s *Server) notifyAnnotationComment(
	ctx context.Context,
	annotationID, projectID, commenterID, commentID int64,
	content, commenterName, link string,
	parentCommentID *int64,
) {
	notified := map[int64]bool{commenterID: true}

	// Notify annotation author
	var annotationAuthorID int64
	if err := s.db.QueryRowContext(ctx,
		`SELECT author_id FROM wiki_annotations WHERE id = $1`, annotationID,
	).Scan(&annotationAuthorID); err == nil && !notified[annotationAuthorID] {
		notified[annotationAuthorID] = true
		msg := commenterName + " commented on your annotation"
		s.createNotification(ctx, annotationAuthorID, commenterID, projectID, commentID, "annotation_comment", "annotation_comment", msg, link)
	}

	// Notify parent comment author if this is a reply
	if parentCommentID != nil {
		var parentAuthorID int64
		if err := s.db.QueryRowContext(ctx,
			`SELECT author_id FROM wiki_annotation_comments WHERE id = $1`, *parentCommentID,
		).Scan(&parentAuthorID); err == nil && !notified[parentAuthorID] {
			notified[parentAuthorID] = true
			msg := commenterName + " replied to your comment"
			s.createNotification(ctx, parentAuthorID, commenterID, projectID, commentID, "reply", "annotation_comment", msg, link)
		}
	}

	// Notify @mentions
	s.notifyMentions(ctx, content, projectID, commenterID, commentID, "annotation_comment", commenterName, link)
}
