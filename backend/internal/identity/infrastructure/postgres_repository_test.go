package infrastructure_test

import (
	"context"
	"os"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alfredomendoza/questlog/backend/internal/identity/domain"
	"github.com/alfredomendoza/questlog/backend/internal/identity/infrastructure"
)

func testPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Skip("DATABASE_URL not set; run against the docker-compose postgres to exercise this test")
	}
	pool, err := pgxpool.New(context.Background(), url)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

func TestPostgresProfileRepository_InsertAndFind(t *testing.T) {
	pool := testPool(t)
	repo := infrastructure.NewPostgresProfileRepository(pool)
	ctx := context.Background()

	keycloakID := uuid.New()
	profile, err := domain.NewUserProfile(keycloakID, "test_"+keycloakID.String()[:8], "https://example.com/a.png")
	if err != nil {
		t.Fatalf("NewUserProfile: %v", err)
	}
	// Give Bio a distinct, non-empty value so an empty-string default can't
	// masquerade as a passing round-trip.
	profile.Bio = "a distinct bio for round-trip verification"

	inserted, err := repo.Insert(ctx, profile)
	if err != nil {
		t.Fatalf("Insert: %v", err)
	}
	if inserted.ID == uuid.Nil {
		t.Fatal("Insert did not assign an ID")
	}

	found, ok, err := repo.FindByKeycloakID(ctx, keycloakID)
	if err != nil {
		t.Fatalf("FindByKeycloakID: %v", err)
	}
	if !ok {
		t.Fatal("expected profile to be found")
	}

	// Assert every field round-trips, not just Username, so a column-order
	// bug in the SELECT can't slip a valid-looking string into the wrong
	// field and still pass.
	if found.ID != inserted.ID {
		t.Errorf("ID = %v, want %v", found.ID, inserted.ID)
	}
	if found.KeycloakID != keycloakID {
		t.Errorf("KeycloakID = %v, want %v", found.KeycloakID, keycloakID)
	}
	if found.Username != profile.Username {
		t.Errorf("Username = %q, want %q", found.Username, profile.Username)
	}
	if found.AvatarURL != profile.AvatarURL {
		t.Errorf("AvatarURL = %q, want %q", found.AvatarURL, profile.AvatarURL)
	}
	if found.Bio != profile.Bio {
		t.Errorf("Bio = %q, want %q", found.Bio, profile.Bio)
	}
	// Insert never accepts a suspended flag (it's not one of the insertable
	// columns), so a freshly-inserted profile is always unsuspended here;
	// this asserts the DB default and that Scan lands the column in the
	// right field, not a positive "suspended = true survives" case.
	if found.Suspended {
		t.Error("Suspended = true, want false for a freshly inserted profile")
	}
	if found.CreatedAt.IsZero() {
		t.Error("CreatedAt is zero, want a server-assigned timestamp")
	}
	if found.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero, want a server-assigned timestamp")
	}
}

func TestPostgresProfileRepository_FindByKeycloakID_NotFound(t *testing.T) {
	pool := testPool(t)
	repo := infrastructure.NewPostgresProfileRepository(pool)

	got, ok, err := repo.FindByKeycloakID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("FindByKeycloakID: %v", err)
	}
	if ok {
		t.Fatal("expected not found for a random keycloak id")
	}
	if got != (domain.UserProfile{}) {
		t.Errorf("got = %+v, want zero value domain.UserProfile{}", got)
	}
}
