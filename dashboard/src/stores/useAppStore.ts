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
  agentEventsById: Record<string, ActivityEvent[]>;
  setAgents: (agents: Agent[]) => void;
  upsertAgentFromEvent: (event: ActivityEvent) => void;
  hydrateAgentEvents: (agentId: string, events: ActivityEvent[]) => void;
  appendAgentEvent: (event: ActivityEvent) => void;
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
const MAX_AGENT_CACHE_SIZE = 20;
const MAX_AGENT_EVENTS_PER_AGENT = 200;

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

function agentActivityTime(agent: Agent): number {
  return new Date(agent.lastActivity || agent.connectedAt).getTime();
}

function normalizeAgents(agents: Agent[]): Agent[] {
  const deduped = new Map<string, Agent>();
  for (const agent of agents) {
    deduped.set(agent.id, agent);
  }
  return [...deduped.values()].sort(
    (a, b) => agentActivityTime(b) - agentActivityTime(a),
  );
}

function retainedAgentIds(
  agents: Agent[],
  selectedAgentId: string | null,
  extraAgentIds: string[] = [],
): string[] {
  const ids: string[] = [];
  const seen = new Set<string>();
  const push = (id: string | null | undefined) => {
    const normalized = id?.trim();
    if (
      !normalized ||
      seen.has(normalized) ||
      ids.length >= MAX_AGENT_CACHE_SIZE
    ) {
      return;
    }
    seen.add(normalized);
    ids.push(normalized);
  };

  push(selectedAgentId);
  for (const id of extraAgentIds) {
    push(id);
  }
  for (const agent of normalizeAgents(agents)) {
    push(agent.id);
  }
  return ids;
}

function pruneAgentEventsById(
  agentEventsById: Record<string, ActivityEvent[]>,
  retainedIds: string[],
): Record<string, ActivityEvent[]> {
  const retained = new Set(retainedIds);
  return Object.fromEntries(
    Object.entries(agentEventsById).filter(([agentId]) =>
      retained.has(agentId),
    ),
  );
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
  agentEventsById: {},
  setAgents: (agents) =>
    set((state) => {
      const normalized = normalizeAgents(agents);
      const retainedIds = retainedAgentIds(normalized, state.selectedAgentId);
      const retained = new Set(retainedIds);
      return {
        agents: normalized.filter((agent) => retained.has(agent.id)),
        agentEventsById: pruneAgentEventsById(
          state.agentEventsById,
          retainedIds,
        ),
      };
    }),
  upsertAgentFromEvent: (event) =>
    set((state) => {
      const agentId = event.agentId?.trim();
      if (!agentId) {
        return state;
      }
      const existing = state.agents.find((agent) => agent.id === agentId);

      if (!existing) {
        const nextAgents = normalizeAgents([
          {
            id: agentId,
            name: agentId,
            connectedAt: event.timestamp,
            lastActivity: event.timestamp,
            requestCount: 1,
          },
          ...state.agents,
        ]);
        const retainedIds = retainedAgentIds(
          nextAgents,
          state.selectedAgentId,
          [agentId],
        );
        const retained = new Set(retainedIds);
        return {
          agents: nextAgents.filter((agent) => retained.has(agent.id)),
          agentEventsById: pruneAgentEventsById(
            state.agentEventsById,
            retainedIds,
          ),
        };
      }

      const nextAgents = normalizeAgents(
        state.agents.map((agent) =>
          agent.id === agentId
            ? {
                ...agent,
                lastActivity: event.timestamp,
                requestCount: agent.requestCount + 1,
              }
            : agent,
        ),
      );
      const retainedIds = retainedAgentIds(nextAgents, state.selectedAgentId, [
        agentId,
      ]);
      const retained = new Set(retainedIds);
      return {
        agents: nextAgents.filter((agent) => retained.has(agent.id)),
        agentEventsById: pruneAgentEventsById(
          state.agentEventsById,
          retainedIds,
        ),
      };
    }),
  hydrateAgentEvents: (agentId, events) =>
    set((state) => {
      const retainedIds = retainedAgentIds(
        state.agents,
        state.selectedAgentId,
        [agentId],
      );
      return {
        agentEventsById: {
          ...pruneAgentEventsById(state.agentEventsById, retainedIds),
          [agentId]: [...(state.agentEventsById[agentId] ?? []), ...events]
            .reduce<ActivityEvent[]>((merged, event) => {
              if (merged.some((existing) => existing.id === event.id)) {
                return merged;
              }
              merged.push(event);
              return merged;
            }, [])
            .sort(
              (left, right) =>
                new Date(left.timestamp).getTime() -
                new Date(right.timestamp).getTime(),
            )
            .slice(-MAX_AGENT_EVENTS_PER_AGENT),
        },
      };
    }),
  appendAgentEvent: (event) =>
    set((state) => {
      const agentId = event.agentId?.trim();
      if (!agentId) {
        return state;
      }
      const current = state.agentEventsById[agentId] ?? [];
      if (current.some((existing) => existing.id === event.id)) {
        return state;
      }
      const next = [...current, event]
        .sort(
          (left, right) =>
            new Date(left.timestamp).getTime() -
            new Date(right.timestamp).getTime(),
        )
        .slice(-MAX_AGENT_EVENTS_PER_AGENT);
      const retainedIds = retainedAgentIds(
        state.agents,
        state.selectedAgentId,
        [agentId],
      );
      return {
        agentEventsById: {
          ...pruneAgentEventsById(state.agentEventsById, retainedIds),
          [agentId]: next,
        },
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
