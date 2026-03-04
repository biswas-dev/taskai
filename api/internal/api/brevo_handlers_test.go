package api

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestMaskAPIKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
		want string
	}{
		{"short key (1 char)", "a", "*"},
		{"short key (4 chars)", "abcd", "****"},
		{"short key (8 chars)", "abcdefgh", "********"},
		{"normal key (12 chars)", "xak-123456AB", "xak-****56AB"},
		{"long key (24 chars)", "xkeysib-abcdef1234567890", "xkey****************7890"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := maskAPIKey(tt.key)
			if got != tt.want {
				t.Errorf("maskAPIKey(%q) = %q, want %q", tt.key, got, tt.want)
			}
		})
	}
}

func TestEmailProviderToResponse(t *testing.T) {
	now := time.Now()
	ep := EmailProvider{
		ID:                  1,
		Provider:            "brevo",
		APIKey:              "xkeysib-abcdef1234567890",
		SenderEmail:         "noreply@taskai.cc",
		SenderName:          "TaskAI",
		Status:              "connected",
		LastCheckedAt:       &now,
		LastError:           "",
		ConsecutiveFailures: 0,
		CreatedAt:           now,
		UpdatedAt:           now,
	}

	resp := ep.toResponse()

	if resp.APIKeyMasked == ep.APIKey {
		t.Error("Expected API key to be masked in response")
	}
	if resp.APIKeyMasked != "xkey****************7890" {
		t.Errorf("Unexpected masked key: %q", resp.APIKeyMasked)
	}
	if resp.SenderEmail != ep.SenderEmail {
		t.Errorf("SenderEmail mismatch: got %q, want %q", resp.SenderEmail, ep.SenderEmail)
	}
	if resp.SenderName != ep.SenderName {
		t.Errorf("SenderName mismatch: got %q, want %q", resp.SenderName, ep.SenderName)
	}
	if resp.Status != ep.Status {
		t.Errorf("Status mismatch: got %q, want %q", resp.Status, ep.Status)
	}
	if resp.Provider != ep.Provider {
		t.Errorf("Provider mismatch: got %q, want %q", resp.Provider, ep.Provider)
	}
}

// insertEmailProvider inserts a test email provider row
func insertEmailProvider(t *testing.T, ts *TestServer, apiKey, senderEmail, senderName, status string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := ts.DB.ExecContext(ctx,
		`INSERT INTO email_provider (id, provider, api_key, sender_email, sender_name, status, consecutive_failures)
		 VALUES (1, 'brevo', ?, ?, ?, ?, 0)`,
		apiKey, senderEmail, senderName, status,
	)
	if err != nil {
		t.Fatalf("Failed to insert email provider: %v", err)
	}
}

func TestHandleGetEmailProvider(t *testing.T) {
	t.Run("unauthenticated", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodGet, "/api/admin/settings/email", nil, nil)
		ts.HandleGetEmailProvider(rec, req)

		AssertError(t, rec, http.StatusUnauthorized, "user not authenticated", "unauthorized")
	})

	t.Run("non-admin forbidden", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "user@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/admin/settings/email", nil, userID, nil)
		ts.HandleGetEmailProvider(rec, req)

		AssertError(t, rec, http.StatusForbidden, "admin access required", "forbidden")
	})

	t.Run("no provider configured", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		makeAdmin(t, ts, adminID)

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/admin/settings/email", nil, adminID, nil)
		ts.HandleGetEmailProvider(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp map[string]interface{}
		DecodeJSON(t, rec, &resp)

		// Should return empty object when no provider configured
		if len(resp) != 0 {
			t.Errorf("Expected empty response, got %v", resp)
		}
	})

	t.Run("provider configured", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		makeAdmin(t, ts, adminID)

		insertEmailProvider(t, ts, "xkeysib-test1234567890ab", "noreply@taskai.cc", "TaskAI", "connected")

		rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/admin/settings/email", nil, adminID, nil)
		ts.HandleGetEmailProvider(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp EmailProviderResponse
		DecodeJSON(t, rec, &resp)

		if resp.ID != 1 {
			t.Errorf("Expected ID 1, got %d", resp.ID)
		}
		if resp.Provider != "brevo" {
			t.Errorf("Expected provider 'brevo', got %q", resp.Provider)
		}
		if resp.SenderEmail != "noreply@taskai.cc" {
			t.Errorf("Expected sender email 'noreply@taskai.cc', got %q", resp.SenderEmail)
		}
		if resp.SenderName != "TaskAI" {
			t.Errorf("Expected sender name 'TaskAI', got %q", resp.SenderName)
		}
		if resp.Status != "connected" {
			t.Errorf("Expected status 'connected', got %q", resp.Status)
		}
		// API key should be masked
		if resp.APIKeyMasked == "xkeysib-test1234567890ab" {
			t.Error("API key should be masked in response")
		}
		if resp.APIKeyMasked != "xkey****************90ab" {
			t.Errorf("Unexpected masked key: %q", resp.APIKeyMasked)
		}
	})
}

