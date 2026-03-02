import { Card, Badge, Button, StatusDot } from '../atoms'
import type { Instance } from '../../types'

interface Props {
  instance: Instance
  onOpen: () => void
  onStop: () => void
}

function formatUptime(startTime: string): string {
  const diff = Date.now() - new Date(startTime).getTime()
  const mins = Math.floor(diff / 60000)
  if (mins < 60) return `${mins}m`
  const hours = Math.floor(mins / 60)
  if (hours < 24) return `${hours}h ${mins % 60}m`
  return `${Math.floor(hours / 24)}d ${hours % 24}h`
}

export default function InstanceCard({ instance, onOpen, onStop }: Props) {
  return (
    <Card hover className="flex flex-col p-4">
      <div className="mb-3 flex items-start justify-between">
        <div className="flex items-center gap-2">
          <StatusDot status="online" />
          <div>
            <div className="font-medium text-text-primary">{instance.profileName}</div>
            <div className="text-xs text-text-muted">:{instance.port}</div>
          </div>
        </div>
        <Badge variant={instance.headless ? 'info' : 'default'}>
          {instance.headless ? 'Headless' : 'Headed'}
        </Badge>
      </div>

      <div className="mb-3 rounded bg-bg-elevated px-2 py-1.5 text-xs">
        <span className="text-text-muted">Uptime:</span>{' '}
        <span className="text-text-secondary">{formatUptime(instance.startTime)}</span>
      </div>

      <div className="mt-auto flex gap-2">
        <Button size="sm" variant="primary" className="flex-1" onClick={onOpen}>
          Open Dashboard
        </Button>
        <Button size="sm" variant="danger" onClick={onStop}>
          Stop
        </Button>
      </div>
    </Card>
  )
}
