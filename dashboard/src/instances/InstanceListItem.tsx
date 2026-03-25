import type { Instance } from "../generated/types";

interface Props {
  instance: Instance;
  tabCount: number;
  memoryMB?: number;
  selected: boolean;
  autoRestart?: boolean;
  onClick: () => void;
  onStop?: () => void;
  onOpenProfile?: () => void;
}

export default function InstanceListItem({
  instance,
  tabCount,
  memoryMB,
  selected,
  autoRestart = false,
  onClick,
  onStop,
  onOpenProfile,
}: Props) {
  const statusColor =
    instance.status === "running"
      ? "bg-success"
      : instance.status === "error"
        ? "bg-destructive"
        : "bg-text-muted";

  const stopLabel = autoRestart ? "Restart" : "Stop";
  const stopStyle = autoRestart
    ? "rounded bg-warning/10 px-2 py-0.5 text-[10px] font-medium uppercase text-warning transition-colors hover:bg-warning/20"
    : "rounded bg-destructive/10 px-2 py-0.5 text-[10px] font-medium uppercase text-destructive transition-colors hover:bg-destructive/20";

  return (
    <button
      onClick={onClick}
      className={`mb-2 flex w-full flex-col gap-1 px-3 py-2.5 text-left ${
        selected
          ? "dashboard-panel dashboard-panel-selected border-primary"
          : "dashboard-panel dashboard-panel-hover"
      }`}
    >
      <div className="flex w-full items-center gap-2">
        <div className={`h-2 w-2 shrink-0 rounded-full ${statusColor}`} />
        <div className="min-w-0 flex-1">
          <h3 className="truncate text-sm font-medium text-text-primary">
            {instance.profileName}
          </h3>
          <div className="dashboard-mono text-xs text-text-muted">
            :{instance.port} · {tabCount} tabs
            {memoryMB !== undefined && ` · ${memoryMB.toFixed(0)}MB`}
          </div>
        </div>
      </div>
      {selected && (
        <div className="flex gap-1 pl-4 pt-1">
          {onOpenProfile && (
            <span
              role="button"
              aria-label="Open Profile"
              tabIndex={0}
              onClick={(e) => {
                e.stopPropagation();
                onOpenProfile();
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.stopPropagation();
                  onOpenProfile();
                }
              }}
              className="rounded bg-bg-elevated px-2 py-0.5 text-[10px] font-medium uppercase text-text-muted transition-colors hover:bg-border-subtle hover:text-text-primary"
            >
              Open Profile
            </span>
          )}
          {onStop && instance.status === "running" && (
            <span
              role="button"
              tabIndex={0}
              onClick={(e) => {
                e.stopPropagation();
                onStop();
              }}
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  e.stopPropagation();
                  onStop();
                }
              }}
              className={stopStyle}
            >
              {stopLabel}
            </span>
          )}
        </div>
      )}
    </button>
  );
}
