package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"time"

	"go.uber.org/zap"

	"taskai/ent"
	"taskai/ent/apikey"
)

// APIKey represents an API key for user authentication
type APIKey struct {
	ID         int64      `json:"id"`
	UserID     int64      `json:"user_id"`
	Name       string     `json:"name"`
	KeyPrefix  string     `json:"key_prefix"`
	LastUsedAt *time.Time `json:"last_used_at,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
	ExpiresAt  *time.Time `json:"expires_at,omitempty"`
}

// APIKeyWithSecret includes the full key (only returned on creation)
type APIKeyWithSecret struct {
	APIKey
	Key string `json:"key"`
}

// GenerateAPIKey creates a new API key with a cryptographically secure random value
func GenerateAPIKey() (key, keyHash, prefix string, err error) {
	// Generate 32 random bytes
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Encode to base64
	key = base64.URLEncoding.EncodeToString(b)

	// Create hash for storage
	hash := sha256.Sum256([]byte(key))
	keyHash = base64.URLEncoding.EncodeToString(hash[:])

	// Prefix for display (first 8 chars)
	prefix = key[:8]

	return key, keyHash, prefix, nil
}

// HashAPIKey creates a hash of an API key for comparison
func HashAPIKey(key string) string {
	hash := sha256.Sum256([]byte(key))
	return base64.URLEncoding.EncodeToString(hash[:])
}

// CreateAPIKey creates a new API key for a user
func (db *DB) CreateAPIKey(ctx context.Context, userID int64, name string, expiresAt *time.Time) (*APIKeyWithSecret, error) {
	// Generate API key
	key, keyHash, prefix, err := GenerateAPIKey()
	if err != nil {
		return nil, fmt.Errorf("failed to generate API key: %w", err)
	}

	// Create using Ent
	builder := db.Client.APIKey.Create().
		SetUserID(userID).
		SetName(name).
		SetKeyHash(keyHash).
		SetKeyPrefix(prefix)

	if expiresAt != nil {
		builder.SetExpiresAt(*expiresAt)
	}

	newAPIKey, err := builder.Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to insert API key: %w", err)
	}

	return &APIKeyWithSecret{
		APIKey: APIKey{
			ID:        newAPIKey.ID,
			UserID:    newAPIKey.UserID,
			Name:      newAPIKey.Name,
			KeyPrefix: newAPIKey.KeyPrefix,
			CreatedAt: newAPIKey.CreatedAt,
			ExpiresAt: newAPIKey.ExpiresAt,
		},
		Key: key,
	}, nil
}

// GetAPIKeysByUserID retrieves all API keys for a user
func (db *DB) GetAPIKeysByUserID(ctx context.Context, userID int64) ([]APIKey, error) {
	entKeys, err := db.Client.APIKey.Query().
		Where(apikey.UserID(userID)).
		Order(ent.Desc(apikey.FieldCreatedAt)).
		All(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to query API keys: %w", err)
	}

	keys := make([]APIKey, 0, len(entKeys))
	for _, ek := range entKeys {
		keys = append(keys, APIKey{
			ID:         ek.ID,
			UserID:     ek.UserID,
			Name:       ek.Name,
			KeyPrefix:  ek.KeyPrefix,
			LastUsedAt: ek.LastUsedAt,
			CreatedAt:  ek.CreatedAt,
			ExpiresAt:  ek.ExpiresAt,
		})
	}

	return keys, nil
}

// ValidateAPIKey checks if an API key is valid and returns the user ID and API key ID
func (db *DB) ValidateAPIKey(ctx context.Context, key string) (int64, int64, error) {
	keyHash := HashAPIKey(key)

	apiKeyEntity, err := db.Client.APIKey.Query().
		Where(apikey.KeyHash(keyHash)).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, 0, fmt.Errorf("invalid API key")
		}
		return 0, 0, fmt.Errorf("failed to validate API key: %w", err)
	}

	// Check expiration
	if apiKeyEntity.ExpiresAt != nil && apiKeyEntity.ExpiresAt.Before(time.Now()) {
		return 0, 0, fmt.Errorf("API key expired")
	}

	// Update last used timestamp
	now := time.Now()
	_, err = db.Client.APIKey.UpdateOneID(apiKeyEntity.ID).
		SetLastUsedAt(now).
		Save(ctx)
	if err != nil {
		// Log but don't fail on update error
		db.logger.Warn("Failed to update API key last_used_at", zap.Error(err))
	}

	return apiKeyEntity.UserID, apiKeyEntity.ID, nil
}

// DeleteAPIKey removes an API key
func (db *DB) DeleteAPIKey(ctx context.Context, keyID, userID int64) error {
	// Verify the API key belongs to the user before deleting
	apiKeyEntity, err := db.Client.APIKey.Query().
		Where(
			apikey.ID(keyID),
			apikey.UserID(userID),
		).
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return fmt.Errorf("API key not found or access denied")
		}
		return fmt.Errorf("failed to query API key: %w", err)
	}

	err = db.Client.APIKey.DeleteOneID(apiKeyEntity.ID).Exec(ctx)
	if err != nil {
		return fmt.Errorf("failed to delete API key: %w", err)
	}

	return nil
}

// GetUserByAPIKey retrieves user info using an API key and returns (userID, email, apiKeyID, error)
func (db *DB) GetUserByAPIKey(ctx context.Context, key string) (int64, string, int64, error) {
	userID, apiKeyID, err := db.ValidateAPIKey(ctx, key)
	if err != nil {
		return 0, "", 0, err
	}

	// Get user email
	userEntity, err := db.Client.User.Get(ctx, userID)
	if err != nil {
		if ent.IsNotFound(err) {
			return 0, "", 0, fmt.Errorf("user not found")
		}
		return 0, "", 0, fmt.Errorf("failed to get user: %w", err)
	}

	return userID, userEntity.Email, apiKeyID, nil
}
