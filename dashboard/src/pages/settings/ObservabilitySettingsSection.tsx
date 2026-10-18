import type { BackendConfig } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
import { fieldClass } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";
import { useTranslation } from "react-i18next";

interface ObservabilitySettingsSectionProps {
  backendConfig: BackendConfig;
  updateBackendSection: UpdateBackendSection;
}

export function ObservabilitySettingsSection({
  backendConfig,
  updateBackendSection,
}: ObservabilitySettingsSectionProps) {
  const { t } = useTranslation();
  const activity = backendConfig.observability.activity;

  const updateActivity = (
    patch: Partial<BackendConfig["observability"]["activity"]>,
  ) => {
    updateBackendSection("observability", {
      activity: { ...activity, ...patch },
    });
  };

  return (
    <SectionCard
      title={t("settings.sections.observability.label")}
      description={t("settings.panels.observability.description")}
    >
      <SettingRow
        label={t("settings.observability.activityLogging.label")}
        description={t("settings.observability.activityLogging.description")}
      >
        <label className="flex cursor-pointer items-center gap-3">
          <input
            type="checkbox"
            checked={activity.enabled}
            onChange={(e) => updateActivity({ enabled: e.target.checked })}
            className="h-4 w-4 rounded border-border-subtle bg-bg-elevated text-primary focus:ring-primary/50"
          />
          <span className="text-sm text-text-secondary">
            {activity.enabled ? t("common.enabled") : t("common.disabled")}
          </span>
        </label>
      </SettingRow>

      <SettingRow
        label={t("settings.observability.retentionDays.label")}
        description={t("settings.observability.retentionDays.description")}
      >
        <input
          type="number"
          min={1}
          max={365}
          value={activity.retentionDays}
          onChange={(e) =>
            updateActivity({ retentionDays: Number(e.target.value) })
          }
          className={fieldClass}
        />
      </SettingRow>

      <SettingRow
        label={t("settings.observability.sessionIdleSec.label")}
        description={t("settings.observability.sessionIdleSec.description")}
      >
        <input
          type="number"
          min={60}
          value={activity.sessionIdleSec}
          onChange={(e) =>
            updateActivity({ sessionIdleSec: Number(e.target.value) })
          }
          className={fieldClass}
        />
      </SettingRow>
    </SectionCard>
  );
}
