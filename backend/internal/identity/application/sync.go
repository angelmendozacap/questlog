// Package application implements the identity context's use cases.
package application

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/alfredomendoza/questlog/backend/internal/identity/domain"
)

// SyncInput is what the interfaces layer extracts from a validated JWT.
type SyncInput struct {
	KeycloakID uuid.UUID
	Username   string
	AvatarURL  string
}

// SyncService implements the "first login" use case: ensure a local
// UserProfile exists for this Keycloak identity, without ever overwriting
// a profile the user has since customized (bio, avatar) on subsequent
// logins — sync only ever creates, it never updates an existing row.
type SyncService struct {
	repo domain.ProfileRepository
}

func NewSyncService(repo domain.ProfileRepository) *SyncService {
	return &SyncService{repo: repo}
}

func (s *SyncService) EnsureProfile(ctx context.Context, in SyncInput) (domain.UserProfile, error) {
	existing, found, err := s.repo.FindByKeycloakID(ctx, in.KeycloakID)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("sync: find profile: %w", err)
	}
	if found {
		return existing, nil
	}

	fresh, err := domain.NewUserProfile(in.KeycloakID, in.Username, in.AvatarURL)
	if err != nil {
		return domain.UserProfile{}, err
	}

	inserted, err := s.repo.Insert(ctx, fresh)
	if err != nil {
		return domain.UserProfile{}, fmt.Errorf("sync: insert profile: %w", err)
	}
	return inserted, nil
}
