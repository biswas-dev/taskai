package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"go.uber.org/zap"
)

// EmailProvider represents the email provider configuration
type EmailProvider struct {
	ID                  int64      `json:"id"`
	Provider            string     `json:"provider"`
	APIKey              string     `json:"api_key"`
	SenderEmail         string     `json:"sender_email"`
	SenderName          string     `json:"sender_name"`
	Status              string     `json:"status"`
	LastCheckedAt       *time.Time `json:"last_checked_at"`
	LastError           string     `json:"last_error"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

// EmailProviderResponse masks the API key in responses
type EmailProviderResponse struct {
	ID                  int64      `json:"id"`
	Provider            string     `json:"provider"`
	APIKeyMasked        string     `json:"api_key"`
	SenderEmail         string     `json:"sender_email"`
	SenderName          string     `json:"sender_name"`
	Status              string     `json:"status"`
	LastCheckedAt       *time.Time `json:"last_checked_at"`
	LastError           string     `json:"last_error"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	CreatedAt           time.Time  `json:"created_at"`
	UpdatedAt           time.Time  `json:"updated_at"`
}

type SaveEmailProviderRequest struct {
	APIKey      string `json:"api_key"`
	SenderEmail string `json:"sender_email"`
	SenderName  string `json:"sender_name"`
}

func maskAPIKey(key string) string {
	if len(key) <= 8 {
		return strings.Repeat("*", len(key))
	}
	return key[:4] + strings.Repeat("*", len(key)-8) + key[len(key)-4:]
}

func (ep *EmailProvider) toResponse() EmailProviderResponse {
	return EmailProviderResponse{
		ID:                  ep.ID,
		Provider:            ep.Provider,
		APIKeyMasked:        maskAPIKey(ep.APIKey),
		SenderEmail:         ep.SenderEmail,
		SenderName:          ep.SenderName,
		Status:              ep.Status,
		LastCheckedAt:       ep.LastCheckedAt,
		LastError:           ep.LastError,
		ConsecutiveFailures: ep.ConsecutiveFailures,
		CreatedAt:           ep.CreatedAt,
		UpdatedAt:           ep.UpdatedAt,
	}
}

