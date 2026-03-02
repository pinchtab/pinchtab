import { useEffect, useState, useCallback, useRef } from 'react'
import { useAppStore } from '../stores/useAppStore'
import { Card, EmptyState, Badge, Button } from '../components/atoms'
import TabsChart, { type TabDataPoint } from '../components/molecules/TabsChart'
import type { InstanceTab } from '../generated/types'
import * as api from '../services/api'

const POLL_INTERVAL = 30000 // 30 seconds
const MAX_DATA_POINTS = 60 // 30 minutes of data

export default function MonitoringPage() {
  const { instances, setInstances, setInstancesLoading } = useAppStore()
  const [selectedId, setSelectedId] = useState<string | null>(null)
  const [tabsData, setTabsData] = useState<TabDataPoint[]>([])
  const [currentTabs, setCurrentTabs] = useState<Record<string, InstanceTab[]>>({})
  const pollRef = useRef<ReturnType<typeof setInterval> | null>(null)

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

  // Fetch tabs for all running instances
  const fetchAllInstanceTabs = useCallback(async () => {
    const runningInstances = instances.filter((i) => i.status === 'running')
    if (runningInstances.length === 0) return

    const timestamp = Date.now()
    const dataPoint: TabDataPoint = { timestamp }
    const tabsByInstance: Record<string, InstanceTab[]> = {}

    await Promise.all(
      runningInstances.map(async (inst) => {
        try {
          const tabs = await api.fetchInstanceTabs(inst.id)
          dataPoint[inst.id] = tabs.length
          tabsByInstance[inst.id] = tabs
        } catch {
          dataPoint[inst.id] = 0
          tabsByInstance[inst.id] = []
        }
      })
    )

    setTabsData((prev) => [...prev.slice(-MAX_DATA_POINTS + 1), dataPoint])
    setCurrentTabs(tabsByInstance)
  }, [instances])

  // Initial load
  useEffect(() => {
    if (instances.length === 0) {
      loadInstances()
    }
  }, [])

  // Poll tabs
  useEffect(() => {
    fetchAllInstanceTabs()
    pollRef.current = setInterval(fetchAllInstanceTabs, POLL_INTERVAL)
    return () => {
      if (pollRef.current) clearInterval(pollRef.current)
    }
  }, [fetchAllInstanceTabs])

  // Auto-select first running instance
  useEffect(() => {
    if (!selectedId) {
      const firstRunning = instances.find((i) => i.status === 'running')
      if (firstRunning) setSelectedId(firstRunning.id)
    }
  }, [instances, selectedId])

  const handleStop = async (id: string) => {
    try {
      await api.stopInstance(id)
    } catch (e) {
      console.error('Failed to stop instance', e)
    }
  }

  const selectedInstance = instances.find((i) => i.id === selectedId)
  const selectedTabs = selectedId ? currentTabs[selectedId] || [] : []
  const runningInstances = instances.filter((i) => i.status === 'running')

  if (instances.length === 0) {
    return (
      <div className="flex flex-1 items-center justify-center p-4">
        <EmptyState
          title="No instances"
          description="Launch a profile to start an instance"
        />
      </div>
    )
  }

  return (
    <div className="flex flex-1 flex-col gap-4 overflow-auto p-4">
      {/* Chart */}
      <TabsChart
        data={tabsData}
        instances={runningInstances.map((i) => ({
          id: i.id,
          profileName: i.profileName,
        }))}
        selectedInstanceId={selectedId}
        onSelectInstance={setSelectedId}
      />

      {/* Bottom section */}
      <div className="flex flex-1 gap-4 overflow-hidden">
        {/* Instance list */}
        <div className="w-64 shrink-0 overflow-auto rounded-lg border border-border-subtle bg-bg-surface">
          <div className="border-b border-border-subtle p-3">
            <h3 className="text-sm font-semibold text-text-secondary">
              Instances ({instances.length})
            </h3>
          </div>
          <div className="p-2">
            {instances.map((inst) => (
              <button
                key={inst.id}
                onClick={() => setSelectedId(inst.id)}
                className={`mb-1 flex w-full items-center gap-2 rounded-lg px-3 py-2 text-left transition-all ${
                  selectedId === inst.id
                    ? 'bg-primary/10 border border-primary'
                    : 'border border-transparent hover:bg-bg-elevated'
                }`}
              >
                <div
                  className={`h-2 w-2 rounded-full ${
                    inst.status === 'running'
                      ? 'bg-success'
                      : inst.status === 'error'
                        ? 'bg-destructive'
                        : 'bg-text-muted'
                  }`}
                />
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm font-medium text-text-primary">
                    {inst.profileName}
                  </div>
                  <div className="text-xs text-text-muted">
                    :{inst.port} · {currentTabs[inst.id]?.length ?? 0} tabs
                  </div>
                </div>
                <Badge
                  variant={
                    inst.status === 'running'
                      ? 'success'
                      : inst.status === 'error'
                        ? 'danger'
                        : 'default'
                  }
                >
                  {inst.status}
                </Badge>
              </button>
            ))}
          </div>
        </div>

        {/* Selected instance details */}
        <div className="flex flex-1 flex-col overflow-hidden rounded-lg border border-border-subtle bg-bg-surface">
          {selectedInstance ? (
            <>
              <div className="flex items-center justify-between border-b border-border-subtle p-3">
                <div>
                  <h3 className="text-sm font-semibold text-text-primary">
                    {selectedInstance.profileName}
                  </h3>
                  <div className="text-xs text-text-muted">
                    Port {selectedInstance.port} ·{' '}
                    {selectedInstance.headless ? 'Headless' : 'Headed'}
                  </div>
                </div>
                <div className="flex gap-2">
                  <Button
                    size="sm"
                    onClick={() =>
                      window.open(
                        `http://localhost:${selectedInstance.port}/dashboard`,
                        '_blank'
                      )
                    }
                  >
                    Open Dashboard
                  </Button>
                  {selectedInstance.status === 'running' && (
                    <Button
                      size="sm"
                      variant="danger"
                      onClick={() => handleStop(selectedInstance.id)}
                    >
                      Stop
                    </Button>
                  )}
                </div>
              </div>

              {/* Tabs list */}
              <div className="flex-1 overflow-auto p-3">
                <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">
                  Open Tabs ({selectedTabs.length})
                </h4>
                {selectedTabs.length === 0 ? (
                  <div className="py-8 text-center text-sm text-text-muted">
                    No tabs open
                  </div>
                ) : (
                  <div className="space-y-1">
                    {selectedTabs.map((tab) => (
                      <Card key={tab.id} className="p-2">
                        <div className="truncate text-sm text-text-primary">
                          {tab.title || 'Untitled'}
                        </div>
                        <div className="truncate text-xs text-text-muted">
                          {tab.url}
                        </div>
                      </Card>
                    ))}
                  </div>
                )}
              </div>
            </>
          ) : (
            <div className="flex flex-1 items-center justify-center text-sm text-text-muted">
              Select an instance to view details
            </div>
          )}
        </div>
      </div>
    </div>
  )
}
