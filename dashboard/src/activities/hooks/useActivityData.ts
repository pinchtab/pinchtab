import { useMemo, useState } from "react";
import { useAsyncResource } from "../../hooks/useAsyncResource";
import * as api from "../../services/api";
import type { ActivityEvent, InstanceTab } from "../../types";
import { fetchActivity } from "../api";
import { buildActivityQuery } from "../helpers";
import {
  normalizeDashboardActivityEvent,
  withClearedSessionFilter,
} from "../selectors";
import type { ActivityFilters, DashboardActivityEvent } from "../types";
import { useAllTabs } from "./useAllTabs";

interface UseActivityDataOptions {
  deferredFilters: ActivityFilters;
  usesAgentThreadView: boolean;
  threadAgentId: string;
  hydrateAgentEvents: (agentId: string, events: ActivityEvent[]) => void;
}

export interface UseActivityDataResult {
  catalogEvents: DashboardActivityEvent[];
  threadEvents: DashboardActivityEvent[];
  tabs: InstanceTab[];
  agentSessions: api.Session[];
  setAgentSessions: React.Dispatch<React.SetStateAction<api.Session[]>>;
  activityLoading: boolean;
  agentLoading: boolean;
  error: string;
  refresh: () => void;
}

export function useActivityData({
  deferredFilters,
  usesAgentThreadView,
  threadAgentId,
  hydrateAgentEvents,
}: UseActivityDataOptions): UseActivityDataResult {
  const [refreshNonce, setRefreshNonce] = useState(0);

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

  const sessions = useAsyncResource(api.fetchSessions, [refreshNonce], {
    initial: [] as api.Session[],
  });

  const tabs = useAllTabs(refreshNonce);

  const catalog = useAsyncResource(
    async () => {
      const response = await fetchActivity(catalogQuery);
      return response.events.map(normalizeDashboardActivityEvent);
    },
    [catalogQueryKey, refreshNonce],
    {
      initial: [] as DashboardActivityEvent[],
      errorMessage: "Failed to load activity",
    },
  );

  const thread = useAsyncResource(
    usesAgentThreadView && threadAgentId
      ? async (signal) => {
          const [detail, response] = await Promise.all([
            api.fetchAgent(threadAgentId),
            threadQuery
              ? fetchActivity(threadQuery)
              : Promise.resolve({ count: 0, events: [] }),
          ]);
          if (!signal.aborted) {
            hydrateAgentEvents(detail.agent.id, detail.events);
          }
          return response.events.map(normalizeDashboardActivityEvent);
        }
      : null,
    [
      hydrateAgentEvents,
      refreshNonce,
      threadAgentId,
      threadQueryKey,
      usesAgentThreadView,
    ],
    {
      initial: [] as DashboardActivityEvent[],
      errorMessage: "Failed to load agent activity",
    },
  );

  const refresh = () => setRefreshNonce((current) => current + 1);

  return {
    catalogEvents: catalog.data,
    threadEvents: thread.data,
    tabs,
    agentSessions: sessions.data,
    setAgentSessions: sessions.setData,
    activityLoading: catalog.loading,
    agentLoading: thread.loading,
    error: thread.error || catalog.error,
    refresh,
  };
}
