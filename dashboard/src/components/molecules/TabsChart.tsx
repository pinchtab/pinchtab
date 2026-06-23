import { useMemo } from "react";
import {
  AreaChart,
  Area,
  XAxis,
  YAxis,
  Tooltip,
  CartesianGrid,
  ResponsiveContainer,
} from "recharts";
import type { TooltipContentProps, TooltipProps } from "recharts";
import type {
  TabDataPoint,
  MemoryDataPoint,
  ServerDataPoint,
} from "../../stores/useAppStore";

interface Props {
  data: TabDataPoint[];
  memoryData?: MemoryDataPoint[];
  serverData?: ServerDataPoint[];
  instances: { id: string; profileName: string }[];
  selectedInstanceId: string | null;
  onSelectInstance: (id: string) => void;
}

const COLORS = [
  "#6366f1",
  "#8b5cf6",
  "#06b6d4",
  "#10b981",
  "#f59e0b",
  "#ef4444",
  "#ec4899",
  "#14b8a6",
];

function formatTime(timestamp: number): string {
  return new Date(timestamp).toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatMetricValue(value: number, maximumFractionDigits = 0): string {
  if (!Number.isFinite(value)) return "0";
  return new Intl.NumberFormat("en-US", { maximumFractionDigits }).format(
    value,
  );
}

type SeriesKind = "tabs" | "mem" | "heap";

interface SeriesDescriptor {
  dataKey: string;
  instanceId: string;
  kind: SeriesKind;
  displayName: string;
  suffix: string;
  color: string;
}

const HEAP_COLOR = "#94a3b8";

function formatSeriesValue(kind: SeriesKind, value: number): string {
  return kind === "tabs"
    ? formatMetricValue(value)
    : `${formatMetricValue(value, 1)}MB`;
}

/* ── Glassmorphism Tooltip ── */
function GlassTooltip({
  active,
  payload,
  label,
  seriesByKey,
}: {
  active?: TooltipContentProps<number, string>["active"];
  payload?: TooltipContentProps<number, string>["payload"];
  label?: TooltipContentProps<number, string>["label"];
  seriesByKey: Map<string, SeriesDescriptor>;
}) {
  if (!active || !payload?.length) return null;

  return (
    <div
      style={{
        background: "rgba(15, 17, 23, 0.75)",
        backdropFilter: "blur(16px)",
        WebkitBackdropFilter: "blur(16px)",
        border: "1px solid rgba(255,255,255,0.12)",
        borderRadius: "14px",
        padding: "10px 14px",
        boxShadow:
          "0 20px 40px rgba(0,0,0,0.4), inset 0 1px 0 rgba(255,255,255,0.06)",
        minWidth: "140px",
      }}
    >
      <div
        style={{
          fontSize: "10px",
          color: "#94a3b8",
          marginBottom: "6px",
          fontWeight: 600,
          letterSpacing: "0.05em",
          textTransform: "uppercase" as const,
        }}
      >
        {formatTime(label as number)}
      </div>
      {payload.map((entry) => {
        const nameStr = String(entry.dataKey);
        const s = seriesByKey.get(nameStr);
        const displayName = s?.displayName ?? nameStr;
        const suffix = s?.suffix ?? " (tabs)";
        const formattedVal = formatSeriesValue(
          s?.kind ?? "tabs",
          Number(entry.value),
        );

        return (
          <div
            key={String(entry.dataKey)}
            style={{
              display: "flex",
              alignItems: "center",
              gap: "8px",
              padding: "2px 0",
              fontSize: "12px",
            }}
          >
            <span
              style={{
                width: "8px",
                height: "8px",
                borderRadius: "50%",
                background: entry.color,
                boxShadow: `0 0 6px ${entry.color}80`,
                flexShrink: 0,
              }}
            />
            <span style={{ color: "#cbd5e1", flex: 1 }}>
              {displayName}
              {suffix}
            </span>
            <span
              style={{
                color: "#f1f5f9",
                fontWeight: 600,
                fontFamily: "var(--font-mono, monospace)",
              }}
            >
              {formattedVal}
            </span>
          </div>
        );
      })}
    </div>
  );
}

/* ── Animated Cursor ── */
function AnimatedCursor({
  points,
  height,
}: {
  points?: Array<{ x: number; y: number }>;
  width?: number;
  height?: number;
}) {
  if (!points?.length || !height) return null;
  const x = points[0].x;
  return (
    <line
      x1={x}
      y1={0}
      x2={x}
      y2={height}
      stroke="rgba(99,102,241,0.4)"
      strokeWidth={1}
      strokeDasharray="4 3"
      style={{ transition: "x1 80ms ease, x2 80ms ease" }}
    />
  );
}

/* ── Loading Dots ── */
function LoadingDots({ text }: { text: string }) {
  return (
    <div className="flex h-56 flex-col items-center justify-center gap-3">
      <div className="flex gap-1.5">
        {[0, 1, 2].map((i) => (
          <span
            key={i}
            className="inline-block h-2 w-2 rounded-full bg-indigo-500/70"
            style={{
              animation: "pulse 1.4s ease-in-out infinite",
              animationDelay: `${i * 0.2}s`,
            }}
          />
        ))}
      </div>
      <span className="text-sm text-text-muted">{text}</span>
    </div>
  );
}

/* ── Main Component ── */
export default function TabsChart({
  data,
  memoryData,
  serverData,
  instances,
  selectedInstanceId,
  onSelectInstance,
}: Props) {
  const instanceColors = useMemo(() => {
    const colors: Record<string, string> = {};
    instances.forEach((inst, i) => {
      colors[inst.id] = COLORS[i % COLORS.length];
    });
    return colors;
  }, [instances]);

  const series = useMemo<SeriesDescriptor[]>(() => {
    const list: SeriesDescriptor[] = [];
    instances.forEach((inst) => {
      const color = instanceColors[inst.id] || COLORS[0];
      const displayName = inst.profileName || inst.id;
      list.push({
        dataKey: inst.id,
        instanceId: inst.id,
        kind: "tabs",
        displayName,
        suffix: " (tabs)",
        color,
      });
      list.push({
        dataKey: `${inst.id}_mem`,
        instanceId: inst.id,
        kind: "mem",
        displayName,
        suffix: " (mem)",
        color,
      });
    });
    list.push({
      dataKey: "goHeapMB",
      instanceId: "",
      kind: "heap",
      displayName: "Server Heap",
      suffix: "",
      color: HEAP_COLOR,
    });
    return list;
  }, [instances, instanceColors]);

  const seriesByKey = useMemo(
    () => new Map(series.map((s) => [s.dataKey, s])),
    [series],
  );

  const mergedData = useMemo(() => {
    // Build rows from the union of all three sources' timestamps so memory- or
    // server-only samples (timestamps absent from tab data) still appear on the
    // timeline instead of being dropped.
    const byTime = new Map<number, Record<string, number>>();
    const rowFor = (ts: number) => {
      let row = byTime.get(ts);
      if (!row) {
        row = { timestamp: ts };
        byTime.set(ts, row);
      }
      return row;
    };

    for (const d of data) {
      const row = rowFor(d.timestamp);
      for (const [key, val] of Object.entries(d)) {
        if (key !== "timestamp") row[key] = val as number;
      }
    }
    for (const m of memoryData || []) {
      const row = rowFor(m.timestamp);
      for (const [key, val] of Object.entries(m)) {
        if (key !== "timestamp") row[`${key}_mem`] = val as number;
      }
    }
    for (const s of serverData || []) {
      rowFor(s.timestamp).goHeapMB = s.goHeapMB;
    }

    return Array.from(byTime.values()).sort(
      (a, b) => a.timestamp - b.timestamp,
    );
  }, [data, memoryData, serverData]);

  // Current values for header badges
  const currentValues = useMemo(() => {
    if (mergedData.length === 0) return null;
    const latest = mergedData[mergedData.length - 1];
    const vals: Array<{ label: string; value: string; color: string }> = [];
    for (const s of series) {
      const raw = latest[s.dataKey];
      if (raw === undefined) continue;
      const num = Number(raw);
      if (s.kind === "tabs") {
        vals.push({
          label: s.displayName,
          value: `${formatMetricValue(num)} tabs`,
          color: s.color,
        });
      } else if (s.kind === "mem") {
        vals.push({
          label: `${s.displayName} mem`,
          value: `${formatMetricValue(num, 1)}MB`,
          color: s.color,
        });
      } else {
        vals.push({
          label: "Heap",
          value: `${formatMetricValue(num, 1)}MB`,
          color: s.color,
        });
      }
    }
    return vals;
  }, [mergedData, series]);

  const tooltipContent: NonNullable<TooltipProps<number, string>["content"]> = (
    props: TooltipContentProps<number, string>,
  ) => <GlassTooltip {...props} seriesByKey={seriesByKey} />;

  if (mergedData.length < 2) {
    return (
      <LoadingDots
        text={
          mergedData.length === 0
            ? "Collecting data..."
            : "Waiting for more data..."
        }
      />
    );
  }

  const hasMemory = memoryData && memoryData.length > 0;
  const hasServer = serverData && serverData.length > 0;

  return (
    <div className="overflow-hidden">
      <div className="flex items-center justify-between border-b border-border-subtle px-4 py-3">
        <div className="flex items-center gap-2.5">
          <div>
            <div className="dashboard-section-label">Monitoring</div>
            <div className="mt-1 flex items-center gap-2 text-sm font-semibold text-text-primary">
              Live telemetry
              {/* Pulse indicator */}
              <span className="relative flex h-2.5 w-2.5">
                <span
                  className="absolute inline-flex h-full w-full rounded-full bg-emerald-400 opacity-75"
                  style={{
                    animation: "ping 1.5s cubic-bezier(0,0,0.2,1) infinite",
                  }}
                />
                <span className="relative inline-flex h-2.5 w-2.5 rounded-full bg-emerald-400" />
              </span>
            </div>
          </div>
        </div>
        <div className="flex flex-wrap items-center gap-1.5">
          {/* Current value badges */}
          {currentValues?.map((cv) => (
            <span
              key={cv.label}
              className="flex items-center gap-1.5 rounded-md border border-white/[0.08] bg-white/[0.04] px-2 py-1 text-[0.65rem] font-medium text-text-secondary"
            >
              <span
                className="h-1.5 w-1.5 rounded-full"
                style={{
                  background: cv.color,
                  boxShadow: `0 0 4px ${cv.color}60`,
                }}
              />
              {cv.value}
            </span>
          ))}
          {/* Metric type badges */}
          <span className="rounded-sm border border-border-subtle bg-white/[0.03] px-2 py-1 text-[0.68rem] font-semibold uppercase tracking-[0.08em] text-text-secondary">
            Tabs
          </span>
          {hasMemory && (
            <span className="rounded-sm border border-info/35 bg-info/10 px-2 py-1 text-[0.68rem] font-semibold uppercase tracking-[0.08em] text-info">
              Memory
            </span>
          )}
          {hasServer && (
            <span className="rounded-sm border border-primary/35 bg-primary/10 px-2 py-1 text-[0.68rem] font-semibold uppercase tracking-[0.08em] text-primary">
              Heap
            </span>
          )}
        </div>
      </div>
      <div className="px-2 py-3">
        <ResponsiveContainer width="100%" height={220}>
          <AreaChart
            data={mergedData}
            margin={{
              top: 16,
              right: hasMemory || hasServer ? 50 : 16,
              bottom: 8,
              left: 8,
            }}
          >
            <defs>
              {/* Gradients for each instance */}
              {instances.map((inst) => {
                const color = instanceColors[inst.id] || COLORS[0];
                return (
                  <linearGradient
                    key={`grad-${inst.id}`}
                    id={`grad-${inst.id}`}
                    x1="0"
                    y1="0"
                    x2="0"
                    y2="1"
                  >
                    <stop offset="0%" stopColor={color} stopOpacity={0.35} />
                    <stop offset="95%" stopColor={color} stopOpacity={0.02} />
                  </linearGradient>
                );
              })}
              {/* Gradient for server heap */}
              <linearGradient id="grad-heap" x1="0" y1="0" x2="0" y2="1">
                <stop offset="0%" stopColor="#94a3b8" stopOpacity={0.2} />
                <stop offset="95%" stopColor="#94a3b8" stopOpacity={0.01} />
              </linearGradient>
            </defs>

            <CartesianGrid
              strokeDasharray="3 3"
              stroke="rgba(255,255,255,0.04)"
              vertical={false}
            />

            <XAxis
              dataKey="timestamp"
              tickFormatter={formatTime}
              stroke="#64748b"
              fontSize={11}
              tickLine={false}
              axisLine={false}
            />
            <YAxis
              yAxisId="tabs"
              stroke="#64748b"
              fontSize={11}
              allowDecimals={false}
              domain={[0, "auto"]}
              tickLine={false}
              axisLine={false}
              width={30}
              tickFormatter={(value) => formatMetricValue(Number(value))}
            />
            {(hasMemory || hasServer) && (
              <YAxis
                yAxisId="memory"
                orientation="right"
                stroke="#94a3b8"
                fontSize={11}
                allowDecimals={false}
                domain={[0, "auto"]}
                tickLine={false}
                axisLine={false}
                width={40}
                tickFormatter={(value) =>
                  `${formatMetricValue(Number(value), 1)}MB`
                }
              />
            )}

            <Tooltip content={tooltipContent} cursor={<AnimatedCursor />} />

            {/* Tab count areas (solid gradient fill) */}
            {instances.map((inst) => (
              <Area
                key={inst.id}
                yAxisId="tabs"
                type="monotone"
                dataKey={inst.id}
                name={inst.id}
                stroke={instanceColors[inst.id]}
                strokeWidth={selectedInstanceId === inst.id ? 2.5 : 1.5}
                strokeLinecap="round"
                strokeOpacity={
                  selectedInstanceId && selectedInstanceId !== inst.id ? 0.3 : 1
                }
                fill={`url(#grad-${inst.id})`}
                fillOpacity={
                  selectedInstanceId && selectedInstanceId !== inst.id
                    ? 0.15
                    : 1
                }
                dot={false}
                activeDot={{
                  r: 5,
                  strokeWidth: 2,
                  stroke: instanceColors[inst.id],
                  fill: "#0f1117",
                  onClick: () => onSelectInstance(inst.id),
                  style: {
                    cursor: "pointer",
                    filter: `drop-shadow(0 0 4px ${instanceColors[inst.id]}80)`,
                  },
                }}
                animationDuration={800}
                animationEasing="ease-in-out"
              />
            ))}

            {/* Memory areas (dashed, lighter fill) */}
            {hasMemory &&
              instances.map((inst) => (
                <Area
                  key={`${inst.id}_mem`}
                  yAxisId="memory"
                  type="monotone"
                  dataKey={`${inst.id}_mem`}
                  name={`${inst.id}_mem`}
                  stroke={instanceColors[inst.id]}
                  strokeWidth={selectedInstanceId === inst.id ? 2 : 1}
                  strokeLinecap="round"
                  strokeOpacity={
                    selectedInstanceId && selectedInstanceId !== inst.id
                      ? 0.2
                      : 0.6
                  }
                  strokeDasharray="4 2"
                  fill="transparent"
                  dot={false}
                  animationDuration={800}
                  animationEasing="ease-in-out"
                />
              ))}

            {/* Server heap area (dotted, gray) */}
            {hasServer && (
              <Area
                yAxisId="memory"
                type="monotone"
                dataKey="goHeapMB"
                name="goHeapMB"
                stroke="#94a3b8"
                strokeWidth={1.5}
                strokeLinecap="round"
                strokeDasharray="2 2"
                fill="url(#grad-heap)"
                fillOpacity={0.5}
                dot={false}
                animationDuration={800}
                animationEasing="ease-in-out"
              />
            )}
          </AreaChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
