import type { Dispatch, SetStateAction } from "react";
import type { LocalDashboardSettings } from "../../types";
import { selectClass } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface DashboardSettingsSectionProps {
  localSettings: LocalDashboardSettings;
  setLocalSettings: Dispatch<SetStateAction<LocalDashboardSettings>>;
}

export function DashboardSettingsSection({
  localSettings,
  setLocalSettings,
}: DashboardSettingsSectionProps) {
  return (
    <SectionCard
      title="Dashboard Preferences"
      description="These controls affect this dashboard UI only. They are stored locally in your browser and do not require a backend restart."
    >
      <SettingRow
        label="Screencast frame rate"
        description="Controls how often live previews request new frames."
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
        label="Screencast quality"
        description="JPEG quality for tab preview streams."
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
        label="Screencast width"
        description="Maximum preview width for live tiles."
      >
        <select
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
          className={selectClass}
        >
          {[400, 600, 800, 1024, 1280].map((width) => (
            <option key={width} value={width}>
              {width}px
            </option>
          ))}
        </select>
      </SettingRow>
      <SettingRow
        label="Memory metrics"
        description="Enable per-tab heap collection in the dashboard. Useful for debugging, but heavier."
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
          Enable
        </label>
      </SettingRow>
      <SettingRow
        label="Polling interval"
        description="How frequently the dashboard asks the backend for fresh metrics."
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
        label="Reasoning output"
        description="Choose whether the live agent feed shows tool calls, progress updates, or both."
      >
        <select
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
          className={selectClass}
        >
          <option value="tool_calls">Tool calls only</option>
          <option value="progress">Progress only</option>
          <option value="both">Both</option>
        </select>
      </SettingRow>
    </SectionCard>
  );
}
