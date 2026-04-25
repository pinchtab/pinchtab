import { useLayoutEffect, useRef } from "react";
import { EmptyState } from "../components/atoms";
import type { Session } from "../services/api";
import ActivityItemLine from "./ActivityItemLine";
import { isHandoffEvent } from "./handoffState";
import type { ActivityFilters, DashboardActivityEvent } from "./types";

interface AgentStreamPanelProps {
  events: DashboardActivityEvent[];
  sessions?: Session[];
  error: string;
  loading: boolean;
  copyTabId?: boolean;
  handoffTabs?: Set<string>;
  activeSessionId?: string;
  onFilterChange: (key: keyof ActivityFilters, value: string) => void;
}

export default function AgentStreamPanel({
  events,
  sessions = [],
  error,
  loading,
  copyTabId = false,
  handoffTabs,
  activeSessionId,
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
            {(() => {
              // Find the most recent handoff event per tab (by timestamp desc)
              const mostRecentHandoffByTab = new Map<string, string>();
              for (let i = events.length - 1; i >= 0; i--) {
                const ev = events[i];
                if (
                  isHandoffEvent(ev) &&
                  ev.tabId &&
                  handoffTabs?.has(ev.tabId) &&
                  !mostRecentHandoffByTab.has(ev.tabId)
                ) {
                  mostRecentHandoffByTab.set(
                    ev.tabId,
                    ev.requestId || ev.timestamp,
                  );
                }
              }

              return events.map((event, index) => {
                const suppressSessionLabel =
                  Boolean(activeSessionId) &&
                  event.sessionId === activeSessionId;
                // Only show badge on the MOST RECENT handoff event per tab
                const eventKey = event.requestId || event.timestamp;
                const inHandoff =
                  isHandoffEvent(event) &&
                  Boolean(event.tabId && handoffTabs?.has(event.tabId)) &&
                  mostRecentHandoffByTab.get(event.tabId!) === eventKey;
                return (
                  <ActivityItemLine
                    key={`${eventKey}-${index}`}
                    event={event}
                    showTab
                    copyTabId={copyTabId}
                    sessionLabel={
                      suppressSessionLabel
                        ? undefined
                        : event.sessionId
                          ? sessionLabels.get(event.sessionId)
                          : "anonymous"
                    }
                    inHandoff={inHandoff}
                    onFilterChange={onFilterChange}
                  />
                );
              });
            })()}
          </div>
        )}
      </div>
    </section>
  );
}
