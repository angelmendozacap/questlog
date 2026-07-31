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

	// Stricter than public-api: only the admin Next.js app authenticates
	// via questlog-admin, and that's the only client whose tokens should
	// ever reach /admin/*. A quest_user token minted via questlog-web must
	// still be rejected here even though it's a valid, same-realm token —
	// that's the whole point of checking azp instead of just iss.
	allowedAZP := shared.MustEnvList("KEYCLOAK_ALLOWED_AZP")

	app := fiber.New()

	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(shared.OK())
	})

	admin := app.Group("/admin", authmw.RequireAuth(jwks.Keyfunc, issuer, allowedAZP), authmw.RequireRole("admin"))
	admin.Get("/whoami", func(c fiber.Ctx) error {
		claims, ok := authmw.ClaimsFromContext(c)
		if !ok {
			// Unreachable in practice: RequireRole fails closed before
			// Next() if claims are absent. Checked anyway rather than
			// trusting that invariant to hold across future refactors.
			return fiber.ErrUnauthorized
		}
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
