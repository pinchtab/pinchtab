import type { BackendConfig } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
import { useTranslation } from "react-i18next";
import {
  csvToList,
  fieldClass,
  idpiToggleRows,
  listToCsv,
} from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface SecurityIdpiSettingsSectionProps {
  backendConfig: BackendConfig;
  idpiEnabled: boolean;
  idpiDomainsConfigured: boolean;
  idpiWildcard: boolean;
  updateBackendSection: UpdateBackendSection;
}

export function SecurityIdpiSettingsSection({
  backendConfig,
  idpiEnabled,
  idpiDomainsConfigured,
  idpiWildcard,
  updateBackendSection,
}: SecurityIdpiSettingsSectionProps) {
  const { t } = useTranslation();
  return (
    <SectionCard
      title={t("settings.sections.security-idpi.label")}
      description={t("settings.panels.security-idpi.description")}
    >
      <div
        className={`mb-4 rounded-sm px-4 py-3 text-sm leading-6 ${
          !idpiEnabled || !idpiDomainsConfigured
            ? "border border-destructive/35 bg-destructive/10 text-destructive"
            : idpiWildcard
              ? "border border-warning/25 bg-warning/10 text-warning"
              : "border border-success/25 bg-success/10 text-success"
        }`}
      >
        {!idpiEnabled
          ? t("settings.idpi.banner.disabled")
          : !idpiDomainsConfigured
            ? t("settings.idpi.banner.noDomains")
            : idpiWildcard
              ? t("settings.idpi.banner.wildcard")
              : t("settings.idpi.banner.enforcing")}
      </div>
      {idpiToggleRows.map(([key, labelKey, descriptionKey]) => (
        <SettingRow
          key={key}
          label={t(labelKey)}
          description={t(descriptionKey)}
        >
          <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
            <input
              type="checkbox"
              checked={backendConfig.security.idpi[key]}
              onChange={(e) =>
                updateBackendSection("security", {
                  idpi: {
                    ...backendConfig.security.idpi,
                    [key]: e.target.checked,
                  },
                })
              }
              className="h-4 w-4"
            />
            {t("common.enable")}
          </label>
        </SettingRow>
      ))}
      <SettingRow
        label={t("settings.idpi.customPatterns.label")}
        description={t("settings.idpi.customPatterns.description")}
      >
        <input
          value={listToCsv(backendConfig.security.idpi.customPatterns)}
          onChange={(e) =>
            updateBackendSection("security", {
              idpi: {
                ...backendConfig.security.idpi,
                customPatterns: csvToList(e.target.value),
              },
            })
          }
          className={fieldClass}
          placeholder={t("settings.idpi.customPatterns.placeholder")}
        />
      </SettingRow>
    </SectionCard>
  );
}
