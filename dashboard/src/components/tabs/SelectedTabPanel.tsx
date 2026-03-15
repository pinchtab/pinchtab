import { useState } from "react";
import type { InstanceTab } from "../../generated/types";
import IdBadge from "./IdBadge";
import { TabsLayout, EmptyView } from "../molecules";
import ScreencastTile from "../screencast/ScreencastTile";

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

  if (!selectedTab) {
    return (
      <div className="flex flex-1 rounded-xl border border-border-subtle bg-white/2">
        <EmptyView message="Select a tab to view details" />
      </div>
    );
  }

  return (
    <div className="flex min-h-48 flex-1 flex-col rounded-xl border border-border-subtle bg-white/2 relative overflow-hidden">
      <div className="border-b border-border-subtle bg-black/5 p-4">
        <div className="flex items-center justify-between gap-4">
          <div className="min-w-0 flex-1">
            <h5 className="truncate text-base font-semibold text-text-primary">
              {selectedTab.title || "Untitled"}
            </h5>
            <div className="mt-1 truncate text-xs text-text-muted">
              {selectedTab.url}
            </div>
          </div>
          <IdBadge id={selectedTab.id} />
        </div>
      </div>
      <div className="flex-1 min-h-0">
        <TabsLayout
          tabs={subTabs}
          activeTab={activeSubTab}
          onChange={(id) => setActiveSubTab(id)}
        >
          {activeSubTab === "actions" && (
            <EmptyView message="No actions available for this tab yet." />
          )}
          {activeSubTab === "live" && (
            <div className="h-full">
              {instanceId ? (
                <div className="flex h-full items-center justify-center">
                  <div className="h-full w-full">
                    <ScreencastTile
                      key={selectedTab.id}
                      instanceId={instanceId}
                      tabId={selectedTab.id}
                      label={selectedTab.title || selectedTab.id.slice(0, 8)}
                      url={selectedTab.url}
                      showTitle={false}
                    />
                  </div>
                </div>
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
