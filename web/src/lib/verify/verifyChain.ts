import type { ApiEvent, VerifyResult } from "./types";

/**
 * Client-side mirror of internal/audit/events.go VerifyChain, refined into a
 * discriminated result so the UI can show where a chain breaks. Anchored at
 * the same genesis hash (sha256("GENESIS")) Go uses for the first link.
 *
 * Limitation carried over from Go: this cannot detect tail truncation — a
 * valid prefix of a chain still verifies. Any consumer of "verified" results
 * must carry that caveat in its copy.
 */
export async function verifyChain(_events: ApiEvent[]): Promise<VerifyResult> {
  throw new Error("not implemented");
}
