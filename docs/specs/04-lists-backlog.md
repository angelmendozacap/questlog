# Spec 04 — Lists & Backlog

**Status:** draft (pending approval)
**Context:** `backend/internal/lists` (new bounded context, added to the plan's four — it outgrew `review`)
**UI source of truth:** `v-lista.html`, `v-perfil.html` (pestaña Listas), `f-hybrid-plata.html` (botones "+ Lista" / "Pendientes"), `f-diario-plata.html` (stat "en pendientes")
**Build phase:** 7

## Problem

Users curate ordered public/private lists ("Los mejores RPG de 2025") and keep a
backlog ("Pendientes") of titles they want to watch/play. Lists are social objects
(likable, savable, in feeds); the backlog is personal.

## Domain model

- **List** (aggregate root): `owner`, `name`, `description`, `visibility (public | private)`,
  ordered **ListEntry[]**: `title_id`, `position`, `note?`. Unique title per list.
  Max 500 entries. `saves_count` read model ("23 guardados").
- **ListSave**: `user × list` — bookmark of someone else's list ("Guardar lista").
- **BacklogItem**: `user × title`, `added_at`, `note?`. Unique per pair.
  "Visto/jugado" status is **not** stored here — it derives from `review.diary_entries`
  at composition time (UI joins both APIs); backlog item is typically removed when the
  title gets logged (client offers it, never automatic).

**Positions** stored as sparse integers (10, 20, 30…) so reorders touch one row;
renumber on collision. **Events:** `ListCreated` (→ social feed). Likes on lists are
owned by social (polymorphic `Like` from spec 03).

## API surface (public API)

| Method | Path | Notes |
| --- | --- | --- |
| POST | `/lists` | `{ name, description?, visibility }` |
| GET | `/lists/{id}` | 404 if private and not owner |
| PATCH / DELETE | `/lists/{id}` | Owner only |
| POST | `/lists/{id}/entries` | `{ titleId, position?, note? }`; 409 duplicate |
| PATCH / DELETE | `/lists/{id}/entries/{titleId}` | Reorder = PATCH `{ position }` |
| PUT / DELETE | `/lists/{id}/save` | Idempotent bookmark |
| GET | `/users/{username}/lists` | Public ones (all if owner) |
| GET | `/lists/popular` | By likes+saves, recent bias |
| PUT / DELETE | `/titles/{id}/backlog` | Idempotent |
| GET | `/me/backlog?kind=` | Private |

## Data (schema `lists`)

`lists`, `list_entries` (unique list+title, indexed position), `list_saves`,
`backlog_items`, read model `list_stats`.

## Edge cases

- Private list opened by non-owner → 404 (not 403: don't leak existence).
- `TitleMerged` (catalog) → entries/backlog repointed; duplicate collapse keeps lower position.
- Concurrent reorder → last-write-wins per entry (positions are per-row, no global lock).
- Adding to backlog a title already logged → allowed with a client-side notice ("ya lo viste").
- Owner deletes list → saves and likes cascade; feed activities removed.

## Non-goals (v1)

Collaborative lists, list comments, backlog priorities/ordering, auto-remove from
backlog on log.

## Task breakdown (Phase 7)

1. Migrations `lists` schema + sqlc.
2. Domain: List/entries ordering rules, backlog (TDD).
3. `ListCreated` event → social activity.
4. Handlers vs OpenAPI; contract tests.
5. Web: list page, create/edit modal, add-to-list & backlog buttons, profile tab.
6. E2E: create list → add titles → reorder → friend sees it in feed and saves it.
