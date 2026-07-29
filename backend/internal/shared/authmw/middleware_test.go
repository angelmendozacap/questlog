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
