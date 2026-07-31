# Phase 3 — Auth & Identity Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Wire real authentication end-to-end — Keycloak realm, NextAuth in both Next.js apps, Go JWT middleware with role enforcement, and an `identity` bounded context that syncs a local user profile on first login — so a person can actually sign up, log in, and reach an authenticated page in `web`, and only `admin`-role users can reach `admin`.

**Architecture:** Keycloak (OIDC provider, username/password only, self-registration on) issues tokens. NextAuth (`packages/auth`, shared config) drives the browser-facing login/session in both apps and calls a Go endpoint (`POST /auth/sync`) right after login to upsert a local profile. Go's `internal/shared/authmw` validates every protected request's Bearer JWT against Keycloak's JWKS (signature, expiry and issuer — see ADR-0001 for why the JWKS host and the pinned issuer host differ) and enforces realm roles. `internal/identity` is a small DDD context: `UserProfile` domain aggregate, a `SyncService` use case, and a Postgres repository — the first context to touch a real database table, so this plan also wires `golang-migrate` for real for the first time.

**Tech Stack:** Keycloak 26 (existing compose service), NextAuth (Auth.js) v5 + `next-auth/providers/keycloak`, Go + Fiber v3 (existing), `golang-jwt/jwt/v5`, `golang-migrate/migrate/v4` (embedded SQL via `go:embed`), `jackc/pgx/v5` (`pgxpool`), `google/uuid`.

## Global Constraints

- Username/password login only — no social IdPs (explicit user decision).
- Self-registration enabled on the Keycloak realm (`registrationAllowed: true`) — Phase 3's own acceptance criterion is "signup → login → authenticated page."
- Every new Go dependency goes through `go get <module>@latest` from `backend/`, never hand-written into `go.mod` — matches how every dependency so far in this repo was added.
- `identity` has no `docs/specs/NN-*.md` of its own (it was intentionally left out of Phase 1's spec list — it's infrastructure, not a feature with user stories). Its (small) domain design lives entirely in this plan.
- Go's JWT middleware validates **signature (via JWKS), expiry, and issuer**. The issuer is pinned to the `KEYCLOAK_ISSUER` env var, which must equal the browser-facing realm URL (`http://localhost:8082/realms/questlog`) because that is what Keycloak stamps into every token's `iss` claim once `KC_HOSTNAME` is set (Task 8). Note this deliberately differs from `KEYCLOAK_JWKS_URL`, which uses the container-internal host — the backend *fetches* keys over the Docker network but *validates* `iss` against the public URL. ADR-0001 explains why those two URLs must differ.
- `UserProfile` stores **no role/authorization data** — roles are always read live from the validated JWT's `realm_access.roles` (via `authmw.Claims.HasRole`), never cached locally. The local profile is about display identity (username/avatar/bio) and moderation state (`suspended`, unused until Phase 8) only. Caching authorization state locally would drift the moment an admin changes someone's Keycloak role.
- Full Playwright E2E automation is explicitly **Phase 9** scope (PLAN.md), not this phase. This plan's own "E2E" acceptance criterion is satisfied by a manual/scripted verification checklist (Task 12) run against the real `docker compose` stack, plus real automated Go tests for the highest-risk piece (JWT middleware). Flagging this here so the scope cut is a stated decision, not a silent one.
- All dev-only secrets in this plan (Keycloak client secrets, `AUTH_SECRET`) are hardcoded, clearly-marked dev values — this project has no cloud deployment target yet (PLAN.md non-goals), consistent with `deploy/docker-compose.yml`'s existing `POSTGRES_PASSWORD: questlog`-style pragmatism.

---

## File Structure

New files this plan creates, by concern:

```
backend/
├── cmd/migrate/
│   ├── main.go                                  (rewrite — real golang-migrate runner)
│   └── migrations/
│       ├── 000001_identity_schema.up.sql
│       └── 000001_identity_schema.down.sql
├── cmd/public-api/main.go                        (rewrite — DB pool, JWKS, /auth/sync)
├── cmd/admin-api/main.go                         (rewrite — JWKS, /admin/whoami)
├── internal/identity/
│   ├── domain/
│   │   ├── user_profile.go
│   │   └── repository.go
│   ├── application/
│   │   ├── sync.go
│   │   └── sync_test.go
│   └── infrastructure/
│       ├── postgres_repository.go
│       └── postgres_repository_test.go
└── internal/shared/
    ├── env.go                                    (MustEnv helper)
    └── authmw/
        ├── jwks.go
        ├── middleware.go
        └── middleware_test.go

deploy/
├── keycloak/questlog-realm.json
└── docker-compose.yml                            (rewrite — migrate service, env vars, KC_HOSTNAME)

docs/
├── adr/0001-keycloak-docker-network-split.md
└── verify-phase-3.md

packages/
├── ui/src/tokens.css                             (add [data-admin] accent override)
└── auth/
    ├── package.json                              (rewrite — real deps)
    └── src/
        ├── index.ts                              (rewrite)
        ├── config.ts
        └── types.d.ts

apps/web/
├── next.config.ts                                (add @questlog/auth to transpilePackages)
├── package.json                                  (add next-auth dep)
├── .env.example
├── messages/{es,en}.json                         (rewrite — add cuenta + home.account keys)
└── src/
    ├── auth.ts
    ├── app/api/auth/[...nextauth]/route.ts
    ├── app/[locale]/page.tsx                     (small edit — link to cuenta)
    └── app/[locale]/cuenta/page.tsx

apps/admin/
├── next.config.ts                                (add @questlog/auth to transpilePackages)
├── package.json                                  (add next-auth dep)
├── .env.example
├── messages/{es,en}.json                         (rewrite — add admin namespace)
└── src/
    ├── auth.ts
    ├── app/api/auth/[...nextauth]/route.ts
    └── app/[locale]/layout.tsx                   (rewrite — role gate)

PLAN.md                                            (check off Phase 3)
```

---

### Task 1: `cmd/migrate` for real + identity schema migration

**Files:**
- Create: `backend/cmd/migrate/migrations/000001_identity_schema.up.sql`
- Create: `backend/cmd/migrate/migrations/000001_identity_schema.down.sql`
- Modify: `backend/cmd/migrate/main.go` (currently a stub — full rewrite)

**Interfaces:**
- Produces: `identity.user_profiles` table — columns `id UUID`, `keycloak_id UUID UNIQUE`, `username TEXT UNIQUE`, `avatar_url TEXT`, `bio TEXT`, `suspended BOOLEAN`, `created_at`/`updated_at TIMESTAMPTZ`. Task 4's repository depends on this exact shape.
- Consumes: `DATABASE_URL` env var (already used by `public-api`/`admin-api` in `deploy/docker-compose.yml`).

- [ ] **Step 1: Add the migration SQL files**

`backend/cmd/migrate/migrations/000001_identity_schema.up.sql`:

```sql
CREATE SCHEMA IF NOT EXISTS identity;

CREATE TABLE identity.user_profiles (
    id          UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    keycloak_id UUID NOT NULL UNIQUE,
    username    TEXT NOT NULL UNIQUE,
    avatar_url  TEXT NOT NULL DEFAULT '',
    bio         TEXT NOT NULL DEFAULT '',
    suspended   BOOLEAN NOT NULL DEFAULT false,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
```

`backend/cmd/migrate/migrations/000001_identity_schema.down.sql`:

```sql
DROP SCHEMA IF EXISTS identity CASCADE;
```

- [ ] **Step 2: Add the Go dependencies**

Run from `backend/`:

```bash
cd backend
go get github.com/golang-migrate/migrate/v4@latest
go get github.com/jackc/pgx/v5@latest
```

- [ ] **Step 3: Rewrite `cmd/migrate/main.go`**

```go
// Command migrate applies golang-migrate migrations embedded from
// cmd/migrate/migrations against DATABASE_URL. Run automatically as a
// one-shot service in deploy/docker-compose.yml before the API binaries
// start.
package main

import (
	"database/sql"
	"embed"
	"errors"
	"log"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/source/iofs"
	_ "github.com/jackc/pgx/v5/stdlib"

	"github.com/alfredomendoza/questlog/backend/internal/shared"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	dbURL := shared.MustEnv("DATABASE_URL")

	db, err := sql.Open("pgx", dbURL)
	if err != nil {
		log.Fatalf("migrate: open db: %v", err)
	}
	defer db.Close()

	driver, err := postgres.WithInstance(db, &postgres.Config{})
	if err != nil {
		log.Fatalf("migrate: postgres driver: %v", err)
	}

	src, err := iofs.New(migrationsFS, "migrations")
	if err != nil {
		log.Fatalf("migrate: migrations source: %v", err)
	}

	m, err := migrate.NewWithInstance("iofs", src, "questlog", driver)
	if err != nil {
		log.Fatalf("migrate: init: %v", err)
	}

	if err := m.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		log.Fatalf("migrate: up: %v", err)
	}

	log.Println("migrate: up to date")
}
```

This references `shared.MustEnv`, added in Task 5, Step 1 — build this task together with Task 5's Step 1, or stub `MustEnv` inline temporarily. To keep this task buildable standalone, add `MustEnv` now (Task 5 will already have it and this becomes a no-op there):

`backend/internal/shared/env.go`:

```go
package shared

import (
	"log"
	"os"
)

// MustEnv returns the environment variable's value or exits the process —
// used by cmd/* binaries for required startup configuration.
func MustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		log.Fatalf("missing required env var %s", key)
	}
	return v
}
```

- [ ] **Step 4: Build to verify it compiles**

```bash
cd backend
go build ./...
```

Expected: no errors.

- [ ] **Step 5: Run it against the real dockerized Postgres**

```bash
docker compose -f ../deploy/docker-compose.yml up -d postgres
# from backend/:
DATABASE_URL=postgres://questlog:questlog@localhost:5432/questlog?sslmode=disable \
  go run ./cmd/migrate
```

Expected output: `migrate: up to date`. Then confirm the table exists:

```bash
docker compose -f ../deploy/docker-compose.yml exec postgres \
  psql -U questlog -d questlog -c '\d identity.user_profiles'
```

Expected: column list matching Step 1's SQL.

