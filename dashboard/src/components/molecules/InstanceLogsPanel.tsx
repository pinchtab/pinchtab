import { useEffect, useState } from "react";
import * as api from "../../services/api";

interface Props {
  instanceId?: string;
  emptyMessage?: string;
}

export default function InstanceLogsPanel({
  instanceId,
  emptyMessage = "No instance logs available.",
}: Props) {
  const [logs, setLogs] = useState("");
  const [loading, setLoading] = useState(false);

  useEffect(() => {
    if (!instanceId) {
      setLogs("");
      setLoading(false);
      return;
    }

    let cancelled = false;
    setLoading(true);

    api
      .fetchInstanceLogs(instanceId)
      .then((nextLogs) => {
        if (!cancelled) {
          setLogs(nextLogs);
        }
      })
      .catch((error) => {
        console.error("Failed to load instance logs", error);
        if (!cancelled) {
          setLogs("");
        }
      })
      .finally(() => {
        if (!cancelled) {
          setLoading(false);
        }
      });

    return () => {
      cancelled = true;
    };
  }, [instanceId]);

  useEffect(() => {
    if (!instanceId) {
      return;
    }

    return api.subscribeToInstanceLogs(instanceId, {
      onLogs: (nextLogs) => setLogs(nextLogs),
    });
  }, [instanceId]);

  return (
    <div className="flex h-full min-h-0 flex-col">
      {loading && !logs ? (
        <div className="flex h-full items-center justify-center rounded-xl border border-border-subtle bg-black/10 px-4 py-6 text-sm text-text-muted">
          Loading logs...
        </div>
      ) : logs ? (
        <pre className="h-full overflow-auto rounded-xl border border-border-subtle bg-black/10 p-3 font-mono text-[10px] leading-4 text-text-secondary">
          {logs}
        </pre>
      ) : (
        <div className="flex h-full items-center justify-center rounded-xl border border-border-subtle bg-black/10 px-4 py-6 text-sm text-text-muted">
          {emptyMessage}
        </div>
      )}
    </div>
  );
}
