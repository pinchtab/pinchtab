import { useState } from "react";
import * as api from "../services/api";
import type { ErrorLogEntry } from "../services/api";
import { formatLogTime } from "./formatLogTime";
import TabLogPanel from "./TabLogPanel";

interface Props {
  tabId: string;
}

function ErrorRow({ entry }: { entry: ErrorLogEntry }) {
  const [expanded, setExpanded] = useState(false);

  return (
    <div className="border-b border-border-subtle/50 bg-destructive/5 hover:bg-destructive/8">
      <button
        type="button"
        onClick={() => setExpanded(!expanded)}
        className="flex w-full gap-2 px-3 py-1.5 text-left"
      >
        <span className="shrink-0 text-text-muted">
          {formatLogTime(entry.timestamp)}
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
            {expanded ? "▲" : "▼"}
          </span>
        )}
      </button>
      {expanded && (
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
  );
}

export default function ErrorsPanel({ tabId }: Props) {
  return (
    <TabLogPanel
      tabId={tabId}
      fetchEntries={api.fetchErrorLogs}
      renderRow={(entry) => <ErrorRow entry={entry} />}
      loadingLabel="Loading errors..."
      emptyLabel="No errors yet"
    />
  );
}
