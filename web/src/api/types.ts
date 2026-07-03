import type { ApiEvent } from "../lib/verify/types";

export type { ApiEvent };

/**
 * Wire body for POST /v1/events. Kebab-case keys, matching api/events.Event.
 * ID/chain-id/hash fields are server-assigned and intentionally absent.
 */
export interface NewApiEvent {
  action: string;
  actor: Record<string, string>;
  resource: Record<string, string>;
  metadata?: Record<string, string>;
}

/**
 * The write path is asynchronous: POST /v1/events returns 202 before the
 * event has been chained or persisted. There is deliberately no "persisted"
 * variant here — the API never reports that synchronously.
 */
export type PostEventResult = { state: "accepted" };

/** Thrown for a 401 response. */
export class AuthError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "AuthError";
  }
}

/** Thrown for a 503 (queue full) response — safe for the caller to retry. */
export class RetryableError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "RetryableError";
  }
}

/** Thrown for any other non-2xx response. */
export class ApiError extends Error {
  constructor(
    message: string,
    public readonly status: number,
  ) {
    super(message);
    this.name = "ApiError";
  }
}
