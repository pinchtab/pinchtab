import type { BackendConfig, BackendSecurityConfig } from "../../types";
import type {
  SecurityEndpointKey,
  UpdateBackendSection,
} from "./settingsShared";
import { securityEndpointRows } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

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
  return (
    <SectionCard
      title="Security"
      description="These controls define what risky capabilities PinchTab exposes."
    >
      <div
        className={`rounded-sm px-4 py-3 text-sm leading-6 ${
          sensitiveEndpointsEnabled
            ? "border border-destructive/35 bg-destructive/10 text-destructive"
            : "border border-warning/25 bg-warning/10 text-warning"
        }`}
      >
        {sensitiveEndpointsEnabled
          ? "One or more sensitive endpoint families are enabled. Features like script execution, downloads, uploads, and live capture can expose high-risk capabilities. Only enable them in trusted environments. You are responsible for securing network access, authentication, and downstream use."
          : "These endpoint families can expose high-risk capabilities when enabled. Only turn them on in trusted environments, and only when you accept responsibility for network access, authentication, and downstream use."}
      </div>
      {securityEndpointRows.map(([key, label]) => (
        <SettingRow
          key={key}
          label={label}
          description="Controls whether the corresponding endpoint family is enabled."
        >
          <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
            <input
              type="checkbox"
              checked={backendConfig.security[key]}
              onChange={(e) =>
                updateBackendSection("security", {
                  [key]: e.target.checked,
                } as Partial<Pick<BackendSecurityConfig, SecurityEndpointKey>>)
              }
              className="h-4 w-4"
            />
            Enable
          </label>
        </SettingRow>
      ))}
    </SectionCard>
  );
}
