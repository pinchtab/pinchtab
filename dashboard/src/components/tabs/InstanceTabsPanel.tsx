import { useEffect, useMemo, useState } from "react";
import type { InstanceTab } from "../../generated/types";
import TabItem from "./TabItem";
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

  return (
    <div className="flex flex-1 min-h-0 flex-col p-3">
      <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">
        Open Tabs ({tabs.length})
      </h4>

      {tabs.length === 0 ? (
        <div className="py-8 text-center text-sm text-text-muted">
          {emptyMessage}
        </div>
      ) : (
        <div className="flex min-h-0 flex-1 flex-col gap-3 xl:flex-row">
          <div className="min-h-0 overflow-auto xl:w-80 xl:shrink-0">
            <div className="space-y-1">
              {tabs.map((tab) => {
                const isSelected = tab.id === selectedTabId;

                return (
                  <div
                    key={tab.id}
                    role="button"
                    tabIndex={0}
                    onClick={() => setSelectedTabId(tab.id)}
                    onKeyDown={(event) => {
                      if (event.key === "Enter" || event.key === " ") {
                        event.preventDefault();
                        setSelectedTabId(tab.id);
                      }
                    }}
                    className={`w-full rounded-xl border text-left transition ${
                      isSelected
                        ? "border-primary bg-primary/10"
                        : "border-border-subtle bg-white/2 hover:border-border-default hover:bg-white/3"
                    }`}
                  >
                    <TabItem tab={tab} />
                  </div>
                );
              })}
            </div>
          </div>

          <SelectedTabPanel selectedTab={selectedTab} instanceId={instanceId} />
        </div>
      )}
    </div>
  );
}