- [ ] **Step 6: Commit**

```bash
git add backend/cmd/migrate backend/internal/shared/env.go backend/go.mod backend/go.sum
git commit -m "✨ Wire real migrations: identity.user_profiles table"
```

---

### Task 2: Identity domain — `UserProfile` + `ProfileRepository`

**Files:**
- Create: `backend/internal/identity/domain/user_profile.go`
- Create: `backend/internal/identity/domain/repository.go`

**Interfaces:**
- Consumes: nothing (pure domain layer).
- Produces: `domain.UserProfile` struct (fields: `ID, KeycloakID uuid.UUID`; `Username, AvatarURL, Bio string`; `Suspended bool`; `CreatedAt, UpdatedAt time.Time`), `domain.NewUserProfile(keycloakID uuid.UUID, username, avatarURL string) (UserProfile, error)`, `domain.ProfileRepository` interface with `FindByKeycloakID(ctx, uuid.UUID) (UserProfile, bool, error)` and `Insert(ctx, UserProfile) (UserProfile, error)`. Task 3 (application) and Task 4 (infrastructure) both depend on these exact names.

- [ ] **Step 1: Add the `google/uuid` dependency**

```bash
cd backend
go get github.com/google/uuid@latest
```

(Likely already present transitively via `oapi-codegen`'s generated code — running `go get` explicitly makes it a direct dependency, which is correct since `domain` imports it directly.)

- [ ] **Step 2: Write `user_profile.go`**

`backend/internal/identity/domain/user_profile.go`:

```go
// Package domain holds the identity context's aggregate: the local profile
// row synced from Keycloak on first login. It intentionally stores no
// role/authorization data — roles are always read live from the validated
// JWT (see internal/shared/authmw), never cached here.
package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// UserProfile is the identity context's aggregate root.
type UserProfile struct {
	ID         uuid.UUID
	KeycloakID uuid.UUID
	Username   string
	AvatarURL  string
	Bio        string
	Suspended  bool
	CreatedAt  time.Time
	UpdatedAt  time.Time
}

var (
	ErrEmptyKeycloakID = errors.New("identity: keycloak id is required")
	ErrEmptyUsername   = errors.New("identity: username is required")
)

// NewUserProfile validates and builds a fresh profile for first-login sync.
// ID/CreatedAt/UpdatedAt are assigned by the repository on insert.
func NewUserProfile(keycloakID uuid.UUID, username, avatarURL string) (UserProfile, error) {
	if keycloakID == uuid.Nil {
		return UserProfile{}, ErrEmptyKeycloakID
	}
	if username == "" {
		return UserProfile{}, ErrEmptyUsername
	}
	return UserProfile{
		KeycloakID: keycloakID,
		Username:   username,
		AvatarURL:  avatarURL,
	}, nil
}
```

- [ ] **Step 3: Write `repository.go`**

`backend/internal/identity/domain/repository.go`:

```go
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
```

- [ ] **Step 4: Build to verify it compiles**

```bash
cd backend
go build ./internal/identity/...
```

Expected: no errors (no `.go` files reference this package yet, so this just checks syntax/types).

- [ ] **Step 5: Commit**

```bash
git add backend/internal/identity/domain backend/go.mod backend/go.sum
git commit -m "✨ Add identity domain: UserProfile aggregate + repository interface"
```

---

### Task 3: Identity application — `SyncService` (TDD)

**Files:**
- Create: `backend/internal/identity/application/sync_test.go`
- Create: `backend/internal/identity/application/sync.go`

**Interfaces:**
- Consumes: `domain.UserProfile`, `domain.ProfileRepository`, `domain.NewUserProfile` (Task 2).
- Produces: `application.SyncInput` struct (`KeycloakID uuid.UUID`, `Username, AvatarURL string`), `application.NewSyncService(repo domain.ProfileRepository) *SyncService`, `(*SyncService).EnsureProfile(ctx, SyncInput) (domain.UserProfile, error)`. Task 6 (public-api wiring) depends on these exact names.

- [ ] **Step 1: Write the failing tests**

`backend/internal/identity/application/sync_test.go`:

```go
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
```

- [ ] **Step 2: Run the tests to verify they fail**

```bash
cd backend
go test ./internal/identity/application/... -v
```

Expected: FAIL — `application.SyncInput`/`NewSyncService` undefined (package doesn't exist yet).

- [ ] **Step 3: Write the implementation**

`backend/internal/identity/application/sync.go`:

```go
// Package application implements the identity context's use cases.
package application

import (
	"context"

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
		return domain.UserProfile{}, err
	}
	if found {
		return existing, nil
	}

	fresh, err := domain.NewUserProfile(in.KeycloakID, in.Username, in.AvatarURL)
	if err != nil {
		return domain.UserProfile{}, err
	}
	return s.repo.Insert(ctx, fresh)
}
```

- [ ] **Step 4: Run the tests to verify they pass**

```bash
cd backend
go test ./internal/identity/application/... -v
```

Expected: PASS — all three tests green.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/identity/application
git commit -m "✅ Add SyncService: first-login profile sync, TDD'd"
```

---

### Task 4: Identity infrastructure — Postgres repository

**Files:**
- Create: `backend/internal/identity/infrastructure/postgres_repository.go`
- Create: `backend/internal/identity/infrastructure/postgres_repository_test.go`

**Interfaces:**
- Consumes: `domain.UserProfile`, `domain.ProfileRepository` (Task 2); the `identity.user_profiles` table (Task 1).
- Produces: `infrastructure.NewPostgresProfileRepository(pool *pgxpool.Pool) *PostgresProfileRepository`, implementing `domain.ProfileRepository`. Task 6 depends on this exact constructor name.

- [ ] **Step 1: Write the repository**

`backend/internal/identity/infrastructure/postgres_repository.go`:

```go
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
```

- [ ] **Step 2: Write the integration test**

`backend/internal/identity/infrastructure/postgres_repository_test.go`:

```go
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
```

- [ ] **Step 3: Run it against the real dockerized Postgres**

Requires Task 1's migration already applied (Task 1, Step 5).

```bash
docker compose -f ../deploy/docker-compose.yml up -d postgres
cd backend
DATABASE_URL=postgres://questlog:questlog@localhost:5432/questlog?sslmode=disable \
  go test ./internal/identity/infrastructure/... -v
```

Expected: PASS — both tests green.

- [ ] **Step 4: Confirm the skip path works without a database**

```bash
cd backend
go test ./internal/identity/infrastructure/... -v
```

Expected: `SKIP` for both tests (no `DATABASE_URL` set), not a failure — this is what keeps `go test ./...` green in CI without a live Postgres.

- [ ] **Step 5: Commit**

```bash
git add backend/internal/identity/infrastructure backend/go.mod backend/go.sum
git commit -m "✨ Add PostgresProfileRepository, integration-tested"
```

---

### Task 5: Shared JWT middleware — `internal/shared/authmw` (TDD)

**Files:**
- Create: `backend/internal/shared/authmw/jwks.go`
- Create: `backend/internal/shared/authmw/middleware.go`
- Create: `backend/internal/shared/authmw/middleware_test.go`
- (Task 1 already created `backend/internal/shared/env.go` — skip if done.)

**Interfaces:**
- Consumes: nothing beyond stdlib + `golang-jwt/jwt/v5` + Fiber.
- Produces: `authmw.NewJWKS(url string) *JWKS`, `(*JWKS).Refresh(ctx) error`, `(*JWKS).Keyfunc jwt.Keyfunc` (method value usable directly as a `jwt.Keyfunc`), `(*JWKS).StartBackgroundRefresh(interval time.Duration)`, `authmw.Claims` struct (embeds `jwt.RegisteredClaims`; fields `PreferredUsername, Email, Picture string`; method `HasRole(role string) bool`), `authmw.RequireAuth(keyFunc jwt.Keyfunc, issuer string) fiber.Handler` (note the **two** parameters — the issuer to pin), `authmw.RequireRole(role string) fiber.Handler`, `authmw.ClaimsFromContext(c fiber.Ctx) (*Claims, bool)`. Tasks 6 and 7 depend on every one of these exact names and signatures.

- [ ] **Step 1: Add the `golang-jwt` dependency**

```bash
cd backend
go get github.com/golang-jwt/jwt/v5@latest
```

- [ ] **Step 2: Write the JWKS fetcher/cache**

`backend/internal/shared/authmw/jwks.go`:

```go
// Package authmw validates Keycloak-issued JWTs on incoming requests and
// enforces realm roles. Tokens are checked for signature (against the
// realm's JWKS), expiry, and issuer.
//
// The JWKS URL and the expected issuer intentionally point at different
// hosts: keys are fetched over the Docker network (keycloak:8080) while
// `iss` is pinned to the browser-facing URL Keycloak stamps into tokens
// (localhost:8082). See docs/adr/0001-keycloak-docker-network-split.md.
package authmw

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type jwksKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

// JWKS fetches and caches RSA public keys from a Keycloak JWKS endpoint,
// keyed by `kid`, so RequireAuth can validate RS256-signed tokens.
type JWKS struct {
	url  string
	mu   sync.RWMutex
	keys map[string]*rsa.PublicKey
}

func NewJWKS(url string) *JWKS {
	return &JWKS{url: url, keys: map[string]*rsa.PublicKey{}}
}

// Refresh fetches the current key set and replaces the cache atomically.
func (j *JWKS) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authmw: jwks fetch %s: status %d", j.url, resp.StatusCode)
	}

	var parsed jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("authmw: decode jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(parsed.Keys))
	for _, k := range parsed.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k.N, k.E)
		if err != nil {
			return fmt.Errorf("authmw: parse jwk %s: %w", k.Kid, err)
		}
		keys[k.Kid] = pub
	}

	j.mu.Lock()
	j.keys = keys
	j.mu.Unlock()
	return nil
}

// StartBackgroundRefresh refreshes the key cache on a ticker. Refresh
// failures are logged to stderr and the previous cache is kept — a
// transient Keycloak outage shouldn't take down already-validating auth.
func (j *JWKS) StartBackgroundRefresh(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := j.Refresh(context.Background()); err != nil {
				fmt.Printf("authmw: background jwks refresh failed (keeping cached keys): %v\n", err)
			}
		}
	}()
}

