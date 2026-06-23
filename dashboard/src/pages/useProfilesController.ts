import { useEffect, useState, useMemo } from "react";
import { useLocation } from "react-router-dom";
import { useAppStore } from "../stores/useAppStore";
import * as api from "../services/api";
import type { Profile } from "../generated/types";

export function getProfileKey(profile: Profile) {
  return profile.id || profile.name;
}

// Match a profile by its key (id-or-name) OR by name — the preferred-profile
// rule shared by the loader and the route-sync effect.
function findProfileByKey(
  profiles: Profile[],
  key: string,
): Profile | undefined {
  return profiles.find((p) => getProfileKey(p) === key || p.name === key);
}

interface ProfilesLocationState {
  selectedProfileKey?: string;
}

export function useProfilesController() {
  const location = useLocation();
  const {
    profiles,
    instances,
    profilesLoading,
    setProfiles,
    setProfilesLoading,
    setInstances,
  } = useAppStore();
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
        const preferred = findProfileByKey(data, preferredProfileKey);
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

    const preferred = findProfileByKey(profiles, routeSelectedProfileKey);
    if (preferred && getProfileKey(preferred) !== selectedProfileKey) {
      setSelectedProfileKey(getProfileKey(preferred));
    }
  }, [profiles, routeSelectedProfileKey, selectedProfileKey]);

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

  return {
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
  };
}
