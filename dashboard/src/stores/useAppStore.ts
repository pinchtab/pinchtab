import { create } from "zustand";
import type { Profile, Instance, ActivityEvent } from "../generated/types";
import type { DashboardServerInfo } from "../types";
import {
  createMonitoringSlice,
  type MonitoringSlice,
} from "./slices/monitoringSlice";
import { createAgentsSlice, type AgentsSlice } from "./slices/agentsSlice";
import {
  createSettingsSlice,
  type SettingsSlice,
} from "./slices/settingsSlice";

export type {
  TabDataPoint,
  MemoryDataPoint,
  ServerDataPoint,
} from "./slices/monitoringSlice";

export interface HandoffNotification {
  tabId: string;
  reason: string;
  hint?: string;
  source?: string;
  url?: string;
  title?: string;
  receivedAt: number;
}

interface ProfilesSlice {
  profiles: Profile[];
  profilesLoading: boolean;
  setProfiles: (profiles: Profile[]) => void;
  setProfilesLoading: (loading: boolean) => void;
}

interface InstancesSlice {
  instances: Instance[];
  instancesLoading: boolean;
  setInstances: (instances: Instance[]) => void;
  setInstancesLoading: (loading: boolean) => void;
}

interface ActivitySlice {
  events: ActivityEvent[];
  eventFilter: string;
  addEvent: (event: ActivityEvent) => void;
  setEventFilter: (filter: string) => void;
  clearEvents: () => void;
}

interface ServerInfoSlice {
  serverInfo: DashboardServerInfo | null;
  setServerInfo: (info: DashboardServerInfo | null) => void;
}

interface HandoffSlice {
  handoffNotifications: HandoffNotification[];
  addHandoffNotification: (notification: HandoffNotification) => void;
  dismissHandoffNotification: (tabId: string) => void;
  clearHandoffNotifications: () => void;
}

interface MonitoringUiSlice {
  monitoringSidebarCollapsed: boolean;
  setMonitoringSidebarCollapsed: (collapsed: boolean) => void;
  selectedMonitoringInstanceId: string | null;
  setSelectedMonitoringInstanceId: (id: string | null) => void;
  monitoringShowTelemetry: boolean;
  setMonitoringShowTelemetry: (show: boolean) => void;
}

export type AppState = ProfilesSlice &
  InstancesSlice &
  MonitoringSlice &
  AgentsSlice &
  ActivitySlice &
  SettingsSlice &
  ServerInfoSlice &
  HandoffSlice &
  MonitoringUiSlice;

export const useAppStore = create<AppState>()((...a) => {
  const [set] = a;
  return {
    ...createMonitoringSlice(...a),
    ...createAgentsSlice(...a),
    ...createSettingsSlice(...a),

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

    // Activity feed
    events: [],
    eventFilter: "all",
    addEvent: (event) =>
      set((state) => ({ events: [event, ...state.events].slice(0, 100) })),
    setEventFilter: (eventFilter) => set({ eventFilter }),
    clearEvents: () => set({ events: [] }),

    // Server info
    serverInfo: null,
    setServerInfo: (serverInfo) => set({ serverInfo }),

    // Handoff notifications
    handoffNotifications: [],
    addHandoffNotification: (notification) =>
      set((state) => {
        const filtered = state.handoffNotifications.filter(
          (n) => n.tabId !== notification.tabId,
        );
        return { handoffNotifications: [...filtered, notification] };
      }),
    dismissHandoffNotification: (tabId) =>
      set((state) => ({
        handoffNotifications: state.handoffNotifications.filter(
          (n) => n.tabId !== tabId,
        ),
      })),
    clearHandoffNotifications: () => set({ handoffNotifications: [] }),

    // Monitoring UI
    monitoringSidebarCollapsed: true,
    setMonitoringSidebarCollapsed: (monitoringSidebarCollapsed) =>
      set({ monitoringSidebarCollapsed }),
    selectedMonitoringInstanceId: null,
    setSelectedMonitoringInstanceId: (selectedMonitoringInstanceId) =>
      set({ selectedMonitoringInstanceId }),
    monitoringShowTelemetry: true,
    setMonitoringShowTelemetry: (monitoringShowTelemetry) =>
      set({ monitoringShowTelemetry }),
  };
});
