import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import TabsChart2 from "./TabsChart2";
import type { TabDataPoint } from "../../stores/useAppStore";

const mockInstances = [
  { id: "inst_1", profileName: "profile-1" },
  { id: "inst_2", profileName: "profile-2" },
];

const mockData: TabDataPoint[] = [
  { timestamp: Date.now() - 60000, inst_1: 3, inst_2: 5 },
  { timestamp: Date.now() - 30000, inst_1: 4, inst_2: 6 },
  { timestamp: Date.now(), inst_1: 5, inst_2: 4 },
];

describe("TabsChart2", () => {
  it('shows "Collecting data..." when no data exists', () => {
    render(
      <TabsChart2
        data={[]}
        instances={[]}
        selectedInstanceId={null}
        onSelectInstance={() => {}}
      />,
    );

    expect(screen.getByText("Collecting data...")).toBeInTheDocument();
  });

  it("renders the terminal chart shell when data exists", () => {
    render(
      <TabsChart2
        data={mockData}
        instances={mockInstances}
        selectedInstanceId={null}
        onSelectInstance={() => {}}
      />,
    );

    expect(screen.getByTestId("terminal-tabs-chart")).toBeInTheDocument();
    expect(screen.getByText("Terminal telemetry")).toBeInTheDocument();
    expect(screen.getByText("Tabs")).toBeInTheDocument();
  });
});
