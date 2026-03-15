import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import MonitoringPage from "./MonitoringPage";
import { useAppStore } from "../stores/useAppStore";
import type { Instance } from "../generated/types";

vi.mock("../services/api", () => ({
  stopInstance: vi.fn(),
}));

vi.mock("../components/molecules", () => ({
  TabsChart: () => <div>Tabs Chart</div>,
  InstanceListItem: ({
    instance,
    onClick,
  }: {
    instance: Instance;
    onClick: () => void;
  }) => <button onClick={onClick}>{instance.profileName}</button>,
  InstanceTabsPanel: ({ tabs }: { tabs: { title: string }[] }) => (
    <div>Tabs Panel ({tabs.length})</div>
  ),
}));

const instances: Instance[] = [
  {
    id: "inst_beta",
    profileId: "prof_beta",
    profileName: "beta",
    port: "9988",
    headless: false,
    status: "running",
    startTime: "2026-03-06T10:00:00Z",
    attached: false,
  },
];

function ProfilesRouteProbe() {
  const location = useLocation();
  const state = location.state as { selectedProfileKey?: string } | null;

  return (
    <div>
      <div>Profiles Route</div>
      <div>{state?.selectedProfileKey ?? "missing"}</div>
    </div>
  );
}

describe("MonitoringPage", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    useAppStore.setState({
      instances,
      tabsChartData: [],
      memoryChartData: [],
      serverChartData: [],
      currentTabs: {
        inst_beta: [],
      },
      currentMemory: {},
      settings: {
        ...useAppStore.getState().settings,
        monitoring: {
          ...useAppStore.getState().settings.monitoring,
          memoryMetrics: false,
        },
      },
    });
  });

  it("opens the selected profile from the active instance header", async () => {
    render(
      <MemoryRouter initialEntries={["/dashboard/monitoring"]}>
        <Routes>
          <Route path="/dashboard/monitoring" element={<MonitoringPage />} />
          <Route path="/dashboard/profiles" element={<ProfilesRouteProbe />} />
        </Routes>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: "beta" })).toBeInTheDocument();
    });

    await userEvent.click(screen.getByRole("button", { name: "Open Profile" }));

    await waitFor(() => {
      expect(screen.getByText("Profiles Route")).toBeInTheDocument();
    });
    expect(screen.getByText("prof_beta")).toBeInTheDocument();
  });
});
