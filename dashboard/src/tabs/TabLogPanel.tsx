import type { ReactNode } from "react";
import { useAsyncResource } from "../hooks/useAsyncResource";

const TAB_LOG_POLL_MS = 3000;

interface TabLogEntry {
  timestamp: string;
}

interface Props<T extends TabLogEntry> {
  tabId: string;
  fetchEntries: (tabId: string) => Promise<T[]>;
  renderRow: (entry: T) => ReactNode;
  loadingLabel: string;
  emptyLabel: string;
}

function PanelMessage({ children }: { children: ReactNode }) {
  return (
    <div className="flex h-full flex-1 items-center justify-center text-sm text-text-muted">
      {children}
    </div>
  );
}

export default function TabLogPanel<T extends TabLogEntry>({
  tabId,
  fetchEntries,
  renderRow,
  loadingLabel,
  emptyLabel,
}: Props<T>) {
  const { data, loading, error } = useAsyncResource(
    () => fetchEntries(tabId),
    [tabId],
    { initial: [] as T[], pollMs: TAB_LOG_POLL_MS },
  );
  const entries = error ? [] : data;

  if (loading) return <PanelMessage>{loadingLabel}</PanelMessage>;
  if (entries.length === 0) return <PanelMessage>{emptyLabel}</PanelMessage>;

  return (
    <div className="min-h-0 flex-1 overflow-auto font-mono text-xs">
      {entries.map((entry, i) => (
        <div key={`${entry.timestamp}-${i}`}>{renderRow(entry)}</div>
      ))}
    </div>
  );
}
