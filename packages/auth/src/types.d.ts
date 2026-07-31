import type { DefaultSession } from "next-auth";

declare module "next-auth" {
  interface Session extends DefaultSession {
    // No accessToken here on purpose — this shape is what
    // GET /api/auth/session serves to client JS. The raw Keycloak bearer
    // token stays on the JWT only (see the JWT augmentation below and
    // config.ts's session callback).
    roles?: string[];
  }
}

declare module "next-auth/jwt" {
  interface JWT {
    accessToken?: string;
    roles?: string[];
  }
}
