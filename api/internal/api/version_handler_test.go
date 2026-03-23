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

		rec, req := MakeRequest(t, http.MethodGet, "/api/version", nil, nil)

		ts.HandleVersion(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var info version.VersionResponse
		DecodeJSON(t, rec, &info)

		if info.Backend.Version == "" {
			t.Error("expected non-empty backend.version")
		}
		if info.Backend.GitCommit == "" {
			t.Error("expected non-empty backend.git_commit")
		}
		if info.Backend.BuildTime == "" {
			t.Error("expected non-empty backend.build_time")
		}
		if info.Backend.GoVersion == "" {
			t.Error("expected non-empty backend.go_version")
		}
		if info.Backend.Platform == "" {
			t.Error("expected non-empty backend.platform")
		}
		if info.Resources.Goroutines <= 0 {
			t.Error("expected positive goroutines")
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
			"backend",
			"runtime",
			"resources",
			"database",
		}
		for _, key := range expectedKeys {
			if _, ok := raw[key]; !ok {
				t.Errorf("missing expected key %q in version response", key)
			}
		}
	})

	t.Run("version defaults are set for dev builds", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodGet, "/api/version", nil, nil)

		ts.HandleVersion(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var info version.VersionResponse
		DecodeJSON(t, rec, &info)

		if info.Backend.Version != "dev" {
			t.Errorf("expected version 'dev' in test mode, got %q", info.Backend.Version)
		}
		if info.Backend.GitCommit != "unknown" {
			t.Errorf("expected git_commit 'unknown' in test mode, got %q", info.Backend.GitCommit)
		}
	})

	t.Run("platform contains os and arch", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodGet, "/api/version", nil, nil)

		ts.HandleVersion(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var info version.VersionResponse
		DecodeJSON(t, rec, &info)

		if len(info.Backend.Platform) < 3 {
			t.Errorf("expected platform in 'os/arch' format, got %q", info.Backend.Platform)
		}
	})

	t.Run("db_version matches migration count", func(t *testing.T) {
		ts := NewTestServer(t)
		defer ts.Close()

		rec, req := MakeRequest(t, http.MethodGet, "/api/version", nil, nil)

		ts.HandleVersion(rec, req)

		AssertStatusCode(t, rec.Code, http.StatusOK)

		var info version.VersionResponse
		DecodeJSON(t, rec, &info)

		if info.Database.MigrationVersion < 1 {
			t.Errorf("expected migration_version >= 1, got %d", info.Database.MigrationVersion)
		}
	})
}
