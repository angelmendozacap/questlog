import type { NextAuthConfig } from "next-auth";
import Keycloak from "next-auth/providers/keycloak";

const PUBLIC_ISSUER = requireEnv("AUTH_KEYCLOAK_ISSUER");
const INTERNAL_ISSUER =
  process.env.AUTH_KEYCLOAK_INTERNAL_ISSUER ?? PUBLIC_ISSUER;
const IDENTITY_SYNC_URL = requireEnv("IDENTITY_SYNC_URL");

// Required and app-specific (set per-app in docker-compose.yml /
// .env.example) — this deliberately has no default. Without it, web and
// admin both land on Auth.js's default `authjs.*` cookie names, and
// cookies aren't port-scoped, so localhost:3000 and localhost:3001 share
// one cookie jar. Worse, Auth.js derives its JWE encryption key via
// hkdf(secret, salt=cookieName), so an identical secret *and* identical
// cookie name means each app also decrypts the other's session token as
// valid — signing into either app silently hijacks the other's session. A
// distinct AUTH_SECRET per app (also set in docker-compose.yml) closes the
// same hole from the other direction; both are set, belt and suspenders.
const COOKIE_PREFIX = requireEnv("AUTH_COOKIE_PREFIX");

// Every cookie Auth.js sets, not just the session one. The session cookie
// is the security-critical case, but the short-lived flow cookies collide
// too: with one shared `authjs.pkce.code_verifier`, starting a sign-in on
// web and then on admin before finishing the first makes the second
// overwrite the first's verifier, and the earlier callback fails. It fails
// closed and a retry works, so it's an annoyance rather than a hole — but
// it's the same "one cookie jar" root cause, so it's fixed the same way.
//
// Note these are literal names, which forfeits the `__Secure-` prefix
// Auth.js would otherwise apply over HTTPS. The `secure` attribute itself
// survives (Auth.js deep-merges these overrides onto its defaults); only
// the prefix's browser-enforced guarantee is lost. Irrelevant on localhost
// — revisit when there's a real HTTPS deployment.
const cookieNames = {
  sessionToken: { name: `${COOKIE_PREFIX}.session-token` },
  callbackUrl: { name: `${COOKIE_PREFIX}.callback-url` },
  csrfToken: { name: `${COOKIE_PREFIX}.csrf-token` },
  pkceCodeVerifier: { name: `${COOKIE_PREFIX}.pkce.code_verifier` },
  state: { name: `${COOKIE_PREFIX}.state` },
  nonce: { name: `${COOKIE_PREFIX}.nonce` },
};

// Optional, app-specific. Keycloak keeps its own SSO session independent
// of this app's session cookie, so signing out locally and clicking
// "sign in" again normally re-authenticates silently as whoever was last
// signed in, with no way to switch accounts short of clearing cookies by
// hand. That's merely inconvenient on apps/web, but it's a dead end on
// apps/admin's access-denied screen, where switching accounts is the
// entire point (see docs/adr/0001-keycloak-docker-network-split.md's
// sibling ADR context and PLAN.md's Phase 3 tracked gaps). Setting this
// forces Keycloak to always render the login form instead of reusing its
// SSO cookie. It's the cheap fix, not full federated (Keycloak-session)
// logout — see PLAN.md for the trade-off.
const PROMPT_LOGIN = process.env.AUTH_PROMPT_LOGIN === "true";

/**
 * Shared NextAuth config for both apps/web and apps/admin. Each app sets
 * its own AUTH_KEYCLOAK_ID / AUTH_KEYCLOAK_SECRET env vars (a different
 * Keycloak client per app) — next-auth's Keycloak provider infers those
 * two from env automatically, so this config only overrides the four
 * endpoints explicitly.
 *
 * Why two issuer URLs: see docs/adr/0001-keycloak-docker-network-split.md.
 */
