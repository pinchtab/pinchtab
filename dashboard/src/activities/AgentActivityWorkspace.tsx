import { useDeferredValue, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "../stores/useAppStore";
import * as api from "../services/api";
import type { Agent, InstanceTab } from "../types";
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
}

export default function AgentActivityWorkspace({
  initialFilters,
  defaultSidebarTab = "agents",
  hiddenSources = [],
}: Props) {
  const { instances, profiles } = useAppStore();
  const [sidebarTab, setSidebarTab] = useState<WorkspaceTab>(defaultSidebarTab);
  const [filters, setFilters] = useState<ActivityFilters>({
    ...defaultActivityFilters,
    ...initialFilters,
  });
  const [events, setEvents] = useState<DashboardActivityEvent[]>([]);
  const [tabs, setTabs] = useState<InstanceTab[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [refreshNonce, setRefreshNonce] = useState(0);

  const deferredFilters = useDeferredValue(filters);
  const query = useMemo(
    () => buildActivityQuery(deferredFilters),
    [deferredFilters],
  );
  const queryKey = JSON.stringify(query);
  const stableQuery = useRef(query);
  stableQuery.current = query;

  useEffect(() => {
    setSidebarTab(defaultSidebarTab);
  }, [defaultSidebarTab]);

  useEffect(() => {
    const next = {
      ...defaultActivityFilters,
      ...initialFilters,
    };
    setFilters((current) =>
      sameActivityFilters(current, next) ? current : next,
    );
  }, [initialFilters]);

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
      setLoading(true);
      setError("");
      try {
        const response = await fetchActivity(stableQuery.current);
        if (cancelled) return;
        setEvents(response.events);
      } catch (err) {
        if (cancelled) return;
        setError(
          err instanceof Error ? err.message : "Failed to load activity",
        );
      } finally {
        if (!cancelled) {
          setLoading(false);
        }
      }
    };
    void load();
    return () => {
      cancelled = true;
    };
  }, [queryKey, refreshNonce]);

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
    () => events.filter((event) => !hiddenSources.includes(event.source)),
    [events, hiddenSources],
  );

  const visibleAgents = useMemo<Agent[]>(() => {
    const byId = new Map<string, Agent>();
    for (const event of visibleEvents) {
      const agentId = event.agentId?.trim() || "anonymous";
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

  const summary = useMemo(() => {
    const agentsSeen = new Set(
      visibleEvents.map((event) => event.agentId).filter(Boolean),
    );
    const tabsSeen = new Set(
      visibleEvents.map((event) => event.tabId).filter(Boolean),
    );
    const instancesSeen = new Set(
      visibleEvents.map((event) => event.instanceId).filter(Boolean),
    );
    return `${visibleEvents.length} events • ${agentsSeen.size} agents • ${tabsSeen.size} tabs • ${instancesSeen.size} instances`;
  }, [visibleEvents]);

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
    setFilters({ ...defaultActivityFilters });
  };

  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden xl:flex-row">
      <AgentStreamPanel
        filters={filters}
        events={visibleEvents}
        summary={summary}
        error={error}
        loading={loading}
        onClearFilters={clearFilters}
        onFilterChange={updateFilter}
      />

      <AgentWorkspaceSidebar
        sidebarTab={sidebarTab}
        visibleAgents={visibleAgents}
        activeAgentId={filters.agentId}
        filters={filters}
        profiles={profiles}
        filteredInstances={filteredInstances}
        visibleTabs={visibleTabs}
        loading={loading}
        onSidebarTabChange={setSidebarTab}
        onSelectAgent={(agentId) => updateFilter("agentId", agentId)}
        onClearAgentSelection={() => updateFilter("agentId", "")}
        onClearFilters={clearFilters}
        onRefresh={() => setRefreshNonce((current) => current + 1)}
        onFilterChange={updateFilter}
        onProfileChange={handleProfileChange}
        onInstanceChange={handleInstanceChange}
      />
    </div>
  );
}
