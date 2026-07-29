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
