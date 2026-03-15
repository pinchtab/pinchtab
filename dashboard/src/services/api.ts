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
  MonitoringServerMetrics,
  MonitoringSnapshot,
} from "../types";
import {
  normalizeBackendConfigState,
  normalizeDashboardServerInfo,
  normalizeMonitoringSnapshot,
} from "../types";
import {
  addTokenToUrl,
  dispatchAuthRequired,
  getStoredAuthToken,
} from "./auth";

const BASE = ""; // Uses proxy in dev

type RequestMeta = {
  authToken?: string;
  suppressAuthRedirect?: boolean;
};

async function request<T>(
  url: string,
  options?: RequestInit,
  meta?: RequestMeta,
): Promise<T> {
  const headers = new Headers(options?.headers ?? {});
  const token = meta?.authToken?.trim() || getStoredAuthToken();
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const res = await fetch(BASE + url, {
    ...options,
    headers,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    if (
      res.status === 401 &&
      !meta?.suppressAuthRedirect &&
      typeof window !== "undefined"
    ) {
      window.localStorage.removeItem("pinchtab.auth.token");
      dispatchAuthRequired(err.code || "unauthorized");
    }
    throw new Error(err.error || "Request failed");
  }
  return res.json();
}

async function requestText(
  url: string,
  options?: RequestInit,
  meta?: RequestMeta,
): Promise<string> {
  const headers = new Headers(options?.headers ?? {});
  const token = meta?.authToken?.trim() || getStoredAuthToken();
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const res = await fetch(BASE + url, {
    ...options,
    headers,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    if (
      res.status === 401 &&
      !meta?.suppressAuthRedirect &&
      typeof window !== "undefined"
    ) {
      window.localStorage.removeItem("pinchtab.auth.token");
      dispatchAuthRequired(err.code || "unauthorized");
    }
    throw new Error(err.error || "Request failed");
  }
  return res.text();
}

async function requestBlob(
  url: string,
  options?: RequestInit,
  meta?: RequestMeta,
): Promise<Blob> {
  const headers = new Headers(options?.headers ?? {});
  const token = meta?.authToken?.trim() || getStoredAuthToken();
  if (token) {
    headers.set("Authorization", `Bearer ${token}`);
  }

  const res = await fetch(BASE + url, {
    ...options,
    headers,
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({ error: res.statusText }));
    if (
      res.status === 401 &&
      !meta?.suppressAuthRedirect &&
      typeof window !== "undefined"
    ) {
      window.localStorage.removeItem("pinchtab.auth.token");
      dispatchAuthRequired(err.code || "unauthorized");
    }
    throw new Error(err.error || "Request failed");
  }
  return res.blob();
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

export async function fetchInstanceLogs(id: string): Promise<string> {
  return requestText(`/instances/${encodeURIComponent(id)}/logs`);
}

export async function fetchTabScreenshot(
  tabId: string,
  format: "jpeg" | "png" = "jpeg",
): Promise<Blob> {
  return requestBlob(
    `/tabs/${encodeURIComponent(tabId)}/screenshot?raw=true&format=${format}`,
  );
}

export async function fetchTabPdf(tabId: string): Promise<Blob> {
  return requestBlob(`/tabs/${encodeURIComponent(tabId)}/pdf?raw=true`);
}

export function subscribeToInstanceLogs(
  id: string,
  handlers: { onLogs?: (logs: string) => void },
): () => void {
  const url = addTokenToUrl(`/instances/${encodeURIComponent(id)}/logs/stream`);
  const es = new EventSource(url);

  es.addEventListener("log", (e) => {
    try {
      const payload = JSON.parse(e.data) as { logs?: string };
      handlers.onLogs?.(payload.logs || "");
    } catch {
      // ignore malformed events
    }
  });

  es.onerror = () => {
    if (!getStoredAuthToken()) {
      dispatchAuthRequired("missing_token");
    }
  };

  const cleanup = () => es.close();
  window.addEventListener("beforeunload", cleanup);

  return () => {
    window.removeEventListener("beforeunload", cleanup);
    es.close();
  };
}

export async function fetchAllTabs(): Promise<InstanceTab[]> {
  return request<InstanceTab[]>("/instances/tabs");
}

export async function fetchAllMetrics(): Promise<InstanceMetrics[]> {
  return request<InstanceMetrics[]>("/instances/metrics");
}

export async function fetchServerMetrics(): Promise<MonitoringServerMetrics> {
  const res = await request<{ metrics: MonitoringServerMetrics }>("/metrics");
  return res.metrics;
}

// Health
export async function fetchHealth(): Promise<DashboardServerInfo> {
  return normalizeDashboardServerInfo(
    await request<DashboardServerInfo>("/health"),
  );
}

export async function probeBackendAuth(): Promise<{
  requiresAuth: boolean;
  health?: DashboardServerInfo;
}> {
  const res = await fetch(BASE + "/health");
  if (res.ok) {
    return {
      requiresAuth: false,
      health: normalizeDashboardServerInfo(
        (await res.json()) as DashboardServerInfo,
      ),
    };
  }

  const err = (await res
    .json()
    .catch(() => ({ code: "", error: res.statusText }))) as {
    code?: string;
    error?: string;
  };
  if (
    res.status === 401 &&
    (err.code === "missing_token" ||
      err.code === "bad_token" ||
      err.error === "unauthorized")
  ) {
    return { requiresAuth: true };
  }

  throw new Error(err.error || "Request failed");
}

export async function verifyBackendToken(
  token: string,
): Promise<DashboardServerInfo> {
  return normalizeDashboardServerInfo(
    await request<DashboardServerInfo>("/health", undefined, {
      authToken: token,
      suppressAuthRedirect: true,
    }),
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

export async function generateBackendToken(): Promise<string> {
  const res = await request<{ token: string }>("/api/config/generate-token", {
    method: "POST",
  });
  return res.token;
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
  onMonitoring?: (snapshot: MonitoringSnapshot) => void;
};

export function subscribeToEvents(
  handlers: EventHandler,
  options?: { includeMemory?: boolean },
): () => void {
  const url = addTokenToUrl(
    options?.includeMemory ? "/api/events?memory=1" : "/api/events",
  );
  const es = new EventSource(url);

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

  es.addEventListener("monitoring", (e) => {
    try {
      const snapshot = normalizeMonitoringSnapshot(
        JSON.parse(e.data) as Partial<MonitoringSnapshot>,
      );
      handlers.onMonitoring?.(snapshot);
    } catch {
      // ignore
    }
  });

  // Suppress connection errors (expected on page reload/navigation)
  es.onerror = () => {
    if (!getStoredAuthToken()) {
      dispatchAuthRequired("missing_token");
    }
  };

  // Clean up on page unload to prevent ERR_INCOMPLETE_CHUNKED_ENCODING
  const cleanup = () => es.close();
  window.addEventListener("beforeunload", cleanup);

  return () => {
    window.removeEventListener("beforeunload", cleanup);
    es.close();
  };
}
