import { Card, Badge, Button } from '../atoms'
import type { Profile, Instance } from '../../types'

interface Props {
  profile: Profile
  instance?: Instance
  onLaunch: () => void
  onManage: () => void
}

export default function ProfileCard({ profile, instance, onLaunch, onManage }: Props) {
  const isRunning = !!instance

  return (
    <Card hover className="flex flex-col p-4">
      <div className="mb-3 flex items-start justify-between">
        <div className="flex items-center gap-2">
          <span className="text-2xl">📁</span>
          <div>
            <div className="font-medium text-text-primary">{profile.name}</div>
            {profile.useWhen && (
              <div className="mt-0.5 text-xs text-text-muted line-clamp-1">
                {profile.useWhen}
              </div>
            )}
          </div>
        </div>
        {isRunning ? (
          <Badge variant="success">Running</Badge>
        ) : (
          <Badge>Stopped</Badge>
        )}
      </div>

      {isRunning && instance && (
        <div className="mb-3 rounded bg-bg-elevated px-2 py-1.5 text-xs text-text-muted">
          Port {instance.port} · {instance.tabs ?? 0} tabs
        </div>
      )}

      <div className="mt-auto flex gap-2">
        {isRunning ? (
          <Button
            size="sm"
            variant="primary"
            className="flex-1"
            onClick={() => window.open(`http://localhost:${instance.port}/dashboard`, '_blank')}
          >
            Open
          </Button>
        ) : (
          <Button size="sm" variant="primary" className="flex-1" onClick={onLaunch}>
            Launch
          </Button>
        )}
        <Button size="sm" variant="ghost" onClick={onManage}>
          ⋯
        </Button>
      </div>
    </Card>
  )
}
