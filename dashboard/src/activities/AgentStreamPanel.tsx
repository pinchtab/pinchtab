import { EmptyState } from "../components/atoms";
import { Badge } from "../components/atoms";
import { activityMethodVariant, activityStatusVariant } from "./helpers";
import type { ActivityFilters, DashboardActivityEvent } from "./types";

function formatTime(ts: string): string {
  return new Date(ts).toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function eventIcon(event: DashboardActivityEvent): string {
  if (event.action === "click" || event.action === "dblclick") return "👆";
  if (event.action === "type") return "⌨️";
  if (event.action === "hover") return "🖱️";
  if (event.path.includes("/navigate")) return "🧭";
  if (event.path.includes("/snapshot")) return "📸";
  if (event.path.includes("/screencast")) return "🖥️";
  return "📝";
}

function FilterPill({
  label,
  onClick,
}: {
  label: string;
  onClick: () => void;
}) {
  return (
    <button
      type="button"
      className="rounded-sm border border-border-subtle bg-white/3 px-1.5 py-0.5 text-[0.62rem] font-semibold tracking-[0.08em] text-text-secondary transition-all hover:border-primary/30 hover:bg-primary/10 hover:text-primary"
      onClick={onClick}
      title={label}
    >
      <span className="block truncate">{label}</span>
    </button>
  );
}

function ActiveFilterBar({
  filters,
  onClear,
}: {
  filters: ActivityFilters;
  onClear: () => void;
}) {
  const activeFilters = [
    filters.agentId ? `agent:${filters.agentId}` : "",
    filters.profileName ? `profile:${filters.profileName}` : "",
    filters.instanceId ? `instance:${filters.instanceId}` : "",
    filters.tabId ? `tab:${filters.tabId}` : "",
    filters.action ? `action:${filters.action}` : "",
    filters.source ? `source:${filters.source}` : "",
    filters.sessionId ? `session:${filters.sessionId}` : "",
    filters.pathPrefix ? `path:${filters.pathPrefix}` : "",
  ].filter(Boolean);

  if (activeFilters.length === 0) {
    return null;
  }

  return (
    <div className="flex flex-wrap items-center gap-2 border-b border-border-subtle px-4 py-2">
      {activeFilters.map((filter) => (
        <span
          key={filter}
          className="dashboard-mono rounded-sm border border-primary/20 bg-primary/10 px-2 py-1 text-[0.68rem] text-text-secondary"
        >
          {filter}
        </span>
      ))}
      <button
        type="button"
        className="ml-auto text-[0.68rem] font-semibold uppercase tracking-[0.12em] text-text-muted transition-colors hover:text-text-primary"
        onClick={onClear}
      >
        Clear filters
      </button>
    </div>
  );
}

function StreamRow({
  event,
  onFilterChange,
}: {
  event: DashboardActivityEvent;
  onFilterChange: (key: keyof ActivityFilters, value: string) => void;
}) {
  return (
    <div className="border-b border-border-subtle/70 px-4 py-3 text-sm transition-colors hover:bg-white/2">
      <div className="flex items-start gap-3">
        <span className="pt-0.5 text-lg">{eventIcon(event)}</span>
        <span className="dashboard-mono w-18 shrink-0 pt-0.5 text-xs text-text-muted">
          {formatTime(event.timestamp)}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={activityMethodVariant(event.method)}>
              {event.method}
            </Badge>
            <Badge variant={activityStatusVariant(event.status)}>
              {event.status}
            </Badge>
            {event.source && <Badge>{event.source}</Badge>}
            {event.action && <Badge variant="warning">{event.action}</Badge>}
            <span className="dashboard-mono min-w-0 flex-1 truncate text-text-secondary">
              {event.path}
            </span>
          </div>

          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            {event.tabId && (
              <FilterPill
                label={`tab:${event.tabId}`}
                onClick={() => onFilterChange("tabId", event.tabId || "")}
              />
            )}
            {event.profileName && (
              <FilterPill
                label={`profile:${event.profileName}`}
                onClick={() =>
                  onFilterChange("profileName", event.profileName || "")
                }
              />
            )}
            {event.instanceId && (
              <FilterPill
                label={`instance:${event.instanceId}`}
                onClick={() =>
                  onFilterChange("instanceId", event.instanceId || "")
                }
              />
            )}
            {event.url && (
              <span className="dashboard-mono min-w-0 truncate text-[0.68rem] text-text-muted">
                {event.url}
              </span>
            )}
          </div>
        </div>

        <div className="flex shrink-0 flex-col items-end gap-1.5">
          <span className="dashboard-mono text-xs text-text-muted">
            {event.durationMs}ms
          </span>
          {event.agentId && (
            <FilterPill
              label={`agent:${event.agentId}`}
              onClick={() => onFilterChange("agentId", event.agentId || "")}
            />
          )}
        </div>
      </div>
    </div>
  );
}

interface AgentStreamPanelProps {
  filters: ActivityFilters;
  events: DashboardActivityEvent[];
  summary: string;
  error: string;
  loading: boolean;
  onClearFilters: () => void;
  onFilterChange: (key: keyof ActivityFilters, value: string) => void;
}

export default function AgentStreamPanel({
  filters,
  events,
  summary,
  error,
  loading,
  onClearFilters,
  onFilterChange,
}: AgentStreamPanelProps) {
  return (
    <section className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="flex items-center justify-between border-b border-border-subtle bg-bg-surface px-4 py-3">
        <div></div>
        <div className="dashboard-mono text-[0.72rem] text-text-muted">
          {summary}
        </div>
      </div>

      <ActiveFilterBar filters={filters} onClear={onClearFilters} />

      {error && (
        <div className="border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-xs text-destructive">
          {error}
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-auto">
        {!loading && events.length === 0 ? (
          <EmptyState
            icon="📡"
            title="No matching activity"
            description="Adjust the filters or generate some traffic from the CLI, MCP, or dashboard."
          />
        ) : (
          <div>
            {events.map((event, index) => (
              <StreamRow
                key={`${event.requestId || event.timestamp}-${index}`}
                event={event}
                onFilterChange={onFilterChange}
              />
            ))}
          </div>
        )}
      </div>
    </section>
  );
}
