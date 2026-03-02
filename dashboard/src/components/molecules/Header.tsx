import { StatusDot } from '../atoms'
import type { View } from '../../types'

interface Props {
  currentView: View
  onViewChange: (view: View) => void
}

const views: { key: View; label: string }[] = [
  { key: 'profiles', label: 'Profiles' },
  { key: 'instances', label: 'Instances' },
  { key: 'agents', label: 'Agents' },
  { key: 'settings', label: 'Settings' },
]

const tabBase =
  'rounded px-3 py-1.5 text-sm font-medium transition-all duration-150 cursor-pointer'
const tabInactive = 'text-text-muted hover:text-text-secondary hover:bg-bg-elevated'
const tabActive = 'text-primary bg-primary/10'

export default function Header({ currentView, onViewChange }: Props) {
  return (
    <header className="flex items-center justify-between border-b border-border-subtle bg-bg-surface px-4 py-3">
      <div className="flex items-center gap-3">
        <img
          src={`${import.meta.env.BASE_URL}pinchtab-headed-192.png`}
          alt="Pinchtab"
          className="h-8 w-8"
        />
        <span className="text-lg font-semibold text-text-primary">Pinchtab</span>
      </div>

      <nav className="flex items-center gap-1">
        {views.map((v) => (
          <button
            key={v.key}
            className={`${tabBase} ${currentView === v.key ? tabActive : tabInactive}`}
            onClick={() => onViewChange(v.key)}
          >
            {v.label}
          </button>
        ))}
      </nav>

      <StatusDot status="online" label="Live" />
    </header>
  )
}
