import type { BackendConfig, BackendConfigState } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
import { csvToList, fieldClass, listToCsv } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface NetworkSettingsSectionProps {
  apiTokenMissing: boolean;
  attachWildcard: boolean;
  backendConfig: BackendConfig;
  backendState: BackendConfigState | null;
  nonLoopbackBind: boolean;
  updateBackendSection: UpdateBackendSection;
}

export function NetworkSettingsSection({
  apiTokenMissing,
  attachWildcard,
  backendConfig,
  backendState,
  nonLoopbackBind,
  updateBackendSection,
}: NetworkSettingsSectionProps) {
  return (
    <SectionCard
      title="Network & Attach"
      description="Port and bind changes require a restart. API token management is handled outside the dashboard."
    >
      <SettingRow
        label="Server port"
        description="HTTP port for the dashboard process."
      >
        <input
          value={backendConfig.server.port}
          onChange={(e) =>
            updateBackendSection("server", { port: e.target.value })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label="Bind address"
        description="Network interface the dashboard process binds to. Keeping 127.0.0.1 or localhost limits direct reachability to the local machine."
      >
        <div className="space-y-2">
          <input
            value={backendConfig.server.bind}
            onChange={(e) =>
              updateBackendSection("server", {
                bind: e.target.value,
              })
            }
            className={fieldClass}
          />
          {nonLoopbackBind ? (
            <div className="rounded-sm border border-destructive/35 bg-destructive/10 px-3 py-2 text-xs leading-5 text-destructive/80">
              A non-loopback bind is a documented, non-default,
              security-reducing configuration change. It may expose the server
              beyond the local machine unless another network boundary still
              restricts access. Keep a token set and review proxy or
              port-publishing behavior explicitly.
            </div>
          ) : (
            <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
              Loopback bind keeps direct server reachability local. Moving to{" "}
              <code>0.0.0.0</code> or another non-local address widens the trust
              boundary.
            </div>
          )}
        </div>
      </SettingRow>
      <SettingRow
        label="API token"
        description="Bearer token required by authenticated requests when set. The dashboard never returns it and does not manage it."
      >
        <div className="space-y-2">
          <div className="text-xs leading-5 text-text-muted">
            {backendState?.tokenConfigured
              ? "Token configured. Manage rotation through the CLI or config file; the current value is never returned by the server."
              : "No token configured. Set one through the CLI or config file."}
          </div>
          {apiTokenMissing && (
            <div className="rounded-sm border border-destructive/35 bg-destructive/10 px-3 py-2 text-xs leading-5 text-destructive">
              No API token is set. Anyone who can reach this server can access
              exposed endpoints. Keep it on trusted local networks only, or
              configure a strong token through the CLI or config file. You are
              responsible for protecting access.
            </div>
          )}
        </div>
      </SettingRow>
      <SettingRow
        label="State directory"
        description="Base state path used by managed child instances."
      >
        <input
          value={backendConfig.server.stateDir}
          onChange={(e) =>
            updateBackendSection("server", {
              stateDir: e.target.value,
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label="Trust proxy headers"
        description="Trust X-Forwarded-Proto, X-Forwarded-Host, and Forwarded headers for origin checks. Enable only when PinchTab runs behind a trusted reverse proxy (e.g. Caddy, nginx)."
      >
        <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={backendConfig.server.trustProxyHeaders ?? false}
            onChange={(e) =>
              updateBackendSection("server", {
                trustProxyHeaders: e.target.checked,
              })
            }
            className="accent-primary"
          />
          {backendConfig.server.trustProxyHeaders ? "Enabled" : "Disabled"}
        </label>
      </SettingRow>
      <SettingRow
        label="Allow attach"
        description="Permit attaching PinchTab to externally managed Chrome sessions."
      >
        <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={backendConfig.security.attach.enabled}
            onChange={(e) =>
              updateBackendSection("security", {
                attach: {
                  ...backendConfig.security.attach,
                  enabled: e.target.checked,
                },
              })
            }
            className="h-4 w-4"
          />
          Enable
        </label>
      </SettingRow>
      <SettingRow
        label="Allowed attach hosts"
        description='Comma-separated host allowlist for attach requests. Only include hosts you control and trust. Using "*" disables host allowlisting.'
      >
        <div className="space-y-2">
          <input
            value={listToCsv(backendConfig.security.attach.allowHosts)}
            onChange={(e) =>
              updateBackendSection("security", {
                attach: {
                  ...backendConfig.security.attach,
                  allowHosts: csvToList(e.target.value),
                },
              })
            }
            className={fieldClass}
          />
          {attachWildcard ? (
            <div className="rounded-sm border border-destructive/35 bg-destructive/10 px-3 py-2 text-xs leading-5 text-destructive/80">
              <code>allowHosts: ["*"]</code> is a documented, non-default,
              security-reducing override. It disables host allowlisting entirely
              and allows remote attach requests to any reachable host with an
              allowed scheme. Use it only on isolated, operator-controlled
              networks.
            </div>
          ) : (
            <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
              Hosts in this allowlist may be used for remote attach requests.
              Broad or untrusted entries expand the trust boundary and can
              expose external Chrome sessions and browser contents.
            </div>
          )}
        </div>
      </SettingRow>
      <SettingRow
        label="Allowed attach schemes"
        description="Comma-separated scheme allowlist, usually ws and wss."
      >
        <input
          value={listToCsv(backendConfig.security.attach.allowSchemes)}
          onChange={(e) =>
            updateBackendSection("security", {
              attach: {
                ...backendConfig.security.attach,
                allowSchemes: csvToList(e.target.value),
              },
            })
          }
          className={fieldClass}
        />
      </SettingRow>
    </SectionCard>
  );
}
