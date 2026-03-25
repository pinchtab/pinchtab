import type { InstanceTab } from "../generated/types";
import * as api from "../services/api";

interface Props {
  tabs: InstanceTab[];
  selectedTabId: string | null;
  pinnedTabId?: string | null;
  onSelect: (id: string) => void;
  onTogglePinned?: (id: string) => void;
  onTabClosed?: () => void;
}

function PinIcon({ pinned }: { pinned: boolean }) {
  return (
    <svg
      viewBox="0 0 24 24"
      aria-hidden="true"
      className={`h-3.5 w-3.5 transition-colors ${pinned ? "opacity-100" : ""}`}
      fill="none"
      stroke="currentColor"
      strokeWidth="2"
      strokeLinecap="round"
      strokeLinejoin="round"
    >
      <path d="M15 4.5l-4 4l-4 1.5l-1.5 1.5l7 7l1.5 -1.5l1.5 -4l4 -4" />
      <path d="M9 15l-4.5 4.5" />
      <path d="M14.5 4l5.5 5.5" />
    </svg>
  );
}

export default function TabBar({
  tabs,
  selectedTabId,
  pinnedTabId,
  onSelect,
  onTogglePinned,
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
        const isPinned = tab.id === pinnedTabId;
        const title = tab.title || "Untitled";

        return (
          <div
            key={tab.id}
            className={`group relative flex max-w-52 min-w-0 items-center gap-1 rounded-t-md pl-3 pr-1.5 py-1.5 text-left transition-colors ${
              isSelected
                ? "bg-bg-surface text-text-primary border-x border-t border-border-subtle"
                : "text-text-muted hover:bg-white/5 hover:text-text-secondary"
            }`}
          >
            <button
              type="button"
              onClick={() => onSelect(tab.id)}
              title={`${title}\n${tab.url}`}
              className="min-w-0 flex-1 truncate text-left"
            >
              <span className="truncate text-xs font-medium">{title}</span>
            </button>
            {onTogglePinned && (
              <button
                type="button"
                onClick={(e) => {
                  e.stopPropagation();
                  onTogglePinned(tab.id);
                }}
                aria-label={
                  isPinned ? `Unpin ${title} and follow focus` : `Pin ${title}`
                }
                title={
                  isPinned
                    ? "Unpin and follow the focused tab again"
                    : "Pin this tab selection"
                }
                className={`shrink-0 rounded p-0.5 transition-all ${
                  isPinned
                    ? "text-text-primary opacity-100"
                    : "text-text-muted/50 opacity-0 hover:bg-white/10 hover:text-text-primary group-hover:opacity-100"
                }`}
              >
                <PinIcon pinned={isPinned} />
              </button>
            )}
            <button
              type="button"
              onClick={(e) => handleClose(e, tab.id)}
              aria-label={`Close ${title}`}
              className="ml-0.5 shrink-0 rounded p-0.5 text-[10px] leading-none text-text-muted/40 opacity-0 transition-all hover:bg-white/10 hover:text-text-primary group-hover:opacity-100"
            >
              ✕
            </button>
          </div>
        );
      })}
    </div>
  );
}
