import type { Dispatch, FormEvent, SetStateAction } from "react";
import { useEffect, useMemo, useState } from "react";
import * as api from "../../services/api";
import { useAppStore } from "../../stores/useAppStore";
import type {
  BackendConfig,
  BackendConfigState,
  DashboardServerInfo,
  LocalDashboardSettings,
} from "../../types";
import { deepEqual } from "./deepEqual";
import { backendSaveNotice, type UpdateBackendSection } from "./settingsShared";

export type PendingElevatedAction = "save" | null;

export interface UseSettingsControllerResult {
  serverInfo: DashboardServerInfo | null;
  localSettings: LocalDashboardSettings;
  setLocalSettings: Dispatch<SetStateAction<LocalDashboardSettings>>;
  backendState: BackendConfigState | null;
  backendConfig: BackendConfig | null;
  loading: boolean;
  saving: boolean;
  error: string;
  notice: string;
  pendingElevatedAction: PendingElevatedAction;
  elevationToken: string;
  setElevationToken: Dispatch<SetStateAction<string>>;
  elevationError: string;
  elevating: boolean;
  hasChanges: boolean;
  restartRequired: boolean;
  restartReasons: string[];
  sensitiveEndpointsEnabled: boolean;
  apiTokenMissing: boolean;
  idpiEnabled: boolean;
  idpiWildcard: boolean;
  idpiDomainsConfigured: boolean;
  attachWildcard: boolean;
  nonLoopbackBind: boolean;
  updateBackendSection: UpdateBackendSection;
  handleReset: () => void;
  handleSave: () => Promise<void>;
  closeElevationPrompt: () => void;
  handleElevationSubmit: (event: FormEvent<HTMLFormElement>) => Promise<void>;
}

export function useSettingsController(): UseSettingsControllerResult {
  const { settings, setSettings, serverInfo, setServerInfo } = useAppStore();
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
    () => !deepEqual(localSettings, settings),
    [localSettings, settings],
  );

  const hasBackendConfigChanges = useMemo(
    () =>
      Boolean(
        backendConfig &&
        backendState &&
        !deepEqual(backendConfig, backendState.config),
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
        backendConfig.security.allowCookies,
        backendConfig.security.allowUpload,
      ].some(Boolean)
    : false;
  const apiTokenMissing = !backendState?.tokenConfigured;
  const idpiEnabled = backendConfig
    ? backendConfig.security.idpi.enabled
    : false;
  const idpiAllowedDomains = backendConfig
    ? backendConfig.security.allowedDomains
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

  return {
    serverInfo,
    localSettings,
    setLocalSettings,
    backendState,
    backendConfig,
    loading,
    saving,
    error,
    notice,
    pendingElevatedAction,
    elevationToken,
    setElevationToken,
    elevationError,
    elevating,
    hasChanges,
    restartRequired,
    restartReasons,
    sensitiveEndpointsEnabled,
    apiTokenMissing,
    idpiEnabled,
    idpiWildcard,
    idpiDomainsConfigured,
    attachWildcard,
    nonLoopbackBind,
    updateBackendSection,
    handleReset,
    handleSave,
    closeElevationPrompt,
    handleElevationSubmit,
  };
}
