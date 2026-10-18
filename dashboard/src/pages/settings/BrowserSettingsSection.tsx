import type {
  BackendBrowserProvider,
  BackendCloakBrowserConfig,
  BackendConfig,
} from "../../types";
import type { UpdateBackendSection } from "./settingsShared";
import { useTranslation } from "react-i18next";
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
  const { t } = useTranslation();
  const currentProvider: BackendBrowserProvider =
    backendConfig.browsers?.default ?? "chrome";
  const updateCloak = (patch: Partial<BackendCloakBrowserConfig>) =>
    updateBackendSection("browser", {
      cloak: {
        ...cloak,
        ...patch,
      },
    });

  return (
    <SectionCard
      title={t("settings.sections.browser.label")}
      description={t("settings.panels.browser.description")}
    >
      <SettingRow
        label={t("settings.browser.provider.label")}
        description={t("settings.browser.provider.description")}
      >
        <select
          value={currentProvider}
          onChange={(e) =>
            updateBackendSection("browsers", {
              default: e.target.value as BackendBrowserProvider,
            })
          }
          className={fieldClass}
        >
          <option value="chrome">
            {t("settings.browser.provider.chrome")}
          </option>
          <option value="cloak">{t("settings.browser.provider.cloak")}</option>
          <option value="ghost-chrome">
            {t("settings.browser.provider.ghostChrome")}
          </option>
        </select>
      </SettingRow>
      <SettingRow
        label={t("settings.browser.version.label")}
        description={t("settings.browser.version.description")}
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
        label={t("settings.browser.binary.label")}
        description={t("settings.browser.binary.description")}
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
      {currentProvider === "cloak" && (
        <>
          <SettingRow
            label={t("settings.browser.cloak.fingerprintSeed.label")}
            description={t(
              "settings.browser.cloak.fingerprintSeed.description",
            )}
          >
            <input
              value={cloak.fingerprintSeed}
              onChange={(e) => updateCloak({ fingerprintSeed: e.target.value })}
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label={t("settings.browser.cloak.platform.label")}
            description={t("settings.browser.cloak.platform.description")}
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
              <option value="">
                {t("settings.browser.cloak.platform.auto")}
              </option>
              <option value="windows">
                {t("settings.browser.cloak.platform.windows")}
              </option>
              <option value="macos">
                {t("settings.browser.cloak.platform.macos")}
              </option>
              <option value="linux">
                {t("settings.browser.cloak.platform.linux")}
              </option>
            </select>
          </SettingRow>
          <SettingRow
            label={t("settings.browser.cloak.locale.label")}
            description={t("settings.browser.cloak.locale.description")}
          >
            <input
              value={cloak.locale}
              onChange={(e) => updateCloak({ locale: e.target.value })}
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label={t("settings.browser.cloak.timezone.label")}
            description={t("settings.browser.cloak.timezone.description")}
          >
            <input
              value={cloak.timezone}
              onChange={(e) => updateCloak({ timezone: e.target.value })}
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label={t("settings.browser.cloak.webrtcIP.label")}
            description={t("settings.browser.cloak.webrtcIP.description")}
          >
            <input
              value={cloak.webrtcIP}
              onChange={(e) => updateCloak({ webrtcIP: e.target.value })}
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label={t("settings.browser.cloak.fontsDir.label")}
            description={t("settings.browser.cloak.fontsDir.description")}
          >
            <input
              value={cloak.fontsDir}
              onChange={(e) => updateCloak({ fontsDir: e.target.value })}
              className={fieldClass}
            />
          </SettingRow>
          <SettingRow
            label={t("settings.browser.cloak.storageQuota.label")}
            description={t("settings.browser.cloak.storageQuota.description")}
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
            label={t("settings.browser.cloak.nativeStealthOnly.label")}
            description={t(
              "settings.browser.cloak.nativeStealthOnly.description",
            )}
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
              {t("settings.browser.cloak.nativeStealthOnly.toggle")}
            </label>
          </SettingRow>
        </>
      )}
      <SettingRow
        label={t("settings.browser.extraFlags.label")}
        description={t("settings.browser.extraFlags.description")}
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
        label={t("settings.browser.extensionPaths.label")}
        description={t("settings.browser.extensionPaths.description")}
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
