import type { InstanceTab } from "../generated/types";
import IdBadge from "../components/molecules/IdBadge";

interface Props {
  tab: InstanceTab;
}

export default function SelectedTabTitle({ tab }: Props) {
  return (
    <div className="flex min-w-0 items-center gap-3">
      <div className="flex min-w-0 flex-col gap-0.5 text-right">
        <div className="flex items-center justify-end gap-1.5">
          <h3 className="truncate text-xs font-medium text-text-secondary">
            {tab.title || "Untitled"}
          </h3>
          <IdBadge id={tab.id} variant="compact" />
        </div>
        <div className="truncate text-[10px] text-text-muted">{tab.url}</div>
      </div>
    </div>
  );
}
