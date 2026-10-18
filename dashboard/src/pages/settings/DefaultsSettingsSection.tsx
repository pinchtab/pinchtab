import { Select } from "../../components/atoms";
import type { BackendConfig } from "../../types";
import { Trans, useTranslation } from "react-i18next";
import type { UpdateBackendSection } from "./settingsShared";
import { fieldClass, instanceDefaultsBooleanRows } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface DefaultsSettingsSectionProps {
  backendConfig: BackendConfig;
  updateBackendSection: UpdateBackendSection;
}

export function DefaultsSettingsSection({
  backendConfig,
  updateBackendSection,
}: DefaultsSettingsSectionProps) {
  const { t } = useTranslation();

  return (
    <SectionCard
      title={t("settings.sections.defaults.label")}
      description={t("settings.panels.defaults.description")}
    >
      <SettingRow
        label={t("settings.instanceDefaults.mode.label")}
        description={t("settings.instanceDefaults.mode.description")}
      >
        <Select
          value={backendConfig.instanceDefaults.mode}
          onChange={(e) =>
            updateBackendSection("instanceDefaults", {
              mode: e.target.value as BackendConfig["instanceDefaults"]["mode"],
            })
          }
        >
          <option value="headless">
            {t("settings.instanceDefaults.mode.headless")}
          </option>
          <option value="headed">
            {t("settings.instanceDefaults.mode.headed")}
          </option>
        </Select>
      </SettingRow>
      <SettingRow
        label={t("settings.instanceDefaults.stealthLevel.label")}
        description={t("settings.instanceDefaults.stealthLevel.description")}
      >
        <div className="space-y-2">
          <Select
            value={backendConfig.instanceDefaults.stealthLevel}
            onChange={(e) =>
              updateBackendSection("instanceDefaults", {
                stealthLevel: e.target
                  .value as BackendConfig["instanceDefaults"]["stealthLevel"],
              })
            }
          >
            <option value="light">
              {t("settings.instanceDefaults.stealthLevel.light")}
            </option>
            <option value="medium">
              {t("settings.instanceDefaults.stealthLevel.medium")}
            </option>
            <option value="full">
              {t("settings.instanceDefaults.stealthLevel.full")}
            </option>
          </Select>
          <div className="rounded-sm border border-border-subtle bg-black/10 px-3 py-2 text-xs leading-5 text-text-muted">
            {backendConfig.instanceDefaults.stealthLevel === "light" && (
              <div className="space-y-2">
                <div>
                  <Trans
                    i18nKey="settings.instanceDefaults.stealthLevel.lightLead"
                    components={{
                      strong: <strong className="text-text-secondary" />,
                    }}
                  />
                </div>
                <div className="text-success/80">
                  {t("settings.instanceDefaults.stealthLevel.lightOk1")}
                </div>
                <div className="text-success/80">
                  {t("settings.instanceDefaults.stealthLevel.lightOk2")}
                </div>
              </div>
            )}
            {backendConfig.instanceDefaults.stealthLevel === "medium" && (
              <div className="space-y-2">
                <div>
                  <Trans
                    i18nKey="settings.instanceDefaults.stealthLevel.mediumLead"
                    components={{ strong: <strong className="text-warning" /> }}
                  />
                </div>
                <div className="text-warning/80">
                  {t("settings.instanceDefaults.stealthLevel.mediumWarn1")}
                </div>
                <div className="text-warning/80">
                  {t("settings.instanceDefaults.stealthLevel.mediumWarn2")}
                </div>
                <div className="text-warning/80">
                  {t("settings.instanceDefaults.stealthLevel.mediumWarn3")}
                </div>
              </div>
            )}
            {backendConfig.instanceDefaults.stealthLevel === "full" && (
              <div className="space-y-2">
                <div>
                  <Trans
                    i18nKey="settings.instanceDefaults.stealthLevel.fullLead"
                    components={{
                      strong: <strong className="text-destructive" />,
                    }}
                  />
                </div>
                <div className="text-destructive/80">
                  {t("settings.instanceDefaults.stealthLevel.fullWarn1")}
                </div>
                <div className="text-destructive/80">
                  {t("settings.instanceDefaults.stealthLevel.fullWarn2")}
                </div>
                <div className="text-destructive/80">
                  {t("settings.instanceDefaults.stealthLevel.fullWarn3")}
                </div>
                <div className="text-destructive/80">
                  {t("settings.instanceDefaults.stealthLevel.fullWarn4")}
                </div>
              </div>
            )}
          </div>
        </div>
      </SettingRow>
      <SettingRow
        label={t("settings.instanceDefaults.tabEvictionPolicy.label")}
        description={t(
          "settings.instanceDefaults.tabEvictionPolicy.description",
        )}
      >
        <Select
          value={backendConfig.instanceDefaults.tabEvictionPolicy}
          onChange={(e) =>
            updateBackendSection("instanceDefaults", {
              tabEvictionPolicy: e.target
                .value as BackendConfig["instanceDefaults"]["tabEvictionPolicy"],
            })
          }
        >
          <option value="reject">
            {t("settings.instanceDefaults.tabEvictionPolicy.reject")}
          </option>
          <option value="close_oldest">
            {t("settings.instanceDefaults.tabEvictionPolicy.closeOldest")}
          </option>
          <option value="close_lru">
            {t("settings.instanceDefaults.tabEvictionPolicy.closeLru")}
          </option>
        </Select>
      </SettingRow>
      <SettingRow
        label={t("settings.instanceDefaults.tabLifecycle.label")}
        description={t("settings.instanceDefaults.tabLifecycle.description")}
      >
        <Select
          value={backendConfig.instanceDefaults.tabPolicy?.lifecycle ?? "keep"}
          onChange={(e) =>
            updateBackendSection("instanceDefaults", {
              tabPolicy: {
                ...(backendConfig.instanceDefaults.tabPolicy ?? {}),
                lifecycle: e.target.value as NonNullable<
                  BackendConfig["instanceDefaults"]["tabPolicy"]
                >["lifecycle"],
              },
            })
          }
        >
          <option value="keep">
            {t("settings.instanceDefaults.tabLifecycle.keep")}
          </option>
          <option value="close_idle">
            {t("settings.instanceDefaults.tabLifecycle.closeIdle")}
          </option>
          <option value="freeze_idle">
            {t("settings.instanceDefaults.tabLifecycle.freezeIdle")}
          </option>
        </Select>
      </SettingRow>
      <SettingRow
        label={t("settings.instanceDefaults.autoCloseDelay.label")}
        description={t("settings.instanceDefaults.autoCloseDelay.description")}
      >
        <input
          type="number"
          min={1}
          value={backendConfig.instanceDefaults.tabPolicy?.closeDelaySec ?? 300}
          onChange={(e) =>
            updateBackendSection("instanceDefaults", {
              tabPolicy: {
                ...(backendConfig.instanceDefaults.tabPolicy ?? {}),
                closeDelaySec: Number(e.target.value),
              },
            })
          }
          disabled={
            (backendConfig.instanceDefaults.tabPolicy?.lifecycle ?? "keep") ===
            "keep"
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.instanceDefaults.restoreTabs.label")}
        description={t("settings.instanceDefaults.restoreTabs.description")}
      >
        <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={backendConfig.instanceDefaults.tabPolicy?.restore ?? false}
            onChange={(e) =>
              updateBackendSection("instanceDefaults", {
                tabPolicy: {
                  ...(backendConfig.instanceDefaults.tabPolicy ?? {}),
                  restore: e.target.checked,
                },
              })
            }
            className="h-4 w-4"
          />
          {t("common.enable")}
        </label>
      </SettingRow>
      <SettingRow
        label={t("settings.instanceDefaults.maxTabs.label")}
        description={t("settings.instanceDefaults.maxTabs.description")}
      >
        <input
          type="number"
          min={1}
          value={backendConfig.instanceDefaults.maxTabs}
          onChange={(e) =>
            updateBackendSection("instanceDefaults", {
              maxTabs: Number(e.target.value),
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.instanceDefaults.maxParallelTabs.label")}
        description={t("settings.instanceDefaults.maxParallelTabs.description")}
      >
        <input
          type="number"
          min={0}
          value={backendConfig.instanceDefaults.maxParallelTabs}
          onChange={(e) =>
            updateBackendSection("instanceDefaults", {
              maxParallelTabs: Number(e.target.value),
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.instanceDefaults.timezone.label")}
        description={t("settings.instanceDefaults.timezone.description")}
      >
        <input
          value={backendConfig.instanceDefaults.timezone}
          onChange={(e) =>
            updateBackendSection("instanceDefaults", {
              timezone: e.target.value,
            })
          }
          placeholder={t("settings.instanceDefaults.timezone.placeholder")}
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.instanceDefaults.userAgent.label")}
        description={t("settings.instanceDefaults.userAgent.description")}
      >
        <input
          value={backendConfig.instanceDefaults.userAgent}
          onChange={(e) =>
            updateBackendSection("instanceDefaults", {
              userAgent: e.target.value,
            })
          }
          placeholder={t("settings.instanceDefaults.userAgent.placeholder")}
          className={fieldClass}
        />
      </SettingRow>
      {instanceDefaultsBooleanRows.map(([key, labelKey]) => (
        <SettingRow
          key={key}
          label={t(labelKey)}
          description={t("settings.instanceDefaultsHint")}
        >
          <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
            <input
              type="checkbox"
              checked={backendConfig.instanceDefaults[key]}
              onChange={(e) =>
                updateBackendSection("instanceDefaults", {
                  [key]: e.target.checked,
                } as Partial<BackendConfig["instanceDefaults"]>)
              }
              className="h-4 w-4"
            />
            {t("common.enable")}
          </label>
        </SettingRow>
      ))}
    </SectionCard>
  );
}
