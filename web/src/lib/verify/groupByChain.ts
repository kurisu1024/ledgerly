import type { ApiEvent, ChainGroup } from "./types";

/**
 * Groups a flat, possibly-interleaved event array by chain-id, preserving
 * the order each chain first appears in and the relative order of events
 * within each group.
 */
export function groupByChain(events: ApiEvent[]): ChainGroup[] {
  const groups: ChainGroup[] = [];
  const indexByChainId = new Map<string, number>();

  for (const event of events) {
    const chainId = event["chain-id"];
    let index = indexByChainId.get(chainId);
    if (index === undefined) {
      index = groups.length;
      indexByChainId.set(chainId, index);
      groups.push({ chainId, events: [] });
    }
    groups[index]!.events.push(event);
  }

  return groups;
}
