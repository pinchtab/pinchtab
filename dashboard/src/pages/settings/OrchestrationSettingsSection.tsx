import { Select } from "../../components/atoms";
import type { BackendConfig } from "../../types";
import { useTranslation } from "react-i18next";
import type { UpdateBackendSection } from "./settingsShared";
import { fieldClass } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface OrchestrationSettingsSectionProps {
  backendConfig: BackendConfig;
  updateBackendSection: UpdateBackendSection;
}

export function OrchestrationSettingsSection({
  backendConfig,
  updateBackendSection,
}: OrchestrationSettingsSectionProps) {
  const { t } = useTranslation();

  return (
    <SectionCard
      title={t("settings.sections.orchestration.label")}
      description={t("settings.panels.orchestration.description")}
    >
      <SettingRow
        label={t("settings.orchestration.strategy.label")}
        description={t("settings.orchestration.strategy.description")}
      >
        <Select
          value={backendConfig.multiInstance.strategy}
          onChange={(e) =>
            updateBackendSection("multiInstance", {
              strategy: e.target
                .value as BackendConfig["multiInstance"]["strategy"],
            })
          }
        >
          <option value="always-on">
            {t("settings.orchestration.strategy.alwaysOn")}
          </option>
          <option value="simple">
            {t("settings.orchestration.strategy.simple")}
          </option>
          <option value="explicit">
            {t("settings.orchestration.strategy.explicit")}
          </option>
          <option value="simple-autorestart">
            {t("settings.orchestration.strategy.simpleAutorestart")}
          </option>
          <option value="no-instance">
            {t("settings.orchestration.strategy.noInstance")}
          </option>
        </Select>
        <div className="mt-2 text-[11px] leading-relaxed text-text-muted">
          {backendConfig.multiInstance.strategy === "always-on" &&
            t("settings.orchestration.strategyHint.alwaysOn")}
          {backendConfig.multiInstance.strategy === "simple" &&
            t("settings.orchestration.strategyHint.simple")}
          {backendConfig.multiInstance.strategy === "explicit" &&
            t("settings.orchestration.strategyHint.explicit")}
          {backendConfig.multiInstance.strategy === "simple-autorestart" &&
            t("settings.orchestration.strategyHint.simpleAutorestart")}
          {backendConfig.multiInstance.strategy === "no-instance" &&
            t("settings.orchestration.strategyHint.noInstance")}
        </div>
      </SettingRow>
      <SettingRow
        label={t("settings.orchestration.allocationPolicy.label")}
        description={t("settings.orchestration.allocationPolicy.description")}
      >
        <Select
          value={backendConfig.multiInstance.allocationPolicy}
          onChange={(e) =>
            updateBackendSection("multiInstance", {
              allocationPolicy: e.target
                .value as BackendConfig["multiInstance"]["allocationPolicy"],
            })
          }
        >
          <option value="fcfs">
            {t("settings.orchestration.allocationPolicy.fcfs")}
          </option>
          <option value="round_robin">
            {t("settings.orchestration.allocationPolicy.roundRobin")}
          </option>
          <option value="random">
            {t("settings.orchestration.allocationPolicy.random")}
          </option>
        </Select>
      </SettingRow>
      <SettingRow
        label={t("settings.orchestration.instancePortStart.label")}
        description={t("settings.orchestration.instancePortStart.description")}
      >
        <input
          type="number"
          min={1}
          value={backendConfig.multiInstance.instancePortStart}
          onChange={(e) =>
            updateBackendSection("multiInstance", {
              instancePortStart: Number(e.target.value),
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label={t("settings.orchestration.instancePortEnd.label")}
        description={t("settings.orchestration.instancePortEnd.description")}
      >
        <input
          type="number"
          min={1}
          value={backendConfig.multiInstance.instancePortEnd}
          onChange={(e) =>
            updateBackendSection("multiInstance", {
              instancePortEnd: Number(e.target.value),
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      {(backendConfig.multiInstance.strategy === "always-on" ||
        backendConfig.multiInstance.strategy === "simple-autorestart") && (
        <>
          <SettingRow
            label={t("settings.orchestration.restart.maxRestarts.label")}
            description={t(
              "settings.orchestration.restart.maxRestarts.description",
            )}
          >
            <input
              type="number"
              min={-1}
              value={backendConfig.multiInstance.restart.maxRestarts}
              onChange={(e) =>
                updateBackendSection("multiInstance", {
                  restart: {
                    ...backendConfig.multiInstance.restart,
                    maxRestarts: Number(e.target.value),
                  },
                })
              }
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label={t("settings.orchestration.restart.initBackoff.label")}
            description={t(
              "settings.orchestration.restart.initBackoff.description",
            )}
          >
            <input
              type="number"
              min={1}
              value={backendConfig.multiInstance.restart.initBackoffSec}
              onChange={(e) =>
                updateBackendSection("multiInstance", {
                  restart: {
                    ...backendConfig.multiInstance.restart,
                    initBackoffSec: Number(e.target.value),
                  },
                })
              }
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label={t("settings.orchestration.restart.maxBackoff.label")}
            description={t(
              "settings.orchestration.restart.maxBackoff.description",
            )}
          >
            <input
              type="number"
              min={1}
              value={backendConfig.multiInstance.restart.maxBackoffSec}
              onChange={(e) =>
                updateBackendSection("multiInstance", {
                  restart: {
                    ...backendConfig.multiInstance.restart,
                    maxBackoffSec: Number(e.target.value),
                  },
                })
              }
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label={t("settings.orchestration.restart.stableAfter.label")}
            description={t(
              "settings.orchestration.restart.stableAfter.description",
            )}
          >
            <input
              type="number"
              min={1}
              value={backendConfig.multiInstance.restart.stableAfterSec}
              onChange={(e) =>
                updateBackendSection("multiInstance", {
                  restart: {
                    ...backendConfig.multiInstance.restart,
                    stableAfterSec: Number(e.target.value),
                  },
                })
              }
              className={fieldClass}
            />
          </SettingRow>
        </>
      )}
    </SectionCard>
  );
}
