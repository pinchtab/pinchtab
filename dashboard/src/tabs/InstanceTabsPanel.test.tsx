import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import type { InstanceTab } from "../generated/types";
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

    // Find heading for first tab title
    const titleHeading = screen.getByRole("heading", { name: "Alpha Tab" });
    const detailPanel = titleHeading.closest(".rounded-xl") as HTMLElement;

    expect(
      within(detailPanel).getByText("https://example.com/alpha"),
    ).toBeInTheDocument();

    // IdBadge will show "ID" instead of "alpha" (shortened tab_alpha)
    expect(within(detailPanel).getByText("ID")).toBeInTheDocument();
  });

  it("updates the selected tab details when a tab is clicked", async () => {
    const user = userEvent.setup();
    render(<InstanceTabsPanel tabs={tabs} />);

    // Select Beta tab from list
    const betaTabItem = screen.getByRole("button", { name: /^Beta Tab$/ });
    await user.click(betaTabItem);

    // Find heading for beta tab title
    const titleHeading = screen.getByRole("heading", { name: "Beta Tab" });
    const detailPanel = titleHeading.closest(".rounded-xl") as HTMLElement;

    expect(
      within(detailPanel).getByText("https://example.com/beta"),
    ).toBeInTheDocument();
    expect(within(detailPanel).getByText("ID")).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Unpin Beta Tab and follow focus" }),
    ).toBeInTheDocument();
  });

  it("follows the focused tab until a manual selection is pinned", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<InstanceTabsPanel tabs={tabs} />);

    rerender(<InstanceTabsPanel tabs={[tabs[1], tabs[0]]} />);
    expect(
      screen.getByRole("heading", { name: "Beta Tab" }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /^Alpha Tab$/ }));
    expect(
      screen.getByRole("heading", { name: "Alpha Tab" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Unpin Alpha Tab and follow focus" }),
    ).toBeInTheDocument();

    rerender(<InstanceTabsPanel tabs={[tabs[1], tabs[0]]} />);
    expect(
      screen.getByRole("heading", { name: "Alpha Tab" }),
    ).toBeInTheDocument();

    await user.click(
      screen.getByRole("button", { name: "Unpin Alpha Tab and follow focus" }),
    );
    expect(
      screen.getByRole("button", { name: "Pin Beta Tab" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("heading", { name: "Beta Tab" }),
    ).toBeInTheDocument();
  });

  it("shows an empty state when there are no tabs", () => {
    render(<InstanceTabsPanel tabs={[]} />);

    expect(screen.getByText("No tabs open")).toBeInTheDocument();
  });
});
