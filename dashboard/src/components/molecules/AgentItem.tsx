import { IconRobot, IconBolt } from "../atoms/Icon";
import type { Agent } from "../../types";
import type { Session } from "../../services/api";

interface Props {
  agent: Agent;
  selected: boolean;
  sessions: Session[];
  activeSessionId?: string;
  onClick: (autoSessionId?: string) => void;
  onSelectSession: (sessionId: string) => void;
}

function timeAgo(date: string): string {
  const diff = Date.now() - new Date(date).getTime();
  const secs = Math.floor(diff / 1000);
  if (secs < 5) return "just now";
  if (secs < 60) return `${secs}s ago`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
  if (secs < 86400) return `${Math.floor(secs / 3600)}h ago`;
  return `${Math.floor(secs / 86400)}d`;
}

function formatSessionTime(ts: string): string {
  return new Date(ts).toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function sessionDisplayName(session: Session): string {
  if (session.label) return session.label;
  const start = formatSessionTime(session.createdAt);
  const end = formatSessionTime(session.lastSeenAt || session.createdAt);
  return start === end ? `Session ${start}` : `Session ${start}–${end}`;
}

export default function AgentItem({
  agent,
  selected,
  sessions,
  activeSessionId,
  onClick,
  onSelectSession,
}: Props) {
  const handleAgentClick = () => {
    const latestSession = sessions.length > 0 ? sessions[0].id : undefined;
    onClick(latestSession);
  };

  return (
    <div>
      <button
        className={`flex w-full items-center gap-3 px-3 py-2.5 text-left transition-colors ${
          selected
            ? "bg-primary/8 text-text-primary"
            : "text-text-secondary hover:bg-bg-elevated hover:text-text-primary"
        }`}
        onClick={handleAgentClick}
      >
        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-primary">
          <IconRobot />
        </div>
        <div className="min-w-0 flex-1">
          <span className="truncate text-sm font-medium">
            {agent.name || agent.id}
          </span>
        </div>
        <span className="dashboard-mono text-[0.65rem] text-text-muted">
          {timeAgo(agent.lastActivity || agent.connectedAt)}
        </span>
      </button>

      {selected && sessions.length > 0 && (
        <div className="ml-5 border-l border-border-subtle">
          {sessions.map((session) => {
            const isActive = activeSessionId === session.id;
            return (
              <button
                key={session.id}
                className={`flex w-full items-center gap-2 py-1.5 pl-4 pr-3 text-left transition-colors ${
                  isActive
                    ? "bg-primary/6 text-primary"
                    : "text-text-muted hover:bg-bg-elevated hover:text-text-secondary"
                }`}
                onClick={() => onSelectSession(session.id)}
              >
                <IconBolt size={14} />
                <span className="min-w-0 flex-1 truncate text-xs font-medium">
                  {sessionDisplayName(session)}
                </span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
