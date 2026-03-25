import type { Dispatch, FormEvent, SetStateAction } from "react";
import { useEffect, useMemo, useState } from "react";
import { Button, Card, Input, Modal } from "../components/atoms";
import { credentialUsername, storeTokenCredential } from "../services/auth";
import * as api from "../services/api";
import { useAppStore } from "../stores/useAppStore";
import type {
  BackendConfig,
  BackendConfigState,
  LocalDashboardSettings,
} from "../types";
import { BrowserSettingsSection } from "./settings/BrowserSettingsSection";
import { DashboardSettingsSection } from "./settings/DashboardSettingsSection";
import { DefaultsSettingsSection } from "./settings/DefaultsSettingsSection";
import { NetworkSettingsSection } from "./settings/NetworkSettingsSection";
import { OrchestrationSettingsSection } from "./settings/OrchestrationSettingsSection";
import { ProfilesSettingsSection } from "./settings/ProfilesSettingsSection";
import { SecurityIdpiSettingsSection } from "./settings/SecurityIdpiSettingsSection";
import { SecuritySettingsSection } from "./settings/SecuritySettingsSection";
import {
  backendSaveNotice,
  sections,
  type SectionId,
  type UpdateBackendSection,
} from "./settings/settingsShared";
import { TimeoutsSettingsSection } from "./settings/TimeoutsSettingsSection";

type PendingElevatedAction = "save" | null;

