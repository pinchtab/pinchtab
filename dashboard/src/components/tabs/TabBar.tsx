import type { InstanceTab } from "../../generated/types";

interface Props {
  tabs: InstanceTab[];
  selectedTabId: string | null;
  onSelect: (id: string) => void;
}

export default function TabBar({ tabs, selectedTabId, onSelect }: Props) {
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
            className={`group relative flex max-w-48 min-w-0 items-center gap-1.5 rounded-t-md px-3 py-1.5 text-left transition-colors ${
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
          </button>
        );
      })}
    </div>
  );
}
