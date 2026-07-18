# Spec 01 — Catalog

**Status:** draft (pending approval)
**Context:** `backend/internal/catalog`
**UI source of truth:** `docs/design/mockups/v-explorar.html`, `f-hybrid-plata.html` (hero/metadata), `v-admin.html` (curation section)
**Build phase:** 4

## Problem

QuestLog needs a local catalog of movies, series and games that users can search,
view and attach activity to (reviews, diary, lists). We don't own metadata: it comes
from TMDB (movies + series) and IGDB (games). The catalog must import titles on demand,
never leak external API shapes into the domain, and stay curatable by admins.

## User stories

1. As a visitor, I search a title and see results already in QuestLog with their scores.
2. As a user, when local results aren't enough, I see external TMDB/IGDB results and can
   import one with a single action (or it auto-imports on first log/review/list-add).
3. As a user, I open a media detail page with localized (ES-first) metadata: title, year,
   creator, synopsis, genres, poster.
4. As an admin, I can edit imported metadata, merge duplicates, and see import warnings.

## Domain model

- **Title** (aggregate root): `id`, `kind` (movie | series | game), `name` (localized),
  `original_name`, `year`, `synopsis` (localized), `genres[]`, `poster_url`,
  `status` (active | merged | hidden), `created_at`.
- Kind-specific value objects:
  - **Movie**: `runtime_min`, `director`, `studio`
  - **Series**: `seasons`, `episodes`, `network`, `first/last_air_year` (series-level only in v1)
  - **Game**: `platforms[]`, `developer`, `publisher`, `playtime_est` (optional)
- **ExternalRef**: `source` (tmdb_movie | tmdb_tv | igdb), `external_id`, `title_id`.
  Unique on `(source, external_id)` — this is the dedup/race guard.
- Aggregate score is **not** owned here — the review context computes it; catalog pages
  read it via a read model (`title_stats`).

## Import flow (anti-corruption layer)

```
search(q, kind) → local results (Postgres FTS) + external results (TMDB/IGDB, mapped to DTOs)
import(source, external_id) → fetch full record → map to Title → upsert by ExternalRef
```

- Import triggers: explicit "+ Importar", or implicitly on first interaction
  (log/review/add-to-list of an external result).
- Localization: TMDB fetched with `language=es-ES`, fallback `en-US` (missing ES synopsis
  ⇒ store EN + flag `needs_es_translation` for the admin queue). IGDB is EN-only ⇒ same flag.
- Images: hotlink TMDB/IGDB CDN URLs (store path only); attribution shown in footer.
- IGDB auth: Twitch client-credentials token, cached and auto-refreshed (~60 days);
  rate limit 4 req/s → client throttles + retries with backoff.

## API surface (OpenAPI)

Public API (`:8080`):

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/search?q=&kind=` | `{ local: TitleSummary[], external: ExternalResult[] }` |
| GET | `/titles/{id}` | Title detail + `title_stats` |
| POST | `/titles/import` | `{ source, externalId }` → 201 Title (idempotent: 200 if exists) |
| GET | `/titles/trending` | Top titles by recent activity |

Admin API (`:8081`, role `admin`):

| Method | Path | Notes |
| --- | --- | --- |
| PATCH | `/admin/titles/{id}` | Edit metadata overrides |
| POST | `/admin/titles/{id}/merge` | `{ duplicateId }` → repoint refs, mark merged |
| GET | `/admin/titles/issues` | Duplicates + `needs_es_translation` queue |

## Data (schema `catalog`)

`titles` (common cols + kind), `title_movie` / `title_series` / `title_game` (1:1 detail),
`external_refs` (unique source+external_id), `genres` + `title_genres`.
Local search: Postgres `tsvector` over name/original_name (ES + EN configs).

## Edge cases

- Two users import the same title concurrently → `ON CONFLICT` on ExternalRef returns existing.
- External API down → search degrades to local-only with a notice; import returns 503.
- Admin merges a duplicate → all cross-context references repoint via `TitleMerged` domain event.
- TMDB/IGDB record disappears later → title stays (we own imported data); no re-sync in v1.
- Metadata edited by admin wins over future re-imports (override columns, never overwritten).

## Non-goals (v1)

Per-episode tracking, automatic periodic re-sync, self-hosted images, People/cast pages.

## Task breakdown (Phase 4)

1. Migrations for `catalog` schema; sqlc queries.
2. TMDB + IGDB clients (infra) with token cache, throttle, DTO mappers + unit tests (TDD).
3. Domain: Title aggregate + import service (TDD).
4. Search service (local FTS + external merge).
5. HTTP handlers vs OpenAPI types; contract tests.
6. Web: explorar page + detail page wiring; admin: curation queue.
7. E2E: search → import → detail happy path.
