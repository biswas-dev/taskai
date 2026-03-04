package db

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"fmt"
	"strings"
	"time"

	gologin "github.com/anchoo2kewl/go-login"
	"go.uber.org/zap"
	"golang.org/x/crypto/bcrypt"

	"taskai/ent"
	"taskai/ent/invite"
)

// OAuthStore implements gologin.UserStore using the Ent ORM and raw SQL for
// the oauth_providers table (which lives outside the Ent schema).
type OAuthStore struct {
	db     *DB
	logger *zap.Logger
}

// NewOAuthStore returns an OAuthStore backed by the given DB.
func NewOAuthStore(db *DB) *OAuthStore {
	return &OAuthStore{db: db, logger: db.logger}
}

// FindUserByProviderID looks up a user via the oauth_providers join table.
// Returns (nil, nil) if no match is found.
func (s *OAuthStore) FindUserByProviderID(ctx context.Context, provider, providerUserID string) (*gologin.User, error) {
	var userID int64
	err := s.db.QueryRowContext(ctx,
		s.db.Rebind(`SELECT op.user_id FROM oauth_providers op
			JOIN users u ON u.id = op.user_id
			WHERE op.provider = ? AND op.provider_user_id = ?
			AND u.deleted_at IS NULL LIMIT 1`),
		provider, providerUserID,
	).Scan(&userID)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("oauth_store: FindUserByProviderID: %w", err)
	}

	entUser, err := s.db.Client.User.Get(ctx, userID)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("oauth_store: FindUserByProviderID: fetch user: %w", err)
	}
	return &gologin.User{ID: entUser.ID, Email: entUser.Email}, nil
}

// FindUserByEmail looks up a user by email address.
// Returns (nil, nil) if no match is found.
func (s *OAuthStore) FindUserByEmail(ctx context.Context, email string) (*gologin.User, error) {
	// Use raw SQL to exclude soft-deleted users (deleted_at not in ent schema)
	var userID int64
	var userEmail string
	err := s.db.QueryRowContext(ctx,
		s.db.Rebind(`SELECT id, email FROM users WHERE email = ? AND deleted_at IS NULL LIMIT 1`),
		email,
	).Scan(&userID, &userEmail)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("oauth_store: FindUserByEmail: %w", err)
	}
	return &gologin.User{ID: userID, Email: userEmail}, nil
}

// GetUserAuthProvider returns the auth_provider column value for the user.
func (s *OAuthStore) GetUserAuthProvider(ctx context.Context, userID int64) (string, error) {
	var provider string
	err := s.db.QueryRowContext(ctx,
		s.db.Rebind(`SELECT auth_provider FROM users WHERE id = ? LIMIT 1`), userID,
	).Scan(&provider)
	if err == sql.ErrNoRows {
		return "password", nil
	}
	if err != nil {
		return "", fmt.Errorf("oauth_store: GetUserAuthProvider: %w", err)
	}
	return provider, nil
}

