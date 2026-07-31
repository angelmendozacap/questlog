// Package domain holds the identity context's aggregate: the local profile
// row synced from Keycloak on first login. It intentionally stores no
// role/authorization data — roles are always read live from the validated
// JWT (see internal/shared/authmw), never cached here.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// UserProfile is the identity context's aggregate root.
type UserProfile struct {
	ID         uuid.UUID
	KeycloakID uuid.UUID
	Username   string
	AvatarURL  string
	Bio        string
	Suspended  bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

var (
	ErrEmptyKeycloakID = errors.New("identity: keycloak id is required")
	ErrEmptyUsername   = errors.New("identity: username is required")
	// ErrUsernameTaken means the insert collided on username with a
	// *different* keycloak_id — not the ordinary "this identity already has
	// a profile" case (that's handled by EnsureProfile's find-first check
	// and by the repository treating a keycloak_id conflict as "found").
	// It happens because Keycloak's own state is ephemeral in local dev
	// (no volume; re-imported on every `docker compose up`) while
	// Postgres's is persistent: a user who re-registers the same username
	// gets a new keycloak_id, misses the lookup, and collides here. Sync
	// must not silently rename the user to paper over this — it's surfaced
	// as a distinct, actionable error instead.
	ErrUsernameTaken = errors.New("identity: username already in use by another account")
)

// NewUserProfile validates and builds a fresh profile for first-login sync.
// ID/CreatedAt/UpdatedAt are assigned by the repository on insert.
func NewUserProfile(keycloakID uuid.UUID, username, avatarURL string) (UserProfile, error) {
	if keycloakID == uuid.Nil {
		return UserProfile{}, ErrEmptyKeycloakID
	}
	if username == "" {
		return UserProfile{}, ErrEmptyUsername
	}
	return UserProfile{
		KeycloakID: keycloakID,
		Username:   username,
		AvatarURL:  avatarURL,
	}, nil
}