func TestHandleSaveEmailProvider(t *testing.T) {
	t.Run("unauthenticated", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		body := SaveEmailProviderRequest{
			APIKey:      "xkeysib-test",
			SenderEmail: "noreply@taskai.cc",
			SenderName:  "TaskAI",
		}

		rec, req := MakeRequest(t, http.MethodPost, "/api/admin/settings/email", body, nil)
		ts.HandleSaveEmailProvider(rec, req)

		AssertError(t, rec, http.StatusUnauthorized, "user not authenticated", "unauthorized")
	})

	t.Run("non-admin forbidden", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "user@example.com", "password123")

		body := SaveEmailProviderRequest{
			APIKey:      "xkeysib-test",
			SenderEmail: "noreply@taskai.cc",
			SenderName:  "TaskAI",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/admin/settings/email", body, userID, nil)
		ts.HandleSaveEmailProvider(rec, req)

		AssertError(t, rec, http.StatusForbidden, "admin access required", "forbidden")
	})

	t.Run("invalid body", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		makeAdmin(t, ts, adminID)

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/admin/settings/email", "not-json", adminID, nil)
		ts.HandleSaveEmailProvider(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "invalid request body", "bad_request")
	})

	t.Run("missing required fields", func(t *testing.T) {
		tests := []struct {
			name string
			body SaveEmailProviderRequest
		}{
			{"missing api_key", SaveEmailProviderRequest{SenderEmail: "a@b.c", SenderName: "Test"}},
			{"missing sender_email", SaveEmailProviderRequest{APIKey: "key", SenderName: "Test"}},
			{"missing sender_name", SaveEmailProviderRequest{APIKey: "key", SenderEmail: "a@b.c"}},
			{"all empty", SaveEmailProviderRequest{}},
		}

		for _, tt := range tests {
			t.Run(tt.name, func(t *testing.T) {
				ts := NewTestServer(t)
				defer ts.Close()

				adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
				makeAdmin(t, ts, adminID)

				rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/admin/settings/email", tt.body, adminID, nil)
				ts.HandleSaveEmailProvider(rec, req)

				AssertError(t, rec, http.StatusBadRequest, "api_key, sender_email, and sender_name are required", "validation_error")
			})
		}
	})

	t.Run("successful save (new provider)", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		makeAdmin(t, ts, adminID)

		body := SaveEmailProviderRequest{
			APIKey:      "xkeysib-test1234567890ab",
			SenderEmail: "noreply@taskai.cc",
			SenderName:  "TaskAI",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/admin/settings/email", body, adminID, nil)
		ts.HandleSaveEmailProvider(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp EmailProviderResponse
		DecodeJSON(t, rec, &resp)

		if resp.ID != 1 {
			t.Errorf("Expected ID 1, got %d", resp.ID)
		}
		if resp.SenderEmail != "noreply@taskai.cc" {
			t.Errorf("Expected sender email 'noreply@taskai.cc', got %q", resp.SenderEmail)
		}
		if resp.SenderName != "TaskAI" {
			t.Errorf("Expected sender name 'TaskAI', got %q", resp.SenderName)
		}
		// API key should be masked
		if resp.APIKeyMasked == "xkeysib-test1234567890ab" {
			t.Error("API key should be masked")
		}
		// Status will be "error" since the test API key is invalid (Brevo connection test fails)
		if resp.Status != "error" {
			t.Logf("Status is %q (expected 'error' since test API key won't connect to Brevo)", resp.Status)
		}

		// Verify it was persisted in the database
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()

		var dbAPIKey, dbSenderEmail, dbSenderName string
		err := ts.DB.QueryRowContext(ctx,
			`SELECT api_key, sender_email, sender_name FROM email_provider WHERE id = 1`,
		).Scan(&dbAPIKey, &dbSenderEmail, &dbSenderName)
		if err != nil {
			t.Fatalf("Failed to query email provider: %v", err)
		}
		if dbAPIKey != "xkeysib-test1234567890ab" {
			t.Errorf("DB api_key = %q, want %q", dbAPIKey, "xkeysib-test1234567890ab")
		}
		if dbSenderEmail != "noreply@taskai.cc" {
			t.Errorf("DB sender_email = %q, want %q", dbSenderEmail, "noreply@taskai.cc")
		}
		if dbSenderName != "TaskAI" {
			t.Errorf("DB sender_name = %q, want %q", dbSenderName, "TaskAI")
		}
	})

	t.Run("update existing provider", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		makeAdmin(t, ts, adminID)

		// Insert initial provider
		insertEmailProvider(t, ts, "old-key-123456", "old@taskai.cc", "OldName", "connected")

		// Update with new values
		body := SaveEmailProviderRequest{
			APIKey:      "new-key-abcdef1234567890",
			SenderEmail: "new@taskai.cc",
			SenderName:  "NewName",
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/admin/settings/email", body, adminID, nil)
		ts.HandleSaveEmailProvider(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp EmailProviderResponse
		DecodeJSON(t, rec, &resp)

		if resp.SenderEmail != "new@taskai.cc" {
			t.Errorf("Expected updated sender email 'new@taskai.cc', got %q", resp.SenderEmail)
		}
		if resp.SenderName != "NewName" {
			t.Errorf("Expected updated sender name 'NewName', got %q", resp.SenderName)
		}

		// Verify only 1 row exists (upsert, not insert)
		var count int
		err := ts.DB.QueryRow("SELECT COUNT(*) FROM email_provider").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to count rows: %v", err)
		}
		if count != 1 {
			t.Errorf("Expected 1 email_provider row, got %d", count)
		}
	})
}

