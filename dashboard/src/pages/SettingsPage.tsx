import type { FormEvent } from "react";
import { useEffect, useMemo, useState } from "react";
import { useAppStore } from "../stores/useAppStore";
import { Button, Card, Input, Modal } from "../components/atoms";
import * as api from "../services/api";
import { credentialUsername, storeTokenCredential } from "../services/auth";
import type {
  BackendConfig,
  BackendConfigState,
  BackendIDPIConfig,
  BackendSecurityConfig,
  LocalDashboardSettings,
} from "../types";

type SectionId =
  | "dashboard"
  | "defaults"
  | "orchestration"
  | "security"
  | "security-idpi"
  | "profiles"
  | "network"
  | "browser"
  | "timeouts";

type PendingElevatedAction = "save" | null;

const sections: Array<{
  id: SectionId;
  label: string;
  description: string;
}> = [
  {
    id: "dashboard",
    label: "Dashboard",
    description: "Local monitoring and screencast preferences.",
  },
  {
    id: "defaults",
    label: "Instance Defaults",
    description: "How new managed browser instances launch.",
  },
  {
    id: "orchestration",
    label: "Orchestration",
    description: "Routing strategy, port range, and allocation policy.",
  },
  {
    id: "security",
    label: "Security",
    description: "Sensitive endpoint gates and access controls.",
  },
  {
    id: "security-idpi",
    label: "Security IDPI",
    description: "Indirect prompt injection website and content defenses.",
  },
  {
    id: "profiles",
    label: "Profiles",
    description: "Shared profile storage and default profile behavior.",
  },
  {
    id: "network",
    label: "Network & Attach",
    description: "Server binding, auth, and attach policy.",
  },
  {
    id: "browser",
    label: "Browser Runtime",
    description: "Chrome binary, version, flags, and extensions.",
  },
  {
    id: "timeouts",
    label: "Timeouts",
    description: "Action, navigation, shutdown, and wait timing.",
  },
];

const fieldClass =
  "w-full rounded-sm border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.72)] px-3 py-2 text-sm text-text-primary placeholder:text-text-muted transition-all duration-150 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20";

function backendSaveNotice(state: BackendConfigState | null): string {
  if (state?.restartRequired) {
    return "Backend config saved. Dynamic changes were applied where possible. Restart advised for server-level changes.";
  }
  return "Backend config saved. Dynamic changes were applied where possible.";
}

const selectClass =
  "rounded-sm border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.72)] px-3 py-2 text-sm text-text-primary transition-all duration-150 focus:border-primary focus:outline-none focus:ring-2 focus:ring-primary/20";

type SecurityEndpointKey = Exclude<keyof BackendSecurityConfig, "attach">;
type IDPIToggleKey = Exclude<
  keyof BackendIDPIConfig,
  "allowedDomains" | "customPatterns"
>;

const securityEndpointRows = [
  ["allowEvaluate", "Allow evaluate"],
  ["allowMacro", "Allow macro"],
  ["allowScreencast", "Allow screencast"],
  ["allowDownload", "Allow download"],
  ["allowUpload", "Allow upload"],
] as const satisfies ReadonlyArray<readonly [SecurityEndpointKey, string]>;

const idpiToggleRows = [
  ["enabled", "Enable IDPI", "Turn on indirect prompt injection defenses."],
  [
    "strictMode",
    "Strict mode",
    "Block disallowed domains and suspicious content instead of only warning.",
  ],
  [
    "scanContent",
    "Scan content",
    "Inspect extracted text and snapshots for prompt-injection patterns.",
  ],
  [
    "wrapContent",
    "Wrap content",
    "Mark returned page text as untrusted content for downstream consumers.",
  ],
] as const satisfies ReadonlyArray<readonly [IDPIToggleKey, string, string]>;

