import { useEffect, useMemo, useState } from "react";
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
  const [catalogEvents, setCatalogEvents] = useState<DashboardActivityEvent[]>(
    [],
  );
  const [threadEvents, setThreadEvents] = useState<DashboardActivityEvent[]>(
    [],
  );
  const [activityLoading, setActivityLoading] = useState(false);
  const [agentLoading, setAgentLoading] = useState(false);
  const [agentSessions, setAgentSessions] = useState<api.Session[]>([]);
  const [error, setError] = useState("");
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

  useEffect(() => {
    let cancelled = false;
    void api
      .fetchSessions()
      .then((sessions) => {
        if (!cancelled) setAgentSessions(sessions);
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [refreshNonce]);

  // Keyed on refreshNonce so tab filters refresh alongside sessions/activity
  // (previously mount-only, which left tab filters stale after refresh/churn).
  const tabs = useAllTabs(refreshNonce);

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

  const refresh = () => setRefreshNonce((current) => current + 1);

  return {
    catalogEvents,
    threadEvents,
    tabs,
    agentSessions,
    setAgentSessions,
    activityLoading,
    agentLoading,
    error,
    refresh,
  };
}
