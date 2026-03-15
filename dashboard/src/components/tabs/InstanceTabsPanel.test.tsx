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

    expect(screen.getByText("Open Tabs (2)")).toBeInTheDocument();

    // Find heading for first tab title
    const titleHeading = screen.getByRole("heading", { name: "Alpha Tab" });
    const detailPanel = titleHeading.closest(".rounded-xl") as HTMLElement;

    expect(
      within(detailPanel).getByText("https://example.com/alpha"),
    ).toBeInTheDocument();

    // IdBadge will show "alpha" instead of "tab_alpha"
    expect(within(detailPanel).getByText("alpha")).toBeInTheDocument();
  });

  it("updates the selected tab details when a tab is clicked", async () => {
    const user = userEvent.setup();
    render(<InstanceTabsPanel tabs={tabs} />);

    // Select Beta tab from list
    const betaTabItem = screen
      .getByText("Beta Tab")
      .closest('[role="button"]')!;
    await user.click(betaTabItem);

    // Find heading for beta tab title
    const titleHeading = screen.getByRole("heading", { name: "Beta Tab" });
    const detailPanel = titleHeading.closest(".rounded-xl") as HTMLElement;

    expect(
      within(detailPanel).getByText("https://example.com/beta"),
    ).toBeInTheDocument();
    expect(within(detailPanel).getByText("beta")).toBeInTheDocument();
  });

  it("shows an empty state when there are no tabs", () => {
    render(<InstanceTabsPanel tabs={[]} />);

    expect(screen.getByText("Open Tabs (0)")).toBeInTheDocument();
    expect(screen.getByText("No tabs open")).toBeInTheDocument();
  });
});
