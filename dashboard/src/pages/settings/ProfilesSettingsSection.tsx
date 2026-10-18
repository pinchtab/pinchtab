import type { BackendConfig } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
import { fieldClass } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";
import { useTranslation } from "react-i18next";

interface ProfilesSettingsSectionProps {
  backendConfig: BackendConfig;
  updateBackendSection: UpdateBackendSection;
}

export function ProfilesSettingsSection({
  backendConfig,
  updateBackendSection,
}: ProfilesSettingsSectionProps) {
  const { t } = useTranslation();

  return (
    <SectionCard
      title={t("settings.sections.profiles.label")}
      description={t("settings.panels.profiles.description")}
    >
      <SettingRow
        label={t("settings.profiles.baseDir.label")}
        description={t("settings.profiles.baseDir.description")}
      >
        <input
          value={backendConfig.profiles.baseDir}
          onChange={(e) =>
            updateBackendSection("profiles", {
              baseDir: e.target.value,
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.profiles.defaultProfile.label")}
        description={t("settings.profiles.defaultProfile.description")}
      >
        <input
          value={backendConfig.profiles.defaultProfile}
          onChange={(e) =>
            updateBackendSection("profiles", {
              defaultProfile: e.target.value,
            })
          }
          className={fieldClass}
        />
      </SettingRow>
    </SectionCard>
  );
}
