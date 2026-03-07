import { useState } from "react";
import type { InstanceTab } from "../../generated/types";

interface Props {
  tab: InstanceTab;
  compact?: boolean;
}

export default function TabItem({ tab, compact }: Props) {
  const [copied, setCopied] = useState(false);

  const handleCopy = (e: React.MouseEvent) => {
    e.stopPropagation();
    navigator.clipboard.writeText(tab.id);
    setCopied(true);
    setTimeout(() => setCopied(false), 2000);
  };

  const shortId = tab.id.split("_").pop()?.substring(0, 8) || tab.id;

  const idBadge = (
    <button
      onClick={handleCopy}
      title={`Click to copy full ID: ${tab.id}`}
      className="group flex shrink-0 items-center gap-1.5 rounded bg-bg-elevated px-1.5 py-0.5 text-[10px] font-mono text-text-muted transition-colors hover:bg-border-subtle hover:text-text-primary focus:outline-none"
    >
      <span>{shortId}</span>
      <span className="text-[8px] opacity-0 transition-opacity group-hover:opacity-100">
        {copied ? "✅" : "📋"}
      </span>
    </button>
  );

  if (compact) {
    return (
      <div className="rounded-sm border border-border-subtle/70 bg-white/[0.02] px-3 py-2">
        <div className="flex items-center gap-2 overflow-hidden">
          {idBadge}
          <div className="text-text-dim">/</div>
          <div className="truncate text-sm font-medium text-text-primary">
            {tab.title || "Untitled"}
          </div>
        </div>
        <div className="mt-0.5 truncate text-[11px] text-text-muted opacity-70">
          {tab.url}
        </div>
      </div>
    );
  }

  return (
    <div className="rounded-md border border-border-subtle/80 bg-white/[0.02] px-3 py-2.5 transition-colors hover:border-border-default hover:bg-white/[0.03]">
      <div className="flex items-center gap-2 overflow-hidden">
        {idBadge}
        <div className="text-text-dim">/</div>
        <div className="truncate text-sm font-medium text-text-primary">
          {tab.title || "Untitled"}
        </div>
      </div>
      <div className="mt-1 truncate text-xs text-text-muted opacity-80">
        {tab.url}
      </div>
    </div>
  );
}
