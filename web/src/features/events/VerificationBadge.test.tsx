import { render, screen } from "@testing-library/react";
import { describe, expect, test } from "vitest";
import { VerificationBadge } from "./VerificationBadge";

describe("VerificationBadge", () => {
  test("verified status is conveyed by visible text", () => {
    render(<VerificationBadge result={{ status: "verified", length: 3 }} />);
    expect(screen.getByText(/verified/i)).toBeInTheDocument();
  });

  test("verified badge always includes the tail-truncation caveat", () => {
    render(<VerificationBadge result={{ status: "verified", length: 3 }} />);
    expect(screen.getByText(/tail|truncat/i)).toBeInTheDocument();
  });

  test("tampered status names the failure reason in text", () => {
    render(
      <VerificationBadge
        result={{ status: "tampered", failedIndex: 1, reason: "hash-mismatch" }}
      />,
    );
    expect(screen.getByText(/tamper/i)).toBeInTheDocument();
    expect(screen.getByText(/hash-mismatch|hash mismatch/i)).toBeInTheDocument();
  });

  test("unverifiable status is conveyed by text", () => {
    render(
      <VerificationBadge result={{ status: "unverifiable", reason: "empty" }} />,
    );
    expect(screen.getByText(/unverifiable/i)).toBeInTheDocument();
  });

  test("does not rely on color alone (has a non-empty text label)", () => {
    const { container } = render(
      <VerificationBadge result={{ status: "verified", length: 1 }} />,
    );
    expect(container.textContent?.trim().length).toBeGreaterThan(0);
  });
});
