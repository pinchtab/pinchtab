interface Props {
  icon?: string
  title: string
  description?: string
  action?: React.ReactNode
}

export default function EmptyState({ icon = '🦀', title, description, action }: Props) {
  return (
    <div className="flex flex-col items-center justify-center py-16 text-center">
      <div className="mb-3 text-5xl">{icon}</div>
      <div className="text-text-secondary">{title}</div>
      {description && (
        <div className="mt-1 text-sm text-text-muted">{description}</div>
      )}
      {action && <div className="mt-4">{action}</div>}
    </div>
  )
}
