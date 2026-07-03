/**
 * Result of decoding a JWT's payload. This is a DISPLAY-ONLY decode — no
 * signature verification happens client-side; the server is the sole
 * authority on token validity. This exists purely so TokenGate can show
 * which tenant a pasted token claims to belong to.
 */
export type DecodeJwtResult =
  | { ok: true; tenantId: string }
  | { ok: false; error: string };

/**
 * Extracts the `tenant_id` claim (snake_case, matching api/http/auth.go's
 * JWTClaims) from an unsigned/undecoded JWT's payload segment. Never
 * throws — malformed input resolves to an { ok: false } result.
 */
export function decodeJwtPayload(token: string): DecodeJwtResult {
  const segments = token.split(".");
  if (segments.length < 3 || !segments[1]) {
    return {
      ok: false,
      error: "token does not have the expected header.payload.signature segment format",
    };
  }

  let payload: unknown;
  try {
    payload = JSON.parse(base64UrlDecode(segments[1]));
  } catch {
    return { ok: false, error: "token payload segment is not valid base64/JSON" };
  }

  if (
    typeof payload !== "object" ||
    payload === null ||
    typeof (payload as Record<string, unknown>).tenant_id !== "string"
  ) {
    return { ok: false, error: "token payload is missing a tenant_id claim" };
  }

  return { ok: true, tenantId: (payload as { tenant_id: string }).tenant_id };
}

function base64UrlDecode(segment: string): string {
  const base64 = segment.replace(/-/g, "+").replace(/_/g, "/");
  const paddingNeeded = (4 - (base64.length % 4)) % 4;
  return atob(base64.padEnd(base64.length + paddingNeeded, "="));
}
