import type { BackendConfig } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
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
  return (
    <SectionCard
      title="Security IDPI"
      description="Indirect prompt injection controls restrict which websites are allowed and add protections around extracted content before it reaches downstream automation."
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
          ? "IDPI is disabled. Browser content is not being filtered by website allowlist or content protections."
          : !idpiDomainsConfigured
            ? "The website whitelist is not set to a restricted domain list. This is the main IDPI defense and should be configured."
            : idpiWildcard
              ? "The website whitelist contains '*', which effectively disables domain restriction."
              : "IDPI is enforcing a specific website whitelist and content protections."}
      </div>
      {idpiToggleRows.map(([key, label, description]) => (
        <SettingRow key={key} label={label} description={description}>
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
            Enable
          </label>
        </SettingRow>
      ))}
      <SettingRow
        label="Custom patterns"
        description="Optional comma-separated phrases to treat as suspicious prompt-injection content."
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
          placeholder="ignore previous instructions, exfiltrate data"
        />
      </SettingRow>
    </SectionCard>
  );
}
