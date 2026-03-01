import { Badge } from '../atoms'
import type { ActivityEvent } from '../../types'

interface Props {
  event: ActivityEvent
}

const typeColors: Record<string, 'default' | 'success' | 'info' | 'warning'> = {
  navigate: 'info',
  snapshot: 'success',
  action: 'warning',
  screenshot: 'default',
  other: 'default',
}

const typeIcons: Record<string, string> = {
  navigate: '🧭',
  snapshot: '📸',
  action: '👆',
  screenshot: '🖼️',
  other: '📝',
}

function formatTime(ts: string): string {
  return new Date(ts).toLocaleTimeString('en-GB', {
    hour: '2-digit',
    minute: '2-digit',
    second: '2-digit',
  })
}

export default function ActivityLine({ event }: Props) {
  return (
    <div className="flex items-center gap-3 border-b border-border-subtle px-4 py-2 text-sm">
      <span className="text-lg">{typeIcons[event.type] || '📝'}</span>
      <span className="w-16 shrink-0 font-mono text-xs text-text-muted">
        {formatTime(event.timestamp)}
      </span>
      <Badge variant={typeColors[event.type] || 'default'}>{event.method}</Badge>
      <span className="min-w-0 flex-1 truncate font-mono text-text-secondary">
        {event.path}
      </span>
      <span className="shrink-0 text-xs text-text-muted">{event.agentId}</span>
    </div>
  )
}
