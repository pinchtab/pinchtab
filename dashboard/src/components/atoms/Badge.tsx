type Variant = 'default' | 'success' | 'warning' | 'danger' | 'info'

interface Props {
  variant?: Variant
  children: React.ReactNode
}

const variants: Record<Variant, string> = {
  default: 'bg-bg-elevated text-text-secondary',
  success: 'bg-success/15 text-success',
  warning: 'bg-warning/15 text-warning',
  danger: 'bg-destructive/15 text-destructive',
  info: 'bg-info/15 text-info',
}

export default function Badge({ variant = 'default', children }: Props) {
  return (
    <span
      className={`inline-flex items-center rounded px-1.5 py-0.5 text-xs font-medium ${variants[variant]}`}
    >
      {children}
    </span>
  )
}
