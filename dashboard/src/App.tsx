import { useEffect } from 'react'
import { useAppStore } from './stores/useAppStore'
import { Header } from './components/molecules'
import { ProfilesPage, InstancesPage, AgentsPage, SettingsPage } from './pages'
import * as api from './services/api'

export default function App() {
  const { view, setView, setInstances } = useAppStore()

  // Load instances globally (needed for profile cards)
  useEffect(() => {
    const load = async () => {
      try {
        const data = await api.fetchInstances()
        setInstances(data)
      } catch (e) {
        console.error('Failed to load instances', e)
      }
    }
    load()
    const interval = setInterval(load, 5000)
    return () => clearInterval(interval)
  }, [setInstances])

  return (
    <div className="flex h-screen flex-col bg-bg-app">
      <Header currentView={view} onViewChange={setView} />
      <main className="flex flex-1 overflow-hidden">
        {view === 'profiles' && <ProfilesPage />}
        {view === 'instances' && <InstancesPage />}
        {view === 'agents' && <AgentsPage />}
        {view === 'settings' && <SettingsPage />}
      </main>
    </div>
  )
}
