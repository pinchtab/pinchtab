import { useEffect, useRef, useState } from "react";
import { useAsyncResource } from "../hooks/useAsyncResource";
import * as api from "../services/api";

interface Props {
  instanceId?: string;
  emptyMessage?: string;
}

const MAX_LOG_CHARS = 256 * 1024;

export default function InstanceLogsPanel({
  instanceId,
  emptyMessage = "No instance logs available.",
}: Props) {
  const [instanceLogs, setLogs] = useState("");
  const bottomRef = useRef<HTMLDivElement>(null);
  const streamVersionRef = useRef(0);

  const { loading } = useAsyncResource(
    instanceId
      ? async (signal) => {
          const fetchStartedAtVersion = streamVersionRef.current;
          try {
            const nextLogs = await api.fetchInstanceLogs(instanceId);
            if (
              !signal.aborted &&
              streamVersionRef.current === fetchStartedAtVersion
            ) {
              setLogs(nextLogs);
            }
          } catch (error) {
            console.error("Failed to load instance logs", error);
            if (!signal.aborted) setLogs("");
          }
        }
      : null,
    [instanceId],
    { initial: undefined },
  );
  const logs = instanceId ? instanceLogs : "";

  useEffect(() => {
    if (!instanceId) {
      return;
    }

    return api.subscribeToInstanceLogs(instanceId, {
      onLogs: (chunk, reset) => {
        streamVersionRef.current += 1;
        setLogs((prev) => {
          const next = reset ? chunk : prev + chunk;
          return next.length > MAX_LOG_CHARS
            ? next.slice(next.length - MAX_LOG_CHARS)
            : next;
        });
      },
    });
  }, [instanceId]);

  useEffect(() => {
    bottomRef.current?.scrollIntoView({ behavior: "smooth" });
  }, [logs]);

  if (loading && !logs) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-text-muted">
        Loading logs...
      </div>
    );
  }

  if (!logs) {
    return (
      <div className="flex h-full items-center justify-center text-sm text-text-muted">
        {emptyMessage}
      </div>
    );
  }

  const lines = logs.split("\n");

  return (
    <div className="min-h-0 flex-1 overflow-auto font-mono text-xs">
      {lines.map((line, i) => (
        <div
          key={i}
          className="border-b border-border-subtle/50 px-3 py-1.5 hover:bg-white/2"
        >
          <span className="break-all text-text-secondary">{line}</span>
        </div>
      ))}
      <div ref={bottomRef} />
    </div>
  );
}
