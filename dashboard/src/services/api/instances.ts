import type {
  Instance,
  InstanceTab,
  LaunchInstanceRequest,
} from "../../generated/types";
import { sameOriginUrl } from "../auth";
import {
  request,
  requestText,
  normalizeInstance,
  createEventStream,
  subscribeJsonEvent,
} from "./client";

// Instances — endpoint is /instances (no /api prefix)
export async function fetchInstances(): Promise<Instance[]> {
  return (await request<Instance[]>("/instances")).map(normalizeInstance);
}

export async function launchInstance(
  data: LaunchInstanceRequest,
): Promise<Instance> {
  // Use the canonical start endpoint to avoid legacy launch alias validation edge cases.
  return normalizeInstance(
    await request<Instance>("/instances/start", {
      method: "POST",
      headers: { "Content-Type": "application/json" },
      body: JSON.stringify(data),
    }),
  );
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

export function subscribeToInstanceLogs(
  id: string,
  handlers: { onLogs?: (logs: string, reset: boolean) => void },
): () => void {
  const url = sameOriginUrl(`/instances/${encodeURIComponent(id)}/logs/stream`);
  const { es, unsubscribe } = createEventStream(url);

  subscribeJsonEvent<{ logs?: string; reset?: boolean }>(
    es,
    "log",
    (payload) => {
      handlers.onLogs?.(payload.logs ?? "", payload.reset === true);
    },
  );

  return unsubscribe;
}
