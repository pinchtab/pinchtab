import { useAppStore } from "../../stores/useAppStore";
import { resumeTab } from "../../services/api";

export default function HandoffNotifications() {
  const notifications = useAppStore((state) => state.handoffNotifications);
  const dismiss = useAppStore((state) => state.dismissHandoffNotification);

  if (notifications.length === 0) {
    return null;
  }

  const handleResume = async (tabId: string) => {
    try {
      await resumeTab(tabId);
      dismiss(tabId);
    } catch (err) {
      console.error("Failed to resume tab", err);
    }
  };

  return (
    <div className="fixed bottom-4 right-4 z-50 flex w-96 max-w-full flex-col gap-2">
      {notifications.map((n) => (
        <div
          key={n.tabId}
          className="rounded-sm border border-warning/50 bg-warning/10 p-3 text-sm shadow-lg"
        >
          <div className="mb-1 flex items-start justify-between gap-2">
            <div className="font-semibold text-warning">
              Human intervention required
            </div>
            <button
              type="button"
              onClick={() => dismiss(n.tabId)}
              className="text-text-muted hover:text-text-primary"
              aria-label="Dismiss notification"
            >
              ×
            </button>
          </div>
          <div className="mb-1 text-text-primary">
            <code className="text-xs">{n.tabId}</code>
            {n.title && <span className="ml-2 text-text-muted">{n.title}</span>}
          </div>
          <div className="mb-2 text-text-secondary">
            <span className="text-text-muted">Reason:</span>{" "}
            <code className="text-xs">{n.reason}</code>
            {n.source && (
              <span className="ml-2 text-text-muted">via {n.source}</span>
            )}
          </div>
          {n.hint && (
            <div className="mb-2 text-xs text-text-muted">{n.hint}</div>
          )}
          <div className="flex justify-end gap-2">
            <button
              type="button"
              onClick={() => handleResume(n.tabId)}
              className="rounded-sm border border-border-subtle px-3 py-1 text-xs text-text-primary transition-all duration-150 hover:border-primary/30 hover:bg-bg-elevated"
            >
              Resume
            </button>
          </div>
        </div>
      ))}
    </div>
  );
}