// CreateOAuthUser creates a new user, their personal team, and the
// oauth_providers row in a single transaction. The invite code is claimed so
// it cannot be reused.
func (s *OAuthStore) CreateOAuthUser(ctx context.Context, info gologin.ProviderUserInfo, provider, inviteCode string) (*gologin.User, error) {
	// Verify the invite exists (caller already validated it; safety re-check).
	inv, err := s.db.Client.Invite.Query().
		Where(invite.Code(inviteCode)).
		Only(ctx)
	if err != nil {
		return nil, fmt.Errorf("oauth_store: CreateOAuthUser: invite lookup: %w", err)
	}
	if inv.UsedAt != nil {
		return nil, fmt.Errorf("oauth_store: CreateOAuthUser: invite already used")
	}

	// Generate a random bcrypt hash as placeholder password_hash.
	// OAuth users can never discover this value, so they can never use password login.
	fakeHash, err := randomPlaceholderHash()
	if err != nil {
		return nil, fmt.Errorf("oauth_store: CreateOAuthUser: %w", err)
	}

	tx, err := s.db.Client.Tx(ctx)
	if err != nil {
		return nil, fmt.Errorf("oauth_store: CreateOAuthUser: begin tx: %w", err)
	}
	defer tx.Rollback()

	// Build display name from provider info
	firstName := info.FirstName
	lastName := info.LastName
	if firstName == "" && info.Name != "" {
		parts := strings.SplitN(info.Name, " ", 2)
		firstName = parts[0]
		if len(parts) > 1 {
			lastName = parts[1]
		}
	}

	userCreate := tx.User.Create().
		SetEmail(info.Email).
		SetPasswordHash(fakeHash)

	if firstName != "" {
		userCreate = userCreate.SetFirstName(firstName)
	}
	if lastName != "" {
		userCreate = userCreate.SetLastName(lastName)
	}
	fullName := strings.TrimSpace(firstName + " " + lastName)
	if fullName != "" {
		userCreate = userCreate.SetName(fullName)
	}

	newUser, err := userCreate.Save(ctx)
	if err != nil {
		if ent.IsConstraintError(err) {
			return nil, fmt.Errorf("oauth_store: CreateOAuthUser: email already exists: %w", err)
		}
		return nil, fmt.Errorf("oauth_store: CreateOAuthUser: create user: %w", err)
	}

	// Create personal team
	displayName := fullName
	if displayName == "" {
		displayName = info.Email
	}
	team, err := tx.Team.Create().
		SetName(displayName + "'s Team").
		SetOwnerID(newUser.ID).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("oauth_store: CreateOAuthUser: create team: %w", err)
	}

	_, err = tx.TeamMember.Create().
		SetTeamID(team.ID).
		SetUserID(newUser.ID).
		SetRole("owner").
		SetStatus("active").
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("oauth_store: CreateOAuthUser: add team member: %w", err)
	}

	// Mark invite as used
	now := time.Now()
	_, err = tx.Invite.UpdateOneID(inv.ID).
		SetInviteeID(newUser.ID).
		SetUsedAt(now).
		Save(ctx)
	if err != nil {
		return nil, fmt.Errorf("oauth_store: CreateOAuthUser: mark invite used: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("oauth_store: CreateOAuthUser: commit: %w", err)
	}

	// Set auth_provider and insert oauth_providers row (raw SQL — not in Ent schema).
	if _, err = s.db.ExecContext(ctx,
		s.db.Rebind(`UPDATE users SET auth_provider = ? WHERE id = ?`),
		provider, newUser.ID,
	); err != nil {
		s.logger.Error("oauth_store: failed to set auth_provider",
			zap.Int64("user_id", newUser.ID),
			zap.String("provider", provider),
			zap.Error(err))
	}

	if _, err = s.db.ExecContext(ctx,
		s.db.Rebind(`INSERT INTO oauth_providers (user_id, provider, provider_user_id) VALUES (?, ?, ?)`),
		newUser.ID, provider, info.ProviderUserID,
	); err != nil {
		s.logger.Error("oauth_store: failed to insert oauth_providers row",
			zap.Int64("user_id", newUser.ID),
			zap.String("provider", provider),
			zap.Error(err))
	}

	s.logger.Info("OAuth user created",
		zap.Int64("user_id", newUser.ID),
		zap.Int64("team_id", team.ID),
		zap.String("email", newUser.Email),
		zap.String("provider", provider),
	)

	return &gologin.User{ID: newUser.ID, Email: newUser.Email}, nil
}

// ValidateInviteCode checks whether the given invite code is valid.
// Returns (nil, nil) when the code does not exist, is used, or is expired.
func (s *OAuthStore) ValidateInviteCode(ctx context.Context, code string) (*gologin.InviteInfo, error) {
	inv, err := s.db.Client.Invite.Query().
		Where(invite.Code(code)).
		WithInviter().
		Only(ctx)
	if err != nil {
		if ent.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("oauth_store: ValidateInviteCode: %w", err)
	}

	if inv.UsedAt != nil {
		return nil, nil
	}
	if inv.ExpiresAt != nil && time.Now().After(*inv.ExpiresAt) {
		return nil, nil
	}

	info := &gologin.InviteInfo{Code: inv.Code}
	if inv.Edges.Inviter != nil {
		info.InviterName = inv.Edges.Inviter.Email
	}
	if inv.ExpiresAt != nil {
		info.ExpiresAt = *inv.ExpiresAt
	}
	return info, nil
}

// randomPlaceholderHash generates a random bcrypt hash to use as a placeholder
// password for OAuth-only users (satisfies NOT NULL constraint; never usable).
func randomPlaceholderHash() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}
	// Use the hex representation as the "password" to bcrypt.
	hash, err := bcrypt.GenerateFromPassword([]byte(hex.EncodeToString(b)), bcrypt.MinCost)
	if err != nil {
		return "", fmt.Errorf("failed to hash placeholder password: %w", err)
	}
	return string(hash), nil
}
