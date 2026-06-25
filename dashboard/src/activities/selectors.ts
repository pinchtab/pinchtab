import type { ActivityEvent, Agent } from "../types";
import type * as api from "../services/api";
import { eventTsMs, withClearedSessionFilter } from "./helpers";
import type { ActivityFilters, DashboardActivityEvent } from "./types";

const ANONYMOUS_AGENT_ID = "anonymous";

function detailString(
  details: Record<string, unknown> | undefined,
  key: string,
): string {
  const value = details?.[key];
  return typeof value === "string" ? value : "";
}

function detailNumber(
  details: Record<string, unknown> | undefined,
  key: string,
): number {
  const value = details?.[key];
  return typeof value === "number" ? value : 0;
}

export function toDashboardActivityEvent(
  event: ActivityEvent,
): DashboardActivityEvent {
  const details = (event.details ?? {}) as Record<string, unknown>;
  return normalizeDashboardActivityEvent({
    channel: event.channel,
    message: event.message,
    progress: event.progress,
    total: event.total,
    timestamp: event.timestamp,
    source: detailString(details, "source"),
    requestId: detailString(details, "requestId") || event.id,
    sessionId: detailString(details, "sessionId"),
    agentId: event.agentId || "",
    method: event.method,
    path: event.path,
    status: detailNumber(details, "status"),
    durationMs: detailNumber(details, "durationMs"),
    instanceId: detailString(details, "instanceId"),
    profileId: detailString(details, "profileId"),
    profileName: detailString(details, "profileName"),
    tabId: detailString(details, "tabId"),
    url: detailString(details, "url"),
    action: detailString(details, "action"),
    ref: detailString(details, "ref"),
  });
}

export function normalizeDashboardActivityEvent(
  event: DashboardActivityEvent,
): DashboardActivityEvent {
  const source = (event.source || "").trim().toLowerCase();
  const agentId = (event.agentId || "").trim();
  return {
    ...event,
    source,
    agentId: source === "client" && !agentId ? ANONYMOUS_AGENT_ID : agentId,
    tsMs: Date.parse(event.timestamp),
  };
}

export function eventIdentity(event: DashboardActivityEvent): string {
  const agentId = (event.agentId || "").trim();
  if (agentId) {
    return agentId;
  }
  return "";
}

export function matchesVisibleEvent(
  event: DashboardActivityEvent,
  filters: ActivityFilters,
  hiddenSources: string[],
  requireAgentIdentity: boolean,
): boolean {
  const source = (event.source || "").trim().toLowerCase();
  const identity = eventIdentity(event);
  if (source !== "client") {
    return false;
  }
  if (hiddenSources.includes(source)) {
    return false;
  }
  if (requireAgentIdentity && !identity) {
    return false;
  }
  if (filters.agentId && event.agentId !== filters.agentId) {
    return false;
  }
  if (filters.tabId && event.tabId !== filters.tabId) {
    return false;
  }
  if (filters.instanceId && event.instanceId !== filters.instanceId) {
    return false;
  }
  if (filters.profileName && event.profileName !== filters.profileName) {
    return false;
  }
  if (filters.sessionId && event.sessionId !== filters.sessionId) {
    return false;
  }
  if (filters.action && event.action !== filters.action) {
    return false;
  }
  if (filters.pathPrefix && !event.path.startsWith(filters.pathPrefix)) {
    return false;
  }
  if (filters.ageSec) {
    const ageSec = Number(filters.ageSec);
    if (Number.isFinite(ageSec) && ageSec >= 0) {
      const cutoff = Date.now() - ageSec * 1000;
      if (eventTsMs(event) < cutoff) {
        return false;
      }
    }
  }
  return true;
}

export function mergeDashboardActivityEvents(
  first: DashboardActivityEvent[],
  second: DashboardActivityEvent[],
): DashboardActivityEvent[] {
  const merged = new Map<string, DashboardActivityEvent>();

  for (const event of first) {
    const key = event.requestId || `${event.timestamp}:${event.path}`;
    merged.set(key, event);
  }
  for (const event of second) {
    const key = event.requestId || `${event.timestamp}:${event.path}`;
    merged.set(key, event);
  }

  return [...merged.values()].sort(
    (left, right) => eventTsMs(left) - eventTsMs(right),
  );
}

