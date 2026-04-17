import { useEffect, useState } from "react";
import * as api from "../services/api";
import type { ConsoleLogEntry } from "../services/api";

interface Props {
  tabId: string;
}

const LEVEL_STYLES: Record<string, string> = {
  error: "text-destructive",
  warn: "text-warning",
  info: "text-info",
  debug: "text-text-muted",
  log: "text-text-secondary",
};

function formatTime(ts: string): string {
  return new Date(ts).toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export default function ConsolePanel({ tabId }: Props) {
  const [logs, setLogs] = useState<ConsoleLogEntry[]>([]);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);

    const fetchLogs = () => {
      api
        .fetchConsoleLogs(tabId)
        .then((data) => {
          if (!cancelled) setLogs(data);
        })
        .catch(() => {
          if (!cancelled) setLogs([]);
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    };

    fetchLogs();
    const interval = setInterval(fetchLogs, 3000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [tabId]);

  if (loading) {
    return (
      <div className="flex h-full flex-1 items-center justify-center text-sm text-text-muted">
        Loading console logs...
      </div>
    );
  }

  if (logs.length === 0) {
    return (
      <div className="flex h-full flex-1 items-center justify-center text-sm text-text-muted">
        No console logs yet
      </div>
    );
  }

  return (
    <div className="min-h-0 flex-1 overflow-auto font-mono text-xs">
      {logs.map((entry, i) => (
        <div
          key={`${entry.timestamp}-${i}`}
          className={`flex gap-2 border-b border-border-subtle/50 px-3 py-1.5 hover:bg-white/2 ${
            entry.level === "error"
              ? "bg-destructive/5"
              : entry.level === "warn"
                ? "bg-warning/5"
                : ""
          }`}
        >
          <span className="shrink-0 text-text-muted">
            {formatTime(entry.timestamp)}
          </span>
          <span
            className={`w-10 shrink-0 font-semibold uppercase ${LEVEL_STYLES[entry.level] || "text-text-muted"}`}
          >
            {entry.level}
          </span>
          <span className="min-w-0 flex-1 break-all text-text-primary">
            {entry.message}
          </span>
          {entry.source && (
            <span className="shrink-0 truncate text-text-muted max-w-48">
              {entry.source}
            </span>
          )}
        </div>
      ))}
    </div>
  );
}