// Keyfunc implements jwt.Keyfunc against the cached key set.
func (j *JWKS) Keyfunc(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
		return nil, fmt.Errorf("authmw: unexpected signing method %v", token.Header["alg"])
	}
	kid, _ := token.Header["kid"].(string)

	j.mu.RLock()
	key, ok := j.keys[kid]
	j.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("authmw: unknown key id %q", kid)
	}
	return key, nil
}

func rsaPublicKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}
```

- [ ] **Step 3: Write the failing middleware tests**

`backend/internal/shared/authmw/middleware_test.go`:

```go
package authmw_test

import (
	"crypto/rand"
	"crypto/rsa"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"

	"github.com/alfredomendoza/questlog/backend/internal/shared/authmw"
)

// testIssuer stands in for the browser-facing realm URL that Keycloak
// stamps into every token's `iss` claim.
const testIssuer = "http://localhost:8082/realms/questlog"

func testKeyPair(t *testing.T) *rsa.PrivateKey {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	return key
}

func testJWKS(pub *rsa.PublicKey) *authmw.JWKS {
	j := authmw.NewJWKS("http://unused.invalid/certs")
	j.SetKeysForTest(map[string]*rsa.PublicKey{"test-kid": pub})
	return j
}

func testToken(t *testing.T, key *rsa.PrivateKey, claims authmw.Claims) string {
	t.Helper()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-kid"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

func TestRequireAuth_ValidToken(t *testing.T) {
	key := testKeyPair(t)
	jwks := testJWKS(&key.PublicKey)

	claims := authmw.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		PreferredUsername: "nekomata",
	}
	claims.RealmAccess.Roles = []string{"user"}
	token := testToken(t, key, claims)

	app := fiber.New()
	app.Get("/whoami", authmw.RequireAuth(jwks.Keyfunc, testIssuer), func(c fiber.Ctx) error {
		got, ok := authmw.ClaimsFromContext(c)
		if !ok {
			t.Fatal("claims not found in context")
		}
		return c.JSON(fiber.Map{"username": got.PreferredUsername})
	})

	req := httptest.NewRequest("GET", "/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}

func TestRequireAuth_MissingToken(t *testing.T) {
	key := testKeyPair(t)
	jwks := testJWKS(&key.PublicKey)

	app := fiber.New()
	app.Get("/whoami", authmw.RequireAuth(jwks.Keyfunc, testIssuer), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/whoami", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestRequireAuth_ExpiredToken(t *testing.T) {
	key := testKeyPair(t)
	jwks := testJWKS(&key.PublicKey)

	claims := authmw.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(-time.Hour)),
		},
	}
	token := testToken(t, key, claims)

	app := fiber.New()
	app.Get("/whoami", authmw.RequireAuth(jwks.Keyfunc, testIssuer), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for expired token", resp.StatusCode)
	}
}

// A correctly-signed, unexpired token from the wrong realm must still be
// rejected — this is the whole point of pinning `iss`.
func TestRequireAuth_WrongIssuer(t *testing.T) {
	key := testKeyPair(t)
	jwks := testJWKS(&key.PublicKey)

	claims := authmw.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    "http://evil.example/realms/questlog",
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	claims.RealmAccess.Roles = []string{"user", "admin"}
	token := testToken(t, key, claims)

	app := fiber.New()
	app.Get("/whoami", authmw.RequireAuth(jwks.Keyfunc, testIssuer), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for wrong issuer", resp.StatusCode)
	}
}

// A token with no `exp` at all must be rejected rather than treated as
// never-expiring — that's what jwt.WithExpirationRequired() buys us.
func TestRequireAuth_MissingExpiry(t *testing.T) {
	key := testKeyPair(t)
	jwks := testJWKS(&key.PublicKey)

	claims := authmw.Claims{
		RegisteredClaims: jwt.RegisteredClaims{Issuer: testIssuer},
	}
	token := testToken(t, key, claims)

	app := fiber.New()
	app.Get("/whoami", authmw.RequireAuth(jwks.Keyfunc, testIssuer), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusUnauthorized {
		t.Fatalf("status = %d, want 401 for token without exp", resp.StatusCode)
	}
}

func TestRequireRole_Forbidden(t *testing.T) {
	key := testKeyPair(t)
	jwks := testJWKS(&key.PublicKey)

	claims := authmw.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	claims.RealmAccess.Roles = []string{"user"}
	token := testToken(t, key, claims)

	app := fiber.New()
	app.Get("/admin/whoami", authmw.RequireAuth(jwks.Keyfunc, testIssuer), authmw.RequireRole("admin"), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/admin/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusForbidden {
		t.Fatalf("status = %d, want 403", resp.StatusCode)
	}
}

func TestRequireRole_Allowed(t *testing.T) {
	key := testKeyPair(t)
	jwks := testJWKS(&key.PublicKey)

	claims := authmw.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	claims.RealmAccess.Roles = []string{"user", "admin"}
	token := testToken(t, key, claims)

	app := fiber.New()
	app.Get("/admin/whoami", authmw.RequireAuth(jwks.Keyfunc, testIssuer), authmw.RequireRole("admin"), func(c fiber.Ctx) error {
		return c.SendString("ok")
	})

	req := httptest.NewRequest("GET", "/admin/whoami", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	if resp.StatusCode != fiber.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
}
```

This test file references `jwks.SetKeysForTest(...)`, a small test-only seam — add it as an exported method in `jwks.go` (Step 2's file) rather than reaching into the unexported `keys` field from a different package (`authmw_test` is an external test package, deliberately — it exercises the real public API, catching accidental unexported-detail coupling). Add this to the end of `jwks.go`:

```go
// SetKeysForTest seeds the key cache directly, bypassing Refresh's HTTP
// call — used by authmw's own tests to avoid a real JWKS server.
func (j *JWKS) SetKeysForTest(keys map[string]*rsa.PublicKey) {
	j.mu.Lock()
	j.keys = keys
	j.mu.Unlock()
}
```

- [ ] **Step 4: Run the tests to verify they fail**

```bash
cd backend
go test ./internal/shared/authmw/... -v
```

Expected: FAIL to compile — `authmw.RequireAuth`/`RequireRole`/`Claims`/`ClaimsFromContext` undefined (middleware.go doesn't exist yet).

- [ ] **Step 5: Write `middleware.go`**

`backend/internal/shared/authmw/middleware.go`:

```go
package authmw

import (
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

// Claims are the token fields QuestLog cares about. PreferredUsername and
// Picture come straight from Keycloak's standard OIDC claims; RealmAccess
// is Keycloak-specific (its default "roles" client scope maps realm roles
// into the access token under this exact shape).
type Claims struct {
	jwt.RegisteredClaims
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
	Picture           string `json:"picture"`
	RealmAccess       struct {
		Roles []string `json:"roles"`
	} `json:"realm_access"`
}

// HasRole reports whether the token carries the given realm role.
func (c Claims) HasRole(role string) bool {
	for _, r := range c.RealmAccess.Roles {
		if r == role {
			return true
		}
	}
	return false
}

type contextKey string

const claimsKey contextKey = "authmw_claims"

// RequireAuth validates the request's Bearer JWT: signature via keyFunc,
// expiry (required — a token with no `exp` is rejected rather than treated
// as never-expiring), and issuer pinned to `issuer`. On success it stores
// the parsed Claims for downstream handlers/middleware via
// ClaimsFromContext.
//
// `issuer` is the browser-facing realm URL, which is what Keycloak puts in
// `iss`. It is deliberately NOT the same host the JWKS is fetched from —
// see docs/adr/0001-keycloak-docker-network-split.md.
func RequireAuth(keyFunc jwt.Keyfunc, issuer string) fiber.Handler {
	return func(c fiber.Ctx) error {
		header := c.Get("Authorization")
		token, ok := strings.CutPrefix(header, "Bearer ")
		if !ok || token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code": "unauthorized", "message": "missing bearer token",
			})
		}

		claims := &Claims{}
		parsed, err := jwt.ParseWithClaims(token, claims, keyFunc,
			jwt.WithIssuer(issuer),
			jwt.WithExpirationRequired(),
			jwt.WithValidMethods([]string{"RS256"}),
		)
		if err != nil || !parsed.Valid {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code": "unauthorized", "message": "invalid token",
			})
		}

		c.Locals(claimsKey, claims)
		return c.Next()
	}
}

// RequireRole must run after RequireAuth. It rejects requests whose token
// doesn't carry the given realm role.
func RequireRole(role string) fiber.Handler {
	return func(c fiber.Ctx) error {
		claims, ok := ClaimsFromContext(c)
		if !ok {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code": "unauthorized", "message": "missing bearer token",
			})
		}
		if !claims.HasRole(role) {
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"code": "forbidden", "message": "requires role " + role,
			})
		}
		return c.Next()
	}
}

// ClaimsFromContext returns the Claims stored by RequireAuth, if any.
func ClaimsFromContext(c fiber.Ctx) (*Claims, bool) {
	claims, ok := c.Locals(claimsKey).(*Claims)
	return claims, ok
}
```

- [ ] **Step 6: Run the tests to verify they pass**

```bash
cd backend
go test ./internal/shared/authmw/... -v
```

Expected: PASS — all seven tests green.

- [ ] **Step 7: `go vet` and `golangci-lint`**

```bash
cd backend
go vet ./internal/shared/...
go tool golangci-lint run ./internal/shared/...
```

Expected: no issues.

- [ ] **Step 8: Commit**

```bash
git add backend/internal/shared/authmw backend/go.mod backend/go.sum
git commit -m "✅ Add JWT auth middleware: JWKS validation + role enforcement, TDD'd"
```

---

### Task 6: Wire `cmd/public-api` — DB pool, JWKS, `POST /auth/sync`

**Files:**
- Modify: `backend/cmd/public-api/main.go` (full rewrite)
- Create: `backend/internal/identity/interfaces/sync_handler.go`

**Interfaces:**
- Consumes: `authmw.NewJWKS`, `authmw.RequireAuth`, `authmw.ClaimsFromContext` (Task 5); `application.NewSyncService`, `application.SyncInput` (Task 3); `infrastructure.NewPostgresProfileRepository` (Task 4); `shared.MustEnv` (Task 1).
- Produces: running `POST /auth/sync` endpoint. Task 9/10 (packages/auth, apps/web) call this at `IDENTITY_SYNC_URL`.

- [ ] **Step 1: Write the sync HTTP handler**

`backend/internal/identity/interfaces/sync_handler.go`:

```go
// Package interfaces adapts the identity context's application layer to
// HTTP (Fiber).
package interfaces

