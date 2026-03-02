import { useEffect } from 'react'
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import { useAppStore } from './stores/useAppStore'
import { Header } from './components/molecules'
import { ProfilesPage, InstancesPage, AgentsPage, SettingsPage } from './pages'
import * as api from './services/api'

function AppContent() {
  const { setInstances, setProfiles, setAgents } = useAppStore()

  // Initial load
  useEffect(() => {
    const load = async () => {
      try {
        const [instances, profiles] = await Promise.all([
          api.fetchInstances(),
          api.fetchProfiles(),
        ])
        setInstances(instances)
        setProfiles(profiles)
      } catch (e) {
        console.error('Failed to load initial data', e)
      }
    }
    load()
  }, [setInstances, setProfiles])

  // Subscribe to SSE events
  useEffect(() => {
    const unsubscribe = api.subscribeToEvents({
      onInit: (agents) => {
        setAgents(agents)
      },
      onSystem: async (event) => {
        console.log('System event:', event)
        // Refresh instances on any instance event
        if (event.type.startsWith('instance.')) {
          try {
            const instances = await api.fetchInstances()
            setInstances(instances)
            // Also refresh profiles to update running status
            const profiles = await api.fetchProfiles()
            setProfiles(profiles)
          } catch (e) {
            console.error('Failed to refresh after event', e)
          }
        }
      },
      onAgent: (event) => {
        console.log('Agent event:', event)
        // Could update agent activity here
      },
    })

    return unsubscribe
  }, [setInstances, setProfiles, setAgents])

  return (
    <div className="flex h-screen flex-col bg-bg-app">
      <Header />
      <main className="flex flex-1 overflow-hidden">
        <Routes>
          <Route path="/" element={<Navigate to="/profiles" replace />} />
          <Route path="/profiles" element={<ProfilesPage />} />
          <Route path="/instances" element={<InstancesPage />} />
          <Route path="/agents" element={<AgentsPage />} />
          <Route path="/settings" element={<SettingsPage />} />
        </Routes>
      </main>
    </div>
  )
}

export default function App() {
  return (
    <HashRouter>
      <AppContent />
    </HashRouter>
  )
}
