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
      <SettingRow
        label="Allowed websites"
        description="Comma-separated domain allowlist for web content. Use exact hosts or patterns like *.example.com."
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
            placeholder="127.0.0.1, localhost, ::1"
          />
          <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
            Keep this list narrow. Empty or wildcard entries weaken the main
            IDPI boundary. Allowing non-local or non-trusted sites increases
            browser attack surface even when IDPI is enabled.
          </div>
        </div>
      </SettingRow>
      <SettingRow
        label="Trusted proxy CIDRs"
        description="Comma-separated CIDRs or IPs whose browser-reported remote IP should be trusted during navigation. Use this only for known internal proxies."
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
            placeholder="10.1.2.3, 10.0.0.0/8"
          />
          <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
            This weakens navigation IP checks for matching remote IPs. Prefer
            specific proxy addresses over broad private ranges. Bare IP entries
            are treated as single hosts.
          </div>
        </div>
      </SettingRow>
      <SettingRow
        label="Trusted resolve CIDRs"
        description="Comma-separated CIDRs or IPs that a hostname may resolve to during navigation preflight. This is intended for internal DNS or proxy setups."
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
            placeholder="198.18.0.0/15, 10.1.2.3"
          />
          <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
            This allows hostnames to resolve to non-public IPs. Keep the list
            narrow and only include infrastructure you control. Bare IP entries
            are treated as single hosts.
          </div>
        </div>
      </SettingRow>
    </SectionCard>
  );
}
