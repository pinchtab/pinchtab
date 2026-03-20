import { Badge } from "../components/atoms";
import type { ActivityEvent } from "../types";

interface Props {
  event: ActivityEvent;
}

const typeColors: Record<string, "default" | "success" | "info" | "warning"> = {
  navigate: "info",
  snapshot: "success",
  action: "warning",
  screenshot: "default",
  text: "default",
  progress: "info",
  other: "default",
};

const typeIcons: Record<string, string> = {
  navigate: "🧭",
  snapshot: "📸",
  action: "👆",
  screenshot: "🖼️",
  text: "🔎",
  progress: "🧠",
  other: "📝",
};

function formatTime(ts: string): string {
  return new Date(ts).toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export default function ActivityLine({ event }: Props) {
  const status =
    typeof event.details?.status === "number" ? event.details.status : null;
  const durationMs =
    typeof event.details?.durationMs === "number"
      ? event.details.durationMs
      : null;
  const progressValue =
    typeof event.progress === "number" &&
    typeof event.total === "number" &&
    event.total > 0
      ? (event.progress / event.total) * 100
      : typeof event.progress === "number"
        ? event.progress
        : undefined;
  const progressLabel =
    typeof event.progress === "number" && typeof event.total === "number"
      ? `${event.progress}/${event.total}`
      : typeof event.progress === "number"
        ? `${event.progress}%`
        : null;

  if (event.channel === "progress") {
    return (
      <div className="border-b border-border-subtle/70 px-4 py-3 text-sm transition-colors hover:bg-white/2">
        <div className="flex items-start gap-3">
          <span className="text-lg">{typeIcons.progress}</span>
          <span className="dashboard-mono w-18 shrink-0 pt-0.5 text-xs text-text-muted">
            {formatTime(event.timestamp)}
          </span>
          <div className="min-w-0 flex-1">
            <div className="flex items-center gap-2">
              <Badge variant={typeColors.progress}>PROGRESS</Badge>
              <span className="truncate text-text-primary">
                {event.message || "Agent reported progress"}
              </span>
            </div>
            {(progressLabel || progressValue !== undefined) && (
              <div className="mt-2 flex items-center gap-3">
                {progressValue !== undefined && (
                  <div className="h-2 flex-1 overflow-hidden rounded-full bg-bg-elevated">
                    <div
                      className="h-full rounded-full bg-primary transition-all"
                      style={{
                        width: `${Math.max(0, Math.min(100, progressValue))}%`,
                      }}
                    />
                  </div>
                )}
                {progressLabel && (
                  <span className="dashboard-mono shrink-0 text-xs text-text-muted">
                    {progressLabel}
                  </span>
                )}
              </div>
            )}
          </div>
          <span className="dashboard-mono shrink-0 text-xs text-text-muted">
            {event.agentId}
          </span>
        </div>
      </div>
    );
  }

  return (
    <div className="flex items-center gap-3 border-b border-border-subtle/70 px-4 py-3 text-sm transition-colors hover:bg-white/2">
      <span className="text-lg">{typeIcons[event.type] || "📝"}</span>
      <span className="dashboard-mono w-18 shrink-0 text-xs text-text-muted">
        {formatTime(event.timestamp)}
      </span>
      <Badge variant={typeColors[event.type] || "default"}>
        {event.method}
      </Badge>
      <span className="dashboard-mono min-w-0 flex-1 truncate text-text-secondary">
        {event.path}
      </span>
      {status !== null && (
        <span className="dashboard-mono shrink-0 text-xs text-text-muted">
          {status}
        </span>
      )}
      {durationMs !== null && (
        <span className="dashboard-mono shrink-0 text-xs text-text-muted">
          {durationMs}ms
        </span>
      )}
      <span className="dashboard-mono shrink-0 text-xs text-text-muted">
        {event.agentId}
      </span>
    </div>
  );
}
