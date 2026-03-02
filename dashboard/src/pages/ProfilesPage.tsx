import { useEffect, useState } from 'react'
import { useAppStore } from '../stores/useAppStore'
import { Toolbar, EmptyState, Button, Modal, Input } from '../components/atoms'
import { ProfileCard } from '../components/molecules'
import * as api from '../services/api'

export default function ProfilesPage() {
  const { profiles, instances, profilesLoading, setProfiles, setProfilesLoading } = useAppStore()
  const [showCreate, setShowCreate] = useState(false)
  const [showLaunch, setShowLaunch] = useState<string | null>(null)

  // Create form
  const [createName, setCreateName] = useState('')
  const [createUseWhen, setCreateUseWhen] = useState('')
  const [createSource, setCreateSource] = useState('')

  // Launch form
  const [launchPort, setLaunchPort] = useState('9868')
  const [launchHeadless, setLaunchHeadless] = useState(false)

  const loadProfiles = async () => {
    setProfilesLoading(true)
    try {
      const data = await api.fetchProfiles()
      setProfiles(data)
    } catch (e) {
      console.error('Failed to load profiles', e)
    } finally {
      setProfilesLoading(false)
    }
  }

  // Only load on mount if empty — SSE handles updates
  useEffect(() => {
    if (profiles.length === 0) {
      loadProfiles()
    }
  }, [])

  const handleCreate = async () => {
    if (!createName.trim()) return
    try {
      await api.createProfile({
        name: createName.trim(),
        useWhen: createUseWhen.trim() || undefined,
        source: createSource.trim() || undefined,
      })
      setShowCreate(false)
      setCreateName('')
      setCreateUseWhen('')
      setCreateSource('')
      loadProfiles()
    } catch (e) {
      console.error('Failed to create profile', e)
    }
  }

  const handleLaunch = async () => {
    if (!showLaunch) return
    try {
      await api.launchInstance({
        profileId: showLaunch,
        port: parseInt(launchPort) || 9868,
        headless: launchHeadless,
      })
      setShowLaunch(null)
      setLaunchPort('9868')
      setLaunchHeadless(false)
      // Instances will refresh automatically
    } catch (e) {
      console.error('Failed to launch instance', e)
    }
  }

  const instanceByProfile = new Map(
    instances.map((i) => [i.profileName, i])
  )

  return (
    <div className="flex flex-1 flex-col">
      <Toolbar
        actions={[
          { key: 'new', label: '+ New Profile', onClick: () => setShowCreate(true), variant: 'primary' },
          { key: 'refresh', label: 'Refresh', onClick: loadProfiles },
        ]}
      />

      <div className="flex-1 overflow-auto p-4">
        {profilesLoading && profiles.length === 0 ? (
          <div className="flex items-center justify-center py-16 text-text-muted">
            Loading profiles...
          </div>
        ) : profiles.length === 0 ? (
          <EmptyState
            title="No profiles yet"
            description="Click + New Profile to create one"
            action={
              <Button variant="primary" onClick={() => setShowCreate(true)}>
                + New Profile
              </Button>
            }
          />
        ) : (
          <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-3 xl:grid-cols-4">
            {profiles.map((p) => (
              <ProfileCard
                key={p.name}
                profile={p}
                instance={instanceByProfile.get(p.name)}
                onLaunch={() => setShowLaunch(p.name)}
                onManage={() => {
                  // TODO: manage modal
                }}
              />
            ))}
          </div>
        )}
      </div>

      {/* Create Profile Modal */}
      <Modal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        title="📁 New Profile"
        wide
        actions={
          <>
            <Button variant="secondary" onClick={() => setShowCreate(false)}>
              Cancel
            </Button>
            <Button variant="primary" onClick={handleCreate} disabled={!createName.trim()}>
              Create
            </Button>
          </>
        }
      >
        <div className="flex flex-col gap-4">
          <Input
            label="Name"
            placeholder="e.g. personal, work, scraping"
            value={createName}
            onChange={(e) => setCreateName(e.target.value)}
          />
          <Input
            label="Use this profile when (helps agents pick the right profile)"
            placeholder="e.g. I need to access Gmail for the team account"
            value={createUseWhen}
            onChange={(e) => setCreateUseWhen(e.target.value)}
          />
          <Input
            label="Import from (optional — Chrome user data path)"
            placeholder="e.g. /Users/you/Library/Application Support/Google/Chrome"
            value={createSource}
            onChange={(e) => setCreateSource(e.target.value)}
          />
        </div>
      </Modal>

      {/* Launch Modal */}
      <Modal
        open={!!showLaunch}
        onClose={() => setShowLaunch(null)}
        title="🖥️ Launch Profile"
        actions={
          <>
            <Button variant="secondary" onClick={() => setShowLaunch(null)}>
              Cancel
            </Button>
            <Button variant="primary" onClick={handleLaunch}>
              Launch
            </Button>
          </>
        }
      >
        <div className="flex flex-col gap-4">
          <Input
            label="Port"
            placeholder="e.g. 9868"
            value={launchPort}
            onChange={(e) => setLaunchPort(e.target.value)}
          />
          <label className="flex items-center gap-2 text-sm text-text-secondary">
            <input
              type="checkbox"
              checked={launchHeadless}
              onChange={(e) => setLaunchHeadless(e.target.checked)}
              className="h-4 w-4"
            />
            Headless (best for Docker/VPS)
          </label>
        </div>
      </Modal>
    </div>
  )
}
