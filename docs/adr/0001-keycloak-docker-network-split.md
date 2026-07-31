# ADR-0001: Split public vs. internal Keycloak URLs for NextAuth

**Status:** accepted
**Context:** Phase 3 — Auth & identity

## Context

Keycloak runs as its own `docker-compose` service (`keycloak`, container port
8080, published to the host as `8082`). The Next.js apps run as separate
containers (`web`, `admin`) in the same compose network. NextAuth's Keycloak
provider needs four Keycloak endpoints: `authorization` (the browser is
redirected here), and `token` / `userinfo` / `jwks_endpoint` (called directly
by the Next.js server, never by the browser).

A single "issuer" URL can't serve both purposes inside Docker Compose:

- The **browser** can only reach Keycloak via the host-published address
  (`http://localhost:8082`) — it has no visibility into the compose network.
- The **Next.js server**, running inside its own container, cannot reach
  `localhost:8082` — inside a container, `localhost` refers to that
  container itself, not the `keycloak` one. It needs the compose network's
  service DNS name instead (`http://keycloak:8080`).

Using only the public URL breaks server-to-server calls (connection
refused). Using only the internal URL breaks the browser redirect (the
browser can't resolve `keycloak`).

## Decision

Configure two env vars and pass explicit endpoint overrides to the NextAuth
Keycloak provider instead of relying on OIDC discovery from a single issuer:

- `AUTH_KEYCLOAK_ISSUER` (public) → builds the `authorization` endpoint.
- `AUTH_KEYCLOAK_INTERNAL_ISSUER` (internal; defaults to the public one when
  unset, so plain `pnpm dev` outside Docker still works with one URL) →
  builds `token`, `userinfo`, `jwks_endpoint`.

Keycloak's `KC_HOSTNAME` is fixed to the public URL
(`http://localhost:8082`) so every issued token's `iss` claim is the same
regardless of which network path was used to request it — otherwise a
token minted via the internal token endpoint would carry
`iss: http://keycloak:8080/realms/questlog`, which doesn't match the
`issuer` NextAuth validates against, and login would fail with an
issuer-mismatch error.

The same split reaches the Go backend, and this is the part most likely to
look like a typo during review. `internal/shared/authmw` validates
signature, expiry **and** issuer, but its two Keycloak-related env vars
deliberately name different hosts:

- `KEYCLOAK_JWKS_URL=http://keycloak:8080/realms/questlog/protocol/openid-connect/certs`
  — the backend *fetches keys* over the compose network.
- `KEYCLOAK_ISSUER=http://localhost:8082/realms/questlog` — the backend
  *validates `iss`* against the public URL, because `KC_HOSTNAME` makes
  Keycloak stamp that value into every token no matter which network path
  minted it.

Pinning `iss` to the JWKS host instead would reject every real token.

## Consequences

- Two Keycloak-related env vars instead of one on the Next.js side, and two
  more (`KEYCLOAK_JWKS_URL`, `KEYCLOAK_ISSUER`) on the Go side — documented in
  `apps/{web,admin}/.env.example` and `deploy/docker-compose.yml`.
- If QuestLog ever deploys behind real domains, `AUTH_KEYCLOAK_INTERNAL_ISSUER`
  can point at an internal service mesh address while
  `AUTH_KEYCLOAK_ISSUER` stays the public one — same mechanism, no rework.
  `KEYCLOAK_ISSUER` follows the public URL wherever it goes.
- The backend trusts exactly one issuer. Accepting tokens from more than one
  Keycloak instance would mean turning `KEYCLOAK_ISSUER` into a list — a
  deliberate change, not an accident. This is a separate dimension from
  *which client within that issuer* a token was minted for: `iss` only
  proves a token came from the `questlog` realm, not which of the realm's
  clients requested it. `internal/shared/authmw` additionally checks the
  token's `azp` claim against a per-binary allow-list
  (`KEYCLOAK_ALLOWED_AZP` — `questlog-web,questlog-admin` for `public-api`,
  `questlog-admin` only for `admin-api`), so a third client added to the
  realm later doesn't silently get its tokens accepted just because it's
  same-realm and well-formed.
