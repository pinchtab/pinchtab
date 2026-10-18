import type { Profile, Instance } from "../generated/types";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
  const accountText = profile.accountEmail || profile.accountName || "";
  const sizeText = profile.sizeMB ? `${profile.sizeMB.toFixed(0)} MB` : "—";
  const browserEngine = instance?.browser || "chrome";
  const browserMode = instance?.attached
    ? t("profileMeta.attachedViaCdp")
    : instance?.headless
      ? t("profileMeta.headless")
      : t("profileMeta.headed");
  const browserType = `${browserEngine} / ${browserMode}`;

  return (
    <MetaBlock label={t("profileMeta.panel")}>
      <div className="space-y-3 text-sm text-text-secondary">
        <div className="flex items-center justify-between gap-3">
          <span className="dashboard-section-title text-[0.68rem]">
            {t("profileMeta.status")}
          </span>
          <span className="text-right">
            {instance?.status || t("profiles.statusStopped")}
          </span>
        </div>
        {instance?.port && (
          <div className="flex items-center justify-between gap-3">
            <span className="dashboard-section-title text-[0.68rem]">
              {t("profileMeta.port")}
            </span>
            <span className="text-right">{instance.port}</span>
          </div>
        )}
        <div className="flex items-center justify-between gap-3">
          <span className="dashboard-section-title text-[0.68rem]">
            {t("profileMeta.browser")}
          </span>
          <span className="text-right">{browserType}</span>
        </div>
        <div className="flex items-center justify-between gap-3">
          <span className="dashboard-section-title text-[0.68rem]">
            {t("profileMeta.size")}
          </span>
          <span className="text-right">{sizeText}</span>
        </div>
        {accountText && (
          <div className="flex items-center justify-between gap-3">
            <span className="dashboard-section-title text-[0.68rem]">
              {t("profileMeta.account")}
            </span>
            <span className="text-right">{accountText}</span>
          </div>
        )}
        {profile.chromeProfileName && (
          <div className="flex items-center justify-between gap-3">
            <span className="dashboard-section-title text-[0.68rem]">
              {t("profileMeta.identity")}
            </span>
            <span className="text-right">{profile.chromeProfileName}</span>
          </div>
        )}
        {instance?.attached && (
          <div className="flex items-center justify-between gap-3">
            <span className="dashboard-section-title text-[0.68rem]">
              {t("profileMeta.connection")}
            </span>
            <span className="text-right">{t("profileMeta.cdpAttached")}</span>
          </div>
        )}
        {instance?.cdpUrl && (
          <div>
            <div className="dashboard-section-title mb-1 text-[0.68rem]">
              {t("profileMeta.cdpUrl")}
            </div>
            <code className="dashboard-mono block break-all text-xs text-text-secondary">
              {instance.cdpUrl}
            </code>
          </div>
        )}
        {profile.path && (
          <div>
            <div className="dashboard-section-title mb-1 text-[0.68rem]">
              {t("profileMeta.path")}
            </div>
            <code
              className={`dashboard-mono block break-all text-xs ${
                profile.pathExists ? "text-text-secondary" : "text-destructive"
              }`}
            >
              {profile.path}
              {!profile.pathExists && t("profileMeta.notFound")}
            </code>
          </div>
        )}
      </div>
    </MetaBlock>
  );
}
