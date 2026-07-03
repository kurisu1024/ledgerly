/**
 * Reproduces `e.OccurredAt.UTC().Format(time.RFC3339Nano)` from
 * internal/audit/events.go string-arithmetically, WITHOUT round-tripping
 * through `Date` — JS `Date` only carries millisecond precision and would
 * silently destroy the nanosecond-level detail that the Go hash is computed
 * over. The wire string may carry any offset (e.g. `+02:00`); the canonical
 * form is always expressed in `Z` with Go's trailing-zero trimming applied.
 */
export function canonicalTime(_occurredAtRaw: string): string {
  throw new Error("not implemented");
}
