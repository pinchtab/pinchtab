import { useMemo } from "react";
import type {
  MemoryDataPoint,
  ServerDataPoint,
  TabDataPoint,
} from "../../stores/useAppStore";

interface Props {
  data: TabDataPoint[];
  memoryData?: MemoryDataPoint[];
  serverData?: ServerDataPoint[];
  instances: { id: string; profileName: string }[];
  selectedInstanceId: string | null;
  onSelectInstance: (id: string) => void;
}

interface SeriesDefinition {
  id: string;
  key: string;
  label: string;
  color: string;
  selectable: boolean;
  selected: boolean;
}

interface TrackDefinition {
  id: string;
  title: string;
  suffix: string;
  rows: number;
  series: SeriesDefinition[];
  data: Record<string, number>[];
}

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

function formatValue(value: number, suffix: string): string {
  return suffix
    ? `${formatMetricValue(value, 1)}${suffix}`
    : formatMetricValue(value);
}

function clampCellCount(value: number, maxValue: number, rows: number): number {
  if (value <= 0 || maxValue <= 0) return 0;
  return Math.max(1, Math.min(rows, Math.round((value / maxValue) * rows)));
}

function TerminalTrack({
  track,
  selectedInstanceId,
  onSelectInstance,
}: {
  track: TrackDefinition;
  selectedInstanceId: string | null;
  onSelectInstance: (id: string) => void;
}) {
  const cell = 4;
  const gap = 3;
  const rowStride = cell + gap;
  const colInset = 3;
  const topInset = 10;
  const bottomInset = 20;
  const rightInset = 8;
  const columns = track.data.length;
  const seriesCount = Math.max(track.series.length, 1);
  const groupWidth = Math.max(12, seriesCount * (cell + 2) + 8);
  const width = colInset + columns * groupWidth + rightInset;
  const height = topInset + track.rows * rowStride + bottomInset;
  const maxValue = Math.max(
    1,
    ...track.data.flatMap((point) =>
      track.series.map((series) => point[series.key] ?? 0),
    ),
  );
  const labelStep = Math.max(1, Math.ceil(columns / 6));

  return (
    <div className="rounded-md border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.55)] px-3 py-3">
      <div className="mb-3 flex items-center justify-between">
        <div className="dashboard-section-title">{track.title}</div>
        <div className="dashboard-mono text-[0.68rem] text-text-muted">
          max {formatValue(maxValue, track.suffix)}
        </div>
      </div>

      <div className="overflow-x-auto">
        <svg
          width={width}
          height={height}
          viewBox={`0 0 ${width} ${height}`}
          className="block min-w-full"
          role="img"
          aria-label={`${track.title} terminal chart`}
        >
          {track.data.map((point, pointIndex) => {
            const groupX = colInset + pointIndex * groupWidth;

            return track.series.map((series, seriesIndex) => {
              const value = point[series.key] ?? 0;
              const lit = clampCellCount(value, maxValue, track.rows);
              const x = groupX + seriesIndex * (cell + 2);
              const dimmed =
                selectedInstanceId &&
                series.selectable &&
                selectedInstanceId !== series.id;

              return (
                <g key={`${track.id}-${series.key}-${point.timestamp}`}>
                  {Array.from({ length: track.rows }).map((_, rowIndex) => {
                    const y =
                      topInset + (track.rows - 1 - rowIndex) * rowStride;
                    const active = rowIndex < lit;

                    return (
                      <rect
                        key={`${track.id}-${series.key}-${pointIndex}-${rowIndex}`}
                        x={x}
                        y={y}
                        width={cell}
                        height={cell}
                        rx={0.8}
                        fill={active ? series.color : "rgb(255 255 255 / 0.07)"}
                        opacity={active ? (dimmed ? 0.3 : 0.92) : 1}
                      >
                        <title>
                          {`${series.label} ${formatValue(value, track.suffix)} at ${formatTime(point.timestamp)}`}
                        </title>
                      </rect>
                    );
                  })}
                </g>
              );
            });
          })}

          {track.data.map(
            (point, pointIndex) =>
              (pointIndex % labelStep === 0 || pointIndex === columns - 1) && (
                <text
                  key={`${track.id}-label-${point.timestamp}`}
                  x={colInset + pointIndex * groupWidth}
                  y={height - 4}
                  fill="#64748b"
                  fontSize="10"
                  fontFamily="JetBrains Mono, monospace"
                >
                  {formatTime(point.timestamp)}
                </text>
              ),
          )}
        </svg>
      </div>

      <div className="mt-3 flex flex-wrap gap-1.5">
        {track.series.map((series) => {
          const active = selectedInstanceId === series.id;
          return (
            <button
              key={`${track.id}-${series.key}-legend`}
              type="button"
              onClick={() => series.selectable && onSelectInstance(series.id)}
              className={`inline-flex items-center gap-2 rounded-sm border px-2 py-1 text-[0.68rem] font-semibold uppercase tracking-[0.08em] transition-all ${
                series.selectable
                  ? active
                    ? "border-primary/35 bg-primary/10 text-primary"
                    : "border-border-subtle bg-white/[0.03] text-text-secondary hover:border-border-default hover:text-text-primary"
                  : "cursor-default border-border-subtle bg-white/[0.03] text-text-secondary"
              }`}
            >
              <span
                className="h-2 w-2 rounded-[2px]"
                style={{ backgroundColor: series.color }}
                aria-hidden="true"
              />
              {series.label}
            </button>
          );
        })}
      </div>
    </div>
  );
}

