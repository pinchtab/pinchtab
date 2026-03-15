import type { InstanceTab } from "../../generated/types";

interface Props {
  tab: InstanceTab;
}

export default function TabItem({ tab }: Props) {
  return (
    <div className="rounded-md border border-border-subtle/80 bg-white/2 px-3 py-2.5 transition-colors hover:border-border-default hover:bg-white/[0.03]">
      <div className="truncate text-sm font-medium text-text-primary">
        {tab.title || "Untitled"}
      </div>
      <div className="mt-1 line-clamp-2 text-xs text-text-muted opacity-80 break-all">
        {tab.url}
      </div>
    </div>
  );
}
