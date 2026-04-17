import type { BackendConfig } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
import { fieldClass } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface ObservabilitySettingsSectionProps {
  backendConfig: BackendConfig;
  updateBackendSection: UpdateBackendSection;
}

export function ObservabilitySettingsSection({
  backendConfig,
  updateBackendSection,
}: ObservabilitySettingsSectionProps) {
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
      title="Observability"
      description="Activity logging tracks API requests for debugging and audit trails. Logs are stored locally and can be queried via the Activity page."
    >
      <SettingRow
        label="Activity logging"
        description="Enable or disable activity event recording."
      >
        <label className="flex cursor-pointer items-center gap-3">
          <input
            type="checkbox"
            checked={activity.enabled}
            onChange={(e) => updateActivity({ enabled: e.target.checked })}
            className="h-4 w-4 rounded border-border-subtle bg-bg-elevated text-primary focus:ring-primary/50"
          />
          <span className="text-sm text-text-secondary">
            {activity.enabled ? "Enabled" : "Disabled"}
          </span>
        </label>
      </SettingRow>

      <SettingRow
        label="Retention (days)"
        description="How long to keep activity logs before automatic cleanup. Longer retention uses more disk space but provides better audit history."
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
        label="Session idle timeout (seconds)"
        description="Time before an inactive agent session is considered idle. Used for grouping activity by session."
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
