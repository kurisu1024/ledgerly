import { describe, expect, test } from "vitest";
import { canonicalTime } from "./canonicalTime";
import fixtures from "./__fixtures__/golden-chains.json";

describe("canonicalTime", () => {
  for (const testCase of fixtures.cases) {
    describe(testCase.name, () => {
      testCase.chain.events.forEach((event, index) => {
        const expected = testCase["canonical-timestamps"][index];
        test(`event ${index}: ${event["occurred-at"]} -> ${expected}`, () => {
          expect(canonicalTime(event["occurred-at"])).toBe(expected);
        });
      });
    });
  }

  test("trims trailing zeros on a Z-form nanosecond timestamp", () => {
    expect(canonicalTime("2026-01-15T10:30:01.1234Z")).toBe(
      "2026-01-15T10:30:01.1234Z",
    );
  });

  test("preserves full nanosecond precision", () => {
    expect(canonicalTime("2026-01-15T10:30:00.123456789Z")).toBe(
      "2026-01-15T10:30:00.123456789Z",
    );
  });

  test("preserves a single trailing non-zero nanosecond digit", () => {
    expect(canonicalTime("2026-01-15T10:30:02.000000007Z")).toBe(
      "2026-01-15T10:30:02.000000007Z",
    );
  });

  test("converts a +02:00 offset to UTC", () => {
    expect(canonicalTime("2026-01-15T12:30:01.5+02:00")).toBe(
      "2026-01-15T10:30:01.5Z",
    );
  });
});
