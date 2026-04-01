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
    if (visibleAgents.some((agent) => agent.id === filters.agentId)) {
      return;
    }
    setFilters((current) => ({
      ...current,
      agentId: visibleAgents[0].id,
    }));
  }, [filters.agentId, requireSelectedAgent, visibleAgents]);

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
      <AgentStreamPanel
        filters={filters}
        events={displayedEvents}
        summary={summary}
        error={error}
        loading={usesAgentThreadView ? agentLoading : activityLoading}
        copyTabId={copyTabId}
        hideAgentFilter={requireSelectedAgent}
        simplifyMeta={simplifyEventRows}
        onClearFilters={clearFilters}
        onFilterChange={updateFilter}
      />

      <AgentWorkspaceSidebar
        sidebarTab={sidebarTab}
        visibleAgents={visibleAgents}
        activeAgentId={filters.agentId}
        filters={filters}
        showAllAgentsOption={showAllAgentsOption}
        showAgentFilter={showAgentFilter}
        profiles={profiles}
        filteredInstances={filteredInstances}
        visibleTabs={visibleTabs}
        loading={sidebarLoading}
        onSidebarTabChange={setSidebarTab}
        onSelectAgent={(agentId) => updateFilter("agentId", agentId)}
        onClearFilters={clearFilters}
        onRefresh={() => setRefreshNonce((current) => current + 1)}
        onFilterChange={updateFilter}
        onProfileChange={handleProfileChange}
        onInstanceChange={handleInstanceChange}
      />
    </div>
  );
}
