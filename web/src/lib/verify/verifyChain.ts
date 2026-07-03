import type { ApiEvent, VerifyResult } from "./types";
import { hashEvent } from "./hashEvent";

// sha256("GENESIS"), base64-encoded — the fixed anchor internal/audit/events.go
// uses for the first link in any chain (see genesisHash in events.go).
const GENESIS_HASH = "kBEx2DixeqwPeIW4HgPL3J9RV6ADQ9MKsiCDaF7RQWo=";

/**
 * Client-side mirror of internal/audit/events.go VerifyChain, refined into a
 * discriminated result so the UI can show where a chain breaks. Anchored at
 * the same genesis hash (sha256("GENESIS")) Go uses for the first link.
 *
 * Limitation carried over from Go: this cannot detect tail truncation — a
 * valid prefix of a chain still verifies. Any consumer of "verified" results
 * must carry that caveat in its copy.
 */
export async function verifyChain(events: ApiEvent[]): Promise<VerifyResult> {
  if (events.length === 0) {
    return { status: "unverifiable", reason: "empty" };
  }

  const chainId = events[0]!["chain-id"];
  const tenantId = events[0]!["tenant-id"];
  let prevHash = GENESIS_HASH;

  for (let index = 0; index < events.length; index++) {
    const event = events[index]!;

    if (event["chain-id"] !== chainId) {
      return { status: "tampered", failedIndex: index, reason: "foreign-chain" };
    }
    if (event["tenant-id"] !== tenantId) {
      return { status: "tampered", failedIndex: index, reason: "tenant-mixed" };
    }
    if (event["prev-hash"] !== prevHash) {
      return { status: "tampered", failedIndex: index, reason: "link-broken" };
    }

    const recomputed = await hashEvent(event);
    if (recomputed !== event["event-hash"]) {
      return { status: "tampered", failedIndex: index, reason: "hash-mismatch" };
    }

    prevHash = event["event-hash"];
  }

  return { status: "verified", length: events.length };
}
