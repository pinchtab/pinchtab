import type { BackendCloakBrowserConfig, BackendConfig } from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
import { csvToList, fieldClass, listToCsv } from "./settingsShared";
import { SectionCard, SettingRow } from "./SettingsSharedComponents";

interface BrowserSettingsSectionProps {
  backendConfig: BackendConfig;
  updateBackendSection: UpdateBackendSection;
}

export function BrowserSettingsSection({
  backendConfig,
  updateBackendSection,
}: BrowserSettingsSectionProps) {
  const cloak = backendConfig.browser.cloak;
  const updateCloak = (patch: Partial<BackendCloakBrowserConfig>) =>
    updateBackendSection("browser", {
      cloak: {
        ...cloak,
        ...patch,
      },
    });

  return (
    <SectionCard
      title="Browser Runtime"
      description="These settings are written into the generated child config for new managed instances."
    >
      <SettingRow
        label="Provider"
        description="Browser backend used for new managed instances."
      >
        <select
          value={backendConfig.browser.provider}
          onChange={(e) =>
            updateBackendSection("browser", {
              provider: e.target.value === "cloak" ? "cloak" : "chrome",
            })
          }
          className={fieldClass}
        >
          <option value="chrome">Chrome</option>
          <option value="cloak">CloakBrowser</option>
        </select>
      </SettingRow>
      <SettingRow
        label="Browser version"
        description="Version string used in generated UA/fingerprint defaults."
      >
        <input
          value={backendConfig.browser.version}
          onChange={(e) =>
            updateBackendSection("browser", {
              version: e.target.value,
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label="Browser binary"
        description="Optional path override for the Chrome or CloakBrowser executable."
      >
        <input
          value={backendConfig.browser.binary}
          onChange={(e) =>
            updateBackendSection("browser", {
              binary: e.target.value,
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      {backendConfig.browser.provider === "cloak" && (
        <>
          <SettingRow
            label="Fingerprint seed"
            description="Deterministic CloakBrowser identity seed. Leave blank for a fresh identity per launch."
          >
            <input
              value={cloak.fingerprintSeed}
              onChange={(e) => updateCloak({ fingerprintSeed: e.target.value })}
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label="Fingerprint platform"
            description="Native platform fingerprint reported by CloakBrowser."
          >
            <select
              value={cloak.platform}
              onChange={(e) =>
                updateCloak({
                  platform: e.target
                    .value as BackendCloakBrowserConfig["platform"],
                })
              }
              className={fieldClass}
            >
              <option value="">Auto</option>
              <option value="windows">Windows</option>
              <option value="macos">macOS</option>
              <option value="linux">Linux</option>
            </select>
          </SettingRow>
          <SettingRow
            label="Cloak locale"
            description="Locale passed as --fingerprint-locale."
          >
            <input
              value={cloak.locale}
              onChange={(e) => updateCloak({ locale: e.target.value })}
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label="Cloak timezone"
            description="Timezone passed as --fingerprint-timezone."
          >
            <input
              value={cloak.timezone}
              onChange={(e) => updateCloak({ timezone: e.target.value })}
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label="WebRTC IP"
            description="Explicit replacement IP or auto for CloakBrowser proxy exit-IP resolution."
          >
            <input
              value={cloak.webrtcIP}
              onChange={(e) => updateCloak({ webrtcIP: e.target.value })}
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label="Fonts directory"
            description="Directory containing target-platform fonts for CloakBrowser."
          >
            <input
              value={cloak.fontsDir}
              onChange={(e) => updateCloak({ fontsDir: e.target.value })}
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label="Storage quota"
            description="Storage quota in MB passed as --fingerprint-storage-quota."
          >
            <input
              type="number"
              min={0}
              value={cloak.storageQuotaMB ?? ""}
              onChange={(e) =>
                updateCloak({
                  storageQuotaMB:
                    e.target.value === "" ? undefined : Number(e.target.value),
                })
              }
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label="Native stealth only"
            description="Disable PinchTab JS stealth overlays and automation-hiding launch flags."
          >
            <label className="flex items-center gap-2 text-sm text-text-primary">
              <input
                type="checkbox"
                checked={cloak.disableDefaultStealthArgs ?? true}
                onChange={(e) =>
                  updateCloak({
                    disableDefaultStealthArgs: e.target.checked,
                  })
                }
                className="h-4 w-4 accent-primary"
              />
              Use CloakBrowser native patches
            </label>
          </SettingRow>
        </>
      )}
      <SettingRow
        label="Extra flags"
        description="Additional Chrome flags appended when launching managed instances."
      >
        <input
          value={backendConfig.browser.extraFlags}
          onChange={(e) =>
            updateBackendSection("browser", {
              extraFlags: e.target.value,
            })
          }
          className={fieldClass}
        />
      </SettingRow>
      <SettingRow
        label="Extension paths"
        description="Comma-separated extension directories to load. By default, PinchTab uses the local extensions/ folder under its state/config directory. Set custom paths here to override that default, or clear the field to disable extension loading."
      >
        <input
          value={listToCsv(backendConfig.browser.extensionPaths)}
          onChange={(e) =>
            updateBackendSection("browser", {
              extensionPaths: csvToList(e.target.value),
            })
          }
          className={fieldClass}
        />
      </SettingRow>
    </SectionCard>
  );
}