import (
	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/alfredomendoza/questlog/backend/internal/identity/application"
	"github.com/alfredomendoza/questlog/backend/internal/shared/authmw"
)

// SyncHandler exposes POST /auth/sync — called by the Next.js apps right
// after a successful Keycloak login. It trusts only the validated JWT
// claims (via authmw.RequireAuth, mounted ahead of this handler), never a
// request body, so identity fields can't be spoofed by the caller.
type SyncHandler struct {
	sync *application.SyncService
}

func NewSyncHandler(sync *application.SyncService) *SyncHandler {
	return &SyncHandler{sync: sync}
}

func (h *SyncHandler) Handle(c fiber.Ctx) error {
	claims, ok := authmw.ClaimsFromContext(c)
	if !ok {
		return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
			"code": "unauthorized", "message": "missing bearer token",
		})
	}

	keycloakID, err := uuid.Parse(claims.Subject)
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
			"code": "invalid_token", "message": "token subject is not a uuid",
		})
	}

	profile, err := h.sync.EnsureProfile(c.Context(), application.SyncInput{
		KeycloakID: keycloakID,
		Username:   claims.PreferredUsername,
		AvatarURL:  claims.Picture,
	})
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code": "sync_failed", "message": err.Error(),
		})
	}

	return c.JSON(fiber.Map{
		"id":        profile.ID,
		"username":  profile.Username,
		"avatarUrl": profile.AvatarURL,
		"bio":       profile.Bio,
	})
}
```

- [ ] **Step 2: Rewrite `cmd/public-api/main.go`**

```go
// Command public-api serves the public-facing QuestLog API (web app, :8080).
// It registers only public-context handlers; admin-only routes live in
// cmd/admin-api. Both share the same internal/ contexts.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5/pgxpool"

	identityapp "github.com/alfredomendoza/questlog/backend/internal/identity/application"
	identityinfra "github.com/alfredomendoza/questlog/backend/internal/identity/infrastructure"
	identityiface "github.com/alfredomendoza/questlog/backend/internal/identity/interfaces"
	"github.com/alfredomendoza/questlog/backend/internal/shared"
	"github.com/alfredomendoza/questlog/backend/internal/shared/authmw"
)

func main() {
	ctx := context.Background()

	pool, err := pgxpool.New(ctx, shared.MustEnv("DATABASE_URL"))
	if err != nil {
		log.Fatalf("public-api: connect postgres: %v", err)
	}
	defer pool.Close()

	jwks := authmw.NewJWKS(shared.MustEnv("KEYCLOAK_JWKS_URL"))
	if err := jwks.Refresh(ctx); err != nil {
		log.Fatalf("public-api: initial jwks fetch: %v", err)
	}
	jwks.StartBackgroundRefresh(10 * time.Minute)

	// Pinned issuer — the browser-facing realm URL Keycloak stamps into
	// `iss`, which is NOT the host we fetch JWKS from. See ADR-0001.
	issuer := shared.MustEnv("KEYCLOAK_ISSUER")

	profiles := identityinfra.NewPostgresProfileRepository(pool)
	syncHandler := identityiface.NewSyncHandler(identityapp.NewSyncService(profiles))

	app := fiber.New()

	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(shared.OK())
	})

	app.Post("/auth/sync", authmw.RequireAuth(jwks.Keyfunc, issuer), syncHandler.Handle)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(app.Listen(":" + port))
}
```

- [ ] **Step 3: Build to verify it compiles**

```bash
cd backend
go build ./...
```

Expected: no errors.

- [ ] **Step 4: Run it against the real dockerized stack and smoke-test**

```bash
docker compose -f ../deploy/docker-compose.yml up -d postgres keycloak
cd backend
DATABASE_URL=postgres://questlog:questlog@localhost:5432/questlog?sslmode=disable \
  go run ./cmd/migrate
PORT=18080 \
DATABASE_URL=postgres://questlog:questlog@localhost:5432/questlog?sslmode=disable \
KEYCLOAK_JWKS_URL=http://localhost:8082/realms/master/protocol/openid-connect/certs \
KEYCLOAK_ISSUER=http://localhost:8082/realms/master \
  go run ./cmd/public-api &
sleep 2
curl -s localhost:18080/healthz
curl -i localhost:18080/auth/sync -X POST
kill %1
```

Expected: `{"status":"ok"}` from `/healthz`, and `401 {"code":"unauthorized",...}` from the unauthenticated `POST /auth/sync` (proves the middleware is actually mounted — the `questlog` realm doesn't exist yet until Task 8, so `master`'s JWKS is used here purely to prove startup/wiring, not full auth).

- [ ] **Step 5: Commit**

```bash
git add backend/cmd/public-api backend/internal/identity/interfaces
git commit -m "✨ Wire public-api: Postgres pool, JWKS, POST /auth/sync"
```

---

### Task 7: Wire `cmd/admin-api` — JWKS, `GET /admin/whoami`

**Files:**
- Modify: `backend/cmd/admin-api/main.go` (full rewrite)

**Interfaces:**
- Consumes: `authmw.NewJWKS`, `authmw.RequireAuth`, `authmw.RequireRole`, `authmw.ClaimsFromContext` (Task 5); `shared.MustEnv` (Task 1).
- Produces: `GET /admin/whoami` — proves role enforcement end-to-end; Task 12's verification checklist calls this directly.

- [ ] **Step 1: Rewrite `cmd/admin-api/main.go`**

```go
// Command admin-api serves the QuestLog moderation/admin API (admin app, :8081).
// Every route under /admin requires the Keycloak "admin" role; handlers
// are composed from each context's admin service, per docs/specs/05-admin.md
// — there is no admin god-context.
package main

import (
	"context"
	"log"
	"os"
	"time"

	"github.com/gofiber/fiber/v3"

	"github.com/alfredomendoza/questlog/backend/internal/shared"
	"github.com/alfredomendoza/questlog/backend/internal/shared/authmw"
)