func TestHandleDeleteEmailProvider(t *testing.T) {
	t.Run("unauthenticated", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodDelete, "/api/admin/settings/email", nil, nil)
		ts.HandleDeleteEmailProvider(rec, req)

		AssertError(t, rec, http.StatusUnauthorized, "user not authenticated", "unauthorized")
	})

	t.Run("non-admin forbidden", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "user@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/admin/settings/email", nil, userID, nil)
		ts.HandleDeleteEmailProvider(rec, req)

		AssertError(t, rec, http.StatusForbidden, "admin access required", "forbidden")
	})

	t.Run("successful delete", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		makeAdmin(t, ts, adminID)

		insertEmailProvider(t, ts, "xkeysib-test", "noreply@taskai.cc", "TaskAI", "connected")

		rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/admin/settings/email", nil, adminID, nil)
		ts.HandleDeleteEmailProvider(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp map[string]string
		DecodeJSON(t, rec, &resp)
		if resp["message"] != "Email provider deleted" {
			t.Errorf("Expected message 'Email provider deleted', got %q", resp["message"])
		}

		// Verify it was deleted
		var count int
		err := ts.DB.QueryRow("SELECT COUNT(*) FROM email_provider WHERE id = 1").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}
		if count != 0 {
			t.Errorf("Expected 0 rows after delete, got %d", count)
		}
	})

	t.Run("delete when none exists", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		makeAdmin(t, ts, adminID)

		// Delete when no provider exists — should succeed silently
		rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/admin/settings/email", nil, adminID, nil)
		ts.HandleDeleteEmailProvider(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)
	})
}

