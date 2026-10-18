import { useState } from "react";
import { useTranslation } from "react-i18next";
import { EmptyState, Button, Badge } from "../components/atoms";
import type { Instance, Profile } from "../generated/types";
import {
  formatProfileBytes,
  groupBytes,
  groupOrder,
  groupProfiles,
  type ProfileGroupKind,
} from "./profileGroups";
import {
  CreateProfileModal,
  StartInstanceModal,
} from "../components/molecules";
import ProfileDetailsPanel from "../profiles/ProfileDetailsPanel";
import { useProfilesController, getProfileKey } from "./useProfilesController";

const kindBadge: Partial<
  Record<
    ProfileGroupKind,
    "profilesPage.badgeTemporary" | "profilesPage.badgeQuarantined"
  >
> = {
  temporary: "profilesPage.badgeTemporary",
  quarantined: "profilesPage.badgeQuarantined",
};

function ProfileRow({
  profile,
  kind,
  instance,
  selected,
  onSelect,
}: {
  profile: Profile;
  kind: ProfileGroupKind;
  instance?: Instance;
  selected: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  const accountText =
    profile.accountEmail || profile.accountName || t("profiles.noAccount");
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
        ? t("profiles.statusError")
        : t("profiles.statusStopped");
  const badge = kindBadge[kind];

  return (
    <button
      type="button"
      onClick={onSelect}
      className={`w-full border-b border-border-subtle px-3 py-2.5 text-left transition-colors ${
        selected ? "bg-bg-hover text-text-primary" : "hover:bg-bg-hover/50"
      }`}
    >
      <div className="flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="truncate text-sm font-semibold text-text-primary">
            {profile.name}
          </div>
          <div className="mt-1 text-xs text-text-muted">{accountText}</div>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          {badge && <Badge variant="warning">{t(badge)}</Badge>}
          <Badge variant={statusVariant}>{statusLabel}</Badge>
        </div>
      </div>

      {profile.useWhen && (
        <div className="mt-3 line-clamp-2 text-xs leading-5 text-text-secondary">
          {profile.useWhen}
        </div>
      )}
    </button>
  );
}

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
    deleteError,
    deleteNotice,
    handleSave,
  } = useProfilesController();
  const { t } = useTranslation();
  const [showCreate, setShowCreate] = useState(false);
  const groups = groupProfiles(orderedProfiles);

  return (
    <div className="flex h-full flex-col">
      <div className="flex flex-1 flex-col overflow-hidden">
        <div className="h-full">
          {profilesLoading && profiles.length === 0 ? (
            <div className="flex items-center justify-center py-16 text-text-muted">
              {t("profilesPage.loading")}
            </div>
          ) : profiles.length === 0 ? (
            <EmptyState
              title={t("profilesPage.emptyTitle")}
              description={t("profilesPage.emptyDescription")}
              action={
                <Button variant="primary" onClick={() => setShowCreate(true)}>
                  {t("profilesPage.newProfile")}
                </Button>
              }
            />
          ) : (
            <div className="dashboard-panel flex h-full min-h-0 flex-col overflow-hidden rounded-none! border-t-0 lg:flex-row">
              <div className="flex max-h-88 w-full shrink-0 flex-col overflow-hidden border-r border-border-subtle bg-bg-surface/50 lg:max-h-none lg:w-80">
                <div className="flex items-center justify-between border-b border-border-subtle px-4 py-2.5">
                  <span className="text-xs font-medium text-text-muted">
                    {t("profilesPage.heading")}
                  </span>
                  <button
                    type="button"
                    onClick={() => setShowCreate(true)}
                    className="rounded bg-primary px-2.5 py-1 text-xs font-medium text-white transition-colors hover:bg-primary/90"
                  >
                    {t("profilesPage.newProfile")}
                  </button>
                </div>

                <div className="flex-1 overflow-auto">
                  {groupOrder.map(({ kind, labelKey }) => {
                    const rows = groups[kind];
                    if (rows.length === 0) {
                      return null;
                    }
                    return (
                      <div key={kind}>
                        {kind !== "user" && (
                          <div
                            className="flex items-baseline justify-between border-b border-border-subtle bg-bg-surface px-3 py-1.5 text-xs text-text-muted"
                            data-testid={`profile-group-${kind}`}
                          >
                            <span className="font-medium uppercase tracking-[0.08em]">
                              {t(labelKey)} ({rows.length})
                            </span>
                            <span>
                              {t("profilesPage.groupTotal", {
                                size: formatProfileBytes(groupBytes(rows)),
                              })}
                            </span>
                          </div>
                        )}
                        {rows.map((profile) => (
                          <ProfileRow
                            key={getProfileKey(profile)}
                            profile={profile}
                            kind={kind}
                            instance={instanceByProfile.get(profile.name)}
                            selected={
                              getProfileKey(profile) === selectedProfileKey
                            }
                            onSelect={() =>
                              setSelectedProfileKey(getProfileKey(profile))
                            }
                          />
                        ))}
                      </div>
                    );
                  })}
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
                  deleteError={deleteError}
                  deleteNotice={deleteNotice}
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
