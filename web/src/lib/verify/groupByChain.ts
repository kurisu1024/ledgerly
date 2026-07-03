import type { ApiEvent, ChainGroup } from "./types";

/**
 * Groups a flat, possibly-interleaved event array by chain-id, preserving
 * the order each chain first appears in and the relative order of events
 * within each group.
 */
export function groupByChain(_events: ApiEvent[]): ChainGroup[] {
  throw new Error("not implemented");
}
