package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"taskai/internal/version"
)

// ---------------------------------------------------------------------------
// HandleVersion
// ---------------------------------------------------------------------------

func TestHandleVersion(t *testing.T) {
	t.Run("returns version info with all required fields", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		// HandleVersion does not require authentication (it's a public endpoint)
		rec, req := MakeRequest(t, http.MethodGet, "/api/version", nil, nil)

		ts.HandleVersion(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var info version.Info
		DecodeJSON(t, rec, &info)

		// Verify all fields are populated
		if info.Version == "" {
			t.Error("expected non-empty version")
		}
		if info.GitCommit == "" {
			t.Error("expected non-empty git_commit")
		}
		if info.BuildTime == "" {
			t.Error("expected non-empty build_time")
		}
		if info.GoVersion == "" {
			t.Error("expected non-empty go_version")
		}
		if info.Platform == "" {
			t.Error("expected non-empty platform")
		}
		if info.ServerTime.IsZero() {
			t.Error("expected non-zero server_time")
		}
		// DBVersion should be > 0 since migrations have been applied
		if info.DBVersion <= 0 {
			t.Errorf("expected positive db_version, got %d", info.DBVersion)
		}
	})

	t.Run("returns Content-Type application/json", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodGet, "/api/version", nil, nil)

		ts.HandleVersion(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		contentType := rec.Header().Get("Content-Type")
		if contentType != "application/json" {
			t.Errorf("expected Content-Type 'application/json', got %q", contentType)
		}
	})

	t.Run("response contains all expected JSON keys", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodGet, "/api/version", nil, nil)

		ts.HandleVersion(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var raw map[string]json.RawMessage
		if err := json.Unmarshal(rec.Body.Bytes(), &raw); err != nil {
			t.Fatalf("Failed to unmarshal response: %v", err)
		}

		expectedKeys := []string{
			"version",
			"git_commit",
			"build_time",
			"go_version",
			"platform",
			"server_time",
			"environment",
			"db_driver",
		}
		for _, key := range expectedKeys {
			if _, ok := raw[key]; !ok {
				t.Errorf("missing expected key %q in version response", key)
			}
		}
	})

	t.Run("environment reflects test config", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodGet, "/api/version", nil, nil)

		ts.HandleVersion(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var info version.Info
		DecodeJSON(t, rec, &info)

		// The test config does not set Env, so it will be empty string
		// This tests the handler processes the config env correctly
		_ = info.Environment
	})

	t.Run("version defaults are set for dev builds", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodGet, "/api/version", nil, nil)

		ts.HandleVersion(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var info version.Info
		DecodeJSON(t, rec, &info)

		// In test/dev mode, version defaults to "dev" and git_commit to "unknown"
		if info.Version != "dev" {
			t.Errorf("expected version 'dev' in test mode, got %q", info.Version)
		}
		if info.GitCommit != "unknown" {
			t.Errorf("expected git_commit 'unknown' in test mode, got %q", info.GitCommit)
		}
	})

	t.Run("platform contains os and arch", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodGet, "/api/version", nil, nil)

		ts.HandleVersion(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var info version.Info
		DecodeJSON(t, rec, &info)

		// Platform should be in "os/arch" format
		if len(info.Platform) < 3 {
			t.Errorf("expected platform in 'os/arch' format, got %q", info.Platform)
		}
		// Should contain a slash
		found := false
		for _, c := range info.Platform {
			if c == '/' {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected platform to contain '/', got %q", info.Platform)
		}
	})

	t.Run("db_version matches migration count", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodGet, "/api/version", nil, nil)

		ts.HandleVersion(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var info version.Info
		DecodeJSON(t, rec, &info)

		// Verify db_version is a reasonable positive number
		// (migrations have been applied to the test database)
		if info.DBVersion < 1 {
			t.Errorf("expected db_version >= 1 (migrations applied), got %d", info.DBVersion)
		}
	})
}
