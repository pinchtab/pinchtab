import type {
  Instance,
  InstanceMetrics,
  InstanceTab,
} from "../../generated/types";

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

function formatUptime(startTime: string): string {
  const ms = Date.now() - new Date(startTime).getTime();
  if (ms < 0) return "just now";
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
  const heapPct =
    metrics && metrics.jsHeapTotalMB > 0
      ? (metrics.jsHeapUsedMB / metrics.jsHeapTotalMB) * 100
      : null;

  const uniqueDomains = countUniqueDomains(tabs);

  return (
    <div className="grid grid-cols-2 gap-y-4 border-t border-border-subtle px-4 py-4">
      <StatGroup title="Instance">
        {instance && (
          <>
            <StatItem label="Status" value={instance.status} />
            <StatItem label="Uptime" value={formatUptime(instance.startTime)} />
            <StatItem label="Port" value={instance.port} />
          </>
        )}
      </StatGroup>

      <StatGroup title="Browsing">
        <StatItem label="Tabs" value={fmt(tabs.length)} />
        <StatItem label="Domains" value={fmt(uniqueDomains)} />
        {metrics && (
          <>
            <StatItem label="Documents" value={fmt(metrics.documents)} />
            <StatItem label="Frames" value={fmt(metrics.frames)} />
          </>
        )}
      </StatGroup>

      {metrics && (
        <StatGroup title="Resources">
          <StatItem
            label="Heap"
            value={`${fmt(metrics.jsHeapUsedMB, 1)} MB`}
            sub={
              heapPct !== null
                ? `${fmt(heapPct, 0)}% of ${fmt(metrics.jsHeapTotalMB, 1)} MB`
                : undefined
            }
          />
          <StatItem label="DOM Nodes" value={fmt(metrics.nodes)} />
          <StatItem label="Listeners" value={fmt(metrics.listeners)} />
        </StatGroup>
      )}
    </div>
  );
}
