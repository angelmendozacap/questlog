// Package authmw validates Keycloak-issued JWTs on incoming requests and
// enforces realm roles. Tokens are checked for signature (against the
// realm's JWKS), expiry, and issuer.
//
// The JWKS URL and the expected issuer intentionally point at different
// hosts: keys are fetched over the Docker network (keycloak:8080) while
// `iss` is pinned to the browser-facing URL Keycloak stamps into tokens
// (localhost:8082). See docs/adr/0001-keycloak-docker-network-split.md.
package authmw

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type jwksKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

// JWKS fetches and caches RSA public keys from a Keycloak JWKS endpoint,
// keyed by `kid`, so RequireAuth can validate RS256-signed tokens.
type JWKS struct {
	url  string
	mu   sync.RWMutex
	keys map[string]*rsa.PublicKey
}

func NewJWKS(url string) *JWKS {
	return &JWKS{url: url, keys: map[string]*rsa.PublicKey{}}
}

// Refresh fetches the current key set and replaces the cache atomically.
func (j *JWKS) Refresh(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, j.url, nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("authmw: jwks fetch %s: status %d", j.url, resp.StatusCode)
	}

	var parsed jwksResponse
	if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
		return fmt.Errorf("authmw: decode jwks: %w", err)
	}

	keys := make(map[string]*rsa.PublicKey, len(parsed.Keys))
	for _, k := range parsed.Keys {
		if k.Kty != "RSA" {
			continue
		}
		pub, err := rsaPublicKeyFromJWK(k.N, k.E)
		if err != nil {
			return fmt.Errorf("authmw: parse jwk %s: %w", k.Kid, err)
		}
		keys[k.Kid] = pub
	}

	j.mu.Lock()
	j.keys = keys
	j.mu.Unlock()
	return nil
}

// StartBackgroundRefresh refreshes the key cache on a ticker. Refresh
// failures are logged to stderr and the previous cache is kept — a
// transient Keycloak outage shouldn't take down already-validating auth.
func (j *JWKS) StartBackgroundRefresh(interval time.Duration) {
	go func() {
		ticker := time.NewTicker(interval)
		defer ticker.Stop()
		for range ticker.C {
			if err := j.Refresh(context.Background()); err != nil {
				fmt.Printf("authmw: background jwks refresh failed (keeping cached keys): %v\n", err)
			}
		}
	}()
}

// Keyfunc implements jwt.Keyfunc against the cached key set.
func (j *JWKS) Keyfunc(token *jwt.Token) (interface{}, error) {
	if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
		return nil, fmt.Errorf("authmw: unexpected signing method %v", token.Header["alg"])
	}
	kid, _ := token.Header["kid"].(string)

	j.mu.RLock()
	key, ok := j.keys[kid]
	j.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("authmw: unknown key id %q", kid)
	}
	return key, nil
}

func rsaPublicKeyFromJWK(nB64, eB64 string) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(nB64)
	if err != nil {
		return nil, fmt.Errorf("decode n: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(eB64)
	if err != nil {
		return nil, fmt.Errorf("decode e: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	e := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{N: n, E: int(e.Int64())}, nil
}

// SetKeysForTest seeds the key cache directly, bypassing Refresh's HTTP
// call — used by authmw's own tests to avoid a real JWKS server.
func (j *JWKS) SetKeysForTest(keys map[string]*rsa.PublicKey) {
	j.mu.Lock()
	j.keys = keys
	j.mu.Unlock()
}
