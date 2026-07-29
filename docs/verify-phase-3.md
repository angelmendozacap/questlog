# Verifying Phase 3 — Auth & identity

Manual/scripted verification for this phase's acceptance criteria (PLAN.md).
Full browser E2E automation (Playwright) is Phase 9 scope — this is a
repeatable checklist to run against the real compose stack instead.

## Setup

Start from a clean slate — Keycloak's own state is ephemeral (no volume; the
realm is re-imported on every `up`), but Postgres has a persistent volume, so
a stale stack can leave `identity.user_profiles` rows pointing at Keycloak
user IDs that no longer exist. `down -v` avoids that:

```bash
docker compose -f deploy/docker-compose.yml down -v
docker compose -f deploy/docker-compose.yml up --build -d
```

Wait for all services healthy:

```bash
docker compose -f deploy/docker-compose.yml ps
```

`keycloak` must show `healthy` before `public-api`/`admin-api` will even
start — both depend on `keycloak: condition: service_healthy`, because both
do a synchronous initial JWKS fetch on boot and `log.Fatalf` if it fails.
Cold builds take several minutes; keep polling `ps` rather than assuming a
fixed sleep is enough.

## 1. Realm imported correctly

Open http://localhost:8082/admin (admin/admin) → realm switcher → `questlog`.
Confirm: roles `user` and `admin` exist; clients `questlog-web` and
`questlog-admin` exist; users `quest_user` and `quest_admin` exist.

## 2. Signup → login → authenticated page (web)

1. Open http://localhost:3000/es/cuenta — should show "Aún no iniciaste
   sesión." with an "Iniciar sesión" button.
2. Click it → redirected to Keycloak's hosted login page.
3. Click "Register" → create a new account (any username/password).
4. Redirected back to `/es/cuenta`, now showing "Sesión iniciada como
   <first> <last>" (the name fields from the registration form) and a
   "Roles:" line. **Expect no `user` realm role here** — self-registration
   only grants Keycloak's own technical roles (observed:
   `offline_access, default-roles-questlog, uma_authorization`). The `user`
   role is not auto-assigned on signup in the current realm export, even
   though `deploy/keycloak/questlog-realm.json`'s `defaultRole.composites`
   lists it — querying the realm's live `default-roles-questlog` role via
   the admin API shows only Keycloak's built-ins (`manage-account`,
   `uma_authorization`, `view-profile`, `offline_access`), not `user`. This
   doesn't block any Phase 3 acceptance criterion (nothing gates on a
   regular user having the `user` role), but don't expect "Roles: user"
   here — use the seeded `quest_user` account in step 3 to check that.
5. `docker compose -f deploy/docker-compose.yml logs public-api` should show
   no new log lines at all after the request (there's no request logger
   middleware, so absence of `log.Fatalf`/error output is the only signal
   here). For positive confirmation the sync landed, query Postgres
   directly:
   ```bash
   docker compose -f deploy/docker-compose.yml exec -T postgres \
     psql -U questlog -d questlog -c \
     "SELECT username, keycloak_id FROM identity.user_profiles;"
   ```
   Expect a row for the username you just registered. If you're re-running
   this against a stack that wasn't started with `down -v`, rows from a
   previous Keycloak instance can still be present with `keycloak_id`
   values that no longer exist in the current realm — Keycloak's own state
   is ephemeral and re-imported every `up`, but Postgres has a persistent
   volume. Match on the username you just created, not on row count.

## 3. Login with a seeded user

Keycloak keeps its own SSO session independent of the Next.js app's
session — signing out of `/es/cuenta` clears the app's cookie but not
Keycloak's, so clicking "Iniciar sesión" again silently re-authenticates
as whoever was last signed in instead of showing the login form. Clear
cookies for `localhost` (or use a fresh/incognito context) before signing
in as a different user in this step and in step 4.

1. Sign out from `/es/cuenta` ("Cerrar sesión"), then clear cookies.
2. Sign in again with `quest_user` / `questpass1`.
3. Confirm the page shows "Sesión iniciada como Quest User" and "Roles:
   user" (no "admin"). Seeded users have realm roles assigned directly
   (not just via the default composite), which is why this one does show
   `user` where the self-registered account in step 2 didn't.

