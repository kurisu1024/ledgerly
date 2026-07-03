/**
 * Wire-format event shape returned by GET /v1/export and accepted by
 * POST /v1/events. Mirrors api/events.Event in the Go server: kebab-case
 * JSON keys, hashes as base64 strings (Go marshals []byte as base64), and
 * `occurred-at` kept as the RAW wire string (never parsed through Date —
 * see canonicalTime.ts for why).
 */
export interface ApiEvent {
  id: string;
  "chain-id": string;
  "tenant-id": string;
  "occurred-at": string;
  action: string;
  actor: Record<string, string>;
  resource: Record<string, string>;
  metadata?: Record<string, string>;
  "prev-hash": string;
  "event-hash": string;
}

/**
 * Discriminated union mirroring internal/audit.VerifyChain, refined so the
 * UI can show *where* and *why* a chain breaks. Go's VerifyChain collapses
 * chain-id and tenant-id mismatches into a single boolean condition; the
 * client separates them into distinct reasons for clearer forensic display.
 */
export type VerifyResult =
  | { status: "verified"; length: number }
  | {
      status: "tampered";
      failedIndex: number;
      reason:
        | "hash-mismatch"
        | "link-broken"
        | "foreign-chain"
        | "tenant-mixed"
        | "malformed";
    }
  | { status: "unverifiable"; reason: "empty" };

export interface ChainGroup {
  chainId: string;
  events: ApiEvent[];
}
