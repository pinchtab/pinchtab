import { create } from 'zustand'
import type { View, Profile, Instance, Agent, ActivityEvent, Settings, ServerInfo } from '../types'

interface AppState {
  // Navigation
  view: View
  setView: (view: View) => void

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
  // Navigation
  view: 'profiles',
  setView: (view) => set({ view }),

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
