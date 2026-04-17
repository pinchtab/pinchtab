import { useMemo } from "react";
import { useLocation } from "react-router-dom";
import type { ActivityFilters } from "./types";
import AgentActivityWorkspace from "./AgentActivityWorkspace";

interface ActivityPageLocationState {
  profileName?: string;
  instanceId?: string;
  tabId?: string;
}

export default function ActivityPage() {
  const location = useLocation();
  const routeState = location.state as ActivityPageLocationState | null;

  const initialFilters = useMemo<Partial<ActivityFilters>>(
    () => ({
      profileName: routeState?.profileName ?? "",
      instanceId: routeState?.instanceId ?? "",
      tabId: routeState?.tabId ?? "",
      ageSec: "",
      limit: "1000",
    }),
    [routeState],
  );

  return (
    <AgentActivityWorkspace
      key={location.key}
      initialFilters={initialFilters}
      defaultSidebarTab="activities"
      preferKnownAgents
    />
  );
}
