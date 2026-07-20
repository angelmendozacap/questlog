// Command admin-api serves the QuestLog moderation/admin API (admin app, :8081).
// Every route requires the Keycloak "admin" role (JWT middleware lands in
// Phase 3); handlers are composed from each context's admin service, per
// docs/specs/05-admin.md — there is no admin god-context.
package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v3"

	"github.com/alfredomendoza/questlog/backend/internal/shared"
)

func main() {
	app := fiber.New()

	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(shared.OK())
	})

	port := os.Getenv("PORT")
	if port == "" {
		port = "8081"
	}
	log.Fatal(app.Listen(":" + port))
}
