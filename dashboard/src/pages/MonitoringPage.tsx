import { useEffect, useState, useCallback, useRef } from "react";
import { useAppStore } from "../stores/useAppStore";
import { EmptyState, Button, ErrorBoundary } from "../components/atoms";
import { TabsChart, InstanceListItem, TabItem } from "../components/molecules";
import type { InstanceTab } from "../generated/types";
import * as api from "../services/api";

const DEFAULT_POLL_INTERVAL = 30; // seconds

export default function MonitoringPage() {
  const {
    instances,
    setInstances,
    setInstancesLoading,
    tabsChartData,
    memoryChartData,
    serverChartData,
    currentTabs,
    currentMemory,
    addChartDataPoint,
    addMemoryDataPoint,
    addServerDataPoint,
    setCurrentTabs,
    setCurrentMemory,
    settings,
  } = useAppStore();
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const loadInstances = async () => {
    setInstancesLoading(true);
    try {
      const data = await api.fetchInstances();
      setInstances(data);
    } catch (e) {
      console.error("Failed to load instances", e);
    } finally {
      setInstancesLoading(false);
    }
  };

  // Fetch tabs and optionally memory for all running instances
  const memoryEnabled = settings.monitoring?.memoryMetrics ?? false;
  const pollInterval =
    (settings.monitoring?.pollInterval || DEFAULT_POLL_INTERVAL) * 1000;

  const fetchAllInstanceData = useCallback(async () => {
    // Guard against undefined/null instances
    if (!instances || !Array.isArray(instances)) return;

    // Only poll instances that have been running for at least 5 seconds
    // (Chrome needs time to initialize after status becomes "running")
    const now = Date.now();
    const runningInstances = instances.filter((i) => {
      if (i?.status !== "running") return false;
      const startTime = i.startTime ? new Date(i.startTime).getTime() : 0;
      const ageMs = now - startTime;
      return ageMs > 5000; // 5 second grace period
    });
    const timestamp = now;

    try {
      // Always fetch server metrics
      const serverMetrics = await api.fetchServerMetrics().catch(() => null);
      if (serverMetrics) {
        addServerDataPoint({
          timestamp,
          goHeapMB: serverMetrics.goHeapAllocMB ?? 0,
          goroutines: serverMetrics.goNumGoroutine ?? 0,
          rateBucketHosts: serverMetrics.rateBucketHosts ?? 0,
        });
      }

      // Skip instance data if no stable running instances
      // (this prevents calling /instances/tabs which proxies to ALL instances,
      // including fresh ones that might not be ready yet)
      if (runningInstances.length === 0) {
        // Clear stale data when no stable instances
        setCurrentTabs({});
        setCurrentMemory({});
        return;
      }

      // Fetch tabs and instance metrics
      const [allTabs, allMetrics] = await Promise.all([
        api.fetchAllTabs().catch(() => []),
        memoryEnabled ? api.fetchAllMetrics().catch(() => []) : [],
      ]);

      const tabsArray = Array.isArray(allTabs) ? allTabs : [];
      const metricsArray = Array.isArray(allMetrics) ? allMetrics : [];

      const tabDataPoint: Record<string, number> = { timestamp };
      const memDataPoint: Record<string, number> = { timestamp };
      const tabsByInstance: Record<string, InstanceTab[]> = {};
      const memoryByInstance: Record<string, number> = {};

      // Group tabs by instance
      for (const inst of runningInstances) {
        const instTabs = tabsArray.filter((t) => t.instanceId === inst.id);
        tabDataPoint[inst.id] = instTabs.length;
        tabsByInstance[inst.id] = instTabs;

        // Find memory for this instance (if enabled)
        if (memoryEnabled) {
          const instMem = metricsArray.find((m) => m.instanceId === inst.id);
          if (instMem) {
            memDataPoint[inst.id] = instMem.jsHeapUsedMB;
            memoryByInstance[inst.id] = instMem.jsHeapUsedMB;
          }
        }
      }

      addChartDataPoint(
        tabDataPoint as Parameters<typeof addChartDataPoint>[0],
      );
      if (memoryEnabled) {
        addMemoryDataPoint(
          memDataPoint as Parameters<typeof addMemoryDataPoint>[0],
        );
      }
      setCurrentTabs(tabsByInstance);
      setCurrentMemory(memoryByInstance);
    } catch (e) {
      console.error("Failed to fetch instance data:", e);
    }
  }, [
    instances,
    memoryEnabled,
    addChartDataPoint,
    addMemoryDataPoint,
    addServerDataPoint,
    setCurrentTabs,
    setCurrentMemory,
  ]);

  // Load once on mount if empty — intentionally omitting deps to avoid refetch loops
  useEffect(() => {
    if (instances.length === 0) {
      loadInstances();
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Poll tabs
  useEffect(() => {
    fetchAllInstanceData();
    pollRef.current = setInterval(fetchAllInstanceData, pollInterval);
    return () => {
      if (pollRef.current) clearInterval(pollRef.current);
    };
  }, [fetchAllInstanceData, pollInterval]);

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

                  {/* Tabs list */}
                  <div className="flex-1 overflow-auto p-3">
                    <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">
                      Open Tabs ({selectedTabs.length})
                    </h4>
                    {selectedTabs.length === 0 ? (
                      <div className="py-8 text-center text-sm text-text-muted">
                        No tabs open
                      </div>
                    ) : (
                      <div className="space-y-1">
                        {selectedTabs.map((tab) => (
                          <TabItem key={tab.id} tab={tab} />
                        ))}
                      </div>
                    )}
                  </div>
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
