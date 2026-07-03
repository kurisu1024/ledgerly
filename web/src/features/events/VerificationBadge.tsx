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
export function VerificationBadge(_props: VerificationBadgeProps): ReactNode {
  throw new Error("not implemented");
}
