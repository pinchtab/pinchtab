import type { ActivityLogEvent, ActivityLogResponse } from "../generated/types";

export type DashboardActivityEvent = ActivityLogEvent & {
  channel?: string;
  message?: string;
  progress?: number;
  total?: number;
};
export type DashboardActivityResponse = ActivityLogResponse;

export interface ActivityFilters {
  agentId: string;
  tabId: string;
  instanceId: string;
  profileName: string;
  sessionId: string;
  action: string;
  pathPrefix: string;
  ageSec: string;
  limit: string;
}

export interface ActivityQuery {
  source?: string;
  requestId?: string;
  sessionId?: string;
  actorId?: string;
  agentId?: string;
  instanceId?: string;
  profileId?: string;
  profileName?: string;
  tabId?: string;
  action?: string;
  engine?: string;
  pathPrefix?: string;
  since?: string;
  until?: string;
  ageSec?: number;
  limit?: number;
}
