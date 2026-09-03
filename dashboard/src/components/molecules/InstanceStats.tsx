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
  const uniqueDomains = countUniqueDomains(tabs);

  return (
    <div className="grid grid-cols-2 gap-y-4 border-t border-border-subtle px-4 py-4">
      <StatGroup title="Instance">
        {instance && (
          <>
            <StatItem label="Status" value={instance.status} />
            <StatItem label="Uptime" value={formatUptime(instance.startTime)} />
            <StatItem label="Port" value={instance.port} />
            {instance.crashes && instance.crashes.total > 0 && (
              <StatItem
                label="Crashes"
                value={fmt(instance.crashes.total)}
                sub={
                  instance.crashes.recent.length > 0
                    ? `last: ${instance.crashes.recent[instance.crashes.recent.length - 1].reason} at ${formatCrashTime(instance.crashes.recent[instance.crashes.recent.length - 1].time)} · tabs open before it were lost`
                    : "tabs open before it were lost"
                }
              />
            )}
          </>
        )}
      </StatGroup>

      <StatGroup title="Browsing">
        <StatItem label="Tabs" value={fmt(tabs.length)} />
        <StatItem label="Domains" value={fmt(uniqueDomains)} />
      </StatGroup>

      {metrics && (
        <StatGroup title="Resources">
          <StatItem
            label="Memory"
            value={`${fmt(metrics.memoryMB, 1)} MB`}
            sub="RSS across the browser process tree"
          />
          <StatItem label="Renderers" value={fmt(metrics.renderers)} />
        </StatGroup>
      )}
    </div>
  );
}
