import { useState } from "react";
import { useTranslation } from "react-i18next";
import { useAppStore } from "../../stores/useAppStore";

export default function DebugPanel() {
  const { t } = useTranslation();
  const [open, setOpen] = useState(false);
  const store = useAppStore();

  if (!open) {
    return (
      <button
        onClick={() => setOpen(true)}
        className="fixed bottom-2 right-2 z-50 rounded bg-yellow-500 px-2 py-1 text-xs text-black"
      >
        {t("debugPanel.toggle")}
      </button>
    );
  }

  return (
    <div className="fixed bottom-2 right-2 z-50 max-h-96 w-80 overflow-auto rounded border border-yellow-500 bg-black/90 p-2 text-xs text-green-400 font-mono">
      <div className="flex justify-between mb-2">
        <span className="text-yellow-500 font-bold">
          {t("debugPanel.title")}
        </span>
        <button onClick={() => setOpen(false)} className="text-white">
          ✕
        </button>
      </div>
      <div className="space-y-1">
        <div>
          {t("debugPanel.instances")} {store.instances?.length ?? "null"}
        </div>
        <div>
          {t("debugPanel.profiles")} {store.profiles?.length ?? "null"}
        </div>
        <div>
          {t("debugPanel.agents")} {store.agents?.length ?? "null"}
        </div>
        <div>
          {t("debugPanel.tabsChartData")}{" "}
          {store.tabsChartData?.length ?? "null"}
        </div>
        <div>
          {t("debugPanel.serverChartData")}{" "}
          {store.serverChartData?.length ?? "null"}
        </div>
        <div>
          {t("debugPanel.memoryChartData")}{" "}
          {store.memoryChartData?.length ?? "null"}
        </div>
        <div>
          {t("debugPanel.selectedAgentId")} {store.selectedAgentId ?? "null"}
        </div>
        <div>
          {t("debugPanel.profilesLoading")} {String(store.profilesLoading)}
        </div>
        <div>
          {t("debugPanel.instancesLoading")} {String(store.instancesLoading)}
        </div>
        <div className="mt-2 text-yellow-500">
          {t("debugPanel.instancesHeading")}
        </div>
        {store.instances?.map((i) => (
          <div key={i.id} className="ml-2 text-gray-400">
            {i.id}: {i.status} ({i.profileName})
          </div>
        ))}
      </div>
    </div>
  );
}
