package infrastructure_test

import (
	"context"
	"errors"
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

// TestPostgresProfileRepository_Insert_ConcurrentDuplicateResolves exercises
// the real ON CONFLICT (keycloak_id) DO NOTHING + re-select path against a
// real Postgres: a second Insert for a keycloak_id that already has a row
// (as would happen when two near-simultaneous first logins both miss
// SyncService's find-before-insert check) must return the existing row,
// not a unique-violation error, and must not clobber it with the second
// call's payload.
func TestPostgresProfileRepository_Insert_ConcurrentDuplicateResolves(t *testing.T) {
	pool := testPool(t)
	repo := infrastructure.NewPostgresProfileRepository(pool)
	ctx := context.Background()

	keycloakID := uuid.New()
	winner, err := domain.NewUserProfile(keycloakID, "test_"+keycloakID.String()[:8], "https://example.com/winner.png")
	if err != nil {
		t.Fatalf("NewUserProfile: %v", err)
	}
	inserted, err := repo.Insert(ctx, winner)
	if err != nil {
		t.Fatalf("first Insert: %v", err)
	}

	// Same keycloak_id, different username/avatar — as if a second,
	// slightly-later request for the same identity also thought it needed
	// to create the profile.
	loser, err := domain.NewUserProfile(keycloakID, "test_"+keycloakID.String()[:8]+"_loser", "https://example.com/loser.png")
	if err != nil {
		t.Fatalf("NewUserProfile: %v", err)
	}
	got, err := repo.Insert(ctx, loser)
	if err != nil {
		t.Fatalf("second Insert for the same keycloak_id returned an error, want the existing row: %v", err)
	}
	if got.ID != inserted.ID {
		t.Errorf("ID = %v, want winner's ID %v", got.ID, inserted.ID)
	}
	if got.Username != winner.Username {
		t.Errorf("Username = %q, want winner's %q — the losing insert must not overwrite it", got.Username, winner.Username)
	}
	if got.AvatarURL != winner.AvatarURL {
		t.Errorf("AvatarURL = %q, want winner's %q — the losing insert must not overwrite it", got.AvatarURL, winner.AvatarURL)
	}

	// And the table itself must still hold exactly the winner's row.
	found, ok, err := repo.FindByKeycloakID(ctx, keycloakID)
	if err != nil {
		t.Fatalf("FindByKeycloakID: %v", err)
	}
	if !ok {
		t.Fatal("expected profile to be found")
	}
	if found.Username != winner.Username {
		t.Errorf("stored Username = %q, want winner's %q", found.Username, winner.Username)
	}
}

// TestPostgresProfileRepository_Insert_UsernameConflict covers the other
// unique constraint: a *different* keycloak_id colliding on username (e.g.
// Keycloak's ephemeral local state losing track of a keycloak_id while
// Postgres's persistent state still has a row for that username) must
// surface as domain.ErrUsernameTaken, not a generic/opaque error, and must
// not silently rename anyone.
func TestPostgresProfileRepository_Insert_UsernameConflict(t *testing.T) {
	pool := testPool(t)
	repo := infrastructure.NewPostgresProfileRepository(pool)
	ctx := context.Background()

	username := "taken_" + uuid.New().String()[:8]
	first, err := domain.NewUserProfile(uuid.New(), username, "https://example.com/a.png")
	if err != nil {
		t.Fatalf("NewUserProfile: %v", err)
	}
	if _, err := repo.Insert(ctx, first); err != nil {
		t.Fatalf("first Insert: %v", err)
	}

	second, err := domain.NewUserProfile(uuid.New(), username, "https://example.com/b.png")
	if err != nil {
		t.Fatalf("NewUserProfile: %v", err)
	}
	_, err = repo.Insert(ctx, second)
	if !errors.Is(err, domain.ErrUsernameTaken) {
		t.Fatalf("Insert error = %v, want domain.ErrUsernameTaken", err)
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
