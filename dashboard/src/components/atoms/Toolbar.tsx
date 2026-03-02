interface ToolbarAction {
  key: string
  label: string
  onClick: () => void
  variant?: 'default' | 'primary'
  disabled?: boolean
}

interface Props {
  actions?: ToolbarAction[]
  children?: React.ReactNode
}

const actionBtn =
  'inline-flex cursor-pointer items-center gap-1.5 whitespace-nowrap rounded border border-border-subtle bg-bg-elevated px-3 py-1.5 text-xs font-medium text-text-secondary transition-all duration-150 hover:not-disabled:border-border-default hover:not-disabled:bg-bg-hover hover:not-disabled:text-text-primary disabled:cursor-not-allowed disabled:opacity-40'

const primaryBtn =
  'border-primary/50 bg-primary/10 text-primary hover:not-disabled:border-primary hover:not-disabled:bg-primary/20'

export default function Toolbar({ actions, children }: Props) {
  return (
    <div className="flex items-center gap-2 border-b border-border-subtle bg-bg-surface px-4 py-2">
      {actions?.map((a) => (
        <button
          key={a.key}
          className={`${actionBtn} ${a.variant === 'primary' ? primaryBtn : ''}`}
          onClick={a.onClick}
          disabled={a.disabled}
        >
          {a.label}
        </button>
      ))}
      {children}
    </div>
  )
}
