import { useEffect, useRef, useState } from "react";
import * as api from "../services/api";

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
  const bottomRef = useRef<HTMLDivElement>(null);
  const streamVersionRef = useRef(0);

  useEffect(() => {
    if (!instanceId) {
      setLogs("");
      setLoading(false);
      return;
    }

    let cancelled = false;
    const fetchStartedAtVersion = streamVersionRef.current;
    setLoading(true);

    api
      .fetchInstanceLogs(instanceId)
      .then((nextLogs) => {
        if (!cancelled && streamVersionRef.current === fetchStartedAtVersion) {
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
      onLogs: (nextLogs) => {
        streamVersionRef.current += 1;
        setLogs(nextLogs);
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
