import { useCallback, useEffect, useState } from "react";
import type { ReactNode } from "react";
import { exportEvents } from "../../api/events";
import { groupByChain } from "../../lib/verify/groupByChain";
import { verifyChain } from "../../lib/verify/verifyChain";
import type { ApiEvent, VerifyResult } from "../../lib/verify/types";
import { ChainCard } from "./ChainCard";

interface EventViewerProps {
  token: string;
}

interface VerifiedGroup {
  chainId: string;
  events: ApiEvent[];
  result: VerifyResult;
}

function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : "Failed to load events";
}

/**
 * Fetches this tenant's export, groups events into per-chain cards, and
 * verifies each chain client-side (lib/verify). Rendering is explicit and
 * inspection-first: an explicit Refresh button re-fetches (no auto-poll,
 * no background revalidation), and there is deliberately NO search or
 * aggregation UI — the only slice available is the API's own `blockID`
 * filter (see CONTEXT.md's prove-it/explore-it scope guard).
 */
export function EventViewer({ token }: EventViewerProps): ReactNode {
  const [groups, setGroups] = useState<VerifiedGroup[] | null>(null);
  const [error, setError] = useState<string | null>(null);

  const load = useCallback(async () => {
    setError(null);
    try {
      const events = await exportEvents(token);
      const chainGroups = groupByChain(events);
      const verified = await Promise.all(
        chainGroups.map(async (group) => ({
          chainId: group.chainId,
          events: group.events,
          result: await verifyChain(group.events),
        })),
      );
      setGroups(verified);
    } catch (caught) {
      setError(errorMessage(caught));
    }
  }, [token]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <section className="event-viewer">
      <div className="event-viewer__toolbar">
        <button type="button" onClick={() => void load()}>
          Refresh
        </button>
      </div>

      {error && (
        <p className="event-viewer__error" role="alert">
          {error}
        </p>
      )}

      {groups && groups.length === 0 && (
        <p className="event-viewer__empty">
          No events for this tenant yet. Run make load-events from the repo
          root to post a sample batch, then Refresh — writes are
          asynchronous, so a Refresh immediately after posting may still show
          nothing until the next flush.
        </p>
      )}

      {groups?.map((group) => (
        <ChainCard
          key={group.chainId}
          chainId={group.chainId}
          events={group.events}
          result={group.result}
        />
      ))}
    </section>
  );
}
