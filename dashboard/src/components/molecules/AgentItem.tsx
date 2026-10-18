import type { TFunction } from "i18next";
import { useTranslation } from "react-i18next";
import { IconRobot, IconBolt } from "../atoms/Icon";
import type { Agent } from "../../types";
import type { Session } from "../../services/api";

interface Props {
  agent: Agent;
  selected: boolean;
  sessions: Session[];
  activeSessionId?: string;
  hasHandoff?: boolean;
  sessionsWithHandoff?: Set<string>;
  onClick: () => void;
  onSelectSession: (sessionId: string) => void;
}

function HandoffDot() {
  const { t } = useTranslation();

  return (
    <span
      aria-label={t("tabs.handoffDot")}
      className="inline-block h-2 w-2 shrink-0 rounded-full bg-red-500 ring-2 ring-bg-surface"
    />
  );
}

function timeAgo(date: string, t: TFunction): string {
  const diff = Date.now() - new Date(date).getTime();
  const secs = Math.floor(diff / 1000);
  if (secs < 5) return t("agents.justNow");
  if (secs < 60) return t("agents.secondsAgo", { count: secs });
  if (secs < 3600)
    return t("agents.minutesAgo", { count: Math.floor(secs / 60) });
  if (secs < 86400)
    return t("agents.hoursAgo", { count: Math.floor(secs / 3600) });
  return t("agents.daysAgo", { count: Math.floor(secs / 86400) });
}

function formatSessionTime(ts: string): string {
  return new Date(ts).toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
  });
}

function sessionDisplayName(session: Session, t: TFunction): string {
  if (session.label) return session.label;
  const start = formatSessionTime(session.createdAt);
  const end = formatSessionTime(session.lastSeenAt || session.createdAt);
  return start === end
    ? t("agents.sessionLabel", { start })
    : t("agents.sessionLabelRange", { start, end });
}

export default function AgentItem({
  agent,
  selected,
  sessions,
  activeSessionId,
  hasHandoff = false,
  sessionsWithHandoff,
  onClick,
  onSelectSession,
}: Props) {
  const { t } = useTranslation();

  return (
    <div>
      <button
        className={`flex w-full items-center gap-3 px-3 py-2.5 text-left transition-colors ${
          selected
            ? "bg-primary/8 text-text-primary"
            : "text-text-secondary hover:bg-bg-elevated hover:text-text-primary"
        }`}
        onClick={onClick}
      >
        <div className="flex h-7 w-7 shrink-0 items-center justify-center rounded-md text-primary">
          <IconRobot />
        </div>
        <div className="min-w-0 flex-1 flex items-center gap-2">
          <span className="truncate text-sm font-medium">
            {agent.name || agent.id}
          </span>
          {hasHandoff && <HandoffDot />}
        </div>
        <span className="dashboard-mono text-[0.65rem] text-text-muted">
          {timeAgo(agent.lastActivity || agent.connectedAt, t)}
        </span>
      </button>

      {selected && sessions.length > 0 && (
        <div className="ml-5 border-l border-border-subtle">
          {sessions.map((session) => {
            const isActive = activeSessionId === session.id;
            const sessionHasHandoff =
              sessionsWithHandoff?.has(session.id) ?? false;
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
                  {sessionDisplayName(session, t)}
                </span>
                {sessionHasHandoff && <HandoffDot />}
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}
