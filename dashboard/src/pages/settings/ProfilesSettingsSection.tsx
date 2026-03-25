import type { BackendConfig } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
import { fieldClass } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface ProfilesSettingsSectionProps {
  backendConfig: BackendConfig;
  updateBackendSection: UpdateBackendSection;
}

export function ProfilesSettingsSection({
  backendConfig,
  updateBackendSection,
}: ProfilesSettingsSectionProps) {
  return (
    <SectionCard
      title="Profiles"
      description="Profile storage is host-level. Changing the base directory requires restart because the profile manager and orchestrator are created with it at boot."
    >
      <SettingRow
        label="Profiles base directory"
        description="Root directory where browser profiles are stored."
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
        label="Default profile"
        description="Profile name used when the server needs an implicit default."
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
