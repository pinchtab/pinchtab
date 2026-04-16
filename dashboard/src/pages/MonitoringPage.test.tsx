import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { MemoryRouter, Route, Routes, useLocation } from "react-router-dom";
import MonitoringPage from "./MonitoringPage";
import { useAppStore } from "../stores/useAppStore";
import type { Instance } from "../generated/types";
import * as api from "../services/api";

vi.mock("../services/api", () => ({
  stopInstance: vi.fn(),
  fetchInstances: vi.fn().mockResolvedValue([]),
  launchInstance: vi.fn().mockResolvedValue({
    id: "inst_default",
    profileId: "default",
    profileName: "default",
    port: "9868",
    mode: "headed",
    headless: false,
    status: "starting",
    startTime: "2026-03-06T10:00:00Z",
    attached: false,
  }),
  fetchBackendConfig: vi.fn().mockResolvedValue({
    config: {
      multiInstance: { strategy: "always-on" },
      profiles: { defaultProfile: "default" },
      instanceDefaults: { mode: "headed" },
    },
  }),
  fetchActivity: vi.fn(),
  fetchAllTabs: vi.fn(),
}));

const instances: Instance[] = [
  {
    id: "inst_beta",
    profileId: "prof_beta",
    profileName: "beta",
    port: "9988",
    mode: "headed",
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
    vi.useRealTimers();
    useAppStore.setState({
      instances,
      tabsChartData: [],
      memoryChartData: [],
      serverChartData: [],
      currentTabs: {
        inst_beta: [],
      },
      currentMemory: {},
      currentMetrics: {},
      monitoringSidebarCollapsed: false,
      selectedMonitoringInstanceId: null,
      monitoringShowTelemetry: false,
      settings: {
        ...useAppStore.getState().settings,
        monitoring: {
          ...useAppStore.getState().settings.monitoring,
          memoryMetrics: false,
        },
      },
    });
  });

  afterEach(() => {
    vi.useRealTimers();
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

  it("refreshes instances after stopping a running instance", async () => {
    render(
      <MemoryRouter initialEntries={["/dashboard/monitoring"]}>
        <Routes>
          <Route path="/dashboard/monitoring" element={<MonitoringPage />} />
        </Routes>
      </MemoryRouter>,
    );

    await waitFor(() => {
      expect(screen.getByRole("button", { name: "Stop" })).toBeInTheDocument();
    });

    await userEvent.click(screen.getByRole("button", { name: "Stop" }));

    await waitFor(() => {
      expect(api.stopInstance).toHaveBeenCalledWith("inst_beta");
    });
    expect(api.fetchInstances).toHaveBeenCalled();
  });

  it("retries while waiting for an expected default instance", async () => {
    useAppStore.setState({
      instances: [],
      currentTabs: {},
      selectedMonitoringInstanceId: null,
    });

    render(
      <MemoryRouter initialEntries={["/dashboard/monitoring"]}>
        <Routes>
          <Route path="/dashboard/monitoring" element={<MonitoringPage />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(
      screen.getByText("Starting default instance..."),
    ).toBeInTheDocument();

    await waitFor(() => {
      expect(api.fetchInstances).toHaveBeenCalledTimes(1);
    });
  });

  it("shows manual actions when no auto-started instance is expected", async () => {
    vi.mocked(api.fetchBackendConfig).mockResolvedValueOnce({
      config: {
        multiInstance: { strategy: "no-instance" },
        profiles: { defaultProfile: "default" },
        instanceDefaults: { mode: "headed" },
      },
    } as never);
    useAppStore.setState({
      instances: [],
      currentTabs: {},
      selectedMonitoringInstanceId: null,
    });

    render(
      <MemoryRouter initialEntries={["/dashboard/monitoring"]}>
        <Routes>
          <Route path="/dashboard/monitoring" element={<MonitoringPage />} />
          <Route path="/dashboard/profiles" element={<ProfilesRouteProbe />} />
        </Routes>
      </MemoryRouter>,
    );

    expect(
      await screen.findByRole("button", { name: "Start Default Instance" }),
    ).toBeInTheDocument();
    expect(
      screen.getByRole("button", { name: "Open Default Profile" }),
    ).toBeInTheDocument();
  });

  it("starts the configured default instance from the empty state", async () => {
    vi.mocked(api.fetchBackendConfig).mockResolvedValueOnce({
      config: {
        multiInstance: { strategy: "no-instance" },
        profiles: { defaultProfile: "default" },
        instanceDefaults: { mode: "headed" },
      },
    } as never);
    useAppStore.setState({
      instances: [],
      currentTabs: {},
      selectedMonitoringInstanceId: null,
    });

    render(
      <MemoryRouter initialEntries={["/dashboard/monitoring"]}>
        <Routes>
          <Route path="/dashboard/monitoring" element={<MonitoringPage />} />
        </Routes>
      </MemoryRouter>,
    );

    await userEvent.click(
      await screen.findByRole("button", { name: "Start Default Instance" }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Start Headed" }),
    );

    await waitFor(() => {
      expect(api.launchInstance).toHaveBeenCalledWith({
        profileId: "default",
        mode: "headed",
      });
    });
    expect(api.fetchInstances).toHaveBeenCalled();
  });

  it("can start the default instance in headless mode", async () => {
    vi.mocked(api.fetchBackendConfig).mockResolvedValueOnce({
      config: {
        multiInstance: { strategy: "no-instance" },
        profiles: { defaultProfile: "default" },
        instanceDefaults: { mode: "headed" },
      },
    } as never);
    useAppStore.setState({
      instances: [],
      currentTabs: {},
      selectedMonitoringInstanceId: null,
    });

    render(
      <MemoryRouter initialEntries={["/dashboard/monitoring"]}>
        <Routes>
          <Route path="/dashboard/monitoring" element={<MonitoringPage />} />
        </Routes>
      </MemoryRouter>,
    );

    await userEvent.click(
      await screen.findByRole("button", { name: "Start Default Instance" }),
    );
    await userEvent.click(
      await screen.findByRole("button", { name: "Start Headless" }),
    );

    await waitFor(() => {
      expect(api.launchInstance).toHaveBeenCalledWith({
        profileId: "default",
        mode: undefined,
      });
    });
  });
});
