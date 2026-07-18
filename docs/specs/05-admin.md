# Spec 05 — Admin

**Status:** draft (pending approval)
**Context:** cross-cutting — the `admin-api` binary (`:8081`) registers admin handlers
**owned by each context** (catalog, review, social, identity) + a small `admin` package
for the audit log. No "admin god-context".
**UI source of truth:** `v-admin.html`
**Build phase:** 8 (curation endpoints land earlier with spec 01)

## Problem

Moderators need to act on reported content, manage users, and curate the catalog —
from a separate app (`apps/admin`, `:3001`, amber accent) restricted to the `admin`
role, with every action audited.

## Access

Keycloak role `admin` required by the whole admin-api (JWT middleware rejects others).
The admin web app refuses login without the role. No granular roles in v1.

## Capabilities

### Moderation (reports come from social, spec 03)

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/admin/reports?status=open` | Grouped per target with reporter count |
| POST | `/admin/reports/{id}/dismiss` | Closes group, content stays |
| POST | `/admin/reports/{id}/remove-content` | Deletes review/comment **via the owning context's service** (same cascades as author deletion), closes group |

### Users (identity context)

| Method | Path | Notes |
| --- | --- | --- |
| GET | `/admin/users?q=` | Search by username/email, report counts |
| POST | `/admin/users/{id}/suspend` | Local flag **+ disables the user in Keycloak** (Admin REST API) so sessions/logins stop; content stays visible |
| POST | `/admin/users/{id}/reinstate` | Reverses both |

### Catalog curation

Defined in spec 01 (`PATCH /admin/titles/{id}`, merge, issues queue).

### Dashboard

`GET /admin/stats` — the four tiles (open reports, active users, titles, reviews),
served from existing read models; no new aggregation infra.

## Audit log (schema `admin`)

Every mutating admin action appends `audit_log`: `admin_id`, `action`, `target`,
`reason?`, `at`. Append-only (no UPDATE/DELETE grants). Shown later as a stretch
("recent admin activity" panel).

## Edge cases

- Report group already resolved → 409.
- Content deleted (by author) before action → report group auto-closes (spec 03), acting on it → 410.
- Suspending a user with the `admin` role → 422 (demote first, via Keycloak console).
- Keycloak unreachable during suspend → whole action fails (no local-only suspension: avoid split-brain).
- Suspended user's JWT still valid (not expired) → public-api checks the local flag on
  writes; reads are acceptable until expiry (documented trade-off, short token TTL).

## Non-goals (v1)

Granular roles/permissions, bulk moderation, user-facing appeal flow, email
notifications, content edit by admins (remove or keep, never rewrite).

## Task breakdown (Phase 8)

1. `admin` schema (audit_log) + middleware wiring in admin-api.
2. Moderation endpoints delegating to owning contexts (TDD on the delegation seams).
3. User suspension: identity flag + Keycloak Admin API client (infra, mocked in tests).
4. Stats endpoint over read models.
5. Admin web: queue, user search, dashboard tiles (mockup `v-admin.html`).
6. E2E: report (as user) → appears in queue → remove content → gone from public app; audit row written.
