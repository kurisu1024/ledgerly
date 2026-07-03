import { describe, expect, test } from "vitest";
import { verifyChain } from "./verifyChain";
import type { ApiEvent } from "./types";
import fixtures from "./__fixtures__/golden-chains.json";

describe("verifyChain", () => {
  for (const testCase of fixtures.cases) {
    test(`${testCase.name}: ${testCase.description}`, async () => {
      const result = await verifyChain(testCase.chain.events as ApiEvent[]);

      if (testCase.expected.verified) {
        expect(result).toEqual({
          status: "verified",
          length: testCase.chain.events.length,
        });
      } else {
        expect(result).toEqual({
          status: "tampered",
          failedIndex: testCase.expected.failedIndex,
          reason: testCase.expected.reason,
        });
      }
    });
  }

  test("empty chain is unverifiable", async () => {
    await expect(verifyChain([])).resolves.toEqual({
      status: "unverifiable",
      reason: "empty",
    });
  });

  // The golden fixture (generated from Go's AppendEvent) has no case for a
  // tenant-mixed chain, so this is hand-authored to exercise the tenant
  // early-exit path directly. In internal/audit VerifyChain, the
  // chain-id/tenant-id check runs *before* prev-hash linkage or hash
  // verification, so the second event's hash/prev-hash values below are
  // deliberately left as placeholders — they are never reached.
  test("mixed-tenant events fail as tenant-mixed before hash is checked", async () => {
    const validCase = fixtures.cases.find((c) => c.name === "valid-nano")!;
    const [first] = validCase.chain.events;

    const events: ApiEvent[] = [
      first as ApiEvent,
      {
        ...(first as ApiEvent),
        id: "eeeeeeee-0000-4000-8000-00000000009a",
        "tenant-id": "22222222-2222-4222-8222-222222222222",
        "prev-hash": (first as ApiEvent)["event-hash"],
        "event-hash": "AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA=",
      },
    ];

    await expect(verifyChain(events)).resolves.toEqual({
      status: "tampered",
      failedIndex: 1,
      reason: "tenant-mixed",
    });
  });
});
