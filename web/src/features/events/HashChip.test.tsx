import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, test } from "vitest";
import { HashChip } from "./HashChip";

const FULL_HASH = "0zU0AUdJWESPYZZZ9A3msErGDmDIWarnshEu9NN8aQg=";

describe("HashChip", () => {
  test("renders a truncated form of the hash by default", () => {
    render(<HashChip hash={FULL_HASH} />);
    expect(screen.queryByText(FULL_HASH)).not.toBeInTheDocument();
  });

  test("expands to the full hash on click", async () => {
    const user = userEvent.setup();
    render(<HashChip hash={FULL_HASH} />);

    await user.click(screen.getByRole("button"));

    expect(screen.getByText(FULL_HASH)).toBeInTheDocument();
  });

  test("expands to the full hash on Enter", async () => {
    const user = userEvent.setup();
    render(<HashChip hash={FULL_HASH} />);

    const chip = screen.getByRole("button");
    chip.focus();
    await user.keyboard("{Enter}");

    expect(screen.getByText(FULL_HASH)).toBeInTheDocument();
  });

  test("chip is keyboard-focusable", () => {
    render(<HashChip hash={FULL_HASH} />);
    const chip = screen.getByRole("button");
    expect(chip).toHaveAttribute("tabIndex", "0");
  });
});
