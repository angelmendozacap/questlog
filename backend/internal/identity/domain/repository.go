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
	// Insert stores a brand-new profile and returns it with ID/timestamps set.
	Insert(ctx context.Context, profile UserProfile) (UserProfile, error)
}
