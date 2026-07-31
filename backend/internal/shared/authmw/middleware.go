package authmw

import (
	"slices"
	"strings"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
)

// Claims are the token fields QuestLog cares about. PreferredUsername and
// Picture come straight from Keycloak's standard OIDC claims; RealmAccess
// is Keycloak-specific (its default "roles" client scope maps realm roles
// into the access token under this exact shape). AuthorizedParty is the
// standard OIDC `azp` claim — Keycloak stamps it with the client ID that
// requested the token, which is what RequireAuth checks against its
// allow-list (see the `allowedAZP` param below).
type Claims struct {
	jwt.RegisteredClaims
	PreferredUsername string `json:"preferred_username"`
	Email             string `json:"email"`
	Picture           string `json:"picture"`
	AuthorizedParty   string `json:"azp"`
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
// as never-expiring), issuer pinned to `issuer`, and `azp` (the client that
// requested the token) against `allowedAZP`. On success it stores the
// parsed Claims for downstream handlers/middleware via ClaimsFromContext.
//
// `issuer` is the browser-facing realm URL, which is what Keycloak puts in
// `iss`. It is deliberately NOT the same host the JWKS is fetched from —
// see docs/adr/0001-keycloak-docker-network-split.md.
//
// `allowedAZP` is which Keycloak client(s) this service trusts tokens from.
// Signature/exp/iss alone only prove a token came from *this* realm — not
// which client minted it. Keycloak's default `aud` claim is just "account"
// for every client, so it's not useful here; `azp` is. A token with no
// `azp` claim at all is rejected (fail closed) rather than treated as
// implicitly trusted — every real Keycloak grant (auth code, password)
// stamps `azp` with the requesting client, so a token missing it is either
// malformed or from a grant type this project doesn't use.
func RequireAuth(keyFunc jwt.Keyfunc, issuer string, allowedAZP []string) fiber.Handler {
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

		if !slices.Contains(allowedAZP, claims.AuthorizedParty) {
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
