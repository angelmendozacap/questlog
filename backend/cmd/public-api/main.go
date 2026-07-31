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

	// Both first-party Next.js apps call /auth/sync right after their own
	// sign-in (see packages/auth/src/config.ts), so public-api must accept
	// tokens minted by either Keycloak client.
	allowedAZP := shared.MustEnvList("KEYCLOAK_ALLOWED_AZP")

	profiles := identityinfra.NewPostgresProfileRepository(pool)
	syncHandler := identityiface.NewSyncHandler(identityapp.NewSyncService(profiles))

	app := fiber.New()

	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(shared.OK())
	})

	app.Post("/auth/sync", authmw.RequireAuth(jwks.Keyfunc, issuer, allowedAZP), syncHandler.Handle)

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(app.Listen(":" + port))
}
