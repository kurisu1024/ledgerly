import { describe, expect, test } from "vitest";
import { hashEvent } from "./hashEvent";
import fixtures from "./__fixtures__/golden-chains.json";

// Only the valid-chain fixtures have event-hash values that are expected to
// match a fresh recomputation — the tampered/broken/foreign fixtures
// deliberately store a hash that will NOT match their (mutated) wire
// content, and are exercised by verifyChain.test.ts instead.
const validCases = fixtures.cases.filter((c) => c.expected.verified);

describe("hashEvent", () => {
  for (const testCase of validCases) {
    describe(testCase.name, () => {
      testCase.chain.events.forEach((event, index) => {
        test(`event ${index} (${event.id}) recomputes to the stored event-hash`, async () => {
          await expect(hashEvent(event)).resolves.toBe(event["event-hash"]);
        });
      });
    });
  }

  test("genesis prev-hash is the fixture's genesis-hash for a chain's first event", async () => {
    const firstEvent = validCases[0]!.chain.events[0]!;
    expect(firstEvent["prev-hash"]).toBe(fixtures["genesis-hash"]);
    await expect(hashEvent(firstEvent)).resolves.toBe(
      firstEvent["event-hash"],
    );
  });
});
