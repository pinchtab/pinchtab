import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAppStore } from "../stores/useAppStore";
import { Button, EmptyState, ErrorBoundary, Modal } from "../components/atoms";
import InstanceListItem from "../instances/InstanceListItem";
import InstanceTabsPanel from "../tabs/InstanceTabsPanel";
import * as api from "../services/api";

const AUTO_INSTANCE_STRATEGIES = new Set(["always-on", "simple-autorestart"]);
const STARTUP_REFRESH_DELAYS_MS = [500, 1000, 1500, 2500, 4000] as const;

export default function MonitoringPage() {
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
  } = useAppStore();
  const navigate = useNavigate();
  const selectedId = selectedMonitoringInstanceId;
  const setSelectedId = setSelectedMonitoringInstanceId;
  const [strategy, setStrategy] = useState<string>("always-on");
  const [defaultProfileId, setDefaultProfileId] = useState("default");
  const [defaultLaunchMode, setDefaultLaunchMode] = useState<
    "headed" | undefined
  >(undefined);
  const [startupRetryCount, setStartupRetryCount] = useState(0);
  const [startingDefaultInstance, setStartingDefaultInstance] = useState(false);
  const [startingDefaultMode, setStartingDefaultMode] = useState<
    "headed" | "headless" | null
  >(null);
  const [showDefaultModeModal, setShowDefaultModeModal] = useState(false);
  const [launchError, setLaunchError] = useState("");
  const memoryEnabled = settings.monitoring?.memoryMetrics ?? false;

  // Fetch backend strategy once
  useEffect(() => {
    let cancelled = false;

    const load = async () => {
      try {
        const cfg = await api.fetchBackendConfig();
        if (cancelled) {
          return;
        }
        setStrategy(cfg.config.multiInstance.strategy);
        setDefaultProfileId(cfg.config.profiles.defaultProfile || "default");
        setDefaultLaunchMode(
          cfg.config.instanceDefaults.mode === "headed" ? "headed" : undefined,
        );
      } catch {
        // ignore — default to always-on
      }
    };
    void load();

    return () => {
      cancelled = true;
    };
  }, []);

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

    let cancelled = false;
    const timer = window.setTimeout(() => {
      setStartupRetryCount((count) => count + 1);
      void refreshInstances().catch(() => {
        if (!cancelled) {
          // Ignore transient startup fetch failures; the retry window remains active.
        }
      });
    }, STARTUP_REFRESH_DELAYS_MS[startupRetryCount]);

    return () => {
      cancelled = true;
      window.clearTimeout(timer);
    };
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

  const manualActions = (
    <div className="flex flex-wrap items-center justify-center gap-2">
      <Button
        variant="primary"
        onClick={() => {
          setLaunchError("");
          setShowDefaultModeModal(true);
        }}
        loading={startingDefaultInstance}
      >
        Start Default Instance
      </Button>
      <Button
        variant="secondary"
        onClick={() =>
          navigate("/dashboard/profiles", {
            state: { selectedProfileKey: defaultProfileId },
          })
        }
      >
        Open Default Profile
      </Button>
    </div>
  );

  const renderEmptyMonitoringState = () => {
    if (waitingForExpectedInstance) {
      return (
        <EmptyState
          title="Starting default instance..."
          description={`PinchTab is waiting for the default profile to come online. Checking again automatically (${startupRetriesRemaining} checks left).`}
          icon="⏳"
        />
      );
    }

    return (
      <div className="flex flex-col items-center gap-4">
        {launchError && (
          <div className="max-w-md rounded border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {launchError}
          </div>
        )}
        <EmptyState
          title="No active instances"
          description={
            expectsAutoInstance
              ? "PinchTab expected a default instance, but it never became available. Start it manually or inspect the profile."
              : "Start the default instance or open Profiles to launch a different one."
          }
          icon="📡"
          action={manualActions}
        />
      </div>
    );
  };

  return (
    <ErrorBoundary>
      <>
        <div className="flex h-full flex-col overflow-hidden">
          {instances.length === 0 && (
            <div className="flex flex-1 items-center justify-center">
              {renderEmptyMonitoringState()}
            </div>
          )}

          {instances.length > 0 && (
            <div className="dashboard-panel flex flex-1 flex-col overflow-hidden rounded-none! border-t-0">
              <div className="flex flex-1 overflow-hidden">
                {!sidebarCollapsed && (
                  <div className="w-64 shrink-0 overflow-auto border-r border-border-subtle bg-bg-surface/50">
                    <div className="flex items-center justify-between border-b border-border-subtle px-3 py-1.5">
                      <span className="text-xs font-medium text-text-muted">
                        Instances
                      </span>
                      <button
                        type="button"
                        onClick={() => setSidebarCollapsed(true)}
                        title="Collapse sidebar"
                        className="rounded p-1 text-text-muted transition-colors hover:bg-white/10 hover:text-text-secondary"
                      >
                        <svg
                          viewBox="0 0 24 24"
                          aria-hidden="true"
                          className="h-3.5 w-3.5"
                          fill="none"
                          stroke="currentColor"
                          strokeWidth="2"
                          strokeLinecap="round"
                          strokeLinejoin="round"
                        >
                          <polyline points="15 18 9 12 15 6" />
                        </svg>
                      </button>
                    </div>
                    <div>
                      {instances.map((inst) => (
                        <InstanceListItem
                          key={inst.id}
                          instance={inst}
                          tabCount={currentTabs[inst.id]?.length ?? 0}
                          memoryMB={
                            memoryEnabled ? currentMemory[inst.id] : undefined
                          }
                          selected={selectedId === inst.id}
                          autoRestart={
                            inst.profileName === "default" &&
                            (strategy === "always-on" ||
                              strategy === "simple-autorestart")
                          }
                          onClick={() => setSelectedId(inst.id)}
                          onStop={() => handleStop(inst.id)}
                          onOpenProfile={() =>
                            navigate("/dashboard/profiles", {
                              state: {
                                selectedProfileKey:
                                  inst.profileId || inst.profileName,
                              },
                            })
                          }
                        />
                      ))}
                    </div>
                  </div>
                )}

                {/* Selected instance details */}
                <div className="flex flex-1 flex-col overflow-hidden">
                  {selectedInstance ? (
                    <InstanceTabsPanel
                      tabs={selectedTabs}
                      instanceId={selectedId || undefined}
                    />
                  ) : (
                    <div className="flex flex-1 items-center justify-center px-6">
                      {renderEmptyMonitoringState()}
                    </div>
                  )}
                </div>
              </div>
            </div>
          )}
        </div>

        <Modal
          open={showDefaultModeModal}
          onClose={() => {
            if (!startingDefaultInstance) {
              setShowDefaultModeModal(false);
            }
          }}
          title="Start Default Instance"
          actions={
            <>
              <Button
                variant="secondary"
                onClick={() => setShowDefaultModeModal(false)}
                disabled={startingDefaultInstance}
              >
                Cancel
              </Button>
              <Button
                variant={prefersHeadedLaunch ? "primary" : "secondary"}
                onClick={() => {
                  void handleStartDefaultInstance("headed");
                }}
                loading={startingDefaultMode === "headed"}
                disabled={startingDefaultInstance}
              >
                Start Headed
              </Button>
              <Button
                variant={prefersHeadedLaunch ? "secondary" : "primary"}
                onClick={() => {
                  void handleStartDefaultInstance("headless");
                }}
                loading={startingDefaultMode === "headless"}
                disabled={startingDefaultInstance}
              >
                Start Headless
              </Button>
            </>
          }
        >
          <p>Choose how to launch the default profile for this session.</p>
          {launchError && (
            <div className="mt-3 rounded border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
              {launchError}
            </div>
          )}
          <p className="mt-2 text-xs text-text-muted">
            Configured default mode:{" "}
            {defaultLaunchMode === "headed" ? "headed" : "headless"}
          </p>
        </Modal>
      </>
    </ErrorBoundary>
  );
}
