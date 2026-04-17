import { render, screen } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { MemoryRouter } from "react-router-dom";
import NavBar from "./NavBar";
import { useAppStore } from "../../stores/useAppStore";

vi.mock("../../services/api", () => ({
  logout: vi.fn(),
}));

describe("NavBar", () => {
  it("uses the first instance when none is selected", () => {
    useAppStore.setState({
      serverInfo: {
        status: "ok",
        mode: "dashboard",
        version: "1.0.0",
        uptime: 120000,
        profiles: 1,
        instances: 1,
        agents: 0,
        restartRequired: false,
        restartReasons: [],
      },
      instances: [
        {
          id: "inst_alpha",
          profileId: "prof_alpha",
          profileName: "alpha",
          port: "9988",
          mode: "headed",
          headless: false,
          status: "running",
          startTime: "2026-04-09T10:00:00Z",
          attached: false,
        },
      ],
      currentTabs: {
        inst_alpha: [],
      },
      selectedMonitoringInstanceId: null,
      monitoringSidebarCollapsed: true,
    });

    render(
      <MemoryRouter initialEntries={["/dashboard/profiles"]}>
        <NavBar />
      </MemoryRouter>,
    );

    expect(screen.getByTitle("alpha · running · 9988")).toBeInTheDocument();
  });
});