// HandleGetEmailProvider returns the email provider config (admin only)
func (s *Server) HandleGetEmailProvider(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}
	if !s.isAdmin(r.Context(), userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	ep, err := s.getEmailProvider(ctx)
	if err == sql.ErrNoRows {
		respondJSON(w, http.StatusOK, map[string]interface{}{})
		return
	}
	if err != nil {
		s.logger.Error("Failed to fetch email provider", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch email provider", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, ep.toResponse())
}

// HandleSaveEmailProvider creates or updates the email provider config (admin only)
func (s *Server) HandleSaveEmailProvider(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}
	if !s.isAdmin(r.Context(), userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var req SaveEmailProviderRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		respondError(w, http.StatusBadRequest, "invalid request body", "bad_request")
		return
	}

	if req.APIKey == "" || req.SenderEmail == "" || req.SenderName == "" {
		respondError(w, http.StatusBadRequest, "api_key, sender_email, and sender_name are required", "validation_error")
		return
	}

	// Upsert the email provider (singleton — always id=1)
	_, err := s.db.ExecContext(ctx,
		`INSERT INTO email_provider (id, provider, api_key, sender_email, sender_name, updated_at)
		 VALUES (1, 'brevo', $1, $2, $3, CURRENT_TIMESTAMP)
		 ON CONFLICT(id) DO UPDATE SET
		   api_key = excluded.api_key,
		   sender_email = excluded.sender_email,
		   sender_name = excluded.sender_name,
		   updated_at = CURRENT_TIMESTAMP`,
		req.APIKey, req.SenderEmail, req.SenderName,
	)
	if err != nil {
		s.logger.Error("Failed to save email provider", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to save email provider", "internal_error")
		return
	}

	s.logger.Info("Email provider saved", zap.Int64("admin_id", userID))

	// Auto-test connection
	status, lastError := testBrevoConnection(ctx, req.APIKey)

	now := time.Now()
	consecutiveFailures := 0
	if status != "connected" {
		consecutiveFailures = 1
	}

	_, err = s.db.ExecContext(ctx,
		`UPDATE email_provider SET status = $1, last_checked_at = $2, last_error = $3, consecutive_failures = $4 WHERE id = 1`,
		status, now, lastError, consecutiveFailures,
	)
	if err != nil {
		s.logger.Error("Failed to update email provider status", zap.Error(err))
	}

	// Invalidate cached email service
	s.invalidateEmailService()

	ep, err := s.getEmailProvider(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch saved email provider", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "saved but failed to retrieve", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, ep.toResponse())
}

// HandleDeleteEmailProvider removes the email provider config (admin only)
func (s *Server) HandleDeleteEmailProvider(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}
	if !s.isAdmin(r.Context(), userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()

	_, err := s.db.ExecContext(ctx, `DELETE FROM email_provider WHERE id = 1`)
	if err != nil {
		s.logger.Error("Failed to delete email provider", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to delete email provider", "internal_error")
		return
	}

	s.invalidateEmailService()

	s.logger.Info("Email provider deleted", zap.Int64("admin_id", userID))
	respondJSON(w, http.StatusOK, map[string]string{"message": "Email provider deleted"})
}

// HandleTestEmailProvider manually tests the email provider connection (admin only)
func (s *Server) HandleTestEmailProvider(w http.ResponseWriter, r *http.Request) {
	userID, ok := GetUserID(r)
	if !ok {
		respondError(w, http.StatusUnauthorized, "user not authenticated", "unauthorized")
		return
	}
	if !s.isAdmin(r.Context(), userID) {
		respondError(w, http.StatusForbidden, "admin access required", "forbidden")
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()

	var apiKey string
	var consecutiveFailures int
	err := s.db.QueryRowContext(ctx,
		`SELECT api_key, consecutive_failures FROM email_provider WHERE id = 1`,
	).Scan(&apiKey, &consecutiveFailures)

	if err == sql.ErrNoRows {
		respondError(w, http.StatusBadRequest, "no email provider configured", "no_credentials")
		return
	}
	if err != nil {
		s.logger.Error("Failed to fetch email provider for test", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "failed to fetch email provider", "internal_error")
		return
	}

	status, lastError := testBrevoConnection(ctx, apiKey)

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
		`UPDATE email_provider SET status = $1, last_checked_at = $2, last_error = $3, consecutive_failures = $4 WHERE id = 1`,
		status, now, lastError, consecutiveFailures,
	)
	if err != nil {
		s.logger.Error("Failed to update email provider status after test", zap.Error(err))
	}

	ep, err := s.getEmailProvider(ctx)
	if err != nil {
		s.logger.Error("Failed to fetch email provider after test", zap.Error(err))
		respondError(w, http.StatusInternalServerError, "test completed but failed to retrieve status", "internal_error")
		return
	}

	respondJSON(w, http.StatusOK, ep.toResponse())
}

// testBrevoConnection tests a Brevo API key by calling the account endpoint
func testBrevoConnection(ctx context.Context, apiKey string) (status string, lastError string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "https://api.brevo.com/v3/account", nil)
	if err != nil {
		return "error", fmt.Sprintf("failed to create request: %v", err)
	}
	req.Header.Set("api-key", apiKey)
	req.Header.Set("Accept", "application/json")

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
	return "error", fmt.Sprintf("Brevo returned HTTP %d", resp.StatusCode)
}

// getEmailProvider fetches the singleton email provider row
func (s *Server) getEmailProvider(ctx context.Context) (*EmailProvider, error) {
	var ep EmailProvider
	err := s.db.QueryRowContext(ctx,
		`SELECT id, provider, api_key, sender_email, sender_name, status,
		        last_checked_at, last_error, consecutive_failures, created_at, updated_at
		 FROM email_provider WHERE id = 1`,
	).Scan(&ep.ID, &ep.Provider, &ep.APIKey, &ep.SenderEmail, &ep.SenderName, &ep.Status,
		&ep.LastCheckedAt, &ep.LastError, &ep.ConsecutiveFailures, &ep.CreatedAt, &ep.UpdatedAt)
	if err != nil {
		return nil, err
	}
	return &ep, nil
}

// StartBrevoHealthCheck starts a background goroutine that checks Brevo health every 24 hours
func (s *Server) StartBrevoHealthCheck(ctx context.Context) {
	ticker := time.NewTicker(24 * time.Hour)
	go func() {
		for {
			select {
			case <-ctx.Done():
				ticker.Stop()
				return
			case <-ticker.C:
				s.checkBrevoHealth()
			}
		}
	}()
	s.logger.Info("Brevo health check goroutine started (24h interval)")
}

func (s *Server) checkBrevoHealth() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var apiKey string
	var consecutiveFailures int
	err := s.db.QueryRowContext(ctx,
		`SELECT api_key, consecutive_failures FROM email_provider WHERE id = 1`,
	).Scan(&apiKey, &consecutiveFailures)

	if err != nil {
		// No provider configured — nothing to check
		return
	}

	status, lastError := testBrevoConnection(ctx, apiKey)

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
		`UPDATE email_provider SET status = $1, last_checked_at = $2, last_error = $3, consecutive_failures = $4 WHERE id = 1`,
		status, now, lastError, consecutiveFailures,
	)
	if err != nil {
		s.logger.Error("Failed to update email provider health", zap.Error(err))
		return
	}

	if status != "connected" {
		s.logger.Warn("Brevo health check failed",
			zap.String("status", status),
			zap.String("error", lastError),
			zap.Int("consecutive_failures", consecutiveFailures),
		)
	}
}
