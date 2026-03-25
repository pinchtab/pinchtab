import type { BackendConfig } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
import { csvToList, fieldClass, listToCsv } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface BrowserSettingsSectionProps {
  backendConfig: BackendConfig;
  updateBackendSection: UpdateBackendSection;
}

export function BrowserSettingsSection({
  backendConfig,
  updateBackendSection,
}: BrowserSettingsSectionProps) {
  return (
    <SectionCard
      title="Browser Runtime"
      description="These settings are written into the generated child config for new managed instances."
    >
      <SettingRow
        label="Chrome version"
        description="Version string used in generated UA/fingerprint defaults."
      >
        <input
          value={backendConfig.browser.version}
          onChange={(e) =>
            updateBackendSection("browser", {
              version: e.target.value,
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label="Chrome binary"
        description="Optional path override for the Chrome executable."
      >
        <input
          value={backendConfig.browser.binary}
          onChange={(e) =>
            updateBackendSection("browser", {
              binary: e.target.value,
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label="Extra flags"
        description="Additional Chrome flags appended when launching managed instances."
      >
        <input
          value={backendConfig.browser.extraFlags}
          onChange={(e) =>
            updateBackendSection("browser", {
              extraFlags: e.target.value,
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label="Extension paths"
        description="Comma-separated extension directories to load."
      >
        <input
          value={listToCsv(backendConfig.browser.extensionPaths)}
          onChange={(e) =>
            updateBackendSection("browser", {
              extensionPaths: csvToList(e.target.value),
            })
          }
          className={fieldClass}
        />
      </SettingRow>
    </SectionCard>
  );
}
