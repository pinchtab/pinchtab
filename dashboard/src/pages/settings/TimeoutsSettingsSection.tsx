import type { BackendConfig } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
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
  return (
    <SectionCard
      title="Timeouts"
      description="Runtime timing defaults written into new child configs. Existing running instances keep their current timeouts."
    >
      {timeoutRows.map(([key, label, description]) => (
        <SettingRow key={key} label={label} description={description}>
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
