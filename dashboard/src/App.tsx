import { useEffect } from 'react'
import { HashRouter, Routes, Route, Navigate } from 'react-router-dom'
import { useAppStore } from './stores/useAppStore'
import { Header } from './components/molecules'
import { ProfilesPage, InstancesPage, AgentsPage, SettingsPage } from './pages'
import * as api from './services/api'

function AppContent() {
  const { setInstances } = useAppStore()

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
