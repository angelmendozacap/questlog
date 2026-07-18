# Spec 03 — Social

**Status:** draft (pending approval)
**Context:** `backend/internal/social`
**UI source of truth:** `v-home.html` (feed, a quién seguir), `v-resena.html` (likes, comentarios), `v-perfil.html` (seguir, contadores), `v-admin.html` (los reportes alimentan su cola)
**Build phase:** 6

## Problem

QuestLog is social: users follow each other, like and comment reviews, and see a home
feed of what the people they follow watch, play and write. Users can also report
content, which feeds the admin moderation queue. This context must react to what
happens in `review` (and later `lists`) **only through domain events** — never by
importing their internals.

## User stories

1. As a user, I follow/unfollow others from their profile and see follower counts.
2. As a user, my home feed shows recent activity from people I follow: logs, reviews
   (with excerpt), new lists.
3. As a user, I like a review and comment on it; the author sees counts grow.
4. As a user, I report a review or comment (spam, ofensivo, spoilers sin marcar, otro).
5. As a user, I get simple follow suggestions ("a quién seguir").

## Domain model

- **Follow**: `follower → followee`, unique pair, no self-follow. Counts are a read model.
- **Like**: `user × target`, where `target = (type: review | list, id)`. Unique per pair,
  idempotent add/remove. (Lists join in Phase 7; the model is ready.)
- **Comment**: on reviews only in v1. `user`, `review_id`, `text`, `created_at`.
  Flat thread (no nesting) — matches mockup.
- **Report**: `reporter`, `target (review | comment)`, `reason (spam | offensive |
  unmarked_spoilers | other)`, `note?`, `status (open | dismissed | actioned)`.
  Multiple reports on one target collapse into one queue item with a counter.
- **Activity** (feed source): `actor`, `verb (logged | reviewed | rated | created_list)`,
  `object (title | review | list)`, `snapshot` (denormalized name/score/excerpt so the
  feed never joins other schemas), `at`.

## Feed strategy

**Fan-in (query time)** for v1: `SELECT activities WHERE actor IN (my followees) ORDER BY
at DESC` with keyset pagination. No per-user feed copies (fan-out) until scale demands it —
that upgrade is a stretch-goal ADR (Redis).

Activities are written by consuming domain events from other contexts:
`TitleLogged`, `ReviewPublished`, `RatingChanged` (review), `ListCreated` (lists, later).
Private diary entries never produce activities (flag travels in the event).

## Follow suggestions (v1, simple)

Users followed by people you follow (2nd degree), ranked by overlap; exclude already-followed.
No ML, one SQL query. Cold start: most-followed users.

## API surface (public API)

| Method | Path | Notes |
| --- | --- | --- |
| PUT / DELETE | `/users/{username}/follow` | Idempotent |
| GET | `/users/{username}/followers` · `/following` | Paginated |
| PUT / DELETE | `/reviews/{id}/like` | Idempotent |
| GET / POST | `/reviews/{id}/comments` | Flat, chronological |
| DELETE | `/comments/{id}` | Author (admin via admin-api) |
| POST | `/reports` | `{ targetType, targetId, reason, note? }` |
| GET | `/me/feed` | Keyset-paginated activities |
| GET | `/me/follow-suggestions` | Top N (mockup shows 2) |

Admin reads/resolves reports via admin-api (spec 05).

## Data (schema `social`)

`follows`, `likes` (user, target_type, target_id — unique), `comments`, `reports`,
`activities` (indexed `(actor, at desc)`), read models `follow_counts`, `like_counts`.

## Edge cases

- Self-follow → 422. Double like/follow → idempotent 200.
- Review deleted → cascade likes/comments; its activities removed; open reports auto-close.
- Comment on a spoiler review → comments hidden behind the same spoiler gate.
- Followee deletes account (identity event `UserDeleted`) → follows/activities purged.
- Report by the content's own author → rejected (422).
- Feed of a user who follows nobody → trending fallback ("popular esta semana").

## Non-goals (v1)

Nested comment threads, mentions/notifications, blocking users, activity privacy
settings beyond the diary `private` flag, fan-out feeds.

## Task breakdown (Phase 6)

1. Migrations `social` schema + sqlc.
2. Event consumers (outbox → activities) with idempotent handling (TDD).
3. Domain: follow/like/comment/report rules (TDD).
4. Feed query (fan-in, keyset) + suggestions query.
5. HTTP handlers vs OpenAPI; contract tests.
6. Web: home feed, follow buttons + counters, likes/comments on review page, report modal.
7. E2E: follow → friend logs a title → appears in my feed → like + comment.
