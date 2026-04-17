import { useLayoutEffect, useRef } from "react";
import { EmptyState } from "../components/atoms";
import type { Session } from "../services/api";
import ActivityItemLine from "./ActivityItemLine";
import type { ActivityFilters, DashboardActivityEvent } from "./types";

interface AgentStreamPanelProps {
  events: DashboardActivityEvent[];
  sessions?: Session[];
  error: string;
  loading: boolean;
  copyTabId?: boolean;
  onFilterChange: (key: keyof ActivityFilters, value: string) => void;
}

export default function AgentStreamPanel({
  events,
  sessions = [],
  error,
  loading,
  copyTabId = false,
  onFilterChange,
}: AgentStreamPanelProps) {
  const sessionLabels = new Map(
    sessions
      .filter((session) => session.label?.trim())
      .map((session) => [session.id, session.label!.trim()] as const),
  );
  const scrollContainerRef = useRef<HTMLDivElement | null>(null);
  const shouldStickToBottomRef = useRef(true);
  const previousEventCountRef = useRef(0);

  useLayoutEffect(() => {
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
  }, [events]);

  const handleScroll = () => {
    const container = scrollContainerRef.current;
    if (!container) {
      return;
    }
    const distanceFromBottom =
      container.scrollHeight - container.scrollTop - container.clientHeight;
    shouldStickToBottomRef.current = distanceFromBottom <= 48;
  };

  return (
    <section className="dashboard-panel flex min-h-0 flex-1 flex-col overflow-hidden rounded-none">
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
          <div className="divide-y divide-border-subtle/70">
            {events.map((event, index) => (
              <ActivityItemLine
                key={`${event.requestId || event.timestamp}-${index}`}
                event={event}
                showTab
                copyTabId={copyTabId}
                sessionLabel={
                  event.sessionId
                    ? sessionLabels.get(event.sessionId)
                    : "anonymous"
                }
                onFilterChange={onFilterChange}
              />
            ))}
          </div>
        )}
      </div>
    </section>
  );
}
