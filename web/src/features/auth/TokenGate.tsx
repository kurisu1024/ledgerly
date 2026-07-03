import { useState } from "react";
import type { FormEvent, ReactNode } from "react";
import { decodeJwtPayload } from "../../lib/jwt";

export const TOKEN_STORAGE_KEY = "ledgerly.token";

export interface TokenGateSession {
  token: string;
  tenantId: string;
}

interface TokenGateProps {
  /** Rendered once a well-formed token with a tenant_id claim is present. */
  children: (session: TokenGateSession) => ReactNode;
}

function sessionFromStoredToken(token: string): TokenGateSession | null {
  const decoded = decodeJwtPayload(token);
  return decoded.ok ? { token, tenantId: decoded.tenantId } : null;
}

/**
 * Gates the app behind a pasted dev JWT. Persists the raw token to
 * sessionStorage (NEVER localStorage — cleared on tab close, not
 * accessible across tabs, smaller XSS blast radius for a bearer token).
 * Decoding is display-only (lib/jwt.ts); the server remains the sole
 * authority on token validity.
 */
export function TokenGate({ children }: TokenGateProps): ReactNode {
  const [session, setSession] = useState<TokenGateSession | null>(() => {
    const stored = sessionStorage.getItem(TOKEN_STORAGE_KEY);
    return stored ? sessionFromStoredToken(stored) : null;
  });
  const [draft, setDraft] = useState("");
  const [error, setError] = useState<string | null>(null);

  if (session) {
    return <>{children(session)}</>;
  }

  function handleSubmit(event: FormEvent<HTMLFormElement>): void {
    event.preventDefault();
    const token = draft.trim();
    const decoded = decodeJwtPayload(token);

    if (!decoded.ok) {
      setError(`Invalid token — ${decoded.error}`);
      return;
    }

    sessionStorage.setItem(TOKEN_STORAGE_KEY, token);
    setError(null);
    setSession({ token, tenantId: decoded.tenantId });
  }

  return (
    <form className="token-gate" onSubmit={handleSubmit}>
      <label className="token-gate__label" htmlFor="token-gate-input">
        Auth token
      </label>
      <input
        id="token-gate-input"
        className="token-gate__input"
        type="text"
        autoComplete="off"
        spellCheck={false}
        value={draft}
        onChange={(event) => setDraft(event.target.value)}
      />
      {error && (
        <p className="token-gate__error" role="alert">
          {error}
        </p>
      )}
      <button type="submit" className="token-gate__submit">
        Continue
      </button>
    </form>
  );
}
