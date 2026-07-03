import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, test, vi } from "vitest";
import { EventViewer } from "./EventViewer";
import { exportEvents } from "../../api/events";
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
