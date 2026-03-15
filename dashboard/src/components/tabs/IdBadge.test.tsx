import { render, screen, fireEvent, waitFor } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import IdBadge from "./IdBadge";

describe("IdBadge", () => {
  const mockId = "tab_1234567890";

  beforeEach(() => {
    vi.stubGlobal("navigator", {
      clipboard: {
        writeText: vi.fn(),
      },
    });
  });

  it("renders shortened ID", () => {
    render(<IdBadge id={mockId} />);
    // Should split by _ and take the last part, shortened to 8 chars
    expect(screen.getByText("12345678")).toBeInTheDocument();
  });

  it("renders full ID if no underscore", () => {
    render(<IdBadge id="simpleid" />);
    expect(screen.getByText("simpleid")).toBeInTheDocument();
  });

  it("copies full ID to clipboard on click", async () => {
    render(<IdBadge id={mockId} />);
    const button = screen.getByRole("button");
    fireEvent.click(button);

    expect(navigator.clipboard.writeText).toHaveBeenCalledWith(mockId);

    // Check for success emoji
    expect(screen.getByText("✅")).toBeInTheDocument();

    // Wait for it to reset (optional, but shows we understand the logic)
    await waitFor(
      () => {
        expect(screen.queryByText("✅")).not.toBeInTheDocument();
      },
      { timeout: 2500 },
    );
  });

  it("renders as div if not copyable", () => {
    render(<IdBadge id={mockId} copyable={false} />);
    expect(screen.queryByRole("button")).not.toBeInTheDocument();
    expect(screen.getByText("12345678")).toBeInTheDocument();
  });
});
