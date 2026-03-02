import { Card, Badge, Button } from '../atoms'
import type { Profile, Instance } from '../../types'

interface Props {
  profile: Profile
  instance?: Instance
  onLaunch: () => void
  onStop?: () => void
  onDetails?: () => void
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between text-xs">
      <span className="text-text-muted">{label}</span>
      <span className="text-text-secondary">{value}</span>
    </div>
  )
}

export default function ProfileCard({
  profile,
  instance,
  onLaunch,
  onStop,
  onDetails,
}: Props) {
  const isRunning = instance?.status === 'running'
  const isError = instance?.status === 'error'
  const accountText = profile.accountEmail || profile.accountName || '—'
  const sizeText = profile.sizeMB ? `${profile.sizeMB.toFixed(0)} MB` : '—'

  return (
    <Card hover className="flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-border-subtle p-3">
        <span className="truncate font-medium text-text-primary">
          {profile.name}
        </span>
        {isRunning ? (
          <Badge variant="success">:{instance.port}</Badge>
        ) : isError ? (
          <Badge variant="danger">error</Badge>
        ) : (
          <Badge>stopped</Badge>
        )}
      </div>

      {/* Body */}
      <div className="flex flex-1 flex-col gap-1.5 p-3">
        <InfoRow label="Size" value={sizeText} />
        <InfoRow label="Account" value={accountText} />
        {profile.useWhen && (
          <div className="mt-1">
            <div className="text-xs text-text-muted">Use when</div>
            <div className="mt-0.5 line-clamp-2 text-xs text-text-secondary">
              {profile.useWhen}
            </div>
          </div>
        )}
        {isError && instance?.error && (
          <div className="mt-1 text-xs text-destructive">{instance.error}</div>
        )}
      </div>

      {/* Actions */}
      <div className="flex gap-2 border-t border-border-subtle p-3">
        {onDetails && (
          <Button size="sm" variant="ghost" onClick={onDetails}>
            Details
          </Button>
        )}
        {isRunning ? (
          <Button
            size="sm"
            variant="danger"
            className="flex-1"
            onClick={onStop}
          >
            Stop
          </Button>
        ) : (
          <Button
            size="sm"
            variant="primary"
            className="flex-1"
            onClick={onLaunch}
          >
            Launch
          </Button>
        )}
      </div>
    </Card>
  )
}
