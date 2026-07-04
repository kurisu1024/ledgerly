import { describe, expect, test } from "vitest";
import { decodeJwtPayload } from "./jwt";

function makeUnsignedJwt(payload: Record<string, unknown>): string {
  const b64url = (obj: Record<string, unknown>) =>
    btoa(JSON.stringify(obj))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "");

  const header = b64url({ alg: "none", typ: "JWT" });
  const body = b64url(payload);
  return `${header}.${body}.`;
}

describe("decodeJwtPayload", () => {
  test("extracts tenant_id from a valid dev JWT", () => {
    const token = makeUnsignedJwt({
      tenant_id: "11111111-1111-4111-8111-111111111111",
    });

    expect(decodeJwtPayload(token)).toEqual({
      ok: true,
      tenantId: "11111111-1111-4111-8111-111111111111",
    });
  });

  test("returns an error result for a token with too few segments", () => {
    const result = decodeJwtPayload("not-a-jwt");
    expect(result.ok).toBe(false);
    if (!result.ok) {
      expect(result.error).toMatch(/format|segment/i);
    }
  });

  test("returns an error result for an unparseable payload segment", () => {
    const result = decodeJwtPayload("aGVhZGVy.not-base64-json!!!.sig");
    expect(result.ok).toBe(false);
  });

  test("returns an error result when tenant_id is missing", () => {
    const token = makeUnsignedJwt({ sub: "someone" });
    const result = decodeJwtPayload(token);
    expect(result.ok).toBe(false);
  });

  test("never throws on malformed input", () => {
    expect(() => decodeJwtPayload("")).not.toThrow();
    expect(() => decodeJwtPayload("....")).not.toThrow();
  });
});
