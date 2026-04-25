import { useDeferredValue, useEffect, useMemo, useState } from "react";
import { useAppStore } from "../stores/useAppStore";
import * as api from "../services/api";
import type { ActivityEvent, Agent, InstanceTab } from "../types";
import { fetchActivity } from "./api";
import AgentStreamPanel from "./AgentStreamPanel";
import AgentWorkspaceSidebar from "./AgentWorkspaceSidebar";
import {
  buildActivityQuery,
  defaultActivityFilters,
  sameActivityFilters,
} from "./helpers";
import { computeHandoffTabs, deriveHandoffIndex } from "./handoffState";
import type { ActivityFilters, DashboardActivityEvent } from "./types";

type WorkspaceTab = "agents" | "activities";
const ANONYMOUS_AGENT_ID = "anonymous";

interface Props {
  initialFilters?: Partial<ActivityFilters>;
  defaultSidebarTab?: WorkspaceTab;
  hiddenSources?: string[];
  requireAgentIdentity?: boolean;
  requireSelectedAgent?: boolean;
  showAllAgentsOption?: boolean;
  showAgentFilter?: boolean;
  copyTabId?: boolean;
  preferKnownAgents?: boolean;
  useAgentEventStore?: boolean;
  clearToInitialFilters?: boolean;
}

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

function toDashboardActivityEvent(
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
    engine: detailString(details, "engine"),
    ref: detailString(details, "ref"),
  });
}

function normalizeDashboardActivityEvent(
  event: DashboardActivityEvent,
): DashboardActivityEvent {
  const source = (event.source || "").trim().toLowerCase();
  const agentId = (event.agentId || "").trim();
  return {
    ...event,
    source,
    agentId: source === "client" && !agentId ? ANONYMOUS_AGENT_ID : agentId,
  };
}

function eventIdentity(event: DashboardActivityEvent): string {
  const agentId = (event.agentId || "").trim();
  if (agentId) {
    return agentId;
  }
  return "";
}

function matchesVisibleEvent(
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
      if (new Date(event.timestamp).getTime() < cutoff) {
        return false;
      }
    }
  }
  return true;
}

function withClearedSessionFilter(filters: ActivityFilters): ActivityFilters {
  return {
    ...filters,
    sessionId: "",
  };
}

function mergeDashboardActivityEvents(
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
    (left, right) =>
      new Date(left.timestamp).getTime() - new Date(right.timestamp).getTime(),
  );
}

