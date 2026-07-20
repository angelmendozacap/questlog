/**
 * Shared NextAuth + Keycloak configuration (Phase 3).
 * Placeholder so both apps can already import from a stable path.
 */
export const KEYCLOAK_REALM = "questlog";

export function authPlaceholder(): string {
  return `auth wired in Phase 3 (realm: ${KEYCLOAK_REALM})`;
}
