// Command public-api serves the public-facing QuestLog API (web app, :8080).
// It registers only public-context handlers; admin-only routes live in
// cmd/admin-api. Both share the same internal/ contexts.
package main

import (
	"log"
	"os"

	"github.com/gofiber/fiber/v3"

	"github.com/alfredomendoza/questlog/backend/internal/shared"
	genapi "github.com/alfredomendoza/questlog/backend/internal/shared/api/generated"
)

func main() {
	app := fiber.New()

	app.Get("/healthz", func(c fiber.Ctx) error {
		return c.JSON(shared.OK())
	})

	// Confirms the generated OpenAPI types link into the binary; real routes
	// land per spec starting Phase 4 (catalog).
	_ = genapi.Health{}

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Fatal(app.Listen(":" + port))
}