## 4. Admin login → admin portal

1. Open http://localhost:3001/es — should show the "Acceso administrativo"
   sign-in gate, not the portal.
2. Sign in with `quest_user` / `questpass1` (a non-admin user).
3. Confirm you see "Acceso denegado" / "Tu cuenta no tiene el rol de
   administrador." — not the portal.
4. **Check the raw response body, not just the rendered page.** The layout's
   role gate controls what's *displayed*, but `page.tsx` is always invoked by
   the App Router to build the layout's `children` prop, even when the
   layout discards it — a page that doesn't also gate itself can still leak
   its markup into the RSC flight payload as an inert, unmounted chunk.
   `QuestLog Admin` / the tagline text are **not** good markers by
   themselves — they're also present unconditionally in `<title>` and the
   `<meta name="description">` tag, and next-intl embeds the *entire*
   messages bundle (including unused `home.*` keys) into the client
   provider's props regardless of which components render, so both strings
   legitimately appear even when nothing leaked. Instead:
   ```bash
   TOKEN=$(playwright-cli --raw cookie-get authjs.session-token)  # or copy from devtools
   curl -s -H "Cookie: authjs.session-token=$TOKEN" http://localhost:3001/es \
     | grep -o "max-w-2xl"
   ```
   `max-w-2xl` is the Home page's own className (`apps/admin/src/app/[locale]/page.tsx`),
   not used anywhere in the denied-gate markup — it must **not** appear.
   Also look for the page's flight-payload slot (grep for `5:null` or
   similar single-digit `id:null` pairs near the top of the trailing
   `<script>` blocks) — that null is `Home()`'s own early return, proving
   the page component ran and produced nothing, rather than the router
   simply never invoking it.
5. Sign out, clear cookies, sign in again with `quest_admin` / `adminpass1`.
6. Confirm the portal content renders (the existing home Panel, heading
   "QuestLog Admin", tagline "Moderación, usuarios y curación de
   catálogo.").

## 5. Go JWT middleware, directly

```bash
curl -i http://localhost:8081/admin/whoami
# expect: 401 (no bearer token)

curl -i -H "Authorization: Bearer not-a-real-token" http://localhost:8081/admin/whoami
# expect: 401 (invalid token)

# Real tokens via Keycloak's direct grant (enabled for local dev
# convenience only — see deploy/keycloak/questlog-realm.json). Note the
# token endpoint uses localhost:8082, the browser-facing host — the same
# host the middleware validates `iss` against. Key *fetching* happens
# server-side over the compose network (keycloak:8080) and isn't something
# you interact with directly; see docs/adr/0001-keycloak-docker-network-split.md
# if the two different Keycloak hosts in this file and in docker-compose.yml
# look inconsistent — they're deliberate.
ADMIN_TOKEN=$(curl -s -d "grant_type=password" \
  -d "client_id=questlog-admin" -d "client_secret=questlog-admin-dev-secret" \
  -d "username=quest_admin" -d "password=adminpass1" \
  http://localhost:8082/realms/questlog/protocol/openid-connect/token \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

curl -i -H "Authorization: Bearer $ADMIN_TOKEN" http://localhost:8081/admin/whoami
# expect: 200 {"username":"quest_admin","roles":[...,"admin","user",...]}

USER_TOKEN=$(curl -s -d "grant_type=password" \
  -d "client_id=questlog-web" -d "client_secret=questlog-web-dev-secret" \
  -d "username=quest_user" -d "password=questpass1" \
  http://localhost:8082/realms/questlog/protocol/openid-connect/token \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

curl -i -H "Authorization: Bearer $USER_TOKEN" http://localhost:8081/admin/whoami
# expect: 403 (quest_user lacks the admin role)

curl -i -H "Authorization: Bearer $USER_TOKEN" -X POST http://localhost:8080/auth/sync
# expect: 200 — any authenticated user can sync, not just admins
```

## 6. Automated coverage

```bash
cd backend
go test ./internal/identity/... ./internal/shared/... -race
# includes an integration test against the real dockerized Postgres:
DATABASE_URL=postgres://questlog:questlog@localhost:5432/questlog?sslmode=disable \
  go test ./internal/identity/infrastructure/... -race
```

Expected: all green.
