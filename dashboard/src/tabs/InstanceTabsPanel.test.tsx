import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { InstanceTab } from "../generated/types";
import InstanceTabsPanel from "./InstanceTabsPanel";

const mockStore = {
  instances: [],
  tabsChartData: [],
  memoryChartData: [],
  serverChartData: [],
  currentMetrics: {},
  settings: {},
  monitoringShowTelemetry: true,
  setMonitoringShowTelemetry: vi.fn((show: boolean) => {
    mockStore.monitoringShowTelemetry = show;
  }),
};

vi.mock("../stores/useAppStore", () => ({
  useAppStore: () => mockStore,
}));

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
  beforeEach(() => {
    mockStore.monitoringShowTelemetry = true;
    mockStore.setMonitoringShowTelemetry.mockClear();
  });

  it("enables telemetry once when tabs disappear", () => {
    mockStore.monitoringShowTelemetry = false;
    render(<InstanceTabsPanel tabs={[]} />);

    expect(mockStore.setMonitoringShowTelemetry).toHaveBeenCalledTimes(1);
    expect(mockStore.setMonitoringShowTelemetry).toHaveBeenCalledWith(true);
  });

  it("shows telemetry by default without marking existing tabs as new", () => {
    render(<InstanceTabsPanel tabs={tabs} />);

    expect(screen.getByText("Collecting data...")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: "Tabs" })).toBeInTheDocument();
    expect(
      screen.queryByRole("button", { name: /Tabs \(\d+ new\)/ }),
    ).toBeNull();
  });

  it("marks only newly added tabs and clears the badge after visiting tabs", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<InstanceTabsPanel tabs={[tabs[0]]} />);

    rerender(<InstanceTabsPanel tabs={tabs} />);
    expect(
      screen.getByRole("button", { name: "Tabs (1 new)" }),
    ).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Tabs (1 new)" }));
    rerender(<InstanceTabsPanel tabs={tabs} />);

    expect(screen.getByRole("button", { name: "Tabs" })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Tabs (1 new)" })).toBeNull();
  });

  it("auto-selects the first tab and shows its details", () => {
    mockStore.monitoringShowTelemetry = false;
    render(<InstanceTabsPanel tabs={tabs} />);

    const titleHeading = screen.getByRole("heading", { name: "Alpha Tab" });
    const detailPanel = titleHeading.closest(".rounded-xl") as HTMLElement;

    expect(
      within(detailPanel).getByText("https://example.com/alpha"),
    ).toBeInTheDocument();

    expect(within(detailPanel).getByText("ID")).toBeInTheDocument();
  });

  it("updates the selected tab details when a tab is clicked", async () => {
    const user = userEvent.setup();
    const { rerender } = render(<InstanceTabsPanel tabs={tabs} />);

    const betaTabItem = screen.getByRole("button", { name: /^Beta Tab$/ });
    await user.click(betaTabItem);
    rerender(<InstanceTabsPanel tabs={tabs} />);

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
    mockStore.monitoringShowTelemetry = false;
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
    mockStore.monitoringShowTelemetry = false;
    render(<InstanceTabsPanel tabs={[]} />);

    expect(screen.getByText("No tabs open")).toBeInTheDocument();
  });
});
