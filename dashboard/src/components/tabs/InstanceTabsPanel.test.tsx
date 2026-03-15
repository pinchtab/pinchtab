import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { InstanceTab } from "../../generated/types";
import InstanceTabsPanel from "./InstanceTabsPanel";

const tabs: InstanceTab[] = [
  {
    id: "tab_alpha",
    instanceId: "inst_123",
    title: "Alpha Tab",
    url: "https://example.com/alpha",
  },
  {
    id: "tab_beta",
    instanceId: "inst_123",
    title: "Beta Tab",
    url: "https://example.com/beta",
  },
];

describe("InstanceTabsPanel", () => {
  it("auto-selects the first tab and shows its details", () => {
    render(<InstanceTabsPanel tabs={tabs} />);

    const detailPanel = screen
      .getByText("Selected Tab")
      .closest(".rounded-xl") as HTMLElement;

    expect(screen.getByText("Open Tabs (2)")).toBeInTheDocument();
    expect(screen.getByText("Selected Tab")).toBeInTheDocument();
    expect(
      within(detailPanel).getByRole("heading", { name: "Alpha Tab" }),
    ).toBeInTheDocument();
    expect(
      within(detailPanel).getByText("https://example.com/alpha"),
    ).toBeInTheDocument();
    expect(within(detailPanel).getByText("tab_alpha")).toBeInTheDocument();
  });

  it("updates the selected tab details when a tab is clicked", async () => {
    render(<InstanceTabsPanel tabs={tabs} />);

    await userEvent.click(screen.getByRole("button", { name: /beta tab/i }));

    const detailPanel = screen
      .getByText("Selected Tab")
      .closest(".rounded-xl") as HTMLElement;

    expect(
      within(detailPanel).getByRole("heading", { name: "Beta Tab" }),
    ).toBeInTheDocument();
    expect(
      within(detailPanel).getByText("https://example.com/beta"),
    ).toBeInTheDocument();
    expect(within(detailPanel).getByText("tab_beta")).toBeInTheDocument();
  });

  it("shows an empty state when there are no tabs", () => {
    render(<InstanceTabsPanel tabs={[]} />);

    expect(screen.getByText("Open Tabs (0)")).toBeInTheDocument();
    expect(screen.getByText("No tabs open")).toBeInTheDocument();
  });
});
