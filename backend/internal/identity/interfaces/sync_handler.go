// Package interfaces adapts the identity context's application layer to
// HTTP (Fiber).
package interfaces

import (
	"errors"
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/google/uuid"

	"github.com/alfredomendoza/questlog/backend/internal/identity/application"
	"github.com/alfredomendoza/questlog/backend/internal/identity/domain"
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
		// Domain validation failures mean the token's claims themselves were
		// unusable (e.g. an empty preferred_username) — a client/token
		// problem, not a server fault. These sentinel messages are static
		// and safe to return as-is. Everything else (repository/driver
		// errors, wrapped with %w by application.SyncService) may carry
		// schema or SQL-adjacent detail, so it's logged server-side only
		// and the caller gets a generic, static message.
		//
		// Each branch returns the *sentinel's* own message rather than
		// err.Error(). That's load-bearing for the 409 below, where
		// SyncService has wrapped the error ("sync: insert profile: ...")
		// and the prefix names our internal layering for no caller benefit.
		// For the 400s it's currently equivalent — SyncService returns the
		// validation error unwrapped — so it's the branches staying
		// consistent rather than each one deciding for itself, and it stops
		// mattering the day that path gets wrapped too.
		for _, known := range []error{domain.ErrEmptyUsername, domain.ErrEmptyKeycloakID} {
			if errors.Is(err, known) {
				return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{
					"code": "invalid_claims", "message": known.Error(),
				})
			}
		}
		// A username collision with a different keycloak_id is an
		// actionable conflict (the client — or the person behind it — can
		// do something about it), not a server fault, and not a case we
		// paper over by silently renaming the user.
		if errors.Is(err, domain.ErrUsernameTaken) {
			return c.Status(fiber.StatusConflict).JSON(fiber.Map{
				"code": "username_taken", "message": domain.ErrUsernameTaken.Error(),
			})
		}
		log.Printf("identity: sync failed: %v", err)
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{
			"code": "sync_failed", "message": "unable to sync profile",
		})
	}

	return c.JSON(fiber.Map{
		"id":        profile.ID,
		"username":  profile.Username,
		"avatarUrl": profile.AvatarURL,
		"bio":       profile.Bio,
	})
}
