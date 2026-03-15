import { useState, useEffect, useCallback } from "react";
import InstanceLogsPanel from "./InstanceLogsPanel";
import ProfileMetaInfoPanel from "./ProfileMetaInfoPanel";
import ProfileBasicInfoPanel from "./ProfileBasicInfoPanel";
import ProfileLiveViewPanel from "./ProfileLiveViewPanel";
import ProfileToolbar from "./ProfileToolbar";
import { InstanceTabsPanel } from "../tabs";
import { TabsLayout, EmptyView } from "../molecules";
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

type TabId = "profile" | "live" | "tabs" | "logs";

export default function ProfileDetailsPanel({
  profile,
  instance,
  onLaunch,
  onStop,
  onSave,
  onDelete,
}: Props) {
  const [activeTab, setActiveTab] = useState<TabId>("profile");
  const [tabs, setTabs] = useState<InstanceTab[]>([]);
  const [formValues, setFormValues] = useState({ name: "", useWhen: "" });

  const isRunning = instance?.status === "running";

  useEffect(() => {
    if (profile) {
      setFormValues({ name: profile.name, useWhen: profile.useWhen || "" });
    } else {
      setTabs([]);
      setFormValues({ name: "", useWhen: "" });
    }
  }, [profile]);

  const handleProfileChange = useCallback((name: string, useWhen: string) => {
    setFormValues({ name, useWhen });
  }, []);

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

  const handleSave = () => {
    onSave?.(formValues.name, formValues.useWhen);
  };

  if (!profile) {
    return (
      <div className="dashboard-panel h-full min-h-112">
        <EmptyView message="Select a profile to inspect its instance, live tabs, and logs." />
      </div>
    );
  }

  const hasChanges =
    formValues.name.trim() !== profile.name ||
    formValues.useWhen !== (profile.useWhen || "");

  const profileTabs: { id: TabId; label: string; badge?: string | number }[] = [
    { id: "profile", label: "Profile" },
    { id: "live", label: "Live" },
    { id: "tabs", label: "Tabs", badge: tabs.length },
    { id: "logs", label: "Logs" },
  ];

  return (
    <div className="dashboard-panel flex h-full min-h-112 flex-col overflow-hidden">
      <div className="border-b border-border-subtle px-4 py-3 lg:px-5">
        <ProfileToolbar
          profile={profile}
          instance={instance}
          onLaunch={onLaunch}
          onStop={onStop || (() => {})}
          onSave={handleSave}
          onDelete={onDelete || (() => {})}
          isSaveDisabled={!formValues.name.trim() || !hasChanges}
        />
      </div>

      <div className="min-h-0 flex-1 overflow-hidden">
        <TabsLayout
          tabs={profileTabs}
          activeTab={activeTab}
          onChange={(id) => setActiveTab(id)}
        >
          {activeTab === "profile" && (
            <div className="h-full overflow-auto">
              <div className="grid gap-4 xl:grid-cols-2">
                <ProfileBasicInfoPanel
                  profile={profile}
                  onChange={handleProfileChange}
                />

                <ProfileMetaInfoPanel profile={profile} instance={instance} />
              </div>
            </div>
          )}

          {activeTab === "live" && (
            <ProfileLiveViewPanel
              instance={instance}
              tabs={tabs}
              isRunning={isRunning}
            />
          )}

          {activeTab === "tabs" && (
            <div className="flex h-full min-h-0 flex-col">
              <InstanceTabsPanel
                tabs={tabs}
                instanceId={instance?.id}
                emptyMessage={
                  isRunning ? "No tabs open." : "Instance not running."
                }
              />
            </div>
          )}

          {activeTab === "logs" && (
            <InstanceLogsPanel instanceId={instance?.id} />
          )}
        </TabsLayout>
      </div>
    </div>
  );
}
