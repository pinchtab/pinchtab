import type {
  Profile,
  Instance,
  Agent,
  Settings,
  ServerInfo,
  CreateProfileRequest,
  LaunchInstanceRequest,
} from '../generated/types'

const BASE = '' // Uses proxy in dev

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(BASE + url, options)
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }))
    throw new Error(err.error || 'Request failed')
  }
  return res.json()
}

// Profiles
export async function fetchProfiles(): Promise<Profile[]> {
  return request<Profile[]>('/api/profiles')
}

export async function createProfile(data: CreateProfileRequest): Promise<Profile> {
  return request<Profile>('/api/profiles', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export async function deleteProfile(id: string): Promise<void> {
  await request<void>(`/api/profiles/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

// Instances
export async function fetchInstances(): Promise<Instance[]> {
  return request<Instance[]>('/api/instances')
}

export async function launchInstance(data: LaunchInstanceRequest): Promise<Instance> {
  return request<Instance>('/api/instances', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export async function stopInstance(id: string): Promise<void> {
  await request<void>(`/api/instances/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

// Agents (from dashboard API)
export async function fetchAgents(): Promise<Agent[]> {
  return request<Agent[]>('/api/agents')
}

// Settings
export async function fetchSettings(): Promise<Settings> {
  return request<Settings>('/api/settings')
}

export async function updateSettings(settings: Settings): Promise<Settings> {
  return request<Settings>('/api/settings', {
    method: 'PUT',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(settings),
  })
}

// Health
export async function fetchHealth(): Promise<ServerInfo> {
  return request<ServerInfo>('/health')
}

// SSE Events
export interface SystemEvent {
  type: 'instance.started' | 'instance.stopped' | 'instance.error'
  instance?: Instance
}

export interface AgentEvent {
  agentId: string
  action: string
  url?: string
  timestamp: string
}

export type EventHandler = {
  onSystem?: (event: SystemEvent) => void
  onAgent?: (event: AgentEvent) => void
  onInit?: (agents: Agent[]) => void
}

export function subscribeToEvents(handlers: EventHandler): () => void {
  const es = new EventSource('/api/events')

  es.addEventListener('init', (e) => {
    try {
      const agents = JSON.parse(e.data) as Agent[]
      handlers.onInit?.(agents)
    } catch {
      // ignore
    }
  })

  es.addEventListener('system', (e) => {
    try {
      const event = JSON.parse(e.data) as SystemEvent
      handlers.onSystem?.(event)
    } catch {
      // ignore
    }
  })

  es.addEventListener('action', (e) => {
    try {
      const event = JSON.parse(e.data) as AgentEvent
      handlers.onAgent?.(event)
    } catch {
      // ignore
    }
  })

  return () => es.close()
}
