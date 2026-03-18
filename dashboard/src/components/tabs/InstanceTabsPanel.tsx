import { useEffect, useMemo, useState } from "react";
import type { InstanceTab } from "../../generated/types";
import TabBar from "./TabBar";
import SelectedTabPanel from "./SelectedTabPanel";

interface Props {
  tabs: InstanceTab[];
  emptyMessage?: string;
  instanceId?: string;
}

export default function InstanceTabsPanel({
  tabs,
  emptyMessage = "No tabs open",
  instanceId,
}: Props) {
  const [selectedTabId, setSelectedTabId] = useState<string | null>(null);

  useEffect(() => {
    if (tabs.length === 0) {
      setSelectedTabId(null);
      return;
    }

    if (!tabs.some((tab) => tab.id === selectedTabId)) {
      setSelectedTabId(tabs[0].id);
    }
  }, [selectedTabId, tabs]);

  const selectedTab = useMemo(
    () => tabs.find((tab) => tab.id === selectedTabId) ?? null,
    [selectedTabId, tabs],
  );

  const heading = `Open Tabs (${tabs.length})`;

  if (tabs.length === 0) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <div className="border-b border-border-subtle px-4 py-3">
          <h2 className="text-sm font-medium text-text-primary">{heading}</h2>
        </div>
        <div className="flex flex-1 items-center justify-center py-8 text-sm text-text-muted">
          {emptyMessage}
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div className="border-b border-border-subtle px-4 py-3">
        <h2 className="text-sm font-medium text-text-primary">{heading}</h2>
      </div>
      <TabBar
        tabs={tabs}
        selectedTabId={selectedTabId}
        onSelect={setSelectedTabId}
      />
      <SelectedTabPanel selectedTab={selectedTab} instanceId={instanceId} />
    </div>
  );
}
