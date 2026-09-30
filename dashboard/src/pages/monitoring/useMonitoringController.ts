import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAsyncResource } from "../../hooks/useAsyncResource";
import { useAppStore } from "../../stores/useAppStore";
import * as api from "../../services/api";

const AUTO_INSTANCE_STRATEGIES = new Set(["always-on", "simple-autorestart"]);
const STARTUP_REFRESH_DELAYS_MS = [500, 1000, 1500, 2500, 4000] as const;
const DEFAULT_LAUNCH_CONFIG: {
  strategy: string;
  defaultProfileId: string;
  defaultLaunchMode: "headed" | undefined;
} = {
  strategy: "always-on",
  defaultProfileId: "default",
  defaultLaunchMode: undefined,
};

export function useMonitoringController() {
  const {
    instances,
    currentTabs,
    currentMemory,
    settings,
    monitoringSidebarCollapsed: sidebarCollapsed,
    setMonitoringSidebarCollapsed: setSidebarCollapsed,
    selectedMonitoringInstanceId,
    setSelectedMonitoringInstanceId,
    setInstances,
    handoffNotifications,
  } = useAppStore();

  const handoffTabs = useMemo(
    () => new Set(handoffNotifications.map((n) => n.tabId)),
    [handoffNotifications],
  );
  const navigate = useNavigate();
  const selectedId = selectedMonitoringInstanceId;
  const setSelectedId = setSelectedMonitoringInstanceId;
  const [startupRetryCount, setStartupRetryCount] = useState(0);
  const [startingDefaultInstance, setStartingDefaultInstance] = useState(false);
  const [startingDefaultMode, setStartingDefaultMode] = useState<
    "headed" | "headless" | null
  >(null);
  const [showDefaultModeModal, setShowDefaultModeModal] = useState(false);
  const [launchError, setLaunchError] = useState("");
  const memoryEnabled = settings.monitoring?.memoryMetrics ?? false;

  const {
    data: { strategy, defaultProfileId, defaultLaunchMode },
  } = useAsyncResource(
    async () => {
      const cfg = await api.fetchBackendConfig();
      return {
        strategy: cfg.config.multiInstance.strategy,
        defaultProfileId: cfg.config.profiles.defaultProfile || "default",
        defaultLaunchMode:
          cfg.config.instanceDefaults.mode === "headed"
            ? ("headed" as const)
            : undefined,
      };
    },
    [],
    { initial: DEFAULT_LAUNCH_CONFIG },
  );

  const refreshInstances = useCallback(async () => {
    const updated = await api.fetchInstances();
    setInstances(updated);
    return updated;
  }, [setInstances]);

  const expectsAutoInstance = AUTO_INSTANCE_STRATEGIES.has(strategy);
  const availableInstance = useMemo(
    () =>
      instances.find((instance) => instance.status === "running") ??
      instances.find((instance) => instance.status === "starting") ??
      null,
    [instances],
  );
  const hasAvailableInstance = availableInstance !== null;
  const selectedInstance = instances.find(
    (instance) => instance.id === selectedId,
  );
  const selectedTabs = selectedId ? currentTabs?.[selectedId] || [] : [];

  // Auto-select the best available instance, including instances that are still starting.
  useEffect(() => {
    if (selectedInstance) {
      return;
    }
    if (availableInstance) {
      setSelectedId(availableInstance.id);
      return;
    }
    if (selectedId) {
      setSelectedId(null);
    }
  }, [availableInstance, selectedId, selectedInstance, setSelectedId]);

  // When the config expects a default instance, poll a few times during startup
  // so the monitoring page doesn't immediately fall back to the manual empty state.
  useEffect(() => {
    if (
      !expectsAutoInstance ||
      hasAvailableInstance ||
      startupRetryCount >= STARTUP_REFRESH_DELAYS_MS.length
    ) {
      return;
    }

    const timer = window.setTimeout(() => {
      setStartupRetryCount((count) => count + 1);
      void refreshInstances().catch(() => {});
    }, STARTUP_REFRESH_DELAYS_MS[startupRetryCount]);

    return () => window.clearTimeout(timer);
  }, [
    expectsAutoInstance,
    hasAvailableInstance,
    refreshInstances,
    startupRetryCount,
  ]);

  const handleStop = async (id: string) => {
    try {
      await api.stopInstance(id);
      await refreshInstances();
    } catch (e) {
      console.error("Failed to stop instance", e);
    }
  };

  const handleStartDefaultInstance = async (mode: "headed" | "headless") => {
    if (startingDefaultInstance) {
      return;
    }

    setLaunchError("");
    setStartingDefaultInstance(true);
    setStartingDefaultMode(mode);

    try {
      await api.launchInstance({
        profileId: defaultProfileId,
        mode: mode === "headed" ? "headed" : undefined,
      });
      await refreshInstances();
      setStartupRetryCount(0);
      setShowDefaultModeModal(false);
    } catch (error) {
      console.error("Failed to start default instance", error);
      setLaunchError(
        error instanceof Error ? error.message : "Failed to start instance",
      );
    } finally {
      setStartingDefaultMode(null);
      setStartingDefaultInstance(false);
    }
  };

  const startupRetriesRemaining = Math.max(
    0,
    STARTUP_REFRESH_DELAYS_MS.length - startupRetryCount,
  );
  const waitingForExpectedInstance =
    expectsAutoInstance && !hasAvailableInstance && startupRetriesRemaining > 0;
  const prefersHeadedLaunch = defaultLaunchMode === "headed";

  return {
    navigate,
    instances,
    currentTabs,
    currentMemory,
    sidebarCollapsed,
    setSidebarCollapsed,
    selectedId,
    setSelectedId,
    selectedInstance,
    selectedTabs,
    strategy,
    memoryEnabled,
    handoffTabs,
    defaultProfileId,
    defaultLaunchMode,
    expectsAutoInstance,
    waitingForExpectedInstance,
    startupRetriesRemaining,
    startingDefaultInstance,
    startingDefaultMode,
    prefersHeadedLaunch,
    showDefaultModeModal,
    setShowDefaultModeModal,
    launchError,
    setLaunchError,
    handleStop,
    handleStartDefaultInstance,
  };
}
