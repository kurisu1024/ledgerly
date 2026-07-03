import type { ReactNode } from "react";

export const TOKEN_STORAGE_KEY = "ledgerly.token";

export interface TokenGateSession {
  token: string;
  tenantId: string;
}

interface TokenGateProps {
  /** Rendered once a well-formed token with a tenant_id claim is present. */
  children: (session: TokenGateSession) => ReactNode;
}

/**
 * Gates the app behind a pasted dev JWT. Persists the raw token to
 * sessionStorage (NEVER localStorage — cleared on tab close, not
 * accessible across tabs, smaller XSS blast radius for a bearer token).
 * Decoding is display-only (lib/jwt.ts); the server remains the sole
 * authority on token validity.
 */
export function TokenGate(_props: TokenGateProps): ReactNode {
  throw new Error("not implemented");
}
