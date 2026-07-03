import type { ApiEvent } from "./types";

/**
 * Recomputes an event's hash via Web Crypto SHA-256, mirroring
 * internal/audit/events.go computeHash byte-for-byte:
 *
 *   chainID || tenantID || canonical(occurred-at) || sorted(actor k||v)
 *     || action || sorted(resource k||v) || sorted(metadata k||v)
 *     || prevHash (raw bytes, not the base64 text)
 *
 * Returns the base64-encoded digest, matching how Go marshals []byte.
 */
export async function hashEvent(_event: ApiEvent): Promise<string> {
  throw new Error("not implemented");
}
