import { Card, Badge, Button } from "../atoms";
import type { Profile, Instance } from "../../types";

interface Props {
  profile: Profile;
  instance?: Instance;
  onLaunch: () => void;
  onStop?: () => void;
  onDetails?: () => void;
}

function InfoRow({ label, value }: { label: string; value: string }) {
  return (
    <div className="flex items-center justify-between gap-3 text-xs">
      <span className="dashboard-section-title text-[0.68rem]">{label}</span>
      <span className="dashboard-mono text-text-secondary">{value}</span>
    </div>
  );
}

export default function ProfileCard({
  profile,
  instance,
  onLaunch,
  onStop,
  onDetails,
}: Props) {
  const isRunning = instance?.status === "running";
  const isError = instance?.status === "error";
  const accountText = profile.accountEmail || profile.accountName || "—";
  const sizeText = profile.sizeMB ? `${profile.sizeMB.toFixed(0)} MB` : "—";

  return (
    <Card hover className="flex flex-col">
      {/* Header */}
      <div className="flex items-center justify-between border-b border-border-subtle bg-black/10 px-4 py-3">
        <span className="truncate font-medium text-text-primary">
          {profile.name}
        </span>
        {isRunning ? (
          <Badge variant="success">:{instance.port}</Badge>
        ) : isError ? (
          <Badge variant="danger">error</Badge>
        ) : (
          <Badge>stopped</Badge>
        )}
      </div>

      {/* Body */}
      <div className="flex flex-1 flex-col gap-2 px-4 py-4">
        <InfoRow label="Size" value={sizeText} />
        <InfoRow label="Account" value={accountText} />
        {profile.useWhen && (
          <div className="mt-2 rounded-sm border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.4)] p-3">
            <div className="dashboard-section-title text-[0.68rem]">
              Use when
            </div>
            <div className="mt-1 line-clamp-2 text-xs leading-5 text-text-secondary">
              {profile.useWhen}
            </div>
          </div>
        )}
        {isError && instance?.error && (
          <div className="mt-1 text-xs text-destructive">{instance.error}</div>
        )}
      </div>

      {/* Actions */}
      <div className="flex justify-end gap-2 border-t border-border-subtle bg-black/10 px-4 py-3">
        {onDetails && (
          <Button size="sm" variant="ghost" onClick={onDetails}>
            Details
          </Button>
        )}
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
    </Card>
  );
}
