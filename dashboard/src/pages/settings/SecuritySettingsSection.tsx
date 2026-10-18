import type { BackendConfig, BackendSecurityConfig } from "../../types";
import type {
  SecurityEndpointKey,
  UpdateBackendSection,
} from "./settingsShared";
import {
  csvToList,
  fieldClass,
  listToCsv,
  securityEndpointRows,
} from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";
import { useTranslation } from "react-i18next";

interface SecuritySettingsSectionProps {
  backendConfig: BackendConfig;
  sensitiveEndpointsEnabled: boolean;
  updateBackendSection: UpdateBackendSection;
}

export function SecuritySettingsSection({
  backendConfig,
  sensitiveEndpointsEnabled,
  updateBackendSection,
}: SecuritySettingsSectionProps) {
  const { t } = useTranslation();

  return (
    <SectionCard
      title={t("settings.sections.security.label")}
      description={t("settings.panels.security.description")}
    >
      <div
        className={`rounded-sm px-4 py-3 text-sm leading-6 ${
          sensitiveEndpointsEnabled
            ? "border border-destructive/35 bg-destructive/10 text-destructive"
            : "border border-warning/25 bg-warning/10 text-warning"
        }`}
      >
        {sensitiveEndpointsEnabled
          ? t("settings.security.banner.risky")
          : t("settings.security.banner.warning")}
      </div>
      {securityEndpointRows.map((row) => {
        const [key, labelKey] = row;
        const descriptionKey =
          row.length > 2 && typeof row[2] === "string"
            ? row[2]
            : "settings.endpointFallbackDescription";
        return (
          <SettingRow
            key={key}
            label={t(labelKey)}
            description={t(descriptionKey)}
          >
            <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
              <input
                type="checkbox"
                checked={backendConfig.security[key]}
                onChange={(e) =>
                  updateBackendSection("security", {
                    [key]: e.target.checked,
                  } as Partial<
                    Pick<BackendSecurityConfig, SecurityEndpointKey>
                  >)
                }
                className="h-4 w-4"
              />
              {t("common.enable")}
            </label>
          </SettingRow>
        );
      })}
      <SettingRow
        label={t("settings.security.allowedDomains.label")}
        description={t("settings.security.allowedDomains.description")}
      >
        <div className="space-y-2">
          <input
            value={listToCsv(backendConfig.security.allowedDomains)}
            onChange={(e) =>
              updateBackendSection("security", {
                allowedDomains: csvToList(e.target.value),
              })
            }
            className={fieldClass}
            placeholder={t("settings.security.allowedDomains.placeholder")}
          />
          <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
            {t("settings.security.allowedDomains.note")}
          </div>
        </div>
      </SettingRow>
      <SettingRow
        label={t("settings.security.trustedProxyCIDRs.label")}
        description={t("settings.security.trustedProxyCIDRs.description")}
      >
        <div className="space-y-2">
          <input
            value={listToCsv(backendConfig.security.trustedProxyCIDRs)}
            onChange={(e) =>
              updateBackendSection("security", {
                trustedProxyCIDRs: csvToList(e.target.value),
              } as Partial<Pick<BackendSecurityConfig, "trustedProxyCIDRs">>)
            }
            className={fieldClass}
            placeholder={t("settings.security.trustedProxyCIDRs.placeholder")}
          />
          <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
            {t("settings.security.trustedProxyCIDRs.note")}
          </div>
        </div>
      </SettingRow>
      <SettingRow
        label={t("settings.security.trustedResolveCIDRs.label")}
        description={t("settings.security.trustedResolveCIDRs.description")}
      >
        <div className="space-y-2">
          <input
            value={listToCsv(backendConfig.security.trustedResolveCIDRs)}
            onChange={(e) =>
              updateBackendSection("security", {
                trustedResolveCIDRs: csvToList(e.target.value),
              } as Partial<Pick<BackendSecurityConfig, "trustedResolveCIDRs">>)
            }
            className={fieldClass}
            placeholder={t("settings.security.trustedResolveCIDRs.placeholder")}
          />
          <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
            {t("settings.security.trustedResolveCIDRs.note")}
          </div>
        </div>
      </SettingRow>
    </SectionCard>
  );
}
