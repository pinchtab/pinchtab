import { useDeferredValue, useEffect, useMemo, useRef, useState } from "react";
import { useAppStore } from "../stores/useAppStore";
import * as api from "../services/api";
import type { InstanceTab } from "../types";
import { fetchActivity } from "./api";
import ActivityFilterMenu from "./ActivityFilterMenu";
import ActivityTimeline from "./ActivityTimeline";
import {
  applyLockedFilters,
  buildActivityQuery,
  defaultActivityFilters,
  sameActivityFilters,
} from "./helpers";
import type { ActivityFilters, DashboardActivityEvent } from "./types";

interface Props {
  initialFilters?: Partial<ActivityFilters>;
  lockedFilters?: Partial<ActivityFilters>;
  showFilterMenu?: boolean;
  title?: string;
  summaryLabel?: string;
  embedded?: boolean;
}

export default function ActivityExplorer({
  initialFilters,
  lockedFilters,
  showFilterMenu = true,
  title = "Request timeline",
  summaryLabel = "Activity",
  embedded = false,
}: Props) {
  const { instances, profiles } = useAppStore();
  const [filters, setFilters] = useState<ActivityFilters>({
    ...defaultActivityFilters,
    ...initialFilters,
    ...lockedFilters,
  });
  const [events, setEvents] = useState<DashboardActivityEvent[]>([]);
  const [tabs, setTabs] = useState<InstanceTab[]>([]);
  const [count, setCount] = useState(0);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");

  const deferredFilters = useDeferredValue(filters);
  const effectiveFilters = useMemo(
    () => applyLockedFilters(deferredFilters, lockedFilters),
    [deferredFilters, lockedFilters],
  );
  const query = useMemo(
    () => buildActivityQuery(effectiveFilters),
    [effectiveFilters],
  );
  const queryKey = JSON.stringify(query);
  const stableQuery = useRef(query);
  stableQuery.current = query;

  useEffect(() => {
    setFilters((current) => {
      const next = {
        ...current,
        ...initialFilters,
        ...lockedFilters,
      };
      return sameActivityFilters(current, next) ? current : next;
    });
  }, [initialFilters, lockedFilters]);

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
        setCount(response.count);
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
  }, [queryKey]);

  const stats = useMemo(() => {
    const agents = new Set(
      events.map((event) => event.agentId).filter(Boolean),
    );
    const tabsSeen = new Set(
      events.map((event) => event.tabId).filter(Boolean),
    );
    const instancesSeen = new Set(
      events.map((event) => event.instanceId).filter(Boolean),
    );
    return {
      agents: agents.size,
      tabs: tabsSeen.size,
      instances: instancesSeen.size,
    };
  }, [events]);

  const filteredInstances = useMemo(
    () =>
      effectiveFilters.profileName === ""
        ? instances
        : instances.filter(
            (instance) => instance.profileName === effectiveFilters.profileName,
          ),
    [effectiveFilters.profileName, instances],
  );

  const visibleTabs = useMemo(
    () =>
      effectiveFilters.instanceId === ""
        ? tabs
        : tabs.filter((tab) => tab.instanceId === effectiveFilters.instanceId),
    [effectiveFilters.instanceId, tabs],
  );

  const summary = useMemo(
    () =>
      `${count} events • ${stats.agents} agents • ${stats.tabs} tabs • ${stats.instances} instances`,
    [count, stats.agents, stats.instances, stats.tabs],
  );

  const updateFilter = (key: keyof ActivityFilters, value: string) => {
    if (lockedFilters?.[key] !== undefined) {
      return;
    }
    setFilters((current) => ({ ...current, [key]: value }));
  };

  const handleProfileChange = (value: string) => {
    if (lockedFilters?.profileName !== undefined) {
      return;
    }
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
    if (lockedFilters?.instanceId !== undefined) {
      return;
    }
    setFilters((current) => ({
      ...current,
      instanceId: value,
      tabId:
        value === "" || visibleTabs.some((tab) => tab.id === current.tabId)
          ? current.tabId
          : "",
    }));
  };

  const clearFilters = () =>
    setFilters({
      ...defaultActivityFilters,
      ...lockedFilters,
    });

  const layoutClass = embedded
    ? "flex h-full min-h-0 flex-col overflow-hidden"
    : "flex h-full min-h-0 flex-col gap-4 overflow-hidden p-4 xl:flex-row";

  return (
    <div className={layoutClass}>
      {showFilterMenu && (
        <aside className="dashboard-panel flex w-full shrink-0 flex-col overflow-hidden xl:w-80">
          <div className="border-b border-border-subtle px-4 py-4">
            <div className="dashboard-section-label mb-1">{summaryLabel}</div>
            <h1 className="text-lg font-semibold text-text-primary">{title}</h1>
            <p className="mt-2 text-xs leading-5 text-text-muted">{summary}</p>
          </div>
          <ActivityFilterMenu
            filters={effectiveFilters}
            profileOptions={profiles}
            instanceOptions={filteredInstances}
            tabOptions={visibleTabs}
            loading={loading}
            onClear={clearFilters}
            onRefresh={() => setFilters((current) => ({ ...current }))}
            onFilterChange={updateFilter}
            onProfileChange={handleProfileChange}
            onInstanceChange={handleInstanceChange}
          />
        </aside>
      )}

      <ActivityTimeline
        events={events}
        loading={loading}
        error={error}
        summary={summary}
        onFilterChange={updateFilter}
      />
    </div>
  );
}