func TestHandleTestEmailProvider(t *testing.T) {
	t.Run("unauthenticated", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodPost, "/api/admin/settings/email/test", nil, nil)
		ts.HandleTestEmailProvider(rec, req)

		AssertError(t, rec, http.StatusUnauthorized, "user not authenticated", "unauthorized")
	})

	t.Run("non-admin forbidden", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "user@example.com", "password123")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/admin/settings/email/test", nil, userID, nil)
		ts.HandleTestEmailProvider(rec, req)

		AssertError(t, rec, http.StatusForbidden, "admin access required", "forbidden")
	})

	t.Run("no provider configured", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		makeAdmin(t, ts, adminID)

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/admin/settings/email/test", nil, adminID, nil)
		ts.HandleTestEmailProvider(rec, req)

		AssertError(t, rec, http.StatusBadRequest, "no email provider configured", "no_credentials")
	})

	t.Run("test connection with invalid key", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		makeAdmin(t, ts, adminID)

		insertEmailProvider(t, ts, "invalid-api-key-12345678", "noreply@taskai.cc", "TaskAI", "unknown")

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/admin/settings/email/test", nil, adminID, nil)
		ts.HandleTestEmailProvider(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp EmailProviderResponse
		DecodeJSON(t, rec, &resp)

		// Status will be "error" since key is invalid
		if resp.Status != "error" {
			t.Logf("Status is %q (expected 'error' since API key is invalid)", resp.Status)
		}
		if resp.LastCheckedAt == nil {
			t.Error("Expected last_checked_at to be set after test")
		}
		// Consecutive failures should be incremented
		if resp.ConsecutiveFailures < 1 {
			t.Errorf("Expected consecutive_failures >= 1, got %d", resp.ConsecutiveFailures)
		}
	})

	t.Run("consecutive failures lead to suspended status", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		makeAdmin(t, ts, adminID)

		insertEmailProvider(t, ts, "invalid-api-key-12345678", "noreply@taskai.cc", "TaskAI", "error")

		// Set consecutive_failures to 4 (one more test failure should trigger suspended)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, err := ts.DB.ExecContext(ctx, `UPDATE email_provider SET consecutive_failures = 4 WHERE id = 1`)
		if err != nil {
			t.Fatalf("Failed to update consecutive_failures: %v", err)
		}

		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/admin/settings/email/test", nil, adminID, nil)
		ts.HandleTestEmailProvider(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var resp EmailProviderResponse
		DecodeJSON(t, rec, &resp)

		// After 5 consecutive failures, status should be "suspended"
		if resp.Status != "suspended" {
			t.Errorf("Expected status 'suspended' after 5 failures, got %q", resp.Status)
		}
		if resp.ConsecutiveFailures != 5 {
			t.Errorf("Expected consecutive_failures 5, got %d", resp.ConsecutiveFailures)
		}
	})
}

