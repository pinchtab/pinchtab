import { EmptyState } from "../components/atoms";
import type { DashboardActivityEvent, ActivityFilters } from "./types";
import ActivityItemLine from "./ActivityItemLine";

interface Props {
  events: DashboardActivityEvent[];
  loading: boolean;
  error: string;
  summary: string;
  embedded?: boolean;
  showTab?: boolean;
  copyTabId?: boolean;
  onFilterChange: (key: keyof ActivityFilters, value: string) => void;
}

import { useTranslation } from "react-i18next";

export default function ActivityTimeline({
  events,
  loading,
  error,
  summary,
  embedded = false,
  showTab = true,
  copyTabId = false,
  onFilterChange,
}: Props) {
  const { t } = useTranslation();

  return (
    <section
      className={`flex min-h-0 flex-1 flex-col overflow-hidden ${embedded ? "" : "dashboard-panel"}`}
    >
      {!embedded && (
        <div className="flex items-center justify-between border-b border-border-subtle px-4 py-3">
          <div>
            <div className="dashboard-section-label mb-1">
              {t("activityTimeline.timeline")}
            </div>
            <h2 className="text-sm font-semibold text-text-secondary">
              {t("activityTimeline.recentEvents")}
            </h2>
          </div>
          <div className="dashboard-mono text-[0.72rem] text-text-muted">
            {summary}
          </div>
        </div>
      )}

      {error && (
        <div className="border-b border-destructive/30 bg-destructive/10 px-4 py-2 text-xs text-destructive">
          {error}
        </div>
      )}

      <div className="min-h-0 flex-1 overflow-auto">
        {!loading && events.length === 0 ? (
          <EmptyState
            icon="📜"
            title={t("activityTimeline.noMatching")}
            description={t("activityTimeline.adjustFilters")}
          />
        ) : (
          <div className="divide-y divide-border-subtle/70">
            {events.map((event, index) => (
              <ActivityItemLine
                key={`${event.requestId || event.timestamp}-${index}`}
                event={event}
                showTab={showTab}
                copyTabId={copyTabId}
                onFilterChange={onFilterChange}
              />
            ))}
          </div>
        )}
      </div>
    </section>
  );
}
