import { useEffect, useMemo, useState } from "react";
import { Button, Input, Modal } from "../atoms";
import { useAppStore } from "../../stores/useAppStore";
import * as api from "../../services/api";
import type { LaunchInstanceRequest, Profile } from "../../generated/types";

interface Props {
  open: boolean;
  profile: Profile | null;
  onClose: () => void;
}

const PORT_VALIDATION_ERROR =
  "Port must be a whole number between 1 and 65535.";

function normalizeLaunchPort(rawPort: string): {
  port?: string;
  error?: string;
} {
  const trimmed = rawPort.trim();
  if (!trimmed) {
    return {};
  }

  if (!/^\d+$/.test(trimmed)) {
    return { error: PORT_VALIDATION_ERROR };
  }

  const numericPort = Number(trimmed);
  if (numericPort < 1 || numericPort > 65535) {
    return { error: PORT_VALIDATION_ERROR };
  }

  return { port: String(numericPort) };
}

export default function StartInstanceModal({ open, profile, onClose }: Props) {
  const { setInstances } = useAppStore();
  const [port, setPort] = useState("");
  const [headless, setHeadless] = useState(false);
  const [launchError, setLaunchError] = useState("");
  const [launchLoading, setLaunchLoading] = useState(false);
  const [copyFeedback, setCopyFeedback] = useState("");
  const { port: normalizedPort, error: portError } = useMemo(
    () => normalizeLaunchPort(port),
    [port],
  );

  useEffect(() => {
    if (open) {
      setLaunchError("");
      setCopyFeedback("");
      return;
    }

    setPort("");
    setHeadless(false);
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
    };

    return `curl -X POST http://localhost:9867/instances/start -H "Content-Type: application/json" -H "Authorization: Bearer <token>" -d '${JSON.stringify(payload)}'`;
  }, [headless, normalizedPort, profile]);

  const handleLaunch = async () => {
    if (!profile || launchLoading) return;
    if (!profile.id) {
      setLaunchError("Profile ID missing");
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
      };

      await api.launchInstance(payload);
      const updated = await api.fetchInstances();
      setInstances(updated);
      onClose();
    } catch (e) {
      console.error("Launch failed:", e);
      const msg = e instanceof Error ? e.message : "Failed to launch instance";
      setLaunchError(msg);
    } finally {
      setLaunchLoading(false);
    }
  };

  const handleCopyCommand = async () => {
    try {
      await navigator.clipboard.writeText(launchCommand);
      setCopyFeedback("Copied!");
      setTimeout(() => setCopyFeedback(""), 2000);
    } catch {
      setCopyFeedback("Failed to copy");
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
      title="🖥️ Start Profile"
      actions={
        <>
          <Button
            variant="secondary"
            disabled={launchLoading}
            onClick={onClose}
          >
            Cancel
          </Button>
          <Button
            variant="primary"
            onClick={handleLaunch}
            loading={launchLoading}
            disabled={Boolean(portError)}
          >
            Start
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
          label="Port"
          placeholder="Auto-select from configured range"
          value={port}
          onChange={(e) => setPort(e.target.value)}
        />
        {portError ? (
          <p className="-mt-2 text-xs text-destructive">{portError}</p>
        ) : (
          <p className="-mt-2 text-xs text-text-muted">
            Leave blank to auto-select a free port from the configured instance
            port range.
          </p>
        )}
        <label className="flex items-center gap-2 text-sm text-text-secondary">
          <input
            type="checkbox"
            checked={headless}
            onChange={(e) => setHeadless(e.target.checked)}
            className="h-4 w-4"
          />
          Headless (best for Docker/VPS)
        </label>

        <div>
          <label className="mb-1 block text-xs text-text-muted">
            Direct launch command (backup)
          </label>
          <textarea
            readOnly
            value={launchCommand}
            className="h-20 w-full resize-none rounded border border-border-subtle bg-bg-elevated px-3 py-2 font-mono text-xs text-text-secondary"
          />
          <div className="mt-2 flex items-center gap-2">
            <Button size="sm" variant="secondary" onClick={handleCopyCommand}>
              Copy Command
            </Button>
            {copyFeedback && (
              <span className="text-xs text-success">{copyFeedback}</span>
            )}
          </div>
          <p className="mt-2 text-xs text-text-muted">
            Replace <code>{"<token>"}</code> with the value from{" "}
            <code>pinchtab config token</code> when auth is enabled.
          </p>
        </div>
      </div>
    </Modal>
  );
}