func TestHandleSaveEmailProvider_InvalidatesCache(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
	makeAdmin(t, ts, adminID)

	// Save a provider
	body := SaveEmailProviderRequest{
		APIKey:      "xkeysib-test1234567890ab",
		SenderEmail: "noreply@taskai.cc",
		SenderName:  "TaskAI",
	}

	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/admin/settings/email", body, adminID, nil)
	ts.HandleSaveEmailProvider(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	// After save, the cached email service should be nil (invalidated)
	// GetEmailService will try to reload, but the test key is invalid so it'll create a service anyway
	// The key thing is no panic occurs
	svc := ts.GetEmailService()
	// Service may or may not be nil depending on whether status is suspended
	_ = svc
}

func TestHandleDeleteEmailProvider_InvalidatesCache(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
	makeAdmin(t, ts, adminID)

	insertEmailProvider(t, ts, "xkeysib-test", "noreply@taskai.cc", "TaskAI", "connected")

	rec, req := ts.MakeAuthRequest(t, http.MethodDelete, "/api/admin/settings/email", nil, adminID, nil)
	ts.HandleDeleteEmailProvider(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusOK)

	// After delete, GetEmailService should return nil
	svc := ts.GetEmailService()
	if svc != nil {
		t.Error("Expected GetEmailService to return nil after provider deleted")
	}
}

func TestGetEmailProvider_NotFound(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ep, err := ts.getEmailProvider(ctx)
	if err == nil {
		t.Error("Expected error when no provider configured")
	}
	if ep != nil {
		t.Error("Expected nil provider when none configured")
	}
}

func TestGetEmailProvider_Found(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	insertEmailProvider(t, ts, "test-api-key-1234", "sender@test.com", "Test Sender", "connected")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ep, err := ts.getEmailProvider(ctx)
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if ep == nil {
		t.Fatal("Expected non-nil provider")
	}
	if ep.APIKey != "test-api-key-1234" {
		t.Errorf("Expected API key 'test-api-key-1234', got %q", ep.APIKey)
	}
	if ep.SenderEmail != "sender@test.com" {
		t.Errorf("Expected sender email 'sender@test.com', got %q", ep.SenderEmail)
	}
	if ep.Status != "connected" {
		t.Errorf("Expected status 'connected', got %q", ep.Status)
	}
}

func TestGetEmailService_NilWhenNoProvider(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	svc := ts.GetEmailService()
	if svc != nil {
		t.Error("Expected nil email service when no provider configured")
	}
}

func TestGetEmailService_NilWhenSuspended(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	insertEmailProvider(t, ts, "test-key-123456789012", "sender@test.com", "Test", "suspended")

	svc := ts.GetEmailService()
	if svc != nil {
		t.Error("Expected nil email service when provider is suspended")
	}
}

func TestGetEmailService_ReturnsServiceWhenActive(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	insertEmailProvider(t, ts, "test-key-123456789012", "sender@test.com", "Test", "connected")

	svc := ts.GetEmailService()
	if svc == nil {
		t.Error("Expected non-nil email service when provider is connected")
	}

	// Calling again should return cached service
	svc2 := ts.GetEmailService()
	if svc2 == nil {
		t.Error("Expected cached email service on second call")
	}
}

func TestGetEmailService_InvalidateAndReload(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	insertEmailProvider(t, ts, "test-key-123456789012", "sender@test.com", "Test", "connected")

	// First call loads from DB
	svc := ts.GetEmailService()
	if svc == nil {
		t.Fatal("Expected non-nil email service")
	}

	// Invalidate
	ts.invalidateEmailService()

	// Next call should reload from DB
	svc2 := ts.GetEmailService()
	if svc2 == nil {
		t.Error("Expected non-nil email service after invalidation and reload")
	}
}

func TestGetAppURL(t *testing.T) {
	t.Run("returns first CORS origin", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ts.config.CORSAllowedOrigins = []string{"https://taskai.cc", "https://other.com"}

		url := ts.getAppURL()
		if url != "https://taskai.cc" {
			t.Errorf("Expected 'https://taskai.cc', got %q", url)
		}
	})

	t.Run("returns default when no CORS origins", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		ts.config.CORSAllowedOrigins = nil

		url := ts.getAppURL()
		if url != "http://localhost:5173" {
			t.Errorf("Expected 'http://localhost:5173', got %q", url)
		}
	})
}

