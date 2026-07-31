package domain

import (
	"context"

	"github.com/google/uuid"
)

// ProfileRepository persists UserProfile aggregates. Implemented against
// Postgres in infrastructure/postgres_repository.go; application code
// depends only on this interface.
type ProfileRepository interface {
	// FindByKeycloakID returns (profile, true, nil) if found,
	// (zero, false, nil) if not found, or (zero, false, err) on failure.
	FindByKeycloakID(ctx context.Context, keycloakID uuid.UUID) (UserProfile, bool, error)
	// Insert stores a brand-new profile and returns it with ID/timestamps
	// set. Implementations must be idempotent on a keycloak_id conflict
	// (return the existing row instead of erroring — see
	// PostgresProfileRepository.Insert) and must return ErrUsernameTaken,
	// not a generic error, on a username conflict with a different
	// keycloak_id.
	Insert(ctx context.Context, profile UserProfile) (UserProfile, error)
}
