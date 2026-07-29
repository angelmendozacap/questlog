// Package infrastructure implements the identity context's repository
// against Postgres (backend/cmd/migrate/migrations/000001_identity_schema).
package infrastructure

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alfredomendoza/questlog/backend/internal/identity/domain"
)

type PostgresProfileRepository struct {
	pool *pgxpool.Pool
}

func NewPostgresProfileRepository(pool *pgxpool.Pool) *PostgresProfileRepository {
	return &PostgresProfileRepository{pool: pool}
}

func (r *PostgresProfileRepository) FindByKeycloakID(ctx context.Context, keycloakID uuid.UUID) (domain.UserProfile, bool, error) {
	const q = `
		SELECT id, keycloak_id, username, avatar_url, bio, suspended, created_at, updated_at
		FROM identity.user_profiles
		WHERE keycloak_id = $1`

	var p domain.UserProfile
	err := r.pool.QueryRow(ctx, q, keycloakID).Scan(
		&p.ID, &p.KeycloakID, &p.Username, &p.AvatarURL, &p.Bio, &p.Suspended, &p.CreatedAt, &p.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.UserProfile{}, false, nil
	}
	if err != nil {
		return domain.UserProfile{}, false, err
	}
	return p, true, nil
}

func (r *PostgresProfileRepository) Insert(ctx context.Context, p domain.UserProfile) (domain.UserProfile, error) {
	const q = `
		INSERT INTO identity.user_profiles (keycloak_id, username, avatar_url, bio)
		VALUES ($1, $2, $3, $4)
		RETURNING id, keycloak_id, username, avatar_url, bio, suspended, created_at, updated_at`

	var out domain.UserProfile
	err := r.pool.QueryRow(ctx, q, p.KeycloakID, p.Username, p.AvatarURL, p.Bio).Scan(
		&out.ID, &out.KeycloakID, &out.Username, &out.AvatarURL, &out.Bio, &out.Suspended, &out.CreatedAt, &out.UpdatedAt,
	)
	return out, err
}

var _ domain.ProfileRepository = (*PostgresProfileRepository)(nil)
