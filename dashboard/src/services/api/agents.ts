import type { Agent, AgentDetail, Session } from "../../generated/types";
import { request } from "./client";

export async function fetchAgents(): Promise<Agent[]> {
  return request<Agent[]>("/api/agents");
}

export type { Session };

export async function fetchSessions(): Promise<Session[]> {
  return request<Session[]>("/sessions");
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
