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
