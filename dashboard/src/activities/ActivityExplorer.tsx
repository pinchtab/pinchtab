import { useDeferredValue, useEffect, useMemo, useRef, useState } from "react";
import { Select } from "../components/atoms";
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

const ANONYMOUS_AGENT_ID = "anonymous";

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
  const query = useMemo(() => {
    const q = buildActivityQuery(effectiveFilters);
    if (embedded) {
      q.source = "client";
    }
    return q;
  }, [effectiveFilters, embedded]);
  const queryKey = JSON.stringify(query);
  const stableQuery = useRef(query);
  stableQuery.current = query;

  const [sessions, setSessions] = useState<api.Session[]>([]);

  useEffect(() => {
    if (!embedded) return;
    let cancelled = false;
    void api
      .fetchSessions()
      .then((s) => {
        if (!cancelled) setSessions(s);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [embedded]);

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
        setEvents(response.events.map(normalizeDashboardActivityEvent));
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
      embedded
        ? `${count} events • ${stats.agents} agents`
        : `${count} events • ${stats.agents} agents • ${stats.tabs} tabs • ${stats.instances} instances`,
    [count, stats.agents, stats.instances, stats.tabs, embedded],
  );

  const agentOptions = useMemo(() => {
    const ids = new Set<string>();
    for (const e of events) {
      const agentId = (e.agentId || "").trim();
      if (agentId) ids.add(agentId);
    }
    return Array.from(ids).sort();
  }, [events]);

  const sessionOptions = useMemo(() => {
    if (!effectiveFilters.agentId) return [];
    return sessions
      .filter((s) => s.agentId === effectiveFilters.agentId)
      .sort(
        (a, b) =>
          new Date(b.lastSeenAt).getTime() - new Date(a.lastSeenAt).getTime(),
      );
  }, [sessions, effectiveFilters.agentId]);

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
            agentOptions={agentOptions}
            loading={loading}
            onClear={clearFilters}
            onRefresh={() => setFilters((current) => ({ ...current }))}
            onFilterChange={updateFilter}
            onProfileChange={handleProfileChange}
            onInstanceChange={handleInstanceChange}
          />
        </aside>
      )}

      {embedded && agentOptions.length > 0 && (
        <div className="flex items-center gap-2 border-b border-border-subtle px-4 py-1.5">
          <label className="flex items-center gap-1.5 text-xs text-text-muted">
            Agent
            <Select
              value={effectiveFilters.agentId}
              onChange={(e) => {
                updateFilter("agentId", e.target.value);
                if (!e.target.value) updateFilter("sessionId", "");
              }}
              variant="compact"
            >
              <option value="">All</option>
              {agentOptions.map((id) => (
                <option key={id} value={id}>
                  {id}
                </option>
              ))}
            </Select>
          </label>
          <label className="flex items-center gap-1.5 text-xs text-text-muted">
            Session
            <Select
              value={effectiveFilters.sessionId}
              onChange={(e) => updateFilter("sessionId", e.target.value)}
              disabled={!effectiveFilters.agentId}
              variant="compact"
            >
              <option value="">All</option>
              {sessionOptions.map((s) => (
                <option key={s.id} value={s.id}>
                  {s.label || s.id}
                </option>
              ))}
            </Select>
          </label>
          <span className="ml-auto text-[0.68rem] text-text-muted">
            {summary}
          </span>
        </div>
      )}

      <ActivityTimeline
        events={events}
        loading={loading}
        error={error}
        summary={summary}
        embedded={embedded}
        showTab={!embedded}
        onFilterChange={updateFilter}
      />
    </div>
  );
}