export default function AgentActivityWorkspace({
  initialFilters,
  defaultSidebarTab = "agents",
  hiddenSources = [],
  requireAgentIdentity = false,
  requireSelectedAgent = false,
  showAllAgentsOption = true,
  showAgentFilter = true,
  copyTabId = false,
  preferKnownAgents = false,
  useAgentEventStore = false,
  clearToInitialFilters = false,
}: Props) {
  const {
    instances,
    profiles,
    agents,
    agentEventsById,
    hydrateAgentEvents,
    events: liveEvents,
  } = useAppStore();
  const normalizedHiddenSources = useMemo(
    () => [...hiddenSources],
    [hiddenSources],
  );
  const initialBaseFilters = useMemo(
    () => ({
      ...defaultActivityFilters,
      ...initialFilters,
    }),
    [initialFilters],
  );

  const [sidebarTab, setSidebarTab] = useState<WorkspaceTab>(defaultSidebarTab);
  const [filters, setFilters] = useState<ActivityFilters>(initialBaseFilters);
  const [activityScopedAgentId, setActivityScopedAgentId] = useState(
    initialBaseFilters.agentId,
  );
  const [activityScopedSessionId, setActivityScopedSessionId] = useState(
    initialBaseFilters.sessionId,
  );
  const [catalogEvents, setCatalogEvents] = useState<DashboardActivityEvent[]>(
    [],
  );
  const [threadEvents, setThreadEvents] = useState<DashboardActivityEvent[]>(
    [],
  );
  const [tabs, setTabs] = useState<InstanceTab[]>([]);
  const [activityLoading, setActivityLoading] = useState(false);
  const [agentLoading, setAgentLoading] = useState(false);
  const [agentSessions, setSessions] = useState<api.Session[]>([]);
  const [error, setError] = useState("");
  const [refreshNonce, setRefreshNonce] = useState(0);

  const usesAgentThreadView = useAgentEventStore && sidebarTab === "agents";
  const deferredFilters = useDeferredValue(filters);
  const threadAgentId = deferredFilters.agentId.trim();
  const catalogQuery = useMemo(() => {
    const q = buildActivityQuery(deferredFilters);
    if (usesAgentThreadView) {
      delete q.agentId;
      delete q.sessionId;
    }
    q.source = "client";
    return q;
  }, [deferredFilters, usesAgentThreadView]);
  const catalogQueryKey = JSON.stringify(catalogQuery);
  const threadQuery = useMemo(() => {
    if (!usesAgentThreadView || !threadAgentId) {
      return null;
    }
    const q = buildActivityQuery(withClearedSessionFilter(deferredFilters));
    q.source = "client";
    return q;
  }, [deferredFilters, threadAgentId, usesAgentThreadView]);
  const threadQueryKey = JSON.stringify(threadQuery);

  useEffect(() => {
    setSidebarTab(defaultSidebarTab);
  }, [defaultSidebarTab]);

  useEffect(() => {
    const next = initialBaseFilters;
    setFilters((current) =>
      sameActivityFilters(current, next) ? current : next,
    );
    setActivityScopedAgentId(next.agentId);
    setActivityScopedSessionId(next.sessionId);
  }, [initialBaseFilters]);

  useEffect(() => {
    let cancelled = false;
    void api
      .fetchSessions()
      .then((sessions) => {
        if (!cancelled) setSessions(sessions);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [refreshNonce]);

  useEffect(() => {
    let cancelled = false;
    void api
      .fetchAllTabs()
      .then((response) => {
        if (!cancelled) {
          setTabs(response);
        }
      })
      .catch(() => {
        if (!cancelled) {
          setTabs([]);
        }
      });
    return () => {
      cancelled = true;
    };
  }, []);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      setActivityLoading(true);
      setError("");
      try {
        const response = await fetchActivity(catalogQuery);
        if (cancelled) {
          return;
        }
        setCatalogEvents(response.events.map(normalizeDashboardActivityEvent));
      } catch (err) {
        if (cancelled) {
          return;
        }
        setError(
          err instanceof Error ? err.message : "Failed to load activity",
        );
      } finally {
        if (!cancelled) {
          setActivityLoading(false);
        }
      }
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, [catalogQuery, catalogQueryKey, refreshNonce]);

  useEffect(() => {
    if (!usesAgentThreadView || !threadAgentId) {
      setThreadEvents([]);
      setAgentLoading(false);
      return;
    }

    let cancelled = false;
    const load = async () => {
      setAgentLoading(true);
      setError("");
      try {
        const [detail, response] = await Promise.all([
          api.fetchAgent(threadAgentId),
          threadQuery
            ? fetchActivity(threadQuery)
            : Promise.resolve({ count: 0, events: [] }),
        ]);
        if (cancelled) {
          return;
        }
        hydrateAgentEvents(detail.agent.id, detail.events);
        setThreadEvents(response.events.map(normalizeDashboardActivityEvent));
      } catch (err) {
        if (cancelled) {
          return;
        }
        setError(
          err instanceof Error ? err.message : "Failed to load agent activity",
        );
      } finally {
        if (!cancelled) {
          setAgentLoading(false);
        }
      }
    };

    void load();
    return () => {
      cancelled = true;
    };
  }, [
    hydrateAgentEvents,
    refreshNonce,
    threadAgentId,
    threadQuery,
    threadQueryKey,
    usesAgentThreadView,
  ]);

  const filteredInstances = useMemo(
    () =>
      filters.profileName === ""
        ? instances
        : instances.filter(
            (instance) => instance.profileName === filters.profileName,
          ),
    [filters.profileName, instances],
  );

  const visibleTabs = useMemo(
    () =>
      filters.instanceId === ""
        ? tabs
        : tabs.filter((tab) => tab.instanceId === filters.instanceId),
    [filters.instanceId, tabs],
  );

  const agentOptions = useMemo(() => {
    const ids = new Set<string>();
    for (const event of catalogEvents) {
      const agentId = (event.agentId || "").trim();
      if (agentId) {
        ids.add(agentId);
      }
    }
    return Array.from(ids).sort();
  }, [catalogEvents]);

  // Handoff detection runs against the union of polled catalog events and the
  // live SSE-driven store events so the sidebar dots update in real time, not
  // only when filters change and catalogEvents re-fetches.
  const handoffEventPool = useMemo(() => {
    const combined: DashboardActivityEvent[] = [...catalogEvents];
    for (const live of liveEvents) {
      combined.push(toDashboardActivityEvent(live));
    }
    return combined;
  }, [catalogEvents, liveEvents]);

  const handoffTabs = useMemo(
    () => computeHandoffTabs(handoffEventPool),
    [handoffEventPool],
  );
  const { sessionsWithHandoff, agentsWithHandoff } = useMemo(
    () => deriveHandoffIndex(handoffEventPool, handoffTabs),
    [handoffEventPool, handoffTabs],
  );

  const visibleEvents = useMemo(
    () =>
      catalogEvents.filter((event) =>
        matchesVisibleEvent(
          event,
          filters,
          normalizedHiddenSources,
          requireAgentIdentity,
        ),
      ),
    [catalogEvents, filters, normalizedHiddenSources, requireAgentIdentity],
  );

  const sessionCatalogEvents = useMemo(
    () =>
      catalogEvents.filter((event) =>
        matchesVisibleEvent(
          event,
          withClearedSessionFilter(filters),
          normalizedHiddenSources,
          requireAgentIdentity,
        ),
      ),
    [catalogEvents, filters, normalizedHiddenSources, requireAgentIdentity],
  );

  const agentCatalogEvents = useMemo(
    () =>
      catalogEvents.filter((event) =>
        matchesVisibleEvent(
          event,
          {
            ...withClearedSessionFilter(filters),
            agentId: "",
          },
          normalizedHiddenSources,
          requireAgentIdentity,
        ),
      ),
    [catalogEvents, filters, normalizedHiddenSources, requireAgentIdentity],
  );

  const agentThreadBaseEvents = useMemo(() => {
    const liveEvents = (agentEventsById[filters.agentId] ?? []).map(
      toDashboardActivityEvent,
    );
    return mergeDashboardActivityEvents(threadEvents, liveEvents);
  }, [agentEventsById, filters.agentId, threadEvents]);

  const agentThreadEvents = useMemo(() => {
    const effectiveFilters = filters;

    return agentThreadBaseEvents.filter((event) => {
      if (
        effectiveFilters.agentId &&
        eventIdentity(event).toLowerCase() !==
          effectiveFilters.agentId.toLowerCase()
      ) {
        return false;
      }
      return matchesVisibleEvent(
        event,
        effectiveFilters,
        normalizedHiddenSources,
        requireAgentIdentity,
      );
    });
  }, [
    agentThreadBaseEvents,
    filters,
    normalizedHiddenSources,
    requireAgentIdentity,
  ]);

  const agentThreadSessionCatalogEvents = useMemo(() => {
    const effectiveFilters = withClearedSessionFilter(filters);

    return agentThreadBaseEvents.filter((event) => {
      if (
        effectiveFilters.agentId &&
        eventIdentity(event).toLowerCase() !==
          effectiveFilters.agentId.toLowerCase()
      ) {
        return false;
      }
      return matchesVisibleEvent(
        event,
        effectiveFilters,
        normalizedHiddenSources,
        requireAgentIdentity,
      );
    });
  }, [
    agentThreadBaseEvents,
    filters,
    normalizedHiddenSources,
    requireAgentIdentity,
  ]);

  const displayedEvents = usesAgentThreadView
    ? agentThreadEvents
    : visibleEvents;

  const derivedSessions = useMemo<api.Session[]>(() => {
    const bySession = new Map<
      string,
      {
        agentId: string;
        label?: string;
        earliest: string;
        latest: string;
      }
    >();

    for (const s of agentSessions) {
      bySession.set(s.id, {
        agentId: s.agentId,
        label: s.label,
        earliest: s.createdAt,
        latest: s.lastSeenAt || s.createdAt,
      });
    }

    const sourceEvents =
      usesAgentThreadView && filters.agentId
        ? agentThreadSessionCatalogEvents
        : sessionCatalogEvents;

    for (const event of sourceEvents) {
      const sid = event.sessionId?.trim();
      if (!sid) continue;
      const identity = eventIdentity(event);

      const existing = bySession.get(sid);
      if (!existing) {
        bySession.set(sid, {
          agentId: identity,
          earliest: event.timestamp,
          latest: event.timestamp,
        });
        continue;
      }

      const ts = new Date(event.timestamp).getTime();
      if (ts < new Date(existing.earliest).getTime())
        existing.earliest = event.timestamp;
      if (ts > new Date(existing.latest).getTime())
        existing.latest = event.timestamp;
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
  }, [
    usesAgentThreadView,
    filters.agentId,
    agentThreadSessionCatalogEvents,
    sessionCatalogEvents,
    agentSessions,
  ]);

  const unlabeledPtsKey = useMemo(() => {
    const apiIds = new Set(agentSessions.map((s) => s.id));
    return derivedSessions
      .filter((s) => s.id.startsWith("ses_") && !apiIds.has(s.id))
      .map((s) => s.id)
      .sort()
      .join(",");
  }, [derivedSessions, agentSessions]);

  useEffect(() => {
    if (!unlabeledPtsKey) return;

    let cancelled = false;
    const timer = setTimeout(() => {
      void api
        .fetchSessions()
        .then((sessions) => {
          if (!cancelled) setSessions(sessions);
        })
        .catch(() => {});
    }, 500);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [unlabeledPtsKey]);

  const derivedAgents = useMemo<Agent[]>(() => {
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
        new Date(event.timestamp).getTime() >
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
  }, [agentCatalogEvents]);

  const visibleAgents = useMemo<Agent[]>(() => {
    if (!preferKnownAgents) {
      return derivedAgents;
    }

    const merged = new Map<string, Agent>();
    for (const agent of derivedAgents) {
      merged.set(agent.id, agent);
    }
    for (const agent of agents) {
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
  }, [agents, derivedAgents, preferKnownAgents, requireAgentIdentity]);

  useEffect(() => {
    if (sidebarTab !== "agents") {
      return;
    }
    if (!requireSelectedAgent || visibleAgents.length === 0) {
      return;
    }

    const hasAgent = visibleAgents.some(
      (agent) => agent.id === filters.agentId,
    );
    const targetAgent = hasAgent ? filters.agentId : visibleAgents[0].id;
    if (!hasAgent) {
      setFilters((current) => ({
        ...current,
        agentId: targetAgent,
        sessionId: "",
      }));
    }
  }, [filters.agentId, requireSelectedAgent, sidebarTab, visibleAgents]);

  const updateFilter = (key: keyof ActivityFilters, value: string) => {
    if (sidebarTab === "activities") {
      if (key === "agentId") {
        setActivityScopedAgentId(value);
        setActivityScopedSessionId("");
        setFilters((current) => ({
          ...current,
          agentId: value,
          sessionId: "",
        }));
        return;
      }
      if (key === "sessionId") {
        setActivityScopedSessionId(value);
      }
    }
    setFilters((current) => ({ ...current, [key]: value }));
  };

  const handleProfileChange = (value: string) => {
    setFilters((current) => ({
      ...current,
      profileName: value,
      instanceId:
        value === "" ||
        filteredInstances.some((instance) => instance.id === current.instanceId)
          ? current.instanceId
          : "",
      tabId: value === "" ? current.tabId : "",
    }));
  };

  const handleInstanceChange = (value: string) => {
    setFilters((current) => ({
      ...current,
      instanceId: value,
      tabId:
        value === "" || visibleTabs.some((tab) => tab.id === current.tabId)
          ? current.tabId
          : "",
    }));
  };

  const clearFilters = () => {
    const resetBaseFilters = clearToInitialFilters
      ? initialBaseFilters
      : defaultActivityFilters;
    if (sidebarTab === "activities") {
      setActivityScopedAgentId(resetBaseFilters.agentId);
      setActivityScopedSessionId(resetBaseFilters.sessionId);
    }
    setFilters((current) => ({
      ...resetBaseFilters,
      agentId:
        requireSelectedAgent && current.agentId
          ? current.agentId
          : resetBaseFilters.agentId,
    }));
  };

  const sidebarLoading =
    sidebarTab === "activities"
      ? activityLoading
      : activityLoading || agentLoading;

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden lg:flex-row">
      <AgentWorkspaceSidebar
        sidebarTab={sidebarTab}
        visibleAgents={visibleAgents}
        activeAgentId={filters.agentId}
        filters={filters}
        sessions={derivedSessions}
        sessionsWithHandoff={sessionsWithHandoff}
        agentsWithHandoff={agentsWithHandoff}
        showAllAgentsOption={showAllAgentsOption}
        showAgentFilter={showAgentFilter}
        profiles={profiles}
        filteredInstances={filteredInstances}
        visibleTabs={visibleTabs}
        agentOptions={agentOptions}
        loading={sidebarLoading}
        onSidebarTabChange={(nextTab) => {
          setSidebarTab(nextTab);
          if (nextTab === "activities") {
            setFilters((current) => ({
              ...current,
              agentId: activityScopedAgentId,
              sessionId: activityScopedSessionId,
            }));
          }
        }}
        onSelectAgent={(agentId) => {
          setFilters((current) => ({
            ...current,
            agentId,
            sessionId: "",
          }));
        }}
        onSelectSession={(sessionId) => {
          setFilters((current) => ({
            ...current,
            sessionId,
          }));
        }}
        onClearFilters={clearFilters}
        onRefresh={() => setRefreshNonce((current) => current + 1)}
        onFilterChange={updateFilter}
        onProfileChange={handleProfileChange}
        onInstanceChange={handleInstanceChange}
      />

      <AgentStreamPanel
        events={displayedEvents}
        sessions={derivedSessions}
        error={error}
        loading={activityLoading || agentLoading}
        copyTabId={copyTabId}
        handoffTabs={handoffTabs}
        activeSessionId={filters.sessionId}
        onFilterChange={updateFilter}
      />
    </div>
  );
}
