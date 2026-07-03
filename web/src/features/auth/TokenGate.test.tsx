import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, test } from "vitest";
import { TokenGate, TOKEN_STORAGE_KEY } from "./TokenGate";

function makeUnsignedJwt(payload: Record<string, unknown>): string {
  const b64url = (obj: Record<string, unknown>) =>
    btoa(JSON.stringify(obj))
      .replace(/\+/g, "-")
      .replace(/\//g, "_")
      .replace(/=+$/, "");
  return `${b64url({ alg: "none", typ: "JWT" })}.${b64url(payload)}.`;
}

const VALID_TOKEN = makeUnsignedJwt({
  tenant_id: "11111111-1111-4111-8111-111111111111",
});

describe("TokenGate", () => {
  beforeEach(() => {
    sessionStorage.clear();
    localStorage.clear();
  });

  afterEach(() => {
    sessionStorage.clear();
    localStorage.clear();
  });

  test("shows a paste form when no token is stored", () => {
    render(<TokenGate>{() => <div>protected</div>}</TokenGate>);

    expect(
      screen.getByRole("textbox", { name: /token/i }),
    ).toBeInTheDocument();
    expect(screen.queryByText("protected")).not.toBeInTheDocument();
  });

  test("shows an inline error for a malformed token", async () => {
    const user = userEvent.setup();
    render(<TokenGate>{() => <div>protected</div>}</TokenGate>);

    await user.type(
      screen.getByRole("textbox", { name: /token/i }),
      "not-a-jwt",
    );
    await user.click(screen.getByRole("button", { name: /submit|continue/i }));

    expect(await screen.findByRole("alert")).toHaveTextContent(
      /invalid|malformed/i,
    );
    expect(screen.queryByText("protected")).not.toBeInTheDocument();
  });

  test("renders children and the tenant id for a valid token", async () => {
    const user = userEvent.setup();
    render(<TokenGate>{({ tenantId }) => <div>tenant: {tenantId}</div>}</TokenGate>);

    await user.type(
      screen.getByRole("textbox", { name: /token/i }),
      VALID_TOKEN,
    );
    await user.click(screen.getByRole("button", { name: /submit|continue/i }));

    expect(
      await screen.findByText(/11111111-1111-4111-8111-111111111111/),
    ).toBeInTheDocument();
  });

  test("stores the token in sessionStorage, not localStorage", async () => {
    const user = userEvent.setup();
    render(<TokenGate>{() => <div>protected</div>}</TokenGate>);

    await user.type(
      screen.getByRole("textbox", { name: /token/i }),
      VALID_TOKEN,
    );
    await user.click(screen.getByRole("button", { name: /submit|continue/i }));

    await screen.findByText("protected");

    expect(sessionStorage.getItem(TOKEN_STORAGE_KEY)).toBe(VALID_TOKEN);
    expect(localStorage.getItem(TOKEN_STORAGE_KEY)).toBeNull();
  });

  test("renders children immediately when a valid token is already in sessionStorage", async () => {
    sessionStorage.setItem(TOKEN_STORAGE_KEY, VALID_TOKEN);

    render(<TokenGate>{({ tenantId }) => <div>tenant: {tenantId}</div>}</TokenGate>);

    expect(
      await screen.findByText(/11111111-1111-4111-8111-111111111111/),
    ).toBeInTheDocument();
  });
});
