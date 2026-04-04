import { useLayoutEffect, useRef } from "react";
import { EmptyState } from "../components/atoms";
import type { Session } from "../services/api";
import ActiveFilterBar from "./ActiveFilterBar";
import SessionDivider from "./SessionDivider";
import StreamRow from "./StreamRow";
import type { ActivityFilters, DashboardActivityEvent } from "./types";

interface AgentStreamPanelProps {
  filters: ActivityFilters;
  events: DashboardActivityEvent[];
  sessions?: Session[];
  summary: string;
  error: string;
  loading: boolean;
  copyTabId?: boolean;
  hideAgentFilter?: boolean;
  hideSessionFilter?: boolean;
  simplifyMeta?: boolean;
  onClearFilters: () => void;
  onFilterChange: (key: keyof ActivityFilters, value: string) => void;
}

export default function AgentStreamPanel({
  filters,
  events,
  sessions = [],
  summary,
  error,
  loading,
  copyTabId = false,
  hideAgentFilter = false,
  hideSessionFilter = false,
  simplifyMeta = false,
  onClearFilters,
  onFilterChange,
}: AgentStreamPanelProps) {
  const sessionMap = new Map(sessions.map((s) => [s.id, s]));
  const scrollContainerRef = useRef<HTMLDivElement | null>(null);
  const shouldStickToBottomRef = useRef(true);
  const previousEventCountRef = useRef(0);

  useLayoutEffect(() => {
    if (!simplifyMeta) {
      previousEventCountRef.current = events.length;
      return;
    }

    const container = scrollContainerRef.current;
    if (!container) {
      previousEventCountRef.current = events.length;
      return;
    }

    const eventCountChanged = events.length !== previousEventCountRef.current;
    previousEventCountRef.current = events.length;

    if (eventCountChanged && shouldStickToBottomRef.current) {
      container.scrollTop = container.scrollHeight;
    }
  }, [events, simplifyMeta]);

  const handleScroll = () => {
    if (!simplifyMeta) {
      return;
    }
    const container = scrollContainerRef.current;
    if (!container) {
      return;
    }
    const distanceFromBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight;
    shouldStickToBottomRef.current = distanceFromBottom <= 48;
  };

  return (
    <section className="flex min-h-0 flex-1 flex-col overflow-hidden">
      <div className="flex items-center justify-between border-b border-border-subtle bg-bg-surface px-4 py-3">
        <div></div>
        <div className="dashboard-mono text-[0.72rem] text-text-muted">
          {summary}
        </div>
      </div>

      <ActiveFilterBar
        filters={filters}
        hideAgentFilter={hideAgentFilter}
        hideSessionFilter={hideSessionFilter}
        onClear={onClearFilters}
      />

      {error && (
        <div className="border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-xs text-destructive">
          {error}
        </div>
      )}

      <div
        ref={scrollContainerRef}
        className="min-h-0 flex-1 overflow-auto"
        onScroll={handleScroll}
      >
        {!loading && events.length === 0 ? (
          <EmptyState
            icon="📡"
            title="No matching activity"
            description="Adjust the filters or generate some traffic from the CLI, MCP, or dashboard."
          />
        ) : (
          <div
            className={
              simplifyMeta ? "flex min-h-full flex-col justify-end py-2" : ""
            }
          >
            {events.map((event, index) => {
              const prevSessionId =
                index > 0 ? events[index - 1].sessionId : undefined;
              const showDivider =
                event.sessionId && event.sessionId !== prevSessionId;
              return (
                <div key={`${event.requestId || event.timestamp}-${index}`}>
                  {showDivider && (
                    <SessionDivider
                      session={sessionMap.get(event.sessionId || "")}
                      sessionId={event.sessionId || ""}
                      timestamp={event.timestamp}
                    />
                  )}
                  <StreamRow
                    event={event}
                    copyTabId={copyTabId}
                    hideAgentFilter={hideAgentFilter}
                    simplifyMeta={simplifyMeta}
                    onFilterChange={onFilterChange}
                  />
                </div>
              );
            })}
          </div>
        )}
      </div>
    </section>
  );
}
