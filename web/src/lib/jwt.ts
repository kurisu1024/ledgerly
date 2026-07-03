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
export function decodeJwtPayload(_token: string): DecodeJwtResult {
  throw new Error("not implemented");
}
