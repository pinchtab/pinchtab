import { render, screen } from "@testing-library/react";
import { describe, it, expect, beforeAll } from "vitest";
import TabsChart from "./TabsChart";
import type { TabDataPoint } from "../../stores/useAppStore";

// Mock ResizeObserver for Recharts ResponsiveContainer
beforeAll(() => {
  class ResizeObserverMock {
    observe() {}
    unobserve() {}
    disconnect() {}
  }
  window.ResizeObserver = ResizeObserverMock as any;
});

const mockInstances = [
  { id: "inst_1", profileName: "profile-1" },
  { id: "inst_2", profileName: "profile-2" },
];

const mockData: TabDataPoint[] = [
  { timestamp: Date.now() - 60000, inst_1: 3, inst_2: 5 },
  { timestamp: Date.now() - 30000, inst_1: 4, inst_2: 6 },
  { timestamp: Date.now(), inst_1: 5, inst_2: 4 },
];

describe("TabsChart", () => {
  describe("empty states", () => {
    it('shows "Collecting data..." when no data at all', () => {
      render(
        <TabsChart
          data={[]}
          instances={[]}
          selectedInstanceId={null}
          onSelectInstance={() => {}}
        />,
      );
      expect(screen.getByText("Collecting data...")).toBeInTheDocument();
    });

    it("renders chart even with no instances if data exists (e.g., server metrics)", () => {
      const { container } = render(
        <TabsChart
          data={mockData}
          instances={[]}
          selectedInstanceId={null}
          onSelectInstance={() => {}}
        />,
      );
      // Chart container should render, not empty state
      expect(
        container.querySelector(".recharts-responsive-container"),
      ).toBeInTheDocument();
    });
  });

  describe("chart rendering", () => {
    it("renders chart container when data and instances exist", () => {
      const { container } = render(
        <TabsChart
          data={mockData}
          instances={mockInstances}
          selectedInstanceId={null}
          onSelectInstance={() => {}}
        />,
      );
      // Should render the chart container (not the empty state)
      expect(
        container.querySelector(".recharts-responsive-container"),
      ).toBeInTheDocument();
    });

    it("plots memory-only samples whose timestamps are absent from tab data", () => {
      // No tab data at all — only memory samples at two timestamps. These used to
      // be dropped (mergedData anchored to tab/server data); with the union
      // timeline they form rows, so the chart renders instead of the empty state.
      const { container } = render(
        <TabsChart
          data={[]}
          memoryData={[
            { timestamp: Date.now() - 30000, inst_1: 120 },
            { timestamp: Date.now(), inst_1: 130 },
          ]}
          instances={mockInstances}
          selectedInstanceId={null}
          onSelectInstance={() => {}}
        />,
      );
      expect(
        container.querySelector(".recharts-responsive-container"),
      ).toBeInTheDocument();
      expect(screen.queryByText("Collecting data...")).not.toBeInTheDocument();
    });
  });
});
