import { useCallback, useEffect, useRef, useState } from 'react'
import { NavLink } from 'react-router-dom'
import './NavBar.css'

interface Tab {
  id: string
  path: string
  label: string
}

const tabs: Tab[] = [
  { id: 'monitoring', path: '/monitoring', label: 'Monitoring' },
  { id: 'profiles', path: '/profiles', label: 'Profiles' },
  { id: 'agents', path: '/agents', label: 'Agents' },
  { id: 'settings', path: '/settings', label: 'Settings' },
]

interface NavBarProps {
  onRefresh?: () => void
}

export default function NavBar({ onRefresh }: NavBarProps) {
  const [refreshing, setRefreshing] = useState(false)
  const tabsRef = useRef<HTMLElement>(null)

  const handleRefresh = useCallback(() => {
    if (!onRefresh || refreshing) return
    setRefreshing(true)
    onRefresh()
    setTimeout(() => setRefreshing(false), 800)
  }, [onRefresh, refreshing])

  useEffect(() => {
    const handler = (e: KeyboardEvent) => {
      if (!e.metaKey && !e.ctrlKey) return
      const num = parseInt(e.key)
      if (num >= 1 && num <= tabs.length) {
        e.preventDefault()
        window.location.hash = tabs[num - 1].path
        return
      }
      if (e.key === 'r' && onRefresh) {
        e.preventDefault()
        handleRefresh()
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [onRefresh, handleRefresh])

  return (
    <header className="sticky top-0 z-50 flex h-[52px] items-center gap-0 border-b border-border-subtle bg-bg-app px-4">
      <div className="flex min-w-40 items-center gap-2.5">
        <img
          src={`${import.meta.env.BASE_URL}pinchtab-headed-192.png`}
          alt="PinchTab"
          className="h-6 w-6"
        />
        <span className="text-sm font-semibold tracking-wide text-text-primary">
          PinchTab
        </span>
      </div>

      <nav className="ml-6 flex items-center gap-0.5" ref={tabsRef}>
        {tabs.map((tab, i) => (
          <NavLink
            key={tab.id}
            to={tab.path}
            className={({ isActive }) =>
              `navbar-tab relative cursor-pointer border-none bg-transparent px-3.5 py-3.5 text-sm font-medium leading-none whitespace-nowrap transition-colors duration-150 hover:text-text-primary focus-visible:rounded focus-visible:shadow-[0_0_0_2px_var(--primary)/25] focus-visible:outline-none ${
                isActive ? 'active text-text-primary' : 'text-text-secondary'
              }`
            }
            title={`${tab.label} (⌘${i + 1})`}
          >
            {tab.label}
          </NavLink>
        ))}
      </nav>

      <div className="ml-auto flex items-center gap-1.5">
        {onRefresh && (
          <button
            className={`navbar-icon-btn flex h-8 w-8 cursor-pointer items-center justify-center rounded-md border border-transparent bg-transparent text-base text-text-muted transition-all duration-150 hover:border-border-subtle hover:bg-bg-elevated hover:text-text-secondary focus-visible:shadow-[0_0_0_2px_var(--primary)/25] focus-visible:outline-none ${
              refreshing ? 'spinning' : ''
            }`}
            onClick={handleRefresh}
            title="Refresh (⌘R)"
          >
            ↻
          </button>
        )}
      </div>
    </header>
  )
}
