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
	if existing, ok := f.byKeycloakID[p.KeycloakID]; ok {
		// Mirrors PostgresProfileRepository's ON CONFLICT (keycloak_id) DO
		// NOTHING + re-select: a second writer that also missed the
		// find-before-insert check gets the winner's row back, not an
		// error, and its own payload is discarded rather than clobbering
		// the winner's.
		return existing, nil
	}
	p.ID = uuid.New()
	f.byKeycloakID[p.KeycloakID] = p
	f.inserted = append(f.inserted, p)
	return p, nil
}

// raceyRepo wraps fakeRepo so FindByKeycloakID always misses, simulating
// the race window between another goroutine's Insert and this goroutine's
// own now-stale Find — exactly what
// PostgresProfileRepository.Insert's ON CONFLICT handling is built to
// survive. Insert still goes to the shared, real state.
type raceyRepo struct {
	*fakeRepo
}

func (r *raceyRepo) FindByKeycloakID(context.Context, uuid.UUID) (domain.UserProfile, bool, error) {
	return domain.UserProfile{}, false, nil
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

// TestEnsureProfile_SurvivesRaceOnInsert covers the "no retry, no record"
// finding's root cause: EnsureProfile's own find-then-act is racy, so this
// asserts the fix lives where it has to — a losing Insert() call must
// resolve to the winner's existing profile, not error or create a second
// row, exactly like two near-simultaneous first logins for the same
// Keycloak identity against the real Postgres repository.
func TestEnsureProfile_SurvivesRaceOnInsert(t *testing.T) {
	inner := newFakeRepo()
	keycloakID := uuid.New()

	// Simulate a concurrent winner: another request already inserted this
	// keycloak_id's row directly against the shared state.
	winner, err := inner.Insert(context.Background(), domain.UserProfile{
		KeycloakID: keycloakID,
		Username:   "nekomata",
		AvatarURL:  "https://example.com/winner.png",
	})
	if err != nil {
		t.Fatalf("seed winning insert: %v", err)
	}

	// svc's FindByKeycloakID always misses (raceyRepo), so EnsureProfile
	// falls through to Insert exactly as it would if this call's own Find
	// had run a moment before the winner's Insert landed.
	svc := application.NewSyncService(&raceyRepo{inner})
	got, err := svc.EnsureProfile(context.Background(), application.SyncInput{
		KeycloakID: keycloakID,
		Username:   "nekomata",
		AvatarURL:  "https://example.com/loser.png",
	})
	if err != nil {
		t.Fatalf("EnsureProfile after concurrent insert: %v, want the winner's profile with no error", err)
	}
	if got.ID != winner.ID {
		t.Errorf("ID = %v, want winner's ID %v — a losing concurrent insert must return the existing row, not error", got.ID, winner.ID)
	}
	if got.AvatarURL != winner.AvatarURL {
		t.Errorf("AvatarURL = %q, want winner's %q — the losing insert's payload must not clobber the winning row", got.AvatarURL, winner.AvatarURL)
	}
	if len(inner.inserted) != 1 {
		t.Errorf("inserted count = %d, want 1 (the loser's insert must resolve via conflict, not append a second row)", len(inner.inserted))
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
