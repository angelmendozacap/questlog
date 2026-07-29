// Triple-slash reference: pulls the `next-auth` / `next-auth/jwt` module
// augmentations in ./types.d.ts into any consumer's TS program. Without
// this, a file that isn't itself imported never gets loaded, so the
// `Session.roles` / `JWT.roles` shape wouldn't be visible outside this
// package (or even inside config.ts, once it's compiled as part of a
// different program root — e.g. apps/web's).
//
// This must be a triple-slash reference, not `import "./types.d.ts"`. This
// package's `exports` map points straight at `./src/index.ts` (no build
// step, no emitted `.d.ts`), so consumers like apps/web transpile this file
// for real via `transpilePackages` — and a runtime import of a `.d.ts` file
// breaks webpack (`Module parse failed` on `declare module`, since `.d.ts`
// has no runtime JS to emit). A triple-slash reference is still followed
// during TS program construction — so the augmentation still reaches
// consumers — but is inert to webpack, just an ordinary comment. Don't
// "tidy" this back into an import.
/// <reference path="./types.d.ts" />

export { authConfig } from "./config";

export const KEYCLOAK_REALM = "questlog";
