import { useAppStore } from "../../stores/useAppStore";
import { Card } from "../atoms";

export default function ServerSummary() {
  const { serverInfo } = useAppStore();

  if (!serverInfo) return null;

  const uptimeStr = (uptimeMs: number) => {
    const seconds = Math.floor(uptimeMs / 1000);
    const h = Math.floor(seconds / 3600);
    const m = Math.floor((seconds % 3600) / 60);
    const s = seconds % 60;
    if (h > 0) return `${h}h ${m}m ${s}s`;
    if (m > 0) return `${m}m ${s}s`;
    return `${s}s`;
  };

  // const copyToClipboard = (text: string) => {
  //   navigator.clipboard.writeText(text);
  // };

  return (
    <div className="flex flex-col gap-4">
      <Card className="flex flex-col gap-4 p-4">
        <div className="border-b border-border-subtle pb-3">
          <div className="dashboard-section-label mb-2">Settings</div>
          <h2 className="text-[0.72rem] font-bold tracking-[0.18em] text-text-muted uppercase">
            Server Information
          </h2>
          <p className="mt-1 text-[0.7rem] text-text-muted italic opacity-70">
            Technical details for current session
          </p>
        </div>

        <div className="flex flex-col gap-1">
          <label className="dashboard-section-title text-[0.68rem]">
            Version
          </label>
          <div className="dashboard-mono text-sm text-text-secondary">
            {serverInfo.version}
          </div>
        </div>

        <div className="flex flex-col gap-1">
          <label className="dashboard-section-title text-[0.68rem]">
            Uptime
          </label>
          <div className="dashboard-mono text-sm text-text-secondary">
            {uptimeStr(serverInfo.uptime)}
          </div>
        </div>

        {/* {serverInfo.configPath && (
          <div className="flex flex-col gap-0.5">
            <label className="text-[10px] font-semibold text-text-muted uppercase tracking-tight">
              Config
            </label>
            <div
              className="group flex cursor-pointer items-center justify-between rounded bg-bg-elevated px-2 py-1 transition-colors hover:bg-border-subtle"
              onClick={() => copyToClipboard(serverInfo.configPath!)}
              title="Click to copy path"
            >
              <span className="truncate text-[10px] font-mono text-text-muted">
                {serverInfo.configPath.split("/").pop()}
              </span>
              <span className="text-[10px] opacity-0 group-hover:opacity-100 transition-opacity">
                📋
              </span>
            </div>
          </div>
        )} */}
      </Card>
    </div>
  );
}
