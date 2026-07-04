import type { ReactNode } from "react";
import type { ApiEvent, VerifyResult } from "../../lib/verify/types";
import { HashChip } from "./HashChip";
import { VerificationBadge } from "./VerificationBadge";

interface ChainCardProps {
  chainId: string;
  events: ApiEvent[];
  result: VerifyResult;
}

/**
 * One chain rendered as a literal vertical rail: each event is a row, and a
 * rupture (the first point verification failed) is marked distinctly so the
 * forensic evidence is visible at a glance, not just the pass/fail badge.
 */
export function ChainCard({ chainId, events, result }: ChainCardProps): ReactNode {
  const failedIndex = result.status === "tampered" ? result.failedIndex : null;

  return (
    <section className="chain-card" data-testid="chain-card">
      <header className="chain-card__header">
        <h2 className="chain-card__title">
          Chain <code className="chain-card__id">{chainId}</code>
        </h2>
        <VerificationBadge result={result} />
      </header>
      <ol className="chain-card__rail">
        {events.map((event, index) => {
          const isRupture = index === failedIndex;
          return (
            <li
              key={event.id}
              className={
                isRupture ? "chain-row chain-row--rupture" : "chain-row"
              }
              data-testid={isRupture ? "chain-rupture" : undefined}
              data-event-index={isRupture ? index : undefined}
            >
              <span className="chain-row__action">{event.action}</span>
              <span className="chain-row__time">{event["occurred-at"]}</span>
              <HashChip hash={event["event-hash"]} />
            </li>
          );
        })}
      </ol>
    </section>
  );
}
