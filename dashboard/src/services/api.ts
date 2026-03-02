import type {
  Profile,
  Instance,
  Agent,
  Settings,
  ServerInfo,
  CreateProfileRequest,
  CreateProfileResponse,
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

// Profiles — endpoint is /profiles (no /api prefix)
export async function fetchProfiles(): Promise<Profile[]> {
  return request<Profile[]>('/profiles')
}

export async function createProfile(data: CreateProfileRequest): Promise<CreateProfileResponse> {
  return request<CreateProfileResponse>('/profiles', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export async function deleteProfile(id: string): Promise<void> {
  await request<void>(`/profiles/${encodeURIComponent(id)}`, {
    method: 'DELETE',
  })
}

// Instances — endpoint is /instances (no /api prefix)
export async function fetchInstances(): Promise<Instance[]> {
  return request<Instance[]>('/instances')
}

export async function launchInstance(data: LaunchInstanceRequest): Promise<Instance> {
  return request<Instance>('/instances/launch', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify(data),
  })
}

export async function stopInstance(id: string): Promise<void> {
  await request<void>(`/instances/${encodeURIComponent(id)}/stop`, {
    method: 'POST',
  })
}

// Agents — endpoint is /api/agents (dashboard API)
export async function fetchAgents(): Promise<Agent[]> {
  return request<Agent[]>('/api/agents')
}

// Settings — TODO: check if endpoint exists
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

// SSE Events — endpoint is /api/events
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
