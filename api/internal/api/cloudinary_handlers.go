package api

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

var nonAlphanumericRe = regexp.MustCompile(`[^a-z0-9]+`)

// slugifyProjectName converts a project name into a URL-safe slug for Cloudinary folders.
// Falls back to "project-{id}" if the result is empty.
func slugifyProjectName(name string, projectID int64) string {
	slug := strings.ToLower(name)
	slug = nonAlphanumericRe.ReplaceAllString(slug, "-")
	slug = strings.Trim(slug, "-")
	if slug == "" {
		return fmt.Sprintf("project-%d", projectID)
	}
	return slug
}

// convertToPostgresQuery converts SQLite-style ? placeholders to Postgres $1, $2, etc.
func convertToPostgresQuery(query string) string {
	count := 0
	result := strings.Builder{}
	for i := 0; i < len(query); i++ {
		if query[i] == '?' {
			count++
			result.WriteString(fmt.Sprintf("$%d", count))
		} else {
			result.WriteByte(query[i])
		}
	}
	return result.String()
}

// Cloudinary credential types

type CloudinaryCredential struct {
	ID                  int64      `json:"id"`
	UserID              int64      `json:"user_id"`
	CloudName           string     `json:"cloud_name"`
	APIKey              string     `json:"api_key"`
	MaxFileSizeMB       int        `json:"max_file_size_mb"`
	Status              string     `json:"status"`
	LastCheckedAt       *time.Time `json:"last_checked_at"`
	LastError           string     `json:"last_error"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type SaveCloudinaryCredentialRequest struct {
	CloudName     string `json:"cloud_name"`
	APIKey        string `json:"api_key"`
	APISecret     string `json:"api_secret"`
	MaxFileSizeMB *int   `json:"max_file_size_mb,omitempty"`
}

type UploadSignatureResponse struct {
	Signature string `json:"signature"`
	Timestamp int64  `json:"timestamp"`
	CloudName string `json:"cloud_name"`
	APIKey    string `json:"api_key"`
	Folder    string `json:"folder"`
	PublicID  string `json:"public_id"`
}

type TaskAttachment struct {
	ID                 int64     `json:"id"`
	TaskID             int64     `json:"task_id"`
	ProjectID          int64     `json:"project_id"`
	UserID             int64     `json:"user_id"`
	Filename           string    `json:"filename"`
	AltName            string    `json:"alt_name"`
	FileType           string    `json:"file_type"`
	ContentType        string    `json:"content_type"`
	FileSize           int64     `json:"file_size"`
	CloudinaryURL      string    `json:"cloudinary_url"`
	CloudinaryPublicID string    `json:"cloudinary_public_id"`
	CreatedAt          time.Time `json:"created_at"`
	UserName           *string   `json:"user_name,omitempty"`
}

type WikiPageAttachment struct {
	ID                 int64     `json:"id"`
	WikiPageID         int64     `json:"wiki_page_id"`
	ProjectID          int64     `json:"project_id"`
	UserID             int64     `json:"user_id"`
	Filename           string    `json:"filename"`
	AltName            string    `json:"alt_name"`
	FileType           string    `json:"file_type"`
	ContentType        string    `json:"content_type"`
	FileSize           int64     `json:"file_size"`
	CloudinaryURL      string    `json:"cloudinary_url"`
	CloudinaryPublicID string    `json:"cloudinary_public_id"`
	CreatedAt          time.Time `json:"created_at"`
	UserName           *string   `json:"user_name,omitempty"`
}

type CreateAttachmentRequest struct {
	Filename           string `json:"filename"`
	AltName            string `json:"alt_name"`
	FileType           string `json:"file_type"`
	ContentType        string `json:"content_type"`
	FileSize           int64  `json:"file_size"`
	CloudinaryURL      string `json:"cloudinary_url"`
	CloudinaryPublicID string `json:"cloudinary_public_id"`
}

type UpdateAttachmentRequest struct {
	AltName *string `json:"alt_name"`
}

type StorageUsage struct {
	UserID    int64  `json:"user_id"`
	UserName  string `json:"user_name"`
	FileCount int    `json:"file_count"`
	TotalSize int64  `json:"total_size"`
}

type AssetResponse struct {
	TaskAttachment
	IsOwner bool `json:"is_owner"`
}

// HandleGetCloudinaryCredential returns the current user's Cloudinary credentials
func (s *Server) HandleGetCloudinaryCredential(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	var cred CloudinaryCredential
	query := convertToPostgresQuery(`SELECT id, user_id, cloud_name, api_key, max_file_size_mb,
		        status, last_checked_at, last_error, consecutive_failures,
		        created_at, updated_at
		 FROM cloudinary_credentials WHERE user_id = $1`)
	err := s.db.QueryRowContext(ctx, query, userID).Scan(&cred.ID, &cred.UserID, &cred.CloudName, &cred.APIKey, &cred.MaxFileSizeMB,
		&cred.Status, &cred.LastCheckedAt, &cred.LastError, &cred.ConsecutiveFailures,
		&cred.CreatedAt, &cred.UpdatedAt)

	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}
	if err != nil {
		s.logger.Error("Failed to fetch cloudinary credentials", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch credentials", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, cred)
}

// HandleSaveCloudinaryCredential creates or updates the current user's Cloudinary credentials
func (s *Server) HandleSaveCloudinaryCredential(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	var req SaveCloudinaryCredentialRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "bad_request")
		return
	}

	if req.CloudName == "" || req.APIKey == "" || req.APISecret == "" {
		respondError(w, http.StatusBadRequest, "cloud_name, api_key, and api_secret are required", "validation_error")
		return
	}

	maxSize := 10
	if req.MaxFileSizeMB != nil && *req.MaxFileSizeMB > 0 {
		maxSize = *req.MaxFileSizeMB
	}

	_, err := s.db.ExecContext(ctx,
		`INSERT INTO cloudinary_credentials (user_id, cloud_name, api_key, api_secret, max_file_size_mb, updated_at)
		 VALUES ($1, $2, $3, $4, $5, CURRENT_TIMESTAMP)
		 ON CONFLICT(user_id) DO UPDATE SET
		   cloud_name = excluded.cloud_name,
		   api_key = excluded.api_key,
		   api_secret = excluded.api_secret,
		   max_file_size_mb = excluded.max_file_size_mb,
		   updated_at = CURRENT_TIMESTAMP`,
		userID, req.CloudName, req.APIKey, req.APISecret, maxSize,
	)
	if err != nil {
		s.logger.Error("Failed to save cloudinary credentials", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to save credentials", "internal_error")
		return
	}

	s.logger.Info("Cloudinary credentials saved", zap.Int64("user_id", userID))

	// Test connection with the provided credentials
	status, lastError := testCloudinaryConnection(ctx, req.CloudName, req.APIKey, req.APISecret)

	now := time.Now()
	consecutiveFailures := 0
	if status == "error" {
		consecutiveFailures = 1
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE cloudinary_credentials
		 SET status = $1, last_checked_at = $2, last_error = $3, consecutive_failures = $4
		 WHERE user_id = $5`,
		status, now, lastError, consecutiveFailures, userID,
	)
	if err != nil {
		s.logger.Error("Failed to update cloudinary health status", zap.Error(err))
	}

	// Return the full credential object
	var cred CloudinaryCredential
	err = s.db.QueryRowContext(ctx,
		`SELECT id, user_id, cloud_name, api_key, max_file_size_mb,
		        status, last_checked_at, last_error, consecutive_failures,
		        created_at, updated_at
		 FROM cloudinary_credentials WHERE user_id = $1`, userID,
	).Scan(&cred.ID, &cred.UserID, &cred.CloudName, &cred.APIKey, &cred.MaxFileSizeMB,
		&cred.Status, &cred.LastCheckedAt, &cred.LastError, &cred.ConsecutiveFailures,
		&cred.CreatedAt, &cred.UpdatedAt)
	if err != nil {
		s.logger.Error("Failed to fetch saved cloudinary credentials", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "credentials saved but failed to retrieve", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, cred)
}

