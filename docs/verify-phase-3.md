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
session — signing out of `/es/cuenta` clears `apps/web`'s cookie but not
Keycloak's, so clicking "Iniciar sesión" again silently re-authenticates
as whoever was last signed in instead of showing the login form. This is
`apps/web`-specific: clear cookies for `localhost` (or use a fresh/
incognito context) before signing in as a different user in this step.
(`apps/admin` doesn't have this problem — it sets `prompt: "login"` on the
Keycloak authorization request, via `AUTH_PROMPT_LOGIN`, specifically so
its access-denied screen can always offer a real way to switch accounts;
see step 4.)

Note this is *not* the same issue as `apps/web` and `apps/admin` sharing a
session — they don't: each app has its own `AUTH_SECRET` and its own
session cookie name (from `AUTH_COOKIE_PREFIX`: `questlog-web.session-token` /
`questlog-admin.session-token`), so signing into one no longer affects the
other's session at all. See step 5.

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
   administrador. Cierra sesión para entrar con otra cuenta." — not the
   portal.
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
   # Note the cookie name: apps/admin, not authjs.session-token.
   TOKEN=$(playwright-cli --raw cookie-get questlog-admin.session-token)  # or copy from devtools
   curl -s -H "Cookie: questlog-admin.session-token=$TOKEN" http://localhost:3001/es \
     | grep -o "max-w-2xl"
   ```
   `max-w-2xl` is the Home page's own className (`apps/admin/src/app/[locale]/page.tsx`),
   not used anywhere in the denied-gate markup — it must **not** appear.
   Also look for the page's flight-payload slot (grep for `5:null` or
   similar single-digit `id:null` pairs near the top of the trailing
   `<script>` blocks) — that null is `Home()`'s own early return, proving
   the page component ran and produced nothing, rather than the router
   simply never invoking it.
5. **Without manually clearing cookies**, click "Cerrar sesión" on the
   denied screen. You land back on the "Acceso administrativo" sign-in
   gate. Click "Iniciar sesión" again: Keycloak must show a credentials
   form, not silently re-authenticate you as `quest_user` again, even
   though Keycloak's own SSO cookie for `quest_user` is still live — this
   is `AUTH_PROMPT_LOGIN` doing its job, and it's the fix for the denial
   screen previously being a dead end.

   Because Keycloak's SSO session survives an app-local sign-out (see
   PLAN.md's "admin sign-out doesn't terminate the Keycloak SSO session"),
   what `prompt=login` gets you is Keycloak's *re-authenticate* form:
   "Please re-authenticate to continue", with the username field locked to
   `quest_user`. To switch accounts, click **"Restart login"** next to that
   field — the form clears and accepts any username. Two clicks, no cookie
   surgery. Sign in as `quest_admin` / `adminpass1`.
6. Confirm the portal content renders (the existing home Panel, heading
   "QuestLog Admin", tagline "Moderación, usuarios y curación de
   catálogo.").

## 5. Independent sessions (web vs admin)

Before the Phase 3 fix wave, `apps/web` and `apps/admin` shared one
session cookie name and one `AUTH_SECRET`, so signing into either app
silently overwrote the other's session — the admin portal would even
admit a session established purely against the `questlog-web` Keycloak
client. Confirm that's fixed:

1. Continuing from step 4 above (signed in to `apps/admin` as
   `quest_admin`, same browser), open http://localhost:3000/es/cuenta.
2. Expect "Aún no iniciaste sesión." — **not** a session for
   `quest_admin`. `apps/web` must not see `apps/admin`'s session.
3. Sign in on `apps/web` as `quest_user` / `questpass1`. Confirm
   `/es/cuenta` now shows that session.
4. Reload http://localhost:3001/es. Confirm `apps/admin` still shows
   `quest_admin`'s portal — the new `apps/web` sign-in must not have
   touched it.

## 6. Go JWT middleware, directly

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

# quest_user, but a token minted via the questlog-admin client (azp is
# allowed for admin-api; only the realm role is missing).
USER_VIA_ADMIN_CLIENT_TOKEN=$(curl -s -d "grant_type=password" \
  -d "client_id=questlog-admin" -d "client_secret=questlog-admin-dev-secret" \
  -d "username=quest_user" -d "password=questpass1" \
  http://localhost:8082/realms/questlog/protocol/openid-connect/token \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

curl -i -H "Authorization: Bearer $USER_VIA_ADMIN_CLIENT_TOKEN" http://localhost:8081/admin/whoami
# expect: 403 (azp=questlog-admin is allowed; quest_user just lacks the
# admin role — this is authmw.RequireRole rejecting it, not the azp check)

# quest_user, token minted via the questlog-web client — a valid,
# same-realm, unexpired token, but from a client outside admin-api's
# KEYCLOAK_ALLOWED_AZP allow-list (admin-api only trusts questlog-admin).
USER_TOKEN=$(curl -s -d "grant_type=password" \
  -d "client_id=questlog-web" -d "client_secret=questlog-web-dev-secret" \
  -d "username=quest_user" -d "password=questpass1" \
  http://localhost:8082/realms/questlog/protocol/openid-connect/token \
  | python3 -c "import sys,json; print(json.load(sys.stdin)['access_token'])")

curl -i -H "Authorization: Bearer $USER_TOKEN" http://localhost:8081/admin/whoami
# expect: 401 (azp=questlog-web is not in admin-api's allow-list — rejected
# by authmw.RequireAuth itself, before role is even checked)

curl -i -H "Authorization: Bearer $USER_TOKEN" -X POST http://localhost:8080/auth/sync
# expect: 200 — public-api's allow-list is questlog-web,questlog-admin, and
# any authenticated user can sync, not just admins
```

## 7. Automated coverage

```bash
cd backend
go test ./internal/identity/... ./internal/shared/... -race
# includes an integration test against the real dockerized Postgres:
DATABASE_URL=postgres://questlog:questlog@localhost:5432/questlog?sslmode=disable \
  go test ./internal/identity/infrastructure/... -race
```

Expected: all green.
