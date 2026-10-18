import type { BackendConfig } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
import { useTranslation } from "react-i18next";
import { fieldClass, timeoutRows } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface TimeoutsSettingsSectionProps {
  backendConfig: BackendConfig;
  updateBackendSection: UpdateBackendSection;
}

export function TimeoutsSettingsSection({
  backendConfig,
  updateBackendSection,
}: TimeoutsSettingsSectionProps) {
  const { t } = useTranslation();

  return (
    <SectionCard
      title={t("settings.sections.timeouts.label")}
      description={t("settings.panels.timeouts.description")}
    >
      {timeoutRows.map(([key, labelKey, descriptionKey]) => (
        <SettingRow
          key={key}
          label={t(labelKey)}
          description={t(descriptionKey)}
        >
          <input
            type="number"
            min={0}
            value={backendConfig.timeouts[key]}
            onChange={(e) =>
              updateBackendSection("timeouts", {
                [key]: Number(e.target.value),
              } as Partial<BackendConfig["timeouts"]>)
            }
            className={fieldClass}
          />
        </SettingRow>
      ))}
    </SectionCard>
  );
}
