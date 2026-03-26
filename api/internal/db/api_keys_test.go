package db

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

// newTestDB creates an in-memory database with migrations for testing
func newTestDB(t *testing.T) *DB {
	t.Helper()
	logger := zaptest.NewLogger(t)

	cfg := Config{
		Driver:         "sqlite",
		DBPath:         ":memory:",
		MigrationsPath: "./migrations",
	}

	database, err := New(cfg, logger)
	if err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	return database
}

// createTestUser inserts a user and returns their ID
func createTestUser(t *testing.T, db *DB, email string) int64 {
	t.Helper()
	ctx := context.Background()

	var id int64
	err := db.QueryRowContext(ctx,
		`INSERT INTO users (email, password_hash) VALUES (?, ?) RETURNING id`,
		email, "$2a$04$fakehashfortest",
	).Scan(&id)
	if err != nil {
		t.Fatalf("Failed to create test user: %v", err)
	}
	return id
}

func TestGenerateAPIKey(t *testing.T) {
	key, keyHash, prefix, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}

	if key == "" {
		t.Error("Expected non-empty key")
	}
	if keyHash == "" {
		t.Error("Expected non-empty keyHash")
	}
	if prefix == "" {
		t.Error("Expected non-empty prefix")
	}
	if len(prefix) != 8 {
		t.Errorf("Expected prefix length 8, got %d", len(prefix))
	}
	if prefix != key[:8] {
		t.Errorf("Prefix should be first 8 chars of key")
	}
}

func TestGenerateAPIKey_Uniqueness(t *testing.T) {
	keys := make(map[string]bool)
	for i := 0; i < 100; i++ {
		key, _, _, err := GenerateAPIKey()
		if err != nil {
			t.Fatalf("GenerateAPIKey failed on iteration %d: %v", i, err)
		}
		if keys[key] {
			t.Fatalf("Duplicate key generated on iteration %d", i)
		}
		keys[key] = true
	}
}

func TestHashAPIKey(t *testing.T) {
	tests := []struct {
		name string
		key  string
	}{
		{"simple key", "test-key-123"},
		{"empty key", ""},
		{"long key", "a-very-long-api-key-that-has-lots-of-characters-in-it-to-test-hashing"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hash := HashAPIKey(tt.key)
			if hash == "" {
				t.Error("Expected non-empty hash")
			}

			// Same key should produce same hash
			hash2 := HashAPIKey(tt.key)
			if hash != hash2 {
				t.Error("Same key should produce same hash")
			}
		})
	}

	// Different keys should produce different hashes
	hash1 := HashAPIKey("key-one")
	hash2 := HashAPIKey("key-two")
	if hash1 == hash2 {
		t.Error("Different keys should produce different hashes")
	}
}

func TestHashAPIKey_ConsistentWithGenerate(t *testing.T) {
	key, keyHash, _, err := GenerateAPIKey()
	if err != nil {
		t.Fatalf("GenerateAPIKey failed: %v", err)
	}

	// HashAPIKey(key) should produce the same hash as returned by GenerateAPIKey
	computedHash := HashAPIKey(key)
	if computedHash != keyHash {
		t.Errorf("HashAPIKey(%q) = %q, want %q", key, computedHash, keyHash)
	}
}

