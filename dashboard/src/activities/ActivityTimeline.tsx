import { Badge, EmptyState } from "../components/atoms";
import type { DashboardActivityEvent, ActivityFilters } from "./types";
import {
  activityMethodVariant,
  activityStatusVariant,
  formatActivityTimestamp,
} from "./helpers";

interface Props {
  events: DashboardActivityEvent[];
  loading: boolean;
  error: string;
  summary: string;
  onFilterChange: (key: keyof ActivityFilters, value: string) => void;
}

function FilterChip({
  label,
  onClick,
  className = "",
}: {
  label: string;
  onClick: () => void;
  className?: string;
}) {
  return (
    <button
      type="button"
      className={`rounded-sm border border-border-subtle bg-white/3 px-1.5 py-0.5 text-[0.62rem] font-semibold tracking-[0.08em] text-text-secondary transition-all hover:border-primary/30 hover:bg-primary/10 hover:text-primary ${className}`}
      onClick={onClick}
      title={label}
    >
      <span className="block truncate">{label}</span>
    </button>
  );
}

export default function ActivityTimeline({
  events,
  loading,
  error,
  summary,
  onFilterChange,
}: Props) {
  return (
    <section className="dashboard-panel flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="flex items-center justify-between border-b border-border-subtle px-4 py-3">
        <div>
          <div className="dashboard-section-label mb-1">Timeline</div>
          <h2 className="text-sm font-semibold text-text-secondary">
            Recent events
          </h2>
        </div>
        <div className="dashboard-mono text-[0.72rem] text-text-muted">
          {summary}
        </div>
      </div>

      {error && (
        <div className="border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-xs text-destructive">
          {error}
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-auto">
        {!loading && events.length === 0 ? (
          <EmptyState
            icon="📜"
            title="No matching activity"
            description="Adjust the filters or generate some traffic from the CLI, MCP, or dashboard."
          />
        ) : (
          <div className="divide-y divide-border-subtle/70">
            {events.map((event, index) => (
              <div
                key={`${event.requestId || event.timestamp}-${index}`}
                className="px-4 py-2 transition-colors hover:bg-white/2"
              >
                <div className="flex flex-wrap items-center gap-1.5">
                  <span className="dashboard-mono text-[0.68rem] text-text-muted">
                    {formatActivityTimestamp(event.timestamp)}
                  </span>
                  <Badge variant={activityMethodVariant(event.method)}>
                    {event.method}
                  </Badge>
                  <Badge variant={activityStatusVariant(event.status)}>
                    {event.status}
                  </Badge>
                  {event.source && <Badge>{event.source}</Badge>}
                  {event.action && (
                    <Badge variant="warning">{event.action}</Badge>
                  )}
                  {event.engine && <Badge variant="info">{event.engine}</Badge>}
                  <span className="ml-auto dashboard-mono text-[0.68rem] text-text-muted">
                    {event.durationMs} ms
                  </span>
                </div>

                <div className="mt-1 grid grid-cols-1 gap-1.5 xl:grid-cols-[minmax(0,1fr)_20rem] xl:items-start">
                  <div className="min-w-0">
                    <div className="dashboard-mono break-all text-[0.82rem] text-text-primary">
                      {event.path}
                    </div>
                    {event.tabId && (
                      <div className="mt-1">
                        <FilterChip
                          label={`tab:${event.tabId}`}
                          className="max-w-full xl:max-w-[18rem]"
                          onClick={() =>
                            onFilterChange("tabId", event.tabId || "")
                          }
                        />
                      </div>
                    )}
                    {event.url && (
                      <div className="dashboard-mono mt-0.5 break-all text-[0.68rem] text-text-muted">
                        {event.url}
                      </div>
                    )}
                  </div>
                  <div className="dashboard-mono flex flex-wrap gap-1 text-[0.68rem] text-text-muted xl:justify-end">
                    {event.agentId && (
                      <FilterChip
                        label={`agent:${event.agentId}`}
                        onClick={() =>
                          onFilterChange("agentId", event.agentId || "")
                        }
                      />
                    )}
                    {event.profileName && (
                      <FilterChip
                        label={`profile:${event.profileName}`}
                        onClick={() =>
                          onFilterChange("profileName", event.profileName || "")
                        }
                      />
                    )}
                  </div>
                </div>

                <div className="mt-1 flex flex-wrap items-center gap-x-3 gap-y-1 text-[0.68rem] text-text-muted">
                  {event.requestId && (
                    <span className="dashboard-mono">{event.requestId}</span>
                  )}
                  {event.ref && (
                    <span className="dashboard-mono">ref:{event.ref}</span>
                  )}
                  {event.actorId && (
                    <span className="dashboard-mono">
                      actor:{event.actorId}
                    </span>
                  )}
                  {event.remoteAddr && (
                    <span className="dashboard-mono">{event.remoteAddr}</span>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>
    </section>
  );
}
