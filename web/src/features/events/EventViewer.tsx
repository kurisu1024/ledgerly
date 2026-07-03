import type { ReactNode } from "react";
interface EventViewerProps {
  token: string;
}

/**
 * Fetches this tenant's export, groups events into per-chain cards, and
 * verifies each chain client-side (lib/verify). Rendering is explicit and
 * inspection-first: an explicit Refresh button re-fetches (no auto-poll,
 * no background revalidation), and there is deliberately NO search or
 * aggregation UI — the only slice available is the API's own `blockID`
 * filter (see CONTEXT.md's prove-it/explore-it scope guard).
 */
export function EventViewer(_props: EventViewerProps): ReactNode {
  throw new Error("not implemented");
}