function renderActiveSection(
  activeSection: SectionId,
  options: {
    apiTokenMissing: boolean;
    attachWildcard: boolean;
    backendConfig: BackendConfig;
    backendState: BackendConfigState | null;
    idpiDomainsConfigured: boolean;
    idpiEnabled: boolean;
    idpiWildcard: boolean;
    localSettings: LocalDashboardSettings;
    nonLoopbackBind: boolean;
    sensitiveEndpointsEnabled: boolean;
    setLocalSettings: Dispatch<SetStateAction<LocalDashboardSettings>>;
    updateBackendSection: UpdateBackendSection;
  },
) {
  switch (activeSection) {
    case "dashboard":
      return (
        <DashboardSettingsSection
          localSettings={options.localSettings}
          setLocalSettings={options.setLocalSettings}
        />
      );
    case "defaults":
      return (
        <DefaultsSettingsSection
          backendConfig={options.backendConfig}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "orchestration":
      return (
        <OrchestrationSettingsSection
          backendConfig={options.backendConfig}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "security":
      return (
        <SecuritySettingsSection
          backendConfig={options.backendConfig}
          sensitiveEndpointsEnabled={options.sensitiveEndpointsEnabled}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "security-idpi":
      return (
        <SecurityIdpiSettingsSection
          backendConfig={options.backendConfig}
          idpiDomainsConfigured={options.idpiDomainsConfigured}
          idpiEnabled={options.idpiEnabled}
          idpiWildcard={options.idpiWildcard}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "profiles":
      return (
        <ProfilesSettingsSection
          backendConfig={options.backendConfig}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "network":
      return (
        <NetworkSettingsSection
          apiTokenMissing={options.apiTokenMissing}
          attachWildcard={options.attachWildcard}
          backendConfig={options.backendConfig}
          backendState={options.backendState}
          nonLoopbackBind={options.nonLoopbackBind}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "browser":
      return (
        <BrowserSettingsSection
          backendConfig={options.backendConfig}
          updateBackendSection={options.updateBackendSection}
        />
      );
    case "timeouts":
      return (
        <TimeoutsSettingsSection
          backendConfig={options.backendConfig}
          updateBackendSection={options.updateBackendSection}
        />
      );
  }
}

export default function SettingsPage() {
  const { settings, setSettings, serverInfo, setServerInfo } = useAppStore();
  const [activeSection, setActiveSection] = useState<SectionId>("dashboard");
  const [localSettings, setLocalSettings] =
    useState<LocalDashboardSettings>(settings);
  const [backendState, setBackendState] = useState<BackendConfigState | null>(
    null,
  );
  const [backendConfig, setBackendConfig] = useState<BackendConfig | null>(
    null,
  );
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState("");
  const [notice, setNotice] = useState("");
  const [pendingElevatedAction, setPendingElevatedAction] =
    useState<PendingElevatedAction>(null);
  const [elevationToken, setElevationToken] = useState("");
  const [elevationError, setElevationError] = useState("");
  const [elevating, setElevating] = useState(false);
  const savedCredentialUsername = useMemo(() => credentialUsername(), []);

  useEffect(() => {
    setLocalSettings(settings);
  }, [settings]);

  useEffect(() => {
    const load = async () => {
      setLoading(true);
      setError("");
      try {
        const [configState, health] = await Promise.all([
          api.fetchBackendConfig(),
          api.fetchHealth().catch(() => null),
        ]);
        setBackendState(configState);
        setBackendConfig(configState.config);
        if (health) {
          setServerInfo(health);
        }
      } catch (e) {
        setError(e instanceof Error ? e.message : "Failed to load settings");
      } finally {
        setLoading(false);
      }
    };

    load();
  }, [setServerInfo]);

  const hasDashboardChanges = useMemo(
    () => JSON.stringify(localSettings) !== JSON.stringify(settings),
    [localSettings, settings],
  );

  const hasBackendConfigChanges = useMemo(
    () =>
      Boolean(
        backendConfig &&
        backendState &&
        JSON.stringify(backendConfig) !== JSON.stringify(backendState.config),
      ),
    [backendConfig, backendState],
  );

  const hasBackendChanges = hasBackendConfigChanges;
  const hasChanges = hasDashboardChanges || hasBackendChanges;
  const restartRequired =
    backendState?.restartRequired || serverInfo?.restartRequired || false;
  const restartReasons =
    backendState?.restartReasons || serverInfo?.restartReasons || [];
  const sensitiveEndpointsEnabled = backendConfig
    ? [
        backendConfig.security.allowEvaluate,
        backendConfig.security.allowMacro,
        backendConfig.security.allowScreencast,
        backendConfig.security.allowDownload,
        backendConfig.security.allowUpload,
      ].some(Boolean)
    : false;
  const apiTokenMissing = !backendState?.tokenConfigured;
  const idpiEnabled = backendConfig
    ? backendConfig.security.idpi.enabled
    : false;
  const idpiAllowedDomains = backendConfig
    ? backendConfig.security.idpi.allowedDomains
    : [];
  const idpiWildcard = idpiAllowedDomains.includes("*");
  const idpiDomainsConfigured = idpiAllowedDomains.length > 0 && !idpiWildcard;
  const attachAllowedHosts = backendConfig
    ? backendConfig.security.attach.allowHosts
    : [];
  const attachWildcard = attachAllowedHosts.includes("*");
  const nonLoopbackBind = backendConfig
    ? !["127.0.0.1", "localhost", "::1", ""].includes(
        backendConfig.server.bind.trim().toLowerCase(),
      )
    : false;

  const updateBackendSection: UpdateBackendSection = (section, patch) => {
    setBackendConfig((current) =>
      current
        ? {
            ...current,
            [section]: { ...current[section], ...patch },
          }
        : current,
    );
  };

  const handleReset = () => {
    if (backendState) {
      setBackendConfig(backendState.config);
    }
    setLocalSettings(settings);
    setError("");
    setNotice("");
  };

  const handleSave = async () => {
    if (!hasChanges || !backendConfig) {
      return;
    }

    setSaving(true);
    setError("");
    setNotice("");

    try {
      let latestBackendState = backendState;

      if (hasDashboardChanges) {
        setSettings(localSettings);
      }

      if (hasBackendConfigChanges) {
        const saved = await api.saveBackendConfig(backendConfig);
        latestBackendState = saved;
        setBackendState(saved);
        setBackendConfig(saved.config);
      }

      if (hasBackendChanges) {
        setNotice(backendSaveNotice(latestBackendState));
      }

      const health = await api.fetchHealth().catch(() => null);
      if (health) {
        setServerInfo(health);
      }

      if (!hasBackendChanges) {
        setNotice("Dashboard preferences saved in this browser.");
      }
    } catch (e) {
      if (api.isApiError(e) && e.code === "elevation_required") {
        setElevationToken("");
        setElevationError("");
        setPendingElevatedAction("save");
        return;
      }
      setError(e instanceof Error ? e.message : "Failed to save settings");
    } finally {
      setSaving(false);
    }
  };

  const closeElevationPrompt = () => {
    if (elevating) {
      return;
    }
    setPendingElevatedAction(null);
    setElevationToken("");
    setElevationError("");
  };

  const handleElevationSubmit = async (event: FormEvent<HTMLFormElement>) => {
    event.preventDefault();
    if (!pendingElevatedAction) {
      return;
    }

    const action = pendingElevatedAction;
    setElevating(true);
    setElevationError("");

    try {
      await api.elevate(elevationToken);
      await storeTokenCredential(elevationToken, event.currentTarget);
      setPendingElevatedAction(null);
      setElevationToken("");

      if (action === "save") {
        await handleSave();
      }
    } catch (e) {
      setElevationError(
        e instanceof Error ? e.message : "Failed to verify API token",
      );
    } finally {
      setElevating(false);
    }
  };

  return (
    <div className="flex h-full flex-col overflow-hidden">
      <div className="sticky top-0 z-10 border-b border-border-subtle bg-bg-surface/95 px-4 py-3 backdrop-blur">
        <div className="flex flex-col gap-3 lg:flex-row lg:items-center lg:justify-between">
          <div>
            <div className="dashboard-mono text-sm text-text-secondary">
              Server Version: {serverInfo?.version || "Unknown version"}
            </div>
          </div>
          <div className="flex flex-wrap items-center gap-2">
            {restartRequired && (
              <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs font-semibold uppercase tracking-[0.08em] text-warning">
                Restart required
              </div>
            )}
            <Button
              variant="secondary"
              onClick={handleReset}
              disabled={!hasChanges || saving}
            >
              Reset
            </Button>
            <Button
              variant="primary"
              onClick={handleSave}
              disabled={!hasChanges || saving || !backendConfig}
            >
              {saving ? "Saving..." : "Save"}
            </Button>
          </div>
        </div>
        {(error || notice || restartReasons.length > 0) && (
          <div className="mt-3 flex flex-col gap-2">
            {error && (
              <div className="rounded-sm border border-destructive/35 bg-destructive/10 px-3 py-2 text-sm text-destructive">
                {error}
              </div>
            )}
            {notice && (
              <div className="rounded-sm border border-primary/30 bg-primary/10 px-3 py-2 text-sm text-primary">
                {notice}
              </div>
            )}
            {restartRequired && restartReasons.length > 0 && (
              <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-sm text-warning">
                Restart needed for: {restartReasons.join(", ")}.
              </div>
            )}
          </div>
        )}
      </div>

      <Modal
        open={pendingElevatedAction !== null}
        onClose={closeElevationPrompt}
        title="Confirm admin action"
        actions={
          <>
            <Button
              variant="secondary"
              onClick={closeElevationPrompt}
              disabled={elevating}
            >
              Cancel
            </Button>
            <Button
              variant="primary"
              type="submit"
              form="settings-elevation-form"
              disabled={elevating || elevationToken.trim() === ""}
            >
              {elevating ? "Verifying..." : "Continue"}
            </Button>
          </>
        }
      >
        <form
          id="settings-elevation-form"
          className="space-y-4"
          autoComplete="on"
          onSubmit={handleElevationSubmit}
        >
          <p className="leading-6 text-text-muted">
            Re-enter the API token to save backend configuration changes. The
            elevated session stays active briefly so you do not need to repeat
            this for every admin action.
          </p>
          <input
            id="settings-elevation-username"
            type="text"
            name="username"
            autoComplete="username"
            value={savedCredentialUsername}
            readOnly
            tabIndex={-1}
            aria-hidden="true"
            className="sr-only"
          />
          <Input
            id="settings-elevation-password"
            type="password"
            name="password"
            autoComplete="current-password"
            label="API token"
            placeholder="Paste API token"
            value={elevationToken}
            onChange={(e) => setElevationToken(e.target.value)}
            autoFocus
            spellCheck={false}
            autoCapitalize="none"
          />
          {elevationError && (
            <div className="rounded-sm border border-destructive/35 bg-destructive/10 px-3 py-2 text-xs leading-5 text-destructive">
              {elevationError}
            </div>
          )}
        </form>
      </Modal>

      <div className="flex flex-1 flex-col gap-6 overflow-hidden p-4 lg:flex-row lg:p-6">
        <aside className="flex w-full shrink-0 flex-col gap-4 overflow-y-auto lg:w-72">
          <Card className="p-3">
            <div className="dashboard-section-label mb-3">Sections</div>
            <div className="flex flex-col gap-1.5">
              {sections.map((section) => (
                <button
                  key={section.id}
                  type="button"
                  className={`rounded-sm border px-3 py-2.5 text-left transition-all ${
                    activeSection === section.id
                      ? "border-primary/30 bg-primary/10 text-text-primary"
                      : "border-transparent text-text-secondary hover:border-border-subtle hover:bg-bg-elevated hover:text-text-primary"
                  }`}
                  onClick={() => setActiveSection(section.id)}
                >
                  <div className="text-sm font-medium">{section.label}</div>
                  <div className="mt-1 text-xs leading-5 text-text-muted">
                    {section.description}
                  </div>
                </button>
              ))}
            </div>
          </Card>
        </aside>

        <div className="flex-1 overflow-y-auto pr-1">
          {loading || !backendConfig ? (
            <Card className="p-6">
              <div className="text-sm text-text-muted">Loading settings…</div>
            </Card>
          ) : (
            renderActiveSection(activeSection, {
              apiTokenMissing,
              attachWildcard,
              backendConfig,
              backendState,
              idpiDomainsConfigured,
              idpiEnabled,
              idpiWildcard,
              localSettings,
              nonLoopbackBind,
              sensitiveEndpointsEnabled,
              setLocalSettings,
              updateBackendSection,
            })
          )}
        </div>
      </div>
    </div>
  );
}