export function filterMatchingEvents(
  events: DashboardActivityEvent[],
  filters: ActivityFilters,
  hiddenSources: string[],
  requireAgentIdentity: boolean,
): DashboardActivityEvent[] {
  return events.filter((event) =>
    matchesVisibleEvent(event, filters, hiddenSources, requireAgentIdentity),
  );
}

export function filterAgentThreadEvents(
  events: DashboardActivityEvent[],
  filters: ActivityFilters,
  hiddenSources: string[],
  requireAgentIdentity: boolean,
): DashboardActivityEvent[] {
  return events.filter((event) => {
    if (
      filters.agentId &&
      eventIdentity(event).toLowerCase() !== filters.agentId.toLowerCase()
    ) {
      return false;
    }
    return matchesVisibleEvent(
      event,
      filters,
      hiddenSources,
      requireAgentIdentity,
    );
  });
}

export interface CatalogPartition {
  visibleEvents: DashboardActivityEvent[];
  sessionCatalogEvents: DashboardActivityEvent[];
  agentCatalogEvents: DashboardActivityEvent[];
}

// partitionCatalogEvents does in ONE pass what three filterMatchingEvents calls
// did separately. The three sets nest (agentCatalog ⊇ sessionCatalog ⊇ visible)
// because they differ only in the agentId + sessionId checks:
//   agentCatalogEvents   = matchesVisibleEvent with agentId="" & sessionId=""
//   sessionCatalogEvents = agentCatalog + (filters.agentId match)
//   visibleEvents        = sessionCatalog + (filters.sessionId match)
export function partitionCatalogEvents(
  catalogEvents: DashboardActivityEvent[],
  filters: ActivityFilters,
  hiddenSources: string[],
  requireAgentIdentity: boolean,
): CatalogPartition {
  const base = { ...filters, agentId: "", sessionId: "" };
  const agentCatalogEvents: DashboardActivityEvent[] = [];
  const sessionCatalogEvents: DashboardActivityEvent[] = [];
  const visibleEvents: DashboardActivityEvent[] = [];

  for (const event of catalogEvents) {
    if (
      !matchesVisibleEvent(event, base, hiddenSources, requireAgentIdentity)
    ) {
      continue;
    }
    agentCatalogEvents.push(event);
    if (filters.agentId && event.agentId !== filters.agentId) {
      continue;
    }
    sessionCatalogEvents.push(event);
    if (filters.sessionId && event.sessionId !== filters.sessionId) {
      continue;
    }
    visibleEvents.push(event);
  }

  return { visibleEvents, sessionCatalogEvents, agentCatalogEvents };
}

export interface ThreadPartition {
  agentThreadEvents: DashboardActivityEvent[];
  agentThreadSessionCatalogEvents: DashboardActivityEvent[];
}

// partitionAgentThreadEvents does in ONE pass what two filterAgentThreadEvents
// calls did. The two sets differ only in the sessionId check:
//   agentThreadSessionCatalogEvents = filter with sessionId cleared
//   agentThreadEvents               = that + (filters.sessionId match)
export function partitionAgentThreadEvents(
  events: DashboardActivityEvent[],
  filters: ActivityFilters,
  hiddenSources: string[],
  requireAgentIdentity: boolean,
): ThreadPartition {
  const sessionCleared = withClearedSessionFilter(filters);
  const agentThreadEvents: DashboardActivityEvent[] = [];
  const agentThreadSessionCatalogEvents: DashboardActivityEvent[] = [];

  for (const event of events) {
    if (
      filters.agentId &&
      eventIdentity(event).toLowerCase() !== filters.agentId.toLowerCase()
    ) {
      continue;
    }
    if (
      !matchesVisibleEvent(
        event,
        sessionCleared,
        hiddenSources,
        requireAgentIdentity,
      )
    ) {
      continue;
    }
    agentThreadSessionCatalogEvents.push(event);
    if (filters.sessionId && event.sessionId !== filters.sessionId) {
      continue;
    }
    agentThreadEvents.push(event);
  }

  return { agentThreadEvents, agentThreadSessionCatalogEvents };
}