// HandleDeleteCloudinaryCredential removes the current user's Cloudinary credentials
func (s *Server) HandleDeleteCloudinaryCredential(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	_, err := s.db.ExecContext(ctx,
		`DELETE FROM cloudinary_credentials WHERE user_id = $1`, userID,
	)
	if err != nil {
		s.logger.Error("Failed to delete cloudinary credentials", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to delete credentials", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Cloudinary credentials deleted"})
}

// testCloudinaryConnection tests a Cloudinary connection by calling the ping API
func testCloudinaryConnection(ctx context.Context, cloudName, apiKey, apiSecret string) (status string, lastError string) {
	url := fmt.Sprintf("https://api.cloudinary.com/v1_1/%s/ping", cloudName)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "error", fmt.Sprintf("failed to create request: %v", err)
	}
	req.SetBasicAuth(apiKey, apiSecret)

	client := &http.Client{Timeout: 8 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "error", fmt.Sprintf("connection failed: %v", err)
	}
	defer resp.Body.Close()
	io.Copy(io.Discard, resp.Body)

	if resp.StatusCode == http.StatusOK {
		return "connected", ""
	}
	return "error", fmt.Sprintf("Cloudinary returned HTTP %d", resp.StatusCode)
}

// HandleTestCloudinaryConnection tests the stored Cloudinary credentials
func (s *Server) HandleTestCloudinaryConnection(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	var cloudName, apiKey, apiSecret string
	var consecutiveFailures int
	err := s.db.QueryRowContext(ctx,
		`SELECT cloud_name, api_key, api_secret, consecutive_failures
		 FROM cloudinary_credentials WHERE user_id = $1`, userID,
	).Scan(&cloudName, &apiKey, &apiSecret, &consecutiveFailures)

	if err == sql.ErrNoRows {
		respondError(w, http.StatusBadRequest, "no Cloudinary credentials configured", "no_credentials")
		return
	}
	if err != nil {
		s.logger.Error("Failed to fetch cloudinary credentials for test", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch credentials", "internal_error")
		return
	}

	status, lastError := testCloudinaryConnection(ctx, cloudName, apiKey, apiSecret)

	now := time.Now()
	if status == "connected" {
		consecutiveFailures = 0
	} else {
		consecutiveFailures++
		if consecutiveFailures >= 5 {
			status = "suspended"
		}
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE cloudinary_credentials
		 SET status = $1, last_checked_at = $2, last_error = $3, consecutive_failures = $4
		 WHERE user_id = $5`,
		status, now, lastError, consecutiveFailures, userID,
	)
	if err != nil {
		s.logger.Error("Failed to update cloudinary health status", zap.Error(err))
	}

	// Return the full credential
	var cred CloudinaryCredential
	err = s.db.QueryRowContext(ctx,
		`SELECT id, user_id, cloud_name, api_key, max_file_size_mb,
		        status, last_checked_at, last_error, consecutive_failures,
		        created_at, updated_at
		 FROM cloudinary_credentials WHERE user_id = $1`, userID,
	).Scan(&cred.ID, &cred.UserID, &cred.CloudName, &cred.APIKey, &cred.MaxFileSizeMB,
		&cred.Status, &cred.LastCheckedAt, &cred.LastError, &cred.ConsecutiveFailures,
		&cred.CreatedAt, &cred.UpdatedAt)
	if err != nil {
		s.logger.Error("Failed to fetch updated cloudinary credentials", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "test completed but failed to retrieve status", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, cred)
}

// HandleGetUploadSignature generates a Cloudinary upload signature for the current user.
// Requires exactly one of ?task_id= or ?page_id= query parameter.
func (s *Server) HandleGetUploadSignature(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	taskIDStr := r.URL.Query().Get("task_id")
	pageIDStr := r.URL.Query().Get("page_id")

	// Require exactly one of task_id or page_id
	if taskIDStr == "" && pageIDStr == "" {
		respondError(w, http.StatusBadRequest, "task_id or page_id query parameter is required", "bad_request")
		return
	}
	if taskIDStr != "" && pageIDStr != "" {
		respondError(w, http.StatusBadRequest, "provide either task_id or page_id, not both", "bad_request")
		return
	}

	var projectID int64
	var projectName string
	var publicID string

	if taskIDStr != "" {
		taskID, err := strconv.ParseInt(taskIDStr, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid task_id", "bad_request")
			return
		}

		// Look up task to get project_id
		err = s.db.QueryRowContext(ctx,
			`SELECT project_id FROM tasks WHERE id = $1`, taskID,
		).Scan(&projectID)
		if err == sql.ErrNoRows {
			respondError(w, http.StatusNotFound, "task not found", "not_found")
			return
		}
		if err != nil {
			s.logger.Error("Failed to look up task for signature", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to look up task", "internal_error")
			return
		}

		// Look up project name
		err = s.db.QueryRowContext(ctx,
			`SELECT name FROM projects WHERE id = $1`, projectID,
		).Scan(&projectName)
		if err != nil {
			s.logger.Error("Failed to look up project for signature", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to look up project", "internal_error")
			return
		}

		// Count existing attachments for this task
		var attachmentCount int
		err = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM task_attachments WHERE task_id = $1`, taskID,
		).Scan(&attachmentCount)
		if err != nil {
			s.logger.Error("Failed to count attachments", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to count attachments", "internal_error")
			return
		}

		if attachmentCount >= 99 {
			respondError(w, http.StatusBadRequest, "maximum 99 attachments per task", "attachment_limit_exceeded")
			return
		}

		publicID = fmt.Sprintf("%d_%02d", taskID, attachmentCount+1)
	} else {
		pageID, err := strconv.ParseInt(pageIDStr, 10, 64)
		if err != nil {
			respondError(w, http.StatusBadRequest, "invalid page_id", "bad_request")
			return
		}

		// Look up wiki page to get project_id
		err = s.db.QueryRowContext(ctx,
			`SELECT project_id FROM wiki_pages WHERE id = $1`, pageID,
		).Scan(&projectID)
		if err == sql.ErrNoRows {
			respondError(w, http.StatusNotFound, "wiki page not found", "not_found")
			return
		}
		if err != nil {
			s.logger.Error("Failed to look up wiki page for signature", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to look up wiki page", "internal_error")
			return
		}

		// Look up project name
		err = s.db.QueryRowContext(ctx,
			`SELECT name FROM projects WHERE id = $1`, projectID,
		).Scan(&projectName)
		if err != nil {
			s.logger.Error("Failed to look up project for signature", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to look up project", "internal_error")
			return
		}

		// Count existing wiki page attachments
		var attachmentCount int
		err = s.db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM wiki_page_attachments WHERE wiki_page_id = $1`, pageID,
		).Scan(&attachmentCount)
		if err != nil {
			s.logger.Error("Failed to count wiki page attachments", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to count attachments", "internal_error")
			return
		}

		if attachmentCount >= 100 {
			respondError(w, http.StatusBadRequest, "maximum 100 attachments per wiki page", "attachment_limit_exceeded")
			return
		}

		publicID = fmt.Sprintf("w%d_%03d", pageID, attachmentCount+1)
	}

	// Fetch Cloudinary credentials
	var cloudName, apiKey, apiSecret string
	err := s.db.QueryRowContext(ctx,
		`SELECT cloud_name, api_key, api_secret FROM cloudinary_credentials WHERE user_id = $1`, userID,
	).Scan(&cloudName, &apiKey, &apiSecret)

	if err == sql.ErrNoRows {
		respondError(w, http.StatusBadRequest, "no Cloudinary credentials configured", "no_credentials")
		return
	}
	if err != nil {
		s.logger.Error("Failed to fetch cloudinary credentials for signature", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to generate signature", "internal_error")
		return
	}

	// Build folder and public_id
	folder := "taskai/" + slugifyProjectName(projectName, projectID)

	// Sign with all params alphabetically: folder, public_id, timestamp
	timestamp := time.Now().Unix()
	signStr := fmt.Sprintf("folder=%s&public_id=%s&timestamp=%d%s", folder, publicID, timestamp, apiSecret)
	h := sha1.New()
	h.Write([]byte(signStr))
	signature := hex.EncodeToString(h.Sum(nil))

	respondJSON(w, http.StatusOK, UploadSignatureResponse{
		Signature: signature,
		Timestamp: timestamp,
		CloudName: cloudName,
		APIKey:    apiKey,
		Folder:    folder,
		PublicID:  publicID,
	})
}

// HandleListTaskAttachments returns all attachments for a task
func (s *Server) HandleListTaskAttachments(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	taskID, err := strconv.ParseInt(chi.URLParam(r, "taskId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid task ID", "bad_request")
		return
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT ta.id, ta.task_id, ta.project_id, ta.user_id, ta.filename, ta.alt_name,
		        ta.file_type, ta.content_type, ta.file_size,
		        ta.cloudinary_url, ta.cloudinary_public_id, ta.created_at,
		        u.name as user_name
		 FROM task_attachments ta
		 LEFT JOIN users u ON ta.user_id = u.id
		 WHERE ta.task_id = $1
		 ORDER BY ta.created_at DESC`, taskID,
	)
	if err != nil {
		s.logger.Error("Failed to list task attachments", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list attachments", "internal_error")
		return
	}
	defer rows.Close()

	attachments := []TaskAttachment{}
	for rows.Next() {
		var a TaskAttachment
		if err := rows.Scan(&a.ID, &a.TaskID, &a.ProjectID, &a.UserID,
			&a.Filename, &a.AltName, &a.FileType, &a.ContentType, &a.FileSize,
			&a.CloudinaryURL, &a.CloudinaryPublicID, &a.CreatedAt,
			&a.UserName); err != nil {
			s.logger.Error("Failed to scan attachment", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to scan attachment", "internal_error")
			return
		}
		attachments = append(attachments, a)
	}

	respondJSON(w, http.StatusOK, attachments)
}

// HandleCreateTaskAttachment stores a new attachment record after client-side Cloudinary upload
func (s *Server) HandleCreateTaskAttachment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	taskID, err := strconv.ParseInt(chi.URLParam(r, "taskId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid task ID", "bad_request")
		return
	}

	var req CreateAttachmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "bad_request")
		return
	}

	if req.Filename == "" || req.CloudinaryURL == "" || req.CloudinaryPublicID == "" {
		respondError(w, http.StatusBadRequest, "filename, cloudinary_url, and cloudinary_public_id are required", "validation_error")
		return
	}

	// Look up the project_id from the task
	var projectID int64
	err = s.db.QueryRowContext(ctx,
		`SELECT project_id FROM tasks WHERE id = $1`, taskID,
	).Scan(&projectID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "task not found", "not_found")
		return
	}
	if err != nil {
		s.logger.Error("Failed to look up task", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to look up task", "internal_error")
		return
	}

	// Enforce 99-attachment limit per task
	var attachmentCount int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM task_attachments WHERE task_id = $1`, taskID,
	).Scan(&attachmentCount)
	if err != nil {
		s.logger.Error("Failed to count attachments", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to count attachments", "internal_error")
		return
	}
	if attachmentCount >= 99 {
		respondError(w, http.StatusBadRequest, "maximum 99 attachments per task", "attachment_limit_exceeded")
		return
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO task_attachments (task_id, project_id, user_id, filename, alt_name, file_type, content_type, file_size, cloudinary_url, cloudinary_public_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		taskID, projectID, userID, req.Filename, req.AltName, req.FileType, req.ContentType, req.FileSize, req.CloudinaryURL, req.CloudinaryPublicID,
	)
	if err != nil {
		s.logger.Error("Failed to create attachment", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create attachment", "internal_error")
		return
	}

	id, _ := result.LastInsertId()
	s.logger.Info("Task attachment created",
		zap.Int64("id", id),
		zap.Int64("task_id", taskID),
		zap.Int64("user_id", userID),
		zap.String("filename", req.Filename),
	)

	attachment := TaskAttachment{
		ID:                 id,
		TaskID:             taskID,
		ProjectID:          projectID,
		UserID:             userID,
		Filename:           req.Filename,
		AltName:            req.AltName,
		FileType:           req.FileType,
		ContentType:        req.ContentType,
		FileSize:           req.FileSize,
		CloudinaryURL:      req.CloudinaryURL,
		CloudinaryPublicID: req.CloudinaryPublicID,
		CreatedAt:          time.Now(),
	}

	respondJSON(w, http.StatusCreated, attachment)
}

// HandleDeleteTaskAttachment removes an attachment (only the uploader can delete)
func (s *Server) HandleDeleteTaskAttachment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	attachmentID, err := strconv.ParseInt(chi.URLParam(r, "attachmentId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid attachment ID", "bad_request")
		return
	}

	// Verify ownership
	var ownerID int64
	err = s.db.QueryRowContext(ctx,
		`SELECT user_id FROM task_attachments WHERE id = $1`, attachmentID,
	).Scan(&ownerID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "attachment not found", "not_found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify attachment", "internal_error")
		return
	}
	if ownerID != userID {
		respondError(w, http.StatusForbidden, "you can only delete your own attachments", "forbidden")
		return
	}

	_, err = s.db.ExecContext(ctx,
		`DELETE FROM task_attachments WHERE id = $1`, attachmentID,
	)
	if err != nil {
		s.logger.Error("Failed to delete attachment", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to delete attachment", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Attachment deleted"})
}

// HandleListImages returns images accessible to the current user (own + shared project members)
func (s *Server) HandleListImages(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	query := r.URL.Query().Get("q")

	var rows *sql.Rows
	var err error

	if query != "" {
		searchPattern := "%" + query + "%"
		rows, err = s.db.QueryContext(ctx,
			`SELECT DISTINCT ta.id, ta.task_id, ta.project_id, ta.user_id, ta.filename, ta.alt_name,
			        ta.file_type, ta.content_type, ta.file_size,
			        ta.cloudinary_url, ta.cloudinary_public_id, ta.created_at,
			        u.name as user_name
			 FROM task_attachments ta
			 LEFT JOIN users u ON ta.user_id = u.id
			 WHERE ta.file_type = 'image' AND (
			   ta.user_id = $1 OR ta.user_id IN (
			     SELECT DISTINCT pm2.user_id FROM project_members pm1
			     JOIN project_members pm2 ON pm1.project_id = pm2.project_id
			     WHERE pm1.user_id = $2 AND pm2.user_id != $3
			   )
			 ) AND (ta.alt_name LIKE $4 OR ta.filename LIKE $5)
			 ORDER BY ta.created_at DESC
			 LIMIT 50`, userID, userID, userID, searchPattern, searchPattern,
		)
	} else {
		rows, err = s.db.QueryContext(ctx,
			`SELECT DISTINCT ta.id, ta.task_id, ta.project_id, ta.user_id, ta.filename, ta.alt_name,
			        ta.file_type, ta.content_type, ta.file_size,
			        ta.cloudinary_url, ta.cloudinary_public_id, ta.created_at,
			        u.name as user_name
			 FROM task_attachments ta
			 LEFT JOIN users u ON ta.user_id = u.id
			 WHERE ta.file_type = 'image' AND (
			   ta.user_id = $1 OR ta.user_id IN (
			     SELECT DISTINCT pm2.user_id FROM project_members pm1
			     JOIN project_members pm2 ON pm1.project_id = pm2.project_id
			     WHERE pm1.user_id = $2 AND pm2.user_id != $3
			   )
			 )
			 ORDER BY ta.created_at DESC
			 LIMIT 50`, userID, userID, userID,
		)
	}

	if err != nil {
		s.logger.Error("Failed to list images", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list images", "internal_error")
		return
	}
	defer rows.Close()

	images := []TaskAttachment{}
	for rows.Next() {
		var a TaskAttachment
		if err := rows.Scan(&a.ID, &a.TaskID, &a.ProjectID, &a.UserID,
			&a.Filename, &a.AltName, &a.FileType, &a.ContentType, &a.FileSize,
			&a.CloudinaryURL, &a.CloudinaryPublicID, &a.CreatedAt,
			&a.UserName); err != nil {
			s.logger.Error("Failed to scan image", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to scan image", "internal_error")
			return
		}
		images = append(images, a)
	}

	respondJSON(w, http.StatusOK, images)
}

// HandleUpdateAttachment updates an attachment's alt_name
func (s *Server) HandleUpdateAttachment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	attachmentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid attachment ID", "bad_request")
		return
	}

	// Verify ownership
	var ownerID int64
	err = s.db.QueryRowContext(ctx,
		`SELECT user_id FROM task_attachments WHERE id = $1`, attachmentID,
	).Scan(&ownerID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "attachment not found", "not_found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify attachment", "internal_error")
		return
	}
	if ownerID != userID {
		respondError(w, http.StatusForbidden, "you can only update your own attachments", "forbidden")
		return
	}

	var req UpdateAttachmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "bad_request")
		return
	}

	if req.AltName != nil {
		_, err = s.db.ExecContext(ctx,
			`UPDATE task_attachments SET alt_name = $1 WHERE id = $2`, *req.AltName, attachmentID,
		)
		if err != nil {
			s.logger.Error("Failed to update attachment", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to update attachment", "internal_error")
			return
		}
	}

	// Return updated attachment
	var a TaskAttachment
	err = s.db.QueryRowContext(ctx,
		`SELECT ta.id, ta.task_id, ta.project_id, ta.user_id, ta.filename, ta.alt_name,
		        ta.file_type, ta.content_type, ta.file_size,
		        ta.cloudinary_url, ta.cloudinary_public_id, ta.created_at,
		        u.name as user_name
		 FROM task_attachments ta
		 LEFT JOIN users u ON ta.user_id = u.id
		 WHERE ta.id = $1`, attachmentID,
	).Scan(&a.ID, &a.TaskID, &a.ProjectID, &a.UserID,
		&a.Filename, &a.AltName, &a.FileType, &a.ContentType, &a.FileSize,
		&a.CloudinaryURL, &a.CloudinaryPublicID, &a.CreatedAt,
		&a.UserName)
	if err != nil {
		s.logger.Error("Failed to fetch updated attachment", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch attachment", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, a)
}

// HandleGetStorageUsage returns storage usage per user for a project
func (s *Server) HandleGetStorageUsage(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	projectID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid project ID", "bad_request")
		return
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT ta.user_id, COALESCE(u.name, u.email) as user_name,
		        COUNT(*) as file_count, COALESCE(SUM(ta.file_size), 0) as total_size
		 FROM task_attachments ta
		 LEFT JOIN users u ON ta.user_id = u.id
		 WHERE ta.project_id = $1
		 GROUP BY ta.user_id
		 ORDER BY total_size DESC`, projectID,
	)
	if err != nil {
		s.logger.Error("Failed to get storage usage", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to get storage usage", "internal_error")
		return
	}
	defer rows.Close()

	usage := []StorageUsage{}
	for rows.Next() {
		var u StorageUsage
		if err := rows.Scan(&u.UserID, &u.UserName, &u.FileCount, &u.TotalSize); err != nil {
			respondError(w, http.StatusInternalServerError, "failed to scan usage", "internal_error")
			return
		}
		usage = append(usage, u)
	}

	respondJSON(w, http.StatusOK, usage)
}

// HandleListAssets returns all file assets accessible to the current user (own + shared project members)
func (s *Server) HandleListAssets(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)
	query := r.URL.Query().Get("q")
	fileType := r.URL.Query().Get("type")

	limit := 50
	if l, err := strconv.Atoi(r.URL.Query().Get("limit")); err == nil && l > 0 && l <= 100 {
		limit = l
	}
	offset := 0
	if o, err := strconv.Atoi(r.URL.Query().Get("offset")); err == nil && o >= 0 {
		offset = o
	}

	// Build query dynamically based on filters
	baseQuery := `SELECT DISTINCT ta.id, ta.task_id, ta.project_id, ta.user_id, ta.filename, ta.alt_name,
		ta.file_type, ta.content_type, ta.file_size,
		ta.cloudinary_url, ta.cloudinary_public_id, ta.created_at,
		u.name as user_name,
		CASE WHEN ta.user_id = $1 THEN 1 ELSE 0 END as is_owner
	 FROM task_attachments ta
	 LEFT JOIN users u ON ta.user_id = u.id
	 WHERE (
	   ta.user_id = $2 OR ta.user_id IN (
	     SELECT DISTINCT pm2.user_id FROM project_members pm1
	     JOIN project_members pm2 ON pm1.project_id = pm2.project_id
	     WHERE pm1.user_id = $3 AND pm2.user_id != $4
	   )
	 )`

	args := []interface{}{userID, userID, userID, userID}

	if fileType != "" {
		baseQuery += fmt.Sprintf(` AND ta.file_type = $%d`, len(args)+1)
		args = append(args, fileType)
	}

	if query != "" {
		searchPattern := "%" + query + "%"
		baseQuery += fmt.Sprintf(` AND (ta.alt_name LIKE $%d OR ta.filename LIKE $%d)`, len(args)+1, len(args)+2)
		args = append(args, searchPattern, searchPattern)
	}

	baseQuery += fmt.Sprintf(` ORDER BY ta.created_at DESC LIMIT $%d OFFSET $%d`, len(args)+1, len(args)+2)
	args = append(args, limit, offset)

	rows, err := s.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		s.logger.Error("Failed to list assets", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list assets", "internal_error")
		return
	}
	defer rows.Close()

	assets := []AssetResponse{}
	for rows.Next() {
		var a AssetResponse
		var isOwnerInt int
		if err := rows.Scan(&a.ID, &a.TaskID, &a.ProjectID, &a.UserID,
			&a.Filename, &a.AltName, &a.FileType, &a.ContentType, &a.FileSize,
			&a.CloudinaryURL, &a.CloudinaryPublicID, &a.CreatedAt,
			&a.UserName, &isOwnerInt); err != nil {
			s.logger.Error("Failed to scan asset", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to scan asset", "internal_error")
			return
		}
		a.IsOwner = isOwnerInt == 1
		assets = append(assets, a)
	}

	respondJSON(w, http.StatusOK, assets)
}

// HandleDeleteAttachment removes an attachment by ID (only the uploader can delete)
func (s *Server) HandleDeleteAttachment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	attachmentID, err := strconv.ParseInt(chi.URLParam(r, "id"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid attachment ID", "bad_request")
		return
	}

	// Verify ownership
	var ownerID int64
	err = s.db.QueryRowContext(ctx,
		`SELECT user_id FROM task_attachments WHERE id = $1`, attachmentID,
	).Scan(&ownerID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "attachment not found", "not_found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify attachment", "internal_error")
		return
	}
	if ownerID != userID {
		respondError(w, http.StatusForbidden, "you can only delete your own attachments", "forbidden")
		return
	}

	_, err = s.db.ExecContext(ctx,
		`DELETE FROM task_attachments WHERE id = $1`, attachmentID,
	)
	if err != nil {
		s.logger.Error("Failed to delete attachment", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to delete attachment", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Attachment deleted"})
}

// HandleListWikiPageAttachments returns all attachments for a wiki page
func (s *Server) HandleListWikiPageAttachments(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "bad_request")
		return
	}

	rows, err := s.db.QueryContext(ctx,
		`SELECT wa.id, wa.wiki_page_id, wa.project_id, wa.user_id, wa.filename, wa.alt_name,
		        wa.file_type, wa.content_type, wa.file_size,
		        wa.cloudinary_url, wa.cloudinary_public_id, wa.created_at,
		        u.name as user_name
		 FROM wiki_page_attachments wa
		 LEFT JOIN users u ON wa.user_id = u.id
		 WHERE wa.wiki_page_id = $1
		 ORDER BY wa.created_at DESC`, pageID,
	)
	if err != nil {
		s.logger.Error("Failed to list wiki page attachments", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to list attachments", "internal_error")
		return
	}
	defer rows.Close()

	attachments := []WikiPageAttachment{}
	for rows.Next() {
		var a WikiPageAttachment
		if err := rows.Scan(&a.ID, &a.WikiPageID, &a.ProjectID, &a.UserID,
			&a.Filename, &a.AltName, &a.FileType, &a.ContentType, &a.FileSize,
			&a.CloudinaryURL, &a.CloudinaryPublicID, &a.CreatedAt,
			&a.UserName); err != nil {
			s.logger.Error("Failed to scan wiki page attachment", zap.Error(err))
			respondError(w, http.StatusInternalServerError, "failed to scan attachment", "internal_error")
			return
		}
		attachments = append(attachments, a)
	}

	respondJSON(w, http.StatusOK, attachments)
}

