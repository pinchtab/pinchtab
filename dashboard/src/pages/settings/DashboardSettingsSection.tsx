import type { Dispatch, SetStateAction } from "react";
import { useTranslation } from "react-i18next";
import { Select } from "../../components/atoms";
import type { LocalDashboardSettings } from "../../types";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface DashboardSettingsSectionProps {
  localSettings: LocalDashboardSettings;
  setLocalSettings: Dispatch<SetStateAction<LocalDashboardSettings>>;
}

export function DashboardSettingsSection({
  localSettings,
  setLocalSettings,
}: DashboardSettingsSectionProps) {
  const { t } = useTranslation();

  return (
    <SectionCard
      title={t("settings.panels.dashboard.title")}
      description={t("settings.panels.dashboard.description")}
    >
      <SettingRow
        label={t("settings.dashboard.screencastFps.label")}
        description={t("settings.dashboard.screencastFps.description")}
      >
        <div className="flex items-center gap-3">
          <input
            type="range"
            min={1}
            max={15}
            value={localSettings.screencast.fps}
            onChange={(e) =>
              setLocalSettings((current) => ({
                ...current,
                screencast: {
                  ...current.screencast,
                  fps: Number(e.target.value),
                },
              }))
            }
            className="w-full"
          />
          <span className="dashboard-mono w-16 text-right text-sm text-text-secondary">
            {localSettings.screencast.fps} fps
          </span>
        </div>
      </SettingRow>
      <SettingRow
        label={t("settings.dashboard.screencastQuality.label")}
        description={t("settings.dashboard.screencastQuality.description")}
      >
        <div className="flex items-center gap-3">
          <input
            type="range"
            min={10}
            max={80}
            value={localSettings.screencast.quality}
            onChange={(e) =>
              setLocalSettings((current) => ({
                ...current,
                screencast: {
                  ...current.screencast,
                  quality: Number(e.target.value),
                },
              }))
            }
            className="w-full"
          />
          <span className="dashboard-mono w-16 text-right text-sm text-text-secondary">
            {localSettings.screencast.quality}%
          </span>
        </div>
      </SettingRow>
      <SettingRow
        label={t("settings.dashboard.screencastMaxWidth.label")}
        description={t("settings.dashboard.screencastMaxWidth.description")}
      >
        <Select
          value={localSettings.screencast.maxWidth}
          onChange={(e) =>
            setLocalSettings((current) => ({
              ...current,
              screencast: {
                ...current.screencast,
                maxWidth: Number(e.target.value),
              },
            }))
          }
        >
          {[400, 600, 800, 1024, 1280].map((width) => (
            <option key={width} value={width}>
              {width}px
            </option>
          ))}
        </Select>
      </SettingRow>
      <SettingRow
        label={t("settings.dashboard.memoryMetrics.label")}
        description={t("settings.dashboard.memoryMetrics.description")}
      >
        <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={localSettings.monitoring.memoryMetrics}
            onChange={(e) =>
              setLocalSettings((current) => ({
                ...current,
                monitoring: {
                  ...current.monitoring,
                  memoryMetrics: e.target.checked,
                },
              }))
            }
            className="h-4 w-4"
          />
          {t("common.enable")}
        </label>
      </SettingRow>
      <SettingRow
        label={t("settings.dashboard.pollInterval.label")}
        description={t("settings.dashboard.pollInterval.description")}
      >
        <div className="flex items-center gap-3">
          <input
            type="range"
            min={5}
            max={120}
            step={5}
            value={localSettings.monitoring.pollInterval}
            onChange={(e) =>
              setLocalSettings((current) => ({
                ...current,
                monitoring: {
                  ...current.monitoring,
                  pollInterval: Number(e.target.value),
                },
              }))
            }
            className="w-full"
          />
          <span className="dashboard-mono w-16 text-right text-sm text-text-secondary">
            {localSettings.monitoring.pollInterval}s
          </span>
        </div>
      </SettingRow>
      <SettingRow
        label={t("settings.dashboard.reasoningOutput.label")}
        description={t("settings.dashboard.reasoningOutput.description")}
      >
        <Select
          value={localSettings.agents.reasoningMode}
          onChange={(e) =>
            setLocalSettings((current) => ({
              ...current,
              agents: {
                ...current.agents,
                reasoningMode: e.target.value as
                  | "tool_calls"
                  | "progress"
                  | "both",
              },
            }))
          }
        >
          <option value="tool_calls">
            {t("settings.dashboard.reasoningOutput.toolCallsOnly")}
          </option>
          <option value="progress">
            {t("settings.dashboard.reasoningOutput.progressOnly")}
          </option>
          <option value="both">
            {t("settings.dashboard.reasoningOutput.both")}
          </option>
        </Select>
      </SettingRow>
    </SectionCard>
  );
}
