import { IconBolt } from "../components/atoms/Icon";
import type { Session } from "../services/api";

interface SessionDividerProps {
  session?: Session;
  sessionId: string;
  timestamp: string;
}

function formatTime(ts: string): string {
  return new Date(ts).toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

export default function SessionDivider({
  session,
  sessionId,
  timestamp,
}: SessionDividerProps) {
  const label = session?.label || sessionId;
  const time = session?.createdAt
    ? formatTime(session.createdAt)
    : formatTime(timestamp);

  return (
    <div className="flex items-center gap-3 px-4 py-3">
      <div className="h-px flex-1 bg-border-subtle" />
      <div className="flex items-center gap-2 text-[0.72rem] text-text-muted">
        <IconBolt size={14} />
        <span className="font-medium text-text-secondary">{label}</span>
        <span>·</span>
        <span className="dashboard-mono">{time}</span>
      </div>
      <div className="h-px flex-1 bg-border-subtle" />
    </div>
  );
}
