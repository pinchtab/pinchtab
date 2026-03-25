import { useMemo, useState } from "react";
import { ActivityExplorer } from "../activities";
import type { InstanceTab } from "../generated/types";
import { TabsLayout, EmptyView } from "../components/molecules";
import ScreencastTile from "../components/screencast/ScreencastTile";
import SelectedTabTitle from "./SelectedTabTitle";

interface Props {
  selectedTab: InstanceTab | null;
  instanceId?: string;
}

type SubTabId = "actions" | "live" | "console" | "errors";

export default function SelectedTabPanel({ selectedTab, instanceId }: Props) {
  const [activeSubTab, setActiveSubTab] = useState<SubTabId>("live");

  const subTabs: { id: SubTabId; label: string }[] = [
    { id: "live", label: "Live" },
    { id: "actions", label: "Actions" },
    { id: "console", label: "Console" },
    { id: "errors", label: "Errors" },
  ];

  const activityInitialFilters = useMemo(
    () => ({ instanceId: instanceId || "", tabId: selectedTab?.id ?? "" }),
    [instanceId, selectedTab?.id],
  );
  const activityLockedFilters = useMemo(
    () => ({ tabId: selectedTab?.id ?? "" }),
    [selectedTab?.id],
  );

  if (!selectedTab) {
    return (
      <div className="flex flex-1 items-center justify-center text-sm text-text-muted">
        Select a tab to view details
      </div>
    );
  }

  return (
    <div className="flex min-h-48 flex-1 flex-col overflow-hidden rounded-xl">
      <div className="flex-1 min-h-0">
        <TabsLayout
          tabs={subTabs}
          activeTab={activeSubTab}
          onChange={(id) => setActiveSubTab(id)}
          rightSlot={<SelectedTabTitle tab={selectedTab} />}
        >
          {activeSubTab === "actions" && (
            <div className="h-full">
              <ActivityExplorer
                embedded
                showFilterMenu={false}
                title=""
                summaryLabel="Actions"
                initialFilters={activityInitialFilters}
                lockedFilters={activityLockedFilters}
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
