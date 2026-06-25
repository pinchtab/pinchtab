import { useEffect, useMemo } from "react";
import * as api from "../../services/api";
import type { ActivityEvent, Agent, Instance, InstanceTab } from "../../types";
import { computeHandoffTabs, deriveHandoffIndex } from "../handoffState";
import {
  deriveAgentOptions,
  deriveAgents,
  deriveSessions,
  mergeDashboardActivityEvents,
  mergeVisibleAgents,
  partitionAgentThreadEvents,
  partitionCatalogEvents,
  toDashboardActivityEvent,
} from "../selectors";
import type { ActivityFilters, DashboardActivityEvent } from "../types";

type WorkspaceTab = "agents" | "activities";

interface UseActivityDerivationOptions {
  catalogEvents: DashboardActivityEvent[];
  threadEvents: DashboardActivityEvent[];
  agentSessions: api.Session[];
  setAgentSessions: React.Dispatch<React.SetStateAction<api.Session[]>>;
  liveEvents: ActivityEvent[];
  agentEventsById: Record<string, ActivityEvent[]>;
  agents: Agent[];
  instances: Instance[];
  tabs: InstanceTab[];
  filters: ActivityFilters;
  setFilters: React.Dispatch<React.SetStateAction<ActivityFilters>>;
  sidebarTab: WorkspaceTab;
  usesAgentThreadView: boolean;
  normalizedHiddenSources: string[];
  requireAgentIdentity: boolean;
  requireSelectedAgent: boolean;
  preferKnownAgents: boolean;
}

export interface UseActivityDerivationResult {
  filteredInstances: Instance[];
  visibleTabs: InstanceTab[];
  agentOptions: string[];
  handoffTabs: Set<string>;
  sessionsWithHandoff: Set<string>;
  agentsWithHandoff: Set<string>;
  derivedSessions: api.Session[];
  visibleAgents: Agent[];
  displayedEvents: DashboardActivityEvent[];
}

export function useActivityDerivation({
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
}: UseActivityDerivationOptions): UseActivityDerivationResult {
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

  const agentOptions = useMemo(
    () => deriveAgentOptions(catalogEvents),
    [catalogEvents],
  );

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

  // One pass partitions catalogEvents into the three nested sets that were
  // previously three separate filterMatchingEvents scans.
  const { visibleEvents, sessionCatalogEvents, agentCatalogEvents } = useMemo(
    () =>
      partitionCatalogEvents(
        catalogEvents,
        filters,
        normalizedHiddenSources,
        requireAgentIdentity,
      ),
    [catalogEvents, filters, normalizedHiddenSources, requireAgentIdentity],
  );

  const agentThreadBaseEvents = useMemo(() => {
    const liveEvents = (agentEventsById[filters.agentId] ?? []).map(
      toDashboardActivityEvent,
    );
    return mergeDashboardActivityEvents(threadEvents, liveEvents);
  }, [agentEventsById, filters.agentId, threadEvents]);

  // One pass partitions the thread base events into the two sets that were
  // previously two separate filterAgentThreadEvents scans.
  const { agentThreadEvents, agentThreadSessionCatalogEvents } = useMemo(
    () =>
      partitionAgentThreadEvents(
        agentThreadBaseEvents,
        filters,
        normalizedHiddenSources,
        requireAgentIdentity,
      ),
    [
      agentThreadBaseEvents,
      filters,
      normalizedHiddenSources,
      requireAgentIdentity,
    ],
  );

  const displayedEvents = usesAgentThreadView
    ? agentThreadEvents
    : visibleEvents;

  const derivedSessions = useMemo<api.Session[]>(() => {
    const sourceEvents =
      usesAgentThreadView && filters.agentId
        ? agentThreadSessionCatalogEvents
        : sessionCatalogEvents;
    return deriveSessions(agentSessions, sourceEvents);
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
          if (!cancelled) setAgentSessions(sessions);
        })
        .catch(() => {});
    }, 500);
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [unlabeledPtsKey, setAgentSessions]);

  const derivedAgents = useMemo<Agent[]>(
    () => deriveAgents(agentCatalogEvents),
    [agentCatalogEvents],
  );

  const visibleAgents = useMemo<Agent[]>(
    () =>
      mergeVisibleAgents(
        derivedAgents,
        agents,
        preferKnownAgents,
        requireAgentIdentity,
      ),
    [agents, derivedAgents, preferKnownAgents, requireAgentIdentity],
  );

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
  }, [
    filters.agentId,
    requireSelectedAgent,
    sidebarTab,
    visibleAgents,
    setFilters,
  ]);

  return {
    filteredInstances,
    visibleTabs,
    agentOptions,
    handoffTabs,
    sessionsWithHandoff,
    agentsWithHandoff,
    derivedSessions,
    visibleAgents,
    displayedEvents,
  };
}
