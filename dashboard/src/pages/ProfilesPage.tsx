import { useState } from "react";
import { EmptyState, Button, Badge } from "../components/atoms";
import {
  CreateProfileModal,
  StartInstanceModal,
} from "../components/molecules";
import ProfileDetailsPanel from "../profiles/ProfileDetailsPanel";
import { useProfilesController, getProfileKey } from "./useProfilesController";

export default function ProfilesPage() {
  const {
    profiles,
    profilesLoading,
    orderedProfiles,
    instanceByProfile,
    selectedProfile,
    selectedProfileKey,
    setSelectedProfileKey,
    launchProfile,
    setLaunchProfileKey,
    loadProfiles,
    handleStop,
    handleDelete,
    handleSave,
  } = useProfilesController();
  const [showCreate, setShowCreate] = useState(false);

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-1 flex-col overflow-hidden">
        <div className="h-full">
          {profilesLoading && profiles.length === 0 ? (
            <div className="flex items-center justify-center py-16 text-text-muted">
              Loading profiles...
            </div>
          ) : profiles.length === 0 ? (
            <EmptyState
              title="No profiles yet"
              description="Click New Profile to create one"
              action={
                <Button variant="primary" onClick={() => setShowCreate(true)}>
                  New Profile
                </Button>
              }
            />
          ) : (
            <div className="dashboard-panel flex h-full min-h-0 flex-col overflow-hidden rounded-none! border-t-0 lg:flex-row">
              <div className="flex max-h-88 w-full shrink-0 flex-col overflow-hidden border-r border-border-subtle bg-bg-surface/50 lg:max-h-none lg:w-80">
                <div className="flex items-center justify-between border-b border-border-subtle px-4 py-2.5">
                  <span className="text-xs font-medium text-text-muted">
                    Profiles
                  </span>
                  <button
                    type="button"
                    onClick={() => setShowCreate(true)}
                    className="rounded bg-primary px-2.5 py-1 text-xs font-medium text-white transition-colors hover:bg-primary/90"
                  >
                    New Profile
                  </button>
                </div>

                <div className="flex-1 overflow-auto">
                  <div>
                    {orderedProfiles.map((profile) => {
                      const instance = instanceByProfile.get(profile.name);
                      const isSelected =
                        getProfileKey(profile) === selectedProfileKey;
                      const accountText =
                        profile.accountEmail ||
                        profile.accountName ||
                        "No account";
                      const statusVariant =
                        instance?.status === "running"
                          ? "success"
                          : instance?.status === "error"
                            ? "danger"
                            : "default";
                      const statusLabel =
                        instance?.status === "running"
                          ? `:${instance.port}`
                          : instance?.status === "error"
                            ? "error"
                            : "stopped";

                      return (
                        <button
                          key={getProfileKey(profile)}
                          type="button"
                          onClick={() =>
                            setSelectedProfileKey(getProfileKey(profile))
                          }
                          className={`w-full border-b border-border-subtle px-3 py-2.5 text-left transition-colors ${
                            isSelected
                              ? "bg-bg-hover text-text-primary"
                              : "hover:bg-bg-hover/50"
                          }`}
                        >
                          <div className="flex items-start justify-between gap-3">
                            <div className="min-w-0">
                              <div className="truncate text-sm font-semibold text-text-primary">
                                {profile.name}
                              </div>
                              <div className="mt-1 text-xs text-text-muted">
                                {accountText}
                              </div>
                            </div>
                            <Badge variant={statusVariant}>{statusLabel}</Badge>
                          </div>

                          {profile.useWhen && (
                            <div className="mt-3 line-clamp-2 text-xs leading-5 text-text-secondary">
                              {profile.useWhen}
                            </div>
                          )}
                        </button>
                      );
                    })}
                  </div>
                </div>
              </div>

              <div className="min-h-0 min-w-0 flex-1">
                <ProfileDetailsPanel
                  profile={selectedProfile}
                  instance={
                    selectedProfile
                      ? instanceByProfile.get(selectedProfile.name)
                      : undefined
                  }
                  onLaunch={() =>
                    selectedProfile &&
                    setLaunchProfileKey(getProfileKey(selectedProfile))
                  }
                  onStop={() =>
                    selectedProfile && handleStop(selectedProfile.name)
                  }
                  onSave={handleSave}
                  onDelete={handleDelete}
                />
              </div>
            </div>
          )}
        </div>
      </div>

      <CreateProfileModal
        open={showCreate}
        onClose={() => setShowCreate(false)}
        onCreated={loadProfiles}
      />

      <StartInstanceModal
        open={!!launchProfile}
        profile={launchProfile}
        onClose={() => setLaunchProfileKey(null)}
      />
    </div>
  );
}
