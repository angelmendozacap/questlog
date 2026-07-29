import type { NextAuthConfig } from "next-auth";
import Keycloak from "next-auth/providers/keycloak";

const PUBLIC_ISSUER = requireEnv("AUTH_KEYCLOAK_ISSUER");
const INTERNAL_ISSUER =
  process.env.AUTH_KEYCLOAK_INTERNAL_ISSUER ?? PUBLIC_ISSUER;
const IDENTITY_SYNC_URL = requireEnv("IDENTITY_SYNC_URL");

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
      authorization: `${PUBLIC_ISSUER}/protocol/openid-connect/auth`,
      token: `${INTERNAL_ISSUER}/protocol/openid-connect/token`,
      userinfo: `${INTERNAL_ISSUER}/protocol/openid-connect/userinfo`,
      jwks_endpoint: `${INTERNAL_ISSUER}/protocol/openid-connect/certs`,
    }),
  ],
  session: { strategy: "jwt" },
  callbacks: {
    async jwt({ token, account }) {
      if (account?.access_token) {
        token.accessToken = account.access_token;
        token.roles = decodeRealmRoles(account.access_token);
        await syncProfile(account.access_token);
      }
      return token;
    },
    async session({ session, token }) {
      session.accessToken = token.accessToken as string | undefined;
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

/** Syncs the local identity profile — see backend/internal/identity. */
async function syncProfile(accessToken: string): Promise<void> {
  try {
    const res = await fetch(IDENTITY_SYNC_URL, {
      method: "POST",
      headers: { Authorization: `Bearer ${accessToken}` },
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
