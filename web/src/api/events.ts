import type { ApiEvent, NewApiEvent, PostEventResult } from "./types";
import { ApiError, AuthError, RetryableError } from "./types";

async function request(url: string, init: RequestInit): Promise<Response> {
  const response = await fetch(url, init);
  if (response.ok) {
    return response;
  }

  const message = (await response.text()).trim();

  if (response.status === 401) {
    throw new AuthError(message || "Unauthorized");
  }
  if (response.status === 503) {
    throw new RetryableError(message || "Service unavailable, retry later");
  }
  throw new ApiError(
    message || `Request to ${url} failed with status ${response.status}`,
    response.status,
  );
}

/**
 * GET /v1/export, optionally sliced to a single block via `?blockID=`.
 * Returns events with RAW `occurred-at` wire strings — callers must not
 * parse them through `Date` (see lib/verify/canonicalTime.ts).
 */
export async function exportEvents(
  token: string,
  blockId?: string,
): Promise<ApiEvent[]> {
  const url = blockId
    ? `/v1/export?blockID=${encodeURIComponent(blockId)}`
    : "/v1/export";
  const response = await request(url, {
    headers: { Authorization: `Bearer ${token}` },
  });

  // Go encodes a nil slice as JSON `null`, so a tenant with no flushed
  // blocks legitimately returns `null` rather than `[]`.
  const data = (await response.json()) as ApiEvent[] | null;
  if (data !== null && !Array.isArray(data)) {
    throw new ApiError(
      `GET ${url} returned a non-array body`,
      response.status,
    );
  }
  return data ?? [];
}

/**
 * POST /v1/events. Always resolves to `{ state: "accepted" }` on a 202 —
 * the server never synchronously confirms persistence.
 */
export async function postEvent(
  token: string,
  event: NewApiEvent,
): Promise<PostEventResult> {
  await request("/v1/events", {
    method: "POST",
    headers: {
      Authorization: `Bearer ${token}`,
      "Content-Type": "application/json",
    },
    body: JSON.stringify(event),
  });
  return { state: "accepted" };
}