// HandleCreateWikiPageAttachment stores a new attachment record for a wiki page
func (s *Server) HandleCreateWikiPageAttachment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	pageID, err := strconv.ParseInt(chi.URLParam(r, "pageId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid page ID", "bad_request")
		return
	}

	var req CreateAttachmentRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "bad_request")
		return
	}

	if req.Filename == "" || req.CloudinaryURL == "" || req.CloudinaryPublicID == "" {
		respondError(w, http.StatusBadRequest, "filename, cloudinary_url, and cloudinary_public_id are required", "validation_error")
		return
	}

	// Look up wiki page to get project_id
	var projectID int64
	err = s.db.QueryRowContext(ctx,
		`SELECT project_id FROM wiki_pages WHERE id = $1`, pageID,
	).Scan(&projectID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "wiki page not found", "not_found")
		return
	}
	if err != nil {
		s.logger.Error("Failed to look up wiki page", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to look up wiki page", "internal_error")
		return
	}

	// Enforce 100-attachment limit per wiki page
	var attachmentCount int
	err = s.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM wiki_page_attachments WHERE wiki_page_id = $1`, pageID,
	).Scan(&attachmentCount)
	if err != nil {
		s.logger.Error("Failed to count wiki page attachments", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to count attachments", "internal_error")
		return
	}
	if attachmentCount >= 100 {
		respondError(w, http.StatusBadRequest, "maximum 100 attachments per wiki page", "attachment_limit_exceeded")
		return
	}

	result, err := s.db.ExecContext(ctx,
		`INSERT INTO wiki_page_attachments (wiki_page_id, project_id, user_id, filename, alt_name, file_type, content_type, file_size, cloudinary_url, cloudinary_public_id)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		pageID, projectID, userID, req.Filename, req.AltName, req.FileType, req.ContentType, req.FileSize, req.CloudinaryURL, req.CloudinaryPublicID,
	)
	if err != nil {
		s.logger.Error("Failed to create wiki page attachment", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to create attachment", "internal_error")
		return
	}

	id, _ := result.LastInsertId()
	s.logger.Info("Wiki page attachment created",
		zap.Int64("id", id),
		zap.Int64("wiki_page_id", pageID),
		zap.Int64("user_id", userID),
		zap.String("filename", req.Filename),
	)

	attachment := WikiPageAttachment{
		ID:                 id,
		WikiPageID:         pageID,
		ProjectID:          projectID,
		UserID:             userID,
		Filename:           req.Filename,
		AltName:            req.AltName,
		FileType:           req.FileType,
		ContentType:        req.ContentType,
		FileSize:           req.FileSize,
		CloudinaryURL:      req.CloudinaryURL,
		CloudinaryPublicID: req.CloudinaryPublicID,
		CreatedAt:          time.Now(),
	}

	respondJSON(w, http.StatusCreated, attachment)
}

