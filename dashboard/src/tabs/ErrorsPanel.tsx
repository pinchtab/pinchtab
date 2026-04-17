import { useEffect, useState } from "react";
import * as api from "../services/api";
import type { ErrorLogEntry } from "../services/api";

interface Props {
  tabId: string;
}

function formatTime(ts: string): string {
  return new Date(ts).toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

export default function ErrorsPanel({ tabId }: Props) {
  const [errors, setErrors] = useState<ErrorLogEntry[]>([]);
  const [loading, setLoading] = useState(true);
  const [expandedIndex, setExpandedIndex] = useState<number | null>(null);

  useEffect(() => {
    let cancelled = false;
    setLoading(true);

    const fetchErrors = () => {
      api
        .fetchErrorLogs(tabId)
        .then((data) => {
          if (!cancelled) setErrors(data);
        })
        .catch(() => {
          if (!cancelled) setErrors([]);
        })
        .finally(() => {
          if (!cancelled) setLoading(false);
        });
    };

    fetchErrors();
    const interval = setInterval(fetchErrors, 3000);
    return () => {
      cancelled = true;
      clearInterval(interval);
    };
  }, [tabId]);

  if (loading) {
    return (
      <div className="flex h-full flex-1 items-center justify-center text-sm text-text-muted">
        Loading errors...
      </div>
    );
  }

  if (errors.length === 0) {
    return (
      <div className="flex h-full flex-1 items-center justify-center text-sm text-text-muted">
        No errors yet
      </div>
    );
  }

  return (
    <div className="min-h-0 flex-1 overflow-auto font-mono text-xs">
      {errors.map((entry, i) => (
        <div
          key={`${entry.timestamp}-${i}`}
          className="border-b border-border-subtle/50 bg-destructive/5 hover:bg-destructive/8"
        >
          <button
            type="button"
            onClick={() => setExpandedIndex(expandedIndex === i ? null : i)}
            className="flex w-full gap-2 px-3 py-1.5 text-left"
          >
            <span className="shrink-0 text-text-muted">
              {formatTime(entry.timestamp)}
            </span>
            {entry.type && (
              <span className="shrink-0 font-semibold text-destructive">
                {entry.type}
              </span>
            )}
            <span className="min-w-0 flex-1 truncate text-text-primary">
              {entry.message}
            </span>
            {entry.stack && (
              <span className="shrink-0 text-text-muted">
                {expandedIndex === i ? "▲" : "▼"}
              </span>
            )}
          </button>
          {expandedIndex === i && (
            <div className="px-3 pb-2">
              {entry.url && (
                <div className="mb-1 text-text-muted">
                  {entry.url}
                  {entry.line ? `:${entry.line}` : ""}
                  {entry.column ? `:${entry.column}` : ""}
                </div>
              )}
              {entry.stack && (
                <pre className="overflow-x-auto whitespace-pre-wrap text-text-secondary">
                  {entry.stack}
                </pre>
              )}
            </div>
          )}
        </div>
      ))}
    </div>
  );
}