export function deriveAgentOptions(
  catalogEvents: DashboardActivityEvent[],
): string[] {
  const ids = new Set<string>();
  for (const event of catalogEvents) {
    const agentId = (event.agentId || "").trim();
    if (agentId) {
      ids.add(agentId);
    }
  }
  return Array.from(ids).sort();
}

export function deriveSessions(
  agentSessions: api.Session[],
  sourceEvents: DashboardActivityEvent[],
): api.Session[] {
  const bySession = new Map<
    string,
    {
      agentId: string;
      label?: string;
      earliest: string;
      earliestMs: number;
      latest: string;
      latestMs: number;
    }
  >();

  for (const s of agentSessions) {
    const latest = s.lastSeenAt || s.createdAt;
    bySession.set(s.id, {
      agentId: s.agentId,
      label: s.label,
      earliest: s.createdAt,
      earliestMs: Date.parse(s.createdAt),
      latest,
      latestMs: Date.parse(latest),
    });
  }

  for (const event of sourceEvents) {
    const sid = event.sessionId?.trim();
    if (!sid) continue;
    const identity = eventIdentity(event);
    const ts = eventTsMs(event);

    const existing = bySession.get(sid);
    if (!existing) {
      bySession.set(sid, {
        agentId: identity,
        earliest: event.timestamp,
        earliestMs: ts,
        latest: event.timestamp,
        latestMs: ts,
      });
      continue;
    }

    if (ts < existing.earliestMs) {
      existing.earliest = event.timestamp;
      existing.earliestMs = ts;
    }
    if (ts > existing.latestMs) {
      existing.latest = event.timestamp;
      existing.latestMs = ts;
    }
  }

  return [...bySession.entries()]
    .map(([id, info]) => ({
      id,
      agentId: info.agentId,
      label: info.label,
      createdAt: info.earliest,
      lastSeenAt: info.latest,
      expiresAt: "",
      status: "active",
    }))
    .sort(
      (a, b) =>
        new Date(b.lastSeenAt).getTime() - new Date(a.lastSeenAt).getTime(),
    );
}

export function deriveAgents(
  agentCatalogEvents: DashboardActivityEvent[],
): Agent[] {
  const byId = new Map<string, Agent>();

  for (const event of agentCatalogEvents) {
    const identity = eventIdentity(event);
    if (!identity) {
      continue;
    }

    const existing = byId.get(identity);
    if (!existing) {
      byId.set(identity, {
        id: identity,
        name: identity,
        connectedAt: event.timestamp,
        lastActivity: event.timestamp,
        requestCount: 1,
      });
      continue;
    }

    existing.requestCount += 1;
    if (
      eventTsMs(event) >
      new Date(existing.lastActivity || existing.connectedAt).getTime()
    ) {
      existing.lastActivity = event.timestamp;
    }
  }

  return [...byId.values()].sort(
    (left, right) =>
      new Date(right.lastActivity || right.connectedAt).getTime() -
      new Date(left.lastActivity || left.connectedAt).getTime(),
  );
}

export function mergeVisibleAgents(
  derivedAgents: Agent[],
  knownAgents: Agent[],
  preferKnownAgents: boolean,
  requireAgentIdentity: boolean,
): Agent[] {
  if (!preferKnownAgents) {
    return derivedAgents;
  }

  const merged = new Map<string, Agent>();
  for (const agent of derivedAgents) {
    merged.set(agent.id, agent);
  }
  for (const agent of knownAgents) {
    merged.set(agent.id, {
      ...merged.get(agent.id),
      ...agent,
    });
  }

  return [...merged.values()]
    .filter((agent) =>
      !requireAgentIdentity ? true : !!(agent.id || "").trim(),
    )
    .sort(
      (left, right) =>
        new Date(right.lastActivity || right.connectedAt).getTime() -
        new Date(left.lastActivity || left.connectedAt).getTime(),
    );
}

export { withClearedSessionFilter };
