import type { BackendConfig, BackendConfigState } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
import { Trans, useTranslation } from "react-i18next";
import { csvToList, fieldClass, listToCsv } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface AutoSolverSettingsSectionProps {
  backendConfig: BackendConfig;
  backendState: BackendConfigState | null;
  updateBackendSection: UpdateBackendSection;
}

export function AutoSolverSettingsSection({
  backendConfig,
  backendState,
  updateBackendSection,
}: AutoSolverSettingsSectionProps) {
  const { t } = useTranslation();

  return (
    <SectionCard
      title={t("settings.sections.autosolver.label")}
      description={t("settings.panels.autosolver.description")}
    >
      <SettingRow
        label={t("settings.autosolver.configFile.label")}
        description={t("settings.autosolver.configFile.description")}
      >
        <div className="rounded-sm border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.72)] px-3 py-2 text-sm text-text-secondary">
          <code>
            {backendState?.configPath ||
              t("settings.autosolver.configFile.unavailable")}
          </code>
        </div>
      </SettingRow>
      <SettingRow
        label={t("settings.autosolver.enable.label")}
        description={t("settings.autosolver.enable.description")}
      >
        <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={backendConfig.autoSolver.enabled}
            onChange={(e) =>
              updateBackendSection("autoSolver", {
                enabled: e.target.checked,
              })
            }
            className="h-4 w-4"
          />
          {backendConfig.autoSolver.enabled
            ? t("common.enabled")
            : t("common.disabled")}
        </label>
      </SettingRow>
      <SettingRow
        label={t("settings.autosolver.autoTrigger.label")}
        description={t("settings.autosolver.autoTrigger.description")}
      >
        <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={backendConfig.autoSolver.autoTrigger}
            onChange={(e) =>
              updateBackendSection("autoSolver", {
                autoTrigger: e.target.checked,
              })
            }
            className="h-4 w-4"
          />
          {backendConfig.autoSolver.autoTrigger
            ? t("common.enabled")
            : t("common.disabled")}
        </label>
      </SettingRow>
      <SettingRow
        label={t("settings.autosolver.triggerOnNavigate.label")}
        description={t("settings.autosolver.triggerOnNavigate.description")}
      >
        <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={backendConfig.autoSolver.triggerOnNavigate}
            onChange={(e) =>
              updateBackendSection("autoSolver", {
                triggerOnNavigate: e.target.checked,
              })
            }
            className="h-4 w-4"
          />
          {backendConfig.autoSolver.triggerOnNavigate
            ? t("common.enabled")
            : t("common.disabled")}
        </label>
      </SettingRow>
      <SettingRow
        label={t("settings.autosolver.triggerOnAction.label")}
        description={t("settings.autosolver.triggerOnAction.description")}
      >
        <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={backendConfig.autoSolver.triggerOnAction}
            onChange={(e) =>
              updateBackendSection("autoSolver", {
                triggerOnAction: e.target.checked,
              })
            }
            className="h-4 w-4"
          />
          {backendConfig.autoSolver.triggerOnAction
            ? t("common.enabled")
            : t("common.disabled")}
        </label>
      </SettingRow>
      <SettingRow
        label={t("settings.autosolver.maxAttempts.label")}
        description={t("settings.autosolver.maxAttempts.description")}
      >
        <input
          type="number"
          min={1}
          value={backendConfig.autoSolver.maxAttempts}
          onChange={(e) =>
            updateBackendSection("autoSolver", {
              maxAttempts: Number(e.target.value),
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.autosolver.solverTimeoutSec.label")}
        description={t("settings.autosolver.solverTimeoutSec.description")}
      >
        <input
          type="number"
          min={1}
          value={backendConfig.autoSolver.solverTimeoutSec}
          onChange={(e) =>
            updateBackendSection("autoSolver", {
              solverTimeoutSec: Number(e.target.value),
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.autosolver.retryBaseDelayMs.label")}
        description={t("settings.autosolver.retryBaseDelayMs.description")}
      >
        <input
          type="number"
          min={0}
          value={backendConfig.autoSolver.retryBaseDelayMs}
          onChange={(e) =>
            updateBackendSection("autoSolver", {
              retryBaseDelayMs: Number(e.target.value),
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.autosolver.retryMaxDelayMs.label")}
        description={t("settings.autosolver.retryMaxDelayMs.description")}
      >
        <input
          type="number"
          min={0}
          value={backendConfig.autoSolver.retryMaxDelayMs}
          onChange={(e) =>
            updateBackendSection("autoSolver", {
              retryMaxDelayMs: Number(e.target.value),
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.autosolver.solvers.label")}
        description={t("settings.autosolver.solvers.description")}
      >
        <input
          value={listToCsv(backendConfig.autoSolver.solvers)}
          onChange={(e) =>
            updateBackendSection("autoSolver", {
              solvers: csvToList(e.target.value),
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.autosolver.llmProvider.label")}
        description={t("settings.autosolver.llmProvider.description")}
      >
        <input
          value={backendConfig.autoSolver.llmProvider}
          onChange={(e) =>
            updateBackendSection("autoSolver", {
              llmProvider: e.target.value,
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.autosolver.llmFallback.label")}
        description={t("settings.autosolver.llmFallback.description")}
      >
        <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={backendConfig.autoSolver.llmFallback}
            onChange={(e) =>
              updateBackendSection("autoSolver", {
                llmFallback: e.target.checked,
              })
            }
            className="h-4 w-4"
          />
          {backendConfig.autoSolver.llmFallback
            ? t("common.enabled")
            : t("common.disabled")}
        </label>
      </SettingRow>
      <SettingRow
        label={t("settings.autosolver.externalKeys.label")}
        description={t("settings.autosolver.externalKeys.description")}
      >
        <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
          <Trans
            i18nKey="settings.autosolver.externalKeys.note"
            components={{ code: <code /> }}
          />
        </div>
      </SettingRow>
    </SectionCard>
  );
}
