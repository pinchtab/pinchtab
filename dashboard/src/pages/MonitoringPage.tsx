import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAppStore } from "../stores/useAppStore";
import { EmptyState, ErrorBoundary } from "../components/atoms";
import { TabsChart } from "../components/molecules";
import InstanceListItem from "../instances/InstanceListItem";
import InstanceTabsPanel from "../tabs/InstanceTabsPanel";
import * as api from "../services/api";

export default function MonitoringPage() {
  const {
    instances,
    tabsChartData,
    memoryChartData,
    serverChartData,
    currentTabs,
    currentMemory,
    settings,
  } = useAppStore();
  const navigate = useNavigate();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [strategy, setStrategy] = useState<string>("always-on");
  const memoryEnabled = settings.monitoring?.memoryMetrics ?? false;

  // Fetch backend strategy once
  useEffect(() => {
    const load = async () => {
      try {
        const cfg = await api.fetchBackendConfig();
        setStrategy(cfg.config.multiInstance.strategy);
      } catch {
        // ignore — default to always-on
      }
    };
    load();
  }, []);

  // Auto-select first running instance
  useEffect(() => {
    if (!selectedId) {
      const firstRunning = instances.find((i) => i.status === "running");
      if (firstRunning) setSelectedId(firstRunning.id);
    }
  }, [instances, selectedId]);

  const handleStop = async (id: string) => {
    try {
      await api.stopInstance(id);
    } catch (e) {
      console.error("Failed to stop instance", e);
    }
  };

  const selectedInstance = instances?.find((i) => i.id === selectedId);
  const selectedTabs = selectedId ? currentTabs?.[selectedId] || [] : [];
  const runningInstances =
    instances?.filter((i) => i?.status === "running") || [];

  return (
    <ErrorBoundary>
      <div className="flex h-full flex-col gap-4 overflow-hidden p-4">
        <ErrorBoundary
          fallback={
            <div className="flex h-50 items-center justify-center rounded-lg border border-destructive/50 bg-bg-surface text-sm text-destructive">
              Chart crashed - check console
            </div>
          }
        >
          <TabsChart
            data={tabsChartData || []}
            memoryData={memoryEnabled ? memoryChartData : undefined}
            serverData={serverChartData || []}
            instances={runningInstances.map((i) => ({
              id: i.id,
              profileName: i.profileName || "Unknown",
            }))}
            selectedInstanceId={selectedId}
            onSelectInstance={setSelectedId}
          />
        </ErrorBoundary>

        {instances.length === 0 && (
          <div className="flex flex-1 items-center justify-center">
            <EmptyState
              title="No instances yet"
              description="Start a profile to see instance data"
              icon="📡"
            />
          </div>
        )}

        {instances.length > 0 && (
          <div className="flex flex-1 gap-4 overflow-hidden">
            <div className="dashboard-panel w-64 shrink-0 overflow-auto">
              <div className="p-2">
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

            {/* Selected instance details */}
            <div className="dashboard-panel flex flex-1 flex-col overflow-hidden">
              {selectedInstance ? (
                <InstanceTabsPanel
                  tabs={selectedTabs}
                  instanceId={selectedId || undefined}
                />
              ) : (
                <div className="flex flex-1 items-center justify-center text-sm text-text-muted">
                  Select an instance to view details
                </div>
              )}
            </div>
          </div>
        )}
      </div>
    </ErrorBoundary>
  );
}
