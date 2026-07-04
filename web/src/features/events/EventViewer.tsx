import { useCallback, useEffect, useRef, useState } from "react";
import type { ReactNode } from "react";
import { exportEvents } from "../../api/events";
import { AuthError } from "../../api/types";
import { groupByChain } from "../../lib/verify/groupByChain";
import { verifyChain } from "../../lib/verify/verifyChain";
import type { ApiEvent, VerifyResult } from "../../lib/verify/types";
import { ChainCard } from "./ChainCard";

interface EventViewerProps {
  token: string;
  /**
   * Invoked when the user asks to enter a different token after an auth
   * failure — the parent (TokenGate session) clears the stored token and
   * returns to the paste form.
   */
  onResetSession?: () => void;
}

interface VerifiedGroup {
  chainId: string;
  events: ApiEvent[];
  result: VerifyResult;
}

interface ViewerError {
  message: string;
  isAuth: boolean;
}

function toViewerError(error: unknown): ViewerError {
  return {
    message: error instanceof Error ? error.message : "Failed to load events",
    isAuth: error instanceof AuthError,
  };
}

/**
 * Fetches this tenant's export, groups events into per-chain cards, and
 * verifies each chain client-side (lib/verify). Rendering is explicit and
 * inspection-first: an explicit Refresh button re-fetches (no auto-poll,
 * no background revalidation), and there is deliberately NO search or
 * aggregation UI — the only slice available is the API's own `blockID`
 * filter (see CONTEXT.md's prove-it/explore-it scope guard).
 *
 * Loads are guarded by a monotonic request id: only the most recently
 * started load may commit state, so a slower, older response can never
 * clobber a newer one.
 */
export function EventViewer({ token, onResetSession }: EventViewerProps): ReactNode {
  const [groups, setGroups] = useState<VerifiedGroup[] | null>(null);
  const [error, setError] = useState<ViewerError | null>(null);
  const [isLoading, setIsLoading] = useState(false);
  const requestIdRef = useRef(0);

  const load = useCallback(async () => {
    const requestId = ++requestIdRef.current;
    const isCurrent = () => requestId === requestIdRef.current;
    // A shown error stays visible until this retry settles.
    setIsLoading(true);
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
      if (!isCurrent()) return;
      setGroups(verified);
      setError(null);
    } catch (caught) {
      if (!isCurrent()) return;
      setError(toViewerError(caught));
    } finally {
      if (isCurrent()) setIsLoading(false);
    }
  }, [token]);

  useEffect(() => {
    void load();
    return () => {
      // Invalidate any in-flight load when the token changes or on unmount.
      requestIdRef.current++;
    };
  }, [load]);

  return (
    <section className="event-viewer">
      <div className="event-viewer__toolbar">
        <button type="button" onClick={() => void load()} disabled={isLoading}>
          Refresh
        </button>
      </div>

      {isLoading && (
        <p className="event-viewer__loading" role="status">
          Loading…
        </p>
      )}

      {error && (
        <div className="event-viewer__error-panel">
          <p className="event-viewer__error" role="alert">
            {error.message}
          </p>
          {error.isAuth && onResetSession && (
            <button type="button" onClick={onResetSession}>
              Use a different token
            </button>
          )}
        </div>
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
