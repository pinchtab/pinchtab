import { useState } from "react";
import { ActivityExplorer } from "../../activities";
import type { InstanceTab } from "../../generated/types";
import IdBadge from "./IdBadge";
import { TabsLayout, EmptyView } from "../molecules";
import ScreencastTile from "../screencast/ScreencastTile";

interface Props {
  selectedTab: InstanceTab | null;
  instanceId?: string;
}

type SubTabId = "actions" | "live" | "console" | "errors";

function TabInfo({ tab }: { tab: InstanceTab }) {
  return (
    <div className="flex flex-col gap-0.5 text-right">
      <div className="flex items-center justify-end gap-1.5">
        <span className="truncate text-xs font-medium text-text-secondary">
          {tab.title || "Untitled"}
        </span>
        <IdBadge id={tab.id} />
      </div>
      <div className="truncate text-[10px] text-text-muted">{tab.url}</div>
    </div>
  );
}

export default function SelectedTabPanel({ selectedTab, instanceId }: Props) {
  const [activeSubTab, setActiveSubTab] = useState<SubTabId>("live");

  const subTabs: { id: SubTabId; label: string }[] = [
    { id: "live", label: "Live" },
    { id: "actions", label: "Actions" },
    { id: "console", label: "Console" },
    { id: "errors", label: "Errors" },
  ];

  if (!selectedTab) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-text-muted">
        Select a tab to view details
      </div>
    );
  }

  return (
    <div className="flex min-h-48 flex-1 flex-col overflow-hidden">
      <div className="flex-1 min-h-0">
        <TabsLayout
          tabs={subTabs}
          activeTab={activeSubTab}
          onChange={(id) => setActiveSubTab(id)}
          rightSlot={<TabInfo tab={selectedTab} />}
        >
          {activeSubTab === "actions" && (
            <div className="h-full">
              <ActivityExplorer
                embedded
                showFilterMenu={false}
                title=""
                summaryLabel="Actions"
                initialFilters={{
                  instanceId: instanceId || "",
                  tabId: selectedTab.id,
                }}
                lockedFilters={{
                  tabId: selectedTab.id,
                }}
              />
            </div>
          )}
          {activeSubTab === "live" && (
            <div className="h-full">
              {instanceId ? (
                <ScreencastTile
                  key={selectedTab.id}
                  instanceId={instanceId}
                  tabId={selectedTab.id}
                  label={selectedTab.title || selectedTab.id.slice(0, 8)}
                  url={selectedTab.url}
                  showTitle={false}
                />
              ) : (
                <EmptyView message="No instance ID provided for live view." />
              )}
            </div>
          )}
          {activeSubTab === "console" && (
            <EmptyView message="Console logs for this tab will appear here." />
          )}
          {activeSubTab === "errors" && (
            <EmptyView message="Runtime errors for this tab will appear here." />
          )}
        </TabsLayout>
      </div>
    </div>
  );
}
