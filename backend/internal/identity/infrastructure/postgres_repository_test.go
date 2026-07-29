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
	if found.Username != profile.Username {
		t.Errorf("Username = %q, want %q", found.Username, profile.Username)
	}
}

func TestPostgresProfileRepository_FindByKeycloakID_NotFound(t *testing.T) {
	pool := testPool(t)
	repo := infrastructure.NewPostgresProfileRepository(pool)

	_, ok, err := repo.FindByKeycloakID(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("FindByKeycloakID: %v", err)
	}
	if ok {
		t.Fatal("expected not found for a random keycloak id")
	}
}