export const authConfig: NextAuthConfig = {
  providers: [
    Keycloak({
      issuer: PUBLIC_ISSUER,
      authorization: {
        url: `${PUBLIC_ISSUER}/protocol/openid-connect/auth`,
        params: PROMPT_LOGIN ? { prompt: "login" } : {},
      },
      token: `${INTERNAL_ISSUER}/protocol/openid-connect/token`,
      userinfo: `${INTERNAL_ISSUER}/protocol/openid-connect/userinfo`,
      jwks_endpoint: `${INTERNAL_ISSUER}/protocol/openid-connect/certs`,
    }),
  ],
  session: { strategy: "jwt" },
  cookies: cookieNames,
  callbacks: {
    async jwt({ token, account }) {
      if (account?.access_token) {
        // Kept on the JWT (server-only) so a server component can still
        // reach it via auth() later if something needs it — see the
        // session callback below for why it does NOT also go into the
        // client-visible session.
        token.accessToken = account.access_token;
        token.roles = decodeRealmRoles(account.access_token);
        await syncProfile(account.access_token);
      }
      return token;
    },
    async session({ session, token }) {
      // Deliberately NOT copying token.accessToken onto session: whatever
      // this callback returns is served verbatim by GET /api/auth/session
      // to client-side JS, which would push the raw Keycloak bearer token
      // across the httpOnly cookie boundary for zero benefit — nothing
      // client-side consumes it today, and it's dead within Keycloak's
      // ~5-minute access-token lifetime anyway (this session is JWT-backed
      // with no refresh, see PLAN.md's Phase 3 tracked gaps). Anything
      // that eventually needs it server-side can still reach it via
      // auth() → the JWT, which does keep it.
      session.roles = (token.roles as string[] | undefined) ?? [];
      return session;
    },
  },
};

/**
 * Decodes the access token's realm roles. This is a plain base64url decode
 * with no signature verification — that's deliberate, not an oversight.
 * The token was just returned by Keycloak's `token` endpoint over a direct
 * server-to-server call (see ADR-0001), so it hasn't crossed an untrusted
 * boundary here. Re-verifying it would mean fetching JWKS in Next.js just
 * to re-check a token we received directly from the issuer ourselves.
 *
 * The Go backend, by contrast, *does* verify signatures (internal/shared/authmw)
 * because tokens reach it from browsers as Bearer headers — an untrusted boundary.
 */
function decodeRealmRoles(accessToken: string): string[] {
  const payload = accessToken.split(".")[1];
  if (!payload) return [];
  const json = JSON.parse(
    Buffer.from(payload, "base64url").toString("utf8"),
  ) as { realm_access?: { roles?: string[] } };
  return json.realm_access?.roles ?? [];
}

// How long the jwt callback (and therefore sign-in) will wait on
// public-api before giving up on the identity sync. Without an explicit
// timeout, an unreachable-but-TCP-accepting public-api blocks login for
// undici's default header timeout — login latency shouldn't be coupled to
// the identity API's health any more than this.
const SYNC_TIMEOUT_MS = 5_000;

/**
 * Syncs the local identity profile — see backend/internal/identity.
 *
 * Best-effort: failures are logged and swallowed rather than blocking
 * sign-in, so a down public-api/Postgres doesn't lock users out. Known
 * limitation: if this fails, nothing retries it — see PLAN.md's Phase 3
 * tracked gaps.
 */
async function syncProfile(accessToken: string): Promise<void> {
  try {
    const res = await fetch(IDENTITY_SYNC_URL, {
      method: "POST",
      headers: { Authorization: `Bearer ${accessToken}` },
      signal: AbortSignal.timeout(SYNC_TIMEOUT_MS),
    });
    if (!res.ok) {
      console.error(`identity sync failed: ${res.status} ${res.statusText}`);
    }
  } catch (err) {
    console.error("identity sync failed:", err);
  }
}

function requireEnv(name: string): string {
  const value = process.env[name];
  if (!value) throw new Error(`missing required env var ${name}`);
  return value;
}
