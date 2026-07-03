import type { ApiEvent, NewApiEvent, PostEventResult } from "./types";

/**
 * GET /v1/export, optionally sliced to a single block via `?blockID=`.
 * Returns events with RAW `occurred-at` wire strings — callers must not
 * parse them through `Date` (see lib/verify/canonicalTime.ts).
 */
export async function exportEvents(
  _token: string,
  _blockId?: string,
): Promise<ApiEvent[]> {
  throw new Error("not implemented");
}

/**
 * POST /v1/events. Always resolves to `{ state: "accepted" }` on a 202 —
 * the server never synchronously confirms persistence.
 */
export async function postEvent(
  _token: string,
  _event: NewApiEvent,
): Promise<PostEventResult> {
  throw new Error("not implemented");
}
