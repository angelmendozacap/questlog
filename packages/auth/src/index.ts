// Side-effect import: pulls the `next-auth` / `next-auth/jwt` module
// augmentations in ./types.d.ts into any consumer's TS program. Without
// this, a file that isn't itself imported never gets loaded, so the
// `Session.roles` / `JWT.roles` shape wouldn't be visible outside this
// package (or even inside config.ts, once it's compiled as part of a
// different program root — e.g. apps/web's).
import "./types.d.ts";

export { authConfig } from "./config";

export const KEYCLOAK_REALM = "questlog";
