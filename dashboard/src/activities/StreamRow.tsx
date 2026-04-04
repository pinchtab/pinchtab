import {
  IconCamera,
  IconCompass,
  IconFileText,
  IconHandClick,
  IconKeyboard,
  IconMessageCircle,
  IconPointer,
  IconScreenShare,
} from "../components/atoms/Icon";
import { Badge } from "../components/atoms";
import { activityMethodVariant, activityStatusVariant } from "./helpers";
import type { ActivityFilters, DashboardActivityEvent } from "./types";
import CopyIdPill from "./CopyIdPill";
import FilterPill from "./FilterPill";

function formatTime(ts: string): string {
  return new Date(ts).toLocaleTimeString("en-GB", {
    hour: "2-digit",
    minute: "2-digit",
    second: "2-digit",
  });
}

function EventIcon({ event }: { event: DashboardActivityEvent }) {
  if (event.channel === "progress") return <IconMessageCircle />;
  if (event.action === "click" || event.action === "dblclick")
    return <IconHandClick />;
  if (event.action === "type") return <IconKeyboard />;
  if (event.action === "hover") return <IconPointer />;
  if (event.path.includes("/navigate")) return <IconCompass />;
  if (event.path.includes("/snapshot")) return <IconCamera />;
  if (event.path.includes("/screencast")) return <IconScreenShare />;
  return <IconFileText />;
}

function compactId(value: string): string {
  if (value.length <= 12) {
    return value;
  }
  return `${value.slice(0, 4)}…${value.slice(-4)}`;
}

function isDefaultProfile(name: string | undefined): boolean {
  if (!name) return true;
  const lower = name.toLowerCase();
  return lower === "default" || lower === "chrome-profile";
}

function quoted(value: string): string {
  return `"${value}"`;
}

function eventSummary(event: DashboardActivityEvent): string {
  if (event.channel === "progress" && event.message) {
    return event.message;
  }
  if (event.path.includes("/navigate")) {
    return event.url ? `Navigate to ${event.url}` : "Navigate to page";
  }

  if (event.path.includes("/snapshot")) {
    return "Capture page snapshot";
  }

  if (event.path.includes("/screencast")) {
    return "Open screencast stream";
  }

  if (event.path.includes("/text")) {
    return "Extract text from page";
  }

  switch (event.action) {
    case "click":
      return event.ref ? `Click ${quoted(event.ref)}` : "Click on page";
    case "dblclick":
      return event.ref
        ? `Double-click ${quoted(event.ref)}`
        : "Double-click on page";
    case "type":
      return event.ref ? `Type into ${quoted(event.ref)}` : "Type into page";
    case "hover":
      return event.ref ? `Hover ${quoted(event.ref)}` : "Hover on page";
    case "fill":
      return event.ref ? `Fill ${quoted(event.ref)}` : "Fill field";
    case "select":
      return event.ref ? `Select ${quoted(event.ref)}` : "Select option";
    case "scroll":
      return "Scroll page";
    case "press":
      return event.ref ? `Press key on ${quoted(event.ref)}` : "Press key";
    case "wait":
      return "Wait for condition";
    case "evaluate":
      return "Evaluate JavaScript";
    case "upload":
      return "Upload file";
    case "download":
      return "Download file";
    default:
      if (event.action) {
        return `${event.action} ${event.ref ? quoted(event.ref) : ""}`.trim();
      }
      return `${event.method} ${event.path}`;
  }
}

interface StreamRowProps {
  event: DashboardActivityEvent;
  copyTabId?: boolean;
  hideAgentFilter?: boolean;
  simplifyMeta?: boolean;
  onFilterChange: (key: keyof ActivityFilters, value: string) => void;
}

export default function StreamRow({
  event,
  copyTabId = false,
  hideAgentFilter = false,
  simplifyMeta = false,
  onFilterChange,
}: StreamRowProps) {
  if (simplifyMeta) {
    return (
      <div className="px-4 py-4">
        <div className="flex items-start gap-3">
          <div className="flex h-10 w-10 shrink-0 items-center justify-center rounded-full border border-primary/20 bg-primary/10 text-lg shadow-[0_0_24px_rgb(var(--brand-accent-rgb)/0.08)]">
            <EventIcon event={event} />
          </div>
          <div className="min-w-0 flex-1">
            <div className="mb-2 flex flex-wrap items-center gap-2 text-[0.68rem] text-text-muted">
              <span className="dashboard-mono text-[0.68rem] text-text-muted">
                {formatTime(event.timestamp)}
              </span>
              <span className="dashboard-mono">·</span>
              <span className="dashboard-mono">{event.durationMs}ms</span>
            </div>

            <div className="border-l border-border-subtle pl-4">
              <div className="text-[0.95rem] leading-6 text-text-primary">
                {eventSummary(event)}
                {event.tabId && event.channel !== "progress" && (
                  <>
                    {" on tab "}
                    {copyTabId ? (
                      <CopyIdPill id={event.tabId} compact inline />
                    ) : (
                      <span className="dashboard-mono">
                        {compactId(event.tabId)}
                      </span>
                    )}
                  </>
                )}
              </div>

              <div className="mt-4 flex flex-wrap items-center gap-2">
                {event.profileName && !isDefaultProfile(event.profileName) && (
                  <FilterPill
                    label={`profile:${event.profileName}`}
                    onClick={() =>
                      onFilterChange("profileName", event.profileName || "")
                    }
                  />
                )}
              </div>
            </div>
          </div>
        </div>
      </div>
    );
  }

  return (
    <div className="border-b border-border-subtle/70 px-4 py-3 text-sm transition-colors hover:bg-white/2">
      <div className="flex items-start gap-3">
        <span className="pt-0.5 text-text-muted">
          <EventIcon event={event} />
        </span>
        <span className="dashboard-mono w-18 shrink-0 pt-0.5 text-xs text-text-muted">
          {formatTime(event.timestamp)}
        </span>
        <div className="min-w-0 flex-1">
          <div className="flex flex-wrap items-center gap-2">
            <Badge variant={activityMethodVariant(event.method)}>
              {event.method}
            </Badge>
            <Badge variant={activityStatusVariant(event.status)}>
              {event.status}
            </Badge>
            {event.source && <Badge>{event.source}</Badge>}
            {event.action && <Badge variant="warning">{event.action}</Badge>}
            <span className="dashboard-mono min-w-0 flex-1 truncate text-text-secondary">
              {event.path}
            </span>
          </div>

          <div className="mt-1 flex flex-wrap items-center gap-1.5">
            {event.tabId &&
              (copyTabId ? (
                <CopyIdPill id={event.tabId} />
              ) : (
                <FilterPill
                  label={`tab:${event.tabId}`}
                  onClick={() => onFilterChange("tabId", event.tabId || "")}
                />
              ))}
            {event.profileName && !isDefaultProfile(event.profileName) && (
              <FilterPill
                label={`profile:${event.profileName}`}
                onClick={() =>
                  onFilterChange("profileName", event.profileName || "")
                }
              />
            )}
            {event.url && (
              <span className="dashboard-mono min-w-0 truncate text-[0.68rem] text-text-muted">
                {event.url}
              </span>
            )}
          </div>
        </div>

        <div className="flex shrink-0 flex-col items-end gap-1.5">
          <span className="dashboard-mono text-xs text-text-muted">
            {event.durationMs}ms
          </span>
          {!hideAgentFilter && event.agentId && (
            <FilterPill
              label={`agent:${event.agentId}`}
              onClick={() => onFilterChange("agentId", event.agentId || "")}
            />
          )}
        </div>
      </div>
    </div>
  );
}
