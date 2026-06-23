import type { Instance } from "../../generated/types";
import type { DashboardServerInfo } from "../../types";
import { normalizeDashboardServerInfo } from "../../types";
import { dispatchAuthRequired, dispatchServerUnreachable } from "../auth";

const BASE = ""; // Uses proxy in dev
const DASHBOARD_SOURCE_HEADER = "X-PinchTab-Source";
const DASHBOARD_SOURCE = "dashboard";
const REALTIME_AUTH_PROBE_COOLDOWN_MS = 3000;
let realtimeAuthProbeInFlight: Promise<void> | null = null;
let lastRealtimeAuthProbeAt = 0;

export function resetRealtimeAuthProbeStateForTests(): void {
  realtimeAuthProbeInFlight = null;
  lastRealtimeAuthProbeAt = 0;
}

export type RequestMeta = {
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

export async function fetchOk(
  url: string,
  options?: RequestInit,
  meta?: RequestMeta,
): Promise<Response> {
  let res: Response;
  try {
    res = await fetch(BASE + url, {
      ...withDashboardSource(options),
      credentials: "same-origin",
    });
  } catch (error) {
    dispatchServerUnreachable();
    throw error;
  }
  if (!res.ok) {
    const err = await parseError(res);
    if (res.status === 401) {
      handleUnauthorized(meta, err.code);
    }
    throw new ApiError(err.error || "Request failed", res.status, err.code);
  }
  return res;
}

export async function request<T>(
  url: string,
  options?: RequestInit,
  meta?: RequestMeta,
): Promise<T> {
  return (await fetchOk(url, options, meta)).json();
}

export async function requestText(
  url: string,
  options?: RequestInit,
  meta?: RequestMeta,
): Promise<string> {
  return (await fetchOk(url, options, meta)).text();
}

export async function requestBlob(
  url: string,
  options?: RequestInit,
  meta?: RequestMeta,
): Promise<Blob> {
  return (await fetchOk(url, options, meta)).blob();
}

export function withDashboardSource(options?: RequestInit): RequestInit {
  const headers = new Headers(options?.headers);
  headers.set(DASHBOARD_SOURCE_HEADER, DASHBOARD_SOURCE);
  return {
    ...options,
    headers,
  };
}

export function normalizeInstance(instance: Instance): Instance {
  return {
    ...instance,
    mode: instance.mode ?? (instance.headless ? "headless" : "headed"),
  };
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

// createEventStream opens an EventSource, wires realtime auth-failure handling
// and beforeunload cleanup, and returns the source plus an unsubscribe fn. The
// caller registers its own typed listeners on the returned `es`.
export function createEventStream(url: string): {
  es: EventSource;
  unsubscribe: () => void;
} {
  const es = new EventSource(url);
  // Suppress connection errors (expected on page reload/navigation); a closed
  // stream from an expired session triggers the realtime auth-failure flow.
  es.onerror = () => {
    void handleRealtimeAuthFailure();
  };
  // Clean up on page unload to prevent ERR_INCOMPLETE_CHUNKED_ENCODING.
  const cleanup = () => es.close();
  window.addEventListener("beforeunload", cleanup);
  const unsubscribe = () => {
    window.removeEventListener("beforeunload", cleanup);
    es.close();
  };
  return { es, unsubscribe };
}

// subscribeJsonEvent registers a typed SSE listener that JSON-parses each event's
// data, applies an optional transform, and dispatches to handler. Malformed
// payloads are ignored. The EventSource string-event overload types the callback
// event as MessageEvent, so `e.data` is valid.
export function subscribeJsonEvent<T>(
  es: EventSource,
  event: string,
  handler: ((value: T) => void) | undefined,
  parse: (raw: unknown) => T = (raw) => raw as T,
): void {
  es.addEventListener(event, (e) => {
    try {
      handler?.(parse(JSON.parse(e.data)));
    } catch {
      // ignore malformed events
    }
  });
}

export async function handleRealtimeAuthFailure(): Promise<void> {
  const now = Date.now();
  if (realtimeAuthProbeInFlight) {
    return realtimeAuthProbeInFlight;
  }
  if (now - lastRealtimeAuthProbeAt < REALTIME_AUTH_PROBE_COOLDOWN_MS) {
    return;
  }

  lastRealtimeAuthProbeAt = now;
  realtimeAuthProbeInFlight = (async () => {
    try {
      const result = await probeBackendAuth();
      if (result.mode === "required") {
        dispatchAuthRequired("missing_token");
      }
    } catch {
      dispatchServerUnreachable();
    }
  })().finally(() => {
    realtimeAuthProbeInFlight = null;
  });

  return realtimeAuthProbeInFlight;
}
