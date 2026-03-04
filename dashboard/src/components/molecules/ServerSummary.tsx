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

  const copyToClipboard = (text: string) => {
    navigator.clipboard.writeText(text);
  };

  return (
    <div className="flex flex-col gap-4">
      <div className="flex flex-col gap-1 px-1">
        <h2 className="text-xs font-bold tracking-widest text-text-muted uppercase">
          Server Information
        </h2>
        <p className="text-[10px] text-text-muted italic">
          Technical details for current session
        </p>
      </div>

      <Card className="p-3 flex flex-col gap-3 bg-bg-surface/50">
        <div className="flex flex-col gap-0.5">
          <label className="text-[10px] font-semibold text-text-muted uppercase tracking-tight">
            Version
          </label>
          <div className="text-sm font-mono text-text-secondary">
            {serverInfo.version}
          </div>
        </div>

        <div className="flex flex-col gap-0.5">
          <label className="text-[10px] font-semibold text-text-muted uppercase tracking-tight">
            Uptime
          </label>
          <div className="text-sm text-text-secondary">
            {uptimeStr(serverInfo.uptime)}
          </div>
        </div>

        {serverInfo.strategy && (
          <div className="flex flex-col gap-0.5">
            <label className="text-[10px] font-semibold text-text-muted uppercase tracking-tight">
              Strategy
            </label>
            <div className="flex items-center gap-1.5">
              <span className="rounded bg-primary/10 px-1.5 py-0.5 text-[10px] font-bold text-primary uppercase">
                {serverInfo.strategy}
              </span>
            </div>
          </div>
        )}

        {serverInfo.allocationPolicy && (
          <div className="flex flex-col gap-0.5">
            <label className="text-[10px] font-semibold text-text-muted uppercase tracking-tight">
              Policy
            </label>
            <div className="text-sm text-text-secondary italic">
              {serverInfo.allocationPolicy}
            </div>
          </div>
        )}

        {serverInfo.configPath && (
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
        )}
      </Card>

      <div className="p-3 grid grid-cols-2 gap-2 border-t border-border-subtle mt-2">
        <div className="flex flex-col">
          <span className="text-[10px] text-text-muted">Profiles</span>
          <span className="text-sm font-semibold text-text-secondary">
            {serverInfo.profiles}
          </span>
        </div>
        <div className="flex flex-col">
          <span className="text-[10px] text-text-muted">Instances</span>
          <span className="text-sm font-semibold text-text-secondary">
            {serverInfo.instances}
          </span>
        </div>
      </div>
    </div>
  );
}
