# ADR-0001: Sentry-model trigger architecture with client-side rule evaluation

Date: 2026-07-03
Status: Accepted

## Context

Ledgerly's adoption bet is "drop-in": an app keeps its existing logger and gains
audit-grade logging by adding one SDK handler. But audit events are intentional and
low-volume while log streams are noisy and huge, so *which* log records become chained
audit events must be decided somewhere. Three shapes were considered:

1. **Smart SDK** — SDK fetches tenant rules, evaluates locally, ships only matches.
2. **Thin SDK, smart server** — SDK ships everything above a floor; server evaluates.
3. **Hybrid** — coarse local floor plus full server-side evaluation.

## Decision

Shape 1. The SDK (first: a Go `slog.Handler`) evaluates trigger rules locally against
every log record and ships only triggered events into the existing async ingest path
(`POST /v1/events` → 202 → queue → worker → chain). The rule language is deliberately
tiny *data*, not code — v1: level threshold, field equals/exists, event-type match —
versioned so SDKs refuse rule versions they don't understand. Every SDK implements the
identical matcher, verified by a shared conformance suite (JSON fixtures + expected
outcomes).

## Consequences

- Non-audit logs never leave the host app: no privacy leak for self-hosters, no egress
  or ingest blow-up, "drop-in" stays honest.
- The chain/worker/storage pipeline is untouched; triggered events are ordinary events.
- Rule changes propagate on the SDK refresh interval, not instantly. A push channel can
  be grafted on later without changing the model; the reverse migration (retrofitting
  Shape 1's privacy properties onto Shape 2) would not be possible.
- Every SDK language reimplements the matcher — accepted because the rule language is
  kept small; the conformance suite is the guard against drift.
- SDK delivery must never slow the host app or silently drop events: async batched
  sends, spill to a local disk buffer on failure, retry.
