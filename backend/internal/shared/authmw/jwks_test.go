package authmw_test

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"

	"github.com/alfredomendoza/questlog/backend/internal/shared/authmw"
)

// testJWK and testJWKSDoc mirror the shape of a real Keycloak JWKS document
// (RFC 7517), built independently of authmw's internal jwksKey/jwksResponse
// types so these tests exercise the wire format, not the package's own
// struct tags.
type testJWK struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type testJWKSDoc struct {
	Keys []testJWK `json:"keys"`
}

func encodeRSAPublicKeyJWK(pub *rsa.PublicKey, kid string) testJWK {
	return testJWK{
		Kty: "RSA",
		Kid: kid,
		N:   base64.RawURLEncoding.EncodeToString(pub.N.Bytes()),
		E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pub.E)).Bytes()),
	}
}

// keyfuncToken builds a bare *jwt.Token carrying just enough (Method + kid
// header) to drive JWKS.Keyfunc directly, without going through a full
// jwt.Parse — used to inspect which key the cache currently holds.
func keyfuncToken(kid string) *jwt.Token {
	return &jwt.Token{
		Method: jwt.SigningMethodRS256,
		Header: map[string]any{"kid": kid},
	}
}

// TestRefresh_FetchesRealJWKSAndValidatesToken proves Refresh's HTTP +
// base64/big.Int decoding path produces a key that actually verifies a real
// RS256 signature — not merely that parsing returned a non-nil key. This is
// the one path between Keycloak's JWKS document and every trust decision in
// the project; every other test in this package bypasses it via
// SetKeysForTest.
func TestRefresh_FetchesRealJWKSAndValidatesToken(t *testing.T) {
	key := testKeyPair(t)

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := testJWKSDoc{Keys: []testJWK{encodeRSAPublicKeyJWK(&key.PublicKey, "real-kid")}}
		_ = json.NewEncoder(w).Encode(doc)
	}))
	defer server.Close()

	jwks := authmw.NewJWKS(server.URL)
	if err := jwks.Refresh(context.Background()); err != nil {
		t.Fatalf("Refresh: %v", err)
	}

	claims := authmw.Claims{
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    testIssuer,
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Hour)),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "real-kid"
	signed, err := token.SignedString(key)
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}

	parsed, err := jwt.ParseWithClaims(signed, &authmw.Claims{}, jwks.Keyfunc)
	if err != nil {
		t.Fatalf("ParseWithClaims: %v", err)
	}
	if !parsed.Valid {
		t.Fatal("token should be valid against the key fetched via Refresh")
	}
}

// TestRefresh_EmptyKeysPreservesCache covers the gap where a 200 response
// with no RSA keys (e.g. a misconfigured JWKS URL pointing at realm
// metadata instead of the certs endpoint) would otherwise silently wipe a
// working cache. Refresh must error and leave the previous cache in place.
func TestRefresh_EmptyKeysPreservesCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(testJWKSDoc{})
	}))
	defer server.Close()

	origKey := testKeyPair(t)
	jwks := authmw.NewJWKS(server.URL)
	jwks.SetKeysForTest(map[string]*rsa.PublicKey{"orig-kid": &origKey.PublicKey})

	if err := jwks.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh: want error for a JWKS response with no RSA keys, got nil")
	}

	got, err := jwks.Keyfunc(keyfuncToken("orig-kid"))
	if err != nil {
		t.Fatalf("Keyfunc after failed refresh: %v (cache should still hold orig-kid)", err)
	}
	gotPub, ok := got.(*rsa.PublicKey)
	if !ok {
		t.Fatalf("Keyfunc returned %T, want *rsa.PublicKey", got)
	}
	if !gotPub.Equal(&origKey.PublicKey) {
		t.Fatal("cached key changed after a failed refresh; want the original preserved")
	}
}

// TestRefresh_NonOKStatusPreservesCache covers the network/status error
// path: a non-200 response must error without touching the existing cache.
func TestRefresh_NonOKStatusPreservesCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	origKey := testKeyPair(t)
	jwks := authmw.NewJWKS(server.URL)
	jwks.SetKeysForTest(map[string]*rsa.PublicKey{"orig-kid": &origKey.PublicKey})

	if err := jwks.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh: want error for a non-200 response, got nil")
	}

	got, err := jwks.Keyfunc(keyfuncToken("orig-kid"))
	if err != nil {
		t.Fatalf("Keyfunc after failed refresh: %v (cache should still hold orig-kid)", err)
	}
	if gotPub := got.(*rsa.PublicKey); !gotPub.Equal(&origKey.PublicKey) {
		t.Fatal("cached key changed after a failed refresh; want the original preserved")
	}
}

// TestRefresh_MalformedKeyPreservesCache covers the per-key decode error
// path: a key with invalid base64 in `n` must error without touching the
// existing cache (and without silently dropping just that key and
// succeeding with a partial set).
func TestRefresh_MalformedKeyPreservesCache(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		doc := testJWKSDoc{Keys: []testJWK{{
			Kty: "RSA",
			Kid: "bad-kid",
			N:   "not valid base64url!!!",
			E:   "AQAB",
		}}}
		_ = json.NewEncoder(w).Encode(doc)
	}))
	defer server.Close()

	origKey := testKeyPair(t)
	jwks := authmw.NewJWKS(server.URL)
	jwks.SetKeysForTest(map[string]*rsa.PublicKey{"orig-kid": &origKey.PublicKey})

	if err := jwks.Refresh(context.Background()); err == nil {
		t.Fatal("Refresh: want error for a key with malformed base64 n, got nil")
	}

	got, err := jwks.Keyfunc(keyfuncToken("orig-kid"))
	if err != nil {
		t.Fatalf("Keyfunc after failed refresh: %v (cache should still hold orig-kid)", err)
	}
	if gotPub := got.(*rsa.PublicKey); !gotPub.Equal(&origKey.PublicKey) {
		t.Fatal("cached key changed after a failed refresh; want the original preserved")
	}
}
