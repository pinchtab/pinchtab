import { useEffect, useMemo, useState } from "react";
import { Button, Input, Modal } from "../atoms";
import { useAppStore } from "../../stores/useAppStore";
import * as api from "../../services/api";
import type { LaunchInstanceRequest, Profile } from "../../generated/types";
import { Trans, useTranslation } from "react-i18next";

interface Props {
  open: boolean;
  profile: Profile | null;
  onClose: () => void;
}

const PORT_VALIDATION_ERROR_KEY = "instanceStart.portInvalid";

function normalizeLaunchPort(rawPort: string): {
  port?: string;
  errorKey?: typeof PORT_VALIDATION_ERROR_KEY;
} {
  const trimmed = rawPort.trim();
  if (!trimmed) {
    return {};
  }

  if (!/^\d+$/.test(trimmed)) {
    return { errorKey: PORT_VALIDATION_ERROR_KEY };
  }

  const numericPort = Number(trimmed);
  if (numericPort < 1 || numericPort > 65535) {
    return { errorKey: PORT_VALIDATION_ERROR_KEY };
  }

  return { port: String(numericPort) };
}

export default function StartInstanceModal({ open, profile, onClose }: Props) {
  const { t } = useTranslation();
  const { setInstances } = useAppStore();
  const [port, setPort] = useState("");
  const [headless, setHeadless] = useState(false);
  const [browser, setBrowser] = useState<string>("");
  const [launchError, setLaunchError] = useState("");
  const [launchLoading, setLaunchLoading] = useState(false);
  const [copyFeedback, setCopyFeedback] = useState("");
  const { port: normalizedPort, errorKey: portErrorKey } = useMemo(
    () => normalizeLaunchPort(port),
    [port],
  );
  const portError = portErrorKey ? t(portErrorKey) : "";

  useEffect(() => {
    if (open) {
      setLaunchError("");
      setCopyFeedback("");
      return;
    }

    setPort("");
    setHeadless(false);
    setBrowser("");
    setLaunchError("");
    setLaunchLoading(false);
    setCopyFeedback("");
  }, [open, profile?.id, profile?.name]);

  const launchCommand = useMemo(() => {
    if (!profile?.id) return "";

    const payload: LaunchInstanceRequest = {
      profileId: profile.id,
      mode: headless ? undefined : "headed",
      port: normalizedPort,
      browser: browser || undefined,
    };

    return `curl -X POST http://localhost:9867/instances/start -H "Content-Type: application/json" -H "Authorization: Bearer <token>" -d '${JSON.stringify(payload)}'`;
  }, [browser, headless, normalizedPort, profile]);

  const handleLaunch = async () => {
    if (!profile || launchLoading) return;
    if (!profile.id) {
      setLaunchError(t("instanceStart.profileIdMissing"));
      return;
    }
    if (portError) {
      setLaunchError(portError);
      return;
    }

    setLaunchError("");
    setLaunchLoading(true);

    try {
      const payload: LaunchInstanceRequest = {
        profileId: profile.id,
        port: normalizedPort,
        mode: headless ? undefined : "headed",
        browser: browser || undefined,
      };

      await api.launchInstance(payload);
      const updated = await api.fetchInstances();
      setInstances(updated);
      onClose();
    } catch (e) {
      console.error("Launch failed:", e);
      const msg =
        e instanceof Error ? e.message : t("instanceStart.launchFailed");
      setLaunchError(msg);
    } finally {
      setLaunchLoading(false);
    }
  };

  const handleCopyCommand = async () => {
    try {
      await navigator.clipboard.writeText(launchCommand);
      setCopyFeedback(t("common.copied"));
      setTimeout(() => setCopyFeedback(""), 2000);
    } catch {
      setCopyFeedback(t("common.copyFailed"));
      setTimeout(() => setCopyFeedback(""), 2000);
    }
  };

  return (
    <Modal
      open={open}
      onClose={() => {
        if (!launchLoading) {
          onClose();
        }
      }}
      title={t("instanceStart.title")}
      actions={
        <>
          <Button
            variant="secondary"
            disabled={launchLoading}
            onClick={onClose}
          >
            {t("common.cancel")}
          </Button>
          <Button
            variant="primary"
            onClick={handleLaunch}
            loading={launchLoading}
            disabled={Boolean(portError)}
          >
            {t("instanceStart.start")}
          </Button>
        </>
      }
    >
      <div className="flex flex-col gap-4">
        {launchError && (
          <div className="rounded border border-destructive/50 bg-destructive/10 px-3 py-2 text-sm text-destructive">
            {launchError}
          </div>
        )}
        <Input
          label={t("instanceStart.port")}
          placeholder={t("instanceStart.portPlaceholder")}
          value={port}
          onChange={(e) => setPort(e.target.value)}
        />
        {portError ? (
          <p className="-mt-2 text-xs text-destructive">{portError}</p>
        ) : (
          <p className="-mt-2 text-xs text-text-muted">
            {t("instanceStart.portHint")}
          </p>
        )}
        <label className="flex items-center gap-2 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={headless}
            onChange={(e) => setHeadless(e.target.checked)}
            className="h-4 w-4"
          />
          {t("instanceStart.headless")}
        </label>

        <div>
          <label className="mb-1 block text-xs text-text-muted">
            {t("instanceStart.browser")}
          </label>
          <select
            value={browser}
            onChange={(e) => setBrowser(e.target.value)}
            className="w-full rounded border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary"
          >
            <option value="">{t("instanceStart.browserDefault")}</option>
            <option value="chrome">
              {t("settings.browser.provider.chrome")}
            </option>
            <option value="cloak">
              {t("settings.browser.provider.cloak")}
            </option>
            <option value="ghost-chrome">
              {t("settings.browser.provider.ghostChrome")}
            </option>
          </select>
        </div>

        <div>
          <label className="mb-1 block text-xs text-text-muted">
            {t("instanceStart.commandLabel")}
          </label>
          <textarea
            readOnly
            value={launchCommand}
            className="h-20 w-full resize-none rounded border border-border-subtle bg-bg-elevated px-3 py-2 font-mono text-xs text-text-secondary"
          />
          <div className="mt-2 flex items-center gap-2">
            <Button size="sm" variant="secondary" onClick={handleCopyCommand}>
              {t("instanceStart.copyCommand")}
            </Button>
            {copyFeedback && (
              <span className="text-xs text-success">{copyFeedback}</span>
            )}
          </div>
          <p className="mt-2 text-xs text-text-muted">
            <Trans
              i18nKey="instanceStart.commandHint"
              values={{ token: "<token>" }}
              components={{ code: <code /> }}
            />
          </p>
        </div>
      </div>
    </Modal>
  );
}
