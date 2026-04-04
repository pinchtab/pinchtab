import { useEffect, useMemo, useRef, useState } from "react";
import { useLocation } from "react-router-dom";
import { useAppStore } from "../stores/useAppStore";
import type { ActivityFilters } from "./types";
import AgentActivityWorkspace from "./AgentActivityWorkspace";

interface ActivityPageLocationState {
  profileName?: string;
  instanceId?: string;
  tabId?: string;
}

function hasInitialSelection(filters: Partial<ActivityFilters>): boolean {
  return Boolean(filters.profileName || filters.instanceId || filters.tabId);
}

export default function ActivityPage() {
  const location = useLocation();
  const { profiles, instances, currentTabs } = useAppStore();
  const routeState = location.state as ActivityPageLocationState | null;
  const locationKeyRef = useRef(location.key);

  const routeInitialFilters = useMemo<Partial<ActivityFilters>>(
    () => ({
      profileName: routeState?.profileName ?? "",
      instanceId: routeState?.instanceId ?? "",
      tabId: routeState?.tabId ?? "",
    }),
    [routeState],
  );

  const derivedInitialFilters = useMemo<Partial<ActivityFilters>>(() => {
    let profileName = routeInitialFilters.profileName ?? "";
    let instanceId = routeInitialFilters.instanceId ?? "";
    let tabId = routeInitialFilters.tabId ?? "";

    const runningInstances = instances.filter(
      (instance) => instance.status === "running",
    );

    let selectedInstance =
      (instanceId
        ? instances.find((instance) => instance.id === instanceId)
        : undefined) ?? null;

    if (!selectedInstance) {
      const candidateInstances = profileName
        ? runningInstances.filter(
            (instance) => instance.profileName === profileName,
          )
        : runningInstances;
      if (candidateInstances.length === 1) {
        selectedInstance = candidateInstances[0];
      }
    }

    if (!profileName) {
      if (selectedInstance) {
        profileName = selectedInstance.profileName;
      } else if (profiles.length === 1) {
        profileName = profiles[0].name;
      }
    }

    if (!selectedInstance && profileName) {
      const matchingInstances = runningInstances.filter(
        (instance) => instance.profileName === profileName,
      );
      if (matchingInstances.length === 1) {
        selectedInstance = matchingInstances[0];
      }
    }

    if (selectedInstance && !instanceId) {
      instanceId = selectedInstance.id;
    }

    if (!tabId && selectedInstance) {
      const activeTabs = currentTabs[selectedInstance.id] ?? [];
      if (activeTabs.length > 0) {
        tabId = activeTabs[0].id;
      }
    }

    return {
      profileName,
      instanceId,
      tabId,
    };
  }, [currentTabs, instances, profiles, routeInitialFilters]);

  const [initialFilters, setInitialFilters] = useState<
    Partial<ActivityFilters>
  >(() =>
    hasInitialSelection(routeInitialFilters)
      ? routeInitialFilters
      : derivedInitialFilters,
  );
  const [didAutoSelect, setDidAutoSelect] = useState<boolean>(
    () =>
      hasInitialSelection(routeInitialFilters) ||
      hasInitialSelection(derivedInitialFilters),
  );

  useEffect(() => {
    if (locationKeyRef.current === location.key) {
      return;
    }
    locationKeyRef.current = location.key;
    const nextInitialFilters = hasInitialSelection(routeInitialFilters)
      ? routeInitialFilters
      : derivedInitialFilters;
    setInitialFilters(nextInitialFilters);
    setDidAutoSelect(hasInitialSelection(nextInitialFilters));
  }, [derivedInitialFilters, location.key, routeInitialFilters]);

  useEffect(() => {
    if (didAutoSelect || !hasInitialSelection(derivedInitialFilters)) {
      return;
    }
    setInitialFilters(derivedInitialFilters);
    setDidAutoSelect(true);
  }, [derivedInitialFilters, didAutoSelect]);

  return (
    <AgentActivityWorkspace
      initialFilters={initialFilters}
      defaultSidebarTab="activities"
      preferKnownAgents
    />
  );
}
