import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import TabItem from "./TabItem";
import type { InstanceTab } from "../../generated/types";

const mockTab: InstanceTab = {
  id: "tab_123",
  instanceId: "inst_456",
  url: "https://pinchtab.com/page",
  title: "Example Page",
};

describe("TabItem", () => {
  it("renders tab title and url", () => {
    render(<TabItem tab={mockTab} />);

    expect(screen.getByText("Example Page")).toBeInTheDocument();
    expect(screen.getByText("https://pinchtab.com/page")).toBeInTheDocument();
  });

  it("shows Untitled for tabs without title", () => {
    const noTitleTab = { ...mockTab, title: "" };
    render(<TabItem tab={noTitleTab} />);

    expect(screen.getByText("Untitled")).toBeInTheDocument();
  });

  it("renders short tab ID badge", () => {
    render(<TabItem tab={mockTab} />);

    // Tab ID is shortened to last segment
    expect(screen.getByText("123")).toBeInTheDocument();
  });

  it("renders compact variant", () => {
    render(<TabItem tab={mockTab} compact />);

    expect(screen.getByText("Example Page")).toBeInTheDocument();
    // Compact renders as a flat bordered row
    const titleEl = screen.getByText("Example Page");
    const outerDiv = titleEl.closest(".rounded-sm");
    expect(outerDiv).toBeInTheDocument();
  });

  it("renders flat row variant by default", () => {
    render(<TabItem tab={mockTab} />);

    // Default renders as a bordered row, not a nested card shell
    const titleEl = screen.getByText("Example Page");
    const row = titleEl.closest(".rounded-md");
    expect(row).toBeInTheDocument();
  });
});
