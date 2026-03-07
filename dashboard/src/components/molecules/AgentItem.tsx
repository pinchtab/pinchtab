import { Badge } from "../atoms";
import type { Agent } from "../../types";

interface Props {
  agent: Agent;
  selected: boolean;
  onClick: () => void;
}

function timeAgo(date: string): string {
  const diff = Date.now() - new Date(date).getTime();
  const secs = Math.floor(diff / 1000);
  if (secs < 5) return "just now";
  if (secs < 60) return `${secs}s ago`;
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`;
  return `${Math.floor(secs / 3600)}h ago`;
}

export default function AgentItem({ agent, selected, onClick }: Props) {
  return (
    <button
      className={`flex w-full items-center gap-3 px-3 py-3 text-left ${
        selected
          ? "dashboard-panel dashboard-panel-selected border-primary"
          : "dashboard-panel dashboard-panel-hover"
      }`}
      onClick={onClick}
    >
      <div className="flex h-10 w-10 items-center justify-center rounded-full border border-border-subtle bg-primary/10 text-lg shadow-[0_0_20px_rgb(var(--brand-accent-rgb)/0.08)]">
        🤖
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate font-medium text-text-primary">
          {agent.name || agent.id}
        </div>
        <div className="dashboard-mono text-xs text-text-muted">
          {timeAgo(agent.lastActivity || agent.connectedAt)}
        </div>
      </div>
      <Badge>{agent.requestCount}</Badge>
    </button>
  );
}
