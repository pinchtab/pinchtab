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

  if (tabs.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center py-8 text-sm text-text-muted">
        {emptyMessage}
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <TabBar
        tabs={tabs}
        selectedTabId={selectedTabId}
        onSelect={setSelectedTabId}
      />
      <SelectedTabPanel selectedTab={selectedTab} instanceId={instanceId} />
    </div>
  );
}
