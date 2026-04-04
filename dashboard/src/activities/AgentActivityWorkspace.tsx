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
import type { ActivityFilters, DashboardActivityEvent } from "./types";

type WorkspaceTab = "agents" | "activities";

interface Props {
  initialFilters?: Partial<ActivityFilters>;
  defaultSidebarTab?: WorkspaceTab;
  hiddenSources?: string[];
  requireAgentIdentity?: boolean;
  requireSelectedAgent?: boolean;
  showAllAgentsOption?: boolean;
  showAgentFilter?: boolean;
  simplifyEventRows?: boolean;
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
  return {
    channel: event.channel,
    message: event.message,
    progress: event.progress,
    total: event.total,
    timestamp: event.timestamp,
    source: detailString(details, "source"),
    requestId: detailString(details, "requestId") || event.id,
    sessionId: detailString(details, "sessionId"),
    actorId: detailString(details, "actorId"),
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
  };
}

function matchesVisibleEvent(
  event: DashboardActivityEvent,
  filters: ActivityFilters,
  hiddenSources: string[],
  requireAgentIdentity: boolean,
): boolean {
  if (hiddenSources.includes(event.source)) {
    return false;
  }
  // Hide API management calls (sessions, activity queries, health) from the stream
  if (
    event.path.startsWith("/api/") ||
    event.path === "/health" ||
    event.path === "/metrics"
  ) {
    return false;
  }
  if (requireAgentIdentity && !(event.agentId || "").trim()) {
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

export default function AgentActivityWorkspace({
  initialFilters,
  defaultSidebarTab = "agents",
  hiddenSources = [],
  requireAgentIdentity = false,
  requireSelectedAgent = false,
  showAllAgentsOption = true,
  showAgentFilter = true,
  simplifyEventRows = false,
  copyTabId = false,
  preferKnownAgents = false,
  useAgentEventStore = false,
  clearToInitialFilters = false,
}: Props) {
  const { instances, profiles, agents, agentEventsById, hydrateAgentEvents } =
    useAppStore();
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
  const [activityEvents, setActivityEvents] = useState<
    DashboardActivityEvent[]
  >([]);
  const [tabs, setTabs] = useState<InstanceTab[]>([]);
  const [activityLoading, setActivityLoading] = useState(false);
  const [agentLoading, setAgentLoading] = useState(false);
  const [agentSessions, setSessions] = useState<api.Session[]>([]);
  const [error, setError] = useState("");
  const [refreshNonce, setRefreshNonce] = useState(0);

  const deferredFilters = useDeferredValue(filters);
  const activityQuery = useMemo(
    () => buildActivityQuery(deferredFilters),
    [deferredFilters],
  );
  const activityQueryKey = JSON.stringify(activityQuery);
  const usesAgentThreadView = useAgentEventStore && sidebarTab === "agents";

  useEffect(() => {
    setSidebarTab(defaultSidebarTab);
  }, [defaultSidebarTab]);

  useEffect(() => {
    const next = initialBaseFilters;
    setFilters((current) =>
      sameActivityFilters(current, next) ? current : next,
    );
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
    if (usesAgentThreadView) {
      setActivityLoading(false);
      return;
    }

    let cancelled = false;
    const load = async () => {
      setActivityLoading(true);
      setError("");
      try {
        const response = await fetchActivity(activityQuery);
        if (cancelled) {
          return;
        }
        setActivityEvents(response.events);
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
  }, [activityQuery, activityQueryKey, refreshNonce, usesAgentThreadView]);

  useEffect(() => {
    if (!usesAgentThreadView || !filters.agentId) {
      setAgentLoading(false);
      return;
    }

    let cancelled = false;
    const load = async () => {
      setAgentLoading(true);
      setError("");
      try {
        const response = await api.fetchAgent(filters.agentId, "both");
        if (cancelled) {
          return;
        }
        hydrateAgentEvents(filters.agentId, response.events);
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
  }, [filters.agentId, hydrateAgentEvents, refreshNonce, usesAgentThreadView]);

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

  const visibleEvents = useMemo(
    () =>
      activityEvents.filter((event) =>
        matchesVisibleEvent(
          event,
          filters,
          normalizedHiddenSources,
          requireAgentIdentity,
        ),
      ),
    [activityEvents, filters, normalizedHiddenSources, requireAgentIdentity],
  );

  const agentThreadEvents = useMemo(() => {
    if (!filters.agentId) {
      return [] as DashboardActivityEvent[];
    }

    return (agentEventsById[filters.agentId] ?? [])
      .map(toDashboardActivityEvent)
      .filter((event) =>
        matchesVisibleEvent(
          event,
          {
            ...filters,
            agentId: "",
          },
          normalizedHiddenSources,
          requireAgentIdentity,
        ),
      );
  }, [agentEventsById, filters, normalizedHiddenSources, requireAgentIdentity]);

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
        ? (agentEventsById[filters.agentId] ?? []).map(toDashboardActivityEvent)
        : visibleEvents;

    for (const event of sourceEvents) {
      const sid = event.sessionId?.trim();
      if (!sid) continue;

      const existing = bySession.get(sid);
      if (!existing) {
        bySession.set(sid, {
          agentId: event.agentId || "",
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
    agentEventsById,
    visibleEvents,
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

    for (const event of visibleEvents) {
      const agentId = event.agentId?.trim();
      if (!agentId) {
        continue;
      }

      const existing = byId.get(agentId);
      if (!existing) {
        byId.set(agentId, {
          id: agentId,
          name: agentId,
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
  }, [visibleEvents]);

  const visibleAgents = useMemo<Agent[]>(() => {
    if (!preferKnownAgents) {
      return derivedAgents;
    }

    return [...agents]
      .filter((agent) => {
        if (requireAgentIdentity && !(agent.id || "").trim()) {
          return false;
        }
        if (requireAgentIdentity && agent.id === "anonymous") {
          return false;
        }
        return true;
      })
      .sort(
        (left, right) =>
          new Date(right.lastActivity || right.connectedAt).getTime() -
          new Date(left.lastActivity || left.connectedAt).getTime(),
      );
  }, [agents, derivedAgents, preferKnownAgents, requireAgentIdentity]);

  const summary = useMemo(() => {
    const agentsSeen = new Set(
      displayedEvents.map((event) => event.agentId).filter(Boolean),
    );
    const tabsSeen = new Set(
      displayedEvents.map((event) => event.tabId).filter(Boolean),
    );
    const instancesSeen = new Set(
      displayedEvents.map((event) => event.instanceId).filter(Boolean),
    );

    return `${displayedEvents.length} events • ${agentsSeen.size} agents • ${tabsSeen.size} tabs • ${instancesSeen.size} instances`;
  }, [displayedEvents]);

  useEffect(() => {
    if (!requireSelectedAgent || visibleAgents.length === 0) {
      return;
    }

    const hasAgent = visibleAgents.some(
      (agent) => agent.id === filters.agentId,
    );
    const targetAgent = hasAgent ? filters.agentId : visibleAgents[0].id;
    const agentSessionList = derivedSessions.filter(
      (s) => s.agentId === targetAgent,
    );
    const latestSession =
      agentSessionList.length > 0 ? agentSessionList[0].id : "";

    if (!hasAgent || (!filters.sessionId && latestSession)) {
      setFilters((current) => ({
        ...current,
        agentId: targetAgent,
        sessionId: latestSession,
      }));
    }
  }, [
    filters.agentId,
    filters.sessionId,
    requireSelectedAgent,
    visibleAgents,
    derivedSessions,
  ]);

  const updateFilter = (key: keyof ActivityFilters, value: string) => {
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
    setFilters((current) => ({
      ...resetBaseFilters,
      agentId:
        requireSelectedAgent && current.agentId
          ? current.agentId
          : resetBaseFilters.agentId,
    }));
  };

  const sidebarLoading =
    sidebarTab === "activities" ? activityLoading : agentLoading;

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden xl:flex-row">
      <AgentWorkspaceSidebar
        sidebarTab={sidebarTab}
        visibleAgents={visibleAgents}
        activeAgentId={filters.agentId}
        filters={filters}
        sessions={derivedSessions}
        showAllAgentsOption={showAllAgentsOption}
        showAgentFilter={showAgentFilter}
        profiles={profiles}
        filteredInstances={filteredInstances}
        visibleTabs={visibleTabs}
        loading={sidebarLoading}
        onSidebarTabChange={setSidebarTab}
        onSelectAgent={(agentId, autoSessionId) => {
          setFilters((current) => ({
            ...current,
            agentId,
            sessionId: autoSessionId || "",
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
        filters={filters}
        events={displayedEvents}
        sessions={derivedSessions}
        summary={summary}
        error={error}
        loading={usesAgentThreadView ? agentLoading : activityLoading}
        copyTabId={copyTabId}
        hideAgentFilter={requireSelectedAgent}
        hideSessionFilter={requireSelectedAgent}
        simplifyMeta={simplifyEventRows}
        onClearFilters={clearFilters}
        onFilterChange={updateFilter}
      />
    </div>
  );
}
