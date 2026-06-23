import type { StateCreator } from "zustand";
import type { InstanceTab, InstanceMetrics } from "../../generated/types";
import type { MonitoringSnapshot } from "../../types";
import type { AppState } from "../useAppStore";

export interface TabDataPoint {
  timestamp: number;
  [instanceId: string]: number;
}

export interface MemoryDataPoint {
  timestamp: number;
  [instanceId: string]: number; // jsHeapUsedMB
}

export interface ServerDataPoint {
  timestamp: number;
  goHeapMB: number;
  goroutines: number;
  rateBucketHosts: number;
}

export interface MonitoringSlice {
  // Chart data (persists across navigation)
  tabsChartData: TabDataPoint[];
  memoryChartData: MemoryDataPoint[];
  serverChartData: ServerDataPoint[];
  currentTabs: Record<string, InstanceTab[]>;
  currentMemory: Record<string, number>; // instanceId -> jsHeapUsedMB
  currentMetrics: Record<string, InstanceMetrics>; // instanceId -> full metrics
  addChartDataPoint: (point: TabDataPoint) => void;
  addMemoryDataPoint: (point: MemoryDataPoint) => void;
  addServerDataPoint: (point: ServerDataPoint) => void;
  setCurrentTabs: (tabs: Record<string, InstanceTab[]>) => void;
  setCurrentMemory: (memory: Record<string, number>) => void;
  applyMonitoringSnapshot: (
    snapshot: MonitoringSnapshot,
    includeMemory: boolean,
  ) => void;
}

export const createMonitoringSlice: StateCreator<
  AppState,
  [],
  [],
  MonitoringSlice
> = (set) => ({
  tabsChartData: [],
  memoryChartData: [],
  serverChartData: [],
  currentTabs: {},
  currentMemory: {},
  currentMetrics: {},
  addChartDataPoint: (point) =>
    set((state) => ({
      tabsChartData: [...state.tabsChartData.slice(-59), point], // Keep last 60 points
    })),
  addMemoryDataPoint: (point) =>
    set((state) => ({
      memoryChartData: [...state.memoryChartData.slice(-59), point], // Keep last 60 points
    })),
  addServerDataPoint: (point) =>
    set((state) => ({
      serverChartData: [...state.serverChartData.slice(-59), point], // Keep last 60 points
    })),
  setCurrentTabs: (currentTabs) => set({ currentTabs }),
  setCurrentMemory: (currentMemory) => set({ currentMemory }),
  applyMonitoringSnapshot: (snapshot, includeMemory) =>
    set((state) => {
      const runningInstances = snapshot.instances.filter(
        (instance) => instance?.status === "running",
      );

      // Index the snapshot once per tick so the per-instance loop is O(1)
      // lookups instead of O(instances × (tabs + metrics)) rescans. Push order
      // preserves the previous .filter ordering; the metrics index is first-wins
      // to match the previous .find, and is only built when memory is included.
      const tabsByInstance: Record<string, InstanceTab[]> = {};
      for (const tab of snapshot.tabs) {
        if (!tabsByInstance[tab.instanceId])
          tabsByInstance[tab.instanceId] = [];
        tabsByInstance[tab.instanceId].push(tab);
      }
      const metricsByInstance: Record<string, InstanceMetrics> = {};
      if (includeMemory) {
        for (const entry of snapshot.metrics) {
          if (!(entry.instanceId in metricsByInstance)) {
            metricsByInstance[entry.instanceId] = entry;
          }
        }
      }

      const tabDataPoint: TabDataPoint = { timestamp: snapshot.timestamp };
      const memDataPoint: MemoryDataPoint = { timestamp: snapshot.timestamp };
      const currentTabs: Record<string, InstanceTab[]> = {};
      const currentMemory: Record<string, number> = {};
      const currentMetrics: Record<string, InstanceMetrics> = {};

      for (const instance of runningInstances) {
        const instanceTabs = tabsByInstance[instance.id] ?? [];
        tabDataPoint[instance.id] = instanceTabs.length;
        currentTabs[instance.id] = instanceTabs;

        if (includeMemory) {
          const metrics = metricsByInstance[instance.id];
          if (metrics) {
            memDataPoint[instance.id] = metrics.jsHeapUsedMB;
            currentMemory[instance.id] = metrics.jsHeapUsedMB;
            currentMetrics[instance.id] = metrics;
          }
        }
      }

      return {
        instances: snapshot.instances,
        currentTabs,
        currentMemory,
        currentMetrics,
        tabsChartData:
          runningInstances.length > 0
            ? [...state.tabsChartData.slice(-59), tabDataPoint]
            : state.tabsChartData,
        memoryChartData:
          includeMemory && runningInstances.length > 0
            ? [...state.memoryChartData.slice(-59), memDataPoint]
            : state.memoryChartData,
        serverChartData: [
          ...state.serverChartData.slice(-59),
          {
            timestamp: snapshot.timestamp,
            goHeapMB: snapshot.serverMetrics.goHeapAllocMB,
            goroutines: snapshot.serverMetrics.goNumGoroutine,
            rateBucketHosts: snapshot.serverMetrics.rateBucketHosts,
          },
        ],
      };
    }),
});
