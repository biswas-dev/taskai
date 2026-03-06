package api

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// UserProfileInfo holds basic user info for a profile page.
type UserProfileInfo struct {
	ID        int64   `json:"id"`
	Name      *string `json:"name,omitempty"`
	UserName  *string `json:"user_name,omitempty"`
	Email     string  `json:"email"`
}

// UserActivityItem represents a single activity entry.
type UserActivityItem struct {
	Type        string    `json:"type"`         // task_comment, wiki_page, wiki_edit, annotation_comment, task_created
	EntityID    int64     `json:"entity_id"`
	EntityTitle string    `json:"entity_title"`
	ProjectID   int64     `json:"project_id"`
	ProjectName string    `json:"project_name"`
	Link        string    `json:"link"`
	CreatedAt   time.Time `json:"created_at"`
}

// UserProfileResponse is the full user profile payload.
type UserProfileResponse struct {
	User           UserProfileInfo    `json:"user"`
	RecentActivity []UserActivityItem `json:"recent_activity"`
}

// HandleGetUserProfile returns a user's public profile with recent activity.
// Only accessible to members of shared projects.
func (s *Server) HandleGetUserProfile(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	viewerID := r.Context().Value(UserIDKey).(int64)
	targetUserID, err := strconv.ParseInt(chi.URLParam(r, "userId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user ID", "invalid_input")
		return
	}

	// Verify viewer shares at least one project with target user
	var sharedCount int
	if err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM project_members pm1
		JOIN project_members pm2 ON pm2.project_id = pm1.project_id AND pm2.user_id = $2
		WHERE pm1.user_id = $1
	`, viewerID, targetUserID).Scan(&sharedCount); err != nil || sharedCount == 0 {
		respondError(w, http.StatusForbidden, "access denied", "forbidden")
		return
	}

	// Fetch user info
	var u UserProfileInfo
	if err := s.db.QueryRowContext(ctx, `
		SELECT id, name, user_name, email FROM users WHERE id = $1 AND deleted_at IS NULL
	`, targetUserID).Scan(&u.ID, &u.Name, &u.UserName, &u.Email); err != nil {
		respondError(w, http.StatusNotFound, "user not found", "not_found")
		return
	}

	// Collect recent activity (visible only for shared projects)
	activity, err := s.fetchUserActivity(ctx, viewerID, targetUserID)
	if err != nil {
		s.logger.Error("Failed to fetch user activity", zap.Error(err), zap.Int64("target", targetUserID))
		activity = []UserActivityItem{}
	}

	respondJSON(w, http.StatusOK, UserProfileResponse{
		User:           u,
		RecentActivity: activity,
	})
}

// fetchUserActivity returns recent activity for a user across shared projects.
func (s *Server) fetchUserActivity(ctx context.Context, viewerID, targetUserID int64) ([]UserActivityItem, error) {
	items := []UserActivityItem{}

	// Task comments
	rows, err := s.db.QueryContext(ctx, `
		SELECT tc.id, t.title, t.project_id, p.name, t.task_number, tc.created_at
		FROM task_comments tc
		JOIN tasks t ON t.id = tc.task_id
		JOIN projects p ON p.id = t.project_id
		JOIN project_members pm ON pm.project_id = t.project_id AND pm.user_id = $1
		WHERE tc.user_id = $2
		ORDER BY tc.created_at DESC
		LIMIT 20
	`, viewerID, targetUserID)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var item UserActivityItem
			var taskNumber int64
			if rows.Scan(&item.EntityID, &item.EntityTitle, &item.ProjectID, &item.ProjectName, &taskNumber, &item.CreatedAt) == nil {
				item.Type = "task_comment"
				item.Link = "/app/projects/" + int64ToStr(item.ProjectID) + "/tasks/" + int64ToStr(taskNumber)
				items = append(items, item)
			}
		}
	}

	// Wiki pages created
	wikiRows, err := s.db.QueryContext(ctx, `
		SELECT wp.id, wp.title, wp.project_id, p.name, wp.created_at
		FROM wiki_pages wp
		JOIN projects p ON p.id = wp.project_id
		JOIN project_members pm ON pm.project_id = wp.project_id AND pm.user_id = $1
		WHERE wp.created_by = $2
		ORDER BY wp.created_at DESC
		LIMIT 20
	`, viewerID, targetUserID)
	if err == nil {
		defer wikiRows.Close()
		for wikiRows.Next() {
			var item UserActivityItem
			if wikiRows.Scan(&item.EntityID, &item.EntityTitle, &item.ProjectID, &item.ProjectName, &item.CreatedAt) == nil {
				item.Type = "wiki_page"
				item.Link = "/app/projects/" + int64ToStr(item.ProjectID) + "?tab=wiki"
				items = append(items, item)
			}
		}
	}

	// Annotation comments
	annRows, err := s.db.QueryContext(ctx, `
		SELECT wac.id, wp.title, wp.project_id, p.name, wac.created_at
		FROM wiki_annotation_comments wac
		JOIN wiki_annotations wa ON wa.id = wac.annotation_id
		JOIN wiki_pages wp ON wp.id = wa.wiki_page_id
		JOIN projects p ON p.id = wp.project_id
		JOIN project_members pm ON pm.project_id = wp.project_id AND pm.user_id = $1
		WHERE wac.author_id = $2
		ORDER BY wac.created_at DESC
		LIMIT 20
	`, viewerID, targetUserID)
	if err == nil {
		defer annRows.Close()
		for annRows.Next() {
			var item UserActivityItem
			if annRows.Scan(&item.EntityID, &item.EntityTitle, &item.ProjectID, &item.ProjectName, &item.CreatedAt) == nil {
				item.Type = "annotation_comment"
				item.Link = "/app/projects/" + int64ToStr(item.ProjectID) + "?tab=wiki"
				items = append(items, item)
			}
		}
	}

	// Sort by created_at descending and take top 30
	sortActivityItems(items)
	if len(items) > 30 {
		items = items[:30]
	}
	return items, nil
}

// sortActivityItems sorts by CreatedAt descending (insertion sort-friendly for small slices).
func sortActivityItems(items []UserActivityItem) {
	for i := 1; i < len(items); i++ {
		for j := i; j > 0 && items[j].CreatedAt.After(items[j-1].CreatedAt); j-- {
			items[j], items[j-1] = items[j-1], items[j]
		}
	}
}