function csvToList(value: string): string[] {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function listToCsv(value: string[]): string {
  return value.join(", ");
}

function SectionCard({
  title,
  description,
  children,
}: {
  title: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <Card className="p-5">
      <div className="mb-5 border-b border-border-subtle pb-4">
        <div className="dashboard-section-label mb-2">Settings</div>
        <h3 className="text-lg font-semibold text-text-primary">{title}</h3>
        <p className="mt-2 max-w-2xl text-sm leading-6 text-text-muted">
          {description}
        </p>
      </div>
      <div className="space-y-4">{children}</div>
    </Card>
  );
}

function SettingRow({
  label,
  description,
  children,
}: {
  label: string;
  description: string;
  children: React.ReactNode;
}) {
  return (
    <div className="flex flex-col gap-3 rounded-sm border border-border-subtle bg-black/10 p-4 lg:flex-row lg:items-center lg:justify-between">
      <div className="max-w-xl">
        <div className="text-sm font-medium text-text-primary">{label}</div>
        <p className="mt-1 text-xs leading-5 text-text-muted">{description}</p>
      </div>
      <div className="w-full max-w-md">{children}</div>
    </div>
  );
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
        const message =
          e instanceof Error ? e.message : "Failed to load settings";
        setError(message);
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

  const updateBackendSection = <K extends keyof BackendConfig>(
    section: K,
    patch: Partial<BackendConfig[K]>,
  ) => {
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
    if (!hasChanges || !backendConfig) return;

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
      const message =
        e instanceof Error ? e.message : "Failed to save settings";
      setError(message);
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
      void storeTokenCredential(elevationToken);
      setPendingElevatedAction(null);
      setElevationToken("");

      if (action === "save") {
        await handleSave();
      }
    } catch (e) {
      const message =
        e instanceof Error ? e.message : "Failed to verify API token";
      setElevationError(message);
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
            <>
              {activeSection === "dashboard" && (
                <SectionCard
                  title="Dashboard Preferences"
                  description="These controls affect this dashboard UI only. They are stored locally in your browser and do not require a backend restart."
                >
                  <SettingRow
                    label="Screencast frame rate"
                    description="Controls how often live previews request new frames."
                  >
                    <div className="flex items-center gap-3">
                      <input
                        type="range"
                        min={1}
                        max={15}
                        value={localSettings.screencast.fps}
                        onChange={(e) =>
                          setLocalSettings({
                            ...localSettings,
                            screencast: {
                              ...localSettings.screencast,
                              fps: Number(e.target.value),
                            },
                          })
                        }
                        className="w-full"
                      />
                      <span className="dashboard-mono w-16 text-right text-sm text-text-secondary">
                        {localSettings.screencast.fps} fps
                      </span>
                    </div>
                  </SettingRow>
                  <SettingRow
                    label="Screencast quality"
                    description="JPEG quality for tab preview streams."
                  >
                    <div className="flex items-center gap-3">
                      <input
                        type="range"
                        min={10}
                        max={80}
                        value={localSettings.screencast.quality}
                        onChange={(e) =>
                          setLocalSettings({
                            ...localSettings,
                            screencast: {
                              ...localSettings.screencast,
                              quality: Number(e.target.value),
                            },
                          })
                        }
                        className="w-full"
                      />
                      <span className="dashboard-mono w-16 text-right text-sm text-text-secondary">
                        {localSettings.screencast.quality}%
                      </span>
                    </div>
                  </SettingRow>
                  <SettingRow
                    label="Screencast width"
                    description="Maximum preview width for live tiles."
                  >
                    <select
                      value={localSettings.screencast.maxWidth}
                      onChange={(e) =>
                        setLocalSettings({
                          ...localSettings,
                          screencast: {
                            ...localSettings.screencast,
                            maxWidth: Number(e.target.value),
                          },
                        })
                      }
                      className={selectClass}
                    >
                      {[400, 600, 800, 1024, 1280].map((width) => (
                        <option key={width} value={width}>
                          {width}px
                        </option>
                      ))}
                    </select>
                  </SettingRow>
                  <SettingRow
                    label="Memory metrics"
                    description="Enable per-tab heap collection in the dashboard. Useful for debugging, but heavier."
                  >
                    <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
                      <input
                        type="checkbox"
                        checked={localSettings.monitoring.memoryMetrics}
                        onChange={(e) =>
                          setLocalSettings({
                            ...localSettings,
                            monitoring: {
                              ...localSettings.monitoring,
                              memoryMetrics: e.target.checked,
                            },
                          })
                        }
                        className="h-4 w-4"
                      />
                      Enable
                    </label>
                  </SettingRow>
                  <SettingRow
                    label="Polling interval"
                    description="How frequently the dashboard asks the backend for fresh metrics."
                  >
                    <div className="flex items-center gap-3">
                      <input
                        type="range"
                        min={5}
                        max={120}
                        step={5}
                        value={localSettings.monitoring.pollInterval}
                        onChange={(e) =>
                          setLocalSettings({
                            ...localSettings,
                            monitoring: {
                              ...localSettings.monitoring,
                              pollInterval: Number(e.target.value),
                            },
                          })
                        }
                        className="w-full"
                      />
                      <span className="dashboard-mono w-16 text-right text-sm text-text-secondary">
                        {localSettings.monitoring.pollInterval}s
                      </span>
                    </div>
                  </SettingRow>
                </SectionCard>
              )}

              {activeSection === "defaults" && (
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
                          mode: e.target
                            .value as BackendConfig["instanceDefaults"]["mode"],
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
                        {backendConfig.instanceDefaults.stealthLevel ===
                          "light" && (
                          <div className="space-y-2">
                            <div>
                              <strong className="text-text-secondary">
                                Light:
                              </strong>{" "}
                              Safe baseline stealth. Hides navigator.webdriver,
                              removes CDP markers, spoofs
                              plugins/languages/hardware.
                            </div>
                            <div className="text-success/80">
                              ✓ No functional side effects
                            </div>
                            <div className="text-success/80">
                              ✓ No security implications — standard automation
                              hiding only
                            </div>
                          </div>
                        )}
                        {backendConfig.instanceDefaults.stealthLevel ===
                          "medium" && (
                          <div className="space-y-2">
                            <div>
                              <strong className="text-warning">Medium:</strong>{" "}
                              Adds Client Hints API, chrome.runtime.connect (for
                              Cloudflare Turnstile), chrome.csi/loadTimes,
                              enhanced permissions, video codec spoofing.
                            </div>
                            <div className="text-warning/80">
                              ⚠ May interfere with error monitoring (Sentry,
                              LogRocket) — Error.prepareStackTrace is blocked
                            </div>
                            <div className="text-warning/80">
                              ⚠ Permissions API returns fake states — code
                              relying on accurate permission checks may
                              misbehave
                            </div>
                            <div className="text-warning/80">
                              ⚠ chrome.runtime.connect returns dummy objects —
                              real extension messaging won't work
                            </div>
                          </div>
                        )}
                        {backendConfig.instanceDefaults.stealthLevel ===
                          "full" && (
                          <div className="space-y-2">
                            <div>
                              <strong className="text-destructive">
                                Full:
                              </strong>{" "}
                              Maximum stealth. Adds WebGL/canvas fingerprint
                              noise, WebRTC IP leak prevention, AudioContext
                              protection.
                            </div>
                            <div className="text-destructive/80">
                              ⚠ WebRTC forced to relay mode — direct P2P
                              connections disabled, video calls may fail
                            </div>
                            <div className="text-destructive/80">
                              ⚠ Canvas operations have subtle pixel noise — may
                              affect pixel-precise rendering or image comparison
                            </div>
                            <div className="text-destructive/80">
                              ⚠ WebGL reports spoofed GPU — applications
                              detecting GPU capabilities may behave unexpectedly
                            </div>
                            <div className="text-destructive/80">
                              ⚠ AudioContext has frequency noise — may affect
                              audio fingerprinting detection or processing apps
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
                      <option value="close_lru">
                        Close least recently used
                      </option>
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
                  {[
                    ["blockImages", "Block images"],
                    ["blockMedia", "Block media"],
                    ["blockAds", "Block ads"],
                    ["noAnimations", "Disable CSS animations"],
                    ["noRestore", "Skip session restore"],
                  ].map(([key, label]) => (
                    <SettingRow
                      key={key}
                      label={label}
                      description="Applies to newly launched managed instances."
                    >
                      <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
                        <input
                          type="checkbox"
                          checked={
                            backendConfig.instanceDefaults[
                              key as keyof BackendConfig["instanceDefaults"]
                            ] as boolean
                          }
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
              )}

              {activeSection === "orchestration" && (
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
                      <option value="simple-autorestart">
                        Simple autorestart
                      </option>
                      <option value="no-instance">No instance (hub)</option>
                    </select>
                    <div className="mt-2 text-[11px] leading-relaxed text-text-muted">
                      {backendConfig.multiInstance.strategy === "always-on" &&
                        "Launches a default instance at boot and relaunches on crash."}
                      {backendConfig.multiInstance.strategy === "simple" &&
                        "Launches one instance on first request. No auto-restart."}
                      {backendConfig.multiInstance.strategy === "explicit" &&
                        "All instances managed via API. No automatic launches."}
                      {backendConfig.multiInstance.strategy ===
                        "simple-autorestart" &&
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
                    backendConfig.multiInstance.strategy ===
                      "simple-autorestart") && (
                    <>
                      <SettingRow
                        label="Max restarts"
                        description="Maximum restart attempts. Use -1 for unlimited, 0 for no restarts."
                      >
                        <input
                          type="number"
                          min={-1}
                          value={
                            backendConfig.multiInstance.restart.maxRestarts
                          }
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
                          value={
                            backendConfig.multiInstance.restart.initBackoffSec
                          }
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
                          value={
                            backendConfig.multiInstance.restart.maxBackoffSec
                          }
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
                          value={
                            backendConfig.multiInstance.restart.stableAfterSec
                          }
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
              )}

              {activeSection === "security" && (
                <SectionCard
                  title="Security"
                  description="These controls define what risky capabilities PinchTab exposes."
                >
                  <div
                    className={`rounded-sm px-4 py-3 text-sm leading-6 ${
                      sensitiveEndpointsEnabled
                        ? "border border-destructive/35 bg-destructive/10 text-destructive"
                        : "border border-warning/25 bg-warning/10 text-warning"
                    }`}
                  >
                    {sensitiveEndpointsEnabled
                      ? "One or more sensitive endpoint families are enabled. Features like script execution, downloads, uploads, and live capture can expose high-risk capabilities. Only enable them in trusted environments. You are responsible for securing network access, authentication, and downstream use."
                      : "These endpoint families can expose high-risk capabilities when enabled. Only turn them on in trusted environments, and only when you accept responsibility for network access, authentication, and downstream use."}
                  </div>
                  {securityEndpointRows.map(([key, label]) => (
                    <SettingRow
                      key={key}
                      label={label}
                      description="Controls whether the corresponding endpoint family is enabled."
                    >
                      <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
                        <input
                          type="checkbox"
                          checked={backendConfig.security[key]}
                          onChange={(e) =>
                            updateBackendSection("security", {
                              [key]: e.target.checked,
                            } as Partial<
                              Pick<BackendSecurityConfig, SecurityEndpointKey>
                            >)
                          }
                          className="h-4 w-4"
                        />
                        Enable
                      </label>
                    </SettingRow>
                  ))}
                </SectionCard>
              )}

              {activeSection === "security-idpi" && (
                <SectionCard
                  title="Security IDPI"
                  description="Indirect prompt injection controls restrict which websites are allowed and add protections around extracted content before it reaches downstream automation."
                >
                  <div
                    className={`mb-4 rounded-sm px-4 py-3 text-sm leading-6 ${
                      !idpiEnabled || !idpiDomainsConfigured
                        ? "border border-destructive/35 bg-destructive/10 text-destructive"
                        : idpiWildcard
                          ? "border border-warning/25 bg-warning/10 text-warning"
                          : "border border-success/25 bg-success/10 text-success"
                    }`}
                  >
                    {!idpiEnabled
                      ? "IDPI is disabled. Browser content is not being filtered by website allowlist or content protections."
                      : !idpiDomainsConfigured
                        ? "The website whitelist is not set to a restricted domain list. This is the main IDPI defense and should be configured."
                        : idpiWildcard
                          ? "The website whitelist contains '*', which effectively disables domain restriction."
                          : "IDPI is enforcing a specific website whitelist and content protections."}
                  </div>
                  {idpiToggleRows.map(([key, label, description]) => (
                    <SettingRow
                      key={key}
                      label={label}
                      description={description}
                    >
                      <label className="flex items-center justify-end gap-3 text-sm text-text-secondary">
                        <input
                          type="checkbox"
                          checked={backendConfig.security.idpi[key]}
                          onChange={(e) =>
                            updateBackendSection("security", {
                              idpi: {
                                ...backendConfig.security.idpi,
                                [key]: e.target.checked,
                              },
                            })
                          }
                          className="h-4 w-4"
                        />
                        Enable
                      </label>
                    </SettingRow>
                  ))}
                  <SettingRow
                    label="Allowed websites"
                    description="Comma-separated domain allowlist for web content. Use exact hosts or patterns like *.example.com."
                  >
                    <div className="space-y-2">
                      <input
                        value={listToCsv(
                          backendConfig.security.idpi.allowedDomains,
                        )}
                        onChange={(e) =>
                          updateBackendSection("security", {
                            idpi: {
                              ...backendConfig.security.idpi,
                              allowedDomains: csvToList(e.target.value),
                            },
                          })
                        }
                        className={fieldClass}
                        placeholder="127.0.0.1, localhost, ::1"
                      />
                      <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
                        Keep this list narrow. Empty or wildcard entries weaken
                        the main IDPI boundary.
                      </div>
                    </div>
                  </SettingRow>
                  <SettingRow
                    label="Custom patterns"
                    description="Optional comma-separated phrases to treat as suspicious prompt-injection content."
                  >
                    <input
                      value={listToCsv(
                        backendConfig.security.idpi.customPatterns,
                      )}
                      onChange={(e) =>
                        updateBackendSection("security", {
                          idpi: {
                            ...backendConfig.security.idpi,
                            customPatterns: csvToList(e.target.value),
                          },
                        })
                      }
                      className={fieldClass}
                      placeholder="ignore previous instructions, exfiltrate data"
                    />
                  </SettingRow>
                </SectionCard>
              )}

              {activeSection === "profiles" && (
                <SectionCard
                  title="Profiles"
                  description="Profile storage is host-level. Changing the base directory requires restart because the profile manager and orchestrator are created with it at boot."
                >
                  <SettingRow
                    label="Profiles base directory"
                    description="Root directory where browser profiles are stored."
                  >
                    <input
                      value={backendConfig.profiles.baseDir}
                      onChange={(e) =>
                        updateBackendSection("profiles", {
                          baseDir: e.target.value,
                        })
                      }
                      className={fieldClass}
                    />
                  </SettingRow>
                  <SettingRow
                    label="Default profile"
                    description="Profile name used when the server needs an implicit default."
                  >
                    <input
                      value={backendConfig.profiles.defaultProfile}
                      onChange={(e) =>
                        updateBackendSection("profiles", {
                          defaultProfile: e.target.value,
                        })
                      }
                      className={fieldClass}
                    />
                  </SettingRow>
                </SectionCard>
              )}

              {activeSection === "network" && (
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
                    description="Network interface the dashboard process binds to."
                  >
                    <input
                      value={backendConfig.server.bind}
                      onChange={(e) =>
                        updateBackendSection("server", { bind: e.target.value })
                      }
                      className={fieldClass}
                    />
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
                          No API token is set. Anyone who can reach this server
                          can access exposed endpoints. Keep it on trusted local
                          networks only, or configure a strong token through the
                          CLI or config file. You are responsible for protecting
                          access.
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
                        checked={
                          backendConfig.server.trustProxyHeaders ?? false
                        }
                        onChange={(e) =>
                          updateBackendSection("server", {
                            trustProxyHeaders: e.target.checked,
                          })
                        }
                        className="accent-primary"
                      />
                      {backendConfig.server.trustProxyHeaders
                        ? "Enabled"
                        : "Disabled"}
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
                    description="Comma-separated host allowlist for attach requests. Only include hosts you control and trust."
                  >
                    <div className="space-y-2">
                      <input
                        value={listToCsv(
                          backendConfig.security.attach.allowHosts,
                        )}
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
                      <div className="rounded-sm border border-warning/25 bg-warning/10 px-3 py-2 text-xs leading-5 text-warning">
                        Hosts in this allowlist may be used for remote attach
                        requests. Broad or untrusted entries can expose external
                        Chrome sessions and browser contents.
                      </div>
                    </div>
                  </SettingRow>
                  <SettingRow
                    label="Allowed attach schemes"
                    description="Comma-separated scheme allowlist, usually ws and wss."
                  >
                    <input
                      value={listToCsv(
                        backendConfig.security.attach.allowSchemes,
                      )}
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
              )}

              {activeSection === "browser" && (
                <SectionCard
                  title="Browser Runtime"
                  description="These settings are written into the generated child config for new managed instances."
                >
                  <SettingRow
                    label="Chrome version"
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
                    label="Chrome binary"
                    description="Optional path override for the Chrome executable."
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
                    description="Comma-separated extension directories to load."
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
              )}

              {activeSection === "timeouts" && (
                <SectionCard
                  title="Timeouts"
                  description="Runtime timing defaults written into new child configs. Existing running instances keep their current timeouts."
                >
                  {[
                    [
                      "actionSec",
                      "Action timeout",
                      "Maximum time for action requests.",
                    ],
                    [
                      "navigateSec",
                      "Navigate timeout",
                      "Maximum time for navigation requests.",
                    ],
                    [
                      "shutdownSec",
                      "Shutdown timeout",
                      "Grace period before force-closing a child process.",
                    ],
                    [
                      "waitNavMs",
                      "Wait-after-navigation delay",
                      "Post-navigation stabilization delay in milliseconds.",
                    ],
                  ].map(([key, label, description]) => (
                    <SettingRow
                      key={key}
                      label={label}
                      description={description}
                    >
                      <input
                        type="number"
                        min={0}
                        value={
                          backendConfig.timeouts[
                            key as keyof BackendConfig["timeouts"]
                          ]
                        }
                        onChange={(e) =>
                          updateBackendSection("timeouts", {
                            [key]: Number(e.target.value),
                          } as Partial<BackendConfig["timeouts"]>)
                        }
                        className={fieldClass}
                      />
                    </SettingRow>
                  ))}
                </SectionCard>
              )}
            </>
          )}
        </div>
      </div>
    </div>
  );
}
