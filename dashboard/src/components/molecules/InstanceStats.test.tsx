import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import type { Instance } from "../../generated/types";
import InstanceStats from "./InstanceStats";

const instance: Instance = {
  id: "inst_1",
  profileId: "prof_1",
  profileName: "default",
  port: "9868",
  mode: "headless",
  headless: true,
  status: "running",
  startTime: new Date().toISOString(),
  attached: false,
};

describe("InstanceStats", () => {
  it("shows the crash count and the last reason for an instance whose browser crashed", () => {
    render(
      <InstanceStats
        instance={{
          ...instance,
          crashes: {
            total: 2,
            recent: [
              {
                time: "2026-09-03T08:48:11Z",
                reason: "inspector.targetCrashed",
              },
              {
                time: "2026-09-03T08:49:47Z",
                reason: "unexpected context cancellation",
              },
            ],
          },
        }}
        tabs={[]}
      />,
    );
    expect(screen.getByText("Crashes")).toBeInTheDocument();
    expect(screen.getByText("2")).toBeInTheDocument();
    expect(
      screen.getByText(/last: unexpected context cancellation at/),
    ).toBeInTheDocument();
  });

  it("shows the summed page counters and the tabs that did not answer", () => {
    render(
      <InstanceStats
        instance={instance}
        metrics={{
          instanceId: "inst_1",
          profileName: "default",
          memoryMB: 512,
          renderers: 3,
          unreadableTargets: 1,
          page: {
            targets: 2,
            jsHeapUsedMB: 12.5,
            jsHeapTotalMB: 20,
            documents: 2,
            frames: 3,
            nodes: 6400,
            jsEventListeners: 51,
          },
        }}
        tabs={[]}
      />,
    );
    expect(screen.getByText("12.5 / 20 MB")).toBeInTheDocument();
    expect(
      screen.getByText("used / total, summed over 2 tabs"),
    ).toBeInTheDocument();
    expect(screen.getByText("6,400")).toBeInTheDocument();
    expect(screen.getByText("51")).toBeInTheDocument();
    expect(screen.getByText("Unreadable")).toBeInTheDocument();
  });

  it("shows no page group when no tab could be read and none failed", () => {
    render(
      <InstanceStats
        instance={instance}
        metrics={{
          instanceId: "inst_1",
          profileName: "default",
          memoryMB: 512,
          renderers: 3,
          unreadableTargets: 0,
        }}
        tabs={[]}
      />,
    );
    expect(screen.queryByText("Pages")).not.toBeInTheDocument();
    expect(screen.queryByText("Unreadable")).not.toBeInTheDocument();
  });

  it("stays silent about crashes for an instance that never crashed", () => {
    render(<InstanceStats instance={instance} tabs={[]} />);
    expect(screen.getByText("running")).toBeInTheDocument();
    expect(screen.queryByText("Crashes")).not.toBeInTheDocument();
  });
});
