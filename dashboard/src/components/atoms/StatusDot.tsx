type Status = 'online' | 'offline' | 'warning' | 'loading'

interface Props {
  status: Status
  label?: string
  size?: 'sm' | 'md'
}

const colors: Record<Status, string> = {
  online: 'bg-success',
  offline: 'bg-text-muted',
  warning: 'bg-warning',
  loading: 'bg-primary animate-pulse',
}

const sizes = {
  sm: 'h-2 w-2',
  md: 'h-2.5 w-2.5',
}

export default function StatusDot({ status, label, size = 'md' }: Props) {
  return (
    <div className="flex items-center gap-1.5">
      <div className={`rounded-full ${colors[status]} ${sizes[size]}`} />
      {label && <span className="text-xs text-text-secondary">{label}</span>}
    </div>
  )
}
