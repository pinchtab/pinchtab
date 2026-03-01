// Profile stored on disk
export interface Profile {
  name: string
  path: string
  useWhen?: string
  createdAt?: string
}

// Running browser instance
export interface Instance {
  id: string
  profileName: string
  port: number
  pid: number
  headless: boolean
  startedAt: string
  tabs?: number
}

// Connected agent
export interface Agent {
  id: string
  name?: string
  connectedAt: string
  lastActivity?: string
  requestCount: number
}

// Activity feed event
export interface ActivityEvent {
  id: string
  agentId: string
  type: 'navigate' | 'snapshot' | 'action' | 'screenshot' | 'other'
  method: string
  path: string
  timestamp: string
  details?: Record<string, unknown>
}

// Settings
export interface Settings {
  screencast: {
    fps: number
    quality: number
    maxWidth: number
  }
  stealth: 'light' | 'full'
  browser: {
    blockImages: boolean
    blockMedia: boolean
    noAnimations: boolean
  }
}

// Server health info
export interface ServerInfo {
  version: string
  uptime: number
  profiles: number
  instances: number
  agents: number
}

// View type
export type View = 'profiles' | 'instances' | 'agents' | 'settings'
