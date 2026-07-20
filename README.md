# QuestLog

A Letterboxd-style review platform for **movies, series and games** — a deliberately
over-engineered portfolio monorepo for learning DDD, monorepos, and CI/CD.
See [`PLAN.md`](PLAN.md) for the full roadmap, [`docs/design/`](docs/design/) for the
visual system, and [`docs/specs/`](docs/specs/) for the feature specs behind each phase.

## Architecture at a glance

```
apps/web (:3000)    apps/admin (:3001)     ← Next.js 15, next-intl (es default / en)
      │                    │
      └── @questlog/{ui,types,api-client,auth,config}   ← pnpm workspace, Turborepo
      │
public-api (:8080)   admin-api (:8081)      ← Go + Fiber, generated from api/openapi.yaml
      │
backend/internal/{catalog,review,social,lists,identity}  ← DDD bounded contexts
      │
   PostgreSQL                                Keycloak (OIDC)
```

- **Two API binaries, one codebase.** `public-api` and `admin-api` register different
  handler sets from the same `internal/` contexts — the authorization boundary lives at
  the binary level, not just role checks.
- **Contexts don't import each other's internals.** Cross-context communication goes
  through domain events (see `docs/specs/03-social.md` for how the feed consumes them).
- **OpenAPI is the contract.** `api/openapi.yaml` → generated Go server types +
  `packages/types` (TS). CI fails the build on drift (`.github/workflows/ci.yml`).

## Local development

Requires Node 22+, pnpm, Go 1.26+, and Docker.

```bash
pnpm install
pnpm dev              # web + admin against whatever backend you have running

# full stack (Postgres, Keycloak, both APIs, both apps):
./scripts/dev.sh
```

Other useful commands: `pnpm turbo lint typecheck build` (everything, TS side),
`cd backend && go build ./... && go vet ./... && go test ./...` (Go side),
`pnpm --filter @questlog/types generate` (regenerate TS types from the OpenAPI contract).

## Status

Phases 0–2 done (design system, specs, monorepo scaffold). See `PLAN.md` for what's next.
