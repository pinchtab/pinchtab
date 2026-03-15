import { useState, useEffect, useCallback } from "react";
import { Button, Input, Badge } from "../atoms";
import ScreencastTile from "./ScreencastTile";
import InstanceLogsPanel from "./InstanceLogsPanel";
import { InstanceTabsPanel } from "../tabs";
import type { Profile, Instance, InstanceTab } from "../../generated/types";
import * as api from "../../services/api";

interface Props {
  profile: Profile | null;
  instance?: Instance;
  onLaunch: () => void;
  onStop?: () => void;
  onSave?: (name: string, useWhen: string) => void;
  onDelete?: () => void;
}

type TabId = "profile" | "live" | "tabs" | "agents" | "logs";

function MetaBlock({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <div className="rounded-xl border border-border-subtle bg-black/10 p-4">
      <div className="dashboard-section-title mb-2 text-[0.68rem]">{label}</div>
      {children}
    </div>
  );
}

export default function ProfileDetailsPanel({
  profile,
  instance,
  onLaunch,
  onStop,
  onSave,
  onDelete,
}: Props) {
  const [activeTab, setActiveTab] = useState<TabId>("profile");
  const [name, setName] = useState("");
  const [useWhen, setUseWhen] = useState("");
  const [tabs, setTabs] = useState<InstanceTab[]>([]);
  const [copyFeedback, setCopyFeedback] = useState("");

  const isRunning = instance?.status === "running";

  useEffect(() => {
    if (profile) {
      setName(profile.name);
      setUseWhen(profile.useWhen || "");
      setCopyFeedback("");
    } else {
      setName("");
      setUseWhen("");
      setTabs([]);
    }
  }, [profile]);

  const loadTabs = useCallback(async () => {
    if (!instance?.id) {
      setTabs([]);
      return;
    }

    try {
      const instanceTabs = await api
        .fetchInstanceTabs(instance.id)
        .catch(() => []);
      setTabs(instanceTabs);
    } catch (e) {
      console.error("Failed to load tabs", e);
    }
  }, [instance]);

  useEffect(() => {
    if (activeTab === "live" || activeTab === "tabs" || activeTab === "logs") {
      loadTabs();
    }
  }, [activeTab, loadTabs]);

  const handleCopyId = async () => {
    if (!profile?.id) return;
    try {
      await navigator.clipboard.writeText(profile.id);
      setCopyFeedback("Copied");
      setTimeout(() => setCopyFeedback(""), 2000);
    } catch {
      setCopyFeedback("Failed");
      setTimeout(() => setCopyFeedback(""), 2000);
    }
  };

  const handleSave = () => {
    onSave?.(name, useWhen);
  };

  const tabClasses = (id: TabId) =>
    `rounded-sm px-3 py-2 text-sm font-medium transition-colors ${
      activeTab === id
        ? "bg-bg-hover text-text-primary"
        : "text-text-muted hover:bg-bg-hover hover:text-text-secondary"
    }`;

  if (!profile) {
    return (
      <div className="dashboard-panel flex h-full min-h-[28rem] items-center justify-center p-6 text-center text-sm text-text-muted">
        Select a profile to inspect its instance, live tabs, and logs.
      </div>
    );
  }

  const statusVariant =
    instance?.status === "running"
      ? "success"
      : instance?.status === "error"
        ? "danger"
        : "default";
  const statusLabel =
    instance?.status === "running"
      ? `Running on :${instance.port}`
      : instance?.status === "error"
        ? "Error"
        : "Stopped";
  const accountText = profile.accountEmail || profile.accountName || "";
  const sizeText = profile.sizeMB ? `${profile.sizeMB.toFixed(0)} MB` : "—";
  const browserType = instance?.attached
    ? "Attached via CDP"
    : instance?.headless
      ? "Headless"
      : "Headed";
  const headerMeta = [
    profile.chromeProfileName ? `Chrome ${profile.chromeProfileName}` : null,
    instance?.attached ? "CDP attached" : null,
  ].filter(Boolean);
  const hasChanges =
    name.trim() !== profile.name || useWhen !== (profile.useWhen || "");

  return (
    <div className="dashboard-panel flex h-full min-h-[28rem] flex-col overflow-hidden">
      <div className="border-b border-border-subtle px-4 py-3 lg:px-5">
        <div className="flex flex-col gap-3 xl:flex-row xl:items-center xl:justify-between">
          <div className="min-w-0">
            <div className="flex flex-wrap items-center gap-3">
              <h2 className="truncate text-lg font-semibold text-text-primary">
                {profile.name}
              </h2>
              <Badge variant={statusVariant}>{statusLabel}</Badge>
              {headerMeta.length > 0 && (
                <div className="text-sm text-text-muted">
                  {headerMeta.join(" · ")}
                </div>
              )}
            </div>
            {profile.useWhen?.trim() && (
              <p className="mt-1 text-sm text-text-muted">{profile.useWhen}</p>
            )}
          </div>
          <div className="flex shrink-0 flex-wrap gap-2">
            {profile.id && (
              <Button size="sm" variant="secondary" onClick={handleCopyId}>
                {copyFeedback || "Copy ID"}
              </Button>
            )}
            <Button size="sm" variant="secondary" onClick={onDelete}>
              Delete
            </Button>
            <Button
              size="sm"
              variant="primary"
              onClick={handleSave}
              disabled={!name.trim() || !hasChanges}
            >
              Save
            </Button>
            {isRunning ? (
              <Button size="sm" variant="danger" onClick={onStop}>
                Stop
              </Button>
            ) : (
              <Button size="sm" variant="primary" onClick={onLaunch}>
                Start
              </Button>
            )}
          </div>
        </div>
      </div>

      <div className="border-b border-border-subtle px-4 py-3 lg:px-5">
        <div className="flex flex-wrap gap-2">
          <button
            type="button"
            className={tabClasses("profile")}
            onClick={() => setActiveTab("profile")}
          >
            Profile
          </button>
          <button
            type="button"
            className={tabClasses("live")}
            onClick={() => setActiveTab("live")}
          >
            Live
          </button>
          <button
            type="button"
            className={tabClasses("tabs")}
            onClick={() => setActiveTab("tabs")}
          >
            Tabs ({tabs.length})
          </button>
          <button
            type="button"
            className={tabClasses("logs")}
            onClick={() => setActiveTab("logs")}
          >
            Logs
          </button>
        </div>
      </div>

      <div className="min-h-0 flex-1 overflow-hidden px-4 py-4 lg:px-5">
        {activeTab === "profile" && (
          <div className="h-full overflow-auto">
            <div className="grid gap-4 xl:grid-cols-2">
              <div className="space-y-4">
                <Input
                  label="Name"
                  value={name}
                  onChange={(e) => setName(e.target.value)}
                />

                <div>
                  <label className="dashboard-section-title mb-1 block text-[0.68rem]">
                    Use this profile when
                  </label>
                  <textarea
                    value={useWhen}
                    onChange={(e) => setUseWhen(e.target.value)}
                    className="min-h-[180px] w-full resize-y rounded border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary"
                  />
                </div>
              </div>

              <MetaBlock label="Profile panel">
                <div className="space-y-3 text-sm text-text-secondary">
                  <div className="flex items-center justify-between gap-3">
                    <span className="dashboard-section-title text-[0.68rem]">
                      Status
                    </span>
                    <span className="text-right">
                      {instance?.status || "stopped"}
                    </span>
                  </div>
                  {instance?.port && (
                    <div className="flex items-center justify-between gap-3">
                      <span className="dashboard-section-title text-[0.68rem]">
                        Port
                      </span>
                      <span className="text-right">{instance.port}</span>
                    </div>
                  )}
                  <div className="flex items-center justify-between gap-3">
                    <span className="dashboard-section-title text-[0.68rem]">
                      Browser
                    </span>
                    <span className="text-right">{browserType}</span>
                  </div>
                  <div className="flex items-center justify-between gap-3">
                    <span className="dashboard-section-title text-[0.68rem]">
                      Size
                    </span>
                    <span className="text-right">{sizeText}</span>
                  </div>
                  {accountText && (
                    <div className="flex items-center justify-between gap-3">
                      <span className="dashboard-section-title text-[0.68rem]">
                        Account
                      </span>
                      <span className="text-right">{accountText}</span>
                    </div>
                  )}
                  {profile.chromeProfileName && (
                    <div className="flex items-center justify-between gap-3">
                      <span className="dashboard-section-title text-[0.68rem]">
                        Identity
                      </span>
                      <span className="text-right">
                        {profile.chromeProfileName}
                      </span>
                    </div>
                  )}
                  {instance?.attached && (
                    <div className="flex items-center justify-between gap-3">
                      <span className="dashboard-section-title text-[0.68rem]">
                        Connection
                      </span>
                      <span className="text-right">CDP attached</span>
                    </div>
                  )}
                  {instance?.cdpUrl && (
                    <div>
                      <div className="dashboard-section-title mb-1 text-[0.68rem]">
                        CDP URL
                      </div>
                      <code className="dashboard-mono block break-all text-xs text-text-secondary">
                        {instance.cdpUrl}
                      </code>
                    </div>
                  )}
                  {profile.path && (
                    <div>
                      <div className="dashboard-section-title mb-1 text-[0.68rem]">
                        Path
                      </div>
                      <code
                        className={`dashboard-mono block break-all text-xs ${
                          profile.pathExists
                            ? "text-text-secondary"
                            : "text-destructive"
                        }`}
                      >
                        {profile.path}
                        {!profile.pathExists && " (not found)"}
                      </code>
                    </div>
                  )}
                </div>
              </MetaBlock>
            </div>
          </div>
        )}

        {activeTab === "live" && (
          <div className="flex h-full min-h-0 flex-col">
            {isRunning && instance ? (
              tabs.length === 0 ? (
                <div className="flex flex-1 items-center justify-center rounded-xl border border-border-subtle bg-black/10 text-sm text-text-muted">
                  No tabs open
                </div>
              ) : (
                <div className="min-h-0 flex-1 overflow-auto">
                  <div className="flex min-h-full items-center justify-center">
                    <div className="grid w-full max-w-6xl gap-3 xl:grid-cols-2">
                      {tabs.map((tab) => (
                        <ScreencastTile
                          key={tab.id}
                          instanceId={instance.id}
                          tabId={tab.id}
                          label={tab.title?.slice(0, 20) || tab.id.slice(0, 8)}
                          url={tab.url}
                        />
                      ))}
                    </div>
                  </div>
                </div>
              )
            ) : (
              <div className="flex flex-1 items-center justify-center rounded-xl border border-border-subtle bg-black/10 text-sm text-text-muted">
                Instance not running. Start the profile to see live view.
              </div>
            )}
          </div>
        )}

        {activeTab === "tabs" && (
          <div className="flex h-full min-h-0 flex-col">
            <InstanceTabsPanel
              tabs={tabs}
              emptyMessage={
                isRunning ? "No tabs open." : "Instance not running."
              }
            />
          </div>
        )}

        {activeTab === "logs" && (
          <InstanceLogsPanel instanceId={instance?.id} />
        )}
      </div>
    </div>
  );
}
