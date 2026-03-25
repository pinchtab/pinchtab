import type { BackendConfig } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
import { fieldClass, selectClass } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface OrchestrationSettingsSectionProps {
  backendConfig: BackendConfig;
  updateBackendSection: UpdateBackendSection;
}

export function OrchestrationSettingsSection({
  backendConfig,
  updateBackendSection,
}: OrchestrationSettingsSectionProps) {
  return (
    <SectionCard
      title="Orchestration"
      description="Port range and allocation policy can be applied immediately for future launches. Strategy and restart-policy changes require a dashboard restart because strategy routes and lifecycle state are registered at startup."
    >
      <SettingRow
        label="Strategy"
        description="Controls instance lifecycle and how shorthand routes are routed."
      >
        <select
          value={backendConfig.multiInstance.strategy}
          onChange={(e) =>
            updateBackendSection("multiInstance", {
              strategy: e.target
                .value as BackendConfig["multiInstance"]["strategy"],
            })
          }
          className={selectClass}
        >
          <option value="always-on">Always on</option>
          <option value="simple">Simple</option>
          <option value="explicit">Explicit</option>
          <option value="simple-autorestart">Simple autorestart</option>
          <option value="no-instance">No instance (hub)</option>
        </select>
        <div className="mt-2 text-[11px] leading-relaxed text-text-muted">
          {backendConfig.multiInstance.strategy === "always-on" &&
            "Launches a default instance at boot and relaunches on crash."}
          {backendConfig.multiInstance.strategy === "simple" &&
            "Launches one instance on first request. No auto-restart."}
          {backendConfig.multiInstance.strategy === "explicit" &&
            "All instances managed via API. No automatic launches."}
          {backendConfig.multiInstance.strategy === "simple-autorestart" &&
            "Launches on first request and relaunches on crash."}
          {backendConfig.multiInstance.strategy === "no-instance" &&
            "No local Chrome processes. Acts as a hub for remote bridges only."}
        </div>
      </SettingRow>
      <SettingRow
        label="Allocation policy"
        description="Determines how running instances are chosen for shorthand requests."
      >
        <select
          value={backendConfig.multiInstance.allocationPolicy}
          onChange={(e) =>
            updateBackendSection("multiInstance", {
              allocationPolicy: e.target
                .value as BackendConfig["multiInstance"]["allocationPolicy"],
            })
          }
          className={selectClass}
        >
          <option value="fcfs">First available</option>
          <option value="round_robin">Round robin</option>
          <option value="random">Random</option>
        </select>
      </SettingRow>
      <SettingRow
        label="Instance port start"
        description="Lower bound for auto-allocated instance ports."
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
        label="Instance port end"
        description="Upper bound for auto-allocated instance ports."
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
            label="Max restarts"
            description="Maximum restart attempts. Use -1 for unlimited, 0 for no restarts."
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
            label="Initial backoff"
            description="Delay in seconds before the first restart attempt."
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
            label="Max backoff"
            description="Upper bound in seconds for exponential restart backoff."
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
            label="Stable after"
            description="Seconds the instance must stay healthy before the restart counter resets."
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
