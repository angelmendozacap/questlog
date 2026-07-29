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
