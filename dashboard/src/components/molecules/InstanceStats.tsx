import type {
  Instance,
  InstanceMetrics,
  InstanceTab,
} from "../../generated/types";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";

interface Props {
  instance?: Instance | null;
  metrics?: InstanceMetrics | null;
  tabs: InstanceTab[];
}

function StatItem({
  label,
  value,
  sub,
}: {
  label: string;
  value: string;
  sub?: string;
}) {
  return (
    <div className="flex flex-col gap-0.5">
      <span className="text-[10px] font-medium uppercase tracking-wider text-text-muted">
        {label}
      </span>
      <span className="text-sm font-semibold text-text-primary">{value}</span>
      {sub && <span className="text-[10px] text-text-muted">{sub}</span>}
    </div>
  );
}

function StatGroup({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3">
      <span className="text-[10px] font-semibold uppercase tracking-[0.1em] text-text-muted/70">
        {title}
      </span>
      <div className="flex gap-6">{children}</div>
    </div>
  );
}

function fmt(n: number, decimals = 0): string {
  if (!Number.isFinite(n)) return "0";
  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits: decimals,
  }).format(n);
}

function formatUptime(startTime: string, t: TFunction): string {
  const ms = Date.now() - new Date(startTime).getTime();
  if (ms < 0) return t("instanceStats.justNow");
  const secs = Math.floor(ms / 1000);
  if (secs < 60) return `${secs}s`;
  const mins = Math.floor(secs / 60);
  if (mins < 60) return `${mins}m`;
  const hrs = Math.floor(mins / 60);
  const remainMins = mins % 60;
  if (hrs < 24) return `${hrs}h ${remainMins}m`;
  const days = Math.floor(hrs / 24);
  return `${days}d ${hrs % 24}h`;
}

function formatCrashTime(time: string): string {
  const at = new Date(time);
  if (Number.isNaN(at.getTime())) return time;
  return at.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
}

function countUniqueDomains(tabs: InstanceTab[]): number {
  const domains = new Set<string>();
  for (const tab of tabs) {
    try {
      domains.add(new URL(tab.url).hostname);
    } catch {
      // skip invalid URLs
    }
  }
  return domains.size;
}

export default function InstanceStats({ instance, metrics, tabs }: Props) {
  const { t } = useTranslation();
  const uniqueDomains = countUniqueDomains(tabs);

  return (
    <div className="grid grid-cols-2 gap-y-4 border-t border-border-subtle px-4 py-4">
      <StatGroup title={t("instanceStats.groups.instance")}>
        {instance && (
          <>
            <StatItem
              label={t("instanceStats.status")}
              value={instance.status}
            />
            <StatItem
              label={t("instanceStats.uptime")}
              value={formatUptime(instance.startTime, t)}
            />
            <StatItem label={t("instanceStats.port")} value={instance.port} />
            {instance.crashes && instance.crashes.total > 0 && (
              <StatItem
                label={t("instanceStats.crashes")}
                value={fmt(instance.crashes.total)}
                sub={
                  instance.crashes.recent.length > 0
                    ? t("instanceStats.crashSummary", {
                        reason:
                          instance.crashes.recent[
                            instance.crashes.recent.length - 1
                          ].reason,
                        time: formatCrashTime(
                          instance.crashes.recent[
                            instance.crashes.recent.length - 1
                          ].time,
                        ),
                      })
                    : t("instanceStats.tabsLost")
                }
              />
            )}
          </>
        )}
      </StatGroup>

      <StatGroup title={t("instanceStats.groups.browsing")}>
        <StatItem label={t("instanceStats.tabs")} value={fmt(tabs.length)} />
        <StatItem
          label={t("instanceStats.domains")}
          value={fmt(uniqueDomains)}
        />
      </StatGroup>

      {metrics && (
        <StatGroup title={t("instanceStats.groups.resources")}>
          <StatItem
            label={t("instanceStats.memory")}
            value={`${fmt(metrics.memoryMB, 1)} MB`}
            sub={t("instanceStats.memorySub")}
          />
          <StatItem
            label={t("instanceStats.renderers")}
            value={fmt(metrics.renderers)}
          />
        </StatGroup>
      )}

      {metrics && (metrics.page || metrics.unreadableTargets > 0) && (
        <StatGroup title={t("instanceStats.groups.pages")}>
          {metrics.page && (
            <>
              <StatItem
                label={t("instanceStats.jsHeap")}
                value={`${fmt(metrics.page.jsHeapUsedMB, 1)} / ${fmt(metrics.page.jsHeapTotalMB, 1)} MB`}
                sub={t("instanceStats.jsHeapSub", {
                  count: metrics.page.targets,
                })}
              />
              <StatItem
                label={t("instanceStats.domNodes")}
                value={fmt(metrics.page.nodes)}
              />
              <StatItem
                label={t("instanceStats.listeners")}
                value={fmt(metrics.page.jsEventListeners)}
              />
              <StatItem
                label={t("instanceStats.frames")}
                value={fmt(metrics.page.frames)}
                sub={t("instanceStats.documentsSub", {
                  count: metrics.page.documents,
                })}
              />
            </>
          )}
          {metrics.unreadableTargets > 0 && (
            <StatItem
              label={t("instanceStats.unreadable")}
              value={fmt(metrics.unreadableTargets)}
              sub={t("instanceStats.unreadableSub")}
            />
          )}
        </StatGroup>
      )}
    </div>
  );
}
