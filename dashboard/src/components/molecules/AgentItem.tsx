import { Badge } from '../atoms'
import type { Agent } from '../../types'

interface Props {
  agent: Agent
  selected: boolean
  onClick: () => void
}

function timeAgo(date: string): string {
  const diff = Date.now() - new Date(date).getTime()
  const secs = Math.floor(diff / 1000)
  if (secs < 5) return 'just now'
  if (secs < 60) return `${secs}s ago`
  if (secs < 3600) return `${Math.floor(secs / 60)}m ago`
  return `${Math.floor(secs / 3600)}h ago`
}

export default function AgentItem({ agent, selected, onClick }: Props) {
  return (
    <button
      className={`flex w-full items-center gap-3 rounded-lg px-3 py-2 text-left transition-all duration-150 ${
        selected
          ? 'bg-primary/10 border border-primary'
          : 'border border-transparent hover:bg-bg-elevated'
      }`}
      onClick={onClick}
    >
      <div className="flex h-9 w-9 items-center justify-center rounded-full bg-bg-elevated text-lg">
        🤖
      </div>
      <div className="min-w-0 flex-1">
        <div className="truncate font-medium text-text-primary">
          {agent.name || agent.id}
        </div>
        <div className="text-xs text-text-muted">
          {timeAgo(agent.lastActivity || agent.connectedAt)}
        </div>
      </div>
      <Badge>{agent.requestCount}</Badge>
    </button>
  )
}
