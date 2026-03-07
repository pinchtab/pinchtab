import { useMemo } from "react";
import {
  LineChart,
  Line,
  XAxis,
  YAxis,
  Tooltip,
  ResponsiveContainer,
} from "recharts";
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

// Colors for different instances
const COLORS = [
  "#60a5fa",
  "#93c5fd",
  "#38bdf8",
  "#22c55e",
  "#fbbf24",
  "#ef4444",
  "#818cf8",
  "#2dd4bf",
];

function formatTime(timestamp: number): string {
  return new Date(timestamp).toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function formatMetricValue(value: number, maximumFractionDigits = 0): string {
  if (!Number.isFinite(value)) return "0";

  return new Intl.NumberFormat("en-US", {
    maximumFractionDigits,
  }).format(value);
}

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

  // Merge tab, memory, and server data by timestamp
  const mergedData = useMemo(() => {
    const memByTime = new Map((memoryData || []).map((m) => [m.timestamp, m]));
    const serverByTime = new Map(
      (serverData || []).map((s) => [s.timestamp, s]),
    );

    // Use tab data as base, or server data if no tabs
    const baseData =
      data.length > 0
        ? data
        : (serverData || []).map((s) => ({ timestamp: s.timestamp }));

    return baseData.map((d) => {
      const merged: Record<string, number> = { timestamp: d.timestamp };

      // Add tab data if present
      for (const [key, val] of Object.entries(d)) {
        if (key !== "timestamp") {
          merged[key] = val as number;
        }
      }

      // Add memory keys with _mem suffix
      const mem = memByTime.get(d.timestamp);
      if (mem) {
        for (const [key, val] of Object.entries(mem)) {
          if (key !== "timestamp") {
            merged[`${key}_mem`] = val;
          }
        }
      }

      // Add server metrics
      const srv = serverByTime.get(d.timestamp);
      if (srv) {
        merged.goHeapMB = srv.goHeapMB;
      }

      return merged;
    });
  }, [data, memoryData, serverData]);

  // Show empty state if no data or too few points to render meaningfully
  if (mergedData.length < 2) {
    return (
      <div className="dashboard-panel flex h-56 items-center justify-center text-sm text-text-muted">
        {mergedData.length === 0
          ? "Collecting data..."
          : "Waiting for more data..."}
      </div>
    );
  }

  const hasMemory = memoryData && memoryData.length > 0;
  const hasServer = serverData && serverData.length > 0;

  return (
    <div className="dashboard-panel overflow-hidden">
      <div className="flex items-center justify-between border-b border-border-subtle px-4 py-3">
        <div>
          <div className="dashboard-section-label">Monitoring</div>
          <div className="mt-1 text-sm font-semibold text-text-primary">
            Live telemetry
          </div>
        </div>
        <div className="flex flex-wrap gap-1.5">
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
          <LineChart
            data={mergedData}
            margin={{
              top: 16,
              right: hasMemory || hasServer ? 50 : 16,
              bottom: 8,
              left: 8,
            }}
          >
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
            <Tooltip
              contentStyle={{
                background: "rgb(20 23 32 / 0.94)",
                border: "1px solid rgb(255 255 255 / 0.12)",
                borderRadius: "12px",
                boxShadow: "0 18px 36px rgb(0 0 0 / 0.28)",
                fontSize: "12px",
                color: "#e2e8f0",
              }}
              labelFormatter={(label) => formatTime(label as number)}
              formatter={(value, name) => {
                const numericValue = Number(value);
                const nameStr = String(name);
                if (nameStr === "goHeapMB") {
                  return [
                    `${formatMetricValue(numericValue, 1)}MB`,
                    "Server Heap",
                  ];
                }
                const isMemory = nameStr.endsWith("_mem");
                const instId = isMemory ? nameStr.replace("_mem", "") : nameStr;
                const inst = instances.find((i) => i.id === instId);
                const label = inst?.profileName || instId;
                return [
                  isMemory
                    ? `${formatMetricValue(numericValue, 1)}MB`
                    : formatMetricValue(numericValue),
                  isMemory ? `${label} (mem)` : `${label} (tabs)`,
                ];
              }}
            />
            {/* Tab count lines (solid) */}
            {instances.map((inst) => (
              <Line
                key={inst.id}
                yAxisId="tabs"
                type="monotone"
                dataKey={inst.id}
                name={inst.id}
                stroke={instanceColors[inst.id]}
                strokeWidth={selectedInstanceId === inst.id ? 3 : 1.5}
                strokeOpacity={
                  selectedInstanceId && selectedInstanceId !== inst.id ? 0.3 : 1
                }
                dot={false}
                activeDot={{
                  r: 4,
                  onClick: () => onSelectInstance(inst.id),
                  style: { cursor: "pointer" },
                }}
              />
            ))}
            {/* Memory lines (dashed) */}
            {hasMemory &&
              instances.map((inst) => (
                <Line
                  key={`${inst.id}_mem`}
                  yAxisId="memory"
                  type="monotone"
                  dataKey={`${inst.id}_mem`}
                  name={`${inst.id}_mem`}
                  stroke={instanceColors[inst.id]}
                  strokeWidth={selectedInstanceId === inst.id ? 2 : 1}
                  strokeOpacity={
                    selectedInstanceId && selectedInstanceId !== inst.id
                      ? 0.2
                      : 0.6
                  }
                  strokeDasharray="4 2"
                  dot={false}
                />
              ))}
            {/* Server heap line (dotted, gray) */}
            {hasServer && (
              <Line
                yAxisId="memory"
                type="monotone"
                dataKey="goHeapMB"
                name="goHeapMB"
                stroke="#94a3b8"
                strokeWidth={1.5}
                strokeDasharray="2 2"
                dot={false}
              />
            )}
          </LineChart>
        </ResponsiveContainer>
      </div>
    </div>
  );
}