export default function TabsChart2({
  data,
  memoryData,
  serverData,
  instances,
  selectedInstanceId,
  onSelectInstance,
}: Props) {
  const mergedData = useMemo(() => {
    const memByTime = new Map((memoryData || []).map((m) => [m.timestamp, m]));
    const serverByTime = new Map(
      (serverData || []).map((s) => [s.timestamp, s]),
    );

    const baseData =
      data.length > 0
        ? data
        : (serverData || []).map((s) => ({ timestamp: s.timestamp }));

    return baseData.map((d) => {
      const merged: Record<string, number> = { timestamp: d.timestamp };

      for (const [key, val] of Object.entries(d)) {
        if (key !== "timestamp") {
          merged[key] = val as number;
        }
      }

      const mem = memByTime.get(d.timestamp);
      if (mem) {
        for (const [key, val] of Object.entries(mem)) {
          if (key !== "timestamp") {
            merged[`${key}_mem`] = val;
          }
        }
      }

      const srv = serverByTime.get(d.timestamp);
      if (srv) {
        merged.goHeapMB = srv.goHeapMB;
      }

      return merged;
    });
  }, [data, memoryData, serverData]);

  const hasMemory = Boolean(memoryData && memoryData.length > 0);
  const hasServer = Boolean(serverData && serverData.length > 0);

  const tracks = useMemo<TrackDefinition[]>(() => {
    const tabSeries: SeriesDefinition[] = instances.map((instance, index) => ({
      id: instance.id,
      key: instance.id,
      label: instance.profileName,
      color: COLORS[index % COLORS.length],
      selectable: true,
      selected: selectedInstanceId === instance.id,
    }));

    const memorySeries: SeriesDefinition[] = hasMemory
      ? instances.map((instance, index) => ({
          id: instance.id,
          key: `${instance.id}_mem`,
          label: instance.profileName,
          color: COLORS[index % COLORS.length],
          selectable: true,
          selected: selectedInstanceId === instance.id,
        }))
      : [];

    const nextTracks: TrackDefinition[] = [];

    if (tabSeries.length > 0) {
      nextTracks.push({
        id: "tabs",
        title: "Tabs",
        suffix: "",
        rows: 10,
        series: tabSeries,
        data: mergedData,
      });
    }

    if (memorySeries.length > 0) {
      nextTracks.push({
        id: "memory",
        title: "Memory",
        suffix: "MB",
        rows: 8,
        series: memorySeries,
        data: mergedData,
      });
    }

    if (hasServer) {
      nextTracks.push({
        id: "server",
        title: "Heap",
        suffix: "MB",
        rows: 6,
        series: [
          {
            id: "goHeapMB",
            key: "goHeapMB",
            label: "Server Heap",
            color: "#cbd5e1",
            selectable: false,
            selected: false,
          },
        ],
        data: mergedData,
      });
    }

    return nextTracks;
  }, [hasMemory, hasServer, instances, mergedData, selectedInstanceId]);

  if (mergedData.length < 2) {
    return (
      <div className="dashboard-panel flex h-56 items-center justify-center text-sm text-text-muted">
        {mergedData.length === 0
          ? "Collecting data..."
          : "Waiting for more data..."}
      </div>
    );
  }

  return (
    <div
      className="dashboard-panel overflow-hidden"
      data-testid="terminal-tabs-chart"
    >
      <div className="flex items-center justify-between border-b border-border-subtle px-4 py-3">
        <div>
          <div className="dashboard-section-label">Monitoring</div>
          <div className="mt-1 text-sm font-semibold text-text-primary">
            Terminal telemetry
          </div>
        </div>
        <div className="dashboard-mono text-[0.72rem] uppercase tracking-[0.12em] text-text-muted">
          {mergedData.length} samples
        </div>
      </div>

      <div className="flex flex-col gap-3 p-3">
        {tracks.length === 0 ? (
          <div className="rounded-md border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.55)] px-4 py-6 text-sm text-text-muted">
            No terminal tracks available.
          </div>
        ) : (
          tracks.map((track) => (
            <TerminalTrack
              key={track.id}
              track={track}
              selectedInstanceId={selectedInstanceId}
              onSelectInstance={onSelectInstance}
            />
          ))
        )}
      </div>
    </div>
  );
}
