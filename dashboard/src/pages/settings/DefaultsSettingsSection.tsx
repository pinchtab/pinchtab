import type { BackendConfig } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
import {
  fieldClass,
  instanceDefaultsBooleanRows,
  selectClass,
} from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface DefaultsSettingsSectionProps {
  backendConfig: BackendConfig;
  updateBackendSection: UpdateBackendSection;
}

export function DefaultsSettingsSection({
  backendConfig,
  updateBackendSection,
}: DefaultsSettingsSectionProps) {
  return (
    <SectionCard
      title="Instance Defaults"
      description="These values are written to config and used for new managed instances. Existing running instances keep their current runtime."
    >
      <SettingRow
        label="Mode"
        description="Default browser mode for new launches."
      >
        <select
          value={backendConfig.instanceDefaults.mode}
          onChange={(e) =>
            updateBackendSection("instanceDefaults", {
              mode: e.target.value as BackendConfig["instanceDefaults"]["mode"],
            })
          }
          className={selectClass}
        >
          <option value="headless">Headless</option>
          <option value="headed">Headed</option>
        </select>
      </SettingRow>
      <SettingRow
        label="Stealth level"
        description="Bot detection evasion profile. Higher levels may affect error monitoring and certain browser features."
      >
        <div className="space-y-2">
          <select
            value={backendConfig.instanceDefaults.stealthLevel}
            onChange={(e) =>
              updateBackendSection("instanceDefaults", {
                stealthLevel: e.target
                  .value as BackendConfig["instanceDefaults"]["stealthLevel"],
              })
            }
            className={selectClass}
          >
            <option value="light">Light</option>
            <option value="medium">Medium</option>
            <option value="full">Full</option>
          </select>
          <div className="rounded-sm border border-border-subtle bg-black/10 px-3 py-2 text-xs leading-5 text-text-muted">
            {backendConfig.instanceDefaults.stealthLevel === "light" && (
              <div className="space-y-2">
                <div>
                  <strong className="text-text-secondary">Light:</strong>{" "}
                  Default baseline stealth. Keeps the lowest-risk launch and JS
                  contract while hiding basic automation markers.
                </div>
                <div className="text-success/80">
                  ✓ Default product security baseline
                </div>
                <div className="text-success/80">
                  ✓ No intentional API realism or security tradeoff
                </div>
              </div>
            )}
            {backendConfig.instanceDefaults.stealthLevel === "medium" && (
              <div className="space-y-2">
                <div>
                  <strong className="text-warning">Medium:</strong> Non-default
                  risk mode. Adds Client Hints, `chrome.runtime` shims, iframe
                  propagation, stack filtering, and native-looking function
                  masking to improve anti-bot compatibility.
                </div>
                <div className="text-warning/80">
                  ⚠ Alters browser-visible APIs and error/stack behavior.
                  Monitoring and debugging tools may see different results.
                </div>
                <div className="text-warning/80">
                  ⚠ Permissions and compatibility shims can return intentionally
                  altered values. Do not use this as the default safety
                  baseline.
                </div>
                <div className="text-warning/80">
                  ⚠ Reports that require explicitly enabling Medium should be
                  treated as opt-in risk acceptance, not default-path behavior.
                </div>
              </div>
            )}
            {backendConfig.instanceDefaults.stealthLevel === "full" && (
              <div className="space-y-2">
                <div>
                  <strong className="text-destructive">Full:</strong>{" "}
                  Highest-risk non-default mode. Adds graphics, canvas, audio,
                  system-color, and WebRTC alterations on top of Medium.
                </div>
                <div className="text-destructive/80">
                  ⚠ Browser output is intentionally less native and less stable.
                  Rendering, media, and networking behavior may break or drift
                  from real Chrome.
                </div>
                <div className="text-destructive/80">
                  ⚠ This mode is not an acceptable default security posture.
                  Only enable it when you explicitly accept the tradeoff
                  surface.
                </div>
                <div className="text-destructive/80">
                  ⚠ Reports that depend on enabling Full should be triaged as
                  non-default operator risk unless a default-path bypass is
                  shown.
                </div>
                <div className="text-destructive/80">
                  ⚠ WebRTC, WebGL, canvas, and audio behavior can all diverge
                  from baseline Chrome.
                </div>
              </div>
            )}
          </div>
        </div>
      </SettingRow>
      <SettingRow
        label="Tab eviction policy"
        description="How PinchTab behaves when a managed instance reaches its tab limit."
      >
        <select
          value={backendConfig.instanceDefaults.tabEvictionPolicy}
          onChange={(e) =>
            updateBackendSection("instanceDefaults", {
              tabEvictionPolicy: e.target
                .value as BackendConfig["instanceDefaults"]["tabEvictionPolicy"],
            })
          }
          className={selectClass}
        >
          <option value="reject">Reject new tabs</option>
          <option value="close_oldest">Close oldest</option>
          <option value="close_lru">Close least recently used</option>
        </select>
      </SettingRow>
      <SettingRow
        label="Max tabs"
        description="Maximum number of tabs per managed instance."
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
        label="Max parallel tabs"
        description="Set to 0 to auto-detect from CPU count."
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
        label="Timezone"
        description="Optional timezone override for launched instances."
      >
        <input
          value={backendConfig.instanceDefaults.timezone}
          onChange={(e) =>
            updateBackendSection("instanceDefaults", {
              timezone: e.target.value,
            })
          }
          placeholder="Europe/Rome"
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label="User agent"
        description="Optional override applied to new managed instances."
      >
        <input
          value={backendConfig.instanceDefaults.userAgent}
          onChange={(e) =>
            updateBackendSection("instanceDefaults", {
              userAgent: e.target.value,
            })
          }
          placeholder="Custom user agent"
          className={fieldClass}
        />
      </SettingRow>
      {instanceDefaultsBooleanRows.map(([key, label]) => (
        <SettingRow
          key={key}
          label={label}
          description="Applies to newly launched managed instances."
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
            Enable
          </label>
        </SettingRow>
      ))}
    </SectionCard>
  );
}
