# PLAN — QuestLog

**QuestLog** is a Letterboxd-style review site for **movies, series and games** — your quest log for everything you watch and play: users sign up, pick a title, write a review, score it, keep a diary, follow other users, and share their activity. Built as a deliberately over-engineered portfolio monorepo for learning **DDD, monorepos, CI/CD, and system design**.

Reference architecture: [Saru — Next.js × Go Monorepo Structure](https://zenn.dev/kochan_saru/articles/saru-nextjs-go-monorepo-3) (adapted from multi-tenant SaaS to a single-product social app).

---

## 1. Goals & Non-Goals

**Goals**

- A real, demoable product at every phase (each phase ends with working, dockerized code).
- Showcase-quality architecture: DDD bounded contexts, OpenAPI-first contracts, monorepo tooling, CI/CD.
- Learn by building: every non-obvious decision gets a short ADR in `docs/adr/`.
- A README that explains the whole architecture (diagrams, decisions, trade-offs) like the reference article does.

**Non-Goals (for now)**

- Multi-tenancy — traditional single-realm signup/signin only.
- Cloud deployment — CI builds and tests everything; the stack runs locally via Docker Compose. Deployment is a future phase.
- Mobile apps, monetization.

## 2. Product Scope (v1)

Core: browse/search movies, series & games, review + score them.

| Feature | Description |
| --- | --- |
| Catalog | Search & import titles from TMDB (movies **and series** — same API) and IGDB (games); media detail pages. v1 tracks series at series/season level, not per-episode |
| Reviews & ratings | Write/edit reviews, score titles (0–10), aggregate scores per title |
| Diary | Letterboxd-style log: "watched/played X on date Y", with optional review attached |
| Profiles & activity | Public profile: reviews, ratings, diary, stats |
| Follows & feed | Follow users; home feed of followed users' recent activity |
| Likes & comments | Like and comment on reviews |
| Lists & backlog | Custom lists ("Best RPGs 2025") + status tracking (watched/played/backlog) |
| Pixel avatars | FF-style pixel-art avatar shown beside user scores, with score-reactive animations (cheer on high scores, slump on low ones) |
| i18n | Two languages: **ES (default)** and EN, across web and admin |
| Admin | Moderation queue (reported reviews/comments), user management, catalog curation (fix bad imports) |

## 3. Stack

| Area | Technology | Why |
| --- | --- | --- |
| Frontend | Next.js 15 (App Router) | RSC, ecosystem, matches reference |
| UI foundation | Tailwind + shadcn/ui (owned code, Radix underneath) | Heavy retro reskin is only feasible when you own the components; Radix keeps a11y |
| UI accents | 8bitcn/ui (cherry-picked), Motion (framer-motion) | 8-bit component head start; kinetic JRPG-style transitions |
| i18n | next-intl (ES default, EN secondary) | App Router-native message/routing i18n; locale in URL (`/es`, `/en`) |
| Backend | Go + Fiber | Fast, simple, explicit |
| Data access | pgx + sqlc | Explicit SQL keeps DDD repositories honest; sqlc adds type safety |
| Database | PostgreSQL | Single DB, schema-per-context discipline |
| Migrations | golang-migrate (via `cmd/migrate`) | Versioned, CI-checkable |
| Auth | Keycloak + NextAuth (Auth.js) | Self-hosted OIDC; real-world IAM learning; matches reference |
| Contracts | OpenAPI (`api/openapi.yaml`) | Source of truth: `oapi-codegen` (Go) + `openapi-typescript` (TS); CI fails on drift |
| TS workspace | pnpm workspaces + Turborepo | `apps/*` + `packages/*`, cached task graph |
| Go workspace | `go.work` | Ties `backend/` + `backend/tools/` (+ future services); contexts stay packages, **not** modules |
| E2E | Playwright | Cross-app flows against the compose stack |
| CI | GitHub Actions | Lint, test, typecheck, contract-drift check, docker build, e2e |
| Local dev | Docker Compose + `air` (Go hot reload) | One-command stack |

**Auth decision (Keycloak):** chosen over Clerk/Auth0 (nothing to show architecturally, vendor lock-in), hand-rolled auth (security liability), and Auth.js-only (auth logic stuck in frontend layer). Trade-off accepted: a heavyweight container (~500MB RAM) locally. Flow: Keycloak authenticates users → NextAuth handles OAuth/session in both Next.js apps → Go APIs validate Keycloak JWTs via JWKS + enforce roles.

## 4. Repository Layout

TypeScript packages use the `@questlog/*` scope (`@questlog/ui`, `@questlog/types`, ...); the Go module is `github.com/<user>/questlog/backend`.

```
questlog/
├── apps/
│   ├── web/                  # Public app — browse, review, diary, feed (:3000)
│   └── admin/                # Moderation + catalog curation (:3001)
├── packages/
│   ├── types/                # TS types generated from OpenAPI
│   ├── ui/                   # Pixel/JRPG design system (shadcn-based, owned code)
│   ├── api-client/           # Fetch client + React Query hooks
│   ├── auth/                 # Shared NextAuth + Keycloak config
│   └── config/               # Shared ESLint / TS configs
├── backend/                  # Go module (go.work member)
│   ├── cmd/
│   │   ├── public-api/       # :8080 — consumed by web
│   │   ├── admin-api/        # :8081 — consumed by admin
│   │   └── migrate/          # Migration CLI
│   ├── internal/             # DDD bounded contexts (packages, not modules)
│   │   ├── catalog/          # titles, TMDB/IGDB import (anti-corruption layer)
│   │   ├── review/           # reviews, ratings, diary
│   │   ├── social/           # follows, likes, comments, feed
│   │   ├── lists/            # custom lists, saves, backlog
│   │   ├── identity/         # user profiles (synced from Keycloak)
│   │   └── shared/           # shared kernel: errors, pagination, domain events
│   └── tools/                # Go module: pinned dev tools (oapi-codegen, sqlc, air, golangci-lint)
├── api/
│   └── openapi.yaml          # Contract source of truth
├── e2e/                      # Playwright
├── deploy/                   # docker-compose.yml, Dockerfiles, Keycloak realm export
├── docs/
│   ├── specs/                # SDD feature specs (one per feature, approved before build)
│   ├── adr/                  # Architecture Decision Records
│   └── design/               # Mockups, design tokens, art direction
├── go.work
├── pnpm-workspace.yaml
├── turbo.json
└── PLAN.md                   # This file
```

**Each bounded context** follows the same internal layering:

```
internal/<context>/
├── domain/           # Entities, value objects, domain events, repository interfaces
├── application/      # Use cases / services
├── infrastructure/   # Postgres repos (sqlc), external API clients
└── interfaces/       # HTTP handlers, DTOs (mapped to/from OpenAPI types)
```

**Key architectural rules**

1. **Two API binaries, one codebase.** `public-api` and `admin-api` register different handler sets from the same `internal/`. Authorization boundary lives at the binary level (like the reference's 4 APIs), plus JWT role checks.
2. **Contexts don't import each other's internals.** Cross-context communication goes through domain events (e.g., `ReviewPublished` → social feed) or application-layer interfaces. This is the core DDD discipline of the project.
3. **External APIs never leak into the domain.** TMDB/IGDB clients live in `catalog/infrastructure` as an anti-corruption layer; the domain only knows `Title`, `Movie`, `Series`, `Game`.
4. **OpenAPI is the contract.** Handwritten `openapi.yaml` → generated Go server types + TS types. CI regenerates and fails on diff (fixes the drift problem the reference article admits to).

## 5. Working Method (SDD — Spec-Driven Development, simplified)

Every feature phase follows the same loop:

1. **Spec** — a short doc in `docs/specs/` (problem, user stories, API endpoints, data model, edge cases). Reviewed and approved before code.
2. **Plan** — implementation task breakdown derived from the spec.
3. **Build** — TDD where it pays (domain + application layers especially).
4. **Verify** — tests green, e2e happy path, demoable in compose stack.
5. **Document** — ADR if a non-obvious decision was made; README section updated.

No implementation starts without an approved spec. Specs stay short — one or two pages, not enterprise theater.

## 6. Phases

Each phase ends with: tests passing, stack running in Docker Compose, CI green.

### Phase 0 — Design exploration 🎨 — ✅ COMPLETED

**Outcome:** direction **F** (pixel-modern hybrid + subtle silver frames), dual accent
theme (green/blue, user-switchable), tokens in `docs/design/tokens.md`.

- [x] Explore visual directions as HTML mockups (frontend-design skill), in `docs/design/mockups/`:
  - **A. Retro pixel JRPG** — Sea of Stars / Final Fantasy: pixel borders, dialog-box panels, cozy palette
  - **B. Modern kinetic JRPG** — Persona 5 / Metaphor: angular clip-path panels, high contrast, staggered motion
  - **C. Hybrid** — pixel typography & details on a modern dark layout — *user favorite so far*
  - **D. HD-2D** — Octopath Traveler II: gold filigree panels, painterly dusk backdrops; retyped with C's font stack per user request
  - **E. Diary screen** — the user's private view (chronological log, year goal, private badge), mocked in C style
  - **F.** C/E layouts with D's frame structure in silver/steel (`#a9b6c9`, cool-palette harmony; corner rivets follow the accent color)
  - **Decision:** both accents stay as a user-switchable theme (green & FF blue). Toggle UI = two color swatches in the nav (active one outlined, inactive dimmed); ships as a product feature, not just a mockup aid
- [x] Design the **score-reactive pixel avatar**: FFRK-style sprite (big outlined head; reference image provided by user) shown **only beside the user's own score**, animation driven by it (cheer ≥9, idle 6–8, slump ≤5) — implemented in `docs/design/mockups/sprite.js`
- [x] Pick a direction; extract design tokens (colors, fonts, spacing, border/panel treatments) — **F chosen**
- [x] Document art direction + tokens in `docs/design/` — `docs/design/tokens.md`
- [x] Decide the pixel/readable font pairing — **Silkscreen** (headings/numbers, small doses) + **Inter** (body) + **JetBrains Mono** (data)

### Phase 1 — Specs & tasks 📋 — ✅ COMPLETED (specs pending final user approval)

> Input: full-view mockups in `docs/design/mockups/` (see its README — 9 product views
> covering login, feed, explore/import, detail, review, profile, list, diary, admin).
> Each spec should reference its mockup as the UI source of truth.

- [x] Write `docs/specs/` for: catalog (01), reviews+diary (02), social (03), lists (04), admin (05)
- [x] Define the initial OpenAPI surface — endpoint tables inside each spec (the yaml itself lands in Phase 2)
- [x] Define initial DB schema sketch per bounded context — "Data" section of each spec (one Postgres schema per context: catalog, review, social, lists, admin)
- [x] Break build phases into task lists — "Task breakdown" section of each spec

**Decisions made in specs:** rating scale **1–10 integers** (02); diary page private,
logged activity public with per-entry `private` flag (02); feed via **fan-in** (03);
`lists` promoted to its own bounded context (04); no admin god-context — admin-api
composes handlers owned by each context + append-only audit log (05); user suspension
= local flag + Keycloak disable, fail-closed (05).

### Phase 2 — Monorepo scaffold

- [ ] pnpm workspace + Turborepo (`apps/*`, `packages/*`, task graph: build/dev/lint/type-check)
- [ ] `go.work` with `backend/` + `backend/tools/`
- [ ] Scaffold `web`, `admin` (Next.js 15), `packages/{ui,types,api-client,auth,config}`
- [ ] i18n: next-intl in both apps — locale routing (`/es` default, `/en`), shared message catalogs, language switcher
- [ ] Fiber skeletons for `public-api`, `admin-api`; `cmd/migrate`
- [ ] Docker Compose: Postgres, Keycloak, both APIs, both apps; `scripts/dev.sh`
- [ ] OpenAPI codegen pipeline (oapi-codegen + openapi-typescript) wired into turbo
- [ ] CI skeleton: lint (golangci-lint, eslint), typecheck, test, contract-drift check, docker build
- [ ] Root README with architecture overview + diagram (grows every phase)

### Phase 3 — Auth & identity

- [ ] Keycloak realm `questlog` (exported to `deploy/`): clients for web/admin, roles (user, admin)
- [ ] `packages/auth`: NextAuth + Keycloak provider, shared by both apps
- [ ] Go JWT middleware: JWKS validation, role enforcement (admin-api requires admin role)
- [ ] `identity` context: local user profile row synced on first login (username, avatar, bio)
- [ ] E2E: signup → login → authenticated page in web; admin login → admin portal

### Phase 4 — Catalog

- [ ] TMDB + IGDB clients (anti-corruption layer) with API-key config
- [ ] Search-and-import flow: search external → import into local `Title` on first interaction (movies + series via TMDB endpoints, games via IGDB)
- [ ] Media detail pages (movie/game) in web with the chosen design system
- [ ] Admin: catalog curation (edit imported titles, merge duplicates)

### Phase 5 — Reviews, ratings & diary

- [ ] `review` context: Review aggregate (text, score, spoiler flag), rating-only entries, diary entries
- [ ] Aggregate score per title; review lists on media pages
- [ ] Score-reactive pixel avatar component (`@questlog/ui`) rendered beside each score
- [ ] Diary page (chronological log) on profile
- [ ] `ReviewPublished` domain event emitted (consumed by social in Phase 6)

### Phase 6 — Social

- [ ] `social` context: follows, likes, comments
- [ ] Home feed from followed users' activity (fed by domain events)
- [ ] Profile pages with activity, followers/following
- [ ] Report review/comment action (feeds admin moderation queue)

### Phase 7 — Lists & backlog

- [ ] Custom lists (title, description, ordered entries, public/private)
- [ ] Status tracking: watched/played/backlog per user per title
- [ ] Lists on profiles; add-to-list from media pages

### Phase 8 — Admin portal completion

- [ ] Moderation queue: reported reviews/comments → dismiss/remove
- [ ] User management: view, suspend
- [ ] Admin dashboard: basic stats

### Phase 9 — Hardening & documentation

- [ ] Full Playwright e2e suite over the compose stack in CI
- [ ] CI/CD polish: caching, parallel jobs, docker image publishing to GHCR
- [ ] Final architecture README: diagrams (C4-ish), context map, auth flow, decision log index
- [ ] Stretch goals (each = spec first): async import worker (new `go.work` member), Redis feed caching, OpenTelemetry tracing

## 7. Open Items

- TMDB and IGDB API keys need to be created (free; IGDB requires a Twitch developer account with 2FA — Client ID + Secret, OAuth client-credentials token renewed by the backend).
- Rating scale final call (0–10 vs 5 stars with halves) — decide in the reviews spec (Phase 1).
