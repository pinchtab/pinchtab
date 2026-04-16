import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { DashboardServerInfo } from "../../types";
import type { Instance } from "../../generated/types";
import ServerStatusBadge from "./ServerStatusBadge";

const runningInstance: Instance = {
  id: "inst_alpha",
  profileId: "prof_alpha",
  profileName: "alpha",
  port: "9988",
  mode: "headed",
  headless: false,
  status: "running",
  startTime: "2026-04-09T10:00:00Z",
  attached: false,
};

const serverInfo: DashboardServerInfo = {
  status: "ok",
  mode: "dashboard",
  version: "1.0.0",
  uptime: 120000,
  profiles: 1,
  instances: 1,
  agents: 0,
  restartRequired: false,
  restartReasons: [],
};

describe("ServerStatusBadge", () => {
  it("hides the port in compact mode", () => {
    const { container } = render(
      <ServerStatusBadge
        serverInfo={serverInfo}
        instance={runningInstance}
        compact
        hasRunningInstance
      />,
    );

    expect(screen.queryByText("9988")).not.toBeInTheDocument();
    expect(container.querySelector(".bg-success")).toBeInTheDocument();
  });

  it("shows only a warning dot when the server is up but no instance is running", () => {
    const { container } = render(
      <ServerStatusBadge
        serverInfo={serverInfo}
        instance={{
          ...runningInstance,
          status: "starting",
        }}
        compact
        hasRunningInstance={false}
      />,
    );

    expect(screen.queryByText("Running")).not.toBeInTheDocument();
    expect(screen.queryByText("9988")).not.toBeInTheDocument();
    expect(container.querySelector(".bg-warning")).toBeInTheDocument();
  });

  it("shows the detailed instance summary outside compact mode", () => {
    render(
      <ServerStatusBadge
        serverInfo={serverInfo}
        instance={runningInstance}
        tabCount={3}
        hasRunningInstance
      />,
    );

    expect(screen.getByText("alpha ·")).toBeInTheDocument();
    expect(screen.getByText("running · 3 tabs ·")).toBeInTheDocument();
    expect(screen.getByText("9988")).toBeInTheDocument();
  });
});
