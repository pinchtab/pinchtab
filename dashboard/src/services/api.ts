import type {
  Profile,
  Instance,
  InstanceTab,
  InstanceMetrics,
  Agent,
  CreateProfileRequest,
  CreateProfileResponse,
  LaunchInstanceRequest,
} from "../generated/types";
import type {
  BackendConfig,
  BackendConfigState,
  DashboardServerInfo,
} from "../types";
import {
  normalizeBackendConfigState,
  normalizeDashboardServerInfo,
} from "../types";

const BASE = ""; // Uses proxy in dev

async function request<T>(url: string, options?: RequestInit): Promise<T> {
  const res = await fetch(BASE + url, options);
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    throw new Error(err.error || "Request failed");
  }
  return res.json();
}

// Profiles — endpoint is /profiles (no /api prefix)
export async function fetchProfiles(): Promise<Profile[]> {
  return request<Profile[]>("/profiles");
}

export async function createProfile(
  data: CreateProfileRequest,
): Promise<CreateProfileResponse> {
  return request<CreateProfileResponse>("/profiles", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function deleteProfile(id: string): Promise<void> {
  await request<void>(`/profiles/${encodeURIComponent(id)}`, {
    method: "DELETE",
  });
}

export interface UpdateProfileRequest {
  name?: string;
  useWhen?: string;
  description?: string;
}

export interface UpdateProfileResponse {
  status: string;
  id: string;
  name: string;
}

export async function updateProfile(
  id: string,
  data: UpdateProfileRequest,
): Promise<UpdateProfileResponse> {
  return request<UpdateProfileResponse>(`/profiles/${encodeURIComponent(id)}`, {
    method: "PATCH",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

// Instances — endpoint is /instances (no /api prefix)
export async function fetchInstances(): Promise<Instance[]> {
  return request<Instance[]>("/instances");
}

export async function launchInstance(
  data: LaunchInstanceRequest,
): Promise<Instance> {
  return request<Instance>("/instances/launch", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(data),
  });
}

export async function stopInstance(id: string): Promise<void> {
  await request<void>(`/instances/${encodeURIComponent(id)}/stop`, {
    method: "POST",
  });
}

export async function fetchInstanceTabs(id: string): Promise<InstanceTab[]> {
  return request<InstanceTab[]>(`/instances/${encodeURIComponent(id)}/tabs`);
}

export async function fetchAllTabs(): Promise<InstanceTab[]> {
  return request<InstanceTab[]>("/instances/tabs");
}

export async function fetchAllMetrics(): Promise<InstanceMetrics[]> {
  return request<InstanceMetrics[]>("/instances/metrics");
}

export interface ServerMetrics {
  goHeapAllocMB: number;
  goNumGoroutine: number;
  rateBucketHosts: number;
}

export async function fetchServerMetrics(): Promise<ServerMetrics> {
  const res = await request<{ metrics: ServerMetrics }>("/metrics");
  return res.metrics;
}

// Health
export async function fetchHealth(): Promise<DashboardServerInfo> {
  return normalizeDashboardServerInfo(
    await request<DashboardServerInfo>("/health"),
  );
}

export async function fetchBackendConfig(): Promise<BackendConfigState> {
  return normalizeBackendConfigState(
    await request<BackendConfigState>("/api/config"),
  );
}

export async function saveBackendConfig(
  config: BackendConfig,
): Promise<BackendConfigState> {
  return normalizeBackendConfigState(
    await request<BackendConfigState>("/api/config", {
      method: "PUT",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(config),
    }),
  );
}

// SSE Events — endpoint is /api/events
export interface SystemEvent {
  type: "instance.started" | "instance.stopped" | "instance.error";
  instance?: Instance;
}

export interface AgentEvent {
  agentId: string;
  action: string;
  url?: string;
  timestamp: string;
}

export type EventHandler = {
  onSystem?: (event: SystemEvent) => void;
  onAgent?: (event: AgentEvent) => void;
  onInit?: (agents: Agent[]) => void;
};

export function subscribeToEvents(handlers: EventHandler): () => void {
  const es = new EventSource("/api/events");

  es.addEventListener("init", (e) => {
    try {
      const agents = JSON.parse(e.data) as Agent[];
      handlers.onInit?.(agents);
    } catch {
      // ignore
    }
  });

  es.addEventListener("system", (e) => {
    try {
      const event = JSON.parse(e.data) as SystemEvent;
      handlers.onSystem?.(event);
    } catch {
      // ignore
    }
  });

  es.addEventListener("action", (e) => {
    try {
      const event = JSON.parse(e.data) as AgentEvent;
      handlers.onAgent?.(event);
    } catch {
      // ignore
    }
  });

  // Suppress connection errors (expected on page reload/navigation)
  es.onerror = () => {
    // SSE will auto-reconnect; silence console noise
  };

  // Clean up on page unload to prevent ERR_INCOMPLETE_CHUNKED_ENCODING
  const cleanup = () => es.close();
  window.addEventListener("beforeunload", cleanup);

  return () => {
    window.removeEventListener("beforeunload", cleanup);
    es.close();
  };
}
