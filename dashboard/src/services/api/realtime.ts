import type { Instance, Agent, ActivityEvent } from "../../generated/types";
import type { MonitoringSnapshot } from "../../types";
import { normalizeMonitoringSnapshot } from "../../types";
import { sameOriginUrl } from "../auth";
import { request, createEventStream, subscribeJsonEvent } from "./client";

// SSE Events — endpoint is /api/events
export interface HandoffPayload {
  tabId?: string;
  status?: string;
  reason?: string;
  source?: string;
  hint?: string;
  requestedAt?: string;
  resumedAt?: string;
  timeoutMs?: number;
  url?: string;
  title?: string;
}

export interface SystemEvent {
  type:
    | "instance.started"
    | "instance.stopped"
    | "instance.error"
    | "tab.handoff"
    | "tab.resume";
  instance?: Instance | HandoffPayload;
}

export function activityEventSource(event: ActivityEvent): string {
  const source = event.details?.source;
  return typeof source === "string" ? source.trim().toLowerCase() : "";
}

export function isClientActivityEvent(event: ActivityEvent): boolean {
  return activityEventSource(event) === "client";
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
  const { es, unsubscribe } = createEventStream(url);

  subscribeJsonEvent<Agent[]>(es, "init", handlers.onInit);
  subscribeJsonEvent<SystemEvent>(es, "system", handlers.onSystem);
  subscribeJsonEvent<ActivityEvent>(es, "action", handlers.onActivity);
  subscribeJsonEvent<ActivityEvent>(es, "progress", handlers.onActivity);
  subscribeJsonEvent<MonitoringSnapshot>(
    es,
    "monitoring",
    handlers.onMonitoring,
    (raw) => normalizeMonitoringSnapshot(raw as Partial<MonitoringSnapshot>),
  );

  return unsubscribe;
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
