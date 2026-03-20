import { create } from "zustand";
import type {
  Profile,
  Instance,
  InstanceTab,
  Agent,
  ActivityEvent,
  Settings,
} from "../generated/types";
import type { DashboardServerInfo, MonitoringSnapshot } from "../types";

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

interface AppState {
  // Profiles
  profiles: Profile[];
  profilesLoading: boolean;
  setProfiles: (profiles: Profile[]) => void;
  setProfilesLoading: (loading: boolean) => void;

  // Instances
  instances: Instance[];
  instancesLoading: boolean;
  setInstances: (instances: Instance[]) => void;
  setInstancesLoading: (loading: boolean) => void;

  // Chart data (persists across navigation)
  tabsChartData: TabDataPoint[];
  memoryChartData: MemoryDataPoint[];
  serverChartData: ServerDataPoint[];
  currentTabs: Record<string, InstanceTab[]>;
  currentMemory: Record<string, number>; // instanceId -> jsHeapUsedMB
  addChartDataPoint: (point: TabDataPoint) => void;
  addMemoryDataPoint: (point: MemoryDataPoint) => void;
  addServerDataPoint: (point: ServerDataPoint) => void;
  setCurrentTabs: (tabs: Record<string, InstanceTab[]>) => void;
  setCurrentMemory: (memory: Record<string, number>) => void;
  applyMonitoringSnapshot: (
    snapshot: MonitoringSnapshot,
    includeMemory: boolean,
  ) => void;

  // Agents
  agents: Agent[];
  selectedAgentId: string | null;
  setAgents: (agents: Agent[]) => void;
  upsertAgentFromEvent: (event: ActivityEvent) => void;
  setSelectedAgentId: (id: string | null) => void;

  // Activity feed
  events: ActivityEvent[];
  eventFilter: string;
  addEvent: (event: ActivityEvent) => void;
  setEventFilter: (filter: string) => void;
  clearEvents: () => void;

  // Settings
  settings: Settings;
  setSettings: (settings: Settings) => void;

  // Server info
  serverInfo: DashboardServerInfo | null;
  setServerInfo: (info: DashboardServerInfo | null) => void;
}

const defaultSettings: Settings = {
  screencast: { fps: 1, quality: 40, maxWidth: 800 },
  stealth: "light",
  browser: { blockImages: false, blockMedia: false, noAnimations: false },
  monitoring: { memoryMetrics: false, pollInterval: 30 },
  agents: { reasoningMode: "tool_calls" },
};

const SETTINGS_KEY = "pinchtab_settings";

function loadSettings(): Settings {
  try {
    const saved = localStorage.getItem(SETTINGS_KEY);
    if (saved) {
      return { ...defaultSettings, ...JSON.parse(saved) };
    }
  } catch {
    // ignore parse errors
  }
  return defaultSettings;
}

function saveSettings(settings: Settings) {
  try {
    localStorage.setItem(SETTINGS_KEY, JSON.stringify(settings));
  } catch {
    // ignore storage errors
  }
}

export const useAppStore = create<AppState>((set) => ({
  // Profiles
  profiles: [],
  profilesLoading: false,
  setProfiles: (profiles) => set({ profiles }),
  setProfilesLoading: (profilesLoading) => set({ profilesLoading }),

  // Instances
  instances: [],
  instancesLoading: false,
  setInstances: (instances) => set({ instances }),
  setInstancesLoading: (instancesLoading) => set({ instancesLoading }),

  // Chart data
  tabsChartData: [],
  memoryChartData: [],
  serverChartData: [],
  currentTabs: {},
  currentMemory: {},
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
      const tabDataPoint: TabDataPoint = { timestamp: snapshot.timestamp };
      const memDataPoint: MemoryDataPoint = { timestamp: snapshot.timestamp };
      const currentTabs: Record<string, InstanceTab[]> = {};
      const currentMemory: Record<string, number> = {};

      for (const instance of runningInstances) {
        const instanceTabs = snapshot.tabs.filter(
          (tab) => tab.instanceId === instance.id,
        );
        tabDataPoint[instance.id] = instanceTabs.length;
        currentTabs[instance.id] = instanceTabs;

        if (includeMemory) {
          const metrics = snapshot.metrics.find(
            (entry) => entry.instanceId === instance.id,
          );
          if (metrics) {
            memDataPoint[instance.id] = metrics.jsHeapUsedMB;
            currentMemory[instance.id] = metrics.jsHeapUsedMB;
          }
        }
      }

      return {
        instances: snapshot.instances,
        currentTabs,
        currentMemory,
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

  // Agents
  agents: [],
  selectedAgentId: null,
  setAgents: (agents) => set({ agents }),
  upsertAgentFromEvent: (event) =>
    set((state) => {
      const agentId = event.agentId || "anonymous";
      const existing = state.agents.find((agent) => agent.id === agentId);

      if (!existing) {
        return {
          agents: [
            {
              id: agentId,
              name: agentId,
              connectedAt: event.timestamp,
              lastActivity: event.timestamp,
              requestCount: 1,
            },
            ...state.agents,
          ],
        };
      }

      return {
        agents: state.agents
          .map((agent) =>
            agent.id === agentId
              ? {
                  ...agent,
                  lastActivity: event.timestamp,
                  requestCount: agent.requestCount + 1,
                }
              : agent,
          )
          .sort(
            (a, b) =>
              new Date(b.lastActivity || b.connectedAt).getTime() -
              new Date(a.lastActivity || a.connectedAt).getTime(),
          ),
      };
    }),
  setSelectedAgentId: (selectedAgentId) => set({ selectedAgentId }),

  // Activity feed
  events: [],
  eventFilter: "all",
  addEvent: (event) =>
    set((state) => ({ events: [event, ...state.events].slice(0, 100) })),
  setEventFilter: (eventFilter) => set({ eventFilter }),
  clearEvents: () => set({ events: [] }),

  // Settings (persisted to localStorage)
  settings: loadSettings(),
  setSettings: (settings) => {
    saveSettings(settings);
    set({ settings });
  },

  // Server info
  serverInfo: null,
  setServerInfo: (serverInfo) => set({ serverInfo }),
}));
