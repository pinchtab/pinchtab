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

  it("stays silent about crashes for an instance that never crashed", () => {
    render(<InstanceStats instance={instance} tabs={[]} />);
    expect(screen.getByText("running")).toBeInTheDocument();
    expect(screen.queryByText("Crashes")).not.toBeInTheDocument();
  });
});