func TestCreateAPIKey(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	userID := createTestUser(t, db, "test@example.com")

	ctx := context.Background()
	result, err := db.CreateAPIKey(ctx, userID, "My Test Key", nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	if result.ID == 0 {
		t.Error("Expected non-zero API key ID")
	}
	if result.UserID != userID {
		t.Errorf("Expected user_id %d, got %d", userID, result.UserID)
	}
	if result.Name != "My Test Key" {
		t.Errorf("Expected name 'My Test Key', got %q", result.Name)
	}
	if result.Key == "" {
		t.Error("Expected non-empty key in result")
	}
	if len(result.KeyPrefix) != 8 {
		t.Errorf("Expected prefix length 8, got %d", len(result.KeyPrefix))
	}
	if result.ExpiresAt != nil {
		t.Error("Expected nil expires_at when none provided")
	}
}

func TestCreateAPIKey_WithExpiry(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	userID := createTestUser(t, db, "test@example.com")
	expiry := time.Now().Add(24 * time.Hour)

	ctx := context.Background()
	result, err := db.CreateAPIKey(ctx, userID, "Expiring Key", &expiry)
	if err != nil {
		t.Fatalf("CreateAPIKey with expiry failed: %v", err)
	}

	if result.ExpiresAt == nil {
		t.Error("Expected non-nil expires_at")
	}
}

func TestGetAPIKeysByUserID(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	userID := createTestUser(t, db, "test@example.com")
	ctx := context.Background()

	// Create a few keys
	_, err := db.CreateAPIKey(ctx, userID, "Key 1", nil)
	if err != nil {
		t.Fatalf("Failed to create key 1: %v", err)
	}
	_, err = db.CreateAPIKey(ctx, userID, "Key 2", nil)
	if err != nil {
		t.Fatalf("Failed to create key 2: %v", err)
	}

	keys, err := db.GetAPIKeysByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetAPIKeysByUserID failed: %v", err)
	}

	if len(keys) != 2 {
		t.Fatalf("Expected 2 keys, got %d", len(keys))
	}

	// Verify both keys are present (order depends on created_at which may be same timestamp)
	names := map[string]bool{}
	for _, k := range keys {
		names[k.Name] = true
	}
	if !names["Key 1"] || !names["Key 2"] {
		t.Errorf("Expected both 'Key 1' and 'Key 2', got %v", names)
	}
}

func TestGetAPIKeysByUserID_Empty(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	userID := createTestUser(t, db, "test@example.com")
	ctx := context.Background()

	keys, err := db.GetAPIKeysByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetAPIKeysByUserID failed: %v", err)
	}

	if len(keys) != 0 {
		t.Errorf("Expected empty keys for user with no API keys, got %v", keys)
	}
}

func TestGetAPIKeysByUserID_IsolatedByUser(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	user1 := createTestUser(t, db, "user1@example.com")
	user2 := createTestUser(t, db, "user2@example.com")
	ctx := context.Background()

	_, err := db.CreateAPIKey(ctx, user1, "User1 Key", nil)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}
	_, err = db.CreateAPIKey(ctx, user2, "User2 Key", nil)
	if err != nil {
		t.Fatalf("Failed to create key: %v", err)
	}

	keys1, err := db.GetAPIKeysByUserID(ctx, user1)
	if err != nil {
		t.Fatalf("Failed to get user1 keys: %v", err)
	}
	if len(keys1) != 1 {
		t.Errorf("Expected 1 key for user1, got %d", len(keys1))
	}
	if keys1[0].Name != "User1 Key" {
		t.Errorf("Expected 'User1 Key', got %q", keys1[0].Name)
	}

	keys2, err := db.GetAPIKeysByUserID(ctx, user2)
	if err != nil {
		t.Fatalf("Failed to get user2 keys: %v", err)
	}
	if len(keys2) != 1 {
		t.Errorf("Expected 1 key for user2, got %d", len(keys2))
	}
}

