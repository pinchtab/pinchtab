import { Select } from "../../components/atoms";
import type { BackendConfig, BackendConfigState } from "../../types";
import { Trans, useTranslation } from "react-i18next";
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
  const { t } = useTranslation();

  return (
    <SectionCard
      title={t("settings.sections.network.label")}
      description={t("settings.panels.network.description")}
    >
      <SettingRow
        label={t("settings.network.serverPort.label")}
        description={t("settings.network.serverPort.description")}
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
        label={t("settings.network.bindAddress.label")}
        description={t("settings.network.bindAddress.description")}
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
              {t("settings.network.bindAddress.nonLoopbackNote")}
            </div>
          ) : (
            <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
              <Trans
                i18nKey="settings.network.bindAddress.loopbackNote"
                components={{ code: <code /> }}
              />
            </div>
          )}
        </div>
      </SettingRow>
      <SettingRow
        label={t("settings.network.apiToken.label")}
        description={t("settings.network.apiToken.description")}
      >
        <div className="space-y-2">
          <div className="text-xs leading-5 text-text-muted">
            {backendState?.tokenConfigured ? (
              <Trans
                i18nKey="settings.network.apiToken.tokenConfigured"
                components={{
                  code: (
                    <code className="rounded bg-[rgb(var(--brand-surface-code-rgb)/0.72)] px-1 py-0.5 text-text-secondary" />
                  ),
                }}
              />
            ) : (
              t("settings.network.apiToken.noToken")
            )}
          </div>
          {apiTokenMissing && (
            <div className="rounded-sm border border-destructive/35 bg-destructive/10 px-3 py-2 text-xs leading-5 text-destructive">
              {t("settings.network.apiToken.missingWarning")}
            </div>
          )}
        </div>
      </SettingRow>
      <SettingRow
        label={t("settings.network.stateDirectory.label")}
        description={t("settings.network.stateDirectory.description")}
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
        label={t("settings.network.trustProxyHeaders.label")}
        description={t("settings.network.trustProxyHeaders.description")}
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
          {backendConfig.server.trustProxyHeaders
            ? t("common.enabled")
            : t("common.disabled")}
        </label>
      </SettingRow>
      <SettingRow
        label={t("settings.network.cookieSecure.label")}
        description={t("settings.network.cookieSecure.description")}
      >
        <div className="space-y-2">
          <Select
            value={
              backendConfig.server.cookieSecure === true
                ? "true"
                : backendConfig.server.cookieSecure === false
                  ? "false"
                  : "auto"
            }
            onChange={(e) =>
              updateBackendSection("server", {
                cookieSecure:
                  e.target.value === "auto"
                    ? undefined
                    : e.target.value === "true",
              })
            }
          >
            <option value="auto">
              {t("settings.network.cookieSecure.auto")}
            </option>
            <option value="true">
              {t("settings.network.cookieSecure.forceSecure")}
            </option>
            <option value="false">
              {t("settings.network.cookieSecure.forceInsecure")}
            </option>
          </Select>
          <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
            <Trans
              i18nKey="settings.network.cookieSecure.note"
              components={{ code: <code /> }}
            />
          </div>
        </div>
      </SettingRow>
      <SettingRow
        label={t("settings.network.persistSessions.label")}
        description={t("settings.network.persistSessions.description")}
      >
        <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={backendConfig.sessions.dashboard.persist}
            onChange={(e) =>
              updateBackendSection("sessions", {
                dashboard: {
                  ...backendConfig.sessions.dashboard,
                  persist: e.target.checked,
                },
              })
            }
            className="accent-primary"
          />
          {backendConfig.sessions.dashboard.persist
            ? t("common.enabled")
            : t("common.disabled")}
        </label>
      </SettingRow>
      <SettingRow
        label={t("settings.network.sessionIdleTimeout.label")}
        description={t("settings.network.sessionIdleTimeout.description")}
      >
        <input
          type="number"
          min={60}
          step={60}
          value={backendConfig.sessions.dashboard.idleTimeoutSec}
          onChange={(e) =>
            updateBackendSection("sessions", {
              dashboard: {
                ...backendConfig.sessions.dashboard,
                idleTimeoutSec: Number(e.target.value),
              },
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.network.sessionMaxLifetime.label")}
        description={t("settings.network.sessionMaxLifetime.description")}
      >
        <input
          type="number"
          min={60}
          step={60}
          value={backendConfig.sessions.dashboard.maxLifetimeSec}
          onChange={(e) =>
            updateBackendSection("sessions", {
              dashboard: {
                ...backendConfig.sessions.dashboard,
                maxLifetimeSec: Number(e.target.value),
              },
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.network.requireElevation.label")}
        description={t("settings.network.requireElevation.description")}
      >
        <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={backendConfig.sessions.dashboard.requireElevation}
            onChange={(e) =>
              updateBackendSection("sessions", {
                dashboard: {
                  ...backendConfig.sessions.dashboard,
                  requireElevation: e.target.checked,
                },
              })
            }
            className="accent-primary"
          />
          {backendConfig.sessions.dashboard.requireElevation
            ? t("common.enabled")
            : t("common.disabled")}
        </label>
      </SettingRow>
      <SettingRow
        label={t("settings.network.allowAttach.label")}
        description={t("settings.network.allowAttach.description")}
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
          {t("common.enable")}
        </label>
      </SettingRow>
      <SettingRow
        label={t("settings.network.allowHosts.label")}
        description={t("settings.network.allowHosts.description")}
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
              <Trans
                i18nKey="settings.network.allowHosts.wildcardNote"
                components={{ code: <code /> }}
              />
            </div>
          ) : (
            <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
              {t("settings.network.allowHosts.note")}
            </div>
          )}
        </div>
      </SettingRow>
      <SettingRow
        label={t("settings.network.allowSchemes.label")}
        description={t("settings.network.allowSchemes.description")}
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