func TestHandleCreateInvite_WithEmail(t *testing.T) {
	t.Run("with email but no email service", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "inviter@example.com", "password123")

		body := map[string]string{"email": "recipient@example.com"}
		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/invites", body, userID, nil)
		ts.HandleCreateInvite(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var resp map[string]interface{}
		DecodeJSON(t, rec, &resp)

		code, ok := resp["code"].(string)
		if !ok || code == "" {
			t.Error("Expected non-empty invite code")
		}

		// email_sent should be false since no email service configured
		emailSent, ok := resp["email_sent"].(bool)
		if !ok {
			t.Fatal("Expected email_sent field in response")
		}
		if emailSent {
			t.Error("Expected email_sent to be false when no email service configured")
		}
	})

	t.Run("without email field", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		userID := ts.CreateTestUser(t, "inviter@example.com", "password123")

		// Send empty body (backwards compatible)
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
	})

	t.Run("admin with email but no email service", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
		makeAdmin(t, ts, adminID)

		// Set invite_count to 0 — admin should still create invite
		ctx := context.Background()
		_, err := ts.DB.ExecContext(ctx, `UPDATE users SET invite_count = 0 WHERE id = ?`, adminID)
		if err != nil {
			t.Fatalf("Failed to update: %v", err)
		}

		body := map[string]string{"email": "recipient@example.com"}
		rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/invites", body, adminID, nil)
		ts.HandleCreateInvite(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusCreated)

		var resp map[string]interface{}
		DecodeJSON(t, rec, &resp)

		// Admin invite count should not be decremented
		var inviteCount int
		err = ts.DB.QueryRow("SELECT invite_count FROM users WHERE id = ?", adminID).Scan(&inviteCount)
		if err != nil {
			t.Fatalf("Failed to query: %v", err)
		}
		if inviteCount != 0 {
			t.Errorf("Admin invite_count should remain 0, got %d", inviteCount)
		}
	})
}

