import { create } from 'zustand'
import type {
  Profile,
  Instance,
  InstanceTab,
  Agent,
  ActivityEvent,
  Settings,
  ServerInfo,
} from '../generated/types'

export interface TabDataPoint {
  timestamp: number
  [instanceId: string]: number
}

interface AppState {
  // Profiles
  profiles: Profile[]
  profilesLoading: boolean
  setProfiles: (profiles: Profile[]) => void
  setProfilesLoading: (loading: boolean) => void

  // Instances
  instances: Instance[]
  instancesLoading: boolean
  setInstances: (instances: Instance[]) => void
  setInstancesLoading: (loading: boolean) => void

  // Chart data (persists across navigation)
  tabsChartData: TabDataPoint[]
  currentTabs: Record<string, InstanceTab[]>
  addChartDataPoint: (point: TabDataPoint) => void
  setCurrentTabs: (tabs: Record<string, InstanceTab[]>) => void

  // Agents
  agents: Agent[]
  selectedAgentId: string | null
  setAgents: (agents: Agent[]) => void
  setSelectedAgentId: (id: string | null) => void

  // Activity feed
  events: ActivityEvent[]
  eventFilter: string
  addEvent: (event: ActivityEvent) => void
  setEventFilter: (filter: string) => void
  clearEvents: () => void

  // Settings
  settings: Settings
  setSettings: (settings: Settings) => void

  // Server info
  serverInfo: ServerInfo | null
  setServerInfo: (info: ServerInfo | null) => void
}

const defaultSettings: Settings = {
  screencast: { fps: 1, quality: 30, maxWidth: 800 },
  stealth: 'light',
  browser: { blockImages: false, blockMedia: false, noAnimations: false },
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
  currentTabs: {},
  addChartDataPoint: (point) =>
    set((state) => ({
      tabsChartData: [...state.tabsChartData.slice(-59), point], // Keep last 60 points
    })),
  setCurrentTabs: (currentTabs) => set({ currentTabs }),

  // Agents
  agents: [],
  selectedAgentId: null,
  setAgents: (agents) => set({ agents }),
  setSelectedAgentId: (selectedAgentId) => set({ selectedAgentId }),

  // Activity feed
  events: [],
  eventFilter: 'all',
  addEvent: (event) =>
    set((state) => ({ events: [event, ...state.events].slice(0, 100) })),
  setEventFilter: (eventFilter) => set({ eventFilter }),
  clearEvents: () => set({ events: [] }),

  // Settings
  settings: defaultSettings,
  setSettings: (settings) => set({ settings }),

  // Server info
  serverInfo: null,
  setServerInfo: (serverInfo) => set({ serverInfo }),
}))
