package interfaces_test

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/json"
	"errors"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/alfredomendoza/questlog/backend/internal/identity/application"
	"github.com/alfredomendoza/questlog/backend/internal/identity/domain"
	"github.com/alfredomendoza/questlog/backend/internal/identity/interfaces"
	"github.com/alfredomendoza/questlog/backend/internal/shared/authmw"
)

// These tests mount SyncHandler.Handle behind the real authmw.RequireAuth
// middleware, using a genuinely signed JWT — the same setup authmw's own
// tests use — rather than poking claims into the Fiber context directly.
// claimsKey is unexported in authmw, so there is no way to fake
// ClaimsFromContext's storage even if we wanted to; this also means the
// test exercises the exact path production traffic takes.

const testIssuer = "http://localhost:8082/realms/questlog"

// fakeRepo is a minimal domain.ProfileRepository stub. Using a real
// application.SyncService on top of it (rather than a fake SyncService)
// means these tests exercise the real domain validation and the real
// error-wrapping (fmt.Errorf(..., %w)) that production code goes through —
// the same wrapping the leaked-error regression (case c below) depends on.
type fakeRepo struct {
	findFn   func(ctx context.Context, id uuid.UUID) (domain.UserProfile, bool, error)
	insertFn func(ctx context.Context, p domain.UserProfile) (domain.UserProfile, error)
}

func (f *fakeRepo) FindByKeycloakID(ctx context.Context, id uuid.UUID) (domain.UserProfile, bool, error) {
	if f.findFn != nil {
		return f.findFn(ctx, id)
	}
	return domain.UserProfile{}, false, nil
}

func (f *fakeRepo) Insert(ctx context.Context, p domain.UserProfile) (domain.UserProfile, error) {
	if f.insertFn != nil {
		return f.insertFn(ctx, p)
	}
	p.ID = uuid.New()
	return p, nil
}

var _ domain.ProfileRepository = (*fakeRepo)(nil)

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

func testToken(t *testing.T, key *rsa.PrivateKey, subject, preferredUsername string) string {
	t.Helper()
	claims := authmw.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Subject:   subject,
			Issuer:    testIssuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
		PreferredUsername: preferredUsername,
		Picture:           "https://example.com/a.png",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "test-kid"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return signed
}

// newTestApp mounts the handler exactly as cmd/public-api/main.go does:
// authmw.RequireAuth ahead of SyncHandler.Handle.
func newTestApp(repo domain.ProfileRepository, jwks *authmw.JWKS) *fiber.App {
	handler := interfaces.NewSyncHandler(application.NewSyncService(repo))
	app := fiber.New()
	app.Post("/auth/sync", authmw.RequireAuth(jwks.Keyfunc, testIssuer), handler.Handle)
	return app
}

func doSync(t *testing.T, app *fiber.App, token string) (int, map[string]any, string) {
	t.Helper()
	req := httptest.NewRequest("POST", "/auth/sync", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	var body map[string]any
	if err := json.Unmarshal(raw, &body); err != nil {
		t.Fatalf("unmarshal body %q: %v", raw, err)
	}
	return resp.StatusCode, body, string(raw)
}

func TestSyncHandler(t *testing.T) {
	key := testKeyPair(t)
	jwks := testJWKS(&key.PublicKey)

	cases := []struct {
		name string
		// subject/username become the token's sub / preferred_username.
		subject  string
		username string
		repo     *fakeRepo

		wantStatus int
		check      func(t *testing.T, body map[string]any, raw string)
	}{
		{
			// (a) domain.ErrEmptyUsername surfaces when preferred_username
			// is blank — a client/token problem, so 400, not 500.
			name:     "empty username maps to 400 invalid_claims",
			subject:  uuid.New().String(),
			username: "",
			repo:     &fakeRepo{},

			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any, _ string) {
				if body["code"] != "invalid_claims" {
					t.Errorf("code = %v, want invalid_claims", body["code"])
				}
				msg, _ := body["message"].(string)
				if msg == "" {
					t.Error("message was empty, want a non-empty explanation")
				}
			},
		},
		{
			// (b) sub="00000000-...-0000" is a syntactically valid UUID, so
			// it passes the handler's uuid.Parse guard and only trips
			// domain.NewUserProfile's nil check — must still be a 400, not
			// a 500, with the same invalid_claims shape as (a).
			name:     "nil-uuid subject maps to 400 invalid_claims",
			subject:  uuid.Nil.String(),
			username: "nekomata",
			repo:     &fakeRepo{},

			wantStatus: fiber.StatusBadRequest,
			check: func(t *testing.T, body map[string]any, _ string) {
				if body["code"] != "invalid_claims" {
					t.Errorf("code = %v, want invalid_claims", body["code"])
				}
				msg, _ := body["message"].(string)
				if msg == "" {
					t.Error("message was empty, want a non-empty explanation")
				}
			},
		},
		{
			// (c) THE REGRESSION TEST for the leak: a repository failure
			// carrying driver-level text must never reach the client. The
			// service wraps this with fmt.Errorf("sync: insert profile:
			// %w", ...) exactly as production code does (postgres_repository.go
			// passes pgx errors through unwrapped, sync.go wraps them) —
			// we don't hand-construct the wrapped error, we let the real
			// SyncService wrap a driver-shaped one so the test tracks
			// production behavior.
			name:     "repository failure returns generic 500, driver text not leaked",
			subject:  uuid.New().String(),
			username: "nekomata",
			repo: &fakeRepo{
				insertFn: func(_ context.Context, _ domain.UserProfile) (domain.UserProfile, error) {
					return domain.UserProfile{}, errors.New(`pq: duplicate key value violates unique constraint "user_profiles_username_key"`)
				},
			},

			wantStatus: fiber.StatusInternalServerError,
			check: func(t *testing.T, body map[string]any, raw string) {
				if body["code"] != "sync_failed" {
					t.Errorf("code = %v, want sync_failed", body["code"])
				}
				if body["message"] != "unable to sync profile" {
					t.Errorf("message = %v, want the fixed generic string", body["message"])
				}
				for _, leaked := range []string{"pq:", "duplicate key", "user_profiles_username_key", "constraint"} {
					if strings.Contains(raw, leaked) {
						t.Errorf("response body leaked driver text %q: %s", leaked, raw)
					}
				}
			},
		},
		{
			// (d) success path: unchanged shape, still {id, username,
			// avatarUrl, bio}.
			name:     "success returns profile shape unchanged",
			subject:  uuid.New().String(),
			username: "nekomata",
			repo: &fakeRepo{
				insertFn: func(_ context.Context, p domain.UserProfile) (domain.UserProfile, error) {
					p.ID = uuid.New()
					p.Bio = ""
					return p, nil
				},
			},

			wantStatus: fiber.StatusOK,
			check: func(t *testing.T, body map[string]any, _ string) {
				if body["username"] != "nekomata" {
					t.Errorf("username = %v, want nekomata", body["username"])
				}
				if body["avatarUrl"] != "https://example.com/a.png" {
					t.Errorf("avatarUrl = %v, want https://example.com/a.png", body["avatarUrl"])
				}
				if _, ok := body["id"]; !ok {
					t.Error("response missing id field")
				}
				if _, ok := body["bio"]; !ok {
					t.Error("response missing bio field")
				}
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			app := newTestApp(tc.repo, jwks)
			token := testToken(t, key, tc.subject, tc.username)

			status, body, raw := doSync(t, app, token)
			if status != tc.wantStatus {
				t.Fatalf("status = %d, want %d (body: %s)", status, tc.wantStatus, raw)
			}
			tc.check(t, body, raw)
		})
	}
}
