import { useEffect, useState, useMemo } from "react";
import { useLocation } from "react-router-dom";
import { useAppStore } from "../stores/useAppStore";
import { Toolbar, EmptyState, Button, Badge } from "../components/atoms";
import * as api from "../services/api";
import type { Profile } from "../generated/types";
import {
  CreateProfileModal,
  StartInstanceModal,
} from "../components/molecules";
import ProfileDetailsPanel from "../profiles/ProfileDetailsPanel";

function getProfileKey(profile: Profile) {
  return profile.id || profile.name;
}

interface ProfilesLocationState {
  selectedProfileKey?: string;
}

export default function ProfilesPage() {
  const location = useLocation();
  const {
    profiles,
    instances,
    profilesLoading,
    setProfiles,
    setProfilesLoading,
    setInstances,
  } = useAppStore();
  const [showCreate, setShowCreate] = useState(false);
  const [launchProfileKey, setLaunchProfileKey] = useState<string | null>(null);
  const [selectedProfileKey, setSelectedProfileKey] = useState<string | null>(
    null,
  );

  const locationState = location.state as ProfilesLocationState | null;
  const routeSelectedProfileKey = locationState?.selectedProfileKey ?? null;

  const loadProfiles = async (preferredProfileKey?: string) => {
    setProfilesLoading(true);
    try {
      const data = await api.fetchProfiles();
      setProfiles(data);
      if (preferredProfileKey) {
        const preferred = data.find(
          (profile) =>
            getProfileKey(profile) === preferredProfileKey ||
            profile.name === preferredProfileKey,
        );
        if (preferred) {
          setSelectedProfileKey(getProfileKey(preferred));
        }
      }
    } catch (e) {
      console.error("Failed to load profiles", e);
    } finally {
      setProfilesLoading(false);
    }
  };

  // Load once on mount if empty — SSE handles updates
  useEffect(() => {
    if (profiles.length === 0) {
      loadProfiles(routeSelectedProfileKey ?? undefined);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  useEffect(() => {
    if (!routeSelectedProfileKey || profiles.length === 0) {
      return;
    }

    const preferred = profiles.find(
      (profile) =>
        getProfileKey(profile) === routeSelectedProfileKey ||
        profile.name === routeSelectedProfileKey,
    );
    if (preferred && getProfileKey(preferred) !== selectedProfileKey) {
      setSelectedProfileKey(getProfileKey(preferred));
    }
  }, [profiles, routeSelectedProfileKey, selectedProfileKey]);

  const handleStop = async (profileName: string) => {
    const inst = instanceByProfile.get(profileName);
    if (!inst) return;
    try {
      await api.stopInstance(inst.id);
      const updated = await api.fetchInstances();
      setInstances(updated);
    } catch (e) {
      console.error("Failed to stop instance", e);
    }
  };

  const handleDelete = async () => {
    if (!selectedProfile?.id) return;
    try {
      await api.deleteProfile(selectedProfile.id);
      setSelectedProfileKey(null);
      loadProfiles();
    } catch (e) {
      console.error("Failed to delete profile", e);
    }
  };

  const handleSave = async (name: string, useWhen: string) => {
    if (!selectedProfile?.id) return;
    try {
      const updated = await api.updateProfile(selectedProfile.id, {
        name: name !== selectedProfile.name ? name : undefined,
        useWhen: useWhen !== selectedProfile.useWhen ? useWhen : undefined,
      });
      loadProfiles(updated.id || selectedProfile.id);
    } catch (e) {
      console.error("Failed to update profile", e);
    }
  };

  const instanceByProfile = useMemo(
    () => new Map(instances.map((i) => [i.profileName, i])),
    [instances],
  );
  const orderedProfiles = useMemo(() => {
    const running: Profile[] = [];
    const stopped: Profile[] = [];

    profiles.forEach((profile) => {
      if (instanceByProfile.get(profile.name)?.status === "running") {
        running.push(profile);
        return;
      }
      stopped.push(profile);
    });

    return [...running, ...stopped];
  }, [instanceByProfile, profiles]);
  const runningProfileKeys = orderedProfiles
    .filter(
      (profile) => instanceByProfile.get(profile.name)?.status === "running",
    )
    .map((profile) => getProfileKey(profile));
  const singleRunningProfileKey =
    runningProfileKeys.length === 1 ? runningProfileKeys[0] : null;
  const selectedProfile =
    orderedProfiles.find(
      (profile) => getProfileKey(profile) === selectedProfileKey,
    ) || null;
  const launchProfile =
    orderedProfiles.find(
      (profile) => getProfileKey(profile) === launchProfileKey,
    ) || null;
  const runningProfiles = instances.filter(
    (instance) => instance.status === "running",
  ).length;

  useEffect(() => {
    if (orderedProfiles.length === 0) {
      setSelectedProfileKey(null);
      return;
    }

    const hasValidSelection =
      !!selectedProfileKey &&
      orderedProfiles.some(
        (profile) => getProfileKey(profile) === selectedProfileKey,
      );

    if (!hasValidSelection) {
      setSelectedProfileKey(
        singleRunningProfileKey ?? getProfileKey(orderedProfiles[0]),
      );
    }
  }, [orderedProfiles, selectedProfileKey, singleRunningProfileKey]);

  return (
    <div className="flex h-full flex-col">
      <Toolbar
        actions={[
          { key: "refresh", label: "Refresh", onClick: loadProfiles },
          {
            key: "new",
            label: "New Profile",
            onClick: () => setShowCreate(true),
            variant: "primary",
          },
        ]}
      />

      <div className="flex flex-1 flex-col overflow-hidden p-4 lg:p-6">
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
            <div className="flex h-full min-h-0 flex-col gap-4 lg:flex-row">
              <div className="dashboard-panel flex max-h-88 w-full shrink-0 flex-col overflow-hidden lg:max-h-none lg:w-80">
                <div className="border-b border-border-subtle px-4 py-3">
                  <div className="dashboard-section-label mb-1">Profiles</div>
                  <div className="flex items-center justify-between gap-3">
                    <h3 className="text-sm font-semibold text-text-secondary">
                      Profiles ({profiles.length})
                    </h3>
                    <Badge
                      variant={runningProfiles > 0 ? "success" : "default"}
                    >
                      {runningProfiles} running
                    </Badge>
                  </div>
                </div>

                <div className="flex-1 overflow-auto p-2">
                  <div className="space-y-2">
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
                          className={`w-full rounded-2xl border px-4 py-3 text-left transition ${
                            isSelected
                              ? "dashboard-panel-selected border-primary"
                              : "dashboard-panel-hover border-border-subtle bg-black/10"
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
