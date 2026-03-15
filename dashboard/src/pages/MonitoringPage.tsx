import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAppStore } from "../stores/useAppStore";
import { EmptyState, Button, ErrorBoundary } from "../components/atoms";
import { TabsChart } from "../components/molecules";
import InstanceListItem from "../components/instances/InstanceListItem";
import InstanceTabsPanel from "../components/tabs/InstanceTabsPanel";
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
  const memoryEnabled = settings.monitoring?.memoryMetrics ?? false;

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
        {/* Chart - always show, even with no instances (displays server metrics) */}
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

        {/* Bottom section - only show when instances exist */}
        {instances.length > 0 && (
          <div className="flex flex-1 gap-4 overflow-hidden">
            {/* Instance list */}
            <div className="dashboard-panel w-64 shrink-0 overflow-auto">
              <div className="border-b border-border-subtle px-4 py-3">
                <div className="dashboard-section-label mb-1">Monitoring</div>
                <h3 className="text-sm font-semibold text-text-secondary">
                  Instances ({instances.length})
                </h3>
              </div>
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
                    onClick={() => setSelectedId(inst.id)}
                  />
                ))}
              </div>
            </div>

            {/* Selected instance details */}
            <div className="dashboard-panel flex flex-1 flex-col overflow-hidden">
              {selectedInstance ? (
                <>
                  <div className="flex items-center justify-between border-b border-border-subtle px-4 py-3">
                    <div>
                      <div className="dashboard-section-title mb-1">
                        Active instance
                      </div>
                      <h3 className="text-sm font-semibold text-text-primary">
                        {selectedInstance.profileName}
                      </h3>
                      <div className="dashboard-mono text-xs text-text-muted">
                        Port {selectedInstance.port} ·{" "}
                        {selectedInstance.headless ? "Headless" : "Headed"}
                      </div>
                    </div>
                    <div className="flex items-center gap-2">
                      <Button
                        size="sm"
                        variant="secondary"
                        onClick={() =>
                          navigate("/dashboard/profiles", {
                            state: {
                              selectedProfileKey:
                                selectedInstance.profileId ||
                                selectedInstance.profileName,
                            },
                          })
                        }
                      >
                        Open Profile
                      </Button>
                      {selectedInstance.status === "running" && (
                        <Button
                          size="sm"
                          variant="danger"
                          onClick={() => handleStop(selectedInstance.id)}
                        >
                          Stop
                        </Button>
                      )}
                    </div>
                  </div>
                  <InstanceTabsPanel
                    tabs={selectedTabs}
                    instanceId={selectedId || undefined}
                  />
                </>
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