// HandleDeleteWikiPageAttachment removes a wiki page attachment (only the uploader can delete)
func (s *Server) HandleDeleteWikiPageAttachment(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	userID := r.Context().Value(UserIDKey).(int64)

	attachmentID, err := strconv.ParseInt(chi.URLParam(r, "attachmentId"), 10, 64)
	if err != nil {
		respondError(w, http.StatusBadRequest, "invalid attachment ID", "bad_request")
		return
	}

	// Verify ownership
	var ownerID int64
	err = s.db.QueryRowContext(ctx,
		`SELECT user_id FROM wiki_page_attachments WHERE id = $1`, attachmentID,
	).Scan(&ownerID)
	if err == sql.ErrNoRows {
		respondError(w, http.StatusNotFound, "attachment not found", "not_found")
		return
	}
	if err != nil {
		respondError(w, http.StatusInternalServerError, "failed to verify attachment", "internal_error")
		return
	}
	if ownerID != userID {
		respondError(w, http.StatusForbidden, "you can only delete your own attachments", "forbidden")
		return
	}

	_, err = s.db.ExecContext(ctx,
		`DELETE FROM wiki_page_attachments WHERE id = $1`, attachmentID,
	)
	if err != nil {
		s.logger.Error("Failed to delete wiki page attachment", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to delete attachment", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, map[string]string{"message": "Attachment deleted"})
}
