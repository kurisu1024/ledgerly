import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { exportEvents, postEvent } from "./events";
import { ApiError, AuthError, RetryableError } from "./types";
import type { ApiEvent } from "./types";

const sampleEvent: ApiEvent = {
  id: "eeeeeeee-0000-4000-8000-000000000001",
  "chain-id": "aaaaaaaa-0001-4000-8000-000000000001",
  "tenant-id": "11111111-1111-4111-8111-111111111111",
  "occurred-at": "2026-01-15T10:30:00.123456789Z",
  action: "project.update.1",
  actor: { id: "user-1", type: "user" },
  resource: { id: "proj-1", type: "project" },
  metadata: { "request-id": "req-1" },
  "prev-hash": "kBEx2DixeqwPeIW4HgPL3J9RV6ADQ9MKsiCDaF7RQWo=",
  "event-hash": "0zU0AUdJWESPYZZZ9A3msErGDmDIWarnshEu9NN8aQg=",
};

const TOKEN = "dev-token";

function jsonResponse(body: unknown, status = 200): Response {
  return new Response(JSON.stringify(body), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function textResponse(body: string, status: number): Response {
  return new Response(body, { status });
}

describe("exportEvents", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test("GETs /v1/export with a bearer token", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(jsonResponse([sampleEvent]));

    const result = await exportEvents(TOKEN);

    expect(fetch).toHaveBeenCalledWith(
      "/v1/export",
      expect.objectContaining({
        headers: expect.objectContaining({
          Authorization: `Bearer ${TOKEN}`,
        }),
      }),
    );
    expect(result).toEqual([sampleEvent]);
  });

  test("slices with ?blockID= when a block id is given", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(jsonResponse([]));

    await exportEvents(TOKEN, "bbbbbbbb-0001-4000-8000-000000000001");

    expect(fetch).toHaveBeenCalledWith(
      "/v1/export?blockID=bbbbbbbb-0001-4000-8000-000000000001",
      expect.anything(),
    );
  });

  test("never parses occurred-at through Date — raw string survives", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(jsonResponse([sampleEvent]));

    const [event] = await exportEvents(TOKEN);

    expect(typeof event!["occurred-at"]).toBe("string");
    expect(event!["occurred-at"]).toBe("2026-01-15T10:30:00.123456789Z");
  });

  // Go's export handler json.Marshal's a nil slice, so a tenant with no
  // flushed blocks yields a literal `null` body — not `[]`.
  test("resolves an empty array when the body is a JSON null", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(jsonResponse(null));

    await expect(exportEvents(TOKEN)).resolves.toEqual([]);
  });

  test("rejects with ApiError when the body is non-null but not an array", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(jsonResponse({ not: "an array" }));

    await expect(exportEvents(TOKEN)).rejects.toBeInstanceOf(ApiError);
  });

  test("percent-encodes the blockID query value", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(jsonResponse([]));

    await exportEvents(TOKEN, "block id&x=1");

    expect(fetch).toHaveBeenCalledWith(
      "/v1/export?blockID=block%20id%26x%3D1",
      expect.anything(),
    );
  });

  test("throws AuthError on 401", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      textResponse("Unauthorized\n", 401),
    );

    await expect(exportEvents(TOKEN)).rejects.toBeInstanceOf(AuthError);
  });

  test("throws RetryableError on 503", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      textResponse("Event queue is full\n", 503),
    );

    await expect(exportEvents(TOKEN)).rejects.toBeInstanceOf(RetryableError);
  });

  test("surfaces the plain-text error body as the error message", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      textResponse("Failed to fetch events\n", 500),
    );

    await expect(exportEvents(TOKEN)).rejects.toMatchObject({
      message: expect.stringContaining("Failed to fetch events"),
    });
  });

  test("throws a generic ApiError for other non-2xx statuses", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(textResponse("nope\n", 500));

    await expect(exportEvents(TOKEN)).rejects.toBeInstanceOf(ApiError);
  });
});

describe("postEvent", () => {
  beforeEach(() => {
    vi.stubGlobal("fetch", vi.fn());
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  test("sends a kebab-case body and resolves accepted on 202", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(jsonResponse(sampleEvent, 202));

    const result = await postEvent(TOKEN, {
      action: "project.update.1",
      actor: { id: "user-1", type: "user" },
      resource: { id: "proj-1", type: "project" },
      metadata: { "request-id": "req-1" },
    });

    expect(result).toEqual({ state: "accepted" });

    const [, init] = vi.mocked(fetch).mock.calls[0]!;
    const body = JSON.parse((init as RequestInit).body as string);
    expect(body).toMatchObject({
      action: "project.update.1",
      actor: { id: "user-1", type: "user" },
      resource: { id: "proj-1", type: "project" },
      metadata: { "request-id": "req-1" },
    });
    // No camelCase leakage.
    expect(body).not.toHaveProperty("occurredAt");
  });

  test("the accepted result never carries a persisted state", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(jsonResponse(sampleEvent, 202));

    const result = await postEvent(TOKEN, {
      action: "x",
      actor: {},
      resource: {},
    });

    // @ts-expect-error -- "persisted" must not be assignable to state; this
    // line exists to fail type-checking if the union ever grows that variant.
    const impossible: "persisted" = result.state;
    expect(impossible).not.toBe("persisted");
  });

  test("throws AuthError on 401", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(textResponse("Invalid token\n", 401));

    await expect(
      postEvent(TOKEN, { action: "x", actor: {}, resource: {} }),
    ).rejects.toBeInstanceOf(AuthError);
  });

  test("throws RetryableError on 503", async () => {
    vi.mocked(fetch).mockResolvedValueOnce(
      textResponse("Event queue is full\n", 503),
    );

    await expect(
      postEvent(TOKEN, { action: "x", actor: {}, resource: {} }),
    ).rejects.toBeInstanceOf(RetryableError);
  });
});
