import type { ActivityFilters } from "./types";

interface ActiveFilterBarProps {
  filters: ActivityFilters;
  hideAgentFilter?: boolean;
  hideSessionFilter?: boolean;
  onClear: () => void;
}

export default function ActiveFilterBar({
  filters,
  hideAgentFilter = false,
  hideSessionFilter = false,
  onClear,
}: ActiveFilterBarProps) {
  const activeFilters = [
    !hideAgentFilter && filters.agentId ? `agent:${filters.agentId}` : "",
    filters.profileName ? `profile:${filters.profileName}` : "",
    filters.instanceId ? `instance:${filters.instanceId}` : "",
    filters.tabId ? `tab:${filters.tabId}` : "",
    filters.action ? `action:${filters.action}` : "",
    !hideSessionFilter && filters.sessionId
      ? `session:${filters.sessionId}`
      : "",
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
