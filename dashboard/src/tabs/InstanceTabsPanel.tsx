import { useEffect, useMemo, useState } from "react";
import type { InstanceTab } from "../generated/types";
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
  const [selectionPinned, setSelectionPinned] = useState(false);

  useEffect(() => {
    if (tabs.length === 0) {
      setSelectedTabId(null);
      setSelectionPinned(false);
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
  }, [selectedTabId, selectionPinned, tabs]);

  const selectedTab = useMemo(
    () => tabs.find((tab) => tab.id === selectedTabId) ?? null,
    [selectedTabId, tabs],
  );

  if (tabs.length === 0) {
    return (
      <div className="flex min-h-0 flex-1 flex-col">
        <div className="flex flex-1 items-center justify-center py-8 text-sm text-text-muted">
          {emptyMessage}
        </div>
      </div>
    );
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <TabBar
        tabs={tabs}
        selectedTabId={selectedTabId}
        pinnedTabId={selectionPinned ? selectedTabId : null}
        onSelect={(id) => {
          setSelectedTabId(id);
          setSelectionPinned(true);
        }}
        onTogglePinned={(id) => {
          if (selectionPinned && selectedTabId === id) {
            setSelectionPinned(false);
            setSelectedTabId(tabs[0]?.id ?? null);
            return;
          }
          setSelectedTabId(id);
          setSelectionPinned(true);
        }}
      />
      <SelectedTabPanel selectedTab={selectedTab} instanceId={instanceId} />
    </div>
  );
}
