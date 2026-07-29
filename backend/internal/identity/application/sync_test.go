package application_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/alfredomendoza/questlog/backend/internal/identity/application"
	"github.com/alfredomendoza/questlog/backend/internal/identity/domain"
)

type fakeRepo struct {
	byKeycloakID map[uuid.UUID]domain.UserProfile
	inserted     []domain.UserProfile
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{byKeycloakID: map[uuid.UUID]domain.UserProfile{}}
}

func (f *fakeRepo) FindByKeycloakID(_ context.Context, id uuid.UUID) (domain.UserProfile, bool, error) {
	p, ok := f.byKeycloakID[id]
	return p, ok, nil
}

func (f *fakeRepo) Insert(_ context.Context, p domain.UserProfile) (domain.UserProfile, error) {
	p.ID = uuid.New()
	f.byKeycloakID[p.KeycloakID] = p
	f.inserted = append(f.inserted, p)
	return p, nil
}

func TestEnsureProfile_CreatesOnFirstLogin(t *testing.T) {
	repo := newFakeRepo()
	svc := application.NewSyncService(repo)
	keycloakID := uuid.New()

	got, err := svc.EnsureProfile(context.Background(), application.SyncInput{
		KeycloakID: keycloakID,
		Username:   "nekomata",
		AvatarURL:  "https://example.com/a.png",
	})
	if err != nil {
		t.Fatalf("EnsureProfile: %v", err)
	}
	if got.Username != "nekomata" {
		t.Errorf("Username = %q, want %q", got.Username, "nekomata")
	}
	if got.ID == uuid.Nil {
		t.Error("ID was not assigned")
	}
	if len(repo.inserted) != 1 {
		t.Fatalf("inserted count = %d, want 1", len(repo.inserted))
	}
}

func TestEnsureProfile_ReturnsExistingWithoutOverwriting(t *testing.T) {
	repo := newFakeRepo()
	svc := application.NewSyncService(repo)
	keycloakID := uuid.New()

	first, err := svc.EnsureProfile(context.Background(), application.SyncInput{
		KeycloakID: keycloakID,
		Username:   "nekomata",
		AvatarURL:  "https://example.com/a.png",
	})
	if err != nil {
		t.Fatalf("first EnsureProfile: %v", err)
	}

	// Simulate the user having since customized their bio directly (not via sync).
	customized := first
	customized.Bio = "cine de animación y RPGs"
	repo.byKeycloakID[keycloakID] = customized

	second, err := svc.EnsureProfile(context.Background(), application.SyncInput{
		KeycloakID: keycloakID,
		Username:   "nekomata",
		AvatarURL:  "https://example.com/a-new-picture.png",
	})
	if err != nil {
		t.Fatalf("second EnsureProfile: %v", err)
	}
	if second.Bio != "cine de animación y RPGs" {
		t.Errorf("Bio = %q, want preserved customization, sync must not overwrite", second.Bio)
	}
	if second.AvatarURL != "https://example.com/a.png" {
		t.Errorf("AvatarURL = %q, want preserved stored value %q, sync must not overwrite with new login's avatar", second.AvatarURL, "https://example.com/a.png")
	}
	if len(repo.inserted) != 1 {
		t.Errorf("inserted count = %d, want 1 (no re-insert on second login)", len(repo.inserted))
	}
}

func TestEnsureProfile_RejectsEmptyUsername(t *testing.T) {
	repo := newFakeRepo()
	svc := application.NewSyncService(repo)

	_, err := svc.EnsureProfile(context.Background(), application.SyncInput{
		KeycloakID: uuid.New(),
		Username:   "",
	})
	if err == nil {
		t.Fatal("expected error for empty username, got nil")
	}
}