func TestValidateAPIKey(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	userID := createTestUser(t, db, "test@example.com")
	ctx := context.Background()

	result, err := db.CreateAPIKey(ctx, userID, "Valid Key", nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	// Validate the key
	gotUserID, _, err := db.ValidateAPIKey(ctx, result.Key)
	if err != nil {
		t.Fatalf("ValidateAPIKey failed: %v", err)
	}
	if gotUserID != userID {
		t.Errorf("Expected user_id %d, got %d", userID, gotUserID)
	}
}

func TestValidateAPIKey_Invalid(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _, err := db.ValidateAPIKey(ctx, "completely-invalid-key")
	if err == nil {
		t.Fatal("Expected error for invalid API key")
	}
}

func TestValidateAPIKey_Expired(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	userID := createTestUser(t, db, "test@example.com")
	ctx := context.Background()

	// Create a key that already expired
	pastTime := time.Now().Add(-1 * time.Hour)
	result, err := db.CreateAPIKey(ctx, userID, "Expired Key", &pastTime)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	_, _, err = db.ValidateAPIKey(ctx, result.Key)
	if err == nil {
		t.Fatal("Expected error for expired API key")
	}
}

func TestValidateAPIKey_UpdatesLastUsed(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	userID := createTestUser(t, db, "test@example.com")
	ctx := context.Background()

	result, err := db.CreateAPIKey(ctx, userID, "Test Key", nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	// Validate to trigger last_used_at update
	_, _, err = db.ValidateAPIKey(ctx, result.Key)
	if err != nil {
		t.Fatalf("ValidateAPIKey failed: %v", err)
	}

	// Check that last_used_at is set
	keys, err := db.GetAPIKeysByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetAPIKeysByUserID failed: %v", err)
	}
	if len(keys) != 1 {
		t.Fatalf("Expected 1 key, got %d", len(keys))
	}
	if keys[0].LastUsedAt == nil {
		t.Error("Expected last_used_at to be set after validation")
	}
}

func TestDeleteAPIKey(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	userID := createTestUser(t, db, "test@example.com")
	ctx := context.Background()

	result, err := db.CreateAPIKey(ctx, userID, "Delete Me", nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	err = db.DeleteAPIKey(ctx, result.ID, userID)
	if err != nil {
		t.Fatalf("DeleteAPIKey failed: %v", err)
	}

	// Verify key is gone
	keys, err := db.GetAPIKeysByUserID(ctx, userID)
	if err != nil {
		t.Fatalf("GetAPIKeysByUserID failed: %v", err)
	}
	if len(keys) != 0 {
		t.Errorf("Expected 0 keys after deletion, got %d", len(keys))
	}
}

func TestDeleteAPIKey_WrongUser(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	user1 := createTestUser(t, db, "user1@example.com")
	user2 := createTestUser(t, db, "user2@example.com")
	ctx := context.Background()

	result, err := db.CreateAPIKey(ctx, user1, "User1 Key", nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	// User2 should not be able to delete user1's key
	err = db.DeleteAPIKey(ctx, result.ID, user2)
	if err == nil {
		t.Fatal("Expected error when deleting another user's API key")
	}

	// Verify key still exists
	keys, err := db.GetAPIKeysByUserID(ctx, user1)
	if err != nil {
		t.Fatalf("GetAPIKeysByUserID failed: %v", err)
	}
	if len(keys) != 1 {
		t.Errorf("Expected key to still exist, got %d keys", len(keys))
	}
}

func TestDeleteAPIKey_NotFound(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	userID := createTestUser(t, db, "test@example.com")
	ctx := context.Background()

	err := db.DeleteAPIKey(ctx, 99999, userID)
	if err == nil {
		t.Fatal("Expected error when deleting non-existent API key")
	}
}

func TestGetUserByAPIKey(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	userID := createTestUser(t, db, "test@example.com")
	ctx := context.Background()

	result, err := db.CreateAPIKey(ctx, userID, "Test Key", nil)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	gotUserID, gotEmail, _, err := db.GetUserByAPIKey(ctx, result.Key)
	if err != nil {
		t.Fatalf("GetUserByAPIKey failed: %v", err)
	}
	if gotUserID != userID {
		t.Errorf("Expected user_id %d, got %d", userID, gotUserID)
	}
	if gotEmail != "test@example.com" {
		t.Errorf("Expected email 'test@example.com', got %q", gotEmail)
	}
}

func TestGetUserByAPIKey_InvalidKey(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	ctx := context.Background()

	_, _, _, err := db.GetUserByAPIKey(ctx, "invalid-key")
	if err == nil {
		t.Fatal("Expected error for invalid API key")
	}
}

func TestGetUserByAPIKey_ExpiredKey(t *testing.T) {
	db := newTestDB(t)
	defer db.Close()

	userID := createTestUser(t, db, "test@example.com")
	ctx := context.Background()

	pastTime := time.Now().Add(-1 * time.Hour)
	result, err := db.CreateAPIKey(ctx, userID, "Expired Key", &pastTime)
	if err != nil {
		t.Fatalf("CreateAPIKey failed: %v", err)
	}

	_, _, _, err = db.GetUserByAPIKey(ctx, result.Key)
	if err == nil {
		t.Fatal("Expected error for expired API key")
	}
}
