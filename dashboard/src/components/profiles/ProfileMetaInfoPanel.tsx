import type { Profile, Instance } from "../../generated/types";

interface Props {
  profile: Profile;
  instance?: Instance;
}

function MetaBlock({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-xl border border-border-subtle bg-black/10 p-4">
      <div className="dashboard-section-title mb-2 text-[0.68rem]">{label}</div>
      {children}
    </div>
  );
}

export default function ProfileMetaInfoPanel({ profile, instance }: Props) {
  const accountText = profile.accountEmail || profile.accountName || "";
  const sizeText = profile.sizeMB ? `${profile.sizeMB.toFixed(0)} MB` : "—";
  const browserType = instance?.attached
    ? "Attached via CDP"
    : instance?.headless
      ? "Headless"
      : "Headed";

  return (
    <MetaBlock label="Profile panel">
      <div className="space-y-3 text-sm text-text-secondary">
        <div className="flex items-center justify-between gap-3">
          <span className="dashboard-section-title text-[0.68rem]">Status</span>
          <span className="text-right">{instance?.status || "stopped"}</span>
        </div>
        {instance?.port && (
          <div className="flex items-center justify-between gap-3">
            <span className="dashboard-section-title text-[0.68rem]">Port</span>
            <span className="text-right">{instance.port}</span>
          </div>
        )}
        <div className="flex items-center justify-between gap-3">
          <span className="dashboard-section-title text-[0.68rem]">
            Browser
          </span>
          <span className="text-right">{browserType}</span>
        </div>
        <div className="flex items-center justify-between gap-3">
          <span className="dashboard-section-title text-[0.68rem]">Size</span>
          <span className="text-right">{sizeText}</span>
        </div>
        {accountText && (
          <div className="flex items-center justify-between gap-3">
            <span className="dashboard-section-title text-[0.68rem]">
              Account
            </span>
            <span className="text-right">{accountText}</span>
          </div>
        )}
        {profile.chromeProfileName && (
          <div className="flex items-center justify-between gap-3">
            <span className="dashboard-section-title text-[0.68rem]">
              Identity
            </span>
            <span className="text-right">{profile.chromeProfileName}</span>
          </div>
        )}
        {instance?.attached && (
          <div className="flex items-center justify-between gap-3">
            <span className="dashboard-section-title text-[0.68rem]">
              Connection
            </span>
            <span className="text-right">CDP attached</span>
          </div>
        )}
        {instance?.cdpUrl && (
          <div>
            <div className="dashboard-section-title mb-1 text-[0.68rem]">
              CDP URL
            </div>
            <code className="dashboard-mono block break-all text-xs text-text-secondary">
              {instance.cdpUrl}
            </code>
          </div>
        )}
        {profile.path && (
          <div>
            <div className="dashboard-section-title mb-1 text-[0.68rem]">
              Path
            </div>
            <code
              className={`dashboard-mono block break-all text-xs ${
                profile.pathExists ? "text-text-secondary" : "text-destructive"
              }`}
            >
              {profile.path}
              {!profile.pathExists && " (not found)"}
            </code>
          </div>
        )}
      </div>
    </MetaBlock>
  );
}
