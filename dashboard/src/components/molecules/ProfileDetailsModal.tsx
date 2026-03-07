import { useState, useEffect, useCallback } from "react";
import { Modal, Button, Input } from "../atoms";
import ScreencastTile from "./ScreencastTile";
import TabItem from "./TabItem";
import type { Profile, Instance, InstanceTab } from "../../generated/types";
import * as api from "../../services/api";

interface Props {
  profile: Profile | null;
  instance?: Instance;
  onClose: () => void;
  onSave?: (name: string, useWhen: string) => void;
  onDelete?: () => void;
}

type TabId = "profile" | "live" | "logs";

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex justify-between py-1 text-sm">
      <span className="text-text-muted">{label}</span>
      <span className="text-text-primary">{value}</span>
    </div>
  );
}

export default function ProfileDetailsModal({
  profile,
  instance,
  onClose,
  onSave,
  onDelete,
}: Props) {
  const [activeTab, setActiveTab] = useState<TabId>("profile");
  const [name, setName] = useState("");
  const [useWhen, setUseWhen] = useState("");
  const [tabs, setTabs] = useState<InstanceTab[]>([]);
  const [logs] = useState(""); // TODO: fetch from /instances/{id}/logs
  const [copyFeedback, setCopyFeedback] = useState("");

  const isRunning = instance?.status === "running";

  // Initialize form values when profile changes
  useEffect(() => {
    if (profile) {
      setName(profile.name);
      setUseWhen(profile.useWhen || "");
    }
  }, [profile]);

  // Load live data when switching to live/logs tab
  const loadLiveData = useCallback(async () => {
    if (!instance?.id) return;

    try {
      const allTabs = await api.fetchAllTabs().catch(() => []);
      // Filter tabs for this instance
      const instanceTabs = Array.isArray(allTabs)
        ? allTabs.filter((t) => t.instanceId === instance.id)
        : [];
      setTabs(instanceTabs);
    } catch (e) {
      console.error("Failed to load live data", e);
    }
  }, [instance]);

  useEffect(() => {
    if (activeTab === "live" || activeTab === "logs") {
      loadLiveData();
    }
  }, [activeTab, loadLiveData]);

  const handleCopyId = async () => {
    if (!profile?.id) return;
    try {
      await navigator.clipboard.writeText(profile.id);
      setCopyFeedback("Copied!");
      setTimeout(() => setCopyFeedback(""), 2000);
    } catch {
      setCopyFeedback("Failed");
      setTimeout(() => setCopyFeedback(""), 2000);
    }
  };

  const handleSave = () => {
    onSave?.(name, useWhen);
  };

  if (!profile) return null;

  const tabClasses = (id: TabId) =>
    `flex-1 py-2 text-sm font-medium border-b-2 transition-colors ${
      activeTab === id
        ? "border-primary text-text-primary"
        : "border-transparent text-text-muted hover:text-text-secondary"
    }`;

  return (
    <Modal
      open={!!profile}
      onClose={onClose}
      title={profile.name}
      wide
      actions={
        <div className="flex w-full items-center justify-between">
          <Button variant="danger" onClick={onDelete}>
            Delete
          </Button>
          <div className="flex gap-2">
            <Button variant="primary" onClick={handleSave}>
              Save
            </Button>
            <Button variant="secondary" onClick={onClose}>
              Close
            </Button>
          </div>
        </div>
      }
    >
      {/* Tabs */}
      <div className="-mx-4 -mt-2 mb-4 flex border-b border-border-subtle px-4">
        <button
          className={tabClasses("profile")}
          onClick={() => setActiveTab("profile")}
        >
          Profile
        </button>
        <button
          className={tabClasses("live")}
          onClick={() => setActiveTab("live")}
        >
          Live
        </button>
        <button
          className={tabClasses("logs")}
          onClick={() => setActiveTab("logs")}
        >
          Logs
        </button>
      </div>

      {/* Tab Content */}
      <div className="min-h-[400px]">
        {activeTab === "profile" && (
          <div className="flex flex-col gap-4">
            {/* Profile ID */}
            <div>
              <label className="mb-1 block text-xs text-text-muted">
                Profile ID
              </label>
              <div className="flex gap-2">
                <input
                  readOnly
                  value={profile.id || "—"}
                  className="flex-1 rounded border border-border-subtle bg-bg-elevated px-3 py-2 font-mono text-xs text-text-secondary"
                />
                <Button size="sm" variant="secondary" onClick={handleCopyId}>
                  {copyFeedback || "Copy"}
                </Button>
              </div>
            </div>

            {/* Name */}
            <Input
              label="Name"
              value={name}
              onChange={(e) => setName(e.target.value)}
            />

            {/* Use When */}
            <div>
              <label className="mb-1 block text-xs text-text-muted">
                Use this profile when
              </label>
              <textarea
                value={useWhen}
                onChange={(e) => setUseWhen(e.target.value)}
                className="min-h-[80px] w-full resize-y rounded border border-border-subtle bg-bg-elevated px-3 py-2 text-sm text-text-primary"
              />
            </div>

            {/* Status */}
            <div className="rounded border border-border-subtle bg-bg-elevated p-3">
              <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">
                Status
              </h4>
              <div className="space-y-1">
                <InfoRow label="State" value={instance?.status || "stopped"} />
                <InfoRow label="Port" value={instance?.port || "—"} />
                <InfoRow
                  label="Size"
                  value={
                    profile.sizeMB ? `${profile.sizeMB.toFixed(0)} MB` : "—"
                  }
                />
                <InfoRow label="Source" value={profile.source || "—"} />
                {profile.chromeProfileName && (
                  <InfoRow label="Chrome" value={profile.chromeProfileName} />
                )}
                {(profile.accountEmail || profile.accountName) && (
                  <InfoRow
                    label="Account"
                    value={profile.accountEmail || profile.accountName || ""}
                  />
                )}
              </div>
            </div>

            {/* Path */}
            {profile.path && (
              <div className="rounded border border-border-subtle bg-bg-elevated p-3">
                <h4 className="mb-1 text-xs font-semibold uppercase tracking-wide text-text-muted">
                  Path
                </h4>
                <code
                  className={`text-xs ${profile.pathExists ? "text-text-secondary" : "text-destructive"}`}
                >
                  {profile.path}
                  {!profile.pathExists && " (not found)"}
                </code>
              </div>
            )}
          </div>
        )}

        {activeTab === "live" && (
          <div>
            {isRunning && instance ? (
              <div>
                <div className="mb-3 flex items-center justify-between">
                  <span className="text-sm text-text-muted">
                    {tabs.length} tab(s)
                  </span>
                  <Button size="sm" variant="secondary" onClick={loadLiveData}>
                    Refresh
                  </Button>
                </div>
                {tabs.length === 0 ? (
                  <div className="flex h-[300px] items-center justify-center text-sm text-text-muted">
                    No tabs open
                  </div>
                ) : (
                  <div className="grid gap-3 sm:grid-cols-2">
                    {tabs.map((tab) => (
                      <ScreencastTile
                        key={tab.id}
                        instancePort={instance.port}
                        tabId={tab.id}
                        label={tab.title?.slice(0, 20) || tab.id.slice(0, 8)}
                        url={tab.url}
                      />
                    ))}
                  </div>
                )}
              </div>
            ) : (
              <div className="flex h-[300px] items-center justify-center text-sm text-text-muted">
                Instance not running. Start the profile to see live view.
              </div>
            )}
          </div>
        )}

        {activeTab === "logs" && (
          <div className="space-y-4">
            {/* Tabs */}
            <div>
              <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">
                Tabs ({tabs.length})
              </h4>
              {tabs.length === 0 ? (
                <p className="text-sm text-text-muted">
                  {isRunning ? "No tabs open." : "Instance not running."}
                </p>
              ) : (
                <div className="space-y-1">
                  {tabs.map((tab) => (
                    <TabItem key={tab.id} tab={tab} compact />
                  ))}
                </div>
              )}
            </div>

            {/* Agents */}
            <div>
              <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">
                Agents
              </h4>
              <p className="text-sm text-text-muted">
                Agent registry is not exposed by the current backend.
              </p>
            </div>

            {/* Logs placeholder */}
            <div>
              <h4 className="mb-2 text-xs font-semibold uppercase tracking-wide text-text-muted">
                Logs
              </h4>
              {logs ? (
                <pre className="max-h-[200px] overflow-auto rounded bg-bg-elevated p-3 font-mono text-xs text-text-secondary">
                  {logs}
                </pre>
              ) : (
                <p className="text-sm text-text-muted">
                  No instance logs available.
                </p>
              )}
            </div>
          </div>
        )}
      </div>
    </Modal>
  );
}
