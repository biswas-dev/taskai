package api

import (
	"context"
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// ── Response types ──────────────────────────────────────────────────────────

type AnalyticsOverview struct {
	PeriodDays              int              `json:"period_days"`
	ActiveUsers             int              `json:"active_users"`
	TotalLogins             int              `json:"total_logins"`
	TotalPageViews          int              `json:"total_page_views"`
	TotalAPIRequests        int              `json:"total_api_requests"`
	TasksCreated            int              `json:"tasks_created"`
	WikiPagesCreated        int              `json:"wiki_pages_created"`
	WikiEdits               int              `json:"wiki_edits"`
	CommentsAdded           int              `json:"comments_added"`
	AvgSessionDurationMins  float64          `json:"avg_session_duration_minutes"`
	DailyActiveUsers        []DailyUserCount `json:"daily_active_users"`
}

type DailyUserCount struct {
	Date  string `json:"date"`
	Count int    `json:"count"`
}

type AnalyticsUserRow struct {
	UserID            int64   `json:"user_id"`
	Email             string  `json:"email"`
	Name              string  `json:"name"`
	LoginCount        int     `json:"login_count"`
	PageViewCount     int     `json:"page_view_count"`
	APIRequestCount   int     `json:"api_request_count"`
	TasksCreated      int     `json:"tasks_created"`
	CommentsAdded     int     `json:"comments_added"`
	WikiEdits         int     `json:"wiki_edits"`
	TotalSessionMins  float64 `json:"total_session_minutes"`
	LastActiveAt      *string `json:"last_active_at"`
}

type AnalyticsUserDetail struct {
	User            AnalyticsUserRow       `json:"user"`
	RecentLogins    []AnalyticsLoginItem     `json:"recent_logins"`
	RecentPageViews []PageViewItem         `json:"recent_page_views"`
	RecentActivity  []ActivityEntry        `json:"recent_activity"`
	APIKeys         []AnalyticsAPIKeyUsage `json:"api_keys"`
}

type AnalyticsLoginItem struct {
	ID           int64  `json:"id"`
	ActivityType string `json:"activity_type"`
	IPAddress    string `json:"ip_address,omitempty"`
	UserAgent    string `json:"user_agent,omitempty"`
	CreatedAt    string `json:"created_at"`
}

type PageViewItem struct {
	ID         int64  `json:"id"`
	Path       string `json:"path"`
	Referrer   string `json:"referrer,omitempty"`
	DurationMs *int   `json:"duration_ms,omitempty"`
	CreatedAt  string `json:"created_at"`
}

type AnalyticsAPIKeyUsage struct {
	APIKeyID     int64          `json:"api_key_id"`
	KeyName      string         `json:"key_name"`
	KeyPrefix    string         `json:"key_prefix"`
	UserID       int64          `json:"user_id"`
	UserEmail    string         `json:"user_email"`
	RequestCount int            `json:"request_count"`
	LastUsedAt   *string        `json:"last_used_at"`
	TopPaths     []APIPathCount `json:"top_paths"`
}

type APIPathCount struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	Count  int    `json:"count"`
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func parseDays(r *http.Request) int {
	days := 30
	if d, err := strconv.Atoi(r.URL.Query().Get("days")); err == nil && d > 0 && d <= 90 {
		days = d
	}
	return days
}

// ── Handlers ────────────────────────────────────────────────────────────────

// HandleAnalyticsOverview returns aggregate engagement stats for the given period.
func (s *Server) HandleAnalyticsOverview(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	if !s.isAdmin(ctx, userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	days := parseDays(r)
	since := time.Now().AddDate(0, 0, -days)

	overview := AnalyticsOverview{PeriodDays: days}

	// Active users & total logins from user_activity
	row := s.db.QueryRowContext(ctx, `
		SELECT COUNT(DISTINCT user_id), COUNT(*)
		FROM user_activity
		WHERE activity_type = 'login' AND created_at > $1
	`, since)
	_ = row.Scan(&overview.ActiveUsers, &overview.TotalLogins)

	// Page views
	row = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM page_views WHERE created_at > $1`, since)
	_ = row.Scan(&overview.TotalPageViews)

	// API requests
	row = s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM api_request_log WHERE created_at > $1`, since)
	_ = row.Scan(&overview.TotalAPIRequests)

	// Content creation from project_activity
	row = s.db.QueryRowContext(ctx, `
		SELECT
			COALESCE(SUM(CASE WHEN action = 'task_created' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN action = 'wiki_page_created' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN action = 'wiki_page_updated' THEN 1 ELSE 0 END), 0),
			COALESCE(SUM(CASE WHEN action = 'comment_added' THEN 1 ELSE 0 END), 0)
		FROM project_activity WHERE created_at > $1
	`, since)
	_ = row.Scan(&overview.TasksCreated, &overview.WikiPagesCreated, &overview.WikiEdits, &overview.CommentsAdded)

	// Average session duration
	row = s.db.QueryRowContext(ctx, `
		SELECT COALESCE(AVG(total_duration_ms) / 60000.0, 0)
		FROM user_sessions WHERE started_at > $1
	`, since)
	_ = row.Scan(&overview.AvgSessionDurationMins)

	// Daily active users
	rows, err := s.db.QueryContext(ctx, `
		SELECT DATE(created_at) AS d, COUNT(DISTINCT user_id)
		FROM user_activity
		WHERE activity_type = 'login' AND created_at > $1
		GROUP BY d ORDER BY d
	`, since)
	if err == nil {
		defer rows.Close()
		for rows.Next() {
			var dc DailyUserCount
			if err := rows.Scan(&dc.Date, &dc.Count); err == nil {
				overview.DailyActiveUsers = append(overview.DailyActiveUsers, dc)
			}
		}
	}
	if overview.DailyActiveUsers == nil {
		overview.DailyActiveUsers = []DailyUserCount{}
	}

	respondJSON(w, http.StatusOK, overview)
}

// HandleAnalyticsUsers returns a user engagement leaderboard.
func (s *Server) HandleAnalyticsUsers(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	if !s.isAdmin(ctx, userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	days := parseDays(r)
	since := time.Now().AddDate(0, 0, -days)

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			u.id,
			u.email,
			COALESCE(u.first_name || ' ' || u.last_name, u.name, u.email) AS display_name,
			COALESCE(login_counts.cnt, 0) AS login_count,
			COALESCE(pv_counts.cnt, 0)    AS page_view_count,
			COALESCE(api_counts.cnt, 0)   AS api_request_count,
			COALESCE(task_counts.cnt, 0)  AS tasks_created,
			COALESCE(comment_counts.cnt, 0) AS comments_added,
			COALESCE(wiki_counts.cnt, 0)  AS wiki_edits,
			COALESCE(session_totals.total_mins, 0) AS total_session_minutes,
			last_activity.last_at
		FROM users u
		LEFT JOIN (
			SELECT user_id, COUNT(*) AS cnt FROM user_activity
			WHERE activity_type = 'login' AND created_at > $1 GROUP BY user_id
		) login_counts ON login_counts.user_id = u.id
		LEFT JOIN (
			SELECT user_id, COUNT(*) AS cnt FROM page_views
			WHERE created_at > $1 GROUP BY user_id
		) pv_counts ON pv_counts.user_id = u.id
		LEFT JOIN (
			SELECT user_id, COUNT(*) AS cnt FROM api_request_log
			WHERE created_at > $1 GROUP BY user_id
		) api_counts ON api_counts.user_id = u.id
		LEFT JOIN (
			SELECT user_id, COUNT(*) AS cnt FROM project_activity
			WHERE action = 'task_created' AND created_at > $1 GROUP BY user_id
		) task_counts ON task_counts.user_id = u.id
		LEFT JOIN (
			SELECT user_id, COUNT(*) AS cnt FROM project_activity
			WHERE action = 'comment_added' AND created_at > $1 GROUP BY user_id
		) comment_counts ON comment_counts.user_id = u.id
		LEFT JOIN (
			SELECT user_id, COUNT(*) AS cnt FROM project_activity
			WHERE action IN ('wiki_page_created', 'wiki_page_updated') AND created_at > $1 GROUP BY user_id
		) wiki_counts ON wiki_counts.user_id = u.id
		LEFT JOIN (
			SELECT user_id, SUM(total_duration_ms) / 60000.0 AS total_mins FROM user_sessions
			WHERE started_at > $1 GROUP BY user_id
		) session_totals ON session_totals.user_id = u.id
		LEFT JOIN (
			SELECT user_id, MAX(created_at) AS last_at FROM user_activity GROUP BY user_id
		) last_activity ON last_activity.user_id = u.id
		WHERE u.deleted_at IS NULL
		ORDER BY login_count DESC, page_view_count DESC
	`, since)
	if err != nil {
		s.logger.Error("Failed to query analytics users", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch analytics", "internal_error")
		return
	}
	defer rows.Close()

	result := []AnalyticsUserRow{}
	for rows.Next() {
		var u AnalyticsUserRow
		var lastAt *time.Time
		if err := rows.Scan(
			&u.UserID, &u.Email, &u.Name,
			&u.LoginCount, &u.PageViewCount, &u.APIRequestCount,
			&u.TasksCreated, &u.CommentsAdded, &u.WikiEdits,
			&u.TotalSessionMins, &lastAt,
		); err != nil {
			s.logger.Error("Failed to scan analytics user row", zap.Error(err))
			continue
		}
		if lastAt != nil {
			t := lastAt.Format(time.RFC3339)
			u.LastActiveAt = &t
		}
		result = append(result, u)
	}

	respondJSON(w, http.StatusOK, result)
}

// HandleAnalyticsUserDetail returns comprehensive activity data for a single user.
func (s *Server) HandleAnalyticsUserDetail(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	callerID := r.Context().Value(UserIDKey).(int64)
	if !s.isAdmin(ctx, callerID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	targetID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid user ID", "invalid_input")
		return
	}

	days := parseDays(r)
	since := time.Now().AddDate(0, 0, -days)

	detail := AnalyticsUserDetail{}

	// User summary (same as users list but for one user)
	row := s.db.QueryRowContext(ctx, `
		SELECT
			u.id, u.email,
			COALESCE(u.first_name || ' ' || u.last_name, u.name, u.email),
			COALESCE(lc.cnt, 0), COALESCE(pv.cnt, 0), COALESCE(ar.cnt, 0),
			COALESCE(tc.cnt, 0), COALESCE(cc.cnt, 0), COALESCE(wc.cnt, 0),
			COALESCE(st.total_mins, 0),
			la.last_at
		FROM users u
		LEFT JOIN (SELECT user_id, COUNT(*) cnt FROM user_activity WHERE activity_type='login' AND created_at>$2 GROUP BY user_id) lc ON lc.user_id=u.id
		LEFT JOIN (SELECT user_id, COUNT(*) cnt FROM page_views WHERE created_at>$2 GROUP BY user_id) pv ON pv.user_id=u.id
		LEFT JOIN (SELECT user_id, COUNT(*) cnt FROM api_request_log WHERE created_at>$2 GROUP BY user_id) ar ON ar.user_id=u.id
		LEFT JOIN (SELECT user_id, COUNT(*) cnt FROM project_activity WHERE action='task_created' AND created_at>$2 GROUP BY user_id) tc ON tc.user_id=u.id
		LEFT JOIN (SELECT user_id, COUNT(*) cnt FROM project_activity WHERE action='comment_added' AND created_at>$2 GROUP BY user_id) cc ON cc.user_id=u.id
		LEFT JOIN (SELECT user_id, COUNT(*) cnt FROM project_activity WHERE action IN ('wiki_page_created','wiki_page_updated') AND created_at>$2 GROUP BY user_id) wc ON wc.user_id=u.id
		LEFT JOIN (SELECT user_id, SUM(total_duration_ms)/60000.0 total_mins FROM user_sessions WHERE started_at>$2 GROUP BY user_id) st ON st.user_id=u.id
		LEFT JOIN (SELECT user_id, MAX(created_at) last_at FROM user_activity GROUP BY user_id) la ON la.user_id=u.id
		WHERE u.id = $1
	`, targetID, since)

	var lastAt *time.Time
	if err := row.Scan(
		&detail.User.UserID, &detail.User.Email, &detail.User.Name,
		&detail.User.LoginCount, &detail.User.PageViewCount, &detail.User.APIRequestCount,
		&detail.User.TasksCreated, &detail.User.CommentsAdded, &detail.User.WikiEdits,
		&detail.User.TotalSessionMins, &lastAt,
	); err != nil {
		respondError(w, http.StatusNotFound, "user not found", "not_found")
		return
	}
	if lastAt != nil {
		t := lastAt.Format(time.RFC3339)
		detail.User.LastActiveAt = &t
	}

	// Recent logins
	loginRows, err := s.db.QueryContext(ctx, `
		SELECT id, activity_type, COALESCE(ip_address, ''), COALESCE(user_agent, ''), created_at
		FROM user_activity WHERE user_id = $1 AND created_at > $2
		ORDER BY created_at DESC LIMIT 50
	`, targetID, since)
	if err == nil {
		defer loginRows.Close()
		for loginRows.Next() {
			var item AnalyticsLoginItem
			var createdAt time.Time
			if err := loginRows.Scan(&item.ID, &item.ActivityType, &item.IPAddress, &item.UserAgent, &createdAt); err == nil {
				item.CreatedAt = createdAt.Format(time.RFC3339)
				detail.RecentLogins = append(detail.RecentLogins, item)
			}
		}
	}
	if detail.RecentLogins == nil {
		detail.RecentLogins = []AnalyticsLoginItem{}
	}

	// Recent page views
	pvRows, err := s.db.QueryContext(ctx, `
		SELECT id, path, COALESCE(referrer, ''), duration_ms, created_at
		FROM page_views WHERE user_id = $1 AND created_at > $2
		ORDER BY created_at DESC LIMIT 50
	`, targetID, since)
	if err == nil {
		defer pvRows.Close()
		for pvRows.Next() {
			var item PageViewItem
			var createdAt time.Time
			if err := pvRows.Scan(&item.ID, &item.Path, &item.Referrer, &item.DurationMs, &createdAt); err == nil {
				item.CreatedAt = createdAt.Format(time.RFC3339)
				detail.RecentPageViews = append(detail.RecentPageViews, item)
			}
		}
	}
	if detail.RecentPageViews == nil {
		detail.RecentPageViews = []PageViewItem{}
	}

	// Recent project activity
	actRows, err := s.db.QueryContext(ctx, `
		SELECT pa.id, pa.project_id, pa.user_id,
		       COALESCE(u.first_name || ' ' || u.last_name, u.name, u.email) AS user_name,
		       pa.action, pa.entity_type, pa.entity_id, pa.entity_title, pa.details, pa.created_at
		FROM project_activity pa
		JOIN users u ON u.id = pa.user_id
		WHERE pa.user_id = $1 AND pa.created_at > $2
		ORDER BY pa.created_at DESC LIMIT 50
	`, targetID, since)
	if err == nil {
		defer actRows.Close()
		for actRows.Next() {
			var e ActivityEntry
			var userName, entityTitle, details *string
			if err := actRows.Scan(&e.ID, &e.ProjectID, &e.UserID, &userName, &e.Action, &e.EntityType, &e.EntityID, &entityTitle, &details, &e.CreatedAt); err == nil {
				e.UserName = userName
				e.EntityTitle = entityTitle
				if details != nil && *details != "" {
					e.Details = json.RawMessage(*details)
				}
				detail.RecentActivity = append(detail.RecentActivity, e)
			}
		}
	}
	if detail.RecentActivity == nil {
		detail.RecentActivity = []ActivityEntry{}
	}

	// API key usage
	keyRows, err := s.db.QueryContext(ctx, `
		SELECT
			ak.id, ak.name, ak.key_prefix, ak.user_id, u.email,
			COALESCE(rl.cnt, 0),
			ak.last_used_at
		FROM api_keys ak
		JOIN users u ON u.id = ak.user_id
		LEFT JOIN (
			SELECT api_key_id, COUNT(*) cnt FROM api_request_log
			WHERE created_at > $2 AND api_key_id IS NOT NULL GROUP BY api_key_id
		) rl ON rl.api_key_id = ak.id
		WHERE ak.user_id = $1
		ORDER BY rl.cnt DESC NULLS LAST
	`, targetID, since)
	if err == nil {
		defer keyRows.Close()
		for keyRows.Next() {
			var k AnalyticsAPIKeyUsage
			var lastUsed *time.Time
			if err := keyRows.Scan(&k.APIKeyID, &k.KeyName, &k.KeyPrefix, &k.UserID, &k.UserEmail, &k.RequestCount, &lastUsed); err == nil {
				if lastUsed != nil {
					t := lastUsed.Format(time.RFC3339)
					k.LastUsedAt = &t
				}
				// Get top paths for this key
				k.TopPaths = s.getTopPathsForKey(ctx, k.APIKeyID, since)
				detail.APIKeys = append(detail.APIKeys, k)
			}
		}
	}
	if detail.APIKeys == nil {
		detail.APIKeys = []AnalyticsAPIKeyUsage{}
	}

	respondJSON(w, http.StatusOK, detail)
}

// HandleAnalyticsAPIKeys returns an API key usage leaderboard.
func (s *Server) HandleAnalyticsAPIKeys(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	if !s.isAdmin(ctx, userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	days := parseDays(r)
	since := time.Now().AddDate(0, 0, -days)

	rows, err := s.db.QueryContext(ctx, `
		SELECT
			ak.id, ak.name, ak.key_prefix, ak.user_id, u.email,
			COALESCE(rl.cnt, 0),
			ak.last_used_at
		FROM api_keys ak
		JOIN users u ON u.id = ak.user_id
		LEFT JOIN (
			SELECT api_key_id, COUNT(*) cnt FROM api_request_log
			WHERE created_at > $1 AND api_key_id IS NOT NULL GROUP BY api_key_id
		) rl ON rl.api_key_id = ak.id
		ORDER BY rl.cnt DESC NULLS LAST
	`, since)
	if err != nil {
		s.logger.Error("Failed to query API key analytics", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch analytics", "internal_error")
		return
	}
	defer rows.Close()

	result := []AnalyticsAPIKeyUsage{}
	for rows.Next() {
		var k AnalyticsAPIKeyUsage
		var lastUsed *time.Time
		if err := rows.Scan(&k.APIKeyID, &k.KeyName, &k.KeyPrefix, &k.UserID, &k.UserEmail, &k.RequestCount, &lastUsed); err != nil {
			continue
		}
		if lastUsed != nil {
			t := lastUsed.Format(time.RFC3339)
			k.LastUsedAt = &t
		}
		k.TopPaths = s.getTopPathsForKey(ctx, k.APIKeyID, since)
		result = append(result, k)
	}

	respondJSON(w, http.StatusOK, result)
}

func (s *Server) getTopPathsForKey(ctx context.Context, apiKeyID int64, since time.Time) []APIPathCount {
	rows, err := s.db.QueryContext(ctx, `
		SELECT method, path, COUNT(*) AS cnt
		FROM api_request_log
		WHERE api_key_id = $1 AND created_at > $2
		GROUP BY method, path
		ORDER BY cnt DESC LIMIT 10
	`, apiKeyID, since)
	if err != nil {
		return []APIPathCount{}
	}
	defer rows.Close()

	var paths []APIPathCount
	for rows.Next() {
		var p APIPathCount
		if err := rows.Scan(&p.Method, &p.Path, &p.Count); err == nil {
			paths = append(paths, p)
		}
	}
	if paths == nil {
		return []APIPathCount{}
	}
	return paths
}

// HandleTrackPageView records a page view from the frontend SPA.
// This is available to all authenticated users, not just admins.
func (s *Server) HandleTrackPageView(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID, ok := r.Context().Value(UserIDKey).(int64)
	if !ok || userID == 0 {
		respondError(w, http.StatusUnauthorized, "unauthorized", "unauthorized")
		return
	}

	var req struct {
		Path       string `json:"path"`
		Referrer   string `json:"referrer"`
		SessionID  string `json:"session_id"`
		DurationMs *int   `json:"duration_ms"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "invalid_input")
		return
	}
	if req.Path == "" || req.SessionID == "" {
		respondError(w, http.StatusBadRequest, "path and session_id required", "invalid_input")
		return
	}

	// Truncate long values
	if len(req.Path) > 500 {
		req.Path = req.Path[:500]
	}
	if len(req.Referrer) > 500 {
		req.Referrer = req.Referrer[:500]
	}
	if len(req.SessionID) > 100 {
		req.SessionID = req.SessionID[:100]
	}

	// Insert page view
	_, err := s.db.ExecContext(ctx, `
		INSERT INTO page_views (user_id, path, referrer, session_id, duration_ms)
		VALUES ($1, $2, $3, $4, $5)
	`, userID, req.Path, req.Referrer, req.SessionID, req.DurationMs)
	if err != nil {
		s.logger.Error("Failed to insert page view", zap.Error(err))
		w.WriteHeader(http.StatusNoContent)
		return
	}

	// Upsert session
	durationAdd := 0
	if req.DurationMs != nil {
		durationAdd = *req.DurationMs
	}
	_, err = s.db.ExecContext(ctx, `
		INSERT INTO user_sessions (user_id, session_id, page_count, total_duration_ms)
		VALUES ($1, $2, 1, $3)
		ON CONFLICT (session_id) DO UPDATE SET
			last_seen_at = NOW(),
			page_count = user_sessions.page_count + 1,
			total_duration_ms = user_sessions.total_duration_ms + $3
	`, userID, req.SessionID, durationAdd)
	if err != nil {
		s.logger.Error("Failed to upsert session", zap.Error(err))
	}

	w.WriteHeader(http.StatusNoContent)
}
