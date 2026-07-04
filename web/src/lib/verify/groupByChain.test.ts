import { describe, expect, test } from "vitest";
import { groupByChain } from "./groupByChain";
import type { ApiEvent } from "./types";
import fixtures from "./__fixtures__/golden-chains.json";

describe("groupByChain", () => {
  test("splits two interleaved chains into two groups, preserving order", () => {
    const chainA = fixtures.cases.find((c) => c.name === "valid-nano")!.chain;
    const chainB = fixtures.cases.find(
      (c) => c.name === "valid-nil-metadata",
    )!.chain;

    // Interleave: A0, B0, A1, B1, A2
    const interleaved: ApiEvent[] = [
      chainA.events[0] as ApiEvent,
      chainB.events[0] as ApiEvent,
      chainA.events[1] as ApiEvent,
      chainB.events[1] as ApiEvent,
      chainA.events[2] as ApiEvent,
    ];

    const groups = groupByChain(interleaved);

    expect(groups).toHaveLength(2);
    expect(groups[0]?.chainId).toBe(chainA.id);
    expect(groups[0]?.events.map((e) => e.id)).toEqual(
      chainA.events.map((e) => e.id),
    );
    expect(groups[1]?.chainId).toBe(chainB.id);
    expect(groups[1]?.events.map((e) => e.id)).toEqual(
      chainB.events.map((e) => e.id),
    );
  });

  test("returns an empty array for an empty input", () => {
    expect(groupByChain([])).toEqual([]);
  });

  test("a single chain produces a single group in original order", () => {
    const chain = fixtures.cases.find((c) => c.name === "broken-link")!.chain;
    const groups = groupByChain(chain.events as ApiEvent[]);

    expect(groups).toEqual([
      { chainId: chain.id, events: chain.events },
    ]);
  });
});
