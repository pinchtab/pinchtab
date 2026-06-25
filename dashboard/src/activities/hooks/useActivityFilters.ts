import { useDeferredValue, useEffect, useMemo, useState } from "react";
import type { Instance, InstanceTab } from "../../types";
import { defaultActivityFilters, sameActivityFilters } from "../helpers";
import type { ActivityFilters } from "../types";

type WorkspaceTab = "agents" | "activities";

interface UseActivityFiltersOptions {
  defaultSidebarTab: WorkspaceTab;
  clearToInitialFilters: boolean;
  requireSelectedAgent: boolean;
  // Refs holding the latest derived instance/tab lists. The profile/instance
  // change handlers only read these at call time, so refs avoid a circular
  // hook dependency (the lists are derived from `filters`, which this hook
  // owns, and from `tabs`, which the data hook owns).
  filteredInstancesRef: React.MutableRefObject<Instance[]>;
  visibleTabsRef: React.MutableRefObject<InstanceTab[]>;
}

export interface UseActivityFiltersResult {
  sidebarTab: WorkspaceTab;
  setSidebarTab: React.Dispatch<React.SetStateAction<WorkspaceTab>>;
  filters: ActivityFilters;
  setFilters: React.Dispatch<React.SetStateAction<ActivityFilters>>;
  activityScopedAgentId: string;
  setActivityScopedAgentId: React.Dispatch<React.SetStateAction<string>>;
  activityScopedSessionId: string;
  setActivityScopedSessionId: React.Dispatch<React.SetStateAction<string>>;
  initialBaseFilters: ActivityFilters;
  deferredFilters: ActivityFilters;
  updateFilter: (key: keyof ActivityFilters, value: string) => void;
  handleProfileChange: (value: string) => void;
  handleInstanceChange: (value: string) => void;
  clearFilters: () => void;
}

export function useActivityFilters(
  initialFilters: Partial<ActivityFilters> | undefined,
  {
    defaultSidebarTab,
    clearToInitialFilters,
    requireSelectedAgent,
    filteredInstancesRef,
    visibleTabsRef,
  }: UseActivityFiltersOptions,
): UseActivityFiltersResult {
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

  const deferredFilters = useDeferredValue(filters);

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
    const filteredInstances = filteredInstancesRef.current;
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
    const visibleTabs = visibleTabsRef.current;
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

  return {
    sidebarTab,
    setSidebarTab,
    filters,
    setFilters,
    activityScopedAgentId,
    setActivityScopedAgentId,
    activityScopedSessionId,
    setActivityScopedSessionId,
    initialBaseFilters,
    deferredFilters,
    updateFilter,
    handleProfileChange,
    handleInstanceChange,
    clearFilters,
  };
}
