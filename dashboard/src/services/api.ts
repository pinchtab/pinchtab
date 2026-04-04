import type {
  Profile,
  Instance,
  InstanceTab,
  InstanceMetrics,
  Agent,
  AgentDetail,
  ActivityEvent,
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
import type {
  ActivityQuery,
  DashboardActivityResponse,
} from "../activities/types";
import {
  normalizeBackendConfigState,
  normalizeDashboardServerInfo,
  normalizeMonitoringSnapshot,
} from "../types";
import { dispatchAuthRequired, sameOriginUrl } from "./auth";

const BASE = ""; // Uses proxy in dev
const DASHBOARD_SOURCE_HEADER = "X-PinchTab-Source";
const DASHBOARD_SOURCE = "dashboard";

type RequestMeta = {
  suppressAuthRedirect?: boolean;
};

export class ApiError extends Error {
  status: number;
  code?: string;

  constructor(message: string, status: number, code?: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

export function isApiError(error: unknown): error is ApiError {
  return error instanceof ApiError;
}

async function parseError(
  res: Response,
): Promise<{ code?: string; error?: string }> {
  return (await res
    .json()
    .catch(() => ({ code: "", error: res.statusText }))) as {
    code?: string;
    error?: string;
  };
}

function handleUnauthorized(meta?: RequestMeta, reason?: string): void {
  if (meta?.suppressAuthRedirect || typeof window === "undefined") {
    return;
  }
  dispatchAuthRequired(reason || "unauthorized");
}

async function request<T>(
  url: string,
  options?: RequestInit,
  meta?: RequestMeta,
): Promise<T> {
  const res = await fetch(BASE + url, {
    ...withDashboardSource(options),
    credentials: "same-origin",
  });
  if (!res.ok) {
    const err = await parseError(res);
    if (res.status === 401) {
      handleUnauthorized(meta, err.code);
    }
    throw new ApiError(err.error || "Request failed", res.status, err.code);
  }
  return res.json();
}

async function requestText(
  url: string,
  options?: RequestInit,
  meta?: RequestMeta,
): Promise<string> {
  const res = await fetch(BASE + url, {
    ...withDashboardSource(options),
    credentials: "same-origin",
  });
  if (!res.ok) {
    const err = await parseError(res);
    if (res.status === 401) {
      handleUnauthorized(meta, err.code);
    }
    throw new ApiError(err.error || "Request failed", res.status, err.code);
  }
  return res.text();
}

async function requestBlob(
  url: string,
  options?: RequestInit,
  meta?: RequestMeta,
): Promise<Blob> {
  const res = await fetch(BASE + url, {
    ...withDashboardSource(options),
    credentials: "same-origin",
  });
  if (!res.ok) {
    const err = await parseError(res);
    if (res.status === 401) {
      handleUnauthorized(meta, err.code);
    }
    throw new ApiError(err.error || "Request failed", res.status, err.code);
  }
  return res.blob();
}

function withDashboardSource(options?: RequestInit): RequestInit {
  const headers = new Headers(options?.headers);
  headers.set(DASHBOARD_SOURCE_HEADER, DASHBOARD_SOURCE);
  return {
    ...options,
    headers,
  };
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

export async function closeTab(tabId: string): Promise<void> {
  await request(`/tabs/${encodeURIComponent(tabId)}/close`, { method: "POST" });
}

export async function navigateTab(
  tabId: string,
  url: string,
): Promise<unknown> {
  return request("/navigate", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ tabId, url }),
  });
}

export async function sendAction(
  body: Record<string, unknown>,
): Promise<unknown> {
  return request("/action", {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(body),
  });
}

export function subscribeToInstanceLogs(
  id: string,
  handlers: { onLogs?: (logs: string) => void },
): () => void {
  const url = sameOriginUrl(`/instances/${encodeURIComponent(id)}/logs/stream`);
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
    void handleRealtimeAuthFailure();
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

export async function fetchAgents(): Promise<Agent[]> {
  return request<Agent[]>("/api/agents");
}

export interface Session {
  id: string;
  agentId: string;
  label?: string;
  createdAt: string;
  lastSeenAt: string;
  expiresAt: string;
  status: string;
}

export async function fetchSessions(): Promise<Session[]> {
  return request<Session[]>("/api/sessions");
}

export async function fetchAgent(
  id: string,
  mode?: string,
): Promise<AgentDetail> {
  const params = new URLSearchParams();
  if (mode) {
    params.set("mode", mode);
  }
  const suffix = params.size > 0 ? `?${params.toString()}` : "";
  return request<AgentDetail>(`/api/agents/${encodeURIComponent(id)}${suffix}`);
}

export async function fetchServerMetrics(): Promise<MonitoringServerMetrics> {
  const res = await request<{ metrics: MonitoringServerMetrics }>(
    "/api/metrics",
  );
  return res.metrics;
}

// Health
export async function fetchHealth(): Promise<DashboardServerInfo> {
  return normalizeDashboardServerInfo(
    await request<DashboardServerInfo>("/health"),
  );
}

export async function fetchActivity(
  query?: ActivityQuery,
): Promise<DashboardActivityResponse> {
  const params = new URLSearchParams();
  if (query) {
    for (const [key, value] of Object.entries(query)) {
      if (value === undefined || value === null || value === "") {
        continue;
      }
      params.set(key, String(value));
    }
  }
  const suffix = params.size > 0 ? `?${params.toString()}` : "";
  return request<DashboardActivityResponse>(`/api/activity${suffix}`);
}

export async function probeBackendAuth(): Promise<{
  mode: "open" | "authenticated" | "required";
  health?: DashboardServerInfo;
}> {
  const res = await fetch(BASE + "/health", {
    ...withDashboardSource(),
    credentials: "same-origin",
  });
  if (res.ok) {
    const health = normalizeDashboardServerInfo(
      (await res.json()) as DashboardServerInfo,
    );
    return {
      mode: health.authRequired ? "authenticated" : "open",
      health,
    };
  }

  const err = await parseError(res);
  if (
    res.status === 401 &&
    (err.code === "missing_token" ||
      err.code === "bad_token" ||
      err.error === "unauthorized")
  ) {
    return { mode: "required" };
  }

  throw new Error(err.error || "Request failed");
}

export async function login(token: string): Promise<void> {
  await request<{ status: string }>(
    "/api/auth/login",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    },
    {
      suppressAuthRedirect: true,
    },
  );
}

export async function logout(): Promise<void> {
  await request<{ status: string }>(
    "/api/auth/logout",
    {
      method: "POST",
    },
    {
      suppressAuthRedirect: true,
    },
  );
}

export async function elevate(token: string): Promise<void> {
  await request<{ status: string }>(
    "/api/auth/elevate",
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({ token }),
    },
    {
      suppressAuthRedirect: true,
    },
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

export type EventHandler = {
  onSystem?: (event: SystemEvent) => void;
  onActivity?: (event: ActivityEvent) => void;
  onInit?: (agents: Agent[]) => void;
  onMonitoring?: (snapshot: MonitoringSnapshot) => void;
};

export function subscribeToEvents(
  handlers: EventHandler,
  options?: {
    includeMemory?: boolean;
    reasoningMode?: string;
    agentId?: string;
  },
): () => void {
  const params = new URLSearchParams();
  if (options?.includeMemory) {
    params.set("memory", "1");
  }
  if (options?.reasoningMode) {
    params.set("mode", options.reasoningMode);
  }
  const suffix = params.size > 0 ? `?${params.toString()}` : "";
  const basePath = options?.agentId
    ? `/api/agents/${encodeURIComponent(options.agentId)}/events`
    : "/api/events";
  const url = sameOriginUrl(`${basePath}${suffix}`);
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
      const event = JSON.parse(e.data) as ActivityEvent;
      handlers.onActivity?.(event);
    } catch {
      // ignore
    }
  });

  es.addEventListener("progress", (e) => {
    try {
      const event = JSON.parse(e.data) as ActivityEvent;
      handlers.onActivity?.(event);
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
    void handleRealtimeAuthFailure();
  };

  // Clean up on page unload to prevent ERR_INCOMPLETE_CHUNKED_ENCODING
  const cleanup = () => es.close();
  window.addEventListener("beforeunload", cleanup);

  return () => {
    window.removeEventListener("beforeunload", cleanup);
    es.close();
  };
}

export async function postProgress(
  agentId: string,
  message: string,
  progress?: number,
  total?: number,
): Promise<{ status: string; id: string }> {
  return request<{ status: string; id: string }>(
    `/api/agents/${encodeURIComponent(agentId)}/events`,
    {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify({
        channel: "progress",
        message,
        progress,
        total,
      }),
    },
  );
}

export async function handleRealtimeAuthFailure(): Promise<void> {
  try {
    const result = await probeBackendAuth();
    if (result.mode === "required") {
      dispatchAuthRequired("missing_token");
    }
  } catch {
    // ignore transient network failures
  }
}
