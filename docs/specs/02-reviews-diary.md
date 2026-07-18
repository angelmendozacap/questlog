# Spec 02 — Reviews, Ratings & Diary

**Status:** draft (pending approval)
**Context:** `backend/internal/review`
**UI source of truth:** `f-hybrid-plata.html` ("Tu puntuación", reseñas, distribución), `v-resena.html` (reseña completa), `f-diario-plata.html` (diario privado)
**Build phase:** 5

## Problem

Users need to score titles, write reviews, and keep a dated log (diary) of what they
watch and play. The title pages need an aggregate score + distribution. Activity here
feeds the social feed (Phase 6) via domain events, without coupling the contexts.

## Decision: rating scale = 0–10 integers

Chosen over 5 stars with halves: matches gaming culture and both mockup components
(10 score pips, 10-segment XP bar, 10-bar histogram), maps 1:1 to the avatar moods
(cheer ≥9, idle 6–8, slump ≤5), and needs no half-star UI. Stored as `smallint 1..10`
(0 is not a valid score; absence of rating = null, "sin puntuar").

## Domain model

- **Rating**: `user × title → score (1..10)`. One current rating per user/title (upsert).
  Can exist without a review or diary entry ("quick rate" from any card).
- **DiaryEntry**: `user`, `title`, `date` (day precision), `rewatch: bool`,
  optional `rating` link, optional `review` link, optional metadata: `platform/venue`
  (free text: "PC", "cine", "en casa"), `hours` (games), `in_progress: bool` (games:
  "en curso", no score required while true). Multiple entries per title allowed (rewatches).
- **Review** (aggregate root): `user`, `title`, `text` (markdown-lite), `spoiler: bool`,
  `created/edited_at`. **One review per user per title** in v1 (editable; rewatch context
  comes from the diary, e.g. "3.ª vez que la ve"). A review always carries the user's
  current rating when rendered.
- **TitleStats** (read model, consumed by catalog pages): `title_id`, `avg`, `count`,
  `histogram[1..10]`. Recomputed transactionally on rating upsert/delete (cheap at v1
  scale; async is a later optimization).

**Domain events** (consumed by `social` in Phase 6): `TitleLogged`, `ReviewPublished`,
`RatingChanged`.

## Privacy

Logging activity is public by default (it feeds profiles and the social feed).
The diary **page** (goals, streaks, stats) is private — it's the user's management view.
Each diary entry has a `private: bool` (default `false`); private entries never appear
in feeds or the public profile.

## API surface (public API)

| Method | Path | Notes |
| --- | --- | --- |
| PUT | `/titles/{id}/my-rating` | `{ score }` upsert; DELETE removes |
| GET | `/titles/{id}/reviews` | Paginated; sort: top (likes) / recent; spoilers collapsed |
| POST | `/titles/{id}/reviews` | `{ text, spoiler }` → 201; 409 if user already has one |
| GET | `/reviews/{id}` | Full review (page 4 of mockups) |
| PATCH / DELETE | `/reviews/{id}` | Author only |
| GET | `/me/diary?year=&kind=` | Private; entries grouped by month + year stats |
| POST | `/diary/entries` | `{ titleId, date, rewatch?, rating?, reviewId?, platform?, hours?, inProgress?, private? }` |
| PATCH / DELETE | `/diary/entries/{id}` | Author only |
| GET | `/users/{username}/diary` | Public, non-private entries only (profile tab) |

Likes and comments on reviews belong to the **social** context (spec 03) — the review
page composes both APIs.

## Data (schema `review`)

`ratings` (unique user+title), `reviews` (unique user+title), `diary_entries`,
`title_stats`. FKs to `catalog.titles(id)` by id only (no cross-schema joins in queries
owned by other contexts).

## Edge cases

- Rating changed after review published → review shows current rating; `RatingChanged`
  emitted so feeds can stay coherent.
- Deleting a rating with a published review → allowed; review renders "sin puntuación".
- `in_progress` game entry finished later → user edits entry, sets date/hours/score.
- Review with `spoiler: true` → collapsed by default everywhere, "mostrar spoilers" action.
- Title merged by admin (`TitleMerged` from catalog) → ratings/reviews/entries repointed;
  duplicate user ratings on both titles resolved keeping the most recent.
- Aggregate with < 3 ratings → detail page shows count but no histogram (avoid outliers).

## Non-goals (v1)

Multiple reviews per title per user, per-episode series logging, half points,
review drafts, reactions beyond like (social owns likes).

## Task breakdown (Phase 5)

1. Migrations `review` schema + sqlc queries.
2. Domain: Rating upsert + TitleStats recompute (TDD), DiaryEntry rules, Review rules.
3. Domain events emitted via shared kernel outbox.
4. HTTP handlers vs OpenAPI; contract tests.
5. Web: "Tu puntuación" panel (avatar wired to score mood), review list + review page,
   diary page (private) + profile diary tab (public subset).
6. E2E: rate → review → diary entry → detail page reflects aggregate.
