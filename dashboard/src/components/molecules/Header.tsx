import { NavLink } from 'react-router-dom'
import { StatusDot } from '../atoms'

const views = [
  { path: '/profiles', label: 'Profiles' },
  { path: '/instances', label: 'Instances' },
  { path: '/agents', label: 'Agents' },
  { path: '/settings', label: 'Settings' },
]

const tabBase =
  'rounded px-3 py-1.5 text-sm font-medium transition-all duration-150'
const tabInactive = 'text-text-muted hover:text-text-secondary hover:bg-bg-elevated'
const tabActive = 'text-primary bg-primary/10'

export default function Header() {
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
          <NavLink
            key={v.path}
            to={v.path}
            className={({ isActive }) =>
              `${tabBase} ${isActive ? tabActive : tabInactive}`
            }
          >
            {v.label}
          </NavLink>
        ))}
      </nav>

      <StatusDot status="online" label="Live" />
    </header>
  )
}
