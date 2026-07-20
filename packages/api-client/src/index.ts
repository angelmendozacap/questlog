import createClient from "openapi-fetch";
import type { paths } from "@questlog/types";

/** Typed fetch client over the OpenAPI contract (React Query hooks land in Phase 4). */
export function createApiClient(baseUrl: string) {
  return createClient<paths>({ baseUrl });
}