func main() {
	ctx := context.Background()

	jwks := authmw.NewJWKS(shared.MustEnv("KEYCLOAK_JWKS_URL"))
	if err := jwks.Refresh(ctx); err != nil {
		log.Fatalf("admin-api: initial jwks fetch: %v", err)
	}
	jwks.StartBackgroundRefresh(10 * time.Minute)

	// Pinned issuer — the browser-facing realm URL Keycloak stamps into
	// `iss`, which is NOT the host we fetch JWKS from. See ADR-0001.
	issuer := shared.MustEnv("KEYCLOAK_ISSUER")

	app := fiber.New()

	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(shared.OK())
	})

	admin := app.Group("/admin", authmw.RequireAuth(jwks.Keyfunc, issuer), authmw.RequireRole("admin"))
	admin.Get("/whoami", func(c fiber.Ctx) error {
		claims, _ := authmw.ClaimsFromContext(c)
		return c.JSON(fiber.Map{
			"username": claims.PreferredUsername,
			"roles":    claims.RealmAccess.Roles,
		})
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	log.Fatal(app.Listen(":" + port))
}
```

- [ ] **Step 2: Build to verify it compiles**

```bash
cd backend
go build ./...
```

Expected: no errors.

- [ ] **Step 3: Smoke-test `/healthz` stays open and `/admin/whoami` is gated**

```bash
PORT=18081 KEYCLOAK_JWKS_URL=http://localhost:8082/realms/master/protocol/openid-connect/certs \
  KEYCLOAK_ISSUER=http://localhost:8082/realms/master \
  go run ./cmd/admin-api &
sleep 2
curl -s localhost:18081/healthz
curl -i localhost:18081/admin/whoami
kill %1
```

Expected: `{"status":"ok"}` from `/healthz` (no auth needed — it's registered outside the `/admin` group), `401` from `/admin/whoami` with no token.

- [ ] **Step 4: Full backend test suite, one last time before moving to Keycloak/Docker wiring**

```bash
cd backend
go build ./... && go vet ./... && go test ./... -race && gofmt -l . && go tool golangci-lint run ./...
```

Expected: all clean (the two DB-dependent tests from Task 4 will `SKIP` here since no `DATABASE_URL` is set in this shell — that's correct).

- [ ] **Step 5: Commit**

```bash
git add backend/cmd/admin-api
git commit -m "✨ Wire admin-api: JWKS, /admin/whoami role-gated route"
```

---

### Task 8: Keycloak realm + Docker Compose wiring + ADR-0001

**Files:**
- Create: `deploy/keycloak/questlog-realm.json`
- Modify: `deploy/docker-compose.yml` (full rewrite)
- Create: `docs/adr/0001-keycloak-docker-network-split.md`

**Interfaces:**
- Produces: a running `questlog` Keycloak realm with clients `questlog-web`/`questlog-admin`, roles `user`/`admin`, seed users `quest_user`/`quest_admin`; a `migrate` one-shot compose service; `KEYCLOAK_JWKS_URL`, `AUTH_KEYCLOAK_*`, `IDENTITY_SYNC_URL` env vars threaded through `public-api`/`admin-api`/`web`/`admin`. Tasks 9–11 depend on these exact env var names and client IDs.

- [ ] **Step 1: Write the realm export**

`deploy/keycloak/questlog-realm.json`:

```json
{
  "realm": "questlog",
  "enabled": true,
  "registrationAllowed": true,
  "registrationEmailAsUsername": false,
  "loginWithEmailAllowed": true,
  "duplicateEmailsAllowed": false,
  "resetPasswordAllowed": true,
  "editUsernameAllowed": false,
  "sslRequired": "none",
  "roles": {
    "realm": [
      { "name": "user", "description": "Standard QuestLog member" },
      { "name": "admin", "description": "Moderation and catalog curation access" }
    ]
  },
  "defaultRole": {
    "name": "default-roles-questlog",
    "composite": true,
    "composites": {
      "realm": ["user", "offline_access", "uma_authorization"]
    }
  },
  "clients": [
    {
      "clientId": "questlog-web",
      "enabled": true,
      "publicClient": false,
      "protocol": "openid-connect",
      "clientAuthenticatorType": "client-secret",
      "secret": "questlog-web-dev-secret",
      "redirectUris": ["http://localhost:3000/api/auth/callback/keycloak"],
      "webOrigins": ["http://localhost:3000"],
      "standardFlowEnabled": true,
      "directAccessGrantsEnabled": true
    },
    {
      "clientId": "questlog-admin",
      "enabled": true,
      "publicClient": false,
      "protocol": "openid-connect",
      "clientAuthenticatorType": "client-secret",
      "secret": "questlog-admin-dev-secret",
      "redirectUris": ["http://localhost:3001/api/auth/callback/keycloak"],
      "webOrigins": ["http://localhost:3001"],
      "standardFlowEnabled": true,
      "directAccessGrantsEnabled": true
    }
  ],
  "users": [
    {
      "username": "quest_user",
      "enabled": true,
      "emailVerified": true,
      "email": "quest_user@example.com",
      "firstName": "Quest",
      "lastName": "User",
      "credentials": [{ "type": "password", "value": "questpass1", "temporary": false }],
      "realmRoles": ["user"]
    },
    {
      "username": "quest_admin",
      "enabled": true,
      "emailVerified": true,
      "email": "quest_admin@example.com",
      "firstName": "Quest",
      "lastName": "Admin",
      "credentials": [{ "type": "password", "value": "adminpass1", "temporary": false }],
      "realmRoles": ["user", "admin"]
    }
  ]
}
```

`directAccessGrantsEnabled: true` is a **local-dev-only convenience** so Task 12's verification checklist can fetch real tokens with `curl` instead of driving a browser — reconsider before any non-local deployment.

- [ ] **Step 2: Rewrite `deploy/docker-compose.yml`**

```yaml
name: questlog

services:
  postgres:
    image: postgres:17-alpine
    environment:
      POSTGRES_USER: questlog
      POSTGRES_PASSWORD: questlog
      POSTGRES_DB: questlog
    ports: ["5432:5432"]
    volumes:
      - questlog-postgres:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U questlog"]
      interval: 5s
      timeout: 3s
      retries: 10

  keycloak:
    image: quay.io/keycloak/keycloak:26.0
    command: start-dev --import-realm
    environment:
      KEYCLOAK_ADMIN: admin
      KEYCLOAK_ADMIN_PASSWORD: admin
      # Fixes every issued token's `iss` claim to this value regardless of
      # whether the request came from the browser (localhost:8082) or a
      # container on the compose network (keycloak:8080) — see
      # docs/adr/0001-keycloak-docker-network-split.md.
      KC_HOSTNAME: http://localhost:8082
    ports: ["8082:8080"]
    volumes:
      - ./keycloak:/opt/keycloak/data/import
    # No volume for Keycloak's own data dir: state (including the realm
    # import and any signups made during manual testing) is ephemeral and
    # re-imported fresh on every `docker compose up`.

  migrate:
    build:
      context: ..
      dockerfile: deploy/Dockerfile.go
      args: { CMD_PATH: cmd/migrate }
    environment:
      DATABASE_URL: postgres://questlog:questlog@postgres:5432/questlog?sslmode=disable
    depends_on:
      postgres:
        condition: service_healthy

  public-api:
    build:
      context: ..
      dockerfile: deploy/Dockerfile.go
      args: { CMD_PATH: cmd/public-api }
    environment:
      PORT: "8080"
      DATABASE_URL: postgres://questlog:questlog@postgres:5432/questlog?sslmode=disable
      # Keys are fetched over the compose network...
      KEYCLOAK_JWKS_URL: http://keycloak:8080/realms/questlog/protocol/openid-connect/certs
      # ...but `iss` is pinned to the browser-facing URL, because that's
      # what KC_HOSTNAME makes Keycloak stamp into every token. These two
      # differing hosts are expected, not a typo — see ADR-0001.
      KEYCLOAK_ISSUER: http://localhost:8082/realms/questlog
    ports: ["8080:8080"]
    depends_on:
      postgres:
        condition: service_healthy
      migrate:
        condition: service_completed_successfully
      keycloak:
        condition: service_started

  admin-api:
    build:
      context: ..
      dockerfile: deploy/Dockerfile.go
      args: { CMD_PATH: cmd/admin-api }
    environment:
      PORT: "8081"
      DATABASE_URL: postgres://questlog:questlog@postgres:5432/questlog?sslmode=disable
      # Keys are fetched over the compose network...
      KEYCLOAK_JWKS_URL: http://keycloak:8080/realms/questlog/protocol/openid-connect/certs
      # ...but `iss` is pinned to the browser-facing URL, because that's
      # what KC_HOSTNAME makes Keycloak stamp into every token. These two
      # differing hosts are expected, not a typo — see ADR-0001.
      KEYCLOAK_ISSUER: http://localhost:8082/realms/questlog
    ports: ["8081:8081"]
    depends_on:
      postgres:
        condition: service_healthy
      migrate:
        condition: service_completed_successfully
      keycloak:
        condition: service_started

  web:
    build:
      context: ..
      dockerfile: deploy/Dockerfile.node
      args: { APP: web, PORT: "3000" }
    environment:
      AUTH_SECRET: dev-only-insecure-secret-change-me
      AUTH_KEYCLOAK_ID: questlog-web
      AUTH_KEYCLOAK_SECRET: questlog-web-dev-secret
      AUTH_KEYCLOAK_ISSUER: http://localhost:8082/realms/questlog
      AUTH_KEYCLOAK_INTERNAL_ISSUER: http://keycloak:8080/realms/questlog
      IDENTITY_SYNC_URL: http://public-api:8080/auth/sync
    ports: ["3000:3000"]
    depends_on: [public-api, keycloak]

  admin:
    build:
      context: ..
      dockerfile: deploy/Dockerfile.node
      args: { APP: admin, PORT: "3001" }
    environment:
      AUTH_SECRET: dev-only-insecure-secret-change-me
      AUTH_KEYCLOAK_ID: questlog-admin
      AUTH_KEYCLOAK_SECRET: questlog-admin-dev-secret
      AUTH_KEYCLOAK_ISSUER: http://localhost:8082/realms/questlog
      AUTH_KEYCLOAK_INTERNAL_ISSUER: http://keycloak:8080/realms/questlog
      IDENTITY_SYNC_URL: http://public-api:8080/auth/sync
    ports: ["3001:3001"]
    depends_on: [admin-api, public-api, keycloak]

volumes:
  questlog-postgres:
```

- [ ] **Step 3: Write ADR-0001**

`docs/adr/0001-keycloak-docker-network-split.md`:

```markdown
# ADR-0001: Split public vs. internal Keycloak URLs for NextAuth

**Status:** accepted
**Context:** Phase 3 — Auth & identity

## Context

Keycloak runs as its own `docker-compose` service (`keycloak`, container port
8080, published to the host as `8082`). The Next.js apps run as separate
containers (`web`, `admin`) in the same compose network. NextAuth's Keycloak
provider needs four Keycloak endpoints: `authorization` (the browser is
redirected here), and `token` / `userinfo` / `jwks_endpoint` (called directly
by the Next.js server, never by the browser).

A single "issuer" URL can't serve both purposes inside Docker Compose:

- The **browser** can only reach Keycloak via the host-published address
  (`http://localhost:8082`) — it has no visibility into the compose network.
- The **Next.js server**, running inside its own container, cannot reach
  `localhost:8082` — inside a container, `localhost` refers to that
  container itself, not the `keycloak` one. It needs the compose network's
  service DNS name instead (`http://keycloak:8080`).

Using only the public URL breaks server-to-server calls (connection
refused). Using only the internal URL breaks the browser redirect (the
browser can't resolve `keycloak`).

## Decision

Configure two env vars and pass explicit endpoint overrides to the NextAuth
Keycloak provider instead of relying on OIDC discovery from a single issuer:

- `AUTH_KEYCLOAK_ISSUER` (public) → builds the `authorization` endpoint.
- `AUTH_KEYCLOAK_INTERNAL_ISSUER` (internal; defaults to the public one when
  unset, so plain `pnpm dev` outside Docker still works with one URL) →
  builds `token`, `userinfo`, `jwks_endpoint`.

Keycloak's `KC_HOSTNAME` is fixed to the public URL
(`http://localhost:8082`) so every issued token's `iss` claim is the same
regardless of which network path was used to request it — otherwise a
token minted via the internal token endpoint would carry
`iss: http://keycloak:8080/realms/questlog`, which doesn't match the
`issuer` NextAuth validates against, and login would fail with an
issuer-mismatch error.

The same split reaches the Go backend, and this is the part most likely to
look like a typo during review. `internal/shared/authmw` validates
signature, expiry **and** issuer, but its two Keycloak-related env vars
deliberately name different hosts:

- `KEYCLOAK_JWKS_URL=http://keycloak:8080/realms/questlog/protocol/openid-connect/certs`
  — the backend *fetches keys* over the compose network.
- `KEYCLOAK_ISSUER=http://localhost:8082/realms/questlog` — the backend
  *validates `iss`* against the public URL, because `KC_HOSTNAME` makes
  Keycloak stamp that value into every token no matter which network path
  minted it.

Pinning `iss` to the JWKS host instead would reject every real token.

## Consequences

- Two Keycloak-related env vars instead of one on the Next.js side, and two
  more (`KEYCLOAK_JWKS_URL`, `KEYCLOAK_ISSUER`) on the Go side — documented in
  `apps/{web,admin}/.env.example` and `deploy/docker-compose.yml`.
- If QuestLog ever deploys behind real domains, `AUTH_KEYCLOAK_INTERNAL_ISSUER`
  can point at an internal service mesh address while
  `AUTH_KEYCLOAK_ISSUER` stays the public one — same mechanism, no rework.
  `KEYCLOAK_ISSUER` follows the public URL wherever it goes.
- The backend trusts exactly one issuer. Accepting tokens from more than one
  Keycloak instance would mean turning `KEYCLOAK_ISSUER` into a list — a
  deliberate change, not an accident.
```

- [ ] **Step 4: Bring up the full stack and confirm the realm imports**

```bash
docker compose -f deploy/docker-compose.yml up --build -d
docker compose -f deploy/docker-compose.yml logs keycloak | grep -i "questlog"
```

Expected: a log line confirming the `questlog` realm was imported (Keycloak logs realm import events at startup).

- [ ] **Step 5: Confirm `migrate` ran and `public-api`/`admin-api` started after it**

```bash
docker compose -f deploy/docker-compose.yml ps
docker compose -f deploy/docker-compose.yml logs migrate
docker compose -f deploy/docker-compose.yml logs public-api | tail -5
```

Expected: `migrate` shows `Exited (0)`; its log shows `migrate: up to date`; `public-api` log shows Fiber's startup banner with no fatal errors.

- [ ] **Step 6: Tear down before the next task (apps aren't wired yet — no point leaving it running)**

```bash
docker compose -f deploy/docker-compose.yml down
```

- [ ] **Step 7: Commit**

```bash
git add deploy docs/adr
git commit -m "✨ Add Keycloak realm, wire Docker Compose, document network split (ADR-0001)"
```

---

### Task 9: `packages/auth` — real NextAuth config

**Files:**
- Modify: `packages/auth/package.json` (rewrite)
- Modify: `packages/auth/src/index.ts` (rewrite)
- Create: `packages/auth/src/config.ts`
- Create: `packages/auth/src/types.d.ts`
- Modify: `packages/ui/src/tokens.css` (add `[data-admin]` accent override)

**Interfaces:**
- Consumes: `AUTH_KEYCLOAK_ISSUER`, `AUTH_KEYCLOAK_INTERNAL_ISSUER`, `AUTH_KEYCLOAK_ID`, `AUTH_KEYCLOAK_SECRET`, `IDENTITY_SYNC_URL`, `AUTH_SECRET` env vars (Task 8).
- Produces: `authConfig: NextAuthConfig` (default export of `@questlog/auth`... no — named export `authConfig`), `KEYCLOAK_REALM: string`. Tasks 10 and 11 both import `{ authConfig }` from `@questlog/auth`.

- [ ] **Step 1: Rewrite `package.json`**

`packages/auth/package.json`:

```json
{
  "name": "@questlog/auth",
  "version": "0.0.0",
  "private": true,
  "type": "module",
  "exports": {
    ".": "./src/index.ts"
  },
  "scripts": {
    "typecheck": "tsc --noEmit"
  },
  "dependencies": {
    "next-auth": "^5.0.0"
  },
  "devDependencies": {
    "@questlog/config": "workspace:*",
    "@types/node": "^22.0.0",
    "typescript": "^5.7.0"
  }
}
```

- [ ] **Step 2: Write `config.ts`**

`packages/auth/src/config.ts`:

```typescript
import type { NextAuthConfig } from "next-auth";
import Keycloak from "next-auth/providers/keycloak";

const PUBLIC_ISSUER = requireEnv("AUTH_KEYCLOAK_ISSUER");
const INTERNAL_ISSUER =
  process.env.AUTH_KEYCLOAK_INTERNAL_ISSUER ?? PUBLIC_ISSUER;
const IDENTITY_SYNC_URL = requireEnv("IDENTITY_SYNC_URL");

/**
 * Shared NextAuth config for both apps/web and apps/admin. Each app sets
 * its own AUTH_KEYCLOAK_ID / AUTH_KEYCLOAK_SECRET env vars (a different
 * Keycloak client per app) — next-auth's Keycloak provider infers those
 * two from env automatically, so this config only overrides the four
 * endpoints explicitly.
 *
 * Why two issuer URLs: see docs/adr/0001-keycloak-docker-network-split.md.
 */
export const authConfig: NextAuthConfig = {
  providers: [
    Keycloak({
      issuer: PUBLIC_ISSUER,
      authorization: `${PUBLIC_ISSUER}/protocol/openid-connect/auth`,
      token: `${INTERNAL_ISSUER}/protocol/openid-connect/token`,
      userinfo: `${INTERNAL_ISSUER}/protocol/openid-connect/userinfo`,
      jwks_endpoint: `${INTERNAL_ISSUER}/protocol/openid-connect/certs`,
    }),
  ],
  session: { strategy: "jwt" },
  callbacks: {
    async jwt({ token, account }) {
      if (account?.access_token) {
        token.accessToken = account.access_token;
        token.roles = decodeRealmRoles(account.access_token);
        await syncProfile(account.access_token);
      }
      return token;
    },
    async session({ session, token }) {
      session.accessToken = token.accessToken as string | undefined;
      session.roles = (token.roles as string[] | undefined) ?? [];
      return session;
    },
  },
};

/** Decodes the (already-trusted, freshly-issued) access token's realm roles. */
function decodeRealmRoles(accessToken: string): string[] {
  const payload = accessToken.split(".")[1];
  if (!payload) return [];
  const json = JSON.parse(
    Buffer.from(payload, "base64url").toString("utf8"),
  ) as { realm_access?: { roles?: string[] } };
  return json.realm_access?.roles ?? [];
}

/** Syncs the local identity profile — see backend/internal/identity. */
async function syncProfile(accessToken: string): Promise<void> {
  try {
    const res = await fetch(IDENTITY_SYNC_URL, {
      method: "POST",
      headers: { Authorization: `Bearer ${accessToken}` },
    });
    if (!res.ok) {
      console.error(`identity sync failed: ${res.status} ${res.statusText}`);
    }
  } catch (err) {
    console.error("identity sync failed:", err);
  }
}

function requireEnv(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`missing required env var ${name}`);
  return value;
}
```

- [ ] **Step 3: Write `types.d.ts`**

`packages/auth/src/types.d.ts`:

```typescript
import type { DefaultSession } from "next-auth";

declare module "next-auth" {
  interface Session extends DefaultSession {
    accessToken?: string;
    roles: string[];
  }
}

declare module "next-auth/jwt" {
  interface JWT {
    accessToken?: string;
    roles?: string[];
  }
}
```

- [ ] **Step 4: Rewrite `index.ts`**

`packages/auth/src/index.ts`:

```typescript
export { authConfig } from "./config";

export const KEYCLOAK_REALM = "questlog";
```

- [ ] **Step 5: Add the `[data-admin]` accent override to the design system**

Add to the end of `packages/ui/src/tokens.css` (after the existing `:root[data-accent="blue"]` block):

```css
/* Admin app: fixed amber accent, no user-facing toggle — matches
   docs/design/mockups/v-admin.html's dedicated moderation palette. */
:root[data-admin="true"] {
  --ql-accent: #ffd166;
  --ql-accent-deep: #d4a92f;
  --ql-accent-ink: #231c05;
}
```

- [ ] **Step 6: Install dependencies and typecheck**

```bash
pnpm install
pnpm --filter @questlog/auth typecheck
```

Expected: `pnpm install` resolves `next-auth`; typecheck passes. If `next-auth@^5.0.0` fails to resolve (v5 still in beta at install time), edit `packages/auth/package.json`'s `next-auth` version to the current beta tag shown in the `pnpm install` error output and retry — every other line in this task is unaffected by which 5.x tag resolves.

- [ ] **Step 7: Commit**

```bash
git add packages/auth packages/ui/src/tokens.css pnpm-lock.yaml
git commit -m "✨ Wire packages/auth: real NextAuth + Keycloak config"
```

---

### Task 10: `apps/web` — auth wiring, `/cuenta` page

**Files:**
- Create: `apps/web/src/auth.ts`
- Create: `apps/web/src/app/api/auth/[...nextauth]/route.ts`
- Create: `apps/web/src/app/[locale]/cuenta/page.tsx`
- Modify: `apps/web/src/app/[locale]/page.tsx` (add a link to `cuenta`)
- Modify: `apps/web/next.config.ts` (add `@questlog/auth` to `transpilePackages`)
- Modify: `apps/web/package.json` (add `next-auth` dependency)
- Modify: `apps/web/messages/es.json`, `apps/web/messages/en.json`
- Create: `apps/web/.env.example`

**Interfaces:**
- Consumes: `authConfig` (Task 9).
- Produces: `/es/cuenta` and `/en/cuenta` pages; Task 12's checklist drives these directly.

- [ ] **Step 1: Add `next-auth` to `package.json` and `next.config.ts`**

In `apps/web/package.json`, add to `dependencies`:

```json
"next-auth": "^5.0.0",
```

Rewrite `apps/web/next.config.ts`:

```typescript
import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";

const withNextIntl = createNextIntlPlugin("./src/i18n/request.ts");

const config: NextConfig = {
  transpilePackages: ["@questlog/ui", "@questlog/auth"],
};

export default withNextIntl(config);
```

- [ ] **Step 2: Write `src/auth.ts`**

`apps/web/src/auth.ts`:

```typescript
import NextAuth from "next-auth";
import { authConfig } from "@questlog/auth";

export const { handlers, auth, signIn, signOut } = NextAuth(authConfig);
```

- [ ] **Step 3: Write the route handler**

`apps/web/src/app/api/auth/[...nextauth]/route.ts`:

```typescript
import { handlers } from "@/auth";

export const { GET, POST } = handlers;
```

- [ ] **Step 4: Add message keys**

Rewrite `apps/web/messages/es.json`:

```json
{
  "home": {
    "title": "QuestLog",
    "tagline": "Tu diario de aventuras para todo lo que ves y juegas.",
    "account": "Mi cuenta"
  },
  "cuenta": {
    "signedOut": "Aún no iniciaste sesión.",
    "signIn": "Iniciar sesión",
    "signedInAs": "Sesión iniciada como {name}",
    "roles": "Roles: {roles}",
    "noRoles": "ninguno",
    "signOut": "Cerrar sesión"
  }
}
```

Rewrite `apps/web/messages/en.json`:

```json
{
  "home": {
    "title": "QuestLog",
    "tagline": "Your quest log for everything you watch and play.",
    "account": "My account"
  },
  "cuenta": {
    "signedOut": "You're not signed in yet.",
    "signIn": "Sign in",
    "signedInAs": "Signed in as {name}",
    "roles": "Roles: {roles}",
    "noRoles": "none",
    "signOut": "Sign out"
  }
}
```

- [ ] **Step 5: Write the `/cuenta` page**

`apps/web/src/app/[locale]/cuenta/page.tsx`:

```tsx
import { getTranslations } from "next-intl/server";
import { auth, signIn, signOut } from "@/auth";
import { Panel } from "@questlog/ui";

export default async function CuentaPage() {
  const session = await auth();
  const t = await getTranslations("cuenta");

  if (!session?.user) {
    return (
      <main className="mx-auto max-w-2xl p-10">
        <Panel>
          <p style={{ marginBottom: 16 }}>{t("signedOut")}</p>
          <form
            action={async () => {
              "use server";
              await signIn("keycloak");
            }}
          >
            <button
              type="submit"
              style={{
                background: "var(--ql-accent)",
                color: "var(--ql-accent-ink)",
                padding: "10px 18px",
                border: "none",
              }}
            >
              {t("signIn")}
            </button>
          </form>
        </Panel>
      </main>
    );
  }

  return (
    <main className="mx-auto max-w-2xl p-10">
      <Panel variant="signature">
        <p>
          {t("signedInAs", {
            name: session.user.name ?? session.user.email ?? "",
          })}
        </p>
        <p style={{ color: "var(--ql-muted)", marginTop: 4 }}>
          {t("roles", { roles: session.roles.join(", ") || t("noRoles") })}
        </p>
        <form
          action={async () => {
            "use server";
            await signOut();
          }}
          style={{ marginTop: 16 }}
        >
          <button
            type="submit"
            style={{
              background: "transparent",
              color: "var(--ql-text)",
              border: "1px solid var(--ql-frame-dim)",
              padding: "10px 18px",
            }}
          >
            {t("signOut")}
          </button>
        </form>
      </Panel>
    </main>
  );
}
```

- [ ] **Step 6: Link to it from the home page**

Modify `apps/web/src/app/[locale]/page.tsx` — add an `account` link below the tagline:

```tsx
import { getTranslations } from "next-intl/server";
import { Panel } from "@questlog/ui";

export default async function Home() {
  const t = await getTranslations("home");

  return (
    <main className="mx-auto max-w-2xl p-10">
      <Panel variant="signature">
        <h1 className="text-2xl font-bold">{t("title")}</h1>
        <p style={{ color: "var(--ql-muted)" }}>{t("tagline")}</p>
        <a
          href="cuenta"
          style={{ color: "var(--ql-accent)", display: "inline-block", marginTop: 12 }}
        >
          {t("account")}
        </a>
      </Panel>
    </main>
  );
}
```

- [ ] **Step 7: Add `.env.example`**

`apps/web/.env.example`:

```
# Copy to .env.local for `pnpm dev` outside Docker.
# Requires Keycloak running, e.g.:
#   docker compose -f deploy/docker-compose.yml up -d keycloak

AUTH_SECRET=dev-only-insecure-secret-change-me
AUTH_KEYCLOAK_ID=questlog-web
AUTH_KEYCLOAK_SECRET=questlog-web-dev-secret
AUTH_KEYCLOAK_ISSUER=http://localhost:8082/realms/questlog
# Not needed outside Docker — the web server can reach Keycloak the same
# way the browser does. Only set this when running via docker-compose,
# where AUTH_KEYCLOAK_ISSUER isn't reachable from inside the container.
# AUTH_KEYCLOAK_INTERNAL_ISSUER=http://keycloak:8080/realms/questlog

IDENTITY_SYNC_URL=http://localhost:8080/auth/sync
```

- [ ] **Step 8: Install, typecheck, build**

```bash
pnpm install
pnpm --filter web typecheck
pnpm --filter web build
```

Expected: all green. (`build` will succeed even without live env vars at build time since Next.js only needs them at request time for this app; if it fails complaining about missing env vars, that confirms they're read at module-eval time rather than request time — if that happens, wrap the `requireEnv` calls in `config.ts` so they run lazily inside `authConfig`'s functions instead of at module top level. Not expected to be needed here since Next.js's build step doesn't execute `authConfig`'s callbacks, only imports the module — top-level `requireEnv` calls WILL run at build time though, so make sure `.env` or shell env has these three vars set before running `build` locally, matching what Task 8's compose file already provides in the containerized build.)

- [ ] **Step 9: Commit**

```bash
git add apps/web
git commit -m "✨ Wire apps/web auth: NextAuth route handler, /cuenta page"
```

---

### Task 11: `apps/admin` — auth wiring, role-gated layout

**Files:**
- Create: `apps/admin/src/auth.ts`
- Create: `apps/admin/src/app/api/auth/[...nextauth]/route.ts`
- Modify: `apps/admin/src/app/[locale]/layout.tsx` (rewrite — role gate)
- Modify: `apps/admin/next.config.ts` (add `@questlog/auth` to `transpilePackages`)
- Modify: `apps/admin/package.json` (add `next-auth` dependency)
- Modify: `apps/admin/messages/es.json`, `apps/admin/messages/en.json`
- Create: `apps/admin/.env.example`

**Interfaces:**
- Consumes: `authConfig` (Task 9).
- Produces: the admin portal, reachable only for `admin`-role sessions; Task 12's checklist drives this directly.

- [ ] **Step 1: Add `next-auth` to `package.json` and `next.config.ts`**

In `apps/admin/package.json`, add to `dependencies`:

```json
"next-auth": "^5.0.0",
```

Rewrite `apps/admin/next.config.ts`:

```typescript
import type { NextConfig } from "next";
import createNextIntlPlugin from "next-intl/plugin";

const withNextIntl = createNextIntlPlugin("./src/i18n/request.ts");

const config: NextConfig = {
  transpilePackages: ["@questlog/ui", "@questlog/auth"],
};

export default withNextIntl(config);
```

- [ ] **Step 2: Write `src/auth.ts` and the route handler**

`apps/admin/src/auth.ts`:

```typescript
import NextAuth from "next-auth";
import { authConfig } from "@questlog/auth";

export const { handlers, auth, signIn, signOut } = NextAuth(authConfig);
```

`apps/admin/src/app/api/auth/[...nextauth]/route.ts`:

```typescript
import { handlers } from "@/auth";

export const { GET, POST } = handlers;
```

- [ ] **Step 3: Add message keys**

Rewrite `apps/admin/messages/es.json`:

```json
{
  "home": {
    "title": "QuestLog Admin",
    "tagline": "Moderación, usuarios y curación de catálogo."
  },
  "admin": {
    "signInTitle": "Acceso administrativo",
    "signInMessage": "Inicia sesión con una cuenta con rol de administrador.",
    "signIn": "Iniciar sesión",
    "deniedTitle": "Acceso denegado",
    "deniedMessage": "Tu cuenta no tiene el rol de administrador.",
    "signOut": "Cerrar sesión"
  }
}
```

Rewrite `apps/admin/messages/en.json`:

```json
{
  "home": {
    "title": "QuestLog Admin",
    "tagline": "Moderation, users and catalog curation."
  },
  "admin": {
    "signInTitle": "Admin access",
    "signInMessage": "Sign in with an account that has the admin role.",
    "signIn": "Sign in",
    "deniedTitle": "Access denied",
    "deniedMessage": "Your account doesn't have the admin role.",
    "signOut": "Sign out"
  }
}
```

- [ ] **Step 4: Rewrite the layout with role gating**

`apps/admin/src/app/[locale]/layout.tsx`:

```tsx
import type { ReactNode } from "react";
import { NextIntlClientProvider, hasLocale } from "next-intl";
import { getTranslations } from "next-intl/server";
import { notFound } from "next/navigation";
import { routing } from "@/i18n/routing";
import { auth, signIn, signOut } from "@/auth";
import "@questlog/ui/tokens.css";
import "../globals.css";

export const metadata = {
  title: "QuestLog Admin",
  description: "Moderación, usuarios y curación de catálogo.",
};

export default async function LocaleLayout({
  children,
  params,
}: {
  children: ReactNode;
  params: Promise<{ locale: string }>;
}) {
  const { locale } = await params;
  if (!hasLocale(routing.locales, locale)) notFound();

  const session = await auth();
  const t = await getTranslations("admin");
  const isAdmin = session?.roles?.includes("admin") ?? false;

  return (
    <html lang={locale} data-admin="true">
      <body>
        <NextIntlClientProvider>
          {!session?.user ? (
            <AccessGate
              title={t("signInTitle")}
              message={t("signInMessage")}
              action={{ label: t("signIn"), kind: "signIn" }}
            />
          ) : !isAdmin ? (
            <AccessGate
              title={t("deniedTitle")}
              message={t("deniedMessage")}
              action={{ label: t("signOut"), kind: "signOut" }}
            />
          ) : (
            children
          )}
        </NextIntlClientProvider>
      </body>
    </html>
  );
}

function AccessGate({
  title,
  message,
  action,
}: {
  title: string;
  message: string;
  action: { label: string; kind: "signIn" | "signOut" };
}) {
  return (
    <main
      style={{
        minHeight: "100vh",
        display: "grid",
        placeItems: "center",
        padding: 40,
      }}
    >
      <div
        style={{
          background: "var(--ql-panel)",
          border: "1px solid var(--ql-frame-dim)",
          padding: "28px 32px",
          maxWidth: 420,
          textAlign: "center",
        }}
      >
        <h1 style={{ fontSize: 20, marginBottom: 10 }}>{title}</h1>
        <p style={{ color: "var(--ql-muted)", marginBottom: 20 }}>{message}</p>
        <form
          action={async () => {
            "use server";
            if (action.kind === "signIn") {
              await signIn("keycloak");
            } else {
              await signOut();
            }
          }}
        >
          <button
            type="submit"
            style={{
              background: "var(--ql-accent)",
              color: "var(--ql-accent-ink)",
              border: "none",
              padding: "10px 20px",
            }}
          >
            {action.label}
          </button>
        </form>
      </div>
    </main>
  );
}
```

- [ ] **Step 5: Add `.env.example`**

`apps/admin/.env.example`:

```
# Copy to .env.local for `pnpm dev` outside Docker.
# Requires Keycloak running, e.g.:
#   docker compose -f deploy/docker-compose.yml up -d keycloak

AUTH_SECRET=dev-only-insecure-secret-change-me
AUTH_KEYCLOAK_ID=questlog-admin
AUTH_KEYCLOAK_SECRET=questlog-admin-dev-secret
AUTH_KEYCLOAK_ISSUER=http://localhost:8082/realms/questlog
# AUTH_KEYCLOAK_INTERNAL_ISSUER=http://keycloak:8080/realms/questlog

IDENTITY_SYNC_URL=http://localhost:8080/auth/sync
```

- [ ] **Step 6: Install, typecheck, build**

```bash
pnpm install
pnpm --filter admin typecheck
pnpm --filter admin build
```

Expected: all green (same env-var-at-build-time note as Task 10, Step 8 applies here too).

- [ ] **Step 7: Full monorepo check**

```bash
pnpm turbo lint typecheck build
```

Expected: all packages/apps green.

- [ ] **Step 8: Commit**

```bash
git add apps/admin
git commit -m "✨ Wire apps/admin auth: role-gated layout, access-denied state"
```

---

### Task 12: Verification checklist + `PLAN.md` update

**Files:**
- Create: `docs/verify-phase-3.md`
- Modify: `PLAN.md` (check off Phase 3's items)

**Interfaces:**
- Consumes: the entire running stack from Tasks 1–11.
- Produces: a documented, repeatable manual verification procedure; an accurate `PLAN.md`.

- [ ] **Step 1: Write the verification checklist**

`docs/verify-phase-3.md`:

```markdown
# Verifying Phase 3 — Auth & identity

Manual/scripted verification for this phase's acceptance criteria (PLAN.md).
Full browser E2E automation (Playwright) is Phase 9 scope — this is a
repeatable checklist to run against the real compose stack instead.

## Setup

```bash
./scripts/dev.sh
```

Wait for all services healthy: `docker compose -f deploy/docker-compose.yml ps`.

## 1. Realm imported correctly

Open http://localhost:8082/admin (admin/admin) → realm switcher → `questlog`.
Confirm: roles `user` and `admin` exist; clients `questlog-web` and
`questlog-admin` exist; users `quest_user` and `quest_admin` exist.

## 2. Signup → login → authenticated page (web)

1. Open http://localhost:3000/es/cuenta — should show a "Iniciar sesión" button.
2. Click it → redirected to Keycloak's hosted login page.
3. Click "Register" → create a new account (any username/password).
4. Redirected back to `/es/cuenta`, now showing the signed-in state with
   your username.
5. `docker compose -f deploy/docker-compose.yml logs public-api` should show
   no errors around the `/auth/sync` call for this request.

## 3. Login with a seeded user

1. Sign out from `/es/cuenta`.
2. Sign in again with `quest_user` / `questpass1`.
3. Confirm the page shows "quest_user" and role "user" (no "admin").

## 4. Admin login → admin portal

1. Open http://localhost:3001/es — should show an access-denied/sign-in gate,
   not the portal.
2. Sign in with `quest_user` / `questpass1` (a non-admin user).
3. Confirm you see "Acceso denegado" (access denied), not the portal.
4. Sign out, sign in again with `quest_admin` / `adminpass1`.
5. Confirm the portal content renders (the existing home Panel).

## 5. Go JWT middleware, directly

```bash
curl -i http://localhost:8081/admin/whoami
# expect: 401 (no bearer token)

curl -i -H "Authorization: Bearer not-a-real-token" http://localhost:8081/admin/whoami
# expect: 401 (invalid token)

# Real tokens via Keycloak's direct grant (enabled for local dev
# convenience only — see deploy/keycloak/questlog-realm.json):
ADMIN_TOKEN=$(curl -s -d "grant_type=password" \
  -d "client_id=questlog-admin" -d "client_secret=questlog-admin-dev-secret" \
  -d "username=quest_admin" -d "password=adminpass1" \
  http://localhost:8082/realms/questlog/protocol/openid-connect/token \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

curl -i -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8081/admin/whoami
# expect: 200 {"username":"quest_admin","roles":[...,"admin","user",...]}

USER_TOKEN=$(curl -s -d "grant_type=password" \
  -d "client_id=questlog-web" -d "client_secret=questlog-web-dev-secret" \
  -d "username=quest_user" -d "password=questpass1" \
  http://localhost:8082/realms/questlog/protocol/openid-connect/token \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

curl -i -H "Authorization: Bearer $USER_TOKEN" http://localhost:8081/admin/whoami
# expect: 403 (quest_user lacks the admin role)

curl -i -H "Authorization: Bearer $USER_TOKEN" -X POST http://localhost:8080/auth/sync
# expect: 200 — any authenticated user can sync, not just admins
```

## 6. Automated coverage

```bash
cd backend
go test ./internal/identity/... ./internal/shared/... -race
# includes an integration test against the real dockerized Postgres:
DATABASE_URL=postgres://questlog:questlog@localhost:5432/questlog?sslmode=disable \
  go test ./internal/identity/infrastructure/... -race
```

Expected: all green.
```

- [ ] **Step 2: Actually run the checklist end to end**

Follow every step in `docs/verify-phase-3.md` against a freshly-started stack (`docker compose -f deploy/docker-compose.yml down -v && ./scripts/dev.sh`). Fix anything that doesn't match the expected output before proceeding — this is the real acceptance gate for the whole phase.

- [ ] **Step 3: Update `PLAN.md`**

Find Phase 3's section (`### Phase 3 — Auth & identity`) and replace it with:

```markdown
### Phase 3 — Auth & identity — ✅ COMPLETED

- [x] Keycloak realm `questlog` (exported to `deploy/keycloak/`): clients for web/admin, roles (user, admin)
- [x] `packages/auth`: NextAuth + Keycloak provider, shared by both apps
- [x] Go JWT middleware: JWKS validation, role enforcement (admin-api requires admin role)
- [x] `identity` context: local user profile row synced on first login (username, avatar, bio)
- [x] E2E: signup → login → authenticated page in web; admin login → admin portal — verified manually per `docs/verify-phase-3.md` (full Playwright automation is Phase 9 scope)

**Non-obvious decisions:** Keycloak sits behind Docker Compose, so the browser
and the Next.js server need different, non-interchangeable URLs to reach it —
documented in `docs/adr/0001-keycloak-docker-network-split.md`. The same split
means the Go backend fetches JWKS from `keycloak:8080` but validates the token's
`iss` against `localhost:8082` — deliberate, and the reason `KEYCLOAK_JWKS_URL`
and `KEYCLOAK_ISSUER` name different hosts. `UserProfile` stores no role data
(roles are always read live from the JWT, never cached locally, to avoid drift
when an admin changes someone's Keycloak role).
```

- [ ] **Step 4: Commit**

```bash
git add docs/verify-phase-3.md PLAN.md
git commit -m "📝 Add Phase 3 verification checklist, mark Phase 3 complete"
```

---

## Self-Review

**Spec coverage** — every PLAN.md Phase 3 bullet has a task: Keycloak realm → Task 8; `packages/auth` → Task 9; Go JWT middleware → Task 5; `identity` context sync-on-login → Tasks 1–4, 6; E2E → Task 12. The "username/password only" constraint is realized by simply never adding a social IdP client to Task 8's realm JSON. The "registration enabled" constraint is `registrationAllowed: true` in the same file.

**Placeholder scan** — no TBD/TODO/"add error handling" phrases; every step shows complete, runnable code or an exact command with its expected output.

**Type/name consistency, checked across tasks:**
- `domain.UserProfile`, `domain.ProfileRepository`, `domain.NewUserProfile` (Task 2) match their usage in Task 3 (`application/sync.go`) and Task 4 (`infrastructure/postgres_repository.go`).
- `application.SyncInput`, `application.NewSyncService`, `(*SyncService).EnsureProfile` (Task 3) match Task 6's `sync_handler.go`.
- `infrastructure.NewPostgresProfileRepository` (Task 4) matches Task 6's `main.go`.
- `authmw.NewJWKS`, `(*JWKS).Refresh`, `(*JWKS).Keyfunc`, `(*JWKS).StartBackgroundRefresh`, `authmw.Claims`, `authmw.RequireAuth`, `authmw.RequireRole`, `authmw.ClaimsFromContext` (Task 5) match their usage in Task 6 and Task 7's `main.go` files, and in Task 6's `sync_handler.go`.
- `shared.MustEnv`, `shared.OK` (Task 1 / pre-existing) match usage in Task 6 and Task 7.
- Env var names (`AUTH_KEYCLOAK_ISSUER`, `AUTH_KEYCLOAK_INTERNAL_ISSUER`, `AUTH_KEYCLOAK_ID`, `AUTH_KEYCLOAK_SECRET`, `IDENTITY_SYNC_URL`, `KEYCLOAK_JWKS_URL`, `KEYCLOAK_ISSUER`, `AUTH_SECRET`) are identical across Task 8 (docker-compose.yml), Task 9 (`config.ts`), Task 10/11 (`.env.example` files).
- `KEYCLOAK_JWKS_URL` (host `keycloak:8080`) and `KEYCLOAK_ISSUER` (host `localhost:8082`) intentionally disagree on hostname in Tasks 6, 7 and 8. This is required, not a copy-paste slip: `KC_HOSTNAME` pins every token's `iss` to the public URL while the backend still fetches keys over the compose network. ADR-0001 documents it and both `main.go` files carry an inline comment.
- Keycloak client IDs/secrets (`questlog-web` / `questlog-web-dev-secret`, `questlog-admin` / `questlog-admin-dev-secret`) match exactly between Task 8's realm JSON, docker-compose env vars, and Tasks 10/11's `.env.example` files.
- `authConfig` export name matches between Task 9 (`index.ts`) and Tasks 10/11 (`auth.ts` imports).
