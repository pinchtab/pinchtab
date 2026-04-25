import { useEffect, useMemo, useRef, useState } from "react";
import type { InstanceTab } from "../generated/types";
import { useAppStore } from "../stores/useAppStore";
import { SidebarPanel, TabsChart } from "../components/molecules";
import InstanceStats from "../components/molecules/InstanceStats";
import { ErrorBoundary } from "../components/atoms";
import TabBar from "./TabBar";
import SelectedTabPanel from "./SelectedTabPanel";

interface Props {
  tabs: InstanceTab[];
  emptyMessage?: string;
  instanceId?: string;
  handoffTabs?: Set<string>;
}

function sameIds(left: string[], right: string[]): boolean {
  return (
    left.length === right.length &&
    left.every((value, index) => value === right[index])
  );
}

export default function InstanceTabsPanel({
  tabs,
  emptyMessage = "No tabs open",
  instanceId,
  handoffTabs,
}: Props) {
  const [selectedTabId, setSelectedTabId] = useState<string | null>(null);
  const [selectionPinned, setSelectionPinned] = useState(false);
  const [acknowledgedTabIds, setAcknowledgedTabIds] = useState<string[]>(() =>
    tabs.map((tab) => tab.id),
  );
  const previousInstanceIdRef = useRef(instanceId);

  const {
    instances,
    tabsChartData,
    memoryChartData,
    serverChartData,
    currentMetrics,
    settings,
    monitoringShowTelemetry: showTelemetry,
    setMonitoringShowTelemetry: setShowTelemetry,
  } = useAppStore();

  const memoryEnabled = settings.monitoring?.memoryMetrics ?? false;
  const currentTabIds = useMemo(() => tabs.map((tab) => tab.id), [tabs]);

  const selectedInstance = instances.find((i) => i.id === instanceId);
  const chartInstances = useMemo(
    () =>
      selectedInstance
        ? [
            {
              id: selectedInstance.id,
              profileName: selectedInstance.profileName || "Unknown",
            },
          ]
        : [],
    [selectedInstance],
  );

  useEffect(() => {
    if (tabs.length === 0) {
      setSelectedTabId(null);
      setSelectionPinned(false);
      if (!showTelemetry) {
        setShowTelemetry(true);
      }
      return;
    }

    if (selectionPinned && tabs.some((tab) => tab.id === selectedTabId)) {
      return;
    }

    if (
      !tabs.some((tab) => tab.id === selectedTabId) ||
      selectedTabId !== tabs[0].id
    ) {
      if (selectedTabId !== tabs[0].id) {
        setSelectedTabId(tabs[0].id);
      }
      if (selectionPinned) {
        setSelectionPinned(false);
      }
    }
  }, [selectedTabId, selectionPinned, tabs, showTelemetry, setShowTelemetry]);

  const selectedTab = useMemo(
    () => tabs.find((tab) => tab.id === selectedTabId) ?? null,
    [selectedTabId, tabs],
  );
  const newTabsCount = useMemo(() => {
    if (!showTelemetry) {
      return 0;
    }
    const acknowledged = new Set(acknowledgedTabIds);
    return currentTabIds.reduce(
      (count, tabId) => count + (acknowledged.has(tabId) ? 0 : 1),
      0,
    );
  }, [acknowledgedTabIds, currentTabIds, showTelemetry]);

  useEffect(() => {
    if (previousInstanceIdRef.current === instanceId) {
      return;
    }
    previousInstanceIdRef.current = instanceId;
    setAcknowledgedTabIds(currentTabIds);
  }, [currentTabIds, instanceId]);

  useEffect(() => {
    if (!showTelemetry) {
      setAcknowledgedTabIds((previous) =>
        sameIds(previous, currentTabIds) ? previous : currentTabIds,
      );
    }
  }, [currentTabIds, showTelemetry]);

  return (
    <SidebarPanel
      className="flex-1"
      scrollContent={false}
      contentClassName="flex min-h-0 flex-1 flex-col overflow-hidden"
      header={
        <TabBar
          tabs={tabs}
          selectedTabId={selectedTabId}
          pinnedTabId={selectionPinned ? selectedTabId : null}
          telemetryActive={showTelemetry}
          newTabsCount={newTabsCount}
          handoffTabs={handoffTabs}
          onSelect={(id) => {
            setAcknowledgedTabIds(currentTabIds);
            setSelectedTabId(id);
            setSelectionPinned(true);
            if (showTelemetry) {
              setShowTelemetry(false);
            }
          }}
          onTogglePinned={(id) => {
            if (selectionPinned && selectedTabId === id) {
              setSelectionPinned(false);
              setSelectedTabId(tabs[0]?.id ?? null);
              return;
            }
            setAcknowledgedTabIds(currentTabIds);
            setSelectedTabId(id);
            setSelectionPinned(true);
            if (showTelemetry) {
              setShowTelemetry(false);
            }
          }}
          onSetTelemetry={(active) => {
            if (!active) {
              setAcknowledgedTabIds(currentTabIds);
            }
            if (active !== showTelemetry) {
              setShowTelemetry(active);
            }
          }}
        />
      }
    >
      {tabs.length === 0 && !showTelemetry ? (
        <div className="flex flex-1 items-center justify-center py-8 text-sm text-text-muted">
          {emptyMessage}
        </div>
      ) : showTelemetry ? (
        <div className="flex-1 overflow-auto">
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
              instances={chartInstances}
              selectedInstanceId={instanceId || null}
              onSelectInstance={() => {}}
            />
          </ErrorBoundary>
          <InstanceStats
            instance={selectedInstance}
            metrics={instanceId ? currentMetrics[instanceId] : null}
            tabs={tabs}
          />
        </div>
      ) : (
        <SelectedTabPanel selectedTab={selectedTab} instanceId={instanceId} />
      )}
    </SidebarPanel>
  );
}
