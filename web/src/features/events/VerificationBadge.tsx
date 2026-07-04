import type { ReactNode } from "react";
import type { VerifyResult } from "../../lib/verify/types";

interface VerificationBadgeProps {
  result: VerifyResult;
}

/**
 * Status must be conveyed through text, not color alone (WCAG 1.4.1).
 * A "verified" badge always carries VerifyChain's tail-truncation caveat —
 * the client cannot detect a chain whose tail was silently dropped.
 */
export function VerificationBadge({ result }: VerificationBadgeProps): ReactNode {
  if (result.status === "verified") {
    return (
      <p
        className="verification-badge verification-badge--verified"
        data-status="verified"
      >
        Verified — {result.length} linked event{result.length === 1 ? "" : "s"}.
        Caveat: verification cannot detect tail truncation; a truncated chain
        with a valid prefix still verifies.
      </p>
    );
  }

  if (result.status === "tampered") {
    return (
      <p
        className="verification-badge verification-badge--tampered"
        data-status="tampered"
      >
        Tampered at event {result.failedIndex}: {result.reason}.
      </p>
    );
  }

  return (
    <p
      className="verification-badge verification-badge--unverifiable"
      data-status="unverifiable"
    >
      Unverifiable ({result.reason}): this chain has no events to check.
    </p>
  );
}
