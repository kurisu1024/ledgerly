import type { ApiEvent } from "./types";
import { canonicalTime } from "./canonicalTime";

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
export async function hashEvent(event: ApiEvent): Promise<string> {
  const encoder = new TextEncoder();
  const chunks: Uint8Array[] = [
    encoder.encode(event["chain-id"]),
    encoder.encode(event["tenant-id"]),
    encoder.encode(canonicalTime(event["occurred-at"])),
    ...sortedMapBytes(encoder, event.actor),
    encoder.encode(event.action),
    ...sortedMapBytes(encoder, event.resource),
    ...sortedMapBytes(encoder, event.metadata ?? {}),
    base64ToBytes(event["prev-hash"]),
  ];

  const digest = await crypto.subtle.digest("SHA-256", concatBytes(chunks));
  return bytesToBase64(new Uint8Array(digest));
}

// Mirrors Go's writeMapSorted: keys sorted ascending, each key's bytes
// followed immediately by its value's bytes, with no separators.
function sortedMapBytes(
  encoder: TextEncoder,
  map: Record<string, string>,
): Uint8Array[] {
  const chunks: Uint8Array[] = [];
  for (const key of Object.keys(map).sort()) {
    chunks.push(encoder.encode(key));
    chunks.push(encoder.encode(map[key]!));
  }
  return chunks;
}

function concatBytes(chunks: Uint8Array[]): Uint8Array<ArrayBuffer> {
  const total = chunks.reduce((sum, chunk) => sum + chunk.length, 0);
  const out = new Uint8Array(new ArrayBuffer(total));
  let offset = 0;
  for (const chunk of chunks) {
    out.set(chunk, offset);
    offset += chunk.length;
  }
  return out;
}

function base64ToBytes(base64: string): Uint8Array {
  const binary = atob(base64);
  const bytes = new Uint8Array(binary.length);
  for (let i = 0; i < binary.length; i++) {
    bytes[i] = binary.charCodeAt(i);
  }
  return bytes;
}

function bytesToBase64(bytes: Uint8Array): string {
  let binary = "";
  for (const byte of bytes) {
    binary += String.fromCharCode(byte);
  }
  return btoa(binary);
}
