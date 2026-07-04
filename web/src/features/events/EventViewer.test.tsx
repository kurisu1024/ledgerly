import { act, render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { EventViewer } from "./EventViewer";
import { exportEvents } from "../../api/events";
import { ApiError, AuthError } from "../../api/types";
import type { ApiEvent } from "../../lib/verify/types";
import fixtures from "../../lib/verify/__fixtures__/golden-chains.json";

vi.mock("../../api/events", () => ({
  exportEvents: vi.fn(),
}));

const mockedExportEvents = vi.mocked(exportEvents);

const TOKEN = "dev-token";

function eventsFor(name: string): ApiEvent[] {
  return fixtures.cases.find((c) => c.name === name)!.chain.events as ApiEvent[];
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

describe("EventViewer", () => {
  beforeEach(() => {
    mockedExportEvents.mockReset();
  });

  afterEach(() => {
    vi.clearAllMocks();
  });

  test("groups a mixed export into one card per chain", async () => {
    mockedExportEvents.mockResolvedValueOnce([
      ...eventsFor("valid-nano"),
      ...eventsFor("valid-nil-metadata"),
    ]);

    render(<EventViewer token={TOKEN} />);

    const cards = await screen.findAllByTestId("chain-card");
    expect(cards).toHaveLength(2);
  });

  test("a verified chain shows a badge carrying the tail-truncation caveat", async () => {
    mockedExportEvents.mockResolvedValueOnce(eventsFor("valid-nano"));

    render(<EventViewer token={TOKEN} />);

    const card = await screen.findByTestId("chain-card");
    expect(within(card).getByText(/verified/i)).toBeInTheDocument();
    expect(within(card).getByText(/tail|truncat/i)).toBeInTheDocument();
  });

  test("a tampered chain marks the rupture at the failing row", async () => {
    mockedExportEvents.mockResolvedValueOnce(eventsFor("tampered-action"));

    render(<EventViewer token={TOKEN} />);

    const card = await screen.findByTestId("chain-card");
    const ruptureRow = within(card).getByTestId("chain-rupture");
    expect(ruptureRow).toHaveAttribute("data-event-index", "1");
  });

  test("an explicit Refresh button re-fetches the export", async () => {
    mockedExportEvents.mockResolvedValue(eventsFor("valid-nano"));
    const user = userEvent.setup();

    render(<EventViewer token={TOKEN} />);
    await screen.findByTestId("chain-card");

    expect(mockedExportEvents).toHaveBeenCalledTimes(1);

    await user.click(screen.getByRole("button", { name: /refresh/i }));

    expect(mockedExportEvents).toHaveBeenCalledTimes(2);
  });

  test("shows an empty state mentioning `make load-events` when the export is empty", async () => {
    mockedExportEvents.mockResolvedValueOnce([]);

    render(<EventViewer token={TOKEN} />);

    expect(await screen.findByText(/make load-events/)).toBeInTheDocument();
  });

  test("a slower, older response never clobbers a newer one", async () => {
    const older = deferred<ApiEvent[]>();
    const newer = deferred<ApiEvent[]>();
    mockedExportEvents
      .mockReturnValueOnce(older.promise)
      .mockReturnValueOnce(newer.promise);

    // Overlap two loads by changing the token while the first is in flight.
    const { rerender } = render(<EventViewer token="token-a" />);
    rerender(<EventViewer token="token-b" />);
    expect(mockedExportEvents).toHaveBeenCalledTimes(2);

    // The NEWER request resolves first, with a single chain.
    await act(async () => {
      newer.resolve(eventsFor("valid-nano"));
    });
    expect(await screen.findAllByTestId("chain-card")).toHaveLength(1);

    // The stale request resolves late with two chains — it must be dropped.
    await act(async () => {
      older.resolve([
        ...eventsFor("valid-nano"),
        ...eventsFor("valid-nil-metadata"),
      ]);
      await new Promise((r) => setTimeout(r, 50));
    });
    expect(screen.getAllByTestId("chain-card")).toHaveLength(1);
  });

  test("shows a loading status and disables Refresh while in flight", async () => {
    const pending = deferred<ApiEvent[]>();
    mockedExportEvents.mockReturnValueOnce(pending.promise);

    render(<EventViewer token={TOKEN} />);

    expect(screen.getByRole("status")).toHaveTextContent(/loading/i);
    expect(screen.getByRole("button", { name: /refresh/i })).toBeDisabled();

    await act(async () => {
      pending.resolve(eventsFor("valid-nano"));
    });

    await screen.findByTestId("chain-card");
    expect(screen.queryByRole("status")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /refresh/i })).toBeEnabled();
  });

  test("keeps a shown error visible until the retry settles", async () => {
    mockedExportEvents.mockRejectedValueOnce(new ApiError("boom", 500));
    const retry = deferred<ApiEvent[]>();
    mockedExportEvents.mockReturnValueOnce(retry.promise);
    const user = userEvent.setup();

    render(<EventViewer token={TOKEN} />);
    await screen.findByRole("alert");

    await user.click(screen.getByRole("button", { name: /refresh/i }));

    // Retry is in flight: the previous error must still be visible.
    expect(screen.getByRole("alert")).toHaveTextContent("boom");

    await act(async () => {
      retry.resolve(eventsFor("valid-nano"));
    });

    await screen.findByTestId("chain-card");
    expect(screen.queryByRole("alert")).not.toBeInTheDocument();
  });

  test("an auth failure offers a way to use a different token", async () => {
    mockedExportEvents.mockRejectedValueOnce(new AuthError("Unauthorized"));
    const onResetSession = vi.fn();
    const user = userEvent.setup();

    render(<EventViewer token={TOKEN} onResetSession={onResetSession} />);

    const action = await screen.findByRole("button", {
      name: /use a different token/i,
    });
    await user.click(action);

    expect(onResetSession).toHaveBeenCalledTimes(1);
  });

  test("non-auth failures do not offer the token reset action", async () => {
    mockedExportEvents.mockRejectedValueOnce(new ApiError("boom", 500));

    render(<EventViewer token={TOKEN} onResetSession={vi.fn()} />);

    await screen.findByRole("alert");
    expect(
      screen.queryByRole("button", { name: /use a different token/i }),
    ).not.toBeInTheDocument();
  });

  test("has no search input anywhere in the viewer", async () => {
    mockedExportEvents.mockResolvedValueOnce(eventsFor("valid-nano"));

    render(<EventViewer token={TOKEN} />);
    await screen.findByTestId("chain-card");

    expect(screen.queryByRole("searchbox")).not.toBeInTheDocument();
    expect(
      screen.queryByPlaceholderText(/search/i),
    ).not.toBeInTheDocument();
  });
});
