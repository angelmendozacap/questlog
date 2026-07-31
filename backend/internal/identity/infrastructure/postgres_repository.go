// Package infrastructure implements the identity context's repository
// against Postgres (backend/cmd/migrate/migrations/000001_identity_schema).
package infrastructure

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/alfredomendoza/questlog/backend/internal/identity/domain"
)

// postgresUniqueViolation is Postgres's SQLSTATE for a unique constraint
// violation (23505).
const postgresUniqueViolation = "23505"

// usernameUniqueConstraint is the default name Postgres assigns an inline
// `UNIQUE` column constraint: <table>_<column>_key. See
// cmd/migrate/migrations/000001_identity_schema.up.sql.
const usernameUniqueConstraint = "user_profiles_username_key"

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

// Insert stores a brand-new profile and returns it with ID/timestamps set.
//
// It is idempotent on keycloak_id: two near-simultaneous first logins for
// the same identity can both miss SyncService's find-before-insert check
// and both call Insert. Rather than the second one erroring on the unique
// violation, ON CONFLICT (keycloak_id) DO NOTHING makes the insert a no-op
// for the loser, and the re-select below returns the winner's row — the
// caller sees "found the existing profile", not a failure.
//
// A conflict on username instead (a *different* keycloak_id) is not
// swallowed the same way: that means Keycloak's ephemeral local state and
// Postgres's persistent state have diverged (see domain.ErrUsernameTaken's
// doc comment), and silently treating it as "found" would hand the caller
// back a profile that belongs to a different identity. It's reported as a
// distinct, actionable error instead.
func (r *PostgresProfileRepository) Insert(ctx context.Context, p domain.UserProfile) (domain.UserProfile, error) {
	const q = `
		INSERT INTO identity.user_profiles (keycloak_id, username, avatar_url, bio)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (keycloak_id) DO NOTHING
		RETURNING id, keycloak_id, username, avatar_url, bio, suspended, created_at, updated_at`

	var out domain.UserProfile
	err := r.pool.QueryRow(ctx, q, p.KeycloakID, p.Username, p.AvatarURL, p.Bio).Scan(
		&out.ID, &out.KeycloakID, &out.Username, &out.AvatarURL, &out.Bio, &out.Suspended, &out.CreatedAt, &out.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// ON CONFLICT DO NOTHING means no row came back, not that the
		// insert failed — someone else won the race for this keycloak_id.
		// Re-select and hand back their row.
		existing, found, findErr := r.FindByKeycloakID(ctx, p.KeycloakID)
		if findErr != nil {
			return domain.UserProfile{}, findErr
		}
		if !found {
			// Vanishingly unlikely (the winner's row would have to be
			// deleted in the instant between the conflict and this
			// re-select), but surfaced distinctly rather than silently
			// returning a zero-value profile.
			return domain.UserProfile{}, fmt.Errorf("identity: insert conflicted on keycloak_id %s but no row found on re-select", p.KeycloakID)
		}
		return existing, nil
	}
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == postgresUniqueViolation && pgErr.ConstraintName == usernameUniqueConstraint {
			return domain.UserProfile{}, domain.ErrUsernameTaken
		}
		return domain.UserProfile{}, err
	}
	return out, nil
}

var _ domain.ProfileRepository = (*PostgresProfileRepository)(nil)
