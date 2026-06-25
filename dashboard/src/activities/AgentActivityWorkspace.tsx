import { useEffect, useMemo, useRef } from "react";
import { useAppStore } from "../stores/useAppStore";
import type { Instance, InstanceTab } from "../types";
import AgentStreamPanel from "./AgentStreamPanel";
import AgentWorkspaceSidebar from "./AgentWorkspaceSidebar";
import { useActivityData } from "./hooks/useActivityData";
import { useActivityDerivation } from "./hooks/useActivityDerivation";
import { useActivityFilters } from "./hooks/useActivityFilters";
import type { ActivityFilters } from "./types";

type WorkspaceTab = "agents" | "activities";

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

  const filteredInstancesRef = useRef<Instance[]>([]);
  const visibleTabsRef = useRef<InstanceTab[]>([]);

  const {
    sidebarTab,
    setSidebarTab,
    filters,
    setFilters,
    activityScopedAgentId,
    activityScopedSessionId,
    deferredFilters,
    updateFilter,
    handleProfileChange,
    handleInstanceChange,
    clearFilters,
  } = useActivityFilters(initialFilters, {
    defaultSidebarTab,
    clearToInitialFilters,
    requireSelectedAgent,
    filteredInstancesRef,
    visibleTabsRef,
  });

  const usesAgentThreadView = useAgentEventStore && sidebarTab === "agents";
  const threadAgentId = deferredFilters.agentId.trim();

  const {
    catalogEvents,
    threadEvents,
    tabs,
    agentSessions,
    setAgentSessions,
    activityLoading,
    agentLoading,
    error,
    refresh,
  } = useActivityData({
    deferredFilters,
    usesAgentThreadView,
    threadAgentId,
    hydrateAgentEvents,
  });

  const {
    filteredInstances,
    visibleTabs,
    agentOptions,
    handoffTabs,
    sessionsWithHandoff,
    agentsWithHandoff,
    derivedSessions,
    visibleAgents,
    displayedEvents,
  } = useActivityDerivation({
    catalogEvents,
    threadEvents,
    agentSessions,
    setAgentSessions,
    liveEvents,
    agentEventsById,
    agents,
    instances,
    tabs,
    filters,
    setFilters,
    sidebarTab,
    usesAgentThreadView,
    normalizedHiddenSources,
    requireAgentIdentity,
    requireSelectedAgent,
    preferKnownAgents,
  });

  // Keep the handler-facing refs in sync after each commit so the profile/
  // instance change handlers read the current derived lists at call time.
  useEffect(() => {
    filteredInstancesRef.current = filteredInstances;
    visibleTabsRef.current = visibleTabs;
  });

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
        onRefresh={refresh}
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
