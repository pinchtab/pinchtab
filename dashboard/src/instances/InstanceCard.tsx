import { Card, Badge, Button, StatusDot } from "../components/atoms";
import type { Instance } from "../types";
import { formatUptime } from "../lib/format";

interface Props {
  instance: Instance;
  onOpen: () => void;
  onStop: () => void;
}

export default function InstanceCard({ instance, onOpen, onStop }: Props) {
  return (
    <Card hover className="flex flex-col p-4">
      <div className="mb-3 flex items-start justify-between">
        <div className="flex items-center gap-2">
          <StatusDot status="online" />
          <div>
            <div className="font-medium text-text-primary">
              {instance.profileName}
            </div>
            <div className="text-xs text-text-muted">:{instance.port}</div>
          </div>
        </div>
        <div className="flex items-center gap-1">
          <Badge variant="default">{instance.browser || "chrome"}</Badge>
          <Badge variant={instance.headless ? "info" : "default"}>
            {instance.headless ? "Headless" : "Headed"}
          </Badge>
        </div>
      </div>

      <div className="mb-3 rounded-sm border border-border-subtle bg-[rgb(var(--brand-surface-code-rgb)/0.4)] px-3 py-2 text-xs">
        <span className="dashboard-section-title mr-2 text-[0.68rem]">
          Uptime
        </span>
        <span className="dashboard-mono text-text-secondary">
          {formatUptime(instance.startTime)}
        </span>
      </div>

      <div className="mt-auto flex gap-2">
        <Button size="sm" variant="primary" className="flex-1" onClick={onOpen}>
          Open Dashboard
        </Button>
        <Button size="sm" variant="danger" onClick={onStop}>
          Stop
        </Button>
      </div>
    </Card>
  );
}
