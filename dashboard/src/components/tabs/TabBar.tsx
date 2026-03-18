import type { InstanceTab } from "../../generated/types";
import * as api from "../../services/api";

interface Props {
  tabs: InstanceTab[];
  selectedTabId: string | null;
  onSelect: (id: string) => void;
  onTabClosed?: () => void;
}

export default function TabBar({
  tabs,
  selectedTabId,
  onSelect,
  onTabClosed,
}: Props) {
  const handleClose = async (e: React.MouseEvent, tabId: string) => {
    e.stopPropagation();
    try {
      await api.closeTab(tabId);
      onTabClosed?.();
    } catch (err) {
      console.error("Failed to close tab", err);
    }
  };
  if (tabs.length === 0) return null;

  return (
    <div className="flex min-h-0 items-end gap-px overflow-x-auto border-b border-border-subtle bg-black/10 px-1 pt-1">
      {tabs.map((tab) => {
        const isSelected = tab.id === selectedTabId;
        const title = tab.title || "Untitled";
        const shortId = tab.id.substring(0, 8);

        return (
          <button
            key={tab.id}
            onClick={() => onSelect(tab.id)}
            title={`${title}\n${tab.url}\n${tab.id}`}
            className={`group relative flex max-w-52 min-w-0 items-center gap-1 rounded-t-md pl-3 pr-1.5 py-1.5 text-left transition-colors ${
              isSelected
                ? "bg-bg-surface text-text-primary border-x border-t border-border-subtle"
                : "text-text-muted hover:bg-white/5 hover:text-text-secondary"
            }`}
          >
            <span className="truncate text-xs font-medium">{title}</span>
            <span
              className={`shrink-0 font-mono text-[9px] ${isSelected ? "text-text-muted" : "text-text-muted/50"}`}
            >
              {shortId}
            </span>
            <span
              role="button"
              tabIndex={0}
              onClick={(e) => handleClose(e, tab.id)}
              onKeyDown={(e) => {
                if (e.key === "Enter")
                  handleClose(e as unknown as React.MouseEvent, tab.id);
              }}
              className="ml-0.5 shrink-0 rounded p-0.5 text-[10px] leading-none text-text-muted/40 opacity-0 transition-all hover:bg-white/10 hover:text-text-primary group-hover:opacity-100"
            >
              ✕
            </span>
          </button>
        );
      })}
    </div>
  );
}
