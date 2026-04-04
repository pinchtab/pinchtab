import type { ActivityFilters, ActivityQuery } from "./types";

export const defaultActivityFilters: ActivityFilters = {
  agentId: "",
  tabId: "",
  instanceId: "",
  profileName: "",
  sessionId: "",
  action: "",
  pathPrefix: "",
  ageSec: "3600",
  limit: "200",
};

export const actionOptions = [
  "",
  "navigate",
  "click",
  "dblclick",
  "type",
  "hover",
];

export function buildActivityQuery(filters: ActivityFilters): ActivityQuery {
  const query: ActivityQuery = {};
  if (filters.agentId.trim()) query.agentId = filters.agentId.trim();
  if (filters.tabId.trim()) query.tabId = filters.tabId.trim();
  if (filters.instanceId.trim()) query.instanceId = filters.instanceId.trim();
  if (filters.profileName.trim()) {
    query.profileName = filters.profileName.trim();
  }
  if (filters.sessionId.trim()) query.sessionId = filters.sessionId.trim();
  if (filters.action.trim()) query.action = filters.action.trim();
  if (filters.pathPrefix.trim()) query.pathPrefix = filters.pathPrefix.trim();
  if (filters.ageSec.trim()) {
    const ageSec = Number(filters.ageSec);
    if (Number.isFinite(ageSec) && ageSec >= 0) {
      query.ageSec = ageSec;
    }
  }
  if (filters.limit.trim()) {
    const limit = Number(filters.limit);
    if (Number.isFinite(limit) && limit > 0) {
      query.limit = limit;
    }
  }
  return query;
}

export function applyLockedFilters(
  filters: ActivityFilters,
  lockedFilters?: Partial<ActivityFilters>,
): ActivityFilters {
  if (!lockedFilters) {
    return filters;
  }
  return { ...filters, ...lockedFilters };
}

export function sameActivityFilters(
  left: ActivityFilters,
  right: ActivityFilters,
): boolean {
  return (
    left.agentId === right.agentId &&
    left.tabId === right.tabId &&
    left.instanceId === right.instanceId &&
    left.profileName === right.profileName &&
    left.sessionId === right.sessionId &&
    left.action === right.action &&
    left.pathPrefix === right.pathPrefix &&
    left.ageSec === right.ageSec &&
    left.limit === right.limit
  );
}

export function formatActivityTimestamp(timestamp: string): string {
  return new Date(timestamp).toLocaleString("en-GB", {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export function activityMethodVariant(
  method: string,
): "default" | "info" | "success" | "warning" {
  switch (method.toUpperCase()) {
    case "GET":
      return "info";
    case "POST":
      return "success";
    case "DELETE":
      return "warning";
    default:
      return "default";
  }
}

export function activityStatusVariant(
  status: number,
): "default" | "success" | "warning" | "danger" {
  if (status >= 500) return "danger";
  if (status >= 400) return "warning";
  if (status >= 200) return "success";
  return "default";
}
