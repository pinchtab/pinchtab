import { Badge } from "../atoms";
import type { Instance } from "../../generated/types";

interface Props {
  instance: Instance;
  tabCount: number;
  memoryMB?: number;
  selected: boolean;
  onClick: () => void;
}

export default function InstanceListItem({
  instance,
  tabCount,
  memoryMB,
  selected,
  onClick,
}: Props) {
  const statusColor =
    instance.status === "running"
      ? "bg-success"
      : instance.status === "error"
        ? "bg-destructive"
        : "bg-text-muted";

  const badgeVariant =
    instance.status === "running"
      ? "success"
      : instance.status === "error"
        ? "danger"
        : "default";

  return (
    <button
      onClick={onClick}
      className={`mb-2 flex w-full items-center gap-3 px-3 py-3 text-left ${
        selected
          ? "dashboard-panel dashboard-panel-selected border-primary"
          : "dashboard-panel dashboard-panel-hover"
      }`}
    >
      <div className={`h-2 w-2 rounded-full ${statusColor}`} />
      <div className="min-w-0 flex-1">
        <div className="truncate text-sm font-medium text-text-primary">
          {instance.profileName}
        </div>
        <div className="dashboard-mono text-xs text-text-muted">
          :{instance.port} · {tabCount} tabs
          {memoryMB !== undefined && ` · ${memoryMB.toFixed(0)}MB`}
        </div>
      </div>
      <Badge variant={badgeVariant}>{instance.status}</Badge>
    </button>
  );
}
