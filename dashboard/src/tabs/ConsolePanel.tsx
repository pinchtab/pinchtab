import * as api from "../services/api";
import type { ConsoleLogEntry } from "../services/api";
import { formatClockTime } from "../lib/format";
import TabLogPanel from "./TabLogPanel";

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

const LEVEL_BACKGROUNDS: Record<string, string> = {
  error: "bg-destructive/5",
  warn: "bg-warning/5",
};

function ConsoleRow({ entry }: { entry: ConsoleLogEntry }) {
  return (
    <div
      className={`flex gap-2 border-b border-border-subtle/50 px-3 py-1.5 hover:bg-white/2 ${LEVEL_BACKGROUNDS[entry.level] ?? ""}`}
    >
      <span className="shrink-0 text-text-muted">
        {formatClockTime(entry.timestamp)}
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
  );
}

export default function ConsolePanel({ tabId }: Props) {
  return (
    <TabLogPanel
      tabId={tabId}
      fetchEntries={api.fetchConsoleLogs}
      renderRow={(entry) => <ConsoleRow entry={entry} />}
      loadingLabel="Loading console logs..."
      emptyLabel="No console logs yet"
    />
  );
}