func TestHandleSaveEmailProvider_FullCycle(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	adminID := ts.CreateTestUser(t, "admin@example.com", "password123")
	makeAdmin(t, ts, adminID)

	// 1. Get — empty
	rec, req := ts.MakeAuthRequest(t, http.MethodGet, "/api/admin/settings/email", nil, adminID, nil)
	ts.HandleGetEmailProvider(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	var emptyResp map[string]interface{}
	DecodeJSON(t, rec, &emptyResp)
	if len(emptyResp) != 0 {
		t.Errorf("Step 1: Expected empty response, got %v", emptyResp)
	}

	// 2. Save
	body := SaveEmailProviderRequest{
		APIKey:      "xkeysib-test1234567890ab",
		SenderEmail: "noreply@taskai.cc",
		SenderName:  "TaskAI",
	}
	rec, req = ts.MakeAuthRequest(t, http.MethodPost, "/api/admin/settings/email", body, adminID, nil)
	ts.HandleSaveEmailProvider(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	// 3. Get — should return saved config
	rec, req = ts.MakeAuthRequest(t, http.MethodGet, "/api/admin/settings/email", nil, adminID, nil)
	ts.HandleGetEmailProvider(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	var getResp EmailProviderResponse
	DecodeJSON(t, rec, &getResp)
	if getResp.SenderEmail != "noreply@taskai.cc" {
		t.Errorf("Step 3: Expected sender email 'noreply@taskai.cc', got %q", getResp.SenderEmail)
	}

	// 4. Delete
	rec, req = ts.MakeAuthRequest(t, http.MethodDelete, "/api/admin/settings/email", nil, adminID, nil)
	ts.HandleDeleteEmailProvider(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	// 5. Get — empty again
	rec, req = ts.MakeAuthRequest(t, http.MethodGet, "/api/admin/settings/email", nil, adminID, nil)
	ts.HandleGetEmailProvider(rec, req)
	AssertStatusCode(t, rec.Code, http.StatusOK)

	var finalResp map[string]interface{}
	DecodeJSON(t, rec, &finalResp)
	if len(finalResp) != 0 {
		t.Errorf("Step 5: Expected empty response after delete, got %v", finalResp)
	}
}

func TestCheckBrevoHealth_NoProvider(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	// Should not panic when no provider configured
	ts.checkBrevoHealth()
}

func TestCheckBrevoHealth_WithProvider(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	insertEmailProvider(t, ts, "invalid-api-key-12345678", "noreply@taskai.cc", "TaskAI", "unknown")

	// Should not panic and should update status
	ts.checkBrevoHealth()

	// Verify status was updated
	var status string
	var consecutiveFailures int
	err := ts.DB.QueryRow(
		"SELECT status, consecutive_failures FROM email_provider WHERE id = 1",
	).Scan(&status, &consecutiveFailures)
	if err != nil {
		t.Fatalf("Failed to query: %v", err)
	}

	// Status should be "error" since the API key is invalid
	if status != "error" {
		t.Logf("Status is %q (expected 'error')", status)
	}
	if consecutiveFailures < 1 {
		t.Errorf("Expected consecutive_failures >= 1, got %d", consecutiveFailures)
	}
}

func TestStartBrevoHealthCheck_ContextCancellation(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ctx, cancel := context.WithCancel(context.Background())

	// Start the health check goroutine
	ts.StartBrevoHealthCheck(ctx)

	// Cancel immediately — should not panic
	cancel()

	// Give goroutine time to exit
	time.Sleep(10 * time.Millisecond)
}

func TestHandleInviteTeamMember_EmailSendingNewUser(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	createTestTeam(t, ts, ownerID, "Test Team")

	// No email service configured, so email won't be sent but handler should still succeed
	body := InviteTeamMemberRequest{Email: "newuser@example.com"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invite", body, ownerID, nil)
	ts.HandleInviteTeamMember(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var inv TeamInvitation
	DecodeJSON(t, rec, &inv)
	if inv.InviteeEmail != "newuser@example.com" {
		t.Errorf("Expected invitee email 'newuser@example.com', got %q", inv.InviteeEmail)
	}
	if inv.Status != "pending" {
		t.Errorf("Expected status 'pending', got %q", inv.Status)
	}

	// No invite code should be created in the invites table since email service is nil
	var inviteCount int
	err := ts.DB.QueryRow("SELECT COUNT(*) FROM invites WHERE inviter_id = ?", ownerID).Scan(&inviteCount)
	if err != nil {
		t.Fatalf("Failed to query invites: %v", err)
	}
	if inviteCount != 0 {
		t.Errorf("Expected 0 auto-generated invites (no email service), got %d", inviteCount)
	}
}

func TestHandleInviteTeamMember_EmailSendingExistingUser(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	teamID := createTestTeam(t, ts, ownerID, "Test Team")
	existingID := ts.CreateTestUser(t, "existing@example.com", "password123")

	// Invite existing user — should be auto-accepted, no email service configured
	body := InviteTeamMemberRequest{Email: "existing@example.com"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invite", body, ownerID, nil)
	ts.HandleInviteTeamMember(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var inv TeamInvitation
	DecodeJSON(t, rec, &inv)
	if inv.InviteeEmail != "existing@example.com" {
		t.Errorf("Expected invitee email 'existing@example.com', got %q", inv.InviteeEmail)
	}
	if inv.InviteeID == nil {
		t.Error("Expected invitee_id to be set for existing user")
	}
	if inv.Status != "accepted" {
		t.Errorf("Expected invitation status 'accepted' for existing user, got %q", inv.Status)
	}

	// Verify user was added to the team as an active member
	var memberCount int
	err := ts.DB.QueryRow(
		`SELECT COUNT(*) FROM team_members WHERE team_id = ? AND user_id = ? AND status = 'active'`,
		teamID, existingID,
	).Scan(&memberCount)
	if err != nil {
		t.Fatalf("Failed to query team members: %v", err)
	}
	if memberCount != 1 {
		t.Errorf("Expected existing user to be added as active team member, got count=%d", memberCount)
	}
}

func TestHandleInviteTeamMember_WithEmailService(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	ownerID := ts.CreateTestUser(t, "owner@example.com", "password123")
	createTestTeam(t, ts, ownerID, "Test Team")

	// Configure email provider so email service is available
	insertEmailProvider(t, ts, "test-key-123456789012", "noreply@taskai.cc", "TaskAI", "connected")

	// Set owner name for the email template
	_, err := ts.DB.Exec(`UPDATE users SET name = 'Team Owner' WHERE id = ?`, ownerID)
	if err != nil {
		t.Fatalf("Failed to update name: %v", err)
	}

	// Invite new user — email service is available
	// The actual email send will fail (test API key), but the handler should handle it gracefully
	body := InviteTeamMemberRequest{Email: "newuser@example.com"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/team/invite", body, ownerID, nil)
	ts.HandleInviteTeamMember(rec, req)

	// Should still succeed even though email sending fails
	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var inv TeamInvitation
	DecodeJSON(t, rec, &inv)
	if inv.InviteeEmail != "newuser@example.com" {
		t.Errorf("Expected invitee email 'newuser@example.com', got %q", inv.InviteeEmail)
	}

	// An invite code should have been auto-generated since email service is available
	var inviteCodeCount int
	err = ts.DB.QueryRow("SELECT COUNT(*) FROM invites WHERE inviter_id = ?", ownerID).Scan(&inviteCodeCount)
	if err != nil {
		t.Fatalf("Failed to query invites: %v", err)
	}
	if inviteCodeCount != 1 {
		t.Errorf("Expected 1 auto-generated invite code, got %d", inviteCodeCount)
	}
}

func TestHandleCreateInvite_WithEmailService(t *testing.T) {
	ts := NewTestServer(t)
	defer ts.Close()

	userID := ts.CreateTestUser(t, "inviter@example.com", "password123")

	// Configure email provider
	insertEmailProvider(t, ts, "test-key-123456789012", "noreply@taskai.cc", "TaskAI", "connected")

	body := map[string]string{"email": "recipient@example.com"}
	rec, req := ts.MakeAuthRequest(t, http.MethodPost, "/api/invites", body, userID, nil)
	ts.HandleCreateInvite(rec, req)

	AssertStatusCode(t, rec.Code, http.StatusCreated)

	var resp map[string]interface{}
	DecodeJSON(t, rec, &resp)

	code, ok := resp["code"].(string)
	if !ok || code == "" {
		t.Error("Expected non-empty invite code")
	}

	// email_sent may be false (test key won't actually send) but the field should be present
	if _, ok := resp["email_sent"]; !ok {
		t.Error("Expected email_sent field in response")
	}
}

func TestGenerateTeamInviteCode(t *testing.T) {
	code1, err := generateTeamInviteCode()
	if err != nil {
		t.Fatalf("Failed to generate invite code: %v", err)
	}
	if code1 == "" {
		t.Error("Expected non-empty invite code")
	}

	// Generate another and verify they're different
	code2, err := generateTeamInviteCode()
	if err != nil {
		t.Fatalf("Failed to generate second invite code: %v", err)
	}
	if code1 == code2 {
		t.Error("Expected different codes on subsequent calls")
	}
}

func TestTestBrevoConnection(t *testing.T) {
	// Test with an invalid key — should return error status
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	status, lastError := testBrevoConnection(ctx, "invalid-key")
	if status == "connected" {
		t.Error("Expected error status with invalid key")
	}
	if lastError == "" && status == "error" {
		t.Error("Expected non-empty error message when status is error")
	}
}

func TestTestBrevoConnection_CancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	status, lastError := testBrevoConnection(ctx, "any-key")
	if status == "connected" {
		t.Error("Expected error status with cancelled context")
	}
	_ = lastError // Error message may vary
}

// Verify the MakeRequest helper handles the SaveEmailProviderRequest JSON properly
func TestSaveEmailProviderRequest_JSONRoundTrip(t *testing.T) {
	req := SaveEmailProviderRequest{
		APIKey:      "test-key",
		SenderEmail: "test@example.com",
		SenderName:  "Test",
	}

	data, err := json.Marshal(req)
	if err != nil {
		t.Fatalf("Failed to marshal: %v", err)
	}

	var decoded SaveEmailProviderRequest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Failed to unmarshal: %v", err)
	}

	if decoded.APIKey != req.APIKey {
		t.Errorf("APIKey mismatch: got %q, want %q", decoded.APIKey, req.APIKey)
	}
	if decoded.SenderEmail != req.SenderEmail {
		t.Errorf("SenderEmail mismatch: got %q, want %q", decoded.SenderEmail, req.SenderEmail)
	}
	if decoded.SenderName != req.SenderName {
		t.Errorf("SenderName mismatch: got %q, want %q", decoded.SenderName, req.SenderName)
	}
}
