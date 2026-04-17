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
import { activityStatusVariant } from "./helpers";
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

function quoted(value: string): string {
  return `"${value}"`;
}

function eventDescription(event: DashboardActivityEvent): string {
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
  if (event.path.includes("/screenshot")) {
    return "Take screenshot";
  }
  if (event.path.includes("/pdf")) {
    return "Export page as PDF";
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

const statusColor: Record<string, string> = {
  success: "text-success",
  warning: "text-warning",
  danger: "text-destructive",
  default: "text-text-muted",
};

interface Props {
  event: DashboardActivityEvent;
  showTab?: boolean;
  copyTabId?: boolean;
  sessionLabel?: string;
  onFilterChange?: (key: keyof ActivityFilters, value: string) => void;
}

export default function ActivityItemLine({
  event,
  showTab = true,
  copyTabId = false,
  sessionLabel,
  onFilterChange,
}: Props) {
  const variant = activityStatusVariant(event.status);

  return (
    <div className="flex items-center gap-2.5 px-4 py-2 text-sm transition-colors hover:bg-white/2">
      <span className="shrink-0 text-text-muted">
        <EventIcon event={event} />
      </span>

      <span className="dashboard-mono w-16 shrink-0 text-[0.68rem] text-text-muted">
        {formatTime(event.timestamp)}
      </span>

      <span className="min-w-0 flex-1 truncate text-text-primary">
        {eventDescription(event)}
      </span>

      {sessionLabel && (
        <span className="truncate rounded-sm border border-border-subtle bg-white/3 px-1.5 py-0.5 text-[0.68rem] text-text-muted">
          {sessionLabel}
        </span>
      )}

      {showTab &&
        event.tabId &&
        onFilterChange &&
        (copyTabId ? (
          <CopyIdPill id={event.tabId} compact />
        ) : (
          <FilterPill
            label={`tab:${event.tabId}`}
            onClick={() => onFilterChange("tabId", event.tabId || "")}
          />
        ))}

      <span
        className={`dashboard-mono shrink-0 text-[0.68rem] ${statusColor[variant] || "text-text-muted"}`}
      >
        {event.status}
      </span>
    </div>
  );
}
