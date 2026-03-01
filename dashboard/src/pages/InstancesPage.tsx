import { useEffect } from 'react'
import { useAppStore } from '../stores/useAppStore'
import { Toolbar, EmptyState } from '../components/atoms'
import { InstanceCard } from '../components/molecules'
import * as api from '../services/api'

export default function InstancesPage() {
  const { instances, instancesLoading, setInstances, setInstancesLoading } = useAppStore()

  const loadInstances = async () => {
    setInstancesLoading(true)
    try {
      const data = await api.fetchInstances()
      setInstances(data)
    } catch (e) {
      console.error('Failed to load instances', e)
    } finally {
      setInstancesLoading(false)
    }
  }

  useEffect(() => {
    loadInstances()
    const interval = setInterval(loadInstances, 5000)
    return () => clearInterval(interval)
  }, [])

  const handleStop = async (id: string) => {
    try {
      await api.stopInstance(id)
      loadInstances()
    } catch (e) {
      console.error('Failed to stop instance', e)
    }
  }

  return (
    <div className="flex flex-1 flex-col">
      <Toolbar
        actions={[{ key: 'refresh', label: 'Refresh', onClick: loadInstances }]}
      />

      <div className="flex-1 overflow-auto p-4">
        {instancesLoading && instances.length === 0 ? (
          <div className="flex items-center justify-center py-16 text-text-muted">
            Loading instances...
          </div>
        ) : instances.length === 0 ? (
          <EmptyState
            title="No running instances"
            description="Launch a profile to start one"
          />
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {instances.map((inst) => (
              <InstanceCard
                key={inst.id}
                instance={inst}
                onOpen={() =>
                  window.open(`http://localhost:${inst.port}/dashboard`, '_blank')
                }
                onStop={() => handleStop(inst.id)}
              />
            ))}
          </div>
        )}
      </div>
    </div>
  )
}
